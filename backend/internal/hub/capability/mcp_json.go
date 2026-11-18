package capability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

// Absent new fields are unfinished legacy authoring; explicit null is invalid.
func ValidateDraftMcpJSON(raw []byte) error {
	if _, err := jsoncanonicalizer.Transform(raw); err != nil {
		return ErrInvalidDraft
	}
	var content map[string]json.RawMessage
	if json.Unmarshal(raw, &content) != nil {
		return ErrInvalidDraft
	}
	for _, collection := range []struct {
		name, field string
		required    []string
	}{{"mcp", "allowedTools", []string{"name", "contractHash", "approvalPolicy", "definition"}}, {"assistants", "mcpBindings", []string{"mcpServerId", "toolSelection", "toolNames"}}} {
		var entities []map[string]json.RawMessage
		if json.Unmarshal(content[collection.name], &entities) != nil {
			if _, exists := content[collection.name]; !exists {
				continue
			}
			return ErrInvalidDraft
		}
		for i, entity := range entities {
			if collection.name == "mcp" {
				if mode, exists := entity["toolAccessMode"]; exists && bytes.Equal(bytes.TrimSpace(mode), []byte("null")) {
					return ErrInvalidDraft
				}
			}
			field, exists := entity[collection.field]
			if !exists {
				continue
			}
			if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return fmt.Errorf("%s[%d].%s must be an explicit array", collection.name, i, collection.field)
			}
			var items []json.RawMessage
			if json.Unmarshal(field, &items) != nil {
				return ErrInvalidDraft
			}
			for _, item := range items {
				if _, err := requiredObject(item, collection.required, true); err != nil {
					return fmt.Errorf("%s[%d].%s: %w", collection.name, i, collection.field, err)
				}
			}
		}
	}
	return nil
}
