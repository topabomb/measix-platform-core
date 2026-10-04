package capability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
)

// DecodeManagedDraftContent preserves absent legacy openings. Decoding never
// authors instructions or writes back an immutable release or a mutable draft.
func DecodeManagedDraftContent(raw []byte, target *adminapi.ManagedDraftContent) error {
	if err := ValidateDraftOpeningJSON(raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// PublishedContent uses the immutable Snapshot's effective opening values for
// comparisons and republication. ReleaseContentJSON retains authoring intent and
// runtime bindings; its blank System may be an inheritance marker, not a literal.
// Neither durable input is changed, and a missing or mismatched projection fails.
func PublishedContent(content adminapi.ManagedDraftContent, snapshotJSON []byte) (adminapi.ManagedDraftContent, int, error) {
	var source struct {
		SchemaVersion int             `json:"schemaVersion"`
		Starters      json.RawMessage `json:"starters"`
	}
	if err := json.Unmarshal(snapshotJSON, &source); err != nil {
		return content, 0, err
	}
	if source.SchemaVersion != 4 && source.SchemaVersion != 5 {
		return content, 0, ErrInvalidDraft
	}
	if len(source.Starters) == 0 || bytes.Equal(bytes.TrimSpace(source.Starters), []byte("null")) {
		return content, 0, ErrInvalidDraft
	}
	if err := ValidateDraftOpeningJSON(snapshotJSON); err != nil {
		return content, 0, err
	}
	var starters []clientapi.AssistantStarterDefinition
	if err := json.Unmarshal(source.Starters, &starters); err != nil {
		return content, 0, err
	}
	if len(starters) != len(content.Starters) {
		return content, 0, ErrInvalidDraft
	}
	byID := make(map[string]clientapi.AssistantStarterDefinition, len(starters))
	for _, starter := range starters {
		if _, duplicate := byID[starter.StarterId]; duplicate {
			return content, 0, ErrInvalidDraft
		}
		byID[starter.StarterId] = starter
	}
	result := NormalizeManagedDraftContent(content)
	result.Starters = append([]adminapi.AssistantStarterDefinition{}, content.Starters...)
	for i, starter := range result.Starters {
		published, found := byID[starter.StarterId]
		if !found || published.AssistantDefinitionId != starter.AssistantDefinitionId {
			return content, 0, ErrInvalidDraft
		}
		result.Starters[i].OpeningSnapshot = toAdminOpening(published.OpeningSnapshot)
		delete(byID, starter.StarterId)
	}
	if len(byID) != 0 {
		return content, 0, ErrInvalidDraft
	}
	if err := validateStarterVersion(result.Starters, source.SchemaVersion); err != nil {
		return content, 0, err
	}
	return result, source.SchemaVersion, nil
}

// ValidateDraftOpeningJSON distinguishes absent (unfinished) from null/broken.
// Generated Go value fields alone cannot distinguish null from valid empty text.
func ValidateDraftOpeningJSON(raw []byte) error {
	var content struct {
		Starters []json.RawMessage `json:"starters"`
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		return err
	}
	for i, item := range content.Starters {
		var starter map[string]json.RawMessage
		if err := json.Unmarshal(item, &starter); err != nil {
			return err
		}
		opening, exists := starter["openingSnapshot"]
		if !exists {
			continue
		}
		fields, err := requiredObject(opening, []string{"format", "systemPrompt", "initialContexts"})
		if err != nil {
			return fmt.Errorf("starters[%d].openingSnapshot: %w", i, err)
		}
		var blocks []json.RawMessage
		if err := json.Unmarshal(fields["initialContexts"], &blocks); err != nil {
			return fmt.Errorf("starters[%d].openingSnapshot.initialContexts must be an array", i)
		}
		for j, block := range blocks {
			if _, err := requiredObject(block, []string{"id", "content"}); err != nil {
				return fmt.Errorf("starters[%d].openingSnapshot.initialContexts[%d]: %w", i, j, err)
			}
		}
		var value adminapi.StarterOpeningSnapshot
		decoder := json.NewDecoder(bytes.NewReader(opening))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if err := validateDraftOpening(&value); err != nil {
			return fmt.Errorf("starters[%d].openingSnapshot: %w", i, err)
		}
	}
	return nil
}

func requiredObject(raw json.RawMessage, names []string) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("object must not be null")
	}
	for _, name := range names {
		field, exists := value[name]
		if !exists || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return nil, fmt.Errorf("%s is required and must not be null", name)
		}
	}
	if len(value) != len(names) {
		return nil, fmt.Errorf("unknown object field")
	}
	return value, nil
}

