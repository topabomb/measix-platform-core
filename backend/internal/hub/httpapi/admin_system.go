package httpapi

import (
	"net/http"
	"strings"
	"time"

	"measix/platform/internal/common/observability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
)

func (h *fullAdminHandler) SystemHealth(w http.ResponseWriter, r *http.Request) {
	if h.services.System == nil {
		writeJSON(w, http.StatusServiceUnavailable, adminapi.Health{Live: true, Ready: false})
		return
	}
	ready := h.services.System.Health(r.Context()) == nil
	code := http.StatusOK
	if !ready {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, adminapi.Health{Live: true, Ready: ready})
}

func (h *fullAdminHandler) SystemStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	if h.services.System == nil {
		writeProblem(w, http.StatusServiceUnavailable, "system_status_unavailable", "System status unavailable")
		return
	}
	status, err := h.services.System.Status(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	wire := adminapi.SystemStatus{
		BuildVersion: status.BuildVersion, DbHealth: status.DBHealth, SchemaIdentity: status.SchemaIdentity,
		RuntimeStatus:           adminapi.SystemStatusRuntimeStatus(status.RuntimeStatus),
		ActiveManagedGeneration: status.ActiveManagedGeneration, ManagedStateRevision: status.ManagedStateRevision,
		DesiredControlRevision: status.DesiredControlRevision, RelayReady: status.RelayReady,
		RelayBuildVersion:      status.RelayBuildVersion,
		AppliedControlRevision: status.AppliedControlRevision, LastRelaySeenAt: status.LastRelaySeenAt,
		RequestUsageIngestLagSeconds: status.RequestUsageIngestLagSeconds, SemanticOrphanCount: status.SemanticOrphanCount,
		SpoolPendingCount: status.SpoolPendingCount, OldestPendingAgeSeconds: status.OldestPendingAgeSeconds,
		SemanticUnknownRequestCount: status.SemanticUnknownRequestCount,
		PortalMode:                  adminapi.SystemStatusPortalMode(status.PortalMode),
		PortalUpstreamUrl:           status.PortalUpstream,
	}
	if publicOrigin := h.identity.PublicOrigin(); publicOrigin != "" {
		wire.PublicOrigin = &publicOrigin
		if status.PortalMode != "UNAVAILABLE" {
			portalURL := publicOrigin + "/portal/"
			wire.PortalUrl = &portalURL
		}
	}
	if status.SpoolState != nil {
		value := adminapi.SystemStatusSpoolState(*status.SpoolState)
		wire.SpoolState = &value
	}
	if status.DesiredBundleHash != nil {
		value := adminapi.Sha256Hash(*status.DesiredBundleHash)
		wire.DesiredBundleHash = &value
	}
	if status.AppliedBundleHash != nil {
		value := adminapi.Sha256Hash(*status.AppliedBundleHash)
		wire.AppliedBundleHash = &value
	}
	if status.CurrentActivation != nil {
		activation := activationWire(*status.CurrentActivation)
		wire.CurrentActivation = &activation
	}
	if status.LastActivation != nil {
		activation := activationWire(*status.LastActivation)
		wire.LastActivation = &activation
	}
	writeJSON(w, http.StatusOK, wire)
}

