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
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
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
