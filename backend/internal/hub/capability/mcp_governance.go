package capability

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"measix/platform/ent/manageddraft"
	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/ent/workspaceserviceconfig"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
)

type mcpSource struct {
	endpoint, hash string
	headers        http.Header
}

func (s *Service) mcpSource(ctx context.Context, content adminapi.ManagedDraftContent, id, user string, auth bool) (mcpSource, error) {
	out := mcpSource{headers: http.Header{}}
	var mcp *adminapi.McpDefinition
	var binding *adminapi.RuntimeBindingDefinition
	for i := range content.Mcp {
		if content.Mcp[i].McpServerId == id {
			mcp = &content.Mcp[i]
		}
	}
	for i := range content.Bindings {
		if content.Bindings[i].ResourceId == id {
			if binding != nil {
				return out, ErrMcpSourceUnavailable
			}
			binding = &content.Bindings[i]
		}
	}
	if mcp == nil || binding == nil || !mcp.Enabled || !validRuntimePath(mcp.RuntimePath) || !runtimePathAllowed(mcp.RuntimePath, binding.AllowedPathPrefixes) || !hasMethods(binding.AllowedMethods, "POST", "GET", "DELETE") {
		return out, ErrMcpSourceUnavailable
	}
	var identity any
	if binding.TargetKind != nil && *binding.TargetKind == "REMOTE_WORKSPACE" {
		if binding.WorkspaceServiceId == nil || user == "" {
			return out, ErrMcpSourceUnavailable
		}
		service, err := s.Client.WorkspaceService.Get(ctx, *binding.WorkspaceServiceId)
		if err != nil || !service.Enabled || service.State != "ACTIVE" || service.ActiveConfigRevision == nil || service.McpServerID != id || mcp.RuntimePath != "/mcp" || binding.RuntimeRouteId != service.RuntimeRouteID {
			return out, ErrMcpSourceUnavailable
		}
		space, err := s.Client.AgentSpace.Get(ctx, user)
		if err != nil || space.WorkspaceServiceID != service.ID || space.State != "CONNECTED" || space.Intent != "CONNECTED" || !space.RemoteActive || space.StopPending || space.McpSecretID == "" {
			return out, ErrMcpSourceUnavailable
		}
		subject, err := s.Client.User.Get(ctx, user)
		if err != nil || subject.Status != "ACTIVE" {
			return out, ErrMcpSourceUnavailable
		}
		rev, err := s.Client.WorkspaceServiceConfig.Query().Where(workspaceserviceconfig.WorkspaceServiceIDEQ(service.ID), workspaceserviceconfig.RevisionEQ(*service.ActiveConfigRevision)).Only(ctx)
		if err != nil {
			return out, ErrMcpSourceUnavailable
		}
		var cfg adminapi.AgentSpaceConfig
		if json.Unmarshal(rev.ConfigJSON, &cfg) != nil {
			return out, ErrMcpSourceUnavailable
		}
		out.endpoint = cfg.McpOrigin + "/u/" + space.RemoteUsername + "/mcp"
		identity = []any{service.ID, service.ConfigRevision, *service.ActiveConfigRevision, cfg.McpOrigin, user, space.BindingRevision, space.McpSecretID, space.McpSecretVersion, space.AgentSpaceID, space.RemoteUsername}
		if auth {
			if s.Secrets == nil {
				return out, ErrMcpSourceUnavailable
			}
			token, err := s.Secrets.ResolveSecret(ctx, space.McpSecretID, int(space.McpSecretVersion))
			if err != nil {
				return out, ErrMcpSourceUnavailable
			}
			out.headers.Set("Authorization", "Bearer "+string(token))
		}
	} else {
		if user != "" || binding.UpstreamId == "" {
			return out, ErrMcpSourceUnavailable
		}
		row, err := s.Client.Upstream.Get(ctx, binding.UpstreamId)
		// A pending candidate must be applied before discovery, so publication and
		// discovery cannot silently refer to different connection configurations.
		if err != nil || row.Status != "ACTIVE" || row.ActiveConfigRevision == nil || *row.ActiveConfigRevision != row.ConfigRevision {
			return out, ErrMcpSourceUnavailable
		}
		rev, err := s.Client.UpstreamConfigRevision.Query().Where(upstreamconfigrevision.UpstreamIDEQ(row.ID), upstreamconfigrevision.RevisionEQ(*row.ActiveConfigRevision)).Only(ctx)
		if err != nil {
			return out, ErrMcpSourceUnavailable
		}
		var cfg adminapi.UpstreamConfig
		if json.Unmarshal(rev.ConfigJSON, &cfg) != nil || upstream.ValidateConfig(ctx, s.Client, cfg) != nil {
			return out, ErrMcpSourceUnavailable
		}
		out.endpoint = strings.TrimRight(cfg.BaseUrl, "/") + mcp.RuntimePath
		identity = []any{row.ID, row.ConfigRevision, cfg}
		if auth {
			refs, err := upstream.SecretRefs(cfg.Auth)
			if err != nil {
				return out, ErrMcpSourceUnavailable
			}
			token := ""
			if len(refs) > 0 {
				if s.Secrets == nil {
					return out, ErrMcpSourceUnavailable
				}
				value, err := s.Secrets.ResolveSecret(ctx, refs[0].SecretID, refs[0].Version)
				if err != nil {
					return out, ErrMcpSourceUnavailable
				}
				token = string(value)
			}
			switch cfg.Auth.Type {
			case "NONE":
			case "BEARER":
				out.headers.Set("Authorization", "Bearer "+token)
			case "STATIC_HEADER":
				if cfg.Auth.HeaderName == nil {
					return out, ErrMcpSourceUnavailable
				}
				out.headers.Set(*cfg.Auth.HeaderName, token)
			case "BASIC":
				if cfg.Auth.Username == nil {
					return out, ErrMcpSourceUnavailable
				}
				out.headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(*cfg.Auth.Username+":"+token)))
			default:
				return out, ErrMcpSourceUnavailable
			}
		}
	}
	raw, _ := json.Marshal([]any{id, mcp.RuntimePath, mcp.AuthOwnership, binding, identity})
	sum := sha256.Sum256(raw)
	out.hash = "sha256:" + hex.EncodeToString(sum[:])
	return out, nil
}
func discoveryUser(d *adminapi.McpToolDiscovery) string {
	if d != nil && d.UserId != nil {
		return *d.UserId
	}
	return ""
}

