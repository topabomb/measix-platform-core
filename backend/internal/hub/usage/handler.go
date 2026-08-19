package usage

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/usageingestapi"
)

const maxUsageBatchBody = 2 << 20

type Handler struct {
	usageingestapi.Unimplemented
	service      *Service
	serviceToken string
}

func NewHandler(service *Service, serviceToken string) *Handler {
	return &Handler{service: service, serviceToken: serviceToken}
}

func (h *Handler) IngestRequestUsageBatch(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || h.serviceToken == "" {
		writeProblem(w, http.StatusServiceUnavailable, "service_unavailable", "Usage ingest unavailable")
		return
	}
	if !validServiceBearer(r.Header.Get("Authorization"), h.serviceToken) {
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid request")
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUsageBatchBody))
	decoder.DisallowUnknownFields()
	var batch usageingestapi.UsageBatch
	if err := decoder.Decode(&batch); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid request")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid request")
		return
	}
	ack, err := h.service.Ingest(r.Context(), batch)
	if err != nil {
		if errors.Is(err, ErrInvalidBatch) {
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_request", "Invalid usage batch")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ack)
}

func validServiceBearer(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(usageingestapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code})
}
