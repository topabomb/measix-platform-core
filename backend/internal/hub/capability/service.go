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
	DraftRevision  int
	ProjectionHash string
	Providers      []adminapi.ProviderDefinition
	Models         []adminapi.ModelDefinition
	TTS            []adminapi.TtsDefinition
	ASR            []adminapi.AsrDefinition
	MCP            []adminapi.McpDefinition
	Policy         adminapi.ManagedPolicy
	Assistants     []adminapi.ManagedAssistantDefinition
	Starters       []adminapi.AssistantStarterDefinition
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
		if previous.Assistants != nil {
			for _, a := range *previous.Assistants {
				prev[string(a.AssistantDefinitionId)] = defHash(a)
				prevKinds[string(a.AssistantDefinitionId)] = adminapi.ReleaseDiffKindASSISTANT
			}
		}
		if previous.Starters != nil {
			for _, s := range *previous.Starters {
				prev[string(s.StarterId)] = defHash(s)
				prevKinds[string(s.StarterId)] = adminapi.ReleaseDiffKindSTARTER
			}
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
	if current.Assistants != nil {
		for _, a := range *current.Assistants {
			process(adminapi.ReleaseDiffKindASSISTANT, string(a.AssistantDefinitionId))
		}
	}
	if current.Starters != nil {
		for _, s := range *current.Starters {
			process(adminapi.ReleaseDiffKindSTARTER, string(s.StarterId))
		}
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
		adminapi.ReleaseDiffKindASSISTANT,
		adminapi.ReleaseDiffKindSTARTER,
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
	if content.Assistants != nil {
		for _, a := range *content.Assistants {
			if string(a.AssistantDefinitionId) == id {
				return defHash(a)
			}
		}
	}
	if content.Starters != nil {
		for _, s := range *content.Starters {
			if string(s.StarterId) == id {
				return defHash(s)
			}
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

func (s *Service) ListReleases(ctx context.Context, limit int, before int) ([]ReleaseView, error) {
	q := s.Client.ManagedRelease.Query()
	if before > 0 {
		q = q.Where(managedrelease.ManagedGenerationLT(int64(before)))
	}
	rows, err := q.Order(ent.Desc(managedrelease.FieldManagedGeneration)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]ReleaseView, 0, len(rows))
	for i, row := range rows {
		view, err := s.buildReleaseView(ctx, row, previousReleaseRow(rows, i))
		if err != nil {
			return nil, err
		}
		views = append(views, view)
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
	return s.buildReleaseView(ctx, row, prev)
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

func (s *Service) buildReleaseView(ctx context.Context, row, prev *ent.ManagedRelease) (ReleaseView, error) {
	var current, previous *adminapi.ManagedDraftContent
	if err := json.Unmarshal(row.ReleaseContentJSON, &current); err != nil {
		return ReleaseView{}, err
	}
	if current == nil {
		return ReleaseView{}, fmt.Errorf("invalid persisted release content")
	}
	if prev != nil {
		if err := json.Unmarshal(prev.ReleaseContentJSON, &previous); err != nil {
			return ReleaseView{}, err
		}
		if previous == nil {
			return ReleaseView{}, fmt.Errorf("invalid previous release content")
		}
	}
	diff := releaseContentDiff(current, previous)
	history, err := s.activationHistory(ctx, int(row.ManagedGeneration))
	if err != nil {
		return ReleaseView{}, err
	}
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
	}, nil
}

// activationHistory returns the ordered activation attempts targeting the given
// managed generation (release), newest first.
func (s *Service) activationHistory(ctx context.Context, generation int) ([]adminapi.ActivationSummary, error) {
	rows, err := s.Client.Activation.Query().Where(
		activation.TargetGenerationEQ(int64(generation)),
	).Order(ent.Desc(activation.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
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
	return out, nil
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
// It returns the canonical projection hash and sorted resource arrays so the operator can
// review the exact shape that would be published. The projectionHash is computed from
// the same canonical projection as a real snapshot but uses placeholder
// releaseId/generation/publishedAt — it is NOT the final snapshotHash that will be
// assigned on Publish.
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
	// Compile the snapshot using the same canonical projection as Publish.
	// The placeholder releaseId/generation/publishedAt are used only for hash computation;
	// they do NOT appear in the returned projectionHash as the final snapshotHash.
	snapshot, hash, err := s.CompileSnapshot(SnapshotInput{
		DeploymentID:      deployment.ID,
		ReleaseID:         platformid.New(platformid.Release),
		ManagedGeneration: 1,
		Content:           draft.Content,
		PublishedAt:       s.Now().UTC(),
	})
	if err != nil {
		return DraftPreview{}, err
	}
	// Return the canonical projection (sorted arrays from compiler output),
	// not the raw Draft arrays. This ensures Preview == actual Snapshot shape.
	return DraftPreview{
		DraftRevision:  draft.DraftRevision,
		ProjectionHash: hash,
		Providers:      projectionToAdminProviders(snapshot.Providers),
		Models:         projectionToAdminModels(snapshot.Models),
		TTS:            projectionToAdminTts(snapshot.Tts),
		ASR:            projectionToAdminAsr(snapshot.Asr),
		MCP:            projectionToAdminMcp(snapshot.Mcp),
		Policy: adminapi.ManagedPolicy{
			PolicyId:            snapshot.Policy.PolicyId,
			AllowLocalProviders: snapshot.Policy.AllowLocalProviders,
			AllowLocalTts:       snapshot.Policy.AllowLocalTts,
			AllowLocalAsr:       snapshot.Policy.AllowLocalAsr,
			AllowLocalMcp:       snapshot.Policy.AllowLocalMcp,
			DefaultModelId:      snapshot.Policy.DefaultModelId,
			DefaultTtsId:        snapshot.Policy.DefaultTtsId,
			DefaultAsrId:        snapshot.Policy.DefaultAsrId,
		},
		Assistants: projectionToAdminAssistants(snapshot.Assistants),
		Starters:   projectionToAdminStarters(snapshot.Starters),
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
		SetSnapshotSchemaVersion(2).
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
	return s.buildReleaseView(ctx, created, prev)
}

func (s *Service) validateContent(ctx context.Context, content adminapi.ManagedDraftContent) ValidationResult {
	result := ValidationResult{Errors: []adminapi.ValidationIssue{}, Warnings: []adminapi.ValidationIssue{}}
	addError := func(code, path, message string, kind *adminapi.ValidationIssueResourceKind, resourceID *string, field *string) {
		result.Errors = append(result.Errors, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.ValidationIssueSeverityERROR, ResourceKind: kind, ResourceId: resourceID, Field: field})
	}
	addWarning := func(code, path, message string, kind *adminapi.ValidationIssueResourceKind, resourceID *string, field *string) {
		result.Warnings = append(result.Warnings, adminapi.ValidationIssue{Code: code, Path: path, Message: message, Severity: adminapi.ValidationIssueSeverityWARNING, ResourceKind: kind, ResourceId: resourceID, Field: field})
	}
	// Helpers for common patterns
	kindProvider := adminapi.ValidationIssueResourceKind("PROVIDER")
	kindModel := adminapi.ValidationIssueResourceKind("MODEL")
	kindTTS := adminapi.ValidationIssueResourceKind("TTS")
	kindASR := adminapi.ValidationIssueResourceKind("ASR")
	kindMCP := adminapi.ValidationIssueResourceKind("MCP")
	kindPolicy := adminapi.ValidationIssueResourceKind("POLICY")
	kindBinding := adminapi.ValidationIssueResourceKind("BINDING")
	kindAssistant := adminapi.ValidationIssueResourceKind("ASSISTANT")
	kindStarter := adminapi.ValidationIssueResourceKind("STARTER")
	ptrStr := func(s string) *string { return &s }
	if err := validateCandidateIDs(content); err != nil {
		addError("invalid_candidate_id", "$", err.Error(), nil, nil, nil)
		result.Valid = false
		return result
	}

	providers := make(map[string]adminapi.ProviderDefinition, len(content.Providers))
	for i, provider := range content.Providers {
		providers[provider.ProviderId] = provider
		if !provider.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("providers[%d].clientProtocol", i), "unsupported provider client protocol", &kindProvider, ptrStr(provider.ProviderId), ptrStr("clientProtocol"))
		}
	}
	resources := map[string]bool{}
	for i, model := range content.Models {
		resources[model.ModelId] = model.Enabled
		if _, ok := providers[model.ProviderId]; !ok {
			addError("missing_provider", fmt.Sprintf("models[%d].providerId", i), "model references an unknown provider", &kindModel, ptrStr(model.ModelId), ptrStr("providerId"))
		}
		if !validRuntimePath(model.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("models[%d].runtimePath", i), "runtimePath must be an absolute normalized path", &kindModel, ptrStr(model.ModelId), ptrStr("runtimePath"))
		}
		for j, c := range model.Capabilities {
			if !c.Valid() {
				addError("invalid_capability", fmt.Sprintf("models[%d].capabilities[%d]", i, j), "unsupported model capability", &kindModel, ptrStr(model.ModelId), ptrStr("capabilities"))
			}
		}
		for j, m := range model.InputModalities {
			if !m.Valid() {
				addError("invalid_input_modality", fmt.Sprintf("models[%d].inputModalities[%d]", i, j), "unsupported input modality", &kindModel, ptrStr(model.ModelId), ptrStr("inputModalities"))
			}
		}
		for j, m := range model.OutputModalities {
			if !m.Valid() {
				addError("invalid_output_modality", fmt.Sprintf("models[%d].outputModalities[%d]", i, j), "unsupported output modality", &kindModel, ptrStr(model.ModelId), ptrStr("outputModalities"))
			}
		}
	}
	for i, value := range content.Tts {
		resources[value.TtsId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("tts[%d].clientProtocol", i), "unsupported TTS client protocol", &kindTTS, ptrStr(value.TtsId), ptrStr("clientProtocol"))
		}
		if strings.TrimSpace(value.UpstreamModelKey) == "" {
			addError("missing_tts_model_key", fmt.Sprintf("tts[%d].upstreamModelKey", i), "TTS requires a non-empty upstreamModelKey", &kindTTS, ptrStr(value.TtsId), ptrStr("upstreamModelKey"))
		}
		if value.Enabled && strings.TrimSpace(value.Voice) == "" {
			addError("missing_tts_voice", fmt.Sprintf("tts[%d].voice", i), "enabled TTS requires a non-empty voice", &kindTTS, ptrStr(value.TtsId), ptrStr("voice"))
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("tts[%d].runtimePath", i), "runtimePath must be an absolute normalized path", &kindTTS, ptrStr(value.TtsId), ptrStr("runtimePath"))
		}
	}
	for i, value := range content.Asr {
		resources[value.AsrId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("asr[%d].clientProtocol", i), "unsupported ASR client protocol", &kindASR, ptrStr(value.AsrId), ptrStr("clientProtocol"))
		}
		if strings.TrimSpace(value.UpstreamModelKey) == "" {
			addError("missing_asr_model_key", fmt.Sprintf("asr[%d].upstreamModelKey", i), "ASR requires a non-empty upstreamModelKey", &kindASR, ptrStr(value.AsrId), ptrStr("upstreamModelKey"))
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("asr[%d].runtimePath", i), "runtimePath must be an absolute normalized path", &kindASR, ptrStr(value.AsrId), ptrStr("runtimePath"))
		}
	}
	for i, value := range content.Mcp {
		resources[value.McpServerId] = value.Enabled
		if !value.ClientProtocol.Valid() {
			addError("invalid_client_protocol", fmt.Sprintf("mcp[%d].clientProtocol", i), "unsupported MCP client protocol", &kindMCP, ptrStr(value.McpServerId), ptrStr("clientProtocol"))
		}
		if !value.AuthOwnership.Valid() {
			addError("invalid_mcp_auth_ownership", fmt.Sprintf("mcp[%d].authOwnership", i), "MCP authOwnership must be ENTERPRISE_MANAGED or NONE", &kindMCP, ptrStr(value.McpServerId), ptrStr("authOwnership"))
		}
		if !validRuntimePath(value.RuntimePath) {
			addError("invalid_runtime_path", fmt.Sprintf("mcp[%d].runtimePath", i), "runtimePath must be an absolute normalized path", &kindMCP, ptrStr(value.McpServerId), ptrStr("runtimePath"))
		}
	}

	// Validate assistants
	enabledModels := map[string]bool{}
	for _, m := range content.Models {
		enabledModels[m.ModelId] = m.Enabled
	}
	enabledMcp := map[string]bool{}
	for _, m := range content.Mcp {
		enabledMcp[m.McpServerId] = m.Enabled
	}
	assistantIds := map[string]bool{}
	if content.Assistants != nil {
		for i, a := range *content.Assistants {
			path := fmt.Sprintf("assistants[%d]", i)
			if strings.TrimSpace(a.DisplayName) == "" {
				addError("missing_display_name", path+".displayName", "assistant displayName is required", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("displayName"))
			}
			if strings.TrimSpace(a.SystemPrompt) == "" {
				addError("missing_system_prompt", path+".systemPrompt", "assistant systemPrompt is required", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("systemPrompt"))
			}
			if !enabledModels[string(a.ModelId)] {
				addError("invalid_model_ref", path+".modelId", "assistant references an unknown or disabled model", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("modelId"))
			}
			if a.MemorySeed == nil {
				addError("missing_memory_seed", path+".memorySeed", "memorySeed must be an array (empty is allowed)", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("memorySeed"))
			}
			for j, s := range a.MemorySeed {
				if strings.TrimSpace(s) == "" {
					addError("empty_memory_seed", fmt.Sprintf("%s.memorySeed[%d]", path, j), "memory seed item must be non-empty", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("memorySeed"))
				}
			}
			for j, m := range a.McpServerIds {
				if !enabledMcp[string(m)] {
					addError("invalid_mcp_ref", fmt.Sprintf("%s.mcpServerIds[%d]", path, j), "assistant references an unknown or disabled MCP server", &kindAssistant, ptrStr(string(a.AssistantDefinitionId)), ptrStr("mcpServerIds"))
				}
			}
			assistantIds[string(a.AssistantDefinitionId)] = a.Enabled
		}
	}

	// Validate starters
	if content.Starters != nil {
		for i, s := range *content.Starters {
			path := fmt.Sprintf("starters[%d]", i)
			if strings.TrimSpace(s.Title) == "" {
				addError("missing_title", path+".title", "starter title is required", &kindStarter, ptrStr(string(s.StarterId)), ptrStr("title"))
			}
			if strings.TrimSpace(s.Prompt) == "" {
				addError("missing_prompt", path+".prompt", "starter prompt is required", &kindStarter, ptrStr(string(s.StarterId)), ptrStr("prompt"))
			}
			if !assistantIds[string(s.AssistantDefinitionId)] {
				addError("invalid_assistant_ref", path+".assistantDefinitionId", "starter references an unknown or disabled assistant", &kindStarter, ptrStr(string(s.StarterId)), ptrStr("assistantDefinitionId"))
			}
		}
	}

	bound := map[string]bool{}
	for i, binding := range content.Bindings {
		path := fmt.Sprintf("bindings[%d]", i)
		if _, ok := resources[binding.ResourceId]; !ok {
			addError("missing_resource", path+".resourceId", "binding references an unknown runtime resource", &kindBinding, ptrStr(binding.ResourceId), ptrStr("resourceId"))
		} else if bound[binding.ResourceId] {
			addError("duplicate_binding", path+".resourceId", "resource has more than one runtime binding", &kindBinding, ptrStr(binding.ResourceId), ptrStr("resourceId"))
		} else {
			bound[binding.ResourceId] = true
		}
		if !binding.TransportPolicy.Valid() {
			addError("invalid_transport_policy", path+".transportPolicy", "unsupported transport policy", &kindBinding, ptrStr(binding.ResourceId), ptrStr("transportPolicy"))
		}
		if len(binding.AllowedMethods) == 0 {
			addError("missing_method_policy", path+".allowedMethods", "at least one HTTP method is required", &kindBinding, ptrStr(binding.ResourceId), ptrStr("allowedMethods"))
		}
		for _, method := range binding.AllowedMethods {
			if !validMethod(method) {
				addError("invalid_method", path+".allowedMethods", "method policy contains an unsupported method", &kindBinding, ptrStr(binding.ResourceId), ptrStr("allowedMethods"))
			}
		}
		if len(binding.AllowedPathPrefixes) == 0 {
			addError("missing_path_policy", path+".allowedPathPrefixes", "at least one path prefix is required", &kindBinding, ptrStr(binding.ResourceId), ptrStr("allowedPathPrefixes"))
		}
		for _, prefix := range binding.AllowedPathPrefixes {
			if !validRuntimePath(prefix) {
				addError("invalid_path_prefix", path+".allowedPathPrefixes", "path prefix must be normalized and absolute", &kindBinding, ptrStr(binding.ResourceId), ptrStr("allowedPathPrefixes"))
			}
		}
		row, err := s.Client.Upstream.Get(ctx, binding.UpstreamId)
		if ent.IsNotFound(err) {
			addError("missing_upstream", path+".upstreamId", "binding references an unknown upstream", &kindBinding, ptrStr(binding.ResourceId), ptrStr("upstreamId"))
			continue
		}
		if err != nil {
			addError("upstream_lookup_failed", path+".upstreamId", err.Error(), &kindBinding, ptrStr(binding.ResourceId), ptrStr("upstreamId"))
			continue
		}
		if row.Status == "DISABLED" {
			addError("upstream_disabled", path+".upstreamId", "binding references a disabled upstream", &kindBinding, ptrStr(binding.ResourceId), ptrStr("upstreamId"))
		}
		if row.Status == "INACTIVE" {
			addWarning("upstream_not_active", path+".upstreamId", "upstream has not yet been applied to Runtime Relay", &kindBinding, ptrStr(binding.ResourceId), ptrStr("upstreamId"))
		}
		config, err := upstream.LoadCandidateConfig(ctx, s.Client, row.ID)
		if err != nil || upstream.ValidateConfig(ctx, s.Client, config) != nil {
			addError("invalid_upstream_config", path+".upstreamId", "upstream candidate config or SecretRef is invalid", &kindBinding, ptrStr(binding.ResourceId), ptrStr("upstreamId"))
		}
	}
	for resourceID, enabled := range resources {
		if enabled && !bound[resourceID] {
			addError("missing_binding", "bindings", "enabled resource has no runtime binding", nil, ptrStr(resourceID), nil)
		}
	}
	if content.Policy.DefaultModelId != nil {
		if enabled, ok := resources[*content.Policy.DefaultModelId]; !ok || !enabled {
			addError("invalid_default_model", "policy.defaultModelId", "default model must reference an enabled model", &kindPolicy, content.Policy.DefaultModelId, ptrStr("defaultModelId"))
		}
	}
	if content.Policy.DefaultTtsId != nil {
		if enabled, ok := resources[*content.Policy.DefaultTtsId]; !ok || !enabled {
			addError("invalid_default_tts", "policy.defaultTtsId", "default TTS must reference an enabled TTS", &kindPolicy, content.Policy.DefaultTtsId, ptrStr("defaultTtsId"))
		}
	}
	if content.Policy.DefaultAsrId != nil {
		if enabled, ok := resources[*content.Policy.DefaultAsrId]; !ok || !enabled {
			addError("invalid_default_asr", "policy.defaultAsrId", "default ASR must reference an enabled ASR", &kindPolicy, content.Policy.DefaultAsrId, ptrStr("defaultAsrId"))
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
	if content.Assistants != nil {
		for i, value := range *content.Assistants {
			if err := check(platformid.Assistant, string(value.AssistantDefinitionId), fmt.Sprintf("assistants[%d]", i)); err != nil {
				return err
			}
		}
	}
	if content.Starters != nil {
		for i, value := range *content.Starters {
			if err := check(platformid.Starter, string(value.StarterId), fmt.Sprintf("starters[%d]", i)); err != nil {
				return err
			}
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