func (s *Service) DiscoverMcpTools(ctx context.Context, actor, id string, request adminapi.DiscoverMcpToolsRequest) (DraftView, error) {
	draft, err := s.GetDraft(ctx)
	if err != nil {
		return DraftView{}, err
	}
	if draft.DraftRevision != request.ExpectedDraftRevision {
		return DraftView{}, ErrRevisionConflict
	}
	user := ""
	if request.UserId != nil {
		user = *request.UserId
	}
	source, err := s.mcpSource(ctx, draft.Content, id, user, true)
	if err != nil {
		return DraftView{}, err
	}
	tools, err := DiscoverMcpCatalog(ctx, source.endpoint, source.headers)
	if err != nil {
		return DraftView{}, err
	}
	// All remote IO has finished. This short transaction gives the source and
	// draft checks the same storage view as the revision update.
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return DraftView{}, err
	}
	defer tx.Rollback()
	local := *s
	local.Client = tx.Client()
	current, err := local.GetDraft(ctx)
	if err != nil {
		return DraftView{}, err
	}
	if current.DraftRevision != request.ExpectedDraftRevision {
		return DraftView{}, ErrRevisionConflict
	}
	fresh, err := local.mcpSource(ctx, current.Content, id, user, false)
	if err != nil || fresh.hash != source.hash {
		return DraftView{}, ErrMcpSourceChanged
	}
	for i := range current.Content.Mcp {
		if current.Content.Mcp[i].McpServerId == id {
			current.Content.Mcp[i].ToolDiscovery = &adminapi.McpToolDiscovery{SourceHash: source.hash, DiscoveredAt: s.Now().UTC(), Tools: tools, UserId: request.UserId}
		}
	}
	raw, err := json.Marshal(current.Content)
	if err != nil {
		return DraftView{}, err
	}
	if len(raw) > MaxSnapshotBytes {
		return DraftView{}, ErrMcpDiscoveryLimit
	}
	n, err := tx.ManagedDraft.Update().Where(manageddraft.IDEQ(current.DraftID), manageddraft.DraftRevisionEQ(int64(current.DraftRevision))).SetContentJSON(raw).SetDraftRevision(int64(current.DraftRevision + 1)).SetUpdatedByUserID(actor).SetUpdatedAt(s.Now().UTC()).Save(ctx)
	if err != nil {
		return DraftView{}, err
	}
	if n != 1 {
		return DraftView{}, ErrRevisionConflict
	}
	if err = tx.Commit(); err != nil {
		return DraftView{}, err
	}
	current.DraftRevision++
	return current, nil
}
func (s *Service) validateMcpSave(ctx context.Context, old, next adminapi.ManagedDraftContent) error {
	byID := map[string]adminapi.McpDefinition{}
	for _, m := range old.Mcp {
		byID[m.McpServerId] = m
	}
	for _, m := range next.Mcp {
		prior := byID[m.McpServerId]
		if defHash(m.ToolDiscovery) != defHash(prior.ToolDiscovery) {
			return ErrMcpToolEvidence
		}
		if m.AllowedTools == nil {
			continue
		}
		seen := map[string]bool{}
		for _, grant := range *m.AllowedTools {
			hash, err := McpToolContractHash(grant.Definition)
			if err != nil || hash != grant.ContractHash || grant.Definition["name"] != grant.Name || !grant.ApprovalPolicy.Valid() || seen[grant.Name] {
				return ErrMcpToolEvidence
			}
			seen[grant.Name] = true
			unchanged := false
			if prior.AllowedTools != nil {
				for _, previous := range *prior.AllowedTools {
					if defHash(previous) == defHash(grant) {
						unchanged = true
					}
				}
			}
			if unchanged {
				continue
			}
			if m.ToolDiscovery == nil {
				return ErrMcpToolEvidence
			}
			source, err := s.mcpSource(ctx, next, m.McpServerId, discoveryUser(m.ToolDiscovery), false)
			if err != nil || source.hash != m.ToolDiscovery.SourceHash {
				return ErrMcpToolEvidence
			}
			found := false
			for _, tool := range m.ToolDiscovery.Tools {
				if tool.Name == grant.Name && tool.ContractHash == grant.ContractHash && defHash(tool.Definition) == defHash(grant.Definition) {
					found = true
				}
			}
			if !found {
				return ErrMcpToolEvidence
			}
		}
	}
	return nil
}

