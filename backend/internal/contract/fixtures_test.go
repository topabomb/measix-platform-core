package contract_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/internal/wire/usageingestapi"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "api", "fixtures"))
}

func decodeFixture[T any](t *testing.T, rel string, strict bool) T {
	t.Helper()
	file, err := os.Open(filepath.Join(fixtureRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var value T
	decoder := json.NewDecoder(io.LimitReader(file, 2<<20))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode fixture %s: %v", rel, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture %s has trailing JSON", rel)
	}
	return value
}

func TestSYSI0001CanonicalFixturesDecodeWithGeneratedWire(t *testing.T) {
	// SYS-I0-001: canonical fixtures and generated Go wire must remain compatible.
	_ = decodeFixture[clientapi.Discovery](t, "identity/discovery.json", true)
	_ = decodeFixture[clientapi.EnrollmentExchangeRequest](t, "identity/enrollment-exchange-request.json", true)
	_ = decodeFixture[clientapi.EnrollmentExchangeResponse](t, "identity/enrollment-exchange-response.json", true)
	_ = decodeFixture[clientapi.ManagedState](t, "managed-state/ready-generation-42.json", true)
	_ = decodeFixture[clientapi.ManagedState](t, "managed-state/sync-required.json", true)
	_ = decodeFixture[adminapi.Draft](t, "draft/minimal.json", true)
	_ = decodeFixture[clientapi.Problem](t, "problem/managed-snapshot-required.json", true)
	_ = decodeFixture[adminapi.Problem](t, "problem/stale-draft-revision.json", true)
	_ = decodeFixture[usageingestapi.UsageBatch](t, "usage/request-batch.json", true)
}

func TestSYSI0002UnknownOptionalResponseFieldIsTolerated(t *testing.T) {
	// SYS-I0-002: response readers tolerate a future optional response field.
	_ = decodeFixture[adminapi.AdminSession](t, "compat/admin-session-unknown-optional-response.json", false)
}

func TestSYSI0003UnknownRequestFieldIsRejected(t *testing.T) {
	// SYS-I0-003: request readers use strict decoding at the HTTP boundary.
	file, err := os.Open(filepath.Join(fixtureRoot(t), "invalid/create-user-unknown-field.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var request adminapi.CreateUserRequest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err == nil {
		t.Fatal("unknown request field unexpectedly accepted")
	}
}

func TestSnapshotAndRuntimeControlGoldenHashes(t *testing.T) {
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/generation-42.json", true)
	hash, err := capability.HashSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if hash != string(snapshot.SnapshotHash) {
		t.Fatalf("snapshot golden hash=%s want=%s", hash, snapshot.SnapshotHash)
	}

	state := decodeFixture[relaycontrolapi.RuntimeControlState](t, "runtime-control/revision-57.json", true)
	controlHash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	if controlHash != state.BundleHash {
		t.Fatalf("runtime-control golden hash=%s want=%s", controlHash, state.BundleHash)
	}
}
