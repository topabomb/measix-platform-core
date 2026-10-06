package capability

import (
	"encoding/json"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"sort"
)

func clientMcpGrants(src *[]adminapi.McpToolGrant) []clientapi.McpToolGrant {
	dst := []clientapi.McpToolGrant{}
	if src != nil {
		for _, v := range *src {
			dst = append(dst, clientapi.McpToolGrant{Name: v.Name, ContractHash: v.ContractHash, ApprovalPolicy: clientapi.McpToolGrantApprovalPolicy(v.ApprovalPolicy)})
		}
	}
	sort.Slice(dst, func(i, j int) bool { return dst[i].Name < dst[j].Name })
	return dst
}
func adminMcpGrants(src []clientapi.McpToolGrant) *[]adminapi.McpToolGrant {
	dst := make([]adminapi.McpToolGrant, 0, len(src))
	for _, v := range src {
		dst = append(dst, adminapi.McpToolGrant{Name: v.Name, ContractHash: v.ContractHash, ApprovalPolicy: adminapi.McpToolGrantApprovalPolicy(v.ApprovalPolicy), Definition: adminapi.McpToolDefinition{}})
	}
	return &dst
}
func clientAssistantBindings(src *[]adminapi.AssistantMcpBinding) []clientapi.AssistantMcpBinding {
	dst := []clientapi.AssistantMcpBinding{}
	if src != nil {
		for _, v := range *src {
			names := append([]string{}, v.ToolNames...)
			sort.Strings(names)
			dst = append(dst, clientapi.AssistantMcpBinding{McpServerId: v.McpServerId, ToolSelection: clientapi.AssistantMcpBindingToolSelection(v.ToolSelection), ToolNames: names})
		}
	}
	sort.Slice(dst, func(i, j int) bool { return dst[i].McpServerId < dst[j].McpServerId })
	return dst
}
func v4Mcp(src []clientapi.McpDefinition) []clientapi.McpDefinitionV4 {
	dst := make([]clientapi.McpDefinitionV4, 0, len(src))
	for _, v := range src {
		dst = append(dst, clientapi.McpDefinitionV4{McpServerId: v.McpServerId, DisplayName: v.DisplayName, ClientProtocol: clientapi.McpDefinitionV4ClientProtocol(v.ClientProtocol), AuthOwnership: clientapi.McpDefinitionV4AuthOwnership(v.AuthOwnership), RuntimePath: v.RuntimePath, Enabled: v.Enabled})
	}
	return dst
}
func mcpPublishedHash(m adminapi.McpDefinition) string {
	m.ToolDiscovery = nil
	// Definitions are review evidence; the approved hash is the published fact.
	if m.AllowedTools != nil {
		grants := append([]adminapi.McpToolGrant{}, (*m.AllowedTools)...)
		sort.Slice(grants, func(i, j int) bool { return grants[i].Name < grants[j].Name })
		for i := range grants {
			grants[i].Definition = nil
		}
		m.AllowedTools = &grants
	}
	return defHash(m)
}
func (s *Snapshot) UnmarshalJSON(raw []byte) error {
	type plain Snapshot
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*s = Snapshot(value)
	if s.SchemaVersion == 4 {
		var historical clientapi.ManagedSnapshotV4
		if err := json.Unmarshal(raw, &historical); err != nil {
			return err
		}
		s.V4Assistants = historical.Assistants
	}
	return nil
}
