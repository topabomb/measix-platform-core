package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/usageingestapi"
)

type davTransferStats struct {
	started        time.Time
	requestBytes   atomic.Int64
	forwarded      bool
	upstreamStatus *int
	errorCode      string
}

func (h *Handler) recordDAV(r *http.Request, state *control.State, claims *accessClaims, resource control.Resource, route control.Route, binding relaycontrolapi.UserDAVBinding, requestID string, observer *responseObserver, stats *davTransferStats) {
	if h.recorder == nil {
		return
	}
	completed := h.store.Now()
	status := observer.status
	if status == 0 {
		status = 500
	}
	statistics := true
	version := usageingestapi.RequestUsageFactTargetVersion(3)
	method := r.Method
	if len(method) > 32 {
		method = "OTHER"
	}
	target := binding.Target
	fact := usageingestapi.RequestUsageFact{DeploymentId: state.DeploymentID, UserId: claims.Subject, DeviceId: &claims.DeviceID, ResourceId: resource.ID, ResourceKind: "WEBDAV", ClientProtocol: "WEBDAV", RuntimeRouteId: route.ID, ManagedGeneration: state.ActiveManagedGeneration, ControlRevision: state.ControlRevision, StartedAt: stats.started, CompletedAt: completed, Forwarded: stats.forwarded, HttpStatus: status, UpstreamHttpStatus: stats.upstreamStatus, HttpMethod: &method, RequestBytes: stats.requestBytes.Load(), ResponseBytes: observer.bytes, DurationMs: completed.Sub(stats.started).Milliseconds(), TargetVersion: &version, WorkspaceTarget: &usageingestapi.WorkspaceTarget{WorkspaceServiceId: target.WorkspaceServiceId, AgentSpaceId: target.AgentSpaceId, RemoteUsername: target.RemoteUsername, BindingRevision: target.BindingRevision}, DavSecretRef: &usageingestapi.DAVSecretRef{SecretId: binding.SecretRef.SecretId, SecretVersion: binding.SecretRef.SecretVersion}}
	if stats.errorCode != "" {
		fact.ErrorClass = &stats.errorCode
	}
	event := usageingestapi.UsageSettlement{RequestId: requestID, Revision: 1, SourceEventId: requestID + ":dav:1", OccurredAt: completed, State: "SETTLED", Completeness: "EXACT", Meters: []usageingestapi.MeterValue{}, StatisticsOnly: &statistics, Request: fact}
	payload, _ := json.Marshal(event)
	sum := sha256.Sum256(payload)
	event.EventHash = "sha256:" + hex.EncodeToString(sum[:])
	if err := h.recorder.Record(event); err != nil {
		slog.Warn("DAV statistic could not be persisted", "event", "dav_statistics_gap", "requestId", requestID)
	}
}
