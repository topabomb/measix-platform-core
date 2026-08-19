package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type HTTP struct{ Server *http.Server }

func New(addr string, handler http.Handler) *HTTP {
	return &HTTP{Server: &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}}
}
func (s *HTTP) Run(ctx context.Context, log *slog.Logger) error {
	errc := make(chan error, 1)
	go func() {
		err := s.Server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.Server.Shutdown(shutdownCtx)
	}
}
