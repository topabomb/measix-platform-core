package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/budgetallocation"
	"measix/platform/ent/budgetreconciliation"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/requestusage"
	"measix/platform/pkg/platformid"
)

const reconciliationBatchSize = 100

var ErrStaleReconciliationPreview = errors.New("pending reconciliations changed since preview")

type ReconciliationBatchResult struct {
	Matched     int
	Resolved    int
	PreviewHash string
}

// ResolveReconciliationBatch previews when expectedHash is empty. An execution
// checks the same selection under the write transaction before changing rows.
func (s *Service) ResolveReconciliationBatch(ctx context.Context, from, to *time.Time, actor, reason, expectedHash string) (ReconciliationBatchResult, error) {
	if utf8.RuneCountInString(reason) > 500 {
		return ReconciliationBatchResult{}, ErrInvalidConfiguration
	}
	reason = strings.TrimSpace(reason)
	if (from == nil) != (to == nil) || from != nil && !from.Before(*to) ||
		(expectedHash == "") != (reason == "") ||
		platformid.Validate(platformid.User, actor) != nil {
		return ReconciliationBatchResult{}, ErrInvalidConfiguration
	}
	if expectedHash == "" {
		_, result, err := reconciliationBatchTargets(ctx, s.Client, from, to)
		return result, err
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return ReconciliationBatchResult{}, err
	}
	defer tx.Rollback()
	rows, result, err := reconciliationBatchTargets(ctx, tx.Client(), from, to)
	if err != nil {
		return ReconciliationBatchResult{}, err
	}
	if result.PreviewHash != expectedHash {
		return ReconciliationBatchResult{}, ErrStaleReconciliationPreview
	}
	now := s.Now().UTC()
	for chunk := range slices.Chunk(rows, reconciliationBatchSize) {
		if err := resolveReconciliationChunk(ctx, tx, chunk, actor, reason, now); err != nil {
			return ReconciliationBatchResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationBatchResult{}, err
	}
	result.Resolved = result.Matched
	return result, nil
}

func reconciliationBatchTargets(ctx context.Context, client *ent.Client, from, to *time.Time) ([]*ent.BudgetReconciliation, ReconciliationBatchResult, error) {
	query := client.BudgetReconciliation.Query().Where(budgetreconciliation.StateEQ(budgetreconciliation.StateOPEN))
	if from != nil {
		query.Where(func(s *sql.Selector) {
			request := sql.Table(budgetrequest.Table)
			usage := sql.Table(requestusage.Table)
			s.Join(request).On(s.C(budgetreconciliation.FieldRequestID), request.C(budgetrequest.FieldID)).
				LeftJoin(usage).On(request.C(budgetrequest.FieldID), usage.C(requestusage.FieldRequestID))
			// Match the displayed request time, including legacy rows without usage or a start event.
			start := fmt.Sprintf("COALESCE(%s, %s, %s)", usage.C(requestusage.FieldStartedAt), request.C(budgetrequest.FieldStartedAt), request.C(budgetrequest.FieldAdmittedAt))
			s.Where(sql.ExprP(start+" >= ? AND "+start+" < ?", from.UTC(), to.UTC()))
		})
	}
	rows, err := query.Order(ent.Asc(budgetreconciliation.FieldID)).
		Select(budgetreconciliation.FieldID, budgetreconciliation.FieldRequestID).All(ctx)
	if err != nil {
		return nil, ReconciliationBatchResult{}, err
	}
	hash := sha256.New()
	if from != nil {
		fmt.Fprintln(hash, from.UTC(), to.UTC())
	}
	for _, row := range rows {
		fmt.Fprintf(hash, "%d:%s\n", row.ID, row.RequestID)
	}
	return rows, ReconciliationBatchResult{Matched: len(rows), PreviewHash: hex.EncodeToString(hash.Sum(nil))}, nil
}

func resolveReconciliationChunk(ctx context.Context, tx *ent.Tx, rows []*ent.BudgetReconciliation, actor, reason string, now time.Time) error {
	reconciliationIDs := make([]int, 0, len(rows))
	requestIDs := make([]string, 0, len(rows))
	byRequest := make(map[string]int, len(rows))
	for _, row := range rows {
		reconciliationIDs = append(reconciliationIDs, row.ID)
		requestIDs = append(requestIDs, row.RequestID)
		byRequest[row.RequestID] = row.ID
	}
	requests, err := tx.BudgetRequest.Query().Where(budgetrequest.IDIn(requestIDs...)).
		Select(budgetrequest.FieldID, budgetrequest.FieldUserID, budgetrequest.FieldCapability,
			budgetrequest.FieldUserBudgetID, budgetrequest.FieldBudgetRevision, budgetrequest.FieldState, budgetrequest.FieldLastSettlementRevision).
		All(ctx)
	if err != nil {
		return err
	}
	if len(requests) != len(rows) {
		return ErrRequestNotFound
	}
	audits := make([]*ent.BudgetAuditCreate, 0, len(requests))
	for _, request := range requests {
		audit, err := resolutionAudit(tx, request, byRequest[request.ID], ResolveInput{
			RequestID: request.ID, Action: ResolutionReleaseUncertain, Revision: request.LastSettlementRevision + 1,
			ActorUserID: actor, Reason: reason,
		}, now)
		if err != nil {
			return err
		}
		audits = append(audits, audit)
	}
	if err := resolveUncertain(ctx, tx, requestIDs, reconciliationIDs, actor, reason, now); err != nil {
		return err
	}
	return tx.BudgetAudit.CreateBulk(audits...).Exec(ctx)
}

func resolveUncertain(ctx context.Context, tx *ent.Tx, requestIDs []string, reconciliationIDs []int, actor, reason string, now time.Time) error {
	allocations, err := tx.BudgetAllocation.Query().
		Where(budgetallocation.RequestIDIn(requestIDs...), budgetallocation.ReservationReleasedEQ(false)).All(ctx)
	if err != nil {
		return err
	}
	if err := releaseReservations(ctx, tx, allocations, now); err != nil {
		return err
	}
	if err := tx.BudgetRequest.Update().Where(budgetrequest.IDIn(requestIDs...)).
		SetState(budgetrequest.State(RequestResolved)).
		SetCompletedAt(now).
		SetTerminalReason("uncertain usage released by authorized reconciliation").
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		return err
	}
	return closeReconciliations(ctx, tx, reconciliationIDs, actor, string(ResolutionReleaseUncertain), reason, now)
}
