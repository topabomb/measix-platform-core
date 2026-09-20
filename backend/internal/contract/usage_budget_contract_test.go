package contract_test

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func loadContractDoc(t *testing.T, relative string) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, relative))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func schemaEnums(t *testing.T, doc *openapi3.T, name string) []string {
	t.Helper()
	ref := doc.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("schema %s is missing", name)
	}
	values := enumStringValues(t, ref.Value.Enum)
	sort.Strings(values)
	return values
}

func TestUsageBudgetContractsFreezeTheSameProductionProtocols(t *testing.T) {
	want := []string{
		"ANTHROPIC_MESSAGES", "DASHSCOPE_HTTP_ASR", "DASHSCOPE_MULTIMODAL_GENERATION", "DASHSCOPE_REALTIME_ASR",
		"GEMINI_GENERATE_CONTENT_TTS", "GOOGLE_GENERATE_CONTENT", "MCP_STREAMABLE_HTTP",
		"MIMO_CHAT_COMPLETIONS_TTS", "OPENAI_AUDIO_SPEECH", "OPENAI_AUDIO_TRANSCRIPTIONS",
		"OPENAI_CHAT_COMPLETIONS", "OPENAI_IMAGES_GENERATIONS", "OPENAI_REALTIME_TRANSCRIPTION", "OPENAI_RESPONSES",
	}
	for _, file := range []string{
		"api/admin/admin.openapi.yaml",
		"api/client/client-control.openapi.yaml",
	} {
		doc := loadContractDoc(t, file)
		got := schemaEnums(t, doc, "UsageClientProtocol")
		if len(got) != len(want) {
			t.Fatalf("%s ClientProtocol count = %d, want %d: %v", file, len(got), len(want), got)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("%s ClientProtocol = %v, want %v", file, got, want)
			}
		}
	}
	usage := loadContractDoc(t, "api/internal/usage-ingest.openapi.yaml")
	gotUsage := schemaEnums(t, usage, "ClientProtocol")
	if len(gotUsage) != len(want) {
		t.Fatalf("usage ingest ClientProtocol count = %d, want %d: %v", len(gotUsage), len(want), gotUsage)
	}
	for index := range want {
		if gotUsage[index] != want[index] {
			t.Fatalf("usage ingest ClientProtocol = %v, want %v", gotUsage, want)
		}
	}

	relay := loadContractDoc(t, "api/internal/relay-control.openapi.yaml")
	resourceRoute := relay.Components.Schemas["ResourceRoute"].Value
	got := schemaEnums(t, &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{"ClientProtocol": resourceRoute.Properties["clientProtocol"]}}}, "ClientProtocol")
	if len(got) != len(want) {
		t.Fatalf("Relay ResourceRoute clientProtocol count = %d, want %d: %v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Relay ResourceRoute clientProtocol = %v, want %v", got, want)
		}
	}
}

func TestS02BudgetAndSelfUsageSurfacesAreComplete(t *testing.T) {
	admin := loadContractDoc(t, "api/admin/admin.openapi.yaml")
	assertBudgetUsageMetersRequired(t, admin, "Admin")
	for _, path := range []string{
		"/api/admin/v1/budget-templates",
		"/api/admin/v1/budget-templates/{budgetTemplateId}",
		"/api/admin/v1/users/{userId}/budget-template",
		"/api/admin/v1/users/{userId}/budgets",
		"/api/admin/v1/users/{userId}/budgets/{capability}",
		"/api/admin/v1/users/{userId}/budgets/{capability}/audit",
		"/api/admin/v1/usage/trend",
		"/api/admin/v1/usage/distribution",
		"/api/admin/v1/usage/users",
		"/api/admin/v1/usage/reconciliations",
		"/api/admin/v1/usage/reconciliations/{requestId}:resolve",
	} {
		if admin.Paths.Find(path) == nil {
			t.Fatalf("Admin contract is missing %s", path)
		}
	}

	client := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	assertBudgetUsageMetersRequired(t, client, "Client")
	for _, path := range []string{
		"/api/client/v1/budgets",
		"/api/portal/v1/budgets",
		"/api/portal/v1/usage/summary",
		"/api/portal/v1/usage/trend",
		"/api/portal/v1/usage/distribution",
		"/api/portal/v1/usage/requests",
		"/api/portal/v1/usage/requests/{requestId}",
	} {
		item := client.Paths.Find(path)
		if item == nil || item.Get == nil {
			t.Fatalf("Client/Portal contract is missing GET %s", path)
		}
		for _, parameter := range item.Get.Parameters {
			if parameter.Value != nil && parameter.Value.Name == "userId" {
				t.Fatalf("self-scoped endpoint %s must not accept userId", path)
			}
		}
	}
	if client.Components.Schemas["BudgetTemplate"] != nil || client.Components.Schemas["BudgetTemplateAssignment"] != nil {
		t.Fatal("Client/Portal contract must not expose Admin budget template metadata")
	}
}

