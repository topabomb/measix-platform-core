package runtime

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Upgraded traffic remains opaque. Counters exclude HTTP handshake headers;
// the request limit bounds total client-to-upstream wire bytes per connection.
type upgradedStream struct {
	io.ReadWriteCloser
	requestBytes, responseBytes atomic.Int64
	timedOut, exceeded          atomic.Bool
	limit                       int64
	idle                        time.Duration
	mu                          sync.Mutex
	timer                       *time.Timer
	closed                      bool
	lastActivity                time.Time
}

func newUpgradedStream(conn io.ReadWriteCloser, idle time.Duration, limit int64) *upgradedStream {
	s := &upgradedStream{ReadWriteCloser: conn, idle: idle, limit: limit, lastActivity: time.Now()}
	s.mu.Lock()
	s.timer = time.AfterFunc(idle, s.checkIdle)
	s.mu.Unlock()
	return s
}

func (s *upgradedStream) checkIdle() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	remaining := s.idle - time.Since(s.lastActivity)
	if remaining > 0 {
		s.timer.Reset(remaining)
		s.mu.Unlock()
		return
	}
	s.timedOut.Store(true)
	s.mu.Unlock()
	_ = s.Close()
}

func (s *upgradedStream) activity() {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

func (s *upgradedStream) Read(p []byte) (int, error) {
	n, err := s.ReadWriteCloser.Read(p)
	if n > 0 {
		s.responseBytes.Add(int64(n))
		s.activity()
	}
	return n, err
}

func (s *upgradedStream) Write(p []byte) (int, error) {
	if int64(len(p)) > s.limit-s.requestBytes.Load() {
		s.exceeded.Store(true)
		_ = s.Close()
		return 0, errors.New("WebSocket request byte limit exceeded")
	}
	n, err := s.ReadWriteCloser.Write(p)
	if n > 0 {
		s.requestBytes.Add(int64(n))
		s.activity()
	}
	return n, err
}

func (s *upgradedStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.timer.Stop()
	s.mu.Unlock()
	return s.ReadWriteCloser.Close()
}
