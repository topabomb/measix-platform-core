package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

type SnapshotInput struct {
	DeploymentID      string
	ReleaseID         string
	ManagedGeneration int
	Content           adminapi.ManagedDraftContent
	PublishedAt       time.Time
	PublishedByUserID string
}

type snapshotDescriptor struct {
	DeploymentID      string                                 `json:"deploymentId"`
	SchemaVersion     int                                    `json:"schemaVersion"`
	ManagedGeneration int                                    `json:"managedGeneration"`
	ReleaseID         string                                 `json:"releaseId"`
	Providers         []clientapi.ProviderDefinition         `json:"providers"`
	Models            []clientapi.ModelDefinition            `json:"models"`
	TTS               []clientapi.TtsDefinition              `json:"tts"`
	ASR               []clientapi.AsrDefinition              `json:"asr"`
	MCP               []clientapi.McpDefinition              `json:"mcp"`
	Policy            clientapi.ManagedPolicy                `json:"policy"`
	Metadata          snapshotMetadata                       `json:"metadata"`
	Assistants        []clientapi.ManagedAssistantDefinition `json:"assistants,omitempty"`
	Starters          []clientapi.AssistantStarterDefinition `json:"starters,omitempty"`
}

type snapshotMetadata struct {
	PublishedAt       time.Time `json:"publishedAt"`
	PublishedByUserID *string   `json:"publishedByUserId,omitempty"`
}

