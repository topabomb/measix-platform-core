package contract_test

import (
	"github.com/getkin/kin-openapi/openapi3"
	"path/filepath"
	"testing"
)

func TestDirectMcpV5ExplicitToolGrantsAndV4Isolation(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(fixtureRoot(t), "..", "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	mcp := getSchema(t, doc, "McpDefinition")
	tools := getProp(t, mcp, "allowedTools")
	if tools.Items == nil {
		t.Fatal("explicit tool grants missing")
	}
	grant := tools.Items.Value
	for _, field := range []string{"name", "contractHash", "approvalPolicy"} {
		getProp(t, grant, field)
	}
	assistant := getSchema(t, doc, "ManagedAssistantDefinition")
	getProp(t, assistant, "mcpBindings")
	if assistant.Properties["mcpServerIds"] != nil {
		t.Fatal("v5 must not accept server-only permission")
	}
	old := getSchema(t, doc, "ManagedAssistantDefinitionV4")
	getProp(t, old, "mcpServerIds")
	if old.Properties["mcpBindings"] != nil {
		t.Fatal("v4 changed")
	}
	if getSchema(t, doc, "McpDefinitionV4").Properties["allowedTools"] != nil {
		t.Fatal("v4 changed")
	}
	for _, value := range []map[string]any{
		{"name": "read", "contractHash": "sha256:bad", "approvalPolicy": "AUTO"},
		{"name": "read", "contractHash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "approvalPolicy": "AUTO_ALL"},
		{"name": "read", "contractHash": nil, "approvalPolicy": "AUTO"},
	} {
		if grant.VisitJSON(value) == nil {
			t.Fatal("invalid grant accepted")
		}
	}
}
