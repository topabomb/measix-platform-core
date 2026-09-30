package main

import (
	"strings"
	"testing"
)

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
