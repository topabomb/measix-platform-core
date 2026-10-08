package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEventsUsesFixedFilesWhitelistsFieldsAndBounds(t *testing.T) {
	dir := t.TempDir()
	line := `{"time":"2026-09-21T10:00:00Z","level":"INFO","msg":"done","service":"hub","event":"http.request_completed","requestId":"req_safe","token":"must-not-leak"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hub.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relay.stderr.log"), []byte("token=must-not-leak "+strings.Repeat("x", maxLineBytes+200)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ReadEvents(dir, EventFilter{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items=%d", len(page.Items))
	}
	encoded := strings.Join([]string{page.Items[0].Message, page.Items[1].Message}, " ")
	if strings.Contains(encoded, "must-not-leak") {
		t.Fatal("unapproved field leaked")
	}
	if !page.Items[0].Truncated && !page.Items[1].Truncated {
		t.Fatal("long stderr not marked truncated")
	}
}

func TestReadEventsFiltersCorrelation(t *testing.T) {
	dir := t.TempDir()
	content := `{"time":"2026-09-21T10:00:00Z","level":"WARN","msg":"a","event":"one","requestId":"req_match"}` + "\n" +
		`{"time":"2026-09-21T10:01:00Z","level":"INFO","msg":"b","event":"two","requestId":"req_other"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hub.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ReadEvents(dir, EventFilter{Service: "hub", Level: "warn", Correlation: "match"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Event != "one" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestReadEventsProjectsRedactedDiagnosticErrorIntoMessage(t *testing.T) {
	dir := t.TempDir()
	line := `{"time":"2026-09-21T10:00:00Z","level":"ERROR","msg":"startup failed","event":"service.start_failed","error":"address_in_use"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hub.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ReadEvents(dir, EventFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if !strings.Contains(page.Items[0].Message, "address_in_use") {
		t.Fatalf("unsafe or unhelpful Admin event: %q", page.Items[0].Message)
	}
}
