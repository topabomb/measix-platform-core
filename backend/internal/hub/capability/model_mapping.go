package capability

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"measix/platform/internal/wire/adminapi"
)

var publishedModelKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func EffectivePublishedModelKey(model adminapi.ModelDefinition) string {
	if model.PublishedModelKey != nil && strings.TrimSpace(*model.PublishedModelKey) != "" {
		return *model.PublishedModelKey
	}
	return model.UpstreamModelKey
}

func ValidPublishedModelKey(value string) bool {
	return publishedModelKeyPattern.MatchString(value)
}

// ModelClientRuntimePath returns the path published to clients. Only Google
// carries its model selector in the path; the other current model protocols
// carry it in the top-level JSON body and retain the upstream path.
func ModelClientRuntimePath(protocol adminapi.ProviderDefinitionClientProtocol, upstreamPath, upstreamModelKey, publishedModelKey string) (string, error) {
	if protocol != adminapi.ProviderDefinitionClientProtocolGOOGLEGENERATECONTENT {
		return upstreamPath, nil
	}
	const marker = "/models/"
	markerIndex := strings.LastIndex(upstreamPath, marker)
	if markerIndex < 0 {
		return "", fmt.Errorf("Google runtimePath must contain /models/{model}:action")
	}
	segmentStart := markerIndex + len(marker)
	remainder := upstreamPath[segmentStart:]
	colon := strings.IndexByte(remainder, ':')
	if colon <= 0 || strings.Contains(remainder[:colon], "/") {
		return "", fmt.Errorf("Google runtimePath must contain one model segment followed by an action")
	}
	action := remainder[colon:]
	if action != ":generateContent" && action != ":streamGenerateContent" {
		return "", fmt.Errorf("Google runtimePath action must be generateContent or streamGenerateContent")
	}
	decoded, err := url.PathUnescape(remainder[:colon])
	if err != nil || decoded != upstreamModelKey {
		return "", fmt.Errorf("Google runtimePath model must match upstreamModelKey")
	}
	return upstreamPath[:segmentStart] + url.PathEscape(publishedModelKey) + action, nil
}
