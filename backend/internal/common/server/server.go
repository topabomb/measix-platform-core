package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type HTTP struct {
	Server        *http.Server
	ShutdownGrace time.Duration
}

func New(addr string, handler http.Handler) *HTTP {
	return NewWithGrace(addr, handler, 30*time.Second)
}

func NewWithGrace(addr string, handler http.Handler, shutdownGrace time.Duration) *HTTP {
	if shutdownGrace <= 0 {
		shutdownGrace = 30 * time.Second
	}
	return &HTTP{
		Server:        &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second},
		ShutdownGrace: shutdownGrace,
	}
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.ShutdownGrace)
		defer cancel()
		return s.Server.Shutdown(shutdownCtx)
	}
}
