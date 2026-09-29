package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"

	"measix/platform/ent"
	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
)

func writeWorkspaceError(w http.ResponseWriter, err error) {
	status, code := 503, "workspace_unavailable"
	switch {
	case ent.IsNotFound(err):
		status, code = 404, "workspace_not_found"
	case errors.Is(err, workspace.ErrConflict):
		status, code = 409, "workspace_revision_conflict"
	case errors.Is(err, workspace.ErrPending):
		status, code = 409, "workspace_operation_pending"
	case errors.Is(err, workspace.ErrInvalid):
		status, code = 400, "invalid_workspace_request"
	}
	var remote *remoteapi.Error
	if errors.As(err, &remote) {
		code = remote.Code
		switch code {
		case "file_not_found":
			status = 404
		case "file_version_conflict", "file_conflict", "file_locked":
			status = 409
		case "file_storage_full":
			status = 507
		case "file_range_invalid":
			status = 416
		case "file_transfer_limit":
			status = 429
		case "invalid_file_path", "invalid_file_request", "invalid_file_condition", "file_condition_required", "recursive_confirmation_required", "directory_overwrite_forbidden":
			status = 400
		}
		if remote.Unknown {
			code = "workspace_result_unknown"
		}
	}
	writeProblem(w, status, code, "Remote workspace operation could not be completed")
}
func (h *fullAdminHandler) workspaceAuth(w http.ResponseWriter, r *http.Request, csrf string, mutation bool) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	actor, err := h.authenticateAdmin(r, csrf, mutation)
	if err != nil {
		writeIdentityError(w, err)
		return "", false
	}
	if h.services.Workspace == nil {
		writeWorkspaceError(w, workspace.ErrUnavailable)
		return "", false
	}
	return actor.UserID, true
}
func (h *fullAdminHandler) ListWorkspaceServices(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.workspaceAuth(w, r, "", false); !ok {
		return
	}
	out, err := h.services.Workspace.List(r.Context())
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) GetWorkspaceService(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId) {
	if _, ok := h.workspaceAuth(w, r, "", false); !ok {
		return
	}
	out, err := h.services.Workspace.Get(r.Context(), id)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) CreateWorkspaceService(w http.ResponseWriter, r *http.Request, p adminapi.CreateWorkspaceServiceParams) {
	h.saveWorkspaceService(w, r, "", p.XCSRFToken, p.IdempotencyKey)
}
func (h *fullAdminHandler) UpdateWorkspaceService(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.UpdateWorkspaceServiceParams) {
	h.saveWorkspaceService(w, r, id, p.XCSRFToken, p.IdempotencyKey)
}
func (h *fullAdminHandler) saveWorkspaceService(w http.ResponseWriter, r *http.Request, id, csrf, key string) {
	actor, ok := h.workspaceAuth(w, r, csrf, true)
	if !ok {
		return
	}
	var input adminapi.SaveWorkspaceServiceRequest
	if decodeStrictJSON(r, &input) != nil {
		writeWorkspaceError(w, workspace.ErrInvalid)
		return
	}
	out, err := h.services.Workspace.Save(r.Context(), actor, key, id, input)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) CheckWorkspaceService(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.CheckWorkspaceServiceParams) {
	if _, ok := h.workspaceAuth(w, r, p.XCSRFToken, true); !ok {
		return
	}
	out, err := h.services.Workspace.Check(r.Context(), id)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) ApplyWorkspaceService(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.ApplyWorkspaceServiceParams) {
	h.workspaceServiceCommand(w, r, id, p.XCSRFToken, p.IdempotencyKey, "APPLY")
}
func (h *fullAdminHandler) DisableWorkspaceService(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.DisableWorkspaceServiceParams) {
	h.workspaceServiceCommand(w, r, id, p.XCSRFToken, p.IdempotencyKey, "DISABLE")
}
func (h *fullAdminHandler) workspaceServiceCommand(w http.ResponseWriter, r *http.Request, id, csrf, key, action string) {
	actor, ok := h.workspaceAuth(w, r, csrf, true)
	if !ok {
		return
	}
	out, err := h.services.Workspace.WorkspaceServiceCommand(r.Context(), actor, key, id, action)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	h.services.Workspace.CancelInvalidFiles(r.Context())
	writeJSON(w, 202, out)
}
func (h *fullAdminHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.ListWorkspacesParams) {
	if _, ok := h.workspaceAuth(w, r, "", false); !ok {
		return
	}
	out, err := h.services.Workspace.ListWorkspaces(r.Context(), id, optionalString(p.Search), optionalString(p.Cursor))
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) GetUserWorkspace(w http.ResponseWriter, r *http.Request, id adminapi.UserId) {
	if _, ok := h.workspaceAuth(w, r, "", false); !ok {
		return
	}
	out, err := h.services.Workspace.Projection(r.Context(), id)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) GetWorkspaceOperation(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceOperationId) {
	if _, ok := h.workspaceAuth(w, r, "", false); !ok {
		return
	}
	out, err := h.services.Workspace.Operation(r.Context(), id)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullAdminHandler) CommandWorkspace(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.CommandWorkspaceParams) {
	actor, ok := h.workspaceAuth(w, r, p.XCSRFToken, true)
	if !ok {
		return
	}
	var input adminapi.WorkspaceCommand
	if decodeStrictJSON(r, &input) != nil {
		writeWorkspaceError(w, workspace.ErrInvalid)
		return
	}
	out, err := h.services.Workspace.Command(r.Context(), actor, p.IdempotencyKey, id, input)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	h.services.Workspace.CancelInvalidFiles(r.Context())
	writeJSON(w, 202, out)
}
func (h *fullAdminHandler) adminFileAuth(r *http.Request) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := h.authenticateAdmin(r.WithContext(ctx), "", false)
		return err
	}
}
func (h *fullAdminHandler) RevealWorkspaceDAV(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.RevealWorkspaceDAVParams) {
	actor, ok := h.workspaceAuth(w, r, p.XCSRFToken, true)
	if !ok {
		return
	}
	out, err := h.services.Workspace.RevealDAV(r.Context(), actor, id, h.adminFileAuth(r))
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func optionalString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (h *fullAdminHandler) ListAdminWorkspaceFiles(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.ListAdminWorkspaceFilesParams) {
	actor, ok := h.workspaceAuth(w, r, "", false)
	if !ok {
		return
	}
	serveWorkspaceFiles(w, r, h.services.Workspace, actor, id, "LIST", optionalString(p.Path), h.adminFileAuth(r))
}
func (h *fullAdminHandler) MutateAdminWorkspaceFile(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.MutateAdminWorkspaceFileParams) {
	actor, ok := h.workspaceAuth(w, r, p.XCSRFToken, true)
	if !ok {
		return
	}
	serveWorkspaceFiles(w, r, h.services.Workspace, actor, id, "MUTATE", "", h.adminFileAuth(r))
}
func (h *fullAdminHandler) DownloadAdminWorkspaceFile(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.DownloadAdminWorkspaceFileParams) {
	actor, ok := h.workspaceAuth(w, r, "", false)
	if !ok {
		return
	}
	serveWorkspaceFiles(w, r, h.services.Workspace, actor, id, "GET", optionalString(p.Path), h.adminFileAuth(r))
}
func (h *fullAdminHandler) HeadAdminWorkspaceFile(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.HeadAdminWorkspaceFileParams) {
	actor, ok := h.workspaceAuth(w, r, "", false)
	if !ok {
		return
	}
	serveWorkspaceFiles(w, r, h.services.Workspace, actor, id, "HEAD", optionalString(p.Path), h.adminFileAuth(r))
}
func (h *fullAdminHandler) UploadAdminWorkspaceFile(w http.ResponseWriter, r *http.Request, id adminapi.UserId, p adminapi.UploadAdminWorkspaceFileParams) {
	actor, ok := h.workspaceAuth(w, r, p.XCSRFToken, true)
	if !ok {
		return
	}
	serveWorkspaceFiles(w, r, h.services.Workspace, actor, id, "PUT", optionalString(p.Path), h.adminFileAuth(r))
}

func (h *fullClientHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.authenticateClient(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if h.workspace == nil {
		writeWorkspaceError(w, workspace.ErrUnavailable)
		return
	}
	out, err := h.workspace.Projection(r.Context(), principal.UserID)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *fullClientHandler) clientWorkspaceFiles(w http.ResponseWriter, r *http.Request, action, path string) {
	principal, ok := h.authenticateClient(w, r)
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
	serveWorkspaceFiles(w, r, h.workspace, principal.UserID, principal.UserID, action, path, authorize)
}
func (h *fullClientHandler) ListClientWorkspaceFiles(w http.ResponseWriter, r *http.Request, p clientapi.ListClientWorkspaceFilesParams) {
	h.clientWorkspaceFiles(w, r, "LIST", optionalString(p.Path))
}
func (h *fullClientHandler) MutateClientWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	h.clientWorkspaceFiles(w, r, "MUTATE", "")
}
func (h *fullClientHandler) DownloadClientWorkspaceFile(w http.ResponseWriter, r *http.Request, p clientapi.DownloadClientWorkspaceFileParams) {
	h.clientWorkspaceFiles(w, r, "GET", optionalString(p.Path))
}
func (h *fullClientHandler) HeadClientWorkspaceFile(w http.ResponseWriter, r *http.Request, p clientapi.HeadClientWorkspaceFileParams) {
	h.clientWorkspaceFiles(w, r, "HEAD", optionalString(p.Path))
}
func (h *fullClientHandler) UploadClientWorkspaceFile(w http.ResponseWriter, r *http.Request, p clientapi.UploadClientWorkspaceFileParams) {
	h.clientWorkspaceFiles(w, r, "PUT", optionalString(p.Path))
}

func serveWorkspaceFiles(w http.ResponseWriter, r *http.Request, service *workspace.Service, actor, userID, action, filePath string, authorize func(context.Context) error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if service == nil {
		writeWorkspaceError(w, workspace.ErrUnavailable)
		return
	}
	var mutation adminapi.WorkspaceFileMutation
	if action == "MUTATE" {
		if decodeStrictJSON(r, &mutation) != nil {
			writeWorkspaceError(w, workspace.ErrInvalid)
			return
		}
		filePath = mutation.Path
	}
	auditAction := action
	if action == "MUTATE" {
		auditAction = string(mutation.Action)
	}
	access, err := service.OpenFiles(r.Context(), actor, userID, auditAction, filePath, authorize)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	outcome := "REJECTED"
	var bytes int64
	defer func() { service.CloseFiles(access, outcome, bytes) }()
	switch action {
	case "LIST":
		out, err := access.Remote.List(access.Context, access.Username, access.Token, filePath)
		if err != nil {
			service.ObserveFileError(context.WithoutCancel(r.Context()), access, err)
			writeWorkspaceError(w, err)
			return
		}
		outcome = "SUCCEEDED"
		writeJSON(w, 200, out)
	case "MUTATE":
		out, err := access.Remote.Mutate(access.Context, access.Username, access.Token, mutation)
		if err != nil {
			var re *remoteapi.Error
			if errors.As(err, &re) && re.Unknown {
				outcome = "UNKNOWN"
			}
			service.ObserveFileError(context.WithoutCancel(r.Context()), access, err)
			writeWorkspaceError(w, err)
			return
		}
		outcome = string(out.Outcome)
		writeJSON(w, 200, out)
	default:
		var body io.Reader
		if action == "PUT" {
			stream := remoteapi.IdleBody(r.Body, access.Idle)
			defer stream.Close()
			body = &workspaceCountingReader{reader: stream, count: &bytes}
		}
		response, err := access.Remote.Content(access.Context, access.Username, access.Token, filePath, action, body, r.Header)
		if err != nil {
			var re *remoteapi.Error
			if errors.As(err, &re) && re.Unknown {
				outcome = "UNKNOWN"
			}
			service.ObserveFileError(context.WithoutCancel(r.Context()), access, err)
			writeWorkspaceError(w, err)
			return
		}
		defer response.Body.Close()
		if action == "PUT" {
			outcome = "SUCCEEDED"
			writeJSON(w, 200, adminapi.WorkspaceFileResult{Outcome: "SUCCEEDED", Failures: []adminapi.WorkspaceFileFailure{}, Truncated: false})
			return
		}
		for _, key := range []string{"ETag", "Last-Modified", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if v := response.Header.Get(key); v != "" {
				w.Header().Set(key, v)
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(filePath)}))
		w.WriteHeader(response.StatusCode)
		stream := remoteapi.IdleBody(response.Body, access.Idle)
		defer stream.Close()
		bytes, err = io.Copy(w, stream)
		if err != nil {
			outcome = "UNKNOWN"
			panic(http.ErrAbortHandler)
		}
		outcome = "SUCCEEDED"
	}
}

type workspaceCountingReader struct {
	reader io.Reader
	count  *int64
}

func (r *workspaceCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	*r.count += int64(n)
	return n, err
}

func (h *fullAdminHandler) StageWorkspaceMCP(w http.ResponseWriter, r *http.Request, id adminapi.WorkspaceServiceId, p adminapi.StageWorkspaceMCPParams) {
	actor, ok := h.workspaceAuth(w, r, p.XCSRFToken, true)
	if !ok {
		return
	}
	var input adminapi.StageWorkspaceMCPRequest
	if decodeStrictJSON(r, &input) != nil {
		writeWorkspaceError(w, workspace.ErrInvalid)
		return
	}
	if err := h.services.Workspace.StageMCP(r.Context(), actor, p.IdempotencyKey, id, input.ExpectedDraftRevision); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
