package harness

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HubEnv is an isolated environment that starts a real Control Hub process
// with its own SQLite database, Ed25519 keys, and service credential.
// It owns no product assertions.
type HubEnv struct {
	Root            string
	HubBin          string
	RelayBin        string
	DBPath          string
	MasterKeyFile   string
	JWTKeyFile      string
	RelayTokenFile  string
	HubPort         int
	RelayPubPort    int
	RelayIntPort    int
	HubBaseURL      string
	RelayPubBaseURL string
	RelayIntBaseURL string
	AdminPassword   string
	hubLog          *os.File
	relayLog        *os.File
	hubProc         *Process
	relayProc       *Process
}

// NewHubEnv allocates directories, generates crypto material, applies migrations
// via devmigrate, and bootstraps the initial admin user. It does not start the
// Hub or Relay processes (call StartHub/StartRelay to do that).
func NewHubEnv(ctx context.Context) (*HubEnv, error) {
	root, err := os.MkdirTemp("", "measix-hub-*")
	if err != nil {
		return nil, err
	}
	hubPort, err := freePort()
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	relayPub, err := freePort()
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	relayInt, err := freePort()
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}

	env := &HubEnv{
		Root:            root,
		DBPath:          filepath.Join(root, "hub.db"),
		MasterKeyFile:   filepath.Join(root, "master.key"),
		JWTKeyFile:      filepath.Join(root, "jwt-ed25519.seed"),
		RelayTokenFile:  filepath.Join(root, "relay-service.token"),
		HubPort:         hubPort,
		RelayPubPort:    relayPub,
		RelayIntPort:    relayInt,
		HubBaseURL:      fmt.Sprintf("http://127.0.0.1:%d", hubPort),
		RelayPubBaseURL: fmt.Sprintf("http://127.0.0.1:%d", relayPub),
		RelayIntBaseURL: fmt.Sprintf("http://127.0.0.1:%d", relayInt),
	}

	// Generate crypto material.
	if err := writeRandomFile(env.MasterKeyFile, 32); err != nil {
		env.Cleanup()
		return nil, err
	}
	if err := writeRandomFile(env.JWTKeyFile, 32); err != nil {
		env.Cleanup()
		return nil, err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		env.Cleanup()
		return nil, err
	}
	if err := os.WriteFile(env.RelayTokenFile, []byte(hex.EncodeToString(token)+"\n"), 0o600); err != nil {
		env.Cleanup()
		return nil, err
	}

	// Generate admin password.
	pw := "sys-test-" + hex.EncodeToString(token[:4])
	env.AdminPassword = pw
	pwFile := filepath.Join(root, "admin-password.txt")
	if err := os.WriteFile(pwFile, []byte(pw+"\n"), 0o600); err != nil {
		env.Cleanup()
		return nil, err
	}

	// Locate backend root.
	_, file, _, _ := runtime.Caller(0)
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))

	// Apply migrations via devmigrate.
	migrateCmd := exec.CommandContext(ctx, "go", "run", "./cmd/devmigrate", "--db", env.DBPath)
	migrateCmd.Dir = backendRoot
	if out, err := migrateCmd.CombinedOutput(); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("devmigrate: %v: %s", err, out)
	}

	// Bootstrap admin.
	bootstrapCmd := exec.CommandContext(ctx, "go", "run", "./cmd/control-hub", "bootstrap-admin",
		"--db", env.DBPath,
		"--master-key-file", env.MasterKeyFile,
		"--jwt-private-key-file", env.JWTKeyFile,
		"--deployment-name", "SYS-TEST",
		"--username", "admin",
		"--display-name", "System Test Admin",
		"--password-file", pwFile,
	)
	bootstrapCmd.Dir = backendRoot
	if out, err := bootstrapCmd.CombinedOutput(); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("bootstrap-admin: %v: %s", err, out)
	}

	// Build binaries.
	env.HubBin = filepath.Join(root, "control-hub")
	env.RelayBin = filepath.Join(root, "runtime-relay")
	if runtime.GOOS == "windows" {
		env.HubBin += ".exe"
		env.RelayBin += ".exe"
	}
	if err := buildBinary(ctx, backendRoot, env.HubBin, "./cmd/control-hub"); err != nil {
		env.Cleanup()
		return nil, err
	}
	if err := buildBinary(ctx, backendRoot, env.RelayBin, "./cmd/runtime-relay"); err != nil {
		env.Cleanup()
		return nil, err
	}

	return env, nil
}

// StartHub launches the control-hub process.
func (e *HubEnv) StartHub(ctx context.Context) error {
	logFile, err := os.Create(filepath.Join(e.Root, "hub.log"))
	if err != nil {
		return err
	}
	e.hubLog = logFile
	proc, err := e.startProcess(ctx, logFile, e.HubBin,
		"run",
		"--listen", fmt.Sprintf("127.0.0.1:%d", e.HubPort),
		"--public-base-url", e.HubBaseURL,
		"--runtime-api-base", e.RelayPubBaseURL,
		"--db", e.DBPath,
		"--master-key-file", e.MasterKeyFile,
		"--jwt-private-key-file", e.JWTKeyFile,
		"--relay-internal-url", e.RelayIntBaseURL,
		"--relay-service-token-file", e.RelayTokenFile,
		"--reconcile-interval", "2s",
	)
	if err != nil {
		return err
	}
	e.hubProc = proc
	if err := WaitLive(ctx, e.HubBaseURL, 30*time.Second); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(e.Root, "hub.log"))
		return fmt.Errorf("hub not live: %w\nhub log:\n%s", err, logContent)
	}
	if err := WaitReady(ctx, e.HubBaseURL, 30*time.Second); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(e.Root, "hub.log"))
		return fmt.Errorf("hub not ready: %w\nhub log:\n%s", err, logContent)
	}
	return nil
}

