package usage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/usageevent"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrInvalidBatch        = errors.New("invalid usage settlement batch")
	ErrSettlementConflict  = errors.New("usage settlement revision conflict")
	ErrAttributionMismatch = errors.New("usage settlement attribution does not match admission")
)

type Service struct {
	Client *ent.Client
	Budget *budget.Service
	Now    func() time.Time
}

func NewService(client *ent.Client, budgets ...*budget.Service) *Service {
	service := &Service{Client: client, Now: time.Now}
	if len(budgets) > 0 {
		service.Budget = budgets[0]
	}
	return service
}

func (s *Service) IngestSettlements(ctx context.Context, batch usageingestapi.UsageSettlementBatch) (usageingestapi.UsageBatchAck, error) {
	if s == nil || s.Client == nil || s.Budget == nil || len(batch.Events) < 1 || len(batch.Events) > 200 {
		return usageingestapi.UsageBatchAck{}, ErrInvalidBatch
	}
	for i := range batch.Events {
		if err := validateSettlement(batch.Events[i]); err != nil {
			return usageingestapi.UsageBatchAck{}, fmt.Errorf("%w: event[%d]: %v", ErrInvalidBatch, i, err)
		}
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return usageingestapi.UsageBatchAck{}, err
	}
	defer tx.Rollback()

	now := s.Now().UTC()
	ack := usageingestapi.UsageBatchAck{}
	for _, event := range batch.Events {
		deleted, err := tx.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(event.Request.UserId)).Exist(ctx)
		if err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		if deleted {
			// A settlement can arrive after the delete barrier because Relay usage
			// delivery is durable and at-least-once. Acknowledge it without
			// recreating any identity, request, budget, or usage fact.
			ack.DiscardedCount++
			continue
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		existing, err := tx.UsageEvent.Query().Where(
			usageevent.RequestIDEQ(event.RequestId), usageevent.RevisionEQ(int64(event.Revision)),
		).Only(ctx)
		if err == nil {
			if existing.EventHash != event.EventHash || existing.SourceEventID != event.SourceEventId || !bytes.Equal(existing.PayloadJSON, payload) {
				return usageingestapi.UsageBatchAck{}, ErrSettlementConflict
			}
			ack.DuplicateCount++
			continue
		}
		if !ent.IsNotFound(err) {
			return usageingestapi.UsageBatchAck{}, err
		}

		admission, err := tx.BudgetRequest.Get(ctx, event.RequestId)
		if ent.IsNotFound(err) {
			return usageingestapi.UsageBatchAck{}, budget.ErrRequestNotFound
		}
		if err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		if err := validateAttribution(event, admission); err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		if event.Request.Forwarded {
			budgetMeters, err := budgetMeterValues(event.Meters)
			if err != nil {
				return usageingestapi.UsageBatchAck{}, err
			}
			complete := event.State == usageingestapi.SETTLED && event.Completeness == usageingestapi.EXACT
			if _, err := s.Budget.SettleInTx(ctx, tx, budget.SettlementInput{
				RequestID: event.RequestId, Revision: int64(event.Revision), Meters: budgetMeters,
				Complete: complete, ReportedBy: "runtime-relay",
			}, now); err != nil {
				return usageingestapi.UsageBatchAck{}, err
			}
		} else if budget.RequestState(admission.State) != budget.RequestDenied {
			return usageingestapi.UsageBatchAck{}, ErrAttributionMismatch
		}
		if _, err := tx.UsageEvent.Create().
			SetRequestID(event.RequestId).SetRevision(int64(event.Revision)).SetEventHash(event.EventHash).
			SetSourceEventID(event.SourceEventId).SetPayloadJSON(payload).SetCreatedAt(now).Save(ctx); err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		if err := upsertRequestFact(ctx, tx, event, admission.BudgetRevision, now); err != nil {
			return usageingestapi.UsageBatchAck{}, err
		}
		if event.Request.Forwarded {
			if err := appendSemanticUsage(ctx, tx, event); err != nil {
				return usageingestapi.UsageBatchAck{}, err
			}
		}
		ack.AcceptedCount++
	}
	if err := tx.Commit(); err != nil {
		return usageingestapi.UsageBatchAck{}, err
	}
	return ack, nil
}

