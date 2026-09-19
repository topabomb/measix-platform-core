package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
)

const adminSessionCookie = "measix_admin_session"

type adminHandler struct {
	adminapi.Unimplemented
	identity *identity.Service
}

type clientHandler struct {
	clientapi.Unimplemented
	identity *identity.Service
}

func New(identityService *identity.Service) http.Handler {
	router := chi.NewRouter()
	adminapi.HandlerFromMux(&adminHandler{identity: identityService}, router)
	clientapi.HandlerFromMux(&clientHandler{identity: identityService}, router)
	return router
}

func (h *adminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var request adminapi.LoginRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	result, err := h.identity.LoginAdmin(r.Context(), request.Username, request.Password)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    result.CookieSecret,
		Path:     "/",
		Expires:  result.ExpiresAt,
		Secure:   strings.HasPrefix(h.identity.PublicOrigin, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, adminapi.AdminSession{
		CsrfToken: result.CSRFToken,
		ExpiresAt: result.ExpiresAt,
		User: adminapi.AdminUserSummary{
			UserId:      result.UserID,
			DisplayName: result.DisplayName,
			Role:        adminapi.AdminUserSummaryRole(result.Role),
		},
	})
}

func (h *adminHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	admin, err := h.authenticateAdmin(r, "", false)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	cookie, _ := r.Cookie(adminSessionCookie)
	writeJSON(w, http.StatusOK, adminapi.AdminSession{
		CsrfToken: security.CSRFToken(cookie.Value, h.identity.CSRFKey),
		ExpiresAt: admin.ExpiresAt,
		User: adminapi.AdminUserSummary{
			UserId:      admin.UserID,
			DisplayName: admin.DisplayName,
			Role:        adminapi.AdminUserSummaryRole(admin.Role),
		},
	})
}

func (h *adminHandler) LogoutAdmin(w http.ResponseWriter, r *http.Request, params adminapi.LogoutAdminParams) {
	_, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	cookie, _ := r.Cookie(adminSessionCookie)
	if err := h.identity.LogoutAdmin(r.Context(), cookie.Value); err != nil {
		writeIdentityError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: adminSessionCookie, Value: "", Path: "/", MaxAge: -1, Secure: strings.HasPrefix(h.identity.PublicOrigin, "https://"), HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) ListUsers(w http.ResponseWriter, r *http.Request, params adminapi.ListUsersParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, after, valid := pageParams(w, r, params.Limit, params.Cursor)
	if !valid {
		return
	}
	search := ""
	if params.Query != nil {
		search = *params.Query
	}
	users, err := h.identity.ListUserViews(r.Context(), search, limit+1, after)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	users, next := pageResult(r, users, limit, func(v identity.UserView) string { return v.ID })
	items := make([]adminapi.User, 0, len(users))
	for _, u := range users {
		items = append(items, userWire(u))
	}
	writeJSON(w, http.StatusOK, adminapi.UserPage{Items: items, NextCursor: next})
}

func (h *adminHandler) CreateUser(w http.ResponseWriter, r *http.Request, params adminapi.CreateUserParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.CreateUserRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	u, err := h.identity.CreateUserView(r.Context(), request.Username, request.DisplayName, string(request.Role))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userWire(u))
}

func (h *adminHandler) GetUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	u, err := h.identity.GetUserView(r.Context(), userID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userWire(u))
}

func (h *adminHandler) UpdateUser(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.UpdateUserParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.UpdateUserRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	u, err := h.identity.UpdateUserView(r.Context(), userID, request.Username, request.DisplayName, string(request.Role))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userWire(u))
}

func (h *adminHandler) SetPassword(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.SetPasswordParams) {
	if _, err := h.authenticateAdmin(r, params.XCSRFToken, true); err != nil {
		writeIdentityError(w, err)
		return
	}
	var request adminapi.SetPasswordRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if err := h.identity.SetPassword(r.Context(), userID, request.NewPassword); err != nil {
		writeIdentityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) CreateEnrollment(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.CreateEnrollmentParams) {
	admin, err := h.authenticateAdmin(r, params.XCSRFToken, true)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	if identity.ValidatePublicOrigin(h.identity.PublicOrigin) != nil {
		writeProblem(w, http.StatusServiceUnavailable, "platform_origin_unconfigured", "Configure the platform public origin before issuing enrollment material")
		return
	}
	var request adminapi.CreateEnrollmentRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	ttl := time.Duration(0)
	if request.ExpiresInSeconds != nil {
		ttl = time.Duration(*request.ExpiresInSeconds) * time.Second
	}
	grant, err := h.identity.CreateEnrollment(r.Context(), userID, admin.UserID, ttl)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminapi.CreateEnrollmentResponse{EnrollmentId: grant.EnrollmentID, Code: grant.Code, ExpiresAt: grant.ExpiresAt, PlatformUrl: h.identity.PublicOrigin})
}

