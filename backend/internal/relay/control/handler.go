package control

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
)

type handler struct {
	relaycontrolapi.Unimplemented
	store        *Store
	serviceToken string
}

func NewHandler(store *Store, serviceToken string) http.Handler {
	router := chi.NewRouter()
	relaycontrolapi.HandlerFromMux(&handler{store: store, serviceToken: serviceToken}, router)
	return router
}

func (h *handler) ApplyControlState(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return
	}
	var input relaycontrolapi.RuntimeControlState
	if err := decodeStrictJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	ack, err := h.store.Apply(input)
	if err != nil {
		switch {
		case errors.Is(err, ErrStaleRevision):
			writeProblem(w, http.StatusConflict, "stale_control_revision", "Stale control revision")
		case errors.Is(err, ErrRevisionHashConflict):
			writeProblem(w, http.StatusConflict, "control_revision_hash_conflict", "Control revision hash conflict")
		default:
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_runtime_control", "Invalid runtime control")
		}
		return
	}
	writeJSON(w, http.StatusOK, ack)
}

func (h *handler) GetControlStatus(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, h.store.Status())
}

func (h *handler) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) || h.serviceToken == "" {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return len(provided) == len(h.serviceToken) && subtle.ConstantTimeCompare([]byte(provided), []byte(h.serviceToken)) == 1
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(relaycontrolapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code})
}
