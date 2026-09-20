package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/budgetallocation"
	"measix/platform/ent/budgetaudit"
	"measix/platform/ent/budgetreconciliation"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/budgetsettlement"
	"measix/platform/pkg/platformid"
)

func (s *Service) Settle(ctx context.Context, input SettlementInput) (SettlementResult, error) {
	quantities, payloadJSON, payloadHash, err := normalizeSettlement(input)
	if err != nil {
		return SettlementResult{}, err
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return SettlementResult{}, err
	}
	defer tx.Rollback()
	result, err := applySettlement(ctx, tx, input, quantities, payloadJSON, payloadHash, "RELAY", input.ReportedBy, "RELIABLE_SETTLEMENT", now)
	if err != nil {
		return SettlementResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SettlementResult{}, err
	}
	return result, nil
}

// SettleInTx applies a Relay settlement inside the caller-owned transaction so
// the request ledger, semantic meters, and budget counters commit atomically.
// The caller owns commit/rollback and supplies the transaction timestamp.
func (s *Service) SettleInTx(ctx context.Context, tx *ent.Tx, input SettlementInput, now time.Time) (SettlementResult, error) {
	if s == nil || tx == nil || now.IsZero() {
		return SettlementResult{}, ErrInvalidSettlement
	}
	quantities, payloadJSON, payloadHash, err := normalizeSettlement(input)
	if err != nil {
		return SettlementResult{}, err
	}
	return applySettlement(ctx, tx, input, quantities, payloadJSON, payloadHash, "RELAY", input.ReportedBy, "RELIABLE_SETTLEMENT", now.UTC())
}

