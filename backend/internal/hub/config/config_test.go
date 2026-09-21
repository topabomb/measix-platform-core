package config_test

import (
	"testing"

	"measix/platform/internal/hub/config"
)

func TestLoadCanonicalizesPublicOrigin(t *testing.T) {
	cfg, err := config.Load([]string{
		"--db", "hub.db",
		"--master-key-file", "master.key",
		"--jwt-private-key-file", "jwt.key",
		"--relay-internal-url", "http://127.0.0.1:8091",
		"--relay-service-token-file", "relay.token",
		"--diagnostics-log-dir", "logs",
		"--public-origin", " HTTPS://Core.Example.COM:443/ ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicOrigin != "https://core.example.com" {
		t.Fatalf("public origin = %q", cfg.PublicOrigin)
	}
	if cfg.DiagnosticsLogDir != "logs" {
		t.Fatalf("diagnostics log dir = %q", cfg.DiagnosticsLogDir)
	}
}