func TestBudgetCapabilitiesAndResourceKindsAreFivePeerCategories(t *testing.T) {
	want := []string{"ASR", "IMAGE_GENERATION", "MCP", "MODEL", "TTS"}
	for _, file := range []string{
		"api/admin/admin.openapi.yaml",
		"api/client/client-control.openapi.yaml",
		"api/internal/usage-ingest.openapi.yaml",
	} {
		doc := loadContractDoc(t, file)
		for _, schema := range []string{"ResourceKind", "BudgetCapability"} {
			got := schemaEnums(t, doc, schema)
			if len(got) != len(want) {
				t.Fatalf("%s %s = %v, want %v", file, schema, got, want)
			}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("%s %s = %v, want %v", file, schema, got, want)
				}
			}
		}
	}
}

func assertBudgetUsageMetersRequired(t *testing.T, doc *openapi3.T, name string) {
	t.Helper()
	schema := doc.Components.Schemas["BudgetCapabilityView"].Value
	if schema.Properties["usageMeters"] == nil {
		t.Fatalf("%s BudgetCapabilityView is missing usageMeters", name)
	}
	for _, required := range schema.Required {
		if required == "usageMeters" {
			return
		}
	}
	t.Fatalf("%s BudgetCapabilityView usageMeters must be required", name)
}

func TestUnforwardedUsageFactsRetainCompleteAttribution(t *testing.T) {
	usage := loadContractDoc(t, "api/internal/usage-ingest.openapi.yaml")
	fact := usage.Components.Schemas["RequestUsageFact"].Value
	required := make(map[string]bool, len(fact.Required))
	for _, name := range fact.Required {
		required[name] = true
	}
	for _, name := range []string{"deploymentId", "userId", "resourceId", "runtimeRouteId", "upstreamId", "resourceKind", "clientProtocol", "forwarded"} {
		if !required[name] {
			t.Fatalf("RequestUsageFact attribution %s must be required for every settlement", name)
		}
	}
}

func TestUsageMetersIncludeTotalTokensWithoutInventedMeters(t *testing.T) {
	usage := loadContractDoc(t, "api/internal/usage-ingest.openapi.yaml")
	got := schemaEnums(t, usage, "UsageMeter")
	want := []string{"AUDIO_SECONDS", "CACHED_TOKENS", "CHARACTERS", "INPUT_TOKENS", "OUTPUT_TOKENS", "REQUESTED_IMAGES", "REQUESTS", "TOTAL_TOKENS"}
	if len(got) != len(want) {
		t.Fatalf("UsageMeter = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("UsageMeter = %v, want %v", got, want)
		}
	}
}

func TestClientRuntimeProblemUsesRelayBudgetShape(t *testing.T) {
	client := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	relay := loadContractDoc(t, "api/internal/relay-control.openapi.yaml")
	clientProblem := client.Components.Schemas["Problem"].Value
	budgetRef := clientProblem.Properties["budget"]
	if budgetRef == nil || budgetRef.Ref != "#/components/schemas/RuntimeBudgetContext" {
		t.Fatalf("Client Problem budget ref = %v, want RuntimeBudgetContext", budgetRef)
	}
	clientBudget := client.Components.Schemas["RuntimeBudgetContext"].Value
	relayBudget := relay.Components.Schemas["BudgetContext"].Value
	for _, property := range []string{"capability", "resourceId", "mode", "blockingLimits", "resetAt", "asOf"} {
		if clientBudget.Properties[property] == nil || relayBudget.Properties[property] == nil {
			t.Fatalf("runtime budget property %q must exist in Client export and Relay contract", property)
		}
	}
	clientLimit := client.Components.Schemas["RuntimeBudgetLimitState"].Value
	relayLimit := relay.Components.Schemas["BudgetLimitState"].Value
	for _, property := range []string{"meter", "period", "limit", "used", "reserved", "resetAt"} {
		if clientLimit.Properties[property] == nil || relayLimit.Properties[property] == nil {
			t.Fatalf("runtime blocking limit property %q must exist in Client export and Relay contract", property)
		}
	}
}
