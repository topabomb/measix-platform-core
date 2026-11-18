package contract_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
)

func TestSharedMcpToolFullContractJCSVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "client-integration", "mcp-tool-contract-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, RawDefinition, Canonical, ContractHash string }
	if json.Unmarshal(raw, &vectors) != nil || len(vectors) < 4 {
		t.Fatal("missing JCS interoperability vectors")
	}
	hashes := map[string]string{}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			sum := sha256.Sum256([]byte(v.Canonical))
			if "sha256:"+hex.EncodeToString(sum[:]) != v.ContractHash {
				t.Fatal("vector canonical bytes differ from digest")
			}
			var def adminapi.McpToolDefinition
			if err := json.Unmarshal([]byte(v.RawDefinition), &def); err != nil {
				t.Fatal(err)
			}
			hash, err := capability.McpToolContractHash(def)
			if err != nil || hash != v.ContractHash {
				t.Fatalf("full-contract JCS mismatch: %s %v", hash, err)
			}
			hashes[v.Name] = hash
		})
	}
	if hashes["full-contract"] != hashes["property-order-only"] || hashes["full-contract"] == hashes["description-drift"] {
		t.Fatal("incorrect definition drift semantics")
	}
}
