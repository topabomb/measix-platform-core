package runtime

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/relay/protocolusage"
	"measix/platform/internal/wire/usageingestapi"
)

type UsageRecorder interface {
	PersistAdmission(usageingestapi.BudgetAdmissionRequest, string) error
	MarkStarted(string, time.Time) error
	AbortAdmission(string) error
	Record(usageingestapi.UsageSettlement) error
	RecordDenied(usageingestapi.BudgetAdmissionRequest, string, usageingestapi.UsageSettlement) error
}

func (h *Handler) recordDenied(admission usageingestapi.BudgetAdmissionRequest, attr usageAttribution, status int, code string) error {
	completedAt := h.store.Now()
	errorClass := code
	deviceID := usageingestapi.DeviceId(attr.claims.DeviceID)
	fact := usageingestapi.RequestUsageFact{
		InteractionId: attr.interactionID, DeploymentId: attr.state.DeploymentID, UserId: attr.claims.Subject, DeviceId: &deviceID,
		ResourceId: attr.resourceID, ResourceKind: usageingestapi.ResourceKind(attr.resource.Kind), ClientProtocol: usageingestapi.ClientProtocol(attr.resource.ClientProtocol),
		RuntimeRouteId: attr.route.ID, UpstreamId: attr.upstream.ID, ManagedGeneration: attr.state.ActiveManagedGeneration, ControlRevision: attr.state.ControlRevision,
		StartedAt: attr.admittedAt, CompletedAt: completedAt, Forwarded: false, HttpStatus: status,
		RequestBytes: 0, ResponseBytes: 0, DurationMs: completedAt.Sub(attr.admittedAt).Milliseconds(), ErrorClass: &errorClass,
	}
	settlement := usageingestapi.UsageSettlement{
		RequestId: attr.requestID, Revision: 1, SourceEventId: attr.requestID + ":denied:1", OccurredAt: completedAt,
		Request: fact, Meters: []usageingestapi.MeterValue{}, Completeness: usageingestapi.EXACT, State: usageingestapi.SETTLED,
		DiagnosticCode: &code,
	}
	settlement.EventHash = settlementHash(attr.requestID, settlement.Meters, fact, settlement.Completeness, settlement.State)
	return h.recorder.RecordDenied(admission, attr.route.ID, settlement)
}

type responseObserver struct {
	http.ResponseWriter
	status      int
	bytes       int64
	tunnel      *upgradedStream
	observation *usageObservation
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
	if n > 0 && w.observation != nil {
		w.observation.observeResponse(value[:n])
	}
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
	bytes       int64
	observation *usageObservation
}

func (r *countingBody) Read(value []byte) (int, error) {
	n, err := r.ReadCloser.Read(value)
	if n > 0 {
		r.bytes += int64(n)
		r.observation.observeRequest(value[:n])
	}
	return n, err
}

type usageAttribution struct {
	state             *control.State
	claims            *accessClaims
	resourceID        string
	interactionID     *string
	resource          control.Resource
	route             control.Route
	upstream          control.Upstream
	admittedAt        time.Time
	startedAt         time.Time
	requestID         string
	budgetRevision    int
	lifecycleRevision int
}

func admissionRequest(attr usageAttribution, observation *usageObservation, request *http.Request) usageingestapi.BudgetAdmissionRequest {
	known := meterValues(observation.knownMeasurements())
	supported := make([]usageingestapi.UsageMeter, 0, len(observation.supportedMeters()))
	for _, meter := range observation.supportedMeters() {
		supported = append(supported, usageingestapi.UsageMeter(meter))
	}
	deviceID := usageingestapi.DeviceId(attr.claims.DeviceID)
	input := usageingestapi.BudgetAdmissionRequest{
		RequestId: attr.requestID, DeploymentId: attr.state.DeploymentID, UserId: attr.claims.Subject,
		DeviceId: &deviceID, InteractionId: attr.interactionID, ResourceId: attr.resourceID,
		ResourceKind: usageingestapi.ResourceKind(attr.resource.Kind), ClientProtocol: usageingestapi.ClientProtocol(attr.resource.ClientProtocol),
		UpstreamId: attr.upstream.ID, ManagedGeneration: attr.state.ActiveManagedGeneration,
		ControlRevision: attr.state.ControlRevision, AdmittedAt: attr.admittedAt,
		SupportedMeters: supported, RequestHash: requestHash(attr, request),
	}
	if len(known) > 0 {
		input.KnownUsage = &known
	}
	return input
}