func validateSettlement(event usageingestapi.UsageSettlement) error {
	if platformid.Validate(platformid.Request, event.RequestId) != nil || event.Revision < 1 ||
		!validHash(event.EventHash) || strings.TrimSpace(event.SourceEventId) == "" || event.OccurredAt.IsZero() ||
		!event.Completeness.Valid() || !event.State.Valid() || (event.Request.Forwarded && len(event.Meters) == 0) {
		return ErrInvalidBatch
	}
	fact := event.Request
	checks := []struct {
		kind  platformid.Kind
		value string
	}{
		{platformid.Deployment, fact.DeploymentId}, {platformid.User, fact.UserId},
		{platformid.Route, fact.RuntimeRouteId}, {platformid.Upstream, fact.UpstreamId},
	}
	for _, check := range checks {
		if platformid.Validate(check.kind, check.value) != nil {
			return ErrInvalidBatch
		}
	}
	if fact.InteractionId != nil && platformid.Validate(platformid.Interaction, *fact.InteractionId) != nil {
		return ErrInvalidBatch
	}
	if fact.DeviceId != nil && platformid.Validate(platformid.Device, *fact.DeviceId) != nil {
		return ErrInvalidBatch
	}
	kind, err := platformid.KindOf(fact.ResourceId)
	if err != nil || !resourceKindMatches(kind, fact.ResourceKind) || !fact.ClientProtocol.Valid() || !fact.ResourceKind.Valid() {
		return ErrInvalidBatch
	}
	if fact.ManagedGeneration < 0 || fact.ControlRevision < 0 || fact.HttpStatus < 100 || fact.HttpStatus > 599 ||
		fact.RequestBytes < 0 || fact.ResponseBytes < 0 || fact.DurationMs < 0 || fact.StartedAt.IsZero() || fact.CompletedAt.IsZero() || fact.CompletedAt.Before(fact.StartedAt) {
		return ErrInvalidBatch
	}
	if fact.UpstreamHttpStatus != nil && (*fact.UpstreamHttpStatus < 100 || *fact.UpstreamHttpStatus > 599) {
		return ErrInvalidBatch
	}
	seen := map[usageingestapi.UsageMeter]bool{}
	for _, meter := range event.Meters {
		if !meter.Meter.Valid() || !meter.Completeness.Valid() || seen[meter.Meter] {
			return ErrInvalidBatch
		}
		seen[meter.Meter] = true
		if (meter.Numerator == nil) != (meter.Denominator == nil) {
			return ErrInvalidBatch
		}
		if meter.Numerator != nil && (*meter.Numerator < 0 || *meter.Denominator < 1) {
			return ErrInvalidBatch
		}
		if meter.Completeness == usageingestapi.EXACT && meter.Numerator == nil {
			return ErrInvalidBatch
		}
	}
	if fact.Forwarded {
		if request, ok := findMeter(event.Meters, usageingestapi.REQUESTS); !ok || request.Numerator == nil || *request.Numerator != 1 || *request.Denominator != 1 || request.Completeness != usageingestapi.EXACT {
			return ErrInvalidBatch
		}
	} else if len(event.Meters) != 0 || event.Completeness != usageingestapi.EXACT || event.State != usageingestapi.SETTLED {
		return ErrInvalidBatch
	}
	return nil
}

func validateAttribution(event usageingestapi.UsageSettlement, admission *ent.BudgetRequest) error {
	fact := event.Request
	if admission.UserID != fact.UserId || admission.DeploymentID != fact.DeploymentId || admission.ResourceID != fact.ResourceId ||
		admission.ClientProtocol != string(fact.ClientProtocol) || admission.UpstreamID != fact.UpstreamId ||
		admission.ManagedGeneration != int64(fact.ManagedGeneration) || admission.ControlRevision != int64(fact.ControlRevision) ||
		string(admission.Capability) != string(fact.ResourceKind) || valueOrEmpty(admission.InteractionID) != valueOrEmpty(fact.InteractionId) ||
		valueOrEmpty(admission.DeviceID) != valueOrEmpty(fact.DeviceId) {
		return ErrAttributionMismatch
	}
	return nil
}

