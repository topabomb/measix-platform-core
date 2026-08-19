package httpapi

import (
	"net/http"

	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/wire/adminapi"
)

func (h *fullAdminHandler) SystemStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.authenticateAdmin(r, "", false); err != nil {
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
		row := status.LatestActivation
		result := runtimecontrol.ActivationResult{
			ActivationID: row.ID, Kind: row.Kind, State: row.State, DesiredControlRevision: int(row.ControlRevision),
			BundleHash: row.BundleHash, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt, ErrorCode: row.ErrorCode,
		}
		if row.SubjectID != nil {
			result.ReleaseID = *row.SubjectID
		}
		if row.TargetGeneration != nil {
			result.TargetManagedGeneration = int(*row.TargetGeneration)
		}
		activation := activationWire(result)
		wire.LatestActivation = &activation
	}
	writeJSON(w, http.StatusOK, wire)
}
