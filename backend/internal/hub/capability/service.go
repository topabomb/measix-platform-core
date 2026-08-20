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
	"measix/platform/ent/activation"
	"measix/platform/ent/manageddraft"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrRevisionConflict = errors.New("draft revision conflict")
	ErrInvalidDraft     = errors.New("invalid managed draft")
	ErrReleaseNotFound  = errors.New("managed release not found")
)

type DraftView struct {
	DraftID       string
	DraftRevision int
	Content       adminapi.ManagedDraftContent
}

type DraftPreview struct {
	DraftRevision int
	SnapshotHash  string
	Providers     []adminapi.ProviderDefinition
	Models        []adminapi.ModelDefinition
	TTS           []adminapi.TtsDefinition
	ASR           []adminapi.AsrDefinition
	MCP           []adminapi.McpDefinition
	Policy        adminapi.ManagedPolicy
}

type ValidationResult struct {
	Valid    bool
	Errors   []adminapi.ValidationIssue
	Warnings []adminapi.ValidationIssue
}

type ReleaseView struct {
	ReleaseID           string
	ManagedGeneration   int
	SnapshotHash        string
	Status              string
	CreatedAt           time.Time
	SourceDraftRevision int
	PublishedBy         string
	DiffSummary         adminapi.DiffSummary
	ActivationHistory   []adminapi.ActivationSummary
}

// releaseContentDiff computes the Added / Changed / Removed summary between two
// draft contents (current vs previous). A definition is keyed by its canonical
// ID; equality is structural (a full JSON compare of the definition body).
func releaseContentDiff(current, previous *adminapi.ManagedDraftContent) adminapi.DiffSummary {
	byKind := map[adminapi.ReleaseDiffKind]*adminapi.ResourceDiff{}
	ensure := func(kind adminapi.ReleaseDiffKind) *adminapi.ResourceDiff {
		if byKind[kind] == nil {
			byKind[kind] = &adminapi.ResourceDiff{Kind: kind}
		}
		return byKind[kind]
	}
	prev := map[string]string{} // id -> definition hash in the previous release
	prevKinds := map[string]adminapi.ReleaseDiffKind{}
	if previous != nil {
		for _, p := range previous.Providers {
			prev[p.ProviderId] = defHash(p)
			prevKinds[p.ProviderId] = adminapi.ReleaseDiffKindPROVIDER
		}
		for _, m := range previous.Models {
			prev[m.ModelId] = defHash(m)
			prevKinds[m.ModelId] = adminapi.ReleaseDiffKindMODEL
		}
		for _, t := range previous.Tts {
			prev[t.TtsId] = defHash(t)
			prevKinds[t.TtsId] = adminapi.ReleaseDiffKindTTS
		}
		for _, a := range previous.Asr {
			prev[a.AsrId] = defHash(a)
			prevKinds[a.AsrId] = adminapi.ReleaseDiffKindASR
		}
		for _, m := range previous.Mcp {
			prev[m.McpServerId] = defHash(m)
			prevKinds[m.McpServerId] = adminapi.ReleaseDiffKindMCP
		}
	}

	var summary adminapi.DiffSummary
	if current == nil {
		return summary
	}
	count := func(kind adminapi.ReleaseDiffKind, id string, oldHash string, exists bool) {
		d := ensure(kind)
		if !exists {
			d.Added++
			return
		}
		if oldHash != currentHash(current, id) {
			d.Changed++
		}
	}
	process := func(kind adminapi.ReleaseDiffKind, id string) {
		old, exists := prev[id]
		count(kind, id, old, exists)
		delete(prev, id)
		delete(prevKinds, id)
	}
	for _, p := range current.Providers {
		process(adminapi.ReleaseDiffKindPROVIDER, p.ProviderId)
	}
	for _, m := range current.Models {
		process(adminapi.ReleaseDiffKindMODEL, m.ModelId)
	}
	for _, t := range current.Tts {
		process(adminapi.ReleaseDiffKindTTS, t.TtsId)
	}
	for _, a := range current.Asr {
		process(adminapi.ReleaseDiffKindASR, a.AsrId)
	}
	for _, m := range current.Mcp {
		process(adminapi.ReleaseDiffKindMCP, m.McpServerId)
	}
	for _, kind := range prevKinds {
		ensure(kind).Removed++
	}

	kinds := []adminapi.ReleaseDiffKind{
		adminapi.ReleaseDiffKindPROVIDER,
		adminapi.ReleaseDiffKindMODEL,
		adminapi.ReleaseDiffKindTTS,
		adminapi.ReleaseDiffKindASR,
		adminapi.ReleaseDiffKindMCP,
		adminapi.ReleaseDiffKindPOLICY,
	}
	var details []adminapi.ResourceDiff
	for _, kind := range kinds {
		if d := byKind[kind]; d != nil {
			summary.Added += d.Added
			summary.Changed += d.Changed
			summary.Removed += d.Removed
			if d.Added > 0 || d.Changed > 0 || d.Removed > 0 {
				details = append(details, *d)
			}
		}
	}
	if len(details) > 0 {
		summary.Details = &details
	}
	return summary
}

