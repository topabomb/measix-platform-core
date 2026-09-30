package server_test

import (
	"context"
	"io"
	"log/slog"
	"measix/platform/internal/common/server"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestGraceExpiryCancelsAndJoinsActiveHandler(t *testing.T) {
	entered, exited := make(chan struct{}), make(chan struct{})
	s := server.NewWithGrace("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(exited) }), 30*time.Millisecond)
	address := make(chan string, 1)
	s.Server.BaseContext = func(l net.Listener) context.Context { address <- l.Addr().String(); return context.Background() }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer s.Server.Close()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	var addr string
	select {
	case addr = <-address:
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not start")
	}
	go func() {
		response, err := http.Get("http://" + addr)
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("grace expiry did not cancel active request")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server did not finish shutdown")
	}
}
