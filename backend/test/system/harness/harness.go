// Package harness owns environment/process/readiness/polling/cleanup for the
// S0.1 real-process system scenarios. It contains no product assertions.
package harness

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Env is a single isolated system-run environment with unique ports and a temp root.
type Env struct {
	Root  string
	Ports Ports
}

// Ports holds allocated loopback ports for a run.
type Ports struct {
	HubPublic int
	RelayPub  int
	RelayInt  int
}

// New allocates a temp root and unique loopback ports.
func New() (*Env, error) {
	root, err := os.MkdirTemp("", "measix-system-*")
	if err != nil {
		return nil, err
	}
	hub, err := freePort()
	if err != nil {
		return nil, err
	}
	relayPub, err := freePort()
	if err != nil {
		return nil, err
	}
	relayInt, err := freePort()
	if err != nil {
		return nil, err
	}
	return &Env{Root: root, Ports: Ports{HubPublic: hub, RelayPub: relayPub, RelayInt: relayInt}}, nil
}

// Cleanup removes the temp root and terminates any started process group.
func (e *Env) Cleanup() {
	_ = os.RemoveAll(e.Root)
}

// Process is a running managed child process.
type Process struct {
	Cmd  *exec.Cmd
	Done chan error
	Log  io.Writer
}

// Start launches a command, streaming stdout/stderr to log.
func (e *Env) Start(ctx context.Context, log io.Writer, name string, args ...string) (*Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = e.Root
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return &Process{Cmd: cmd, Done: done, Log: log}, nil
}

// WaitLive polls an HTTP /live endpoint until it returns 200 or the timeout elapses.
func WaitLive(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(baseURL + "/live")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Sprintf("status=%d", resp.StatusCode)
		} else {
			last = err.Error()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s live: %s", baseURL, last)
}

// WaitReady polls an HTTP /ready endpoint until it returns 200 or the timeout elapses.
func WaitReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(baseURL + "/ready")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Sprintf("status=%d", resp.StatusCode)
		} else {
			last = err.Error()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s: %s", baseURL, last)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return strconv.Atoi(strings.Split(l.Addr().String(), ":")[1])
}