// currentHash returns the canonical JSON hash of the definition with the given
// ID within the current content, or "" if not present.
func currentHash(content *adminapi.ManagedDraftContent, id string) string {
	for _, p := range content.Providers {
		if p.ProviderId == id {
			return defHash(p)
		}
	}
	for _, m := range content.Models {
		if m.ModelId == id {
			return defHash(m)
		}
	}
	for _, t := range content.Tts {
		if t.TtsId == id {
			return defHash(t)
		}
	}
	for _, a := range content.Asr {
		if a.AsrId == id {
			return defHash(a)
		}
	}
	for _, m := range content.Mcp {
		if m.McpServerId == id {
			return defHash(m)
		}
	}
	return ""
}

// defHash returns a canonical JSON string for a definition for structural diffing.
func defHash(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

type SnapshotView struct {
	JSON []byte
	Hash string
}

func (s *Service) ListReleases(ctx context.Context, limit int) ([]ReleaseView, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Client.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]ReleaseView, 0, len(rows))
	for i, row := range rows {
		views = append(views, s.buildReleaseView(ctx, row, previousReleaseRow(rows, i)))
	}
	return views, nil
}

func (s *Service) GetRelease(ctx context.Context, releaseID string) (ReleaseView, error) {
	row, err := s.Client.ManagedRelease.Get(ctx, releaseID)
	if ent.IsNotFound(err) {
		return ReleaseView{}, ErrReleaseNotFound
	}
	if err != nil {
		return ReleaseView{}, err
	}
	prev, err := s.previousRelease(ctx, row)
	if err != nil {
		return ReleaseView{}, err
	}
	return s.buildReleaseView(ctx, row, prev), nil
}

// previousReleaseRow returns the next row in the descending-generation list (the
// chronologically prior release), or nil if this is the first release.
func previousReleaseRow(rows []*ent.ManagedRelease, index int) *ent.ManagedRelease {
	if index >= len(rows)-1 {
		return nil
	}
	return rows[index+1]
}

