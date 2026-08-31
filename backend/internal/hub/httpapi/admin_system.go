package httpapi

import (
	"net/http"

	"measix/platform/internal/wire/adminapi"
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
		BuildVersion: status.BuildVersion, DbHealth: status.DBHealth, MigrationRevision: status.MigrationRevision,
		RuntimeStatus:           adminapi.SystemStatusRuntimeStatus(status.RuntimeStatus),
		ActiveManagedGeneration: status.ActiveManagedGeneration, ManagedStateRevision: status.ManagedStateRevision,
		DesiredControlRevision: status.DesiredControlRevision, RelayReady: status.RelayReady,
		AppliedControlRevision: status.AppliedControlRevision, LastRelaySeenAt: status.LastRelaySeenAt,
		RequestUsageIngestLagSeconds: status.RequestUsageIngestLagSeconds, SemanticOrphanCount: status.SemanticOrphanCount,
		SpoolPendingCount: status.SpoolPendingCount, OldestPendingAgeSeconds: status.OldestPendingAgeSeconds,
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
	if status.LatestActivation != nil {
		activation := activationWire(*status.LatestActivation)
		wire.LatestActivation = &activation
	}
	writeJSON(w, http.StatusOK, wire)
}
