package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
)

type thumbnailResponseWriter struct {
	http.ResponseWriter
	head    bool
	written int64
}

func (w *thumbnailResponseWriter) Write(data []byte) (int, error) {
	if w.head {
		return len(data), nil
	}
	n, err := w.ResponseWriter.Write(data)
	w.written += int64(n)
	return n, err
}

func thumbnailWriter(w http.ResponseWriter, r *http.Request) *thumbnailResponseWriter {
	if existing, ok := w.(*thumbnailResponseWriter); ok {
		return existing
	}
	out := &thumbnailResponseWriter{ResponseWriter: w, head: r.Method == http.MethodHead}
	out.Header().Set("Cache-Control", "no-store")
	out.Header().Set("X-Content-Type-Options", "nosniff")
	return out
}

func thumbnailPath(path string) bool {
	return path == "/api/client/v1/workspace/thumbnail" || strings.HasPrefix(path, "/api/admin/v1/users/") && strings.HasSuffix(path, "/workspace/thumbnail")
}

func (h *fullAdminHandler) adminWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, userID string) {
	out := thumbnailWriter(w, r)
	actor, ok := h.workspaceAuth(out, r, "", false)
	if !ok {
		return
	}
	serveWorkspaceThumbnail(out, r, h.services.Workspace, actor, userID, h.adminFileAuth(r))
}

func (h *fullAdminHandler) GetAdminWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, id adminapi.UserId, _ adminapi.GetAdminWorkspaceThumbnailParams) {
	h.adminWorkspaceThumbnail(w, r, id)
}

func (h *fullAdminHandler) HeadAdminWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, id adminapi.UserId, _ adminapi.HeadAdminWorkspaceThumbnailParams) {
	h.adminWorkspaceThumbnail(w, r, id)
}

func (h *fullAdminHandler) GetAdminWorkspaceThumbnailCapabilities(w http.ResponseWriter, r *http.Request, id adminapi.UserId, _ adminapi.GetAdminWorkspaceThumbnailCapabilitiesParams) {
	h.adminWorkspaceThumbnail(w, r, id)
}

func (h *fullClientHandler) clientWorkspaceThumbnail(w http.ResponseWriter, r *http.Request) {
	out := thumbnailWriter(w, r)
	principal, ok := h.authenticateClient(out, r)
	if !ok {
		return
	}
	token, _ := bearerToken(r)
	authorize := func(ctx context.Context) error {
		fresh, err := h.identity.AuthenticateAccess(ctx, token)
		if err != nil {
			return err
		}
		if fresh.UserID != principal.UserID {
			return workspace.ErrUnavailable
		}
		return nil
	}
	serveWorkspaceThumbnail(out, r, h.workspace, principal.UserID, principal.UserID, authorize)
}

func (h *fullClientHandler) GetClientWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, _ clientapi.GetClientWorkspaceThumbnailParams) {
	h.clientWorkspaceThumbnail(w, r)
}

func (h *fullClientHandler) HeadClientWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, _ clientapi.HeadClientWorkspaceThumbnailParams) {
	h.clientWorkspaceThumbnail(w, r)
}

func (h *fullClientHandler) GetClientWorkspaceThumbnailCapabilities(w http.ResponseWriter, r *http.Request, _ clientapi.GetClientWorkspaceThumbnailCapabilitiesParams) {
	h.clientWorkspaceThumbnail(w, r)
}

func invalidThumbnailRequest() error {
	return &remoteapi.Error{Code: "invalid_preview_request", Status: http.StatusBadRequest}
}

func parseThumbnailRequest(r *http.Request) (string, remoteapi.PreviewRequest, error) {
	request := remoteapi.PreviewRequest{Method: r.Method, Size: 256}
	if r.Method != http.MethodOptions && r.Method != http.MethodGet && r.Method != http.MethodHead {
		return "", request, &remoteapi.Error{Code: "preview_method_not_allowed", Status: 405}
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Transfer-Encoding") != "" {
		return "", request, invalidThumbnailRequest()
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", request, invalidThumbnailRequest()
	}
	for key, values := range query {
		if len(values) != 1 || key != "agentSpaceId" && (r.Method == http.MethodOptions || key != "path" && key != "size" && key != "expectedEtag") {
			return "", request, invalidThumbnailRequest()
		}
	}
	space := query.Get("agentSpaceId")
	if space == "" {
		return "", request, invalidThumbnailRequest()
	}
	if r.Method == http.MethodOptions {
		return space, request, nil
	}
	request.Path, request.ExpectedETag = query.Get("path"), query.Get("expectedEtag")
	if request.Path == "" || !strongMediaETag(request.ExpectedETag) {
		return "", request, invalidThumbnailRequest()
	}
	if raw, present := query["size"]; present {
		request.Size, err = strconv.Atoi(raw[0])
		if err != nil || request.Size != 128 && request.Size != 256 && request.Size != 512 {
			return "", request, invalidThumbnailRequest()
		}
	}
	conditions := make(map[string]string, 2)
	for _, header := range []string{"If-Match", "If-None-Match"} {
		values := r.Header.Values(header)
		combined := strings.Join(values, ", ")
		if len(values) != 0 && strings.TrimSpace(combined) == "" {
			return "", request, invalidThumbnailRequest()
		}
		conditions[header] = combined
	}
	request.IfMatch, request.IfNoneMatch = conditions["If-Match"], conditions["If-None-Match"]
	return space, request, nil
}

