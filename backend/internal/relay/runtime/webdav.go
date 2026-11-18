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
	"strconv"
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
	preview := false
	fail := func(status int, code string) {
		if preview {
			w.Header().Set("Cache-Control", "no-store")
		}
		writeDAVProblem(w, r.Method, status, code, requestID, false)
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
	spaceRoot := runtimePrefix + resourceID + "/spaces/" + binding.Target.AgentSpaceId + "/"
	preview = r.URL.EscapedPath() == spaceRoot+"previews"
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
	if preview && r.Method != "OPTIONS" && r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "OPTIONS, GET, HEAD")
		fail(405, "preview_method_not_allowed")
		return
	}
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
	mapper := davMapper{publicRoot: spaceRoot + "dav/", publicHost: r.Host, publicScheme: "http", upstream: upstream}
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		mapper.publicScheme = "https"
	}
	if r.URL.Scheme != "" {
		mapper.publicScheme = r.URL.Scheme
	}
	var target *url.URL
	var destination, condition string
	if preview {
		if !mapper.sameOrigin(r.URL) {
			fail(403, "dav_target_outside_workspace")
			return
		}
		if !davPreviewEmptyFraming(r) {
			fail(400, "invalid_preview_request")
			return
		}
		if !davPreviewOriginAllowed(r.Header, mapper.publicScheme+"://"+mapper.publicHost) {
			fail(403, "invalid_origin")
			return
		}
		previewTarget := *upstream
		previewTarget.Path = strings.TrimSuffix(upstream.Path, "dav/") + "previews"
		previewTarget.RawQuery = r.URL.RawQuery
		previewTarget.ForceQuery = r.URL.ForceQuery
		target = &previewTarget
	} else {
		target, err = mapper.toUpstream(r.URL)
		if err != nil {
			fail(403, "dav_target_outside_workspace")
			return
		}
		if target.Path == upstream.Path || target.Path == strings.TrimSuffix(upstream.Path, "/") {
			switch r.Method {
			case "PUT", "DELETE", "MKCOL", "MOVE":
				fail(403, "dav_target_outside_workspace")
				return
			}
		}
		if raw := r.Header.Get("Destination"); raw != "" {
			u, e := url.Parse(raw)
			if e != nil {
				fail(400, "invalid_destination")
				return
			}
			u, e = mapper.toUpstream(u)
			if e != nil || u.Path == upstream.Path || u.Path == strings.TrimSuffix(upstream.Path, "/") {
				fail(403, "dav_target_outside_workspace")
				return
			}
			destination = u.String()
		}
		condition, err = mapper.mapIf(r.Header.Get("If"))
		if err != nil {
			fail(403, "dav_target_outside_workspace")
			return
		}
	}
	slots := h.davSlots
	if preview {
		slots = h.davPreviewSlots
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		if preview {
			w.Header().Set("Retry-After", "1")
			fail(429, "preview_busy")
			return
		}
		fail(503, "dav_busy")
		return
	}
	if preview && r.Body != nil && r.Body != http.NoBody {
		if !finishEmptyDAVPreviewStream(w, r.Body) {
			fail(400, "invalid_preview_request")
			return
		}
		r.Body = http.NoBody
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
	watchRead := r.ContentLength != 0 && r.Body != nil && r.Body != http.NoBody
	watch := newDAVActivity(idle, lease, cancel, w, watchRead)
	defer watch.close()
	cancellationDone := make(chan struct{})
	stopCancellation := context.AfterFunc(lease, func() {
		defer close(cancellationDone)
		watch.interrupt()
	})
	defer func() {
		if !stopCancellation() {
			<-cancellationDone
		}
	}()
	if watchRead {
		r.Body = &davActivityBody{ReadCloser: r.Body, activity: watch, bytes: &stats.requestBytes}
	}
	writer := &davActivityWriter{ResponseWriter: w, activity: watch}
	if !preview {
		_ = http.NewResponseController(w).EnableFullDuplex()
	}
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
			if preview {
				for name := range p.Out.Header {
					if strings.HasPrefix(strings.ToLower(name), "sec-fetch-") {
						delete(p.Out.Header, name)
					}
				}
				p.Out.Header.Del("Origin")
				p.Out.Header.Del("Destination")
				p.Out.Header.Del("If")
			}
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
			if err := lease.Err(); err != nil {
				return err
			}
			if response.StatusCode == 401 {
				response.Body.Close()
				return errDAVCredential
			}
			response.Header.Del("Set-Cookie")
			response.Header.Del("WWW-Authenticate")
			if preview {
				if err := validateDAVPreviewResponse(response, binding.Target.AgentSpaceId); err != nil {
					return err
				}
				response.Body = &davActivityBody{ReadCloser: response.Body, activity: watch}
				return nil
			}
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
		ErrorHandler: func(_ http.ResponseWriter, request *http.Request, err error) {
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
			if preview {
				w.Header().Set("Cache-Control", "no-store")
			}
			writeDAVProblem(w, request.Method, status, code, requestID, true)
		},
	}
	proxy.ServeHTTP(writer, r)
}

func davPreviewEmptyFraming(r *http.Request) bool {
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.Trailer) != 0 {
		return false
	}
	lengths := r.Header.Values("Content-Length")
	if len(lengths) > 1 {
		return false
	}
	if len(lengths) == 1 {
		size, err := strconv.ParseUint(lengths[0], 10, 64)
		if err != nil || size != 0 {
			return false
		}
	}
	return r.Body == nil || r.Body == http.NoBody || r.ProtoMajor == 2
}

