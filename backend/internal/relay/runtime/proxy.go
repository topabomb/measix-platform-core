package runtime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
)

var errUpstreamRedirect = errors.New("upstream redirect is not allowed")

type transportKey struct {
	connectMs        int
	responseHeaderMs int
}

type proxyResult struct {
	UpstreamStatus *int
	ErrorClass     string
	IdleTimedOut   atomic.Bool
}

type idleReadCloser struct {
	body       io.ReadCloser
	timeout    time.Duration
	timedOut   *atomic.Bool
	mu         sync.Mutex
	timer      *time.Timer
	generation uint64
	closed     bool
}

func newIdleReadCloser(body io.ReadCloser, timeout time.Duration, timedOut *atomic.Bool) io.ReadCloser {
	reader := &idleReadCloser{body: body, timeout: timeout, timedOut: timedOut}
	reader.reset()
	return reader
}

func (r *idleReadCloser) Read(value []byte) (int, error) {
	n, err := r.body.Read(value)
	if n > 0 {
		r.reset()
	}
	if err != nil {
		r.stopTimer()
	}
	return n, err
}

func (r *idleReadCloser) Close() error {
	return r.close()
}

func (r *idleReadCloser) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.generation++
	generation := r.generation
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(r.timeout, func() { r.expire(generation) })
}

func (r *idleReadCloser) expire(generation uint64) {
	r.mu.Lock()
	if r.closed || generation != r.generation {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.timedOut != nil {
		r.timedOut.Store(true)
	}
	r.mu.Unlock()
	_ = r.body.Close()
}

func (r *idleReadCloser) stopTimer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
	}
}

func (r *idleReadCloser) close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	if r.timer != nil {
		r.timer.Stop()
	}
	r.mu.Unlock()
	return r.body.Close()
}

func (h *Handler) serveProxy(w http.ResponseWriter, r *http.Request, route control.Route, upstream control.Upstream, runtimePath, requestID string, result *proxyResult, maxRequestBytes int64) {
	target := targetURL(upstream.BaseURL, runtimePath, r.URL.RawQuery)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.Out.URL = target
			request.Out.Host = ""
			sanitizeOutboundHeaders(request.Out.Header)
			if route.TransportPolicy == relaycontrolapi.WEBSOCKET {
				request.Out.Header.Set("Connection", "Upgrade")
				request.Out.Header.Set("Upgrade", "websocket")
			}
			request.Out.Header.Set("X-Measix-Request-Id", requestID)
			applyUpstreamAuth(request.Out, upstream.Auth)
		},
		Transport:     h.transportFor(route.TimeoutPolicy),
		FlushInterval: -1,
		ModifyResponse: func(response *http.Response) error {
			status := response.StatusCode
			result.UpstreamStatus = &status
			if response.StatusCode >= 300 && response.StatusCode < 400 && response.Header.Get("Location") != "" {
				return errUpstreamRedirect
			}
			if response.StatusCode == http.StatusSwitchingProtocols {
				if route.TransportPolicy != relaycontrolapi.WEBSOCKET || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") {
					return errors.New("unexpected protocol upgrade")
				}
				conn, ok := response.Body.(io.ReadWriteCloser)
				if !ok {
					return errors.New("upgrade body is not bidirectional")
				}
				observer := w.(*responseObserver)
				observer.status = http.StatusSwitchingProtocols
				observer.tunnel = newUpgradedStream(conn, time.Duration(route.TimeoutPolicy.IdleMs)*time.Millisecond, maxRequestBytes, observer.observation, response.Header.Get("Sec-WebSocket-Extensions"))
				response.Body = observer.tunnel
			} else {
				response.Body = newIdleReadCloser(response.Body, time.Duration(route.TimeoutPolicy.IdleMs)*time.Millisecond, &result.IdleTimedOut)
				if observer, ok := w.(*responseObserver); ok && observer.observation != nil {
					observer.observation.configureResponse(response)
				}
			}
			sanitizeResponseHeaders(response.Header)
			if response.StatusCode == http.StatusSwitchingProtocols {
				response.Header.Set("Connection", "Upgrade")
				response.Header.Set("Upgrade", "websocket")
			}
			response.Header.Set("X-Measix-Request-Id", requestID)
			return nil
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			var timeoutError net.Error
			switch {
			case errors.Is(err, errUpstreamRedirect):
				result.ErrorClass = "UPSTREAM_PROTOCOL_ERROR"
				writeProblem(writer, http.StatusBadGateway, "upstream_protocol_error", "Upstream redirect is not allowed", requestID, nil, true)
			case errors.Is(err, context.Canceled) || errors.Is(request.Context().Err(), context.Canceled):
				result.ErrorClass = "CLIENT_CANCELLED"
				writeProblem(writer, http.StatusBadGateway, "upstream_unavailable", "Upstream request cancelled", requestID, nil, true)
			// HTTP/2 response-header timeouts implement net.Error but do not wrap context.DeadlineExceeded.
			case errors.Is(err, context.DeadlineExceeded) || errors.Is(request.Context().Err(), context.DeadlineExceeded) ||
				errors.As(err, &timeoutError) && timeoutError.Timeout():
				result.ErrorClass = "UPSTREAM_TIMEOUT"
				writeProblem(writer, http.StatusGatewayTimeout, "upstream_timeout", "Upstream timeout", requestID, nil, true)
			default:
				result.ErrorClass = "UPSTREAM_UNAVAILABLE"
				writeProblem(writer, http.StatusBadGateway, "upstream_unavailable", "Upstream unavailable", requestID, nil, true)
			}
		},
	}

	request := r
	if route.TimeoutPolicy.OverallMs != nil {
		var cancel context.CancelFunc
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(*route.TimeoutPolicy.OverallMs)*time.Millisecond)
		defer cancel()
		request = r.WithContext(ctx)
	}
	defer func() {
		if cause := recover(); cause != nil {
			result.ErrorClass = "INTERNAL_ERROR"
			if cause == http.ErrAbortHandler {
				if result.IdleTimedOut.Load() {
					result.ErrorClass = "UPSTREAM_TIMEOUT"
				} else {
					switch request.Context().Err() {
					case context.Canceled:
						result.ErrorClass = "CLIENT_CANCELLED"
					case context.DeadlineExceeded:
						result.ErrorClass = "UPSTREAM_TIMEOUT"
					default:
						result.ErrorClass = "UPSTREAM_UNAVAILABLE"
					}
				}
			}
			// Preserve net/http's connection abort; a truncated 200 is not success.
			panic(cause)
		}
	}()
	proxy.ServeHTTP(w, request)
	if result.IdleTimedOut.Load() {
		result.ErrorClass = "UPSTREAM_TIMEOUT"
	}
	if observer, ok := w.(*responseObserver); ok && observer.tunnel != nil {
		if observer.tunnel.timedOut.Load() {
			result.ErrorClass = "UPSTREAM_TIMEOUT"
		}
		if observer.tunnel.exceeded.Load() {
			result.ErrorClass = "REQUEST_TOO_LARGE"
		}
		if errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			result.ErrorClass = "UPSTREAM_TIMEOUT"
		} else if errors.Is(request.Context().Err(), context.Canceled) {
			result.ErrorClass = "CLIENT_CANCELLED"
		}
	}
}

