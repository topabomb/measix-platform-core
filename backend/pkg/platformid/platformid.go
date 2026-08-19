package platformid

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

type Kind string

const (
	Deployment   Kind = "dep"
	User         Kind = "usr"
	Device       Kind = "dev"
	Enrollment   Kind = "enr"
	Session      Kind = "ses"
	Installation Kind = "ins"
	Provider     Kind = "prv"
	Model        Kind = "mdl"
	TTS          Kind = "tts"
	ASR          Kind = "asr"
	MCP          Kind = "mcp"
	Policy       Kind = "pol"
	Draft        Kind = "drf"
	Release      Kind = "rel"
	Upstream     Kind = "ups"
	Secret       Kind = "sec"
	Route        Kind = "rte"
	Activation   Kind = "act"
	Request      Kind = "req"
	Interaction  Kind = "int"
	Idempotency  Kind = "idem"
	UsageEvent   Kind = "usg"
	PricingRule  Kind = "prc"
)

var ErrInvalid = errors.New("invalid platform id")

var known = map[Kind]struct{}{
	Deployment: {}, User: {}, Device: {}, Enrollment: {}, Session: {}, Installation: {},
	Provider: {}, Model: {}, TTS: {}, ASR: {}, MCP: {}, Policy: {}, Draft: {}, Release: {},
	Upstream: {}, Secret: {}, Route: {}, Activation: {}, Request: {}, Interaction: {},
	Idempotency: {}, UsageEvent: {}, PricingRule: {},
}

func New(kind Kind) string {
	if _, ok := known[kind]; !ok {
		panic("unknown platform id kind: " + string(kind))
	}
	return string(kind) + "_" + uuid.New().String()
}

func Validate(kind Kind, value string) error {
	if _, ok := known[kind]; !ok {
		return ErrInvalid
	}
	prefix := string(kind) + "_"
	if !strings.HasPrefix(value, prefix) {
		return ErrInvalid
	}
	raw := strings.TrimPrefix(value, prefix)
	if raw != strings.ToLower(raw) {
		return ErrInvalid
	}
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed.Version() != 4 || parsed.String() != raw {
		return ErrInvalid
	}
	return nil
}

func KindOf(value string) (Kind, error) {
	idx := strings.IndexByte(value, '_')
	if idx <= 0 {
		return "", ErrInvalid
	}
	kind := Kind(value[:idx])
	if err := Validate(kind, value); err != nil {
		return "", err
	}
	return kind, nil
}
