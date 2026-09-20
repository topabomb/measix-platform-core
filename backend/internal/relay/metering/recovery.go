package metering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	relaybudget "measix/platform/internal/relay/budget"
	"measix/platform/internal/wire/usageingestapi"
)

// RecoverLifecycle closes every durable pre-settlement request before Relay
// accepts traffic. ADMITTED rows were never given a start intent and can be
// released. STARTED rows replay the exact start event and are conservatively
// settled as UNKNOWN, because a process death cannot prove whether upstream
// received bytes.
func RecoverLifecycle(ctx context.Context, spool *Spool, budget relaybudget.Client, recorder *Recorder) error {
	if spool == nil || budget == nil || recorder == nil {
		return fmt.Errorf("invalid relay metering recovery configuration")
	}
	for {
		rows, err := spool.Recoverable(ctx, 1000)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			var journal JournalAdmission
			if err := json.Unmarshal(row.Admission, &journal); err != nil || journal.Admission.RequestId != row.RequestID {
				return fmt.Errorf("invalid admission journal for %s", row.RequestID)
			}
			switch row.State {
			case RequestAdmitted:
				occurredAt := journal.Admission.AdmittedAt.Add(time.Nanosecond)
				release := usageingestapi.BudgetReleaseRequest{
					Revision: 1, OccurredAt: occurredAt, Reason: "relay_recovery_before_start",
					EventHash: lifecycleEventHash(row.RequestID, "release:relay_recovery_before_start", 1, occurredAt),
				}
				if err := budget.Release(ctx, row.RequestID, release); err != nil {
					return err
				}
				if err := recorder.AbortAdmission(row.RequestID); err != nil {
					return err
				}
			case RequestStarted:
				if row.StartedAt == nil {
					return fmt.Errorf("started journal %s has no timestamp", row.RequestID)
				}
				start := usageingestapi.BudgetLifecycleEvent{
					Revision: 1, OccurredAt: *row.StartedAt,
					EventHash: lifecycleEventHash(row.RequestID, "start", 1, *row.StartedAt),
				}
				if err := budget.Start(ctx, row.RequestID, start); err != nil {
					return err
				}
				settlement := recoverySettlement(journal, *row.StartedAt)
				if err := recorder.Record(settlement); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported recovery state %q", row.State)
			}
		}
	}
}

func recoverySettlement(journal JournalAdmission, startedAt time.Time) usageingestapi.UsageSettlement {
	admission := journal.Admission
	one := int64(1)
	meters := []usageingestapi.MeterValue{{Meter: usageingestapi.REQUESTS, Completeness: usageingestapi.EXACT, Numerator: &one, Denominator: &one}}
	for _, meter := range admission.SupportedMeters {
		if meter == usageingestapi.REQUESTS {
			continue
		}
		meters = append(meters, usageingestapi.MeterValue{Meter: meter, Completeness: usageingestapi.UNKNOWN})
	}
	errorClass := "relay_restart_after_start"
	fact := usageingestapi.RequestUsageFact{
		DeploymentId: admission.DeploymentId, UserId: admission.UserId, DeviceId: admission.DeviceId,
		InteractionId: admission.InteractionId, ResourceId: admission.ResourceId, ResourceKind: admission.ResourceKind,
		ClientProtocol: admission.ClientProtocol, RuntimeRouteId: journal.RuntimeRouteID, UpstreamId: admission.UpstreamId,
		ManagedGeneration: admission.ManagedGeneration, ControlRevision: admission.ControlRevision,
		StartedAt: startedAt, CompletedAt: startedAt, Forwarded: true, HttpStatus: 502,
		RequestBytes: 0, ResponseBytes: 0, DurationMs: 0, ErrorClass: &errorClass,
	}
	settlement := usageingestapi.UsageSettlement{
		RequestId: admission.RequestId, Revision: 1, SourceEventId: admission.RequestId + ":recovery:1",
		OccurredAt: startedAt, Request: fact, Meters: meters, Completeness: usageingestapi.UNKNOWN,
		State: usageingestapi.RECONCILIATIONREQUIRED,
	}
	settlement.EventHash = settlementEventHash(settlement)
	return settlement
}

func lifecycleEventHash(requestID, action string, revision int, occurredAt time.Time) string {
	sum := sha256.Sum256([]byte(requestID + "|" + action + "|" + strconv.Itoa(revision) + "|" + occurredAt.UTC().Format(time.RFC3339Nano)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func settlementEventHash(settlement usageingestapi.UsageSettlement) string {
	payload, _ := json.Marshal(struct {
		RequestID    string
		Meters       []usageingestapi.MeterValue
		Fact         usageingestapi.RequestUsageFact
		Completeness usageingestapi.UsageCompleteness
		State        usageingestapi.UsageSettlementState
	}{settlement.RequestId, settlement.Meters, settlement.Request, settlement.Completeness, settlement.State})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