func serveWorkspaceThumbnail(w http.ResponseWriter, r *http.Request, service *workspace.Service, actor, userID string, authorize func(context.Context) error) {
	out := thumbnailWriter(w, r)
	space, request, err := parseThumbnailRequest(r)
	if err != nil {
		writeThumbnailError(out, err)
		return
	}
	if service == nil {
		writeThumbnailError(out, workspace.ErrUnavailable)
		return
	}
	action := "THUMBNAIL"
	if r.Method == http.MethodOptions {
		action = "THUMBNAIL_OPTIONS"
	}
	access, err := service.OpenFiles(r.Context(), actor, userID, space, action, request.Path, authorize)
	if err != nil {
		writeThumbnailError(out, err)
		return
	}
	outcome, startBytes := "REJECTED", out.written
	defer func() { service.CloseFiles(access, outcome, out.written-startBytes) }()
	remoteError := func(err error) {
		service.ObserveFileError(context.WithoutCancel(r.Context()), access, err)
		writeThumbnailError(out, err)
	}
	if r.Method == http.MethodOptions {
		capability, err := access.Remote.PreviewCapabilities(access.Context, access.Username, access.Token, space)
		if err != nil {
			remoteError(err)
			return
		}
		if err := access.Revalidate(r.Context()); err != nil {
			writeThumbnailError(out, err)
			return
		}
		out.Header().Set("Agent-Space-Id", space)
		out.Header().Set("Content-Type", "application/json")
		out.WriteHeader(200)
		if err := json.NewEncoder(out).Encode(capability); err != nil {
			outcome = "UNKNOWN"
			return
		}
		outcome = "SUCCEEDED"
		return
	}
	response, err := access.Remote.Preview(access.Context, access.Username, access.Token, space, request)
	if err != nil {
		remoteError(err)
		return
	}
	if err := access.Revalidate(r.Context()); err != nil {
		writeThumbnailError(out, err)
		return
	}
	for _, header := range []string{"ETag", "Preview-Source-ETag", "Content-Length"} {
		if value := response.Header.Get(header); value != "" {
			out.Header().Set(header, value)
		}
	}
	out.Header().Set("Agent-Space-Id", space)
	out.Header().Set("Content-Type", "image/webp")
	out.Header().Set("Content-Disposition", "inline; filename=thumbnail.webp")
	out.WriteHeader(response.Status)
	if r.Method != http.MethodHead && response.Status != http.StatusNotModified {
		if _, err := out.Write(response.Data); err != nil {
			outcome = "UNKNOWN"
			return
		}
	}
	outcome = "SUCCEEDED"
}

func writeThumbnailError(w http.ResponseWriter, err error) {
	var remote *remoteapi.Error
	if !errors.As(err, &remote) {
		writeWorkspaceError(w, err)
		return
	}
	status := 503
	switch remote.Code {
	case "invalid_preview_request":
		status = 400
	case "file_forbidden":
		status = 403
	case "preview_method_not_allowed":
		status = 405
		w.Header().Set("Allow", "OPTIONS, GET, HEAD")
	case "preview_source_limit":
		status = 413
	case "preview_unsupported_media":
		status = 415
	case "preview_decode_failed":
		status = 422
	case "preview_busy":
		status = 429
		w.Header().Set("Retry-After", "1")
	case "preview_not_supported":
		status = 501
	case "preview_unavailable", "preview_protocol_error", "preview_dependency_unavailable", "preview_timeout":
	case "file_not_found", "file_version_conflict", "workspace_space_mismatch", "invalid_file_path", "invalid_file_request", "dav_credential_unavailable":
		writeWorkspaceError(w, err)
		return
	default:
		writeProblem(w, 503, "preview_unavailable", "Thumbnail could not be obtained")
		return
	}
	writeProblem(w, status, remote.Code, "Thumbnail could not be obtained")
}
