package protocolusage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	OpenAIImagesGenerations       = "OPENAI_IMAGES_GENERATIONS"
	DashScopeMultimodalGeneration = "DASHSCOPE_MULTIMODAL_GENERATION"
)

// ObserveImageGenerationRequest validates only the provider-neutral execution
// controls owned by Relay. The prompt remains opaque and the original payload
// is never rewritten or retained.
func ObserveImageGenerationRequest(body []byte, maxImages int, allowedSizes []string) (Result, error) {
	return observeOpenAIImageGenerationRequest(body, maxImages, allowedSizes)
}

// ObserveImageGenerationRequestForProtocol selects the one observer owned by
// the configured resource protocol. Relay validates provider controls for
// admission and metering, but never translates or rewrites the request body.
func ObserveImageGenerationRequestForProtocol(protocol string, body []byte, maxImages int, allowedSizes []string) (Result, error) {
	switch protocol {
	case OpenAIImagesGenerations:
		return observeOpenAIImageGenerationRequest(body, maxImages, allowedSizes)
	case DashScopeMultimodalGeneration:
		return observeDashScopeImageGenerationRequest(body, maxImages, allowedSizes)
	default:
		return Result{}, fmt.Errorf("unsupported image generation protocol %q", protocol)
	}
}

func observeOpenAIImageGenerationRequest(body []byte, maxImages int, allowedSizes []string) (Result, error) {
	request, err := decodeUniqueImageObject(body)
	if err != nil {
		return Result{}, fmt.Errorf("invalid image generation JSON: %w", err)
	}
	for _, name := range []string{"image", "images", "input_image", "input_images", "mask", "reference_image", "reference_images"} {
		if _, exists := request[name]; exists {
			return Result{}, fmt.Errorf("image edit/reference field %q is unsupported", name)
		}
	}
	for _, name := range []string{"stream", "async", "background"} {
		if _, exists := request[name]; exists {
			return Result{}, fmt.Errorf("%s image generation is unsupported", name)
		}
	}
	allowedFields := map[string]struct{}{
		"model": {}, "prompt": {}, "n": {}, "size": {},
	}
	for name := range request {
		if _, allowed := allowedFields[name]; !allowed {
			return Result{}, fmt.Errorf("image generation field %q is unsupported", name)
		}
	}
	n := int64(1)
	if raw, exists := request["n"]; exists {
		var number json.Number
		if err := json.Unmarshal(raw, &number); err != nil {
			return Result{}, fmt.Errorf("image count must be an integer")
		}
		value, err := number.Int64()
		if err != nil {
			return Result{}, fmt.Errorf("image count must be an integer")
		}
		n = value
	}
	if n < 1 || n > int64(maxImages) {
		return Result{}, fmt.Errorf("image count must be between 1 and %d", maxImages)
	}
	rawSize, exists := request["size"]
	if !exists {
		return Result{}, fmt.Errorf("image size is required")
	}
	var size string
	if err := json.Unmarshal(rawSize, &size); err != nil {
		return Result{}, fmt.Errorf("image size must be a string")
	}
	allowed := false
	for _, candidate := range allowedSizes {
		if size == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return Result{}, fmt.Errorf("image size is not allowed")
	}
	result := requestResult("relay_request")
	result.set(Measurement{Meter: RequestedImages, Value: count(n), Source: "request_n", Completeness: Exact})
	return result, nil
}