func (s *Service) CompileSnapshot(input SnapshotInput) (clientapi.ManagedSnapshot, string, error) {
	if err := platformid.Validate(platformid.Deployment, input.DeploymentID); err != nil {
		return clientapi.ManagedSnapshot{}, "", ErrInvalidDraft
	}
	if err := platformid.Validate(platformid.Release, input.ReleaseID); err != nil || input.ManagedGeneration < 1 {
		return clientapi.ManagedSnapshot{}, "", ErrInvalidDraft
	}
	if err := validateCandidateIDs(input.Content); err != nil {
		return clientapi.ManagedSnapshot{}, "", err
	}
	providers := make([]clientapi.ProviderDefinition, 0, len(input.Content.Providers))
	for _, value := range input.Content.Providers {
		providers = append(providers, clientapi.ProviderDefinition{ProviderId: value.ProviderId, DisplayName: value.DisplayName, ClientProtocol: clientapi.ProviderDefinitionClientProtocol(value.ClientProtocol), Enabled: value.Enabled})
	}
	models := make([]clientapi.ModelDefinition, 0, len(input.Content.Models))
	for _, value := range input.Content.Models {
		capabilities := make([]clientapi.ModelDefinitionCapabilities, 0, len(value.Capabilities))
		for _, c := range value.Capabilities {
			capabilities = append(capabilities, clientapi.ModelDefinitionCapabilities(c))
		}
		inputs := make([]clientapi.ModelDefinitionInputModalities, 0, len(value.InputModalities))
		for _, m := range value.InputModalities {
			inputs = append(inputs, clientapi.ModelDefinitionInputModalities(m))
		}
		outputs := make([]clientapi.ModelDefinitionOutputModalities, 0, len(value.OutputModalities))
		for _, m := range value.OutputModalities {
			outputs = append(outputs, clientapi.ModelDefinitionOutputModalities(m))
		}
		sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
		sort.Slice(inputs, func(i, j int) bool { return inputs[i] < inputs[j] })
		sort.Slice(outputs, func(i, j int) bool { return outputs[i] < outputs[j] })
		models = append(models, clientapi.ModelDefinition{
			ModelId: value.ModelId, ProviderId: value.ProviderId, DisplayName: value.DisplayName,
			UpstreamModelKey: value.UpstreamModelKey, RuntimePath: value.RuntimePath, Enabled: value.Enabled,
			Capabilities: capabilities, InputModalities: inputs, OutputModalities: outputs,
		})
	}
	tts := make([]clientapi.TtsDefinition, 0, len(input.Content.Tts))
	for _, value := range input.Content.Tts {
		tts = append(tts, clientapi.TtsDefinition{TtsId: value.TtsId, DisplayName: value.DisplayName, ClientProtocol: clientapi.TtsDefinitionClientProtocol(value.ClientProtocol), UpstreamModelKey: value.UpstreamModelKey, Voice: value.Voice, RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	asr := make([]clientapi.AsrDefinition, 0, len(input.Content.Asr))
	for _, value := range input.Content.Asr {
		asr = append(asr, clientapi.AsrDefinition{AsrId: value.AsrId, DisplayName: value.DisplayName, ClientProtocol: clientapi.AsrDefinitionClientProtocol(value.ClientProtocol), UpstreamModelKey: value.UpstreamModelKey, Language: value.Language, RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	mcp := make([]clientapi.McpDefinition, 0, len(input.Content.Mcp))
	for _, value := range input.Content.Mcp {
		mcp = append(mcp, clientapi.McpDefinition{McpServerId: value.McpServerId, DisplayName: value.DisplayName, ClientProtocol: clientapi.McpDefinitionClientProtocol(value.ClientProtocol), AuthOwnership: clientapi.McpDefinitionAuthOwnership(value.AuthOwnership), RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	// Compile assistants
	var assistants []clientapi.ManagedAssistantDefinition
	if input.Content.Assistants != nil {
		assistants = make([]clientapi.ManagedAssistantDefinition, 0, len(*input.Content.Assistants))
		for _, a := range *input.Content.Assistants {
			seed := make([]string, len(a.MemorySeed))
			for i, s := range a.MemorySeed {
				seed[i] = strings.TrimSpace(s)
			}
			mcpIds := make([]clientapi.McpServerId, len(a.McpServerIds))
			for i, m := range a.McpServerIds {
				mcpIds[i] = clientapi.McpServerId(m)
			}
			sort.Strings(seed)
			sort.Slice(mcpIds, func(i, j int) bool { return mcpIds[i] < mcpIds[j] })
			assistants = append(assistants, clientapi.ManagedAssistantDefinition{
				AssistantDefinitionId: a.AssistantDefinitionId,
				DisplayName:           a.DisplayName,
				Description:           a.Description,
				SystemPrompt:          a.SystemPrompt,
				ModelId:               a.ModelId,
				MemorySeed:            seed,
				McpServerIds:          mcpIds,
				Enabled:               a.Enabled,
			})
		}
	}
	// Compile starters
	var starters []clientapi.AssistantStarterDefinition
	if input.Content.Starters != nil {
		starters = make([]clientapi.AssistantStarterDefinition, 0, len(*input.Content.Starters))
		for _, s := range *input.Content.Starters {
			starters = append(starters, clientapi.AssistantStarterDefinition{
				StarterId:             s.StarterId,
				AssistantDefinitionId: s.AssistantDefinitionId,
				Title:                 s.Title,
				Prompt:                s.Prompt,
				Description:           s.Description,
				SortOrder:             s.SortOrder,
				Enabled:               s.Enabled,
			})
		}
	}
	sort.Slice(assistants, func(i, j int) bool { return assistants[i].AssistantDefinitionId < assistants[j].AssistantDefinitionId })
	sort.Slice(starters, func(i, j int) bool {
		if starters[i].AssistantDefinitionId == starters[j].AssistantDefinitionId {
			return starters[i].SortOrder < starters[j].SortOrder
		}
		return starters[i].AssistantDefinitionId < starters[j].AssistantDefinitionId
	})
	sort.Slice(providers, func(i, j int) bool { return providers[i].ProviderId < providers[j].ProviderId })
	sort.Slice(models, func(i, j int) bool { return models[i].ModelId < models[j].ModelId })
	sort.Slice(tts, func(i, j int) bool { return tts[i].TtsId < tts[j].TtsId })
	sort.Slice(asr, func(i, j int) bool { return asr[i].AsrId < asr[j].AsrId })
	sort.Slice(mcp, func(i, j int) bool { return mcp[i].McpServerId < mcp[j].McpServerId })

	policy := clientapi.ManagedPolicy{
		PolicyId:            input.Content.Policy.PolicyId,
		AllowLocalProviders: input.Content.Policy.AllowLocalProviders,
		AllowLocalTts:       input.Content.Policy.AllowLocalTts,
		AllowLocalAsr:       input.Content.Policy.AllowLocalAsr,
		AllowLocalMcp:       input.Content.Policy.AllowLocalMcp,
		DefaultModelId:      input.Content.Policy.DefaultModelId,
		DefaultTtsId:        input.Content.Policy.DefaultTtsId,
		DefaultAsrId:        input.Content.Policy.DefaultAsrId,
	}
	var publishedBy *string
	if input.PublishedByUserID != "" {
		value := input.PublishedByUserID
		publishedBy = &value
	}
	metadata := snapshotMetadata{PublishedAt: input.PublishedAt.UTC(), PublishedByUserID: publishedBy}
	descriptor := snapshotDescriptor{
		DeploymentID: input.DeploymentID, SchemaVersion: 2, ManagedGeneration: input.ManagedGeneration,
		ReleaseID: input.ReleaseID, Providers: providers, Models: models, TTS: tts, ASR: asr, MCP: mcp, Policy: policy, Metadata: metadata,
		Assistants: assistants, Starters: starters,
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return clientapi.ManagedSnapshot{}, "", err
	}
	sum := sha256.Sum256(payload)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	var snapshot clientapi.ManagedSnapshot
	snapshot.DeploymentId = input.DeploymentID
	snapshot.SchemaVersion = clientapi.ManagedSnapshotSchemaVersionN2
	snapshot.ManagedGeneration = input.ManagedGeneration
	snapshot.ReleaseId = input.ReleaseID
	snapshot.SnapshotHash = hash
	snapshot.Providers = providers
	snapshot.Models = models
	snapshot.Tts = tts
	snapshot.Asr = asr
	snapshot.Mcp = mcp
	snapshot.Policy = policy
	snapshot.Metadata.PublishedAt = metadata.PublishedAt
	snapshot.Metadata.PublishedByUserId = metadata.PublishedByUserID
	snapshot.Assistants = assistants
	snapshot.Starters = starters
	return snapshot, hash, nil
}

// HashSnapshot recomputes the canonical hash of a decoded managed snapshot.
// It is shared by contract tests and downstream verification tooling so the
// canonical descriptor is not reimplemented outside the capability boundary.
func HashSnapshot(snapshot clientapi.ManagedSnapshot) (string, error) {
	metadata := snapshotMetadata{PublishedAt: snapshot.Metadata.PublishedAt}
	if snapshot.Metadata.PublishedByUserId != nil {
		value := string(*snapshot.Metadata.PublishedByUserId)
		metadata.PublishedByUserID = &value
	}
	descriptor := snapshotDescriptor{
		DeploymentID: string(snapshot.DeploymentId), SchemaVersion: int(snapshot.SchemaVersion), ManagedGeneration: snapshot.ManagedGeneration,
		ReleaseID: string(snapshot.ReleaseId), Providers: snapshot.Providers, Models: snapshot.Models, TTS: snapshot.Tts, ASR: snapshot.Asr, MCP: snapshot.Mcp,
		Policy: snapshot.Policy, Metadata: metadata,
	}
	// Only include v2 fields (assistants, starters) for schemaVersion >= 2.
	// v1 snapshots must produce the same hash as before the v2 fields were added.
	if snapshot.SchemaVersion >= 2 {
		descriptor.Assistants = snapshot.Assistants
		descriptor.Starters = snapshot.Starters
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func projectionToAdminAssistants(src []clientapi.ManagedAssistantDefinition) []adminapi.ManagedAssistantDefinition {
	dst := make([]adminapi.ManagedAssistantDefinition, len(src))
	for i, a := range src {
		seed := make([]string, len(a.MemorySeed))
		for j, s := range a.MemorySeed {
			seed[j] = string(s)
		}
		mcpIds := make([]adminapi.McpServerId, len(a.McpServerIds))
		for j, m := range a.McpServerIds {
			mcpIds[j] = adminapi.McpServerId(string(m))
		}
		dst[i] = adminapi.ManagedAssistantDefinition{
			AssistantDefinitionId: adminapi.AssistantDefinitionId(a.AssistantDefinitionId),
			DisplayName:           a.DisplayName,
			Description:           a.Description,
			SystemPrompt:          a.SystemPrompt,
			ModelId:               adminapi.ModelId(a.ModelId),
			MemorySeed:            seed,
			McpServerIds:          mcpIds,
			Enabled:               a.Enabled,
		}
	}
	return dst
}

func projectionToAdminStarters(src []clientapi.AssistantStarterDefinition) []adminapi.AssistantStarterDefinition {
	dst := make([]adminapi.AssistantStarterDefinition, len(src))
	for i, s := range src {
		dst[i] = adminapi.AssistantStarterDefinition{
			StarterId:             adminapi.StarterId(s.StarterId),
			AssistantDefinitionId: adminapi.AssistantDefinitionId(s.AssistantDefinitionId),
			Title:                 s.Title,
			Prompt:                s.Prompt,
			Description:           s.Description,
			SortOrder:             s.SortOrder,
			Enabled:               s.Enabled,
		}
	}
	return dst
}

// projectionToAdminProviders converts clientapi projection back to adminapi types for preview.
func projectionToAdminProviders(src []clientapi.ProviderDefinition) []adminapi.ProviderDefinition {
	dst := make([]adminapi.ProviderDefinition, len(src))
	for i, v := range src {
		dst[i] = adminapi.ProviderDefinition{
			ProviderId:     v.ProviderId,
			DisplayName:    v.DisplayName,
			ClientProtocol: adminapi.ProviderDefinitionClientProtocol(string(v.ClientProtocol)),
			Enabled:        v.Enabled,
		}
	}
	return dst
}

// projectionToAdminModels converts clientapi projection back to adminapi types for preview.
func projectionToAdminModels(src []clientapi.ModelDefinition) []adminapi.ModelDefinition {
	dst := make([]adminapi.ModelDefinition, len(src))
	for i, v := range src {
		caps := make([]adminapi.ModelDefinitionCapabilities, len(v.Capabilities))
		for j, c := range v.Capabilities {
			caps[j] = adminapi.ModelDefinitionCapabilities(string(c))
		}
		inputs := make([]adminapi.ModelDefinitionInputModalities, len(v.InputModalities))
		for j, m := range v.InputModalities {
			inputs[j] = adminapi.ModelDefinitionInputModalities(string(m))
		}
		outputs := make([]adminapi.ModelDefinitionOutputModalities, len(v.OutputModalities))
		for j, m := range v.OutputModalities {
			outputs[j] = adminapi.ModelDefinitionOutputModalities(string(m))
		}
		dst[i] = adminapi.ModelDefinition{
			ModelId: v.ModelId, ProviderId: v.ProviderId, DisplayName: v.DisplayName,
			UpstreamModelKey: v.UpstreamModelKey, RuntimePath: v.RuntimePath, Enabled: v.Enabled,
			Capabilities: caps, InputModalities: inputs, OutputModalities: outputs,
		}
	}
	return dst
}

// projectionToAdminTts converts clientapi projection back to adminapi types for preview.
func projectionToAdminTts(src []clientapi.TtsDefinition) []adminapi.TtsDefinition {
	dst := make([]adminapi.TtsDefinition, len(src))
	for i, v := range src {
		dst[i] = adminapi.TtsDefinition{
			TtsId: v.TtsId, DisplayName: v.DisplayName,
			ClientProtocol:   adminapi.TtsDefinitionClientProtocol(string(v.ClientProtocol)),
			UpstreamModelKey: v.UpstreamModelKey, Voice: v.Voice,
			RuntimePath: v.RuntimePath, Enabled: v.Enabled,
		}
	}
	return dst
}

// projectionToAdminAsr converts clientapi projection back to adminapi types for preview.
func projectionToAdminAsr(src []clientapi.AsrDefinition) []adminapi.AsrDefinition {
	dst := make([]adminapi.AsrDefinition, len(src))
	for i, v := range src {
		dst[i] = adminapi.AsrDefinition{
			AsrId: v.AsrId, DisplayName: v.DisplayName,
			ClientProtocol:   adminapi.AsrDefinitionClientProtocol(string(v.ClientProtocol)),
			UpstreamModelKey: v.UpstreamModelKey, Language: v.Language,
			RuntimePath: v.RuntimePath, Enabled: v.Enabled,
		}
	}
	return dst
}

// projectionToAdminMcp converts clientapi projection back to adminapi types for preview.
func projectionToAdminMcp(src []clientapi.McpDefinition) []adminapi.McpDefinition {
	dst := make([]adminapi.McpDefinition, len(src))
	for i, v := range src {
		dst[i] = adminapi.McpDefinition{
			McpServerId: v.McpServerId, DisplayName: v.DisplayName,
			ClientProtocol: adminapi.McpDefinitionClientProtocol(string(v.ClientProtocol)),
			AuthOwnership:  adminapi.McpDefinitionAuthOwnership(string(v.AuthOwnership)),
			RuntimePath:    v.RuntimePath, Enabled: v.Enabled,
		}
	}
	return dst
}