func upsertRequestFact(ctx context.Context, tx *ent.Tx, event usageingestapi.UsageSettlement, budgetRevision int64, now time.Time) error {
	fact := event.Request
	existing, err := tx.RequestUsage.Query().Where(requestusage.RequestIDEQ(event.RequestId)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	if err == nil && existing.SettlementRevision >= int64(event.Revision) {
		return ErrSettlementConflict
	}
	if err == nil && !sameImmutableRequestFact(existing, fact, budgetRevision) {
		return ErrAttributionMismatch
	}
	applyCreate := func(create *ent.RequestUsageCreate) *ent.RequestUsageCreate {
		return create.SetRequestID(event.RequestId).SetNillableInteractionID(fact.InteractionId).SetDeploymentID(fact.DeploymentId).
			SetUserID(fact.UserId).SetNillableDeviceID(fact.DeviceId).SetResourceID(fact.ResourceId).SetResourceKind(string(fact.ResourceKind)).
			SetClientProtocol(string(fact.ClientProtocol)).SetRuntimeRouteID(fact.RuntimeRouteId).SetUpstreamID(fact.UpstreamId).
			SetManagedGeneration(int64(fact.ManagedGeneration)).SetControlRevision(int64(fact.ControlRevision)).SetStartedAt(fact.StartedAt.UTC()).
			SetCompletedAt(fact.CompletedAt.UTC()).SetForwarded(fact.Forwarded).SetHTTPStatus(fact.HttpStatus).SetNillableUpstreamHTTPStatus(fact.UpstreamHttpStatus).
			SetRequestBytes(fact.RequestBytes).SetResponseBytes(fact.ResponseBytes).SetDurationMs(fact.DurationMs).SetNillableErrorClass(fact.ErrorClass).
			SetRequestCompleteness(string(event.Completeness)).SetSettlementState(string(event.State)).SetSettlementRevision(int64(event.Revision)).
			SetBudgetRevision(budgetRevision).SetIngestedAt(now)
	}
	if ent.IsNotFound(err) {
		_, err = applyCreate(tx.RequestUsage.Create()).Save(ctx)
		return err
	}
	_, err = tx.RequestUsage.UpdateOneID(existing.ID).SetCompletedAt(fact.CompletedAt.UTC()).SetHTTPStatus(fact.HttpStatus).
		SetNillableUpstreamHTTPStatus(fact.UpstreamHttpStatus).SetResponseBytes(fact.ResponseBytes).SetDurationMs(fact.DurationMs).
		SetNillableErrorClass(fact.ErrorClass).SetRequestCompleteness(string(event.Completeness)).SetSettlementState(string(event.State)).
		SetSettlementRevision(int64(event.Revision)).SetIngestedAt(now).Save(ctx)
	return err
}

func sameImmutableRequestFact(existing *ent.RequestUsage, fact usageingestapi.RequestUsageFact, budgetRevision int64) bool {
	return existing.DeploymentID == fact.DeploymentId &&
		existing.UserID == fact.UserId &&
		valueOrEmpty(existing.InteractionID) == valueOrEmpty(fact.InteractionId) &&
		valueOrEmpty(existing.DeviceID) == valueOrEmpty(fact.DeviceId) &&
		existing.ResourceID == fact.ResourceId &&
		existing.ResourceKind == string(fact.ResourceKind) &&
		existing.ClientProtocol == string(fact.ClientProtocol) &&
		existing.RuntimeRouteID == fact.RuntimeRouteId &&
		existing.UpstreamID == fact.UpstreamId &&
		existing.ManagedGeneration == int64(fact.ManagedGeneration) &&
		existing.ControlRevision == int64(fact.ControlRevision) &&
		existing.StartedAt.Equal(fact.StartedAt.UTC()) &&
		existing.Forwarded == fact.Forwarded &&
		existing.RequestBytes == fact.RequestBytes &&
		existing.BudgetRevision == budgetRevision
}

func appendSemanticUsage(ctx context.Context, tx *ent.Tx, event usageingestapi.UsageSettlement) error {
	meters := append([]usageingestapi.MeterValue(nil), event.Meters...)
	sort.Slice(meters, func(i, j int) bool { return meters[i].Meter < meters[j].Meter })
	for _, meter := range meters {
		units, decimal, err := semanticQuantity(meter)
		if err != nil {
			return err
		}
		source := "unknown"
		if meter.Source != nil {
			source = *meter.Source
		}
		if _, err := tx.SemanticUsage.Create().SetID(stableUsageID(event.RequestId, event.Revision, string(meter.Meter))).
			SetRequestID(event.RequestId).SetSettlementRevision(int64(event.Revision)).SetSourceEventID(event.SourceEventId).
			SetMeter(string(meter.Meter)).SetQuantityUnits(units).SetQuantityDecimal(decimal).SetCompleteness(string(meter.Completeness)).
			SetSource(source).SetOccurredAt(event.OccurredAt.UTC()).Save(ctx); err != nil {
			return err
		}
	}
	if event.Details != nil {
		names := make([]string, 0, len(*event.Details))
		for name := range *event.Details {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			value := (*event.Details)[name]
			if value < 0 {
				return ErrInvalidBatch
			}
			if _, err := tx.UsageDetail.Create().SetID(stableUsageID(event.RequestId, event.Revision, "detail:"+name)).
				SetRequestID(event.RequestId).SetSettlementRevision(int64(event.Revision)).SetName(name).SetQuantityUnits(value).
				SetSource("provider_response").SetCompleteness(string(event.Completeness)).SetOccurredAt(event.OccurredAt.UTC()).Save(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func budgetMeterValues(values []usageingestapi.MeterValue) ([]budget.MeterQuantity, error) {
	result := make([]budget.MeterQuantity, 0, len(values))
	for _, value := range values {
		if value.Numerator == nil || value.Denominator == nil {
			continue
		}
		quantity, err := canonicalBudgetQuantity(value)
		if err != nil {
			return nil, err
		}
		result = append(result, budget.MeterQuantity{Meter: budgetMeter(value.Meter), Quantity: quantity})
	}
	return result, nil
}

func canonicalBudgetQuantity(value usageingestapi.MeterValue) (int64, error) {
	if value.Meter != usageingestapi.AUDIOSECONDS {
		if *value.Denominator != 1 {
			return 0, ErrInvalidBatch
		}
		return *value.Numerator, nil
	}
	n := new(big.Int).Mul(big.NewInt(*value.Numerator), big.NewInt(1000))
	d := big.NewInt(*value.Denominator)
	n.Add(n, new(big.Int).Sub(d, big.NewInt(1))).Quo(n, d)
	if !n.IsInt64() {
		return 0, ErrInvalidBatch
	}
	return n.Int64(), nil
}

func semanticQuantity(value usageingestapi.MeterValue) (int64, string, error) {
	if value.Numerator == nil || value.Denominator == nil {
		return 0, "0", nil
	}
	if value.Meter != usageingestapi.AUDIOSECONDS {
		if *value.Denominator != 1 {
			return 0, "", ErrInvalidBatch
		}
		return *value.Numerator, fmt.Sprintf("%d", *value.Numerator), nil
	}
	millis, err := canonicalBudgetQuantity(value)
	if err != nil {
		return 0, "", err
	}
	rat := new(big.Rat).SetFrac(big.NewInt(*value.Numerator), big.NewInt(*value.Denominator))
	decimal := strings.TrimRight(strings.TrimRight(rat.FloatString(9), "0"), ".")
	if decimal == "" {
		decimal = "0"
	}
	return millis, decimal, nil
}

func budgetMeter(meter usageingestapi.UsageMeter) budget.Meter {
	if meter == usageingestapi.AUDIOSECONDS {
		return budget.MeterAudioMilliseconds
	}
	return budget.Meter(meter)
}

func stableUsageID(requestID string, revision int, name string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", requestID, revision, name)))
	return hex.EncodeToString(sum[:])
}

func findMeter(values []usageingestapi.MeterValue, meter usageingestapi.UsageMeter) (usageingestapi.MeterValue, bool) {
	for _, value := range values {
		if value.Meter == meter {
			return value, true
		}
	}
	return usageingestapi.MeterValue{}, false
}

func resourceKindMatches(kind platformid.Kind, declared usageingestapi.ResourceKind) bool {
	return (kind == platformid.Model && declared == usageingestapi.ResourceKindMODEL) ||
		(kind == platformid.TTS && declared == usageingestapi.ResourceKindTTS) ||
		(kind == platformid.ASR && declared == usageingestapi.ResourceKindASR) ||
		(kind == platformid.MCP && declared == usageingestapi.ResourceKindMCP)
}

func validHash(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
