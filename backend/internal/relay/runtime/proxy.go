package runtime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
)

var errUpstreamRedirect = errors.New("upstream redirect is not allowed")

type transportKey struct {
	connectMs        int
	responseHeaderMs int
	idleMs           int
}

type proxyResult struct {
	UpstreamStatus *int
	ErrorClass     string
}

func (h *Handler) serveProxy(w http.ResponseWriter, r *http.Request, route control.Route, upstream control.Upstream, runtimePath, requestID string) proxyResult {
	result := proxyResult{}
	target := targetURL(upstream.BaseURL, runtimePath, r.URL.RawQuery)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.Out.URL = target
			request.Out.Host = ""
			sanitizeOutboundHeaders(request.Out.Header)
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
			sanitizeResponseHeaders(response.Header)
			response.Header.Set("X-Measix-Request-Id", requestID)
			return nil
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			switch {
			case errors.Is(err, errUpstreamRedirect):
				result.ErrorClass = "UPSTREAM_PROTOCOL_ERROR"
				writeProblem(writer, http.StatusBadGateway, "upstream_protocol_error", "Upstream redirect is not allowed", requestID, nil, true)
			case errors.Is(err, context.Canceled) || errors.Is(request.Context().Err(), context.Canceled):
				result.ErrorClass = "CLIENT_CANCELLED"
				writeProblem(writer, http.StatusBadGateway, "upstream_unavailable", "Upstream request cancelled", requestID, nil, true)
			case errors.Is(err, context.DeadlineExceeded) || errors.Is(request.Context().Err(), context.DeadlineExceeded):
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
	proxy.ServeHTTP(w, request)
	return result
}

func (h *Handler) transportFor(policy relaycontrolapi.TimeoutPolicy) http.RoundTripper {
	key := transportKey{connectMs: policy.ConnectMs, responseHeaderMs: policy.ResponseHeaderMs, idleMs: policy.IdleMs}
	if cached, ok := h.transports.Load(key); ok {
		return cached.(http.RoundTripper)
	}
	transport := h.baseTransport.Clone()
	dialer := &net.Dialer{Timeout: time.Duration(policy.ConnectMs) * time.Millisecond, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
	transport.ResponseHeaderTimeout = time.Duration(policy.ResponseHeaderMs) * time.Millisecond
	transport.IdleConnTimeout = time.Duration(policy.IdleMs) * time.Millisecond
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
		if lower == "authorization" || lower == "cookie" || lower == "host" || strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "x-measix-") || hopByHop(lower) {
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
		if lower == "set-cookie" || strings.HasPrefix(lower, "x-measix-") || hopByHop(lower) {
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
