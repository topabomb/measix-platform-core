package capability_test

import (
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"testing"
)

func TestDirectMcpDraftDistinguishesMissingFromNull(t *testing.T) {
	for _, raw := range []string{
		`{"mcp":[{"allowedTools":null}]}`,
		`{"assistants":[{"mcpBindings":null}]}`,
		`{"mcp":[{"allowedTools":[{"name":"read","contractHash":"sha256:00","approvalPolicy":null,"definition":{}}]}]}`,
		`{"assistants":[{"mcpBindings":[{"mcpServerId":"mcp_x","toolNames":null}]}]}`,
	} {
		var value adminapi.ManagedDraftContent
		if capability.DecodeManagedDraftContent([]byte(raw), &value) == nil {
			t.Fatalf("invalid persisted tool fields accepted: %s", raw)
		}
	}
	var unfinished adminapi.ManagedDraftContent
	if err := capability.DecodeManagedDraftContent([]byte(`{"mcp":[{}],"assistants":[{}]}`), &unfinished); err != nil {
		t.Fatal("missing legacy fields should stay unfinished", err)
	}
}