func (s *Service) Resolve(ctx context.Context, input ResolveInput) error {
	if platformid.Validate(platformid.Request, input.RequestID) != nil || platformid.Validate(platformid.User, input.ActorUserID) != nil || strings.TrimSpace(input.Reason) == "" {
		return ErrInvalidConfiguration
	}
	if input.Action != ResolutionConfirmUsage && input.Action != ResolutionReleaseUncertain {
		return ErrInvalidConfiguration
	}
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	request, err := tx.BudgetRequest.Get(ctx, input.RequestID)
	if ent.IsNotFound(err) {
		return ErrRequestNotFound
	}
	if err != nil {
		return err
	}
	reconciliation, err := tx.BudgetReconciliation.Query().Where(
		budgetreconciliation.RequestIDEQ(input.RequestID),
		budgetreconciliation.StateEQ(budgetreconciliation.StateOPEN),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrReconciliationNotOpen
	}
	if err != nil {
		return err
	}
	before, err := json.Marshal(map[string]any{
		"requestId": input.RequestID, "state": request.State, "reconciliationId": reconciliation.ID,
		"lastSettlementRevision": request.LastSettlementRevision,
	})
	if err != nil {
		return err
	}

	switch input.Action {
	case ResolutionConfirmUsage:
		settlement := SettlementInput{
			RequestID: input.RequestID, Revision: input.Revision, Meters: input.Meters,
			Complete: true, ReportedBy: input.ActorUserID,
		}
		quantities, payloadJSON, payloadHash, err := normalizeSettlement(settlement)
		if err != nil {
			return err
		}
		if _, err := applySettlement(ctx, tx, settlement, quantities, payloadJSON, payloadHash, "ADMIN", input.ActorUserID, string(ResolutionConfirmUsage), now); err != nil {
			return err
		}
	case ResolutionReleaseUncertain:
		allocations, err := requestAllocations(ctx, tx, input.RequestID)
		if err != nil {
			return err
		}
		if err := releaseReservations(ctx, tx, allocations, now); err != nil {
			return err
		}
		if _, err := tx.BudgetRequest.UpdateOneID(request.ID).
			SetState(budgetrequest.State(RequestResolved)).
			SetCompletedAt(now).
			SetTerminalReason("uncertain usage released by authorized reconciliation").
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return err
		}
		if err := closeReconciliation(ctx, tx, reconciliation.ID, input.ActorUserID, string(ResolutionReleaseUncertain), input.Reason, now); err != nil {
			return err
		}
	}
	after, err := json.Marshal(map[string]any{
		"requestId": input.RequestID, "action": input.Action, "reason": strings.TrimSpace(input.Reason),
		"revision": input.Revision, "meters": input.Meters,
	})
	if err != nil {
		return err
	}
	if _, err := tx.BudgetAudit.Create().
		SetUserID(request.UserID).
		SetCapability(budgetaudit.Capability(request.Capability)).
		SetNillableUserBudgetID(request.UserBudgetID).
		SetBudgetRevision(request.BudgetRevision).
		SetRequestID(request.ID).
		SetActorUserID(input.ActorUserID).
		SetAction(budgetaudit.ActionRESOLVE_RECONCILIATION).
		SetReason(strings.TrimSpace(input.Reason)).
		SetBeforeJSON(before).
		SetAfterJSON(after).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func applySettlement(
	ctx context.Context,
	tx *ent.Tx,
	input SettlementInput,
	quantities map[Meter]int64,
	payloadJSON []byte,
	payloadHash, source, reportedBy, reconciliationAction string,
	now time.Time,
) (SettlementResult, error) {
	request, err := tx.BudgetRequest.Get(ctx, input.RequestID)
	if ent.IsNotFound(err) {
		return SettlementResult{}, ErrRequestNotFound
	}
	if err != nil {
		return SettlementResult{}, err
	}
	for meter := range quantities {
		if !meterAllowed(Capability(request.Capability), meter) {
			return SettlementResult{}, ErrInvalidSettlement
		}
	}
	existing, err := tx.BudgetSettlement.Query().Where(
		budgetsettlement.RequestIDEQ(input.RequestID),
		budgetsettlement.RevisionEQ(input.Revision),
	).Only(ctx)
	if err == nil {
		if existing.PayloadHash != payloadHash {
			return SettlementResult{}, ErrSettlementRevisionConflict
		}
		return SettlementResult{
			RequestID: input.RequestID, Revision: input.Revision,
			Outcome: SettlementOutcome(existing.Outcome), Duplicate: true,
		}, nil
	}
	if !ent.IsNotFound(err) {
		return SettlementResult{}, err
	}
	if input.Revision <= request.LastSettlementRevision {
		return SettlementResult{}, ErrSettlementRevisionConflict
	}
	state := RequestState(request.State)
	if request.StartedAt == nil || state == RequestAdmitted || state == RequestDenied || state == RequestReleased {
		return SettlementResult{}, ErrRequestNotStarted
	}
	if !input.Complete && (state == RequestSettled || state == RequestResolved) {
		return SettlementResult{}, ErrInvalidTransition
	}
	allocations, err := requestAllocations(ctx, tx, input.RequestID)
	if err != nil {
		return SettlementResult{}, err
	}
	if input.Complete {
		for _, allocation := range allocations {
			if _, ok := quantities[Meter(allocation.Meter)]; !ok {
				return SettlementResult{}, fmt.Errorf("%w: complete settlement omits %s", ErrInvalidSettlement, allocation.Meter)
			}
		}
	}
	for _, allocation := range allocations {
		quantity, present := quantities[Meter(allocation.Meter)]
		if !present {
			continue
		}
		bucket, err := tx.BudgetBucket.Get(ctx, allocation.BudgetBucketID)
		if err != nil {
			return SettlementResult{}, err
		}
		delta := quantity - allocation.SettledQuantity
		nextSettled := bucket.SettledQuantity + delta
		if nextSettled < 0 {
			return SettlementResult{}, ErrInvalidSettlement
		}
		nextReserved := bucket.ReservedQuantity
		if !allocation.ReservationReleased {
			if nextReserved < allocation.ReservedQuantity {
				return SettlementResult{}, ErrInvalidSettlement
			}
			nextReserved -= allocation.ReservedQuantity
		}
		if _, err := tx.BudgetBucket.UpdateOneID(bucket.ID).
			SetSettledQuantity(nextSettled).
			SetReservedQuantity(nextReserved).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return SettlementResult{}, err
		}
		if _, err := tx.BudgetAllocation.UpdateOneID(allocation.ID).
			SetSettledQuantity(quantity).
			SetReservationReleased(true).
			SetResolved(true).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return SettlementResult{}, err
		}
	}

	outcome := OutcomeReconciliation
	nextState := RequestReconciliation
	if input.Complete {
		outcome = OutcomeSettled
		if request.LastSettlementRevision > 0 {
			outcome = OutcomeCorrected
		}
		nextState = RequestSettled
	}
	if _, err := tx.BudgetSettlement.Create().
		SetRequestID(input.RequestID).
		SetRevision(input.Revision).
		SetPayloadHash(payloadHash).
		SetMetersJSON(payloadJSON).
		SetComplete(input.Complete).
		SetSource(budgetsettlement.Source(source)).
		SetOutcome(budgetsettlement.Outcome(outcome)).
		SetReportedBy(reportedBy).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return SettlementResult{}, err
	}
	requestUpdate := tx.BudgetRequest.UpdateOneID(request.ID).
		SetState(budgetrequest.State(nextState)).
		SetLastSettlementRevision(input.Revision).
		SetUpdatedAt(now)
	if input.Complete {
		requestUpdate.SetCompletedAt(now).ClearTerminalReason()
	}
	if _, err := requestUpdate.Save(ctx); err != nil {
		return SettlementResult{}, err
	}
	open, err := tx.BudgetReconciliation.Query().Where(
		budgetreconciliation.RequestIDEQ(input.RequestID),
		budgetreconciliation.StateEQ(budgetreconciliation.StateOPEN),
	).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return SettlementResult{}, err
	}
	if input.Complete {
		if err == nil {
			if err := closeReconciliation(ctx, tx, open.ID, reportedBy, reconciliationAction, "reliable complete settlement", now); err != nil {
				return SettlementResult{}, err
			}
		}
	} else if ent.IsNotFound(err) {
		if _, err := tx.BudgetReconciliation.Create().
			SetRequestID(input.RequestID).
			SetState(budgetreconciliation.StateOPEN).
			SetReason(fmt.Sprintf("settlement revision %d is incomplete", input.Revision)).
			SetOpenedAt(now).
			Save(ctx); err != nil {
			return SettlementResult{}, err
		}
	}
	return SettlementResult{RequestID: input.RequestID, Revision: input.Revision, Outcome: outcome}, nil
}

