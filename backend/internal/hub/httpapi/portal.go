package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/wire/clientapi"
)

const portalCookie = "measix_portal_session"

func (h *fullClientHandler) CreatePortalGrant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token, ok := bearerToken(r)
	if !ok {
		writeProblem(w, 401, "unauthorized", "Unauthorized")
		return
	}
	grant, err := h.identity.CreatePortalGrant(r.Context(), token)
	if errors.Is(err, identity.ErrPortalUnavailable) {
		writeProblem(w, 503, "portal_unavailable", "Portal is not configured")
		return
	}
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, 201, grant)
}

func (h *fullClientHandler) portalOriginAllowed(r *http.Request, required bool) bool {
	origin := r.Header.Get("Origin")
	return h.identity.PublicOrigin != "" && r.Header.Get("Sec-Fetch-Site") != "cross-site" && ((!required && origin == "") || origin == h.identity.PublicOrigin)
}

// WebView.postUrl is a native navigation with no browser document initiator and therefore
// serializes its opaque origin as "null". The one-time ticket authenticates this endpoint;
// the opaque form is accepted only for a non-cross-site native navigation, never as the
// origin policy for the subsequent cookie-authenticated Portal APIs.
func (h *fullClientHandler) portalExchangeOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return h.identity.PublicOrigin != "" && r.Header.Get("Sec-Fetch-Site") != "cross-site" &&
		(origin == "" || origin == "null" || origin == h.identity.PublicOrigin)
}

func (h *fullClientHandler) setPortalCookie(w http.ResponseWriter, value string, expiry time.Time) {
	maxAge := 0
	if value == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: portalCookie, Value: value, Path: "/", Expires: expiry, MaxAge: maxAge, Secure: strings.HasPrefix(h.identity.PublicOrigin, "https://"), HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (h *fullClientHandler) ExchangePortalGrant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !h.portalExchangeOriginAllowed(r) {
		writeProblem(w, 403, "forbidden", "Origin rejected")
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if r.URL.RawQuery != "" || mediaErr != nil || mediaType != "application/x-www-form-urlencoded" {
		writeProblem(w, 400, "invalid_request", "Invalid native form")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil || len(r.PostForm) != 1 || len(r.PostForm["ticket"]) != 1 {
		writeProblem(w, 400, "invalid_request", "Invalid native form")
		return
	}
	cookie, expiry, err := h.identity.ExchangePortalGrant(r.Context(), r.PostForm.Get("ticket"))
	if err != nil {
		writeProblem(w, 401, "unauthorized", "Invalid or expired grant")
		return
	}
	h.setPortalCookie(w, cookie, expiry)
	w.Header().Set("Location", "/portal/")
	w.WriteHeader(http.StatusSeeOther)
}

func (h *fullClientHandler) authenticatePortal(w http.ResponseWriter, r *http.Request) (clientapi.PortalSession, bool) {
	if _, err := r.Cookie(portalCookie); err != nil || h.identity.PublicOrigin == "" {
		writeProblem(w, 401, "unauthorized", "Unauthorized")
		return clientapi.PortalSession{}, false
	}
	if !h.portalOriginAllowed(r, false) {
		writeProblem(w, 403, "forbidden", "Origin rejected")
		return clientapi.PortalSession{}, false
	}
	cookie, err := r.Cookie(portalCookie)
	if err != nil {
		writeProblem(w, 401, "unauthorized", "Unauthorized")
		return clientapi.PortalSession{}, false
	}
	session, err := h.identity.AuthenticatePortal(r.Context(), cookie.Value)
	if err != nil {
		h.setPortalCookie(w, "", time.Time{})
		writeProblem(w, 401, "unauthorized", "Portal session expired or revoked")
		return clientapi.PortalSession{}, false
	}
	return session, true
}

func (h *fullClientHandler) GetPortalSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if session, ok := h.authenticatePortal(w, r); ok {
		writeJSON(w, 200, session)
	}
}

func (h *fullClientHandler) ClosePortalSession(w http.ResponseWriter, r *http.Request, params clientapi.ClosePortalSessionParams) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.portalOriginAllowed(r, true) {
		writeProblem(w, 403, "forbidden", "Origin rejected")
		return
	}
	if _, ok := h.authenticatePortal(w, r); !ok {
		return
	}
	cookie, _ := r.Cookie(portalCookie)
	if err := h.identity.ClosePortal(r.Context(), cookie.Value, params.XCSRFToken); err != nil {
		if errors.Is(err, identity.ErrNotAuthorized) {
			writeProblem(w, 403, "forbidden", "CSRF rejected")
			return
		}
		writeIdentityError(w, err)
		return
	}
	h.setPortalCookie(w, "", time.Time{})
	w.WriteHeader(204)
}

func (h *fullClientHandler) authenticateFeed(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		token, ok := bearerToken(r)
		if !ok {
			writeProblem(w, 401, "unauthorized", "Unauthorized")
			return false
		}
		if _, err := h.identity.AuthenticateAccess(r.Context(), token); err != nil {
			writeIdentityError(w, err)
			return false
		}
		return true
	}
	_, ok := h.authenticatePortal(w, r)
	return ok
}