func requestHash(attr usageAttribution, request *http.Request) string {
	canonical := struct {
		RequestID, UserID, DeviceID, InteractionID, ResourceID, Method, Path string
	}{attr.requestID, attr.claims.Subject, attr.claims.DeviceID, valueOrEmpty(attr.interactionID), attr.resourceID, request.Method, request.URL.EscapedPath()}
	payload, _ := json.Marshal(canonical)
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *Handler) recordSettlement(observer *responseObserver, body *countingBody, observation *usageObservation, attr usageAttribution, forwarded bool, upstreamStatus *int, errorClass string) error {
	if h.recorder == nil || attr.state == nil || attr.claims == nil || observation == nil {
		return nil
	}
	completedAt := h.store.Now()
	status := observer.status
	if status == 0 {
		status = http.StatusBadGateway
	}
	requestBytes := int64(0)
	if body != nil {
		requestBytes = body.bytes
	}
	responseBytes := observer.bytes
	transportComplete := errorClass == ""
	if observer.tunnel != nil {
		requestBytes = observer.tunnel.requestBytes.Load()
		responseBytes = observer.tunnel.responseBytes.Load()
		transportComplete = transportComplete && !observer.tunnel.timedOut.Load() && !observer.tunnel.exceeded.Load()
	}
	usage := observation.finish(transportComplete)
	meters := meterValues(usage.Measurements)
	completeness := settlementCompleteness(usage.Measurements)
	state := usageingestapi.SETTLED
	if completeness != usageingestapi.EXACT {
		state = usageingestapi.RECONCILIATIONREQUIRED
	}
	var diagnosticCode *string
	if len(usage.Diagnostics) > 0 {
		code := usage.Diagnostics[0].Code
		diagnosticCode = &code
	}
	details := detailValues(usage.Details)
	var detailsPointer *map[string]int64
	if len(details) > 0 {
		detailsPointer = &details
	}
	var errorValue *string
	if errorClass != "" {
		errorValue = &errorClass
	}
	deviceID := usageingestapi.DeviceId(attr.claims.DeviceID)
	fact := usageingestapi.RequestUsageFact{
		InteractionId: attr.interactionID,
		DeploymentId:  attr.state.DeploymentID, UserId: attr.claims.Subject, DeviceId: &deviceID,
		ResourceId: attr.resourceID, ResourceKind: usageingestapi.ResourceKind(attr.resource.Kind),
		ClientProtocol: usageingestapi.ClientProtocol(attr.resource.ClientProtocol), RuntimeRouteId: attr.route.ID,
		UpstreamId: attr.upstream.ID, ManagedGeneration: attr.state.ActiveManagedGeneration,
		ControlRevision: attr.state.ControlRevision, StartedAt: attr.startedAt, CompletedAt: completedAt,
		Forwarded: forwarded, HttpStatus: status, UpstreamHttpStatus: upstreamStatus,
		RequestBytes: requestBytes, ResponseBytes: responseBytes,
		DurationMs: completedAt.Sub(attr.startedAt).Milliseconds(), ErrorClass: errorValue,
	}
	settlement := usageingestapi.UsageSettlement{
		RequestId: attr.requestID, Revision: 1, SourceEventId: attr.requestID + ":settlement:1",
		OccurredAt: completedAt, EventHash: settlementHash(attr.requestID, meters, fact, completeness, state),
		Request: fact, Meters: meters, Completeness: completeness, State: state,
		DiagnosticCode: diagnosticCode, Details: detailsPointer,
	}
	return h.recorder.Record(settlement)
}

func settlementHash(requestID string, meters []usageingestapi.MeterValue, fact usageingestapi.RequestUsageFact, completeness usageingestapi.UsageCompleteness, state usageingestapi.UsageSettlementState) string {
	payload, _ := json.Marshal(struct {
		RequestID    string
		Meters       []usageingestapi.MeterValue
		Fact         usageingestapi.RequestUsageFact
		Completeness usageingestapi.UsageCompleteness
		State        usageingestapi.UsageSettlementState
	}{requestID, meters, fact, completeness, state})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func meterValues(measurements []protocolusage.Measurement) []usageingestapi.MeterValue {
	values := make([]usageingestapi.MeterValue, 0, len(measurements))
	for _, measurement := range measurements {
		value := usageingestapi.MeterValue{
			Meter:        usageingestapi.UsageMeter(measurement.Meter),
			Completeness: usageingestapi.UsageCompleteness(measurement.Completeness),
		}
		if measurement.Source != "" {
			source := measurement.Source
			value.Source = &source
		}
		if measurement.Value != nil {
			numerator, denominator := measurement.Value.Numerator, measurement.Value.Denominator
			value.Numerator, value.Denominator = &numerator, &denominator
		}
		values = append(values, value)
	}
	return values
}

func detailValues(details []protocolusage.Detail) map[string]int64 {
	values := make(map[string]int64, len(details))
	for _, detail := range details {
		if detail.Value.Denominator == 1 {
			values[detail.Name] = detail.Value.Numerator
		}
	}
	return values
}

func settlementCompleteness(measurements []protocolusage.Measurement) usageingestapi.UsageCompleteness {
	value := usageingestapi.EXACT
	for _, measurement := range measurements {
		switch measurement.Completeness {
		case protocolusage.Unknown:
			return usageingestapi.UNKNOWN
		case protocolusage.Partial:
			value = usageingestapi.PARTIAL
		}
	}
	return value
}

func usageRoute(state *control.State, resourceID string) (control.Resource, control.Route, control.Upstream, bool) {
	if state == nil {
		return control.Resource{}, control.Route{}, control.Upstream{}, false
	}
	resource, ok := state.Resources[resourceID]
	if !ok {
		return control.Resource{}, control.Route{}, control.Upstream{}, false
	}
	route, ok := state.Routes[resource.RouteID]
	if !ok {
		return control.Resource{}, control.Route{}, control.Upstream{}, false
	}
	upstream, ok := state.Upstreams[route.UpstreamID]
	if !ok {
		return control.Resource{}, control.Route{}, control.Upstream{}, false
	}
	return resource, route, upstream, true
}
