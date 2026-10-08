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

// CurrentSnapshotSchemaVersion is the default for new draft publication.
const CurrentSnapshotSchemaVersion = 5

const MaxSnapshotBytes = 4 << 20

func SupportedSnapshotSchemaVersions() []int { return []int{4, 5} }

type SnapshotInput struct {
	// Zero compiles a new draft, including System inheritance. An explicit version
	// compiles historical, already materialized content without reinterpreting it.
	SchemaVersion     int
	DeploymentID      string
	ReleaseID         string
	ManagedGeneration int
	Content           adminapi.ManagedDraftContent
	// Only historical v4 material may carry its original Starter descriptions.
	V4Starters        []clientapi.AssistantStarterDefinitionV4
	PublishedAt       time.Time
	PublishedByUserID string
}

type snapshotDescriptor struct {
	DeploymentID      string                                `json:"deploymentId"`
	SchemaVersion     int                                   `json:"schemaVersion"`
	ManagedGeneration int                                   `json:"managedGeneration"`
	ReleaseID         string                                `json:"releaseId"`
	Providers         []clientapi.ProviderDefinition        `json:"providers"`
	Models            []clientapi.ModelDefinition           `json:"models"`
	ImageGenerators   []clientapi.ImageGenerationDefinition `json:"imageGenerators"`
	TTS               []clientapi.TtsDefinition             `json:"tts"`
	ASR               []clientapi.AsrDefinition             `json:"asr"`
	MCP               any                                   `json:"mcp"`
	Policy            clientapi.ManagedPolicy               `json:"policy"`
	Metadata          snapshotMetadata                      `json:"metadata"`
	Assistants        any                                   `json:"assistants"`
	Starters          []SnapshotStarter                     `json:"starters"`
}

// Snapshot is the compiler's versioned known-field projection. Downloaded wire
// extensions are ignored; v4 descriptions remain in this historical adapter.
type Snapshot struct {
	clientapi.ManagedSnapshot
	Starters     []SnapshotStarter                        `json:"starters"`
	V4Assistants []clientapi.ManagedAssistantDefinitionV4 `json:"-"`
}

type SnapshotStarter struct {
	clientapi.AssistantStarterDefinition
	Description *string `json:"description,omitempty"`
}