func finishEmptyDAVPreviewStream(w http.ResponseWriter, body io.ReadCloser) bool {
	// net/http gives HTTP/2 requests a Body even when HEADERS ends the stream.
	// Bound the confirmation of EOF in case an explicitly empty stream stays open.
	const emptyStreamTimeout = time.Second
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(emptyStreamTimeout)); err != nil {
		return false
	}
	defer controller.SetReadDeadline(time.Time{})
	defer body.Close()
	var firstByte [1]byte
	n, err := io.ReadFull(body, firstByte[:])
	return n == 0 && err == io.EOF
}

func writeDAVProblem(w http.ResponseWriter, method string, status int, code, requestID string, forwarded bool) {
	if method == http.MethodHead {
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("X-Measix-Request-Id", requestID)
		w.WriteHeader(status)
		return
	}
	writeProblem(w, status, code, http.StatusText(status), requestID, nil, forwarded)
}

func davPreviewOriginAllowed(header http.Header, publicOrigin string) bool {
	origins := header.Values("Origin")
	if len(origins) > 1 || (len(origins) == 1 && !strings.EqualFold(origins[0], publicOrigin)) {
		return false
	}
	sites := header.Values("Sec-Fetch-Site")
	return len(sites) == 0 || len(sites) == 1 && (sites[0] == "same-origin" || sites[0] == "none")
}

func validateDAVPreviewResponse(response *http.Response, agentSpaceID string) error {
	status := response.StatusCode
	if status == http.StatusSwitchingProtocols || (status >= 300 && status < 400 && status != http.StatusNotModified) {
		return errors.New("unexpected DAV preview redirect or upgrade")
	}
	if status >= 200 && status < 300 || status == http.StatusNotModified {
		spaces := response.Header.Values("Agent-Space-Id")
		if len(spaces) != 1 || spaces[0] != agentSpaceID {
			return errors.New("unexpected DAV preview space")
		}
	}
	response.Header.Del("Location")
	response.Header.Del("Content-Location")
	response.Header.Set("Cache-Control", "no-store")
	return nil
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
	segments := strings.Split(tail, "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil || (decoded == "" && i < len(segments)-1) || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\\x00\r\n") {
			return "", errors.New("invalid DAV path")
		}
	}
	return tail, nil
}

func (m davMapper) sameOrigin(u *url.URL) bool {
	if u.User != nil || u.Fragment != "" {
		return false
	}
	if u.Host == "" {
		return u.Scheme == ""
	}
	return strings.EqualFold(u.Host, m.publicHost) && u.Scheme == m.publicScheme
}

func (m davMapper) toUpstream(u *url.URL) (*url.URL, error) {
	if !m.sameOrigin(u) {
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
	mu          sync.Mutex
	timer       *time.Timer
	deadline    time.Time
	closed      bool
	interrupted bool
	idle        time.Duration
	lease       context.Context
	cancel      context.CancelFunc
	writer      http.ResponseWriter
	watchRead   bool
}

func newDAVActivity(idle time.Duration, lease context.Context, cancel context.CancelFunc, w http.ResponseWriter, watchRead bool) *davActivity {
	a := &davActivity{idle: idle, lease: lease, cancel: cancel, writer: w, watchRead: watchRead, deadline: time.Now().Add(idle)}
	a.mu.Lock()
	a.timer = time.AfterFunc(idle, a.expire)
	a.mu.Unlock()
	return a
}

func (a *davActivity) touch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed && !a.interrupted {
		a.deadline = time.Now().Add(a.idle)
	}
}

func (a *davActivity) expire() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.interrupted {
		return
	}
	if remaining := time.Until(a.deadline); remaining > 0 {
		a.timer.Reset(remaining)
		return
	}
	a.cancel()
	a.interruptLocked()
}

func (a *davActivity) interrupt() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.interruptLocked()
}

func (a *davActivity) interruptLocked() {
	if a.closed || a.interrupted {
		return
	}
	a.interrupted = true
	a.timer.Stop()
	controller := http.NewResponseController(a.writer)
	deadline := time.Now()
	_ = controller.SetWriteDeadline(deadline)
	// A bodyless request has no upload to interrupt. Expiring the HTTP/1 read
	// deadline would cancel the background reader for the keep-alive connection.
	if a.watchRead {
		_ = controller.SetReadDeadline(deadline)
	}
}

func (a *davActivity) beforeWrite() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.lease.Err(); err != nil {
		return err
	}
	if a.interrupted || a.closed {
		return context.Canceled
	}
	_ = http.NewResponseController(a.writer).SetWriteDeadline(time.Now().Add(a.idle))
	return nil
}

func (a *davActivity) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	a.timer.Stop()
	controller := http.NewResponseController(a.writer)
	_ = controller.SetWriteDeadline(time.Time{})
	if a.watchRead {
		_ = controller.SetReadDeadline(time.Time{})
	}
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
}

func (w *davActivityWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *davActivityWriter) Write(p []byte) (int, error) {
	if err := w.activity.beforeWrite(); err != nil {
		return 0, err
	}
	n, e := w.ResponseWriter.Write(p)
	if n > 0 {
		w.activity.touch()
	}
	return n, e
}
