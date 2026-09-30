package protocolusage

import "testing"

func TestImageGenerationDefaultsToOneRequestedImageWithoutReadingPromptContent(t *testing.T) {
	result, err := ObserveImageGenerationRequest([]byte(`{"model":"provider-model","prompt":"opaque text","size":"auto"}`), 6, []string{"auto", "1024x1024"})
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, result, map[Meter]int64{Requests: 1, RequestedImages: 1}, Exact)
}

func TestImageGenerationRejectsNonIntegerCount(t *testing.T) {
	if _, err := ObserveImageGenerationRequest([]byte(`{"model":"provider-model","prompt":"text","n":1.5,"size":"1024x1024"}`), 6, []string{"1024x1024"}); err == nil {
		t.Fatal("fractional n was accepted")
	}
}

func TestImageGenerationRejectsDuplicateObservedField(t *testing.T) {
	if _, err := ObserveImageGenerationRequest([]byte(`{"model":"provider-model","prompt":"text","n":6,"n":1,"size":"1024x1024"}`), 6, []string{"1024x1024"}); err == nil {
		t.Fatal("duplicate n was accepted")
	}
}

func TestImageGenerationDoesNotInterpretProviderModelOrPrompt(t *testing.T) {
	result, err := ObserveImageGenerationRequest([]byte(`{"prompt":"","size":"auto"}`), 6, []string{"auto"})
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, result, map[Meter]int64{Requests: 1, RequestedImages: 1}, Exact)
}

func TestImageGenerationRejectsFieldsOutsideTheFixedProfile(t *testing.T) {
	for _, body := range []string{
		`{"model":"provider-model","prompt":"text","size":"auto","quality":"hd"}`,
		`{"model":"provider-model","prompt":"text","size":"auto","response_format":"url"}`,
	} {
		if _, err := ObserveImageGenerationRequest([]byte(body), 6, []string{"auto"}); err == nil {
			t.Fatalf("unsupported image field was accepted: %s", body)
		}
	}
}

func TestDashScopeImageGenerationObservesNestedCountAndCanonicalSize(t *testing.T) {
	body := []byte(`{"model":"wan2.7-image","input":{"messages":[{"role":"user","content":[{"text":"opaque text"}]}]},"parameters":{"size":"1024*1024","n":2,"watermark":false}}`)
	result, err := ObserveImageGenerationRequestForProtocol(
		"DASHSCOPE_MULTIMODAL_GENERATION",
		body,
		4,
		[]string{"1024x1024", "1536x1024"},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, result, map[Meter]int64{Requests: 1, RequestedImages: 2}, Exact)
}

func TestDashScopeImageGenerationRejectsImageInputAndUnknownParameters(t *testing.T) {
	for _, body := range []string{
		`{"model":"wan2.7-image","input":{"messages":[{"role":"user","content":[{"image":"https://example.invalid/input.png"},{"text":"edit"}]}]},"parameters":{"size":"1024*1024","n":1,"watermark":false}}`,
		`{"model":"wan2.7-image","input":{"messages":[{"role":"user","content":[{"text":"opaque"}]}]},"parameters":{"size":"1024*1024","n":1,"watermark":false,"prompt_extend":true}}`,
		`{"model":"wan2.7-image","input":{"messages":[{"role":"user","content":[{"text":"opaque"}]}]},"parameters":{"size":"1024*1024","n":1,"watermark":true}}`,
	} {
		if _, err := ObserveImageGenerationRequestForProtocol(
			"DASHSCOPE_MULTIMODAL_GENERATION",
			[]byte(body),
			4,
			[]string{"1024x1024"},
		); err == nil {
			t.Fatalf("unsupported DashScope image shape was accepted: %s", body)
		}
	}
}
