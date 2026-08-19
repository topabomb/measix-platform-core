package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/manageddraft"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrRevisionConflict = errors.New("draft revision conflict")
	ErrInvalidDraft     = errors.New("invalid managed draft")
)

type DraftView struct {
	DraftID       string
	DraftRevision int
	Content       adminapi.ManagedDraftContent
}

type ValidationResult struct {
	Valid    bool
	Errors   []adminapi.ValidationIssue
	Warnings []adminapi.ValidationIssue
}

type ReleaseView struct {
	ReleaseID         string
	ManagedGeneration int
	SnapshotHash      string
	Status            string
	CreatedAt         time.Time
}

type Service struct {
	Client *ent.Client
	Now    func() time.Time
}

func NewService(client *ent.Client) *Service {
	return &Service{Client: client, Now: time.Now}
}

func (s *Service) GetDraft(ctx context.Context) (DraftView, error) {
	row, err := s.Client.ManagedDraft.Query().Only(ctx)
	if err != nil {
		return DraftView{}, err
	}
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(row.ContentJSON, &content); err != nil {
		return DraftView{}, err
	}
	return DraftView{DraftID: row.ID, DraftRevision: int(row.DraftRevision), Content: content}, nil
}

func (s *Service) PutDraft(ctx context.Context, updatedBy string, expectedRevision int, content adminapi.ManagedDraftContent) (DraftView, error) {
	if err := validateCandidateIDs(content); err != nil {
		return DraftView{}, err
	}
	row, err := s.Client.ManagedDraft.Query().Only(ctx)
	if err != nil {
		return DraftView{}, err
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return DraftView{}, err
	}
	nextRevision := expectedRevision + 1
	n, err := s.Client.ManagedDraft.Update().Where(
		manageddraft.IDEQ(row.ID),
		manageddraft.DraftRevisionEQ(int64(expectedRevision)),
	).
		SetDraftRevision(int64(nextRevision)).
		SetContentJSON(contentJSON).
		SetUpdatedByUserID(updatedBy).
		SetUpdatedAt(s.Now().UTC()).
		Save(ctx)
	if err != nil {
		return DraftView{}, err
	}
	if n != 1 {
		return DraftView{}, ErrRevisionConflict
	}
	return DraftView{DraftID: row.ID, DraftRevision: nextRevision, Content: content}, nil
}

func (s *Service) ValidateDraft(ctx context.Context, expectedRevision int) (ValidationResult, error) {
	draft, err := s.GetDraft(ctx)
	if err != nil {
		return ValidationResult{}, err
	}
	if draft.DraftRevision != expectedRevision {
		return ValidationResult{}, ErrRevisionConflict
	}
	return s.validateContent(ctx, draft.Content), nil
}

func (s *Service) StageRelease(ctx context.Context, createdBy string, expectedDraftRevision int) (ReleaseView, error) {
	draft, err := s.GetDraft(ctx)
	if err != nil {
		return ReleaseView{}, err
	}
	if draft.DraftRevision != expectedDraftRevision {
		return ReleaseView{}, ErrRevisionConflict
	}
	validation := s.validateContent(ctx, draft.Content)
	if !validation.Valid {
		return ReleaseView{}, ErrInvalidDraft
	}
	generation := 1
	latest, err := s.Client.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).First(ctx)
	if err == nil {
		generation = int(latest.ManagedGeneration) + 1
	} else if !ent.IsNotFound(err) {
		return ReleaseView{}, err
	}
	deployment, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return ReleaseView{}, err
	}
	releaseID := platformid.New(platformid.Release)
	now := s.Now().UTC()
	snapshot, hash, err := s.CompileSnapshot(SnapshotInput{
		DeploymentID:      deployment.ID,
		ReleaseID:         releaseID,
		ManagedGeneration: generation,
		Content:           draft.Content,
		PublishedAt:       now,
		PublishedByUserID: createdBy,
	})
	if err != nil {
		return ReleaseView{}, err
	}
	releaseJSON, err := json.Marshal(draft.Content)
	if err != nil {
		return ReleaseView{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return ReleaseView{}, err
	}
	_, err = s.Client.ManagedRelease.Create().
		SetID(releaseID).
		SetManagedGeneration(int64(generation)).
		SetStatus("STAGED").
		SetReleaseContentJSON(releaseJSON).
		SetSnapshotSchemaVersion(1).
		SetSnapshotJSON(snapshotJSON).
		SetSnapshotHash(hash).
		SetSourceDraftRevision(int64(draft.DraftRevision)).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return ReleaseView{}, err
	}
	return ReleaseView{ReleaseID: releaseID, ManagedGeneration: generation, SnapshotHash: hash, Status: "STAGED", CreatedAt: now}, nil
}

