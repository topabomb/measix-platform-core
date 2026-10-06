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
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return err
	}
	// Project through the selected generated wire DTO before the historical
	// adapter. A v5 field must not acquire meaning (or a type check) in v4.
	var wire any
	switch header.SchemaVersion {
	case 4:
		wire = &clientapi.ManagedSnapshotV4{}
	case 5:
		wire = &clientapi.ManagedSnapshot{}
	default:
		return ErrInvalidDraft
	}
	if err := json.Unmarshal(raw, wire); err != nil {
		return err
	}
	known, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	type plain Snapshot
	var value plain
	if err := json.Unmarshal(known, &value); err != nil {
		return err
	}
	*s = Snapshot(value)
	if s.SchemaVersion == 4 {
		s.V4Assistants = wire.(*clientapi.ManagedSnapshotV4).Assistants
	}
	return nil
}
