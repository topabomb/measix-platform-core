package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedManifestCarriesPlatformContract(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "api", "client", "client-control.openapi.yaml")
	output := filepath.Join(root, "output", "client-control.openapi.yaml")
	manifestPath := filepath.Join(root, "output", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("openapi: 3.0.3\ninfo: {title: Test, version: '1'}\npaths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	baseline := []byte(`{"platformContractVersion":2,"supportedPlatformContractVersions":[2],"coreBaselineVersion":"0.2.0-preview.23"}`)
	if err := os.WriteFile(filepath.Join(root, "api", "protocol-baseline.json"), baseline, 0644); err != nil {
		t.Fatal(err)
	}
	previous := os.Args
	os.Args = []string{"generate-android-wire", source, output, manifestPath}
	t.Cleanup(func() { os.Args = previous })
	main()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		PlatformContractVersion           int
		SupportedPlatformContractVersions []int
		CoreBaselineVersion, BaselineHash string
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.PlatformContractVersion != 2 || len(result.SupportedPlatformContractVersions) != 1 || result.SupportedPlatformContractVersions[0] != 2 || result.CoreBaselineVersion != "0.2.0-preview.23" || result.BaselineHash != sourceHash(baseline) {
		t.Fatalf("missing or wrong platform contract identity: %s", data)
	}
	copy, err := os.ReadFile(filepath.Join(root, "output", "protocol-baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if sourceHash(copy) != result.BaselineHash {
		t.Fatal("baseline export differs from its manifest")
	}
}

// The Android-wire manifest sourceHash must be stable across CRLF (Windows) and
// LF (Linux/CI) working trees, otherwise generated-drift fails non-deterministically.
func TestSourceHashIsLineEndingInsensitive(t *testing.T) {
	lf := "openapi: 3.0.3\ninfo:\n  title: t\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")

	lfHash := sourceHash([]byte(lf))
	crlfHash := sourceHash([]byte(crlf))

	if lfHash != crlfHash {
		t.Fatalf("sourceHash must be line-ending insensitive: lf=%s crlf=%s", lfHash, crlfHash)
	}
}

func TestInvalidContractBaselineRejected(t *testing.T) {
	for _, raw := range []string{
		`{"platformContractVersion":0,"supportedPlatformContractVersions":[2],"coreBaselineVersion":"0.2.0-preview.23"}`,
		`{"platformContractVersion":2,"supportedPlatformContractVersions":[1],"coreBaselineVersion":"0.2.0-preview.23"}`,
		`{"platformContractVersion":2,"supportedPlatformContractVersions":[2,2],"coreBaselineVersion":"0.2.0-preview.23"}`,
		`{"platformContractVersion":2,"supportedPlatformContractVersions":[2,1],"coreBaselineVersion":"0.2.0-preview.23"}`,
		`{"platformContractVersion":2,"supportedPlatformContractVersions":[2],"coreBaselineVersion":"../overwrite"}`,
	} {
		if _, err := loadContractIdentity([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid platform identity: %s", raw)
		}
	}
}