func (h *Handler) transportFor(policy relaycontrolapi.TimeoutPolicy) http.RoundTripper {
	key := transportKey{connectMs: policy.ConnectMs, responseHeaderMs: policy.ResponseHeaderMs}
	if cached, ok := h.transports.Load(key); ok {
		return cached.(http.RoundTripper)
	}
	transport := h.baseTransport.Clone()
	dialer := &net.Dialer{Timeout: time.Duration(policy.ConnectMs) * time.Millisecond, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
	transport.ResponseHeaderTimeout = time.Duration(policy.ResponseHeaderMs) * time.Millisecond
	actual, _ := h.transports.LoadOrStore(key, transport)
	return actual.(http.RoundTripper)
}

func sanitizeOutboundHeaders(header http.Header) {
	connectionTokens := strings.Split(header.Get("Connection"), ",")
	for _, token := range connectionTokens {
		header.Del(strings.TrimSpace(token))
	}
	for key := range header {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "cookie" || lower == "host" ||
			strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "x-measix-") ||
			lower == "x-real-ip" || lower == "x-request-id" || lower == "x-requested-with" ||
			hopByHop(lower) {
			header.Del(key)
		}
	}
}

func applyUpstreamAuth(request *http.Request, auth control.UpstreamAuth) {
	switch auth.Type {
	case relaycontrolapi.BEARER:
		request.Header.Set("Authorization", "Bearer "+auth.Token)
	case relaycontrolapi.STATICHEADER:
		request.Header.Set(auth.HeaderName, auth.Value)
	case relaycontrolapi.BASIC:
		request.SetBasicAuth(auth.Username, auth.Password)
	}
}

func sanitizeResponseHeaders(header http.Header) {
	for key := range header {
		lower := strings.ToLower(key)
		if lower == "set-cookie" || lower == "location" || strings.HasPrefix(lower, "x-measix-") || hopByHop(lower) {
			header.Del(key)
		}
	}
}

func hopByHop(lower string) bool {
	switch lower {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