func (s *Service) previousRelease(ctx context.Context, row *ent.ManagedRelease) (*ent.ManagedRelease, error) {
	prev, err := s.Client.ManagedRelease.Query().Where(
		managedrelease.ManagedGenerationLT(row.ManagedGeneration),
	).Order(ent.Desc(managedrelease.FieldManagedGeneration)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return prev, nil
}

func (s *Service) buildReleaseView(ctx context.Context, row, prev *ent.ManagedRelease) ReleaseView {
	var current, previous *adminapi.ManagedDraftContent
	if len(row.ReleaseContentJSON) > 0 {
		var c adminapi.ManagedDraftContent
		if err := json.Unmarshal(row.ReleaseContentJSON, &c); err == nil {
			current = &c
		}
	}
	if prev != nil && len(prev.ReleaseContentJSON) > 0 {
		var p adminapi.ManagedDraftContent
		if err := json.Unmarshal(prev.ReleaseContentJSON, &p); err == nil {
			previous = &p
		}
	}
	diff := releaseContentDiff(current, previous)
	history := s.activationHistory(ctx, int(row.ManagedGeneration))
	return ReleaseView{
		ReleaseID:           row.ID,
		ManagedGeneration:   int(row.ManagedGeneration),
		SnapshotHash:        row.SnapshotHash,
		Status:              row.Status,
		CreatedAt:           row.CreatedAt,
		SourceDraftRevision: int(row.SourceDraftRevision),
		PublishedBy:         row.CreatedByUserID,
		DiffSummary:         diff,
		ActivationHistory:   history,
	}
}

// activationHistory returns the ordered activation attempts targeting the given
// managed generation (release), newest first.
func (s *Service) activationHistory(ctx context.Context, generation int) []adminapi.ActivationSummary {
	rows, err := s.Client.Activation.Query().Where(
		activation.TargetGenerationEQ(int64(generation)),
	).Order(ent.Desc(activation.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil
	}
	out := make([]adminapi.ActivationSummary, 0, len(rows))
	for _, a := range rows {
		item := adminapi.ActivationSummary{
			ActivationId: adminapi.ActivationId(a.ID),
			State:        adminapi.ActivationSummaryState(a.State),
			CreatedAt:    a.CreatedAt,
		}
		if a.CompletedAt != nil {
			item.CompletedAt = a.CompletedAt
		}
		if a.ErrorCode != nil {
			item.ErrorCode = a.ErrorCode
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) GetSnapshot(ctx context.Context, generation int) (SnapshotView, error) {
	row, err := s.Client.ManagedRelease.Query().Where(
		managedrelease.ManagedGenerationEQ(int64(generation)),
		managedrelease.StatusIn("ACTIVE", "SUPERSEDED"),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return SnapshotView{}, ErrReleaseNotFound
	}
	if err != nil {
		return SnapshotView{}, err
	}
	return SnapshotView{JSON: append([]byte(nil), row.SnapshotJSON...), Hash: row.SnapshotHash}, nil
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

// PreviewDraft compiles the current draft into a snapshot preview without persisting a release.
// It returns the canonical snapshot hash and sorted resource arrays so the operator can
// review the exact shape that would be published.
func (s *Service) PreviewDraft(ctx context.Context, expectedRevision int) (DraftPreview, error) {
	draft, err := s.GetDraft(ctx)
	if err != nil {
		return DraftPreview{}, err
	}
	if draft.DraftRevision != expectedRevision {
		return DraftPreview{}, ErrRevisionConflict
	}
	deployment, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return DraftPreview{}, err
	}
	_, hash, err := s.CompileSnapshot(SnapshotInput{
		DeploymentID:      deployment.ID,
		ReleaseID:         "preview",
		ManagedGeneration: 1,
		Content:           draft.Content,
		PublishedAt:       s.Now().UTC(),
	})
	if err != nil {
		return DraftPreview{}, err
	}
	// The preview response uses adminapi types directly from the draft content.
	// The hash is computed from the canonical (clientapi) snapshot so it matches
	// what would be persisted on publish.
	content := draft.Content
	return DraftPreview{
		DraftRevision: draft.DraftRevision,
		SnapshotHash:  hash,
		Providers:     content.Providers,
		Models:        content.Models,
		TTS:           content.Tts,
		ASR:           content.Asr,
		MCP:           content.Mcp,
		Policy:        content.Policy,
	}, nil
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
	created, err := s.Client.ManagedRelease.Create().
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
	prev, err := s.previousRelease(ctx, created)
	if err != nil {
		return ReleaseView{}, err
	}
	return s.buildReleaseView(ctx, created, prev), nil
}

func (s *Service) validateContent(ctx context.Context, content adminapi.ManagedDraftContent) ValidationResult {
	result := ValidationResult{Errors: []adminapi.ValidationIssue{}, Warnings: []adminapi.ValidationIssue{}}
	addError := func(code, path, message string) {
		result.Errors = append(result.Errors, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.ValidationIssueSeverityERROR})
	}
	addWarning := func(code, path, message string) {
		result.Warnings = append(result.Warnings, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.ValidationIssueSeverityWARNING})
	}
	if err := validateCandidateIDs(content); err != nil {
		addError("invalid_candidate_id", "$", err.Error())
		result.Valid = false
		return result
	}

	providers := make(map[string]adminapi.ProviderDefinition, len(content.Providers))
	for i, provider := range content.Providers {
		providers[provider.ProviderId] = provider
		if !provider.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("providers[%d].clientProtocol", i), "unsupported provider client protocol")
		}
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
		for j, c := range model.Capabilities {
			if !c.Valid() {
				addError("invalid_capability", fmt.Sprintf("models[%d].capabilities[%d]", i, j), "unsupported model capability")
			}
		}
		for j, m := range model.InputModalities {
			if !m.Valid() {
				addError("invalid_input_modality", fmt.Sprintf("models[%d].inputModalities[%d]", i, j), "unsupported input modality")
			}
		}
		for j, m := range model.OutputModalities {
			if !m.Valid() {
				addError("invalid_output_modality", fmt.Sprintf("models[%d].outputModalities[%d]", i, j), "unsupported output modality")
			}
		}
	}
	for i, value := range content.Tts {
		resources[value.TtsId] = value.Enabled
		if value.Enabled && strings.TrimSpace(value.Voice) == "" {
			addError("missing_tts_voice", fmt.Sprintf("tts[%d].voice", i), "enabled TTS requires a non-empty voice")
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("tts[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}
	for i, value := range content.Asr {
		resources[value.AsrId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("asr[%d].clientProtocol", i), "unsupported ASR client protocol")
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("asr[%d].runtimePath", i), "runtimePath must be an absolute normalized path")
		}
	}
	for i, value := range content.Mcp {
		resources[value.McpServerId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("mcp[%d].clientProtocol", i), "unsupported MCP client protocol")
		}
		if !value.AuthOwnership.Valid() {
			addError("invalid_mcp_auth_ownership", fmt.Sprintf("mcp[%d].authOwnership", i), "MCP authOwnership must be ENTERPRISE_MANAGED or NONE")
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
