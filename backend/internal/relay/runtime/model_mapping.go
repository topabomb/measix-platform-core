package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
)

var (
	errInvalidModelSelector       = errors.New("invalid model selector")
	errInvalidModelRequest        = errors.New("invalid model request")
	errUnsupportedContentEncoding = errors.New("unsupported model request content encoding")
)

func prepareModelRequest(resource control.Resource, request *http.Request, runtimePath string, maxBytes int64) (string, error) {
	mapping := resource.ModelMapping
	if resource.Kind != relaycontrolapi.ResourceRouteResourceKindMODEL || mapping == nil {
		return runtimePath, nil
	}
	if runtimePath != mapping.ClientRuntimePath {
		return "", errInvalidModelSelector
	}
	if resource.ClientProtocol == relaycontrolapi.GOOGLEGENERATECONTENT {
		return mapping.UpstreamRuntimePath, nil
	}
	if request.Method != http.MethodPost {
		return "", errInvalidModelRequest
	}
	encoding := strings.TrimSpace(request.Header.Get("Content-Encoding"))
	if encoding != "" && !strings.EqualFold(encoding, "identity") {
		return "", errUnsupportedContentEncoding
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || request.Body == nil {
		return "", errInvalidModelRequest
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
	if err != nil {
		return "", errInvalidModelRequest
	}
	_ = request.Body.Close()
	if int64(len(body)) > maxBytes {
		return "", errObservedRequestTooLarge
	}
	rewritten, err := rewriteTopLevelModel(body, mapping.PublishedModelKey, mapping.UpstreamModelKey)
	if err != nil {
		return "", err
	}
	request.Body = io.NopCloser(bytes.NewReader(rewritten))
	request.ContentLength = int64(len(rewritten))
	request.Header.Del("Content-Encoding")
	request.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	return mapping.UpstreamRuntimePath, nil
}

func rewriteTopLevelModel(body []byte, published, upstream string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errInvalidModelRequest
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return nil, errInvalidModelRequest
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, errInvalidModelRequest
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errInvalidModelRequest
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errInvalidModelRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errInvalidModelRequest
	}
	modelValue, exists := fields["model"]
	if !exists {
		return nil, errInvalidModelSelector
	}
	var model string
	if json.Unmarshal(modelValue, &model) != nil || model != published {
		return nil, errInvalidModelSelector
	}
	encoded, _ := json.Marshal(upstream)
	fields["model"] = encoded
	rewritten, err := json.Marshal(fields)
	if err != nil {
		return nil, errInvalidModelRequest
	}
	return rewritten, nil
}