type openingValidationError struct{ Path, Detail string }

func (e *openingValidationError) Error() string { return e.Path + ": " + e.Detail }
func (e *openingValidationError) Unwrap() error { return ErrInvalidDraft }

func validateDraftOpening(opening *adminapi.StarterOpeningSnapshot) error {
	if opening == nil {
		return nil
	}
	if opening.Format != 1 {
		return &openingValidationError{Path: "format", Detail: "Opening format must be 1"}
	}
	if opening.InitialContexts == nil {
		return &openingValidationError{Path: "initialContexts", Detail: "Initial contexts must be an explicit array"}
	}
	ids := map[string]bool{}
	for i, block := range opening.InitialContexts {
		if strings.TrimSpace(block.Id) == "" {
			return &openingValidationError{Path: fmt.Sprintf("initialContexts[%d].id", i), Detail: "ID must contain non-whitespace text"}
		}
		if ids[block.Id] {
			return &openingValidationError{Path: fmt.Sprintf("initialContexts[%d].id", i), Detail: fmt.Sprintf("Duplicate initial context ID %q", block.Id)}
		}
		ids[block.Id] = true
	}
	return nil
}

func validateStarterVersion(starters []adminapi.AssistantStarterDefinition, version int) error {
	if version != 4 && version != 5 {
		return ErrInvalidDraft
	}
	for _, starter := range starters {
		if version == 4 && starter.OpeningSnapshot != nil {
			return fmt.Errorf("%w: v4 starter cannot contain an opening snapshot", ErrInvalidDraft)
		}
		if version == 5 && starter.OpeningSnapshot == nil {
			return fmt.Errorf("%w: missing_starter_opening", ErrInvalidDraft)
		}
		if err := validateDraftOpening(starter.OpeningSnapshot); err != nil {
			return err
		}
	}
	return nil
}

func toClientOpening(src *adminapi.StarterOpeningSnapshot) *clientapi.StarterOpeningSnapshot {
	if src == nil {
		return nil
	}
	blocks := make([]clientapi.StarterInitialContext, len(src.InitialContexts))
	for i, b := range src.InitialContexts {
		blocks[i] = clientapi.StarterInitialContext{Id: b.Id, Content: b.Content}
	}
	return &clientapi.StarterOpeningSnapshot{Format: clientapi.StarterOpeningSnapshotFormat(src.Format), SystemPrompt: src.SystemPrompt, InitialContexts: blocks}
}

func toAdminOpening(src *clientapi.StarterOpeningSnapshot) *adminapi.StarterOpeningSnapshot {
	if src == nil {
		return nil
	}
	var blocks []adminapi.StarterInitialContext
	if src.InitialContexts != nil {
		blocks = make([]adminapi.StarterInitialContext, len(src.InitialContexts))
	}
	for i, b := range src.InitialContexts {
		blocks[i] = adminapi.StarterInitialContext{Id: b.Id, Content: b.Content}
	}
	return &adminapi.StarterOpeningSnapshot{Format: adminapi.StarterOpeningSnapshotFormat(src.Format), SystemPrompt: src.SystemPrompt, InitialContexts: blocks}
}