func (h *adminHandler) ListDevices(w http.ResponseWriter, r *http.Request, userID adminapi.UserId, params adminapi.ListDevicesParams) {
	if _, err := h.authenticateAdmin(r, "", false); err != nil {
		writeIdentityError(w, err)
		return
	}
	limit, after, valid := pageParams(w, r, params.Limit, params.Cursor)
	if !valid {
		return
	}
	devices, err := h.identity.ListDeviceViews(r.Context(), userID, limit+1, after)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	devices, next := pageResult(r, devices, limit, func(v identity.DeviceView) string { return v.ID })
	items := make([]adminapi.Device, 0, len(devices))
	for _, d := range devices {
		items = append(items, adminapi.Device{
			DeviceName:               d.Name,
			ApplicationState:         adminapi.DeviceApplicationState(d.ApplicationState),
			TargetManagedGeneration:  d.TargetManagedGeneration,
			AppliedManagedGeneration: d.AppliedManagedGeneration,
			AppliedReportedAt:        d.AppliedReportedAt,
			DeviceId:                 d.ID,
			UserId:                   d.UserID,
			InstallationId:           d.InstallationID,
			AppVersion:               d.AppVersion,
			LastSeenAt:               d.LastSeenAt,
			Status:                   adminapi.DeviceStatus(d.Status),
		})
	}
	writeJSON(w, http.StatusOK, adminapi.DevicePage{Items: items, NextCursor: next})
}

func (h *adminHandler) authenticateAdmin(r *http.Request, csrf string, requireCSRF bool) (identity.AdminPrincipalView, error) {
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil {
		return identity.AdminPrincipalView{}, identity.ErrNotAuthorized
	}
	return h.identity.AuthenticateAdminView(r.Context(), cookie.Value, csrf, requireCSRF)
}

func (h *clientHandler) Discover(w http.ResponseWriter, r *http.Request) {
	view, err := h.identity.Discovery(r.Context())
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clientapi.Discovery{
		Product:                         clientapi.MEASIXAGENTPLATFORM,
		ProtocolVersion:                 clientapi.DiscoveryProtocolVersionN1,
		DeploymentId:                    view.DeploymentID,
		DeploymentName:                  view.DeploymentName,
		ClientApiBase:                   "/api/client/v1",
		RuntimeApiBase:                  "/runtime/v1",
		SupportedSnapshotSchemaVersions: []int{capability.CurrentSnapshotSchemaVersion},
	})
}

func (h *clientHandler) ExchangeEnrollment(w http.ResponseWriter, r *http.Request) {
	var request clientapi.EnrollmentExchangeRequest
	if err := decodeStrictJSON(r, &request); err != nil || request.Platform != clientapi.ANDROID {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	result, err := h.identity.ExchangeEnrollment(r.Context(), request.Code, request.InstallationId, request.DeviceName, request.AppVersion)
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, clientapi.EnrollmentExchangeResponse{
		DeploymentId:         result.DeploymentID,
		UserId:               result.UserID,
		DeviceId:             result.DeviceID,
		SessionId:            result.SessionID,
		AccessToken:          result.AccessToken,
		AccessTokenExpiresAt: result.AccessTokenExpiresAt,
		RefreshToken:         result.RefreshToken,
		RefreshExpiresAt:     result.RefreshExpiresAt,
		SessionIdleExpiresAt: result.RefreshExpiresAt,
	})
}

func writeEnrollmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrAlreadyUsed):
		writeProblem(w, http.StatusConflict, "enrollment_already_used", "Enrollment code already used")
	case errors.Is(err, identity.ErrConflict):
		writeProblem(w, http.StatusConflict, "installation_user_conflict", "Installation is already bound to another user")
	default:
		writeIdentityError(w, err)
	}
}

func (h *clientHandler) RefreshSession(w http.ResponseWriter, r *http.Request, params clientapi.RefreshSessionParams) {
	var request clientapi.RefreshRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	result, err := h.identity.Refresh(r.Context(), request.RefreshToken, params.IdempotencyKey)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clientapi.RefreshResponse{AccessToken: result.AccessToken, AccessTokenExpiresAt: result.AccessTokenExpiresAt, RefreshToken: result.RefreshToken, RefreshExpiresAt: result.RefreshExpiresAt, SessionIdleExpiresAt: result.SessionIdleExpiresAt})
}

