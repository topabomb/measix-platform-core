package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
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
	if err := ctx.Err(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", s.Server.Addr)
	if err != nil {
		return err
	}
	originalBase := s.Server.BaseContext
	base := context.Background()
	if originalBase != nil {
		base = originalBase(listener)
	}
	requestCtx, cancelRequests := context.WithCancel(base)
	defer cancelRequests()
	s.Server.BaseContext = func(net.Listener) context.Context { return requestCtx }
	handler := s.Server.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	var handlers sync.WaitGroup
	var admission sync.Mutex
	accepting := true
	s.Server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admission.Lock()
		if !accepting {
			admission.Unlock()
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		}
		handlers.Add(1)
		admission.Unlock()
		defer handlers.Done()
		handler.ServeHTTP(w, r)
	})
	errc := make(chan error, 1)
	go func() { errc <- s.Server.Serve(listener) }()
	select {
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.ShutdownGrace)
		err = s.Server.Shutdown(shutdownCtx)
		cancel()
	}
	// Shutdown on deadline does not close connections or cancel handlers by itself.
	// Force cancellation, then allow terminal metering/cleanup to finish before
	// callers close the durable store/spool. No new request is admitted after Close.
	cancelRequests()
	closeErr := s.Server.Close()
	admission.Lock()
	accepting = false
	admission.Unlock()
	joined := make(chan struct{})
	go func() { handlers.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		log.Error("HTTP handlers exceeded cleanup deadline", "event", "http_shutdown_timeout")
		return errors.Join(err, closeErr, context.DeadlineExceeded)
	}
	return errors.Join(err, closeErr)
}