func (h *fullAdminHandler) SystemTelemetry(w http.ResponseWriter, r *http.Request, params adminapi.SystemTelemetryParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	if h.services.System == nil {
		writeProblem(w, http.StatusServiceUnavailable, "system_telemetry_unavailable", "System telemetry unavailable")
		return
	}
	window := 15 * time.Minute
	windowMinutes := adminapi.N15
	if params.Window != nil {
		switch *params.Window {
		case adminapi.N15m:
		case adminapi.N60m:
			window, windowMinutes = 60*time.Minute, adminapi.N60
		default:
			writeProblem(w, http.StatusBadRequest, "invalid_window", "Invalid telemetry window")
			return
		}
	}
	value := h.services.System.RecentTelemetry(window)
	response := adminapi.SystemTelemetry{CollectedAt: value.CollectedAt, WindowMinutes: windowMinutes, Hub: adminTelemetryWire(value.Hub)}
	if value.Relay != nil {
		relay := relayTelemetryAdminWire(*value.Relay)
		response.Relay = &relay
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *fullAdminHandler) SystemEvents(w http.ResponseWriter, r *http.Request, params adminapi.SystemEventsParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	if h.services.System == nil {
		writeProblem(w, http.StatusServiceUnavailable, "system_events_unavailable", "System events unavailable")
		return
	}
	filter := observability.EventFilter{Limit: 100}
	if params.Service != nil {
		filter.Service = string(*params.Service)
	}
	if params.Level != nil {
		filter.Level = *params.Level
	}
	if params.Event != nil {
		filter.Event = *params.Event
	}
	if params.Correlation != nil {
		filter.Correlation = *params.Correlation
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}
	if filter.Limit < 1 || filter.Limit > 200 || len(filter.Correlation) > 256 || len(filter.Level) > 32 || len(filter.Event) > 128 {
		writeProblem(w, http.StatusBadRequest, "invalid_event_filter", "Invalid event filter")
		return
	}
	page, err := h.services.System.RecentEvents(filter)
	if err != nil {
		if strings.Contains(err.Error(), "not configured") {
			writeProblem(w, http.StatusServiceUnavailable, "system_events_unavailable", "System events unavailable")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "system_events_read_failed", "System events could not be read")
		return
	}
	items := make([]adminapi.SystemEvent, 0, len(page.Items))
	for _, event := range page.Items {
		items = append(items, adminEventWire(event))
	}
	writeJSON(w, http.StatusOK, adminapi.SystemEventPage{Items: items, Truncated: page.Truncated})
}

func adminTelemetryWire(snapshot observability.Snapshot) adminapi.ProcessTelemetry {
	buckets := make([]adminapi.TelemetryBucket, 0, len(snapshot.Buckets))
	for _, bucket := range snapshot.Buckets {
		activation, reconcile := bucket.ActivationFailureCount, bucket.ReconcileFailureCount
		buckets = append(buckets, adminapi.TelemetryBucket{
			Minute: bucket.Minute, RequestCount: bucket.RequestCount, SuccessCount: bucket.SuccessCount,
			ClientErrorCount: bucket.ClientErrorCount, ServerErrorCount: bucket.ServerErrorCount,
			RejectedCount: bucket.RejectedCount, TimeoutCount: bucket.TimeoutCount, CancelledCount: bucket.CancelledCount,
			DurationP95Ms: bucket.DurationP95Ms, InFlight: bucket.InFlightPeak,
			ActivationFailureCount: &activation, ReconcileFailureCount: &reconcile,
		})
	}
	activation, reconcile := snapshot.Summary.ActivationFailureCount, snapshot.Summary.ReconcileFailureCount
	return adminapi.ProcessTelemetry{StartedAt: snapshot.StartedAt, Buckets: buckets, Summary: adminapi.TelemetryMetrics{
		RequestCount: snapshot.Summary.RequestCount, SuccessCount: snapshot.Summary.SuccessCount,
		ClientErrorCount: snapshot.Summary.ClientErrorCount, ServerErrorCount: snapshot.Summary.ServerErrorCount,
		RejectedCount: snapshot.Summary.RejectedCount, TimeoutCount: snapshot.Summary.TimeoutCount, CancelledCount: snapshot.Summary.CancelledCount,
		DurationP95Ms: snapshot.Summary.DurationP95Ms, InFlight: snapshot.InFlight,
		ActivationFailureCount: &activation, ReconcileFailureCount: &reconcile,
	}}
}

func relayTelemetryAdminWire(value relaycontrolapi.ProcessTelemetry) adminapi.ProcessTelemetry {
	buckets := make([]adminapi.TelemetryBucket, 0, len(value.Buckets))
	for _, bucket := range value.Buckets {
		buckets = append(buckets, adminapi.TelemetryBucket{
			Minute: bucket.Minute, RequestCount: bucket.RequestCount, SuccessCount: bucket.SuccessCount,
			ClientErrorCount: bucket.ClientErrorCount, ServerErrorCount: bucket.ServerErrorCount,
			RejectedCount: bucket.RejectedCount, TimeoutCount: bucket.TimeoutCount, CancelledCount: bucket.CancelledCount,
			DurationP95Ms: bucket.DurationP95Ms, InFlight: bucket.InFlight,
			UpstreamErrorCount: bucket.UpstreamErrorCount, BudgetDeniedCount: bucket.BudgetDeniedCount,
		})
	}
	return adminapi.ProcessTelemetry{StartedAt: value.StartedAt, Buckets: buckets, Summary: adminapi.TelemetryMetrics{
		RequestCount: value.Summary.RequestCount, SuccessCount: value.Summary.SuccessCount,
		ClientErrorCount: value.Summary.ClientErrorCount, ServerErrorCount: value.Summary.ServerErrorCount,
		RejectedCount: value.Summary.RejectedCount, TimeoutCount: value.Summary.TimeoutCount, CancelledCount: value.Summary.CancelledCount,
		DurationP95Ms: value.Summary.DurationP95Ms, InFlight: value.Summary.InFlight,
		UpstreamErrorCount: value.Summary.UpstreamErrorCount, BudgetDeniedCount: value.Summary.BudgetDeniedCount,
	}}
}

func adminEventWire(value observability.Event) adminapi.SystemEvent {
	result := adminapi.SystemEvent{Time: value.Time, Service: adminapi.SystemEventService(value.Service), Level: value.Level, Event: value.Event, Message: value.Message}
	if value.RequestID != "" {
		result.RequestId = &value.RequestID
	}
	if value.InteractionID != "" {
		result.InteractionId = &value.InteractionID
	}
	if value.ActivationID != "" {
		result.ActivationId = &value.ActivationID
	}
	if value.DeploymentID != "" {
		result.DeploymentId = &value.DeploymentID
	}
	if value.ResourceID != "" {
		result.ResourceId = &value.ResourceID
	}
	if value.Outcome != "" {
		result.Outcome = &value.Outcome
	}
	if value.ErrorCode != "" {
		result.ErrorCode = &value.ErrorCode
	}
	result.ControlRevision, result.DurationMs, result.HttpStatus = value.ControlRevision, value.DurationMs, value.HTTPStatus
	if value.Truncated {
		truncated := true
		result.Truncated = &truncated
	}
	return result
}
