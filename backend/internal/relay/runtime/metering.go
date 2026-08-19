package runtime

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/usageingestapi"
)

type UsageRecorder interface {
	Record(usageingestapi.RequestUsageEvent) error
}

type responseObserver struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *responseObserver) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseObserver) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(value)
	w.bytes += int64(n)
	return n, err
}

func (w *responseObserver) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseObserver) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *responseObserver) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *responseObserver) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type countingBody struct {
	io.ReadCloser
	bytes int64
}

func (r *countingBody) Read(value []byte) (int, error) {
	n, err := r.ReadCloser.Read(value)
	r.bytes += int64(n)
	return n, err
}

type usageAttribution struct {
	state         *control.State
	claims        *accessClaims
	resourceID    string
	interactionID *string
	route         control.Route
	upstream      control.Upstream
	startedAt     time.Time
	requestID     string
}

func (h *Handler) recordUsage(observer *responseObserver, body *countingBody, attr usageAttribution, forwarded bool, upstreamStatus *int, errorClass string) {
	if h.recorder == nil || attr.state == nil || attr.claims == nil || attr.route.ID == "" || attr.upstream.ID == "" {
		return
	}
	completedAt := h.store.Now()
	status := observer.status
	if status == 0 {
		status = http.StatusOK
	}
	deviceID := attr.claims.DeviceID
	interactionID := attr.interactionID
	var requestBytes int64
	if body != nil {
		requestBytes = body.bytes
	}
	var errorValue *string
	if errorClass != "" {
		errorValue = &errorClass
	}
	event := usageingestapi.RequestUsageEvent{
		RequestId: attr.requestID, InteractionId: interactionID,
		DeploymentId: attr.state.DeploymentID, UserId: attr.claims.Subject, DeviceId: &deviceID,
		ResourceId: attr.resourceID, RuntimeRouteId: attr.route.ID, UpstreamId: attr.upstream.ID,
		ManagedGeneration: attr.state.ActiveManagedGeneration, ControlRevision: attr.state.ControlRevision,
		StartedAt: attr.startedAt, CompletedAt: completedAt, Forwarded: forwarded,
		HttpStatus: status, UpstreamHttpStatus: upstreamStatus,
		RequestBytes: int(requestBytes), ResponseBytes: int(observer.bytes),
		DurationMs: int(completedAt.Sub(attr.startedAt).Milliseconds()), ErrorClass: errorValue,
	}
	_ = h.recorder.Record(event)
}

func usageRoute(state *control.State, resourceID string) (control.Route, control.Upstream, bool) {
	if state == nil {
		return control.Route{}, control.Upstream{}, false
	}
	routeID, ok := state.ResourceRoutes[resourceID]
	if !ok {
		return control.Route{}, control.Upstream{}, false
	}
	route, ok := state.Routes[routeID]
	if !ok {
		return control.Route{}, control.Upstream{}, false
	}
	upstream, ok := state.Upstreams[route.UpstreamID]
	if !ok {
		return control.Route{}, control.Upstream{}, false
	}
	return route, upstream, true
}
