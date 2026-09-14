package contract_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"
)

// This is a contract reference oracle, not evidence that Android adopted the parser.
// Raw vectors preserve duplicate keys and byte limits that parsed JSON fixtures lose.
func TestEnrollmentRawSharedVectors(t *testing.T) {
	root := fixtureRoot(t)
	var suite struct {
		Now              time.Time
		InstalledSources []string
		Cases            []struct{ Name, Raw, Result string }
	}
	data, err := os.ReadFile(filepath.Join(root, "enrollment", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	platform, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, "..", "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	nativeLoader := openapi3.NewLoader()
	nativeLoader.IsExternalRefsAllowed = true
	native, err := nativeLoader.LoadFromFile(filepath.Join(root, "..", "portal", "portal-contract.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, vector := range suite.Cases {
		t.Run(vector.Name, func(t *testing.T) {
			result := func() string {
				if len([]byte(vector.Raw)) > 2048 || !utf8.ValidString(vector.Raw) {
					return "invalid"
				}
				decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(vector.Raw)))
				first, err := decoder.Token()
				if err != nil || first != json.Delim('{') {
					return "invalid"
				}
				fields := map[string]json.RawMessage{}
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil {
						return "invalid"
					}
					name, ok := key.(string)
					if !ok {
						return "invalid"
					}
					if _, exists := fields[name]; exists {
						return "invalid"
					}
					var value json.RawMessage
					if decoder.Decode(&value) != nil {
						return "invalid"
					}
					fields[name] = value
				}
				if _, err = decoder.Token(); err != nil {
					return "invalid"
				}
				if _, err = decoder.Token(); err != io.EOF {
					return "invalid"
				}
				var value map[string]any
				if json.Unmarshal([]byte(vector.Raw), &value) != nil {
					return "invalid"
				}
				schema := platform.Components.Schemas["PlatformEnrollmentMaterial"]
				if value["kind"] == "LOCAL_EXAMPLE_ENROLLMENT" {
					schema = native.Components.Schemas["LocalExampleEnrollmentMaterial"]
				}
				if schema.Value.VisitJSON(value, utcDateFormats()) != nil {
					return "invalid"
				}
				expires, ok := value["expiresAt"].(string)
				if !ok {
					return "invalid"
				}
				normalized := strings.ToUpper(expires)
				normalized = strings.TrimSuffix(normalized, "+00:00")
				if !strings.HasSuffix(normalized, "Z") {
					normalized += "Z"
				}
				expiry, err := time.Parse(time.RFC3339Nano, normalized)
				if err != nil {
					return "invalid"
				}
				if value["kind"] == "PLATFORM_ENROLLMENT" {
					origin, err := url.Parse(value["platformUrl"].(string))
					if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
						return "invalid"
					}
				}
				if !expiry.After(suite.Now) {
					return "expired"
				}
				if value["kind"] == "LOCAL_EXAMPLE_ENROLLMENT" {
					found := false
					for _, source := range suite.InstalledSources {
						if value["sourceNamespace"] == source {
							found = true
						}
					}
					if !found {
						return "unknown_source"
					}
				}
				return "valid"
			}()
			if result != vector.Result {
				t.Fatalf("want %s, got %s", vector.Result, result)
			}
		})
	}
}

// kin-openapi's default date-time regexp rejects RFC3339 lowercase t/z.
// Override only these native wire checks; schema patterns still enforce each field's subset.
func utcDateFormats() openapi3.SchemaValidationOption {
	return openapi3.WithStringFormatValidators(map[string]openapi3.StringFormatValidator{
		"date": openapi3.NewCallbackValidator(func(value string) error {
			_, err := time.Parse("2006-01-02", value)
			return err
		}),
		"date-time": openapi3.NewCallbackValidator(func(value string) error {
			_, err := time.Parse(time.RFC3339Nano, strings.ToUpper(value))
			return err
		}),
	})
}
