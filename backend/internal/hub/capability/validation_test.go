package capability

import "testing"

func TestValidRuntimePathRequiresCanonicalPathOnlyValue(t *testing.T) {
	t.Parallel()
	valid := []string{
		"/",
		"/v1/chat/completions",
		"/v1/files/a%20b",
		"/v1beta/models/gemini-2.5-flash:generateContent",
	}
	for _, value := range valid {
		if !validRuntimePath(value) {
			t.Errorf("validRuntimePath(%q) = false, want true", value)
		}
	}

	invalid := []string{
		"relative/path",
		"//provider.example/v1",
		"/v1//models",
		"/v1/./models",
		"/v1/../models",
		"/v1/models/",
		"/v1/models?key=value",
		"/v1/models#fragment",
		"/v1/%2e%2e/models",
		"/v1/files%2Fprivate",
		`/v1\models`,
	}
	for _, value := range invalid {
		if validRuntimePath(value) {
			t.Errorf("validRuntimePath(%q) = true, want false", value)
		}
	}
}