func normalizeSettlement(input SettlementInput) (map[Meter]int64, []byte, string, error) {
	if platformid.Validate(platformid.Request, input.RequestID) != nil || input.Revision <= 0 || strings.TrimSpace(input.ReportedBy) == "" {
		return nil, nil, "", ErrInvalidSettlement
	}
	quantities := make(map[Meter]int64, len(input.Meters))
	meters := append([]MeterQuantity(nil), input.Meters...)
	for _, quantity := range meters {
		if !validMeter(quantity.Meter) || quantity.Quantity < 0 {
			return nil, nil, "", ErrInvalidSettlement
		}
		if _, duplicate := quantities[quantity.Meter]; duplicate {
			return nil, nil, "", ErrInvalidSettlement
		}
		quantities[quantity.Meter] = quantity.Quantity
	}
	if requests, ok := quantities[MeterRequests]; !ok || requests != 1 {
		return nil, nil, "", fmt.Errorf("%w: REQUESTS must equal one", ErrInvalidSettlement)
	}
	sort.Slice(meters, func(i, j int) bool { return meters[i].Meter < meters[j].Meter })
	payload := struct {
		Complete bool            `json:"complete"`
		Meters   []MeterQuantity `json:"meters"`
	}{Complete: input.Complete, Meters: meters}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(encoded)
	return quantities, encoded, hex.EncodeToString(sum[:]), nil
}

func requestAllocations(ctx context.Context, tx *ent.Tx, requestID string) ([]*ent.BudgetAllocation, error) {
	allocations, err := tx.BudgetAllocation.Query().Where(budgetallocation.RequestIDEQ(requestID)).All(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(allocations, func(i, j int) bool { return allocations[i].ID < allocations[j].ID })
	return allocations, nil
}

func releaseReservations(ctx context.Context, tx *ent.Tx, allocations []*ent.BudgetAllocation, now time.Time) error {
	for _, allocation := range allocations {
		if allocation.ReservationReleased {
			continue
		}
		bucket, err := tx.BudgetBucket.Get(ctx, allocation.BudgetBucketID)
		if err != nil {
			return err
		}
		if bucket.ReservedQuantity < allocation.ReservedQuantity {
			return ErrInvalidSettlement
		}
		if _, err := tx.BudgetBucket.UpdateOneID(bucket.ID).
			SetReservedQuantity(bucket.ReservedQuantity - allocation.ReservedQuantity).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return err
		}
		if _, err := tx.BudgetAllocation.UpdateOneID(allocation.ID).
			SetReservationReleased(true).
			SetResolved(true).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func closeReconciliation(ctx context.Context, tx *ent.Tx, id int, actor, action, reason string, now time.Time) error {
	_, err := tx.BudgetReconciliation.UpdateOneID(id).
		SetState(budgetreconciliation.StateRESOLVED).
		SetResolvedAt(now).
		SetResolvedByActor(actor).
		SetResolutionAction(budgetreconciliation.ResolutionAction(action)).
		SetResolutionReason(strings.TrimSpace(reason)).
		Save(ctx)
	return err
}