func (s *Service) validateContent(ctx context.Context, content adminapi.ManagedDraftContent) ValidationResult {
	result := ValidationResult{Errors: []adminapi.ValidationIssue{}, Warnings: []adminapi.ValidationIssue{}}
	addError := func(code, path, message string) {
		result.Errors = append(result.Errors, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.ERROR})
	}
	addWarning := func(code, path, message string) {
		result.Warnings = append(result.Warnings, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.WARNING})
	}
	if err := validateCandidateIDs(content); err != nil {
		addError("invalid_candidate_id", "$", err.Error())
		result.Valid = false
		return result
	}

	providers := make(map[string]adminapi.ProviderDefinition, len(content.Providers))
	for _, provider := range content.Providers {
		providers[provider.ProviderId] = provider
	}
	resources := map[string]bool{}
	for i, model := range content.Models {
		resources[model.ModelId] = model.Enabled
		if _, ok := providers[model.ProviderId]; !ok {
			addError("missing_provider", fmt.Sprintf("models[%d].providerId", i), "model references an unknown provider")
		}
		if !validRuntimePath(model.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("models[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}
	for i, value := range content.Tts {
		resources[value.TtsId] = value.Enabled
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("tts[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}
	for i, value := range content.Asr {
		resources[value.AsrId] = value.Enabled
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("asr[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}
	for i, value := range content.Mcp {
		resources[value.McpServerId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("mcp[%d].clientProtocol", i), "unsupported MCP client protocol")
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("mcp[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}

	bound := map[string]bool{}
	for i, binding := range content.Bindings {
		path := fmt.Sprintf("bindings[%d]", i)
		if _, ok := resources[binding.ResourceId]; !ok {
			addError("missing_resource", path+".resourceId", "binding references an unknown runtime resource")
		} else if bound[binding.ResourceId] {
			addError("duplicate_binding", path+".resourceId", "resource has more than one runtime binding")
		} else {
			bound[binding.ResourceId] = true
		}
		if !binding.TransportPolicy.Valid() {
			addError("invalid_transport_policy", path+".transportPolicy", "unsupported transport policy")
		}
		if len(binding.AllowedMethods) == 0 {
			addError("missing_method_policy", path+".allowedMethods", "at least one HTTP method is required")
		}
		for _, method := range binding.AllowedMethods {
			if !validMethod(method) {
				addError("invalid_method", path+".allowedMethods", "method policy contains an unsupported method")
			}
		}
		if len(binding.AllowedPathPrefixes) == 0 {
			addError("missing_path_policy", path+".allowedPathPrefixes", "at least one path prefix is required")
		}
		for _, prefix := range binding.AllowedPathPrefixes {
			if !validRuntimePath(prefix) {
				addError("invalid_path_prefix", path+".allowedPathPrefixes", "path prefix must be normalized and absolute")
			}
		}
		row, err := s.Client.Upstream.Get(ctx, binding.UpstreamId)
		if ent.IsNotFound(err) {
			addError("missing_upstream", path+".upstreamId", "binding references an unknown upstream")
			continue
		}
		if err != nil {
			addError("upstream_lookup_failed", path+".upstreamId", err.Error())
			continue
		}
		if row.Status == "DISABLED" {
			addError("upstream_disabled", path+".upstreamId", "binding references a disabled upstream")
		}
		if row.Status == "INACTIVE" {
			addWarning("upstream_not_active", path+".upstreamId", "upstream has not yet been applied to Runtime Relay")
		}
		config, err := upstream.LoadCandidateConfig(ctx, s.Client, row.ID)
		if err != nil || upstream.ValidateConfig(ctx, s.Client, config) != nil {
			addError("invalid_upstream_config", path+".upstreamId", "upstream candidate config or SecretRef is invalid")
		}
	}
	for resourceID, enabled := range resources {
		if enabled && !bound[resourceID] {
			addError("missing_binding", "bindings", "enabled resource has no runtime binding")
		}
	}
	if content.Policy.DefaultModelId != nil {
		if _, ok := resources[*content.Policy.DefaultModelId]; !ok {
			addError("invalid_default_model", "policy.defaultModelId", "default model is not defined")
		}
	}
	if content.Policy.DefaultTtsId != nil {
		if _, ok := resources[*content.Policy.DefaultTtsId]; !ok {
			addError("invalid_default_tts", "policy.defaultTtsId", "default TTS is not defined")
		}
	}
	if content.Policy.DefaultAsrId != nil {
		if _, ok := resources[*content.Policy.DefaultAsrId]; !ok {
			addError("invalid_default_asr", "policy.defaultAsrId", "default ASR is not defined")
		}
	}
	sort.Slice(result.Errors, func(i, j int) bool {
		if result.Errors[i].Path == result.Errors[j].Path {
			return result.Errors[i].Code < result.Errors[j].Code
		}
		return result.Errors[i].Path < result.Errors[j].Path
	})
	sort.Slice(result.Warnings, func(i, j int) bool {
		if result.Warnings[i].Path == result.Warnings[j].Path {
			return result.Warnings[i].Code < result.Warnings[j].Code
		}
		return result.Warnings[i].Path < result.Warnings[j].Path
	})
	result.Valid = len(result.Errors) == 0
	return result
}

func validateCandidateIDs(content adminapi.ManagedDraftContent) error {
	seen := map[string]struct{}{}
	check := func(kind platformid.Kind, value, label string) error {
		if err := platformid.Validate(kind, value); err != nil {
			return fmt.Errorf("%s has invalid %s id", label, kind)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate stable id %s", value)
		}
		seen[value] = struct{}{}
		return nil
	}
	for i, value := range content.Providers {
		if err := check(platformid.Provider, value.ProviderId, fmt.Sprintf("providers[%d]", i)); err != nil {
			return err
		}
	}
	for i, value := range content.Models {
		if err := check(platformid.Model, value.ModelId, fmt.Sprintf("models[%d]", i)); err != nil {
			return err
		}
	}
	for i, value := range content.Tts {
		if err := check(platformid.TTS, value.TtsId, fmt.Sprintf("tts[%d]", i)); err != nil {
			return err
		}
	}
	for i, value := range content.Asr {
		if err := check(platformid.ASR, value.AsrId, fmt.Sprintf("asr[%d]", i)); err != nil {
			return err
		}
	}
	for i, value := range content.Mcp {
		if err := check(platformid.MCP, value.McpServerId, fmt.Sprintf("mcp[%d]", i)); err != nil {
			return err
		}
	}
	if err := check(platformid.Policy, content.Policy.PolicyId, "policy"); err != nil {
		return err
	}
	for i, value := range content.Bindings {
		if err := check(platformid.Route, value.RuntimeRouteId, fmt.Sprintf("bindings[%d]", i)); err != nil {
			return err
		}
		if err := platformid.Validate(platformid.Upstream, value.UpstreamId); err != nil {
			return fmt.Errorf("bindings[%d] has invalid upstream id", i)
		}
	}
	return nil
}

func validRuntimePath(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.Contains(value, "..") && !strings.Contains(value, "//")
}

func validMethod(value string) bool {
	switch value {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}
