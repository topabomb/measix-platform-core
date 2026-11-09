// Generates the Portal's self-contained Feed schema dependency from Client OpenAPI.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	input := "../api/client/client-control.openapi.yaml"
	bytes, err := os.ReadFile(input)
	if err != nil {
		panic(err)
	}
	document, err := openapi3.NewLoader().LoadFromFile(input)
	if err != nil {
		panic(err)
	}
	schemas := map[string]any{}
	var add func(string)
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "$ref" {
					add(strings.TrimPrefix(child.(string), "#/components/schemas/"))
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range v {
				visit(child)
			}
		}
	}
	add = func(name string) {
		if _, exists := schemas[name]; exists {
			return
		}
		schema, exists := document.Components.Schemas[name]
		if !exists {
			panic("missing Client schema: " + name)
		}
		encoded, err := json.Marshal(schema)
		if err != nil {
			panic(err)
		}
		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			panic(err)
		}
		schemas[name] = value
		visit(value)
	}
	add("EnterpriseUpdateFeed")
	add("EnterpriseUpdateItem")
	digest := sha256.Sum256(bytes)
	output := map[string]any{
		"openapi": "3.0.3", "info": map[string]any{"title": "Generated Client Feed schemas; do not edit", "version": "1.0.0"},
		"paths": map[string]any{}, "components": map[string]any{"schemas": schemas},
		"x-source": "api/client/client-control.openapi.yaml", "x-source-sha256": hex.EncodeToString(digest[:]),
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("../api/portal/client-feed.schemas.json", append(encoded, '\n'), 0644); err != nil {
		panic(err)
	}
}
