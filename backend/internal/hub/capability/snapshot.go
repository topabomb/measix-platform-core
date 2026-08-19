package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/adminapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/clientapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
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
	DeploymentID      string                           `json:"deploymentId"`
	SchemaVersion     int                              `json:"schemaVersion"`
	ManagedGeneration int                              `json:"managedGeneration"`
	ReleaseID         string                           `json:"releaseId"`
	Providers         []clientapi.ProviderDefinition   `json:"providers"`
	Models            []clientapi.ModelDefinition      `json:"models"`
	TTS               []clientapi.TtsDefinition        `json:"tts"`
	ASR               []clientapi.AsrDefinition        `json:"asr"`
	MCP               []clientapi.McpDefinition        `json:"mcp"`
	Policy            clientapi.ManagedPolicy          `json:"policy"`
	Metadata          snapshotMetadata                 `json:"metadata"`
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
		providers = append(providers, clientapi.ProviderDefinition{ProviderId: value.ProviderId, DisplayName: value.DisplayName, ClientProtocol: value.ClientProtocol, Enabled: value.Enabled})
	}
	models := make([]clientapi.ModelDefinition, 0, len(input.Content.Models))
	for _, value := range input.Content.Models {
		capabilities := append([]string(nil), value.Capabilities...)
		inputs := append([]string(nil), value.InputModalities...)
		outputs := append([]string(nil), value.OutputModalities...)
		sort.Strings(capabilities)
		sort.Strings(inputs)
		sort.Strings(outputs)
		models = append(models, clientapi.ModelDefinition{
			ModelId: value.ModelId, ProviderId: value.ProviderId, DisplayName: value.DisplayName,
			UpstreamModelKey: value.UpstreamModelKey, RuntimePath: value.RuntimePath, Enabled: value.Enabled,
			Capabilities: capabilities, InputModalities: inputs, OutputModalities: outputs,
		})
	}
	tts := make([]clientapi.TtsDefinition, 0, len(input.Content.Tts))
	for _, value := range input.Content.Tts {
		tts = append(tts, clientapi.TtsDefinition{TtsId: value.TtsId, DisplayName: value.DisplayName, ClientProtocol: value.ClientProtocol, UpstreamModelKey: value.UpstreamModelKey, RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	asr := make([]clientapi.AsrDefinition, 0, len(input.Content.Asr))
	for _, value := range input.Content.Asr {
		asr = append(asr, clientapi.AsrDefinition{AsrId: value.AsrId, DisplayName: value.DisplayName, ClientProtocol: value.ClientProtocol, UpstreamModelKey: value.UpstreamModelKey, RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	mcp := make([]clientapi.McpDefinition, 0, len(input.Content.Mcp))
	for _, value := range input.Content.Mcp {
		mcp = append(mcp, clientapi.McpDefinition{McpServerId: value.McpServerId, DisplayName: value.DisplayName, ClientProtocol: clientapi.McpDefinitionClientProtocol(value.ClientProtocol), RuntimePath: value.RuntimePath, Enabled: value.Enabled})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].ProviderId < providers[j].ProviderId })
	sort.Slice(models, func(i, j int) bool { return models[i].ModelId < models[j].ModelId })
	sort.Slice(tts, func(i, j int) bool { return tts[i].TtsId < tts[j].TtsId })
	sort.Slice(asr, func(i, j int) bool { return asr[i].AsrId < asr[j].AsrId })
	sort.Slice(mcp, func(i, j int) bool { return mcp[i].McpServerId < mcp[j].McpServerId })

	policy := clientapi.ManagedPolicy{
		PolicyId: input.Content.Policy.PolicyId,
		AllowLocalProviders: input.Content.Policy.AllowLocalProviders,
		AllowLocalTts: input.Content.Policy.AllowLocalTts,
		AllowLocalAsr: input.Content.Policy.AllowLocalAsr,
		AllowLocalMcp: input.Content.Policy.AllowLocalMcp,
		DefaultModelId: input.Content.Policy.DefaultModelId,
		DefaultTtsId: input.Content.Policy.DefaultTtsId,
		DefaultAsrId: input.Content.Policy.DefaultAsrId,
	}
	var publishedBy *string
	if input.PublishedByUserID != "" {
		value := input.PublishedByUserID
		publishedBy = &value
	}
	metadata := snapshotMetadata{PublishedAt: input.PublishedAt.UTC(), PublishedByUserID: publishedBy}
	descriptor := snapshotDescriptor{
		DeploymentID: input.DeploymentID, SchemaVersion: 1, ManagedGeneration: input.ManagedGeneration,
		ReleaseID: input.ReleaseID, Providers: providers, Models: models, TTS: tts, ASR: asr, MCP: mcp, Policy: policy, Metadata: metadata,
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return clientapi.ManagedSnapshot{}, "", err
	}
	sum := sha256.Sum256(payload)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	var snapshot clientapi.ManagedSnapshot
	snapshot.DeploymentId = input.DeploymentID
	snapshot.SchemaVersion = clientapi.ManagedSnapshotSchemaVersionN1
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
	return snapshot, hash, nil
}