func observeDashScopeImageGenerationRequest(body []byte, maxImages int, allowedSizes []string) (Result, error) {
	request, err := decodeUniqueImageObject(body)
	if err != nil {
		return Result{}, fmt.Errorf("invalid DashScope image generation JSON: %w", err)
	}
	if err := requireOnlyFields(request, "DashScope image generation", "model", "input", "parameters"); err != nil {
		return Result{}, err
	}
	if err := requireStringField(request, "model", "DashScope image model"); err != nil {
		return Result{}, err
	}
	if err := validateDashScopeTextInput(request["input"]); err != nil {
		return Result{}, err
	}

	parameters, err := decodeRequiredObject(request, "parameters", "DashScope image parameters")
	if err != nil {
		return Result{}, err
	}
	if err := requireOnlyFields(parameters, "DashScope image parameters", "size", "n", "watermark"); err != nil {
		return Result{}, err
	}
	var number json.Number
	rawN, exists := parameters["n"]
	if !exists || json.Unmarshal(rawN, &number) != nil {
		return Result{}, fmt.Errorf("DashScope image count must be an explicit integer")
	}
	n, err := number.Int64()
	if err != nil {
		return Result{}, fmt.Errorf("DashScope image count must be an explicit integer")
	}
	if n < 1 || n > int64(maxImages) {
		return Result{}, fmt.Errorf("image count must be between 1 and %d", maxImages)
	}
	var wireSize string
	if rawSize, ok := parameters["size"]; !ok || json.Unmarshal(rawSize, &wireSize) != nil {
		return Result{}, fmt.Errorf("DashScope image size must be an explicit string")
	}
	canonicalSize := strings.ReplaceAll(wireSize, "*", "x")
	if canonicalSize == wireSize || !containsString(allowedSizes, canonicalSize) {
		return Result{}, fmt.Errorf("image size is not allowed")
	}
	var watermark bool
	if rawWatermark, ok := parameters["watermark"]; !ok || json.Unmarshal(rawWatermark, &watermark) != nil || watermark {
		return Result{}, fmt.Errorf("DashScope image watermark must be explicitly false")
	}

	result := requestResult("relay_request")
	result.set(Measurement{Meter: RequestedImages, Value: count(n), Source: "request_parameters_n", Completeness: Exact})
	return result, nil
}

func validateDashScopeTextInput(raw json.RawMessage) error {
	input, err := decodeNamedObject(raw, "DashScope image input")
	if err != nil {
		return err
	}
	if err := requireOnlyFields(input, "DashScope image input", "messages"); err != nil {
		return err
	}
	var messages []json.RawMessage
	if rawMessages, ok := input["messages"]; !ok || json.Unmarshal(rawMessages, &messages) != nil || len(messages) != 1 {
		return fmt.Errorf("DashScope image input must contain exactly one message")
	}
	message, err := decodeNamedObject(messages[0], "DashScope image message")
	if err != nil {
		return err
	}
	if err := requireOnlyFields(message, "DashScope image message", "role", "content"); err != nil {
		return err
	}
	var role string
	if rawRole, ok := message["role"]; !ok || json.Unmarshal(rawRole, &role) != nil || role != "user" {
		return fmt.Errorf("DashScope image message role must be user")
	}
	var content []json.RawMessage
	if rawContent, ok := message["content"]; !ok || json.Unmarshal(rawContent, &content) != nil || len(content) != 1 {
		return fmt.Errorf("DashScope image message must contain exactly one text item")
	}
	item, err := decodeNamedObject(content[0], "DashScope image content")
	if err != nil {
		return err
	}
	if err := requireOnlyFields(item, "DashScope image content", "text"); err != nil {
		return err
	}
	return requireStringField(item, "text", "DashScope image prompt")
}

func decodeRequiredObject(parent map[string]json.RawMessage, field string, label string) (map[string]json.RawMessage, error) {
	raw, ok := parent[field]
	if !ok {
		return nil, fmt.Errorf("%s is required", label)
	}
	return decodeNamedObject(raw, label)
}

func decodeNamedObject(raw json.RawMessage, label string) (map[string]json.RawMessage, error) {
	value, err := decodeUniqueImageObject(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be one JSON object: %w", label, err)
	}
	return value, nil
}

func requireOnlyFields(value map[string]json.RawMessage, label string, fields ...string) error {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for field := range value {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%s field %q is unsupported", label, field)
		}
	}
	return nil
}

func requireStringField(value map[string]json.RawMessage, field string, label string) error {
	var text string
	raw, ok := value[field]
	if !ok || json.Unmarshal(raw, &text) != nil {
		return fmt.Errorf("%s must be a string", label)
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func decodeUniqueImageObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, fmt.Errorf("image generation request must be one JSON object")
	}
	request := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("image generation object key must be a string")
		}
		if _, exists := request[key]; exists {
			return nil, fmt.Errorf("duplicate image generation field %q", key)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		request[key] = raw
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return nil, fmt.Errorf("image generation request must be one JSON object")
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, fmt.Errorf("image generation request must be one JSON object")
	}
	return request, nil
}