func mcpGovernanceIssues(content adminapi.ManagedDraftContent) []adminapi.ValidationIssue {
	out := []adminapi.ValidationIssue{}
	issue := func(code, path, msg, kind, id, field string) {
		k := adminapi.ValidationIssueResourceKind(kind)
		out = append(out, adminapi.ValidationIssue{Code: code, Path: path, Message: msg, Severity: "ERROR", ResourceKind: &k, ResourceId: &id, Field: &field})
	}
	enabled := map[string]bool{}
	allowed := map[string]map[string]bool{}
	restricted := map[string]bool{}
	for i, m := range content.Mcp {
		path := fmt.Sprintf("mcp[%d].allowedTools", i)
		enabled[m.McpServerId] = m.Enabled
		allowed[m.McpServerId] = map[string]bool{}
		if m.AllowedTools == nil || m.ToolAccessMode == nil {
			issue("missing_mcp_tool_grants", path, "Choose all tools or an explicit server allowlist", "MCP", m.McpServerId, "allowedTools")
			continue
		}
		restricted[m.McpServerId] = *m.ToolAccessMode == "ALLOWLIST"
		if !m.ToolAccessMode.Valid() || (*m.ToolAccessMode == "ALL" && len(*m.AllowedTools) != 0) || (restricted[m.McpServerId] && len(*m.AllowedTools) == 0) {
			issue("invalid_mcp_tool_grant", path, "All tools requires an empty list; a server allowlist requires at least one reviewed tool", "MCP", m.McpServerId, "allowedTools")
		}
		for j, grant := range *m.AllowedTools {
			if allowed[m.McpServerId][grant.Name] {
				issue("duplicate_mcp_tool", path, "Tool name occurs more than once", "MCP", m.McpServerId, "allowedTools")
			}
			allowed[m.McpServerId][grant.Name] = true
			hash, err := McpToolContractHash(grant.Definition)
			if err != nil || hash != grant.ContractHash || grant.Definition["name"] != grant.Name || !grant.ApprovalPolicy.Valid() {
				issue("invalid_mcp_tool_grant", fmt.Sprintf("%s[%d]", path, j), "Tool approval has an invalid definition, hash or confirmation policy", "MCP", m.McpServerId, "allowedTools")
			}
		}
	}
	for i, a := range content.Assistants {
		path := fmt.Sprintf("assistants[%d].mcpBindings", i)
		if a.McpBindings == nil || len(a.McpServerIds) > 0 {
			issue("missing_mcp_tool_bindings", path, "Author mandatory MCP bindings (an empty list is valid); legacy references need explicit conversion", "ASSISTANT", a.AssistantDefinitionId, "mcpBindings")
			continue
		}
		servers := map[string]bool{}
		for j, binding := range *a.McpBindings {
			bp := fmt.Sprintf("%s[%d]", path, j)
			if !enabled[binding.McpServerId] || servers[binding.McpServerId] {
				issue("invalid_mcp_ref", bp, "Assistant references an unknown, disabled or duplicate MCP server", "ASSISTANT", a.AssistantDefinitionId, "mcpBindings")
			}
			servers[binding.McpServerId] = true
			seen := map[string]bool{}
			if binding.ToolNames == nil || !binding.ToolSelection.Valid() || (binding.ToolSelection == "ALL" && len(binding.ToolNames) != 0) || (binding.ToolSelection == "ALLOWLIST" && len(binding.ToolNames) == 0) {
				issue("invalid_mcp_tool_ref", bp, "All tools requires an empty list; selected tools requires at least one tool", "ASSISTANT", a.AssistantDefinitionId, "mcpBindings")
			}
			for _, name := range binding.ToolNames {
				if strings.TrimSpace(name) == "" || strings.Contains(name, "*") || (restricted[binding.McpServerId] && !allowed[binding.McpServerId][name]) || seen[name] {
					issue("invalid_mcp_tool_ref", bp, "Assistant tool is missing, duplicated or outside the server allowlist", "ASSISTANT", a.AssistantDefinitionId, "mcpBindings")
				}
				seen[name] = true
			}
		}
	}
	return out
}
func (s *Service) mcpSourceIssues(ctx context.Context, content adminapi.ManagedDraftContent) []adminapi.ValidationIssue {
	out := []adminapi.ValidationIssue{}
	for i, m := range content.Mcp {
		if !m.Enabled || m.AllowedTools == nil || len(*m.AllowedTools) == 0 {
			continue
		}
		valid := m.ToolDiscovery != nil
		if valid {
			source, err := s.mcpSource(ctx, content, m.McpServerId, discoveryUser(m.ToolDiscovery), false)
			valid = err == nil && source.hash == m.ToolDiscovery.SourceHash
		}
		if valid {
			for _, grant := range *m.AllowedTools {
				found := false
				for _, tool := range m.ToolDiscovery.Tools {
					if grant.Name == tool.Name && grant.ContractHash == tool.ContractHash {
						found = true
					}
				}
				valid = valid && found
			}
		}
		if !valid {
			k := adminapi.ValidationIssueResourceKind("MCP")
			field := "allowedTools"
			id := m.McpServerId
			out = append(out, adminapi.ValidationIssue{Code: "mcp_tool_review_required", Path: fmt.Sprintf("mcp[%d].allowedTools", i), Message: "Discover the current source and explicitly review changed or removed tools", Severity: "ERROR", ResourceKind: &k, ResourceId: &id, Field: &field})
		}
	}
	return out
}
