package protocolusage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ObserveImageGenerationRequest validates only the provider-neutral execution
// controls owned by Relay. The prompt remains opaque and the original payload
// is never rewritten or retained.
func ObserveImageGenerationRequest(body []byte, maxImages int, allowedSizes []string) (Result, error) {
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
