package capability

import (
	"context"
	"testing"

	"measix/platform/internal/wire/adminapi"
)

func TestDisabledMcpKeepsReviewedPolicyWithoutRequiringDiscovery(t *testing.T) {
	mode := adminapi.McpDefinitionToolAccessMode("ALLOWLIST")
	grants := []adminapi.McpToolGrant{{Name: "read"}}
	content := adminapi.ManagedDraftContent{Mcp: []adminapi.McpDefinition{{McpServerId: "mcp_disabled", Enabled: false, ToolAccessMode: &mode, AllowedTools: &grants}}}
	if issues := NewService(nil).mcpSourceIssues(context.Background(), content); len(issues) != 0 {
		t.Fatalf("disabled server should retain its policy without a live source: %+v", issues)
	}
}

func TestDirectMcpSelectionSeparatesBindingFromToolRestriction(t *testing.T) {
	for _, tc := range []struct{ name, serverMode, assistantMode, serverTools, names, code string }{
		{"all dynamically follows server", "ALL", "ALL", `[]`, `[]`, ""},
		{"assistant can restrict an open server", "ALL", "ALLOWLIST", `[]`, `["future-tool"]`, ""},
		{"restricted empty list is not unrestricted", "ALL", "ALLOWLIST", `[]`, `[]`, "invalid_mcp_tool_ref"},
		{"all cannot conceal a list", "ALL", "ALL", `[]`, `["read"]`, "invalid_mcp_tool_ref"},
		{"server restriction cannot be empty", "ALLOWLIST", "ALL", `[]`, `[]`, "invalid_mcp_tool_grant"},
		{"invalid mode is rejected", "ALL", "UNKNOWN", `[]`, `[]`, "invalid_mcp_tool_ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"mcp":[{"mcpServerId":"mcp_test","enabled":true,"toolAccessMode":"` + tc.serverMode + `","allowedTools":` + tc.serverTools + `}],"assistants":[{"assistantDefinitionId":"asst_test","mcpBindings":[{"mcpServerId":"mcp_test","toolSelection":"` + tc.assistantMode + `","toolNames":` + tc.names + `}]}]}`
			var content adminapi.ManagedDraftContent
			if err := DecodeManagedDraftContent([]byte(raw), &content); err != nil {
				t.Fatal(err)
			}
			issues := mcpGovernanceIssues(content)
			if tc.code == "" && len(issues) != 0 {
				t.Fatalf("valid selection rejected: %+v", issues)
			}
			if tc.code != "" {
				found := false
				for _, issue := range issues {
					found = found || issue.Code == tc.code
				}
				if !found {
					t.Fatalf("missing %s: %+v", tc.code, issues)
				}
			}
		})
	}
}
