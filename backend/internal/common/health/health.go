package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

type State struct{ ready atomic.Bool }

func (s *State) SetReady(v bool) { s.ready.Store(v) }
func (s *State) Live(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]bool{"live": true})
}
func (s *State) Ready(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		write(w, http.StatusServiceUnavailable, map[string]bool{"ready": false})
		return
	}
	write(w, http.StatusOK, map[string]bool{"ready": true})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
