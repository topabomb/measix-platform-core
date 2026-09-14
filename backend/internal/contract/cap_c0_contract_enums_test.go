package contract_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// CAP-C0-001: Provider.clientProtocol must be a frozen enum with exactly OPENAI_CHAT_COMPLETIONS.
func TestCAPC0001ProviderClientProtocolIsFrozenEnum(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "ProviderDefinition")
	prop := getProp(t, schema, "clientProtocol")
	if len(prop.Enum) == 0 {
		t.Fatal("ProviderDefinition.clientProtocol is not a frozen enum (enum is empty)")
	}
	values := enumStringValues(t, prop.Enum)
	if len(values) != 1 || values[0] != "OPENAI_CHAT_COMPLETIONS" {
		t.Fatalf("ProviderDefinition.clientProtocol enum=%v want=[OPENAI_CHAT_COMPLETIONS]", values)
	}
}

// CAP-C0-002: Model.inputModalities must be a frozen enum with TEXT and IMAGE.
func TestCAPC0002ModelInputModalitiesFrozenEnum(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "ModelDefinition")
	prop := getProp(t, schema, "inputModalities")
	items := getItems(t, prop)
	if len(items.Enum) == 0 {
		t.Fatal("ModelDefinition.inputModalities items is not a frozen enum (enum is empty)")
	}
	values := enumStringValues(t, items.Enum)
	want := map[string]bool{"TEXT": true, "IMAGE": true}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected input modality %q, allowed: TEXT, IMAGE", v)
		}
	}
	if len(values) != 2 {
		t.Fatalf("inputModalities enum=%v want exactly [TEXT, IMAGE]", values)
	}
}

// CAP-C0-002: Model.outputModalities must be a frozen enum with only TEXT.
func TestCAPC0002ModelOutputModalitiesFrozenEnum(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "ModelDefinition")
	prop := getProp(t, schema, "outputModalities")
	items := getItems(t, prop)
	if len(items.Enum) == 0 {
		t.Fatal("ModelDefinition.outputModalities items is not a frozen enum (enum is empty)")
	}
	values := enumStringValues(t, items.Enum)
	if len(values) != 1 || values[0] != "TEXT" {
		t.Fatalf("ModelDefinition.outputModalities enum=%v want=[TEXT]", values)
	}
}

// CAP-C0-002: Model.capabilities must be a frozen enum with TOOL and REASONING.
func TestCAPC0002ModelCapabilitiesFrozenEnum(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "ModelDefinition")
	prop := getProp(t, schema, "capabilities")
	items := getItems(t, prop)
	if len(items.Enum) == 0 {
		t.Fatal("ModelDefinition.capabilities items is not a frozen enum (enum is empty)")
	}
	values := enumStringValues(t, items.Enum)
	want := map[string]bool{"TOOL": true, "REASONING": true}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected capability %q, allowed: TOOL, REASONING", v)
		}
	}
	if len(values) != 2 {
		t.Fatalf("capabilities enum=%v want exactly [TOOL, REASONING]", values)
	}
}

// CAP-C0-003: ASR.clientProtocol must be a frozen enum with OPENAI_AUDIO_TRANSCRIPTIONS.
func TestCAPC0003AsrClientProtocolFrozenEnum(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "AsrDefinition")
	prop := getProp(t, schema, "clientProtocol")
	if len(prop.Enum) == 0 {
		t.Fatal("AsrDefinition.clientProtocol is not a frozen enum (enum is empty)")
	}
	values := enumStringValues(t, prop.Enum)
	if len(values) != 1 || values[0] != "OPENAI_AUDIO_TRANSCRIPTIONS" {
		t.Fatalf("AsrDefinition.clientProtocol enum=%v want=[OPENAI_AUDIO_TRANSCRIPTIONS]", values)
	}
}

// CAP-C0-003: ASR.language must be an optional field.
func TestCAPC0003AsrLanguageOptionalField(t *testing.T) {
	doc := loadClientDoc(t)
	schema := getSchema(t, doc, "AsrDefinition")
	if _, ok := schema.Properties["language"]; !ok {
		t.Fatal("AsrDefinition.language field is missing from schema")
	}
	for _, req := range schema.Required {
		if req == "language" {
			t.Fatal("AsrDefinition.language should be optional (not required)")
		}
	}
}

// --- helpers ---

func loadClientDoc(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(filepath.Join(root, "api/client/client-control.openapi.yaml"))
	if err != nil {
		t.Fatalf("load client openapi: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate client openapi: %v", err)
	}
	return doc
}

func getSchema(t *testing.T, doc *openapi3.T, name string) *openapi3.Schema {
	t.Helper()
	ref, ok := doc.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %q not found in components", name)
	}
	return ref.Value
}

func getProp(t *testing.T, schema *openapi3.Schema, name string) *openapi3.Schema {
	t.Helper()
	ref, ok := schema.Properties[name]
	if !ok {
		t.Fatalf("property %q not found in schema", name)
	}
	return ref.Value
}

func getItems(t *testing.T, schema *openapi3.Schema) *openapi3.Schema {
	t.Helper()
	if schema.Items == nil || schema.Items.Value == nil {
		t.Fatalf("schema has no items")
	}
	return schema.Items.Value
}

func enumStringValues(t *testing.T, enum []any) []string {
	t.Helper()
	values := make([]string, 0, len(enum))
	for _, v := range enum {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("enum value is not a string: %v", v)
		}
		values = append(values, s)
	}
	sort.Strings(values)
	return values
}

var _ = fmt.Sprintf // keep fmt import if needed later