func (s Snapshot) MarshalJSON() ([]byte, error) {
	type plain Snapshot
	raw, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if s.SchemaVersion == 4 {
		fields["mcp"], err = json.Marshal(v4Mcp(s.Mcp))
		if err != nil {
			return nil, err
		}
		fields["assistants"], err = json.Marshal(s.V4Assistants)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

// MarshalJSON preserves the generated v4 field ordering used by canonical hashes.
func (s SnapshotStarter) MarshalJSON() ([]byte, error) {
	if s.OpeningSnapshot == nil {
		return json.Marshal(clientapi.AssistantStarterDefinitionV4{StarterId: s.StarterId, AssistantDefinitionId: s.AssistantDefinitionId, Title: s.Title, Prompt: s.Prompt, SortOrder: s.SortOrder, Enabled: s.Enabled, Description: s.Description})
	}
	if s.Description != nil {
		return nil, ErrInvalidDraft
	}
	return json.Marshal(s.AssistantStarterDefinition)
}

type snapshotMetadata struct {
	PublishedAt       time.Time `json:"publishedAt"`
	PublishedByUserID *string   `json:"publishedByUserId,omitempty"`
}

func (s *Service) CompileSnapshot(input SnapshotInput) (Snapshot, string, error) {
	input.Content = NormalizeManagedDraftContent(input.Content)
	version := input.SchemaVersion
	if version == 0 {
		version = CurrentSnapshotSchemaVersion
	}
	if err := validateStarterVersion(input.Content.Starters, version); err != nil {
		return Snapshot{}, "", err
	}
	if err := platformid.Validate(platformid.Deployment, input.DeploymentID); err != nil {
		return Snapshot{}, "", ErrInvalidDraft
	}
	if err := platformid.Validate(platformid.Release, input.ReleaseID); err != nil || input.ManagedGeneration < 1 {
		return Snapshot{}, "", ErrInvalidDraft
	}
	if err := validateCandidateIDs(input.Content); err != nil {
		return Snapshot{}, "", err
	}
	if version == 5 && len(mcpGovernanceIssues(input.Content)) > 0 {
		return Snapshot{}, "", ErrInvalidDraft
	}
	providers := make([]clientapi.ProviderDefinition, 0, len(input.Content.Providers))
	for _, value := range input.Content.Providers {
		providers = append(providers, clientapi.ProviderDefinition{ProviderId: value.ProviderId, DisplayName: value.DisplayName, ClientProtocol: clientapi.ProviderDefinitionClientProtocol(value.ClientProtocol), Enabled: value.Enabled})
	}
	models := make([]clientapi.ModelDefinition, 0, len(input.Content.Models))
	providerProtocols := make(map[string]adminapi.ProviderDefinitionClientProtocol, len(input.Content.Providers))
	for _, provider := range input.Content.Providers {
		providerProtocols[provider.ProviderId] = provider.ClientProtocol
	}
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
		publishedKey := EffectivePublishedModelKey(value)
		clientPath, err := ModelClientRuntimePath(providerProtocols[value.ProviderId], value.RuntimePath, value.UpstreamModelKey, publishedKey)
		if err != nil {
			return Snapshot{}, "", ErrInvalidDraft
		}
		models = append(models, clientapi.ModelDefinition{
			ModelId: value.ModelId, ProviderId: value.ProviderId, DisplayName: value.DisplayName,
			UpstreamModelKey: publishedKey, RuntimePath: clientPath, Enabled: value.Enabled,
			Capabilities: capabilities, InputModalities: inputs, OutputModalities: outputs,
		})
	}
	images := make([]clientapi.ImageGenerationDefinition, 0, len(imageGenerators(input.Content)))
	for _, value := range imageGenerators(input.Content) {
		sizes := append([]string(nil), value.AllowedSizes...)
		sort.Strings(sizes)
		images = append(images, clientapi.ImageGenerationDefinition{
			ImageId:             value.ImageId,
			DisplayName:         value.DisplayName,
			ClientProtocol:      clientapi.ImageGenerationDefinitionClientProtocol(value.ClientProtocol),
			UpstreamModelKey:    value.UpstreamModelKey,
			RuntimePath:         value.RuntimePath,
			MaxImagesPerRequest: value.MaxImagesPerRequest,
			AllowedSizes:        sizes,
			Enabled:             value.Enabled,
		})
	}
	tts := make([]clientapi.TtsDefinition, 0, len(input.Content.Tts))
	for _, value := range input.Content.Tts {
		tts = append(tts, clientapi.TtsDefinition{TtsId: value.TtsId, DisplayName: value.DisplayName, ClientProtocol: clientapi.TtsDefinitionClientProtocol(value.ClientProtocol), UpstreamModelKey: value.UpstreamModelKey, Voice: value.Voice, RuntimePath: value.RuntimePath, Enabled: value.Enabled, VoiceDesignPrompt: value.VoiceDesignPrompt, SpeechRate: value.SpeechRate, Pitch: value.Pitch})
	}
	asr := make([]clientapi.AsrDefinition, 0, len(input.Content.Asr))
	for _, value := range input.Content.Asr {
		asr = append(asr, clientapi.AsrDefinition{AsrId: value.AsrId, DisplayName: value.DisplayName, ClientProtocol: clientapi.AsrDefinitionClientProtocol(value.ClientProtocol), UpstreamModelKey: value.UpstreamModelKey, Language: value.Language, RuntimePath: value.RuntimePath, Enabled: value.Enabled,
			SampleRate: (*clientapi.AsrDefinitionSampleRate)(value.SampleRate), VadThreshold: value.VadThreshold, SilenceDurationMs: value.SilenceDurationMs, PrefixPaddingMs: value.PrefixPaddingMs, Prompt: value.Prompt})
	}
	mcp := make([]clientapi.McpDefinition, 0, len(input.Content.Mcp))
	for _, value := range input.Content.Mcp {
		grants := clientMcpGrants(value.AllowedTools)
		mode := clientapi.McpDefinitionToolAccessMode("")
		if value.ToolAccessMode != nil {
			mode = clientapi.McpDefinitionToolAccessMode(*value.ToolAccessMode)
		}
		mcp = append(mcp, clientapi.McpDefinition{ToolAccessMode: mode, AllowedTools: grants, McpServerId: value.McpServerId, DisplayName: value.DisplayName, ClientProtocol: clientapi.McpDefinitionClientProtocol(value.ClientProtocol), AuthOwnership: clientapi.McpDefinitionAuthOwnership(value.AuthOwnership), RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	// Compile assistants
	assistants := make([]clientapi.ManagedAssistantDefinition, 0, len(input.Content.Assistants))
	for _, a := range input.Content.Assistants {
		seed := make([]string, len(a.MemorySeed))
		for i, s := range a.MemorySeed {
			seed[i] = strings.TrimSpace(s)
		}
		bindings := clientAssistantBindings(a.McpBindings)
		assistants = append(assistants, clientapi.ManagedAssistantDefinition{
			AssistantDefinitionId: a.AssistantDefinitionId,
			DisplayName:           a.DisplayName,
			Description:           a.Description,
			SystemPrompt:          a.SystemPrompt,
			ModelId:               a.ModelId,
			MemorySeed:            seed,
			McpBindings:           bindings,
			Enabled:               a.Enabled,
		})
	}
	// Compile starters
	assistantSystems := make(map[string]string, len(input.Content.Assistants))
	for _, assistant := range input.Content.Assistants {
		assistantSystems[assistant.AssistantDefinitionId] = assistant.SystemPrompt
	}
	starters := make([]SnapshotStarter, 0, len(input.Content.Starters))
	for _, s := range input.Content.Starters {
		opening := toClientOpening(s.OpeningSnapshot)
		if input.SchemaVersion == 0 && opening != nil && strings.TrimSpace(opening.SystemPrompt) == "" {
			system, found := assistantSystems[s.AssistantDefinitionId]
			if !found {
				return Snapshot{}, "", ErrInvalidDraft
			}
			opening.SystemPrompt = system
		}
		starters = append(starters, SnapshotStarter{AssistantStarterDefinition: clientapi.AssistantStarterDefinition{
			StarterId:             s.StarterId,
			AssistantDefinitionId: s.AssistantDefinitionId,
			Title:                 s.Title,
			Prompt:                s.Prompt,
			SortOrder:             s.SortOrder,
			Enabled:               s.Enabled,
			OpeningSnapshot:       opening,
		}})
	}
	sort.Slice(assistants, func(i, j int) bool { return assistants[i].AssistantDefinitionId < assistants[j].AssistantDefinitionId })
	if version == 4 {
		for i := range starters {
			for _, old := range input.V4Starters {
				if old.StarterId == starters[i].StarterId {
					starters[i].Description = old.Description
				}
			}
		}
	}
	sort.Slice(starters, func(i, j int) bool { return starters[i].StarterId < starters[j].StarterId })
	sort.Slice(providers, func(i, j int) bool { return providers[i].ProviderId < providers[j].ProviderId })
	sort.Slice(models, func(i, j int) bool { return models[i].ModelId < models[j].ModelId })
	sort.Slice(images, func(i, j int) bool { return images[i].ImageId < images[j].ImageId })
	sort.Slice(tts, func(i, j int) bool { return tts[i].TtsId < tts[j].TtsId })
	sort.Slice(asr, func(i, j int) bool { return asr[i].AsrId < asr[j].AsrId })
	sort.Slice(mcp, func(i, j int) bool { return mcp[i].McpServerId < mcp[j].McpServerId })

	policy := clientapi.ManagedPolicy{
		PolicyId:                           input.Content.Policy.PolicyId,
		AllowLocalProviders:                input.Content.Policy.AllowLocalProviders,
		AllowLocalTts:                      input.Content.Policy.AllowLocalTts,
		AllowLocalAsr:                      input.Content.Policy.AllowLocalAsr,
		AllowLocalMcp:                      input.Content.Policy.AllowLocalMcp,
		AllowLocalAssistants:               input.Content.Policy.AllowLocalAssistants,
		DefaultModelId:                     input.Content.Policy.DefaultModelId,
		DefaultFastModelId:                 input.Content.Policy.DefaultFastModelId,
		DefaultTitleModelId:                input.Content.Policy.DefaultTitleModelId,
		DefaultAttachmentInspectionModelId: input.Content.Policy.DefaultAttachmentInspectionModelId,
		DefaultSuggestionModelId:           input.Content.Policy.DefaultSuggestionModelId,
		DefaultCompressModelId:             input.Content.Policy.DefaultCompressModelId,
		DefaultTtsId:                       input.Content.Policy.DefaultTtsId,
		DefaultAsrId:                       input.Content.Policy.DefaultAsrId,
		DefaultImageGenerationId:           input.Content.Policy.DefaultImageGenerationId,
		DefaultAssistantId:                 input.Content.Policy.DefaultAssistantId,
	}
	var publishedBy *string
	if input.PublishedByUserID != "" {
		value := input.PublishedByUserID
		publishedBy = &value
	}
	metadata := snapshotMetadata{PublishedAt: input.PublishedAt.UTC(), PublishedByUserID: publishedBy}
	var v4Assistants []clientapi.ManagedAssistantDefinitionV4
	if version == 4 {
		v4Assistants = make([]clientapi.ManagedAssistantDefinitionV4, 0, len(assistants))
		for _, a := range assistants {
			ids := []clientapi.McpServerId{}
			for _, original := range input.Content.Assistants {
				if original.AssistantDefinitionId == a.AssistantDefinitionId {
					for _, id := range original.McpServerIds {
						ids = append(ids, id)
					}
				}
			}
			sort.Strings(ids)
			v4Assistants = append(v4Assistants, clientapi.ManagedAssistantDefinitionV4{AssistantDefinitionId: a.AssistantDefinitionId, DisplayName: a.DisplayName, Description: a.Description, SystemPrompt: a.SystemPrompt, ModelId: a.ModelId, MemorySeed: a.MemorySeed, McpServerIds: ids, Enabled: a.Enabled})
		}
	}
	descriptor := snapshotDescriptor{
		DeploymentID: input.DeploymentID, SchemaVersion: version, ManagedGeneration: input.ManagedGeneration,
		ReleaseID: input.ReleaseID, Providers: providers, Models: models, ImageGenerators: images, TTS: tts, ASR: asr, MCP: mcp, Policy: policy, Metadata: metadata,
		Assistants: assistants, Starters: starters,
	}
	if version == 4 {
		descriptor.MCP = v4Mcp(mcp)
		descriptor.Assistants = v4Assistants
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return Snapshot{}, "", err
	}
	sum := sha256.Sum256(payload)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	var snapshot Snapshot
	snapshot.DeploymentId = input.DeploymentID
	snapshot.SchemaVersion = clientapi.ManagedSnapshotSchemaVersion(version)
	snapshot.ManagedGeneration = input.ManagedGeneration
	snapshot.ReleaseId = input.ReleaseID
	snapshot.SnapshotHash = hash
	snapshot.Providers = providers
	snapshot.Models = models
	snapshot.ImageGenerators = &images
	snapshot.Tts = tts
	snapshot.Asr = asr
	snapshot.Mcp = mcp
	snapshot.Policy = policy
	snapshot.Metadata.PublishedAt = metadata.PublishedAt
	snapshot.Metadata.PublishedByUserId = metadata.PublishedByUserID
	snapshot.Assistants = assistants
	snapshot.Starters = starters
	snapshot.V4Assistants = v4Assistants
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return Snapshot{}, "", err
	}
	if len(encoded) > MaxSnapshotBytes {
		return Snapshot{}, "", ErrSnapshotTooLarge
	}
	return snapshot, hash, nil
}

// HashSnapshot recomputes the canonical hash of a decoded managed snapshot.
// It is shared by contract tests and downstream verification tooling so the
// canonical descriptor is not reimplemented outside the capability boundary.
func HashSnapshot(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", err
	}
	if snapshot.SchemaVersion != 4 && snapshot.SchemaVersion != 5 {
		return "", ErrInvalidDraft
	}
	for _, starter := range snapshot.Starters {
		if (snapshot.SchemaVersion == 4 && starter.OpeningSnapshot != nil) || (snapshot.SchemaVersion == 5 && starter.OpeningSnapshot == nil) {
			return "", ErrInvalidDraft
		}
		if err := validateDraftOpening(toAdminOpening(starter.OpeningSnapshot)); err != nil {
			return "", err
		}
	}
	metadata := snapshotMetadata{PublishedAt: snapshot.Metadata.PublishedAt}
	if snapshot.Metadata.PublishedByUserId != nil {
		value := string(*snapshot.Metadata.PublishedByUserId)
		metadata.PublishedByUserID = &value
	}
	descriptor := snapshotDescriptor{
		DeploymentID: string(snapshot.DeploymentId), SchemaVersion: int(snapshot.SchemaVersion), ManagedGeneration: snapshot.ManagedGeneration,
		ReleaseID: string(snapshot.ReleaseId), Providers: snapshot.Providers, Models: snapshot.Models, ImageGenerators: clientImages(snapshot.ImageGenerators), TTS: snapshot.Tts, ASR: snapshot.Asr, MCP: snapshot.Mcp,
		Policy: snapshot.Policy, Metadata: metadata,
	}
	descriptor.Assistants = snapshot.Assistants
	if snapshot.SchemaVersion == 4 {
		descriptor.MCP = v4Mcp(snapshot.Mcp)
		descriptor.Assistants = snapshot.V4Assistants
	}
	descriptor.Starters = snapshot.Starters
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func clientImages(values *[]clientapi.ImageGenerationDefinition) []clientapi.ImageGenerationDefinition {
	if values == nil {
		return []clientapi.ImageGenerationDefinition{}
	}
	return *values
}

func projectionToAdminAssistants(src []clientapi.ManagedAssistantDefinition) []adminapi.ManagedAssistantDefinition {
	dst := make([]adminapi.ManagedAssistantDefinition, len(src))
	for i, a := range src {
		seed := make([]string, len(a.MemorySeed))
		for j, s := range a.MemorySeed {
			seed[j] = string(s)
		}
		bindings := make([]adminapi.AssistantMcpBinding, len(a.McpBindings))
		for j, m := range a.McpBindings {
			bindings[j] = adminapi.AssistantMcpBinding{McpServerId: m.McpServerId, ToolSelection: adminapi.AssistantMcpBindingToolSelection(m.ToolSelection), ToolNames: append([]string{}, m.ToolNames...)}
		}
		dst[i] = adminapi.ManagedAssistantDefinition{
			AssistantDefinitionId: adminapi.AssistantDefinitionId(a.AssistantDefinitionId),
			DisplayName:           a.DisplayName,
			Description:           a.Description,
			SystemPrompt:          a.SystemPrompt,
			ModelId:               adminapi.ModelId(a.ModelId),
			MemorySeed:            seed,
			McpBindings:           &bindings,
			Enabled:               a.Enabled,
		}
	}
	return dst
}

func projectionToAdminStarters(src []SnapshotStarter) []adminapi.AssistantStarterDefinition {
	dst := make([]adminapi.AssistantStarterDefinition, len(src))
	for i, s := range src {
		dst[i] = adminapi.AssistantStarterDefinition{
			StarterId:             adminapi.StarterId(s.StarterId),
			AssistantDefinitionId: adminapi.AssistantDefinitionId(s.AssistantDefinitionId),
			Title:                 s.Title,
			Prompt:                s.Prompt,
			SortOrder:             s.SortOrder,
			Enabled:               s.Enabled,
			OpeningSnapshot:       toAdminOpening(s.OpeningSnapshot),
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
		dst[i].PublishedModelKey = &dst[i].UpstreamModelKey
	}
	return dst
}

func projectionToAdminImages(src *[]clientapi.ImageGenerationDefinition) []adminapi.ImageGenerationDefinition {
	values := clientImages(src)
	dst := make([]adminapi.ImageGenerationDefinition, len(values))
	for i, value := range values {
		dst[i] = adminapi.ImageGenerationDefinition{
			ImageId:             value.ImageId,
			DisplayName:         value.DisplayName,
			ClientProtocol:      adminapi.ImageGenerationDefinitionClientProtocol(value.ClientProtocol),
			UpstreamModelKey:    value.UpstreamModelKey,
			RuntimePath:         value.RuntimePath,
			MaxImagesPerRequest: value.MaxImagesPerRequest,
			AllowedSizes:        append([]string(nil), value.AllowedSizes...),
			Enabled:             value.Enabled,
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
			VoiceDesignPrompt: v.VoiceDesignPrompt, SpeechRate: v.SpeechRate, Pitch: v.Pitch,
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
			SampleRate: (*adminapi.AsrDefinitionSampleRate)(v.SampleRate), VadThreshold: v.VadThreshold, SilenceDurationMs: v.SilenceDurationMs, PrefixPaddingMs: v.PrefixPaddingMs, Prompt: v.Prompt,
			RuntimePath: v.RuntimePath, Enabled: v.Enabled,
		}
	}
	return dst
}

// projectionToAdminMcp converts clientapi projection back to adminapi types for preview.
func projectionToAdminMcp(src []clientapi.McpDefinition) []adminapi.McpDefinition {
	dst := make([]adminapi.McpDefinition, len(src))
	for i, v := range src {
		mode := adminapi.McpDefinitionToolAccessMode(v.ToolAccessMode)
		dst[i] = adminapi.McpDefinition{
			McpServerId: v.McpServerId, DisplayName: v.DisplayName,
			ToolAccessMode: &mode,
			AllowedTools:   adminMcpGrants(v.AllowedTools),
			ClientProtocol: adminapi.McpDefinitionClientProtocol(string(v.ClientProtocol)),
			AuthOwnership:  adminapi.McpDefinitionAuthOwnership(string(v.AuthOwnership)),
			RuntimePath:    v.RuntimePath, Enabled: v.Enabled,
		}
	}
	return dst
}
