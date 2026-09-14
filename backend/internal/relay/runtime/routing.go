package runtime

import (
	"net/url"
	"strings"

	"measix/platform/pkg/platformid"
)

const runtimePrefix = "/runtime/v1/resources/"

func runtimeTarget(path string) (resourceID, runtimePath string, ok bool) {
	if !strings.HasPrefix(path, runtimePrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(path, runtimePrefix)
	separator := strings.IndexByte(remainder, '/')
	if separator <= 0 {
		return "", "", false
	}
	return remainder[:separator], remainder[separator:], true
}

func isRuntimeResourceID(value string) bool {
	kind, err := platformid.KindOf(value)
	if err != nil {
		return false
	}
	switch kind {
	case platformid.Model, platformid.TTS, platformid.ASR, platformid.MCP:
		return true
	default:
		return false
	}
}

func safeRuntimePath(value string) bool {
	decoded, ok := fullyUnescapePath(value)
	if !ok || !strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "//") || strings.Contains(decoded, "\\") {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// fullyUnescapePath closes encoded and repeatedly encoded traversal variants without
// changing the path that is ultimately forwarded. net/http has already decoded one
// layer into URL.Path, so a bounded fixed-point decode is required for %252e-style
// inputs while also preventing pathological expansion.
func fullyUnescapePath(value string) (string, bool) {
	decoded := value
	for range 4 {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return "", false
		}
		if next == decoded {
			return decoded, true
		}
		decoded = next
	}
	if strings.Contains(decoded, "%") {
		return "", false
	}
	return decoded, true
}

func allowedPath(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix)) || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func targetURL(base *url.URL, runtimePath, rawQuery string) *url.URL {
	target := *base
	target.Path = strings.TrimRight(base.Path, "/") + runtimePath
	target.RawPath = ""
	target.RawQuery = rawQuery
	target.Fragment = ""
	return &target
}
