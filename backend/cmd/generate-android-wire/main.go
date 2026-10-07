// Command generate-android-wire exports the Client Control OpenAPI as a deterministic
// schema-only artifact for Android DTO generation. The Android repository may keep its
// existing OkHttp + kotlinx.serialization runtime while consuming this executable wire input.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/getkin/kin-openapi/openapi3"
)

type manifest struct {
	contractIdentity
	Generated  bool   `json:"generated"`
	Source     string `json:"source"`
	SourceHash string `json:"sourceHash"`
	Format     string `json:"format"`
}

type contractIdentity struct {
	PlatformContractVersion           int    `json:"platformContractVersion"`
	SupportedPlatformContractVersions []int  `json:"supportedPlatformContractVersions"`
	CoreBaselineVersion               string `json:"coreBaselineVersion"`
	BaselineHash                      string `json:"baselineHash"`
}

func loadContractIdentity(raw []byte) (contractIdentity, error) {
	var identity contractIdentity
	if err := json.Unmarshal(raw, &identity); err != nil {
		return identity, err
	}
	if identity.PlatformContractVersion < 1 || !regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`).MatchString(identity.CoreBaselineVersion) {
		return identity, fmt.Errorf("invalid platform contract or Core baseline version")
	}
	found := false
	previous := 0
	for _, version := range identity.SupportedPlatformContractVersions {
		if version <= previous {
			return identity, fmt.Errorf("platform contract support must be positive, unique and sorted")
		}
		previous = version
		found = found || version == identity.PlatformContractVersion
	}
	if !found {
		return identity, fmt.Errorf("platform contract support omits main contract")
	}
	identity.BaselineHash = sourceHash(raw)
	return identity, nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: generate-android-wire <source.openapi.yaml> <output.openapi.yaml> <manifest.json>")
		os.Exit(2)
	}
	source, output, manifestPath := os.Args[1], os.Args[2], os.Args[3]
	raw, err := os.ReadFile(source)
	must(err)
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(raw)
	must(err)
	must(doc.Validate(context.Background()))
	baseline, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "protocol-baseline.json"))
	must(err)
	identity, err := loadContractIdentity(baseline)
	must(err)

	payload := append([]byte(nil), raw...)
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		payload = append(payload, '\n')
	}
	meta, err := json.MarshalIndent(manifest{
		contractIdentity: identity,
		Generated:        true,
		Source:           filepath.ToSlash(source),
		SourceHash:       sourceHash(raw),
		Format:           "openapi-3.0.3-client-schema-only",
	}, "", "  ")
	must(err)
	meta = append(meta, '\n')
	must(os.MkdirAll(filepath.Dir(output), 0o755))
	must(os.WriteFile(output, payload, 0o644))
	must(os.WriteFile(manifestPath, meta, 0o644))
	must(os.WriteFile(filepath.Join(filepath.Dir(manifestPath), "protocol-baseline.json"), bytes.ReplaceAll(baseline, []byte("\r\n"), []byte("\n")), 0o644))
}

// sourceHash computes a deterministic source hash that is insensitive to CRLF/LF
// line endings, so the generated manifest is identical whether produced on Windows
// or Linux/CI. sha256 is applied to the raw bytes with CRLF normalized to LF.
func sourceHash(raw []byte) string {
	normalized := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(normalized)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
