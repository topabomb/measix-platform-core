package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"measix/platform/internal/relay/control"
)

var errDAVCredential = errors.New("workspace_credential_unavailable")

type davBufferPool struct{ pool sync.Pool }

func (p *davBufferPool) Get() []byte {
	if b := p.pool.Get(); b != nil {
		return b.([]byte)
	}
	return make([]byte, 32<<10)
}
func (p *davBufferPool) Put(b []byte) {
	if cap(b) == 32<<10 {
		p.pool.Put(b[:32<<10])
	}
}

var davBuffers davBufferPool

func (h *Handler) serveDAV(w http.ResponseWriter, r *http.Request, state *control.State, claims *accessClaims, resourceID, requestID string) {
	fail := func(status int, code string) {
		writeProblem(w, status, code, http.StatusText(status), requestID, nil, false)
	}
	resource, ok := state.Resources[resourceID]
	if !ok || resource.Kind != "WEBDAV" {
		fail(403, "resource_not_allowed")
		return
	}
	route, ok := state.Routes[resource.RouteID]
	if !ok {
		fail(503, "runtime_control_unavailable")
		return
	}
	binding, ok := state.DAVBindings[claims.Subject+"/"+resourceID]
	if !ok {
		fail(403, "workspace_not_connected")
		return
	}
	stats := &davTransferStats{started: h.store.Now()}
	observer, ok := w.(*responseObserver)
	if !ok {
		observer = &responseObserver{ResponseWriter: w}
		w = observer
	}
	defer func() {
		cause := recover()
		if cause != nil {
			stats.errorCode = "transfer_interrupted"
		}
		h.recordDAV(r, state, claims, resource, route, binding, requestID, observer, stats)
		if cause != nil {
			panic(cause)
		}
	}()
	if _, allowed := route.AllowedMethods[r.Method]; !allowed {
		methods := make([]string, 0, len(route.AllowedMethods))
		for method := range route.AllowedMethods {
			if binding.AccessMode != "READ_ONLY" || method == "OPTIONS" || method == "GET" || method == "HEAD" || method == "PROPFIND" {
				methods = append(methods, method)
			}
		}
		sort.Strings(methods)
		w.Header().Set("Allow", strings.Join(methods, ", "))
		fail(405, "method_not_allowed")
		return
	}
	if binding.AccessMode == "READ_ONLY" && r.Method != "OPTIONS" && r.Method != "GET" && r.Method != "HEAD" && r.Method != "PROPFIND" {
		fail(403, "dav_read_only")
		return
	}
	if claims.SessionValidUntil == nil || !claims.SessionValidUntil.After(h.store.Now()) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="measix", error="invalid_token"`)
		fail(401, "session_refresh_required")
		return
	}
	if r.Header.Get("Upgrade") != "" {
		fail(400, "invalid_request")
		return
	}
	upstream, err := url.Parse(binding.Endpoint)
	if err != nil {
		fail(503, "runtime_control_unavailable")
		return
	}
	root := runtimePrefix + resourceID + "/spaces/" + binding.Target.AgentSpaceId + "/dav/"
	mapper := davMapper{publicRoot: root, publicHost: r.Host, publicScheme: "http", upstream: upstream}
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		mapper.publicScheme = "https"
	}
	if r.URL.Scheme != "" {
		mapper.publicScheme = r.URL.Scheme
	}
	target, err := mapper.toUpstream(r.URL)
	if err != nil {
		fail(403, "dav_target_outside_workspace")
		return
	}
	destination := ""
	if raw := r.Header.Get("Destination"); raw != "" {
		u, e := url.Parse(raw)
		if e != nil {
			fail(400, "invalid_destination")
			return
		}
		u, e = mapper.toUpstream(u)
		if e != nil {
			fail(403, "dav_target_outside_workspace")
			return
		}
		destination = u.String()
	}
	condition, err := mapper.mapIf(r.Header.Get("If"))
	if err != nil {
		fail(403, "dav_target_outside_workspace")
		return
	}
	select {
	case h.davSlots <- struct{}{}:
		defer func() { <-h.davSlots }()
	default:
		fail(503, "dav_busy")
		return
	}
	lease, release, err := h.store.Register(r.Context(), func(current *control.State) bool {
		// Authentication was checked at admission. Rechecking a now-expired short
		// JWT would abort valid large transfers on unrelated control updates.
		if current.DeploymentID != state.DeploymentID {
			return false
		}
		if !bytes.Equal(current.AuthKeys[claims.SigningKeyID], state.AuthKeys[claims.SigningKeyID]) {
			return false
		}
		if _, deny := current.DeletedUsers[claims.Subject]; deny {
			return false
		}
		if _, deny := current.DisabledUsers[claims.Subject]; deny {
			return false
		}
		if _, deny := current.RevokedDevices[claims.DeviceID]; deny {
			return false
		}
		if _, deny := current.RevokedSessions[claims.SessionID]; deny {
			return false
		}
		res, exists := current.Resources[resourceID]
		if !exists || !reflect.DeepEqual(res, resource) {
			return false
		}
		currentRoute, exists := current.Routes[resource.RouteID]
		if !exists || !reflect.DeepEqual(currentRoute, route) {
			return false
		}
		b, exists := current.DAVBindings[claims.Subject+"/"+resourceID]
		return exists && b == binding
	})
	if err != nil {
		fail(403, "runtime_access_revoked")
		return
	}
	defer release()
	lease, cancel := context.WithTimeout(lease, claims.SessionValidUntil.Sub(h.store.Now()))
	defer cancel()
	r = r.WithContext(lease)
	idle := time.Duration(route.TimeoutPolicy.IdleMs) * time.Millisecond
	watch := newDAVActivity(idle, cancel, w)
	defer watch.close()
	cancellationDone := make(chan struct{})
	stopCancellation := context.AfterFunc(lease, func() {
		defer close(cancellationDone)
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now())
		_ = controller.SetWriteDeadline(time.Now())
	})
	defer func() {
		if !stopCancellation() {
			<-cancellationDone
		}
	}()
	if r.Body != nil {
		r.Body = &davActivityBody{ReadCloser: r.Body, activity: watch, bytes: &stats.requestBytes}
	}
	writer := &davActivityWriter{ResponseWriter: w, activity: watch, idle: idle}
	_ = http.NewResponseController(w).EnableFullDuplex()
	proxy := &httputil.ReverseProxy{
		BufferPool: &davBuffers,
		Transport:  h.transportFor(route.TimeoutPolicy),
		Rewrite: func(p *httputil.ProxyRequest) {
			stats.forwarded = true
			p.Out.URL = target
			p.Out.Host = ""
			sanitizeOutboundHeaders(p.Out.Header)
			p.Out.Header.Del("X-HTTP-Method-Override")
			p.Out.Header.Del("X-Method-Override")
			p.Out.Header.Del("X-HTTP-Method")
			p.Out.Header.Set("Authorization", "Bearer "+binding.Token)
			if destination != "" {
				p.Out.Header.Set("Destination", destination)
			}
			if condition != "" {
				p.Out.Header.Set("If", condition)
			}
		},
		ModifyResponse: func(response *http.Response) error {
			status := response.StatusCode
			stats.upstreamStatus = &status
			if response.StatusCode == 401 {
				response.Body.Close()
				return errDAVCredential
			}
			response.Header.Del("Set-Cookie")
			response.Header.Del("WWW-Authenticate")
			for _, header := range []string{"Location", "Content-Location"} {
				if raw := response.Header.Get(header); raw != "" {
					mapped, e := mapper.fromUpstream(raw, target)
					if e != nil {
						return e
					}
					response.Header.Set(header, mapped)
				}
			}
			if response.StatusCode == 101 {
				return errors.New("unexpected upgrade")
			}
			response.Body = &davActivityBody{ReadCloser: response.Body, activity: watch}
			if r.Method != "GET" && r.Method != "HEAD" {
				return rewriteDAVXML(response, mapper, target)
			}
			return nil
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			code := "upstream_unavailable"
			status := 502
			if errors.Is(err, errDAVCredential) {
				code = "workspace_credential_unavailable"
			}
			if errors.Is(request.Context().Err(), context.DeadlineExceeded) {
				code = "upstream_timeout"
				status = 504
			}
			stats.errorCode = code
			writeProblem(writer, status, code, http.StatusText(status), requestID, nil, true)
		},
	}
	proxy.ServeHTTP(writer, r)
}

type davMapper struct {
	publicRoot, publicHost, publicScheme string
	upstream                             *url.URL
}

func davRelative(escaped, root string) (string, error) {
	if escaped == strings.TrimSuffix(root, "/") {
		return "", nil
	}
	if !strings.HasPrefix(escaped, root) {
		return "", errors.New("outside DAV root")
	}
	tail := strings.TrimPrefix(escaped, root)
	for _, segment := range strings.Split(tail, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\\x00") {
			return "", errors.New("invalid DAV path")
		}
	}
	return tail, nil
}

func (m davMapper) toUpstream(u *url.URL) (*url.URL, error) {
	if u.User != nil || u.Fragment != "" || (u.Host != "" && (!strings.EqualFold(u.Host, m.publicHost) || u.Scheme != m.publicScheme)) || (u.Host == "" && u.Scheme != "") {
		return nil, errors.New("outside DAV origin")
	}
	tail, err := davRelative(u.EscapedPath(), m.publicRoot)
	if err != nil {
		return nil, err
	}
	result := *m.upstream
	result.RawPath = m.upstream.EscapedPath() + tail
	if u.EscapedPath() == strings.TrimSuffix(m.publicRoot, "/") {
		result.RawPath = strings.TrimSuffix(result.RawPath, "/")
	}
	result.Path, err = url.PathUnescape(result.RawPath)
	result.RawQuery = u.RawQuery
	return &result, err
}

func (m davMapper) fromUpstream(raw string, base *url.URL) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u = base.ResolveReference(u)
	if u.User != nil || u.Fragment != "" || !strings.EqualFold(u.Host, m.upstream.Host) || u.Scheme != m.upstream.Scheme {
		return "", errors.New("outside DAV upstream")
	}
	tail, err := davRelative(u.EscapedPath(), m.upstream.EscapedPath())
	if err != nil {
		return "", err
	}
	result := m.publicRoot + tail
	if u.EscapedPath() == strings.TrimSuffix(m.upstream.EscapedPath(), "/") {
		result = strings.TrimSuffix(result, "/")
	}
	if u.RawQuery != "" {
		result += "?" + u.RawQuery
	}
	return result, nil
}

func (m davMapper) mapIf(raw string) (string, error) {
	var out strings.Builder
	depth := 0
	for i := 0; i < len(raw); {
		ch := raw[i]
		if ch == '(' {
			depth++
		}
		if ch == ')' {
			depth--
		}
		if ch == '"' {
			end := strings.IndexByte(raw[i+1:], '"')
			if end < 0 {
				return "", errors.New("invalid If entity tag")
			}
			out.WriteString(raw[i : i+end+2])
			i += end + 2
			continue
		}
		if ch == '<' {
			end := strings.IndexByte(raw[i:], '>')
			if end < 0 {
				return "", errors.New("invalid If resource tag")
			}
			if depth != 0 {
				out.WriteString(raw[i : i+end+1])
				i += end + 1
				continue
			}
			u, err := url.Parse(raw[i+1 : i+end])
			if err != nil {
				return "", err
			}
			mapped, err := m.toUpstream(u)
			if err != nil {
				return "", err
			}
			out.WriteByte('<')
			out.WriteString(mapped.String())
			out.WriteByte('>')
			i += end + 1
			continue
		}
		out.WriteByte(ch)
		i++
	}
	return out.String(), nil
}

type davActivity struct {
	mu         sync.Mutex
	timer      *time.Timer
	closed     bool
	idle       time.Duration
	cancel     context.CancelFunc
	writer     http.ResponseWriter
	generation uint64
}

func newDAVActivity(idle time.Duration, cancel context.CancelFunc, w http.ResponseWriter) *davActivity {
	a := &davActivity{idle: idle, cancel: cancel, writer: w}
	a.touch()
	return a
}
func (a *davActivity) touch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	if a.timer != nil {
		a.timer.Stop()
	}
	a.generation++
	generation := a.generation
	a.timer = time.AfterFunc(a.idle, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.closed || generation != a.generation {
			return
		}
		a.cancel()
		controller := http.NewResponseController(a.writer)
		_ = controller.SetWriteDeadline(time.Now())
		_ = controller.SetReadDeadline(time.Now())
	})
}
func (a *davActivity) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.timer != nil {
		a.timer.Stop()
	}
	_ = http.NewResponseController(a.writer).SetWriteDeadline(time.Time{})
	_ = http.NewResponseController(a.writer).SetReadDeadline(time.Time{})
}

type davActivityBody struct {
	io.ReadCloser
	activity *davActivity
	bytes    *atomic.Int64
}

func (b *davActivityBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	if n > 0 {
		if b.bytes != nil {
			b.bytes.Add(int64(n))
		}
		b.activity.touch()
	}
	return n, e
}

type davActivityWriter struct {
	http.ResponseWriter
	activity *davActivity
	idle     time.Duration
}

func (w *davActivityWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *davActivityWriter) Write(p []byte) (int, error) {
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(w.idle))
	n, e := w.ResponseWriter.Write(p)
	if n > 0 {
		w.activity.touch()
	}
	return n, e
}
