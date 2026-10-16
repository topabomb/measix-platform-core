package runtime

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestDAVInformationalResponseRecordsFinalStatus(t *testing.T) {
	for _, status := range []int{http.StatusContinue, http.StatusEarlyHints} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			h, token, root, _, _ := davFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusCreated)
			}), "FULL")
			rec := &davRecorder{}
			h.(*Handler).recorder = rec
			req := httptest.NewRequest("PUT", root+"file", strings.NewReader("content"))
			req.Header.Set("Authorization", "Bearer "+token)
			h.ServeHTTP(httptest.NewRecorder(), req)
			if len(rec.events) != 1 {
				t.Fatal("missing event")
			}
			if got := rec.events[0].Request.HttpStatus; got != http.StatusCreated {
				t.Fatalf("final status = %d, want 201", got)
			}
		})
	}
}

func TestResponseObserverRetainsSwitchingProtocols(t *testing.T) {
	w := &responseObserver{ResponseWriter: httptest.NewRecorder()}
	w.WriteHeader(http.StatusSwitchingProtocols)
	if w.status != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d", w.status)
	}
}
