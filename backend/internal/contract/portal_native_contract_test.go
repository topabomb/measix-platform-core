package contract_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestPortalExportManifestMatchesCanonicalInputs(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "..")
	clientBytes, err := os.ReadFile(filepath.Join(root, "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	feedBytes, err := os.ReadFile(filepath.Join(root, "portal", "client-feed.schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	var feedSource struct {
		Sha256 string `json:"x-source-sha256"`
	}
	if err := json.Unmarshal(feedBytes, &feedSource); err != nil {
		t.Fatal(err)
	}
	clientDigest := sha256.Sum256(clientBytes)
	if feedSource.Sha256 != hex.EncodeToString(clientDigest[:]) {
		t.Fatal("stale generated Client Feed dependency")
	}
	exported := filepath.Join(root, "generated", "android", "portal")
	data, err := os.ReadFile(filepath.Join(exported, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		BridgeVersion    int
		LocalReadVersion int
		Artifacts        map[string]struct{ Source, Sha256 string }
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.BridgeVersion != 3 || manifest.LocalReadVersion != 2 || len(manifest.Artifacts) != 8 {
		t.Fatal("incomplete native export manifest")
	}
	for name, artifact := range manifest.Artifacts {
		canonical, err := os.ReadFile(filepath.Join(root, "..", artifact.Source))
		if err != nil {
			t.Fatal(err)
		}
		copied, err := os.ReadFile(filepath.Join(exported, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, bytes := range [][]byte{canonical, copied} {
			digest := sha256.Sum256(bytes)
			if hex.EncodeToString(digest[:]) != artifact.Sha256 {
				t.Fatalf("stale export %s", name)
			}
		}
	}
	// The Android handoff must resolve its Feed dependency without the core checkout.
	standalone := t.TempDir()
	for name := range manifest.Artifacts {
		bytes, err := os.ReadFile(filepath.Join(exported, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(standalone, name), bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(filepath.Join(standalone, "portal-contract.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPortalNativeSharedVectors(t *testing.T) {
	root := fixtureRoot(t)
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join(root, "..", "portal", "portal-contract.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "portal", "native-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name   string
		Schema string
		Valid  bool
		Value  any
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			schema := document.Components.Schemas[vector.Schema]
			if schema == nil {
				t.Fatal("missing schema", vector.Schema)
			}
			err := schema.Value.VisitJSON(vector.Value, utcDateFormats())
			if (err == nil) != vector.Valid {
				t.Fatalf("valid=%t, got %v", vector.Valid, err)
			}
		})
	}
}

func TestPortalDocumentBoundRequest(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join(fixtureRoot(t), "..", "portal", "portal-contract.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []float64{2, 3} {
		request := map[string]any{"bridgeVersion": version, "documentId": "doc_fixture", "requestId": "request_fixture", "method": "listLocalUpdates", "params": map[string]any{"limit": float64(20)}}
		err := document.Components.Schemas["BridgeRequest"].Value.VisitJSON(request)
		if (err == nil) != (version == 3) {
			t.Fatalf("v%v accepted=%v: %v", version, err == nil, err)
		}
	}
}