// StartRelay launches the runtime-relay process.
func (e *HubEnv) StartRelay(ctx context.Context) error {
	logFile, err := os.Create(filepath.Join(e.Root, "relay.log"))
	if err != nil {
		return err
	}
	e.relayLog = logFile
	spoolPath := filepath.Join(e.Root, "relay-spool.db")
	proc, err := e.startProcess(ctx, logFile, e.RelayBin,
		"--public-listen", fmt.Sprintf("127.0.0.1:%d", e.RelayPubPort),
		"--internal-listen", fmt.Sprintf("127.0.0.1:%d", e.RelayIntPort),
		"--spool", spoolPath,
		"--hub-usage-url", fmt.Sprintf("%s/internal/v1/usage/request-events:batch", e.HubBaseURL),
		"--hub-service-token-file", e.RelayTokenFile,
	)
	if err != nil {
		return err
	}
	e.relayProc = proc
	if err := WaitLive(ctx, e.RelayIntBaseURL, 30*time.Second); err != nil {
		_ = proc.Cmd.Process.Kill()
		logContent, _ := os.ReadFile(filepath.Join(e.Root, "relay.log"))
		return fmt.Errorf("relay not live: %w\nrelay log:\n%s", err, logContent)
	}
	return nil
}

// StopHub kills the hub process if running.
func (e *HubEnv) StopHub() {
	if e.hubProc != nil {
		stopProcess(e.hubProc)
		e.hubProc = nil
	}
}

// StopRelay kills the relay process if running.
func (e *HubEnv) StopRelay() {
	if e.relayProc != nil {
		stopProcess(e.relayProc)
		e.relayProc = nil
	}
}

// RestartHub kills and restarts the hub process.
func (e *HubEnv) RestartHub(ctx context.Context) error {
	e.StopHub()
	return e.StartHub(ctx)
}

// RestartRelay kills and restarts the relay process.
func (e *HubEnv) RestartRelay(ctx context.Context) error {
	e.StopRelay()
	return e.StartRelay(ctx)
}

// Cleanup removes the temp root and terminates any running processes.
func (e *HubEnv) Cleanup() {
	e.StopHub()
	e.StopRelay()
	if e.hubLog != nil {
		_ = e.hubLog.Close()
	}
	if e.relayLog != nil {
		_ = e.relayLog.Close()
	}
	_ = os.RemoveAll(e.Root)
}

// HubLog returns the hub log content for diagnostics.
func (e *HubEnv) HubLog() string {
	data, _ := os.ReadFile(filepath.Join(e.Root, "hub.log"))
	return string(data)
}

// RelayLog returns the relay log content for diagnostics.
func (e *HubEnv) RelayLog() string {
	data, _ := os.ReadFile(filepath.Join(e.Root, "relay.log"))
	return string(data)
}

func (e *HubEnv) startProcess(ctx context.Context, log io.Writer, name string, args ...string) (*Process, error) {
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

// WaitReadyRelay polls the relay /ready endpoint. Relay becomes ready only
// after a control state is applied (fail-closed).
func WaitReadyRelay(ctx context.Context, baseURL string, timeout time.Duration) error {
	return WaitReady(ctx, baseURL, timeout)
}

// PollURL polls a URL until it returns 2xx or the timeout elapses.
func PollURL(ctx context.Context, url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return nil
			}
			last = fmt.Sprintf("status=%d", resp.StatusCode)
		} else {
			last = err.Error()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout polling %s: %s", url, last)
}

func buildBinary(ctx context.Context, dir, target, pkg string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", target, pkg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s: %v: %s", pkg, err, out)
	}
	return nil
}

func writeRandomFile(path string, n int) error {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ComputeEd25519KeyID derives a short hex key ID from a public key.
func ComputeEd25519KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "hub-ed25519-" + hex.EncodeToString(sum[:6])
}

// WaitConvergence polls the system status endpoint until the managed runtime
// has converged (desired == applied control revision) or the timeout elapses.
func WaitConvergence(ctx context.Context, hubBaseURL, csrfToken, sessionCookie string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		status, err := doSystemStatus(ctx, hubBaseURL, csrfToken, sessionCookie)
		if err != nil {
			last = err.Error()
		} else {
			if status.Converged {
				return nil
			}
			last = fmt.Sprintf("not converged: desired=%d applied=%d", status.DesiredControlRevision, status.AppliedControlRevision)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for convergence: %s", last)
}

// SystemStatusSummary is a lightweight summary of system status for convergence checks.
type SystemStatusSummary struct {
	Converged              bool
	DesiredControlRevision int
	AppliedControlRevision int
	RuntimeStatus          string
}

func doSystemStatus(ctx context.Context, baseURL, csrfToken, sessionCookie string) (*SystemStatusSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/admin/v1/system/status", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", sessionCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status=%d", resp.StatusCode)
	}
	var raw struct {
		RuntimeStatus  string `json:"runtimeStatus"`
		ControlDesired int    `json:"desiredControlRevision"`
		ControlApplied int    `json:"appliedControlRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return &SystemStatusSummary{
		Converged:              raw.ControlDesired == raw.ControlApplied && raw.ControlApplied > 0,
		DesiredControlRevision: raw.ControlDesired,
		AppliedControlRevision: raw.ControlApplied,
		RuntimeStatus:          raw.RuntimeStatus,
	}, nil
}

// stopProcess kills a process and waits for it to exit (with a bounded timeout).
func stopProcess(proc *Process) {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return
	}
	_ = proc.Cmd.Process.Kill()
	select {
	case <-proc.Done:
	case <-time.After(3 * time.Second):
	}
}

// StripPort returns the host portion of a host:port string.
func StripPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}