func (h *clientHandler) LogoutSession(w http.ResponseWriter, r *http.Request) {
	var request clientapi.RefreshRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if err := h.identity.Logout(r.Context(), request.RefreshToken); err != nil {
		writeIdentityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *clientHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	view, err := h.identity.BootstrapView(r.Context(), token)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	var response clientapi.Bootstrap
	response.Deployment.DeploymentId = view.Principal.DeploymentID
	response.Deployment.Name = view.DeploymentName
	response.User.UserId = view.Principal.UserID
	response.User.DisplayName = view.UserDisplayName
	response.Device.DeviceId = view.Principal.DeviceID
	response.Device.Status = clientapi.BootstrapDeviceStatus(view.DeviceStatus)
	response.Session.SessionId = view.Principal.SessionID
	response.Session.ExpiresAt = view.SessionExpiresAt
	response.Session.SessionIdleExpiresAt = view.SessionExpiresAt
	response.ManagedState = managedStateWire(view.ManagedState, nil)
	response.SupportedSnapshotSchemaVersions = []int{capability.CurrentSnapshotSchemaVersion}
	writeJSON(w, http.StatusOK, response)
}

func (h *clientHandler) GetManagedState(w http.ResponseWriter, r *http.Request, params clientapi.GetManagedStateParams) {
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	if _, err := h.identity.AuthenticateAccess(r.Context(), token); err != nil {
		writeIdentityError(w, err)
		return
	}
	state, err := h.identity.ManagedState(r.Context())
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, managedStateWire(state, params.XMeasixAppliedManagedGeneration))
}

func managedStateWire(state identity.ManagedStateView, applied *int) clientapi.ManagedState {
	syncRequired := applied == nil || *applied != state.ActiveManagedGeneration
	blocked := state.ActiveManagedGeneration == 0 || state.RuntimeStatus != "READY" || syncRequired
	var target *int
	if syncRequired && state.ActiveManagedGeneration > 0 {
		value := state.ActiveManagedGeneration
		target = &value
	}
	return clientapi.ManagedState{
		ActiveManagedGeneration: state.ActiveManagedGeneration,
		ManagedStateRevision:    state.ManagedStateRevision,
		RuntimeStatus:           clientapi.ManagedStateRuntimeStatus(state.RuntimeStatus),
		RuntimeBlocked:          blocked,
		SyncRequired:            syncRequired,
		TargetManagedGeneration: target,
	}
}

func userWire(u identity.UserView) adminapi.User {
	return adminapi.User{
		UserId:      u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        adminapi.UserRole(u.Role),
		Status:      adminapi.UserStatus(u.Status),
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return token, token != ""
}

func decodeStrictJSON(r *http.Request, target any) error {
	payload, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(payload) > 1<<20 {
		return errors.New("request body too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain exactly one JSON value")
	}
	// Generated bool fields cannot distinguish an omitted/null value from false.
	// All current draft writes require explicit values for the five permissions.
	if _, isDraftWrite := target.(*adminapi.PutDraftRequest); isDraftWrite {
		var envelope map[string]json.RawMessage
		var content map[string]json.RawMessage
		var policy map[string]json.RawMessage
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return err
		}
		if err := json.Unmarshal(envelope["content"], &content); err != nil {
			return err
		}
		if err := json.Unmarshal(content["policy"], &policy); err != nil {
			return err
		}
		for _, name := range []string{"providers", "models", "tts", "asr", "mcp", "bindings", "assistants", "starters"} {
			value, present := content[name]
			if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("draft content requires an explicit array for " + name)
			}
			var items []json.RawMessage
			if err := json.Unmarshal(value, &items); err != nil {
				return errors.New("draft content requires an array for " + name)
			}
		}
		for _, name := range []string{"allowLocalProviders", "allowLocalTts", "allowLocalAsr", "allowLocalMcp", "allowLocalAssistants"} {
			value := string(bytes.TrimSpace(policy[name]))
			if value != "true" && value != "false" {
				return errors.New("draft policy requires an explicit Boolean for " + name)
			}
		}
	}
	return nil
}

func writeIdentityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrExpired):
		writeProblem(w, http.StatusUnauthorized, "session_expired", "Session expired")
	case errors.Is(err, identity.ErrRevoked):
		writeProblem(w, http.StatusForbidden, "session_revoked", "Session revoked")
	case errors.Is(err, identity.ErrRefreshConflict):
		writeProblem(w, http.StatusConflict, "refresh_conflict", "Refresh conflict")
	case errors.Is(err, identity.ErrCredential):
		writeProblem(w, http.StatusUnauthorized, "invalid_credential", "Invalid credential")
	case errors.Is(err, identity.ErrNotAuthorized):
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
	case errors.Is(err, identity.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, identity.ErrConflict), errors.Is(err, identity.ErrAlreadyUsed):
		writeProblem(w, http.StatusConflict, "conflict", "Conflict")
	case errors.Is(err, identity.ErrInvalidInput):
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
	default:
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"code":   code,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
