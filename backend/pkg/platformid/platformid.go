package platformid

import "errors"

type Kind string

const (
	Deployment Kind = "dep"
	User Kind = "usr"
	Device Kind = "dev"
	Enrollment Kind = "enr"
	Session Kind = "ses"
	Installation Kind = "ins"
	Provider Kind = "prv"
	Model Kind = "mdl"
	TTS Kind = "tts"
	ASR Kind = "asr"
	MCP Kind = "mcp"
	Policy Kind = "pol"
	Draft Kind = "drf"
	Release Kind = "rel"
	Upstream Kind = "ups"
	Secret Kind = "sec"
	Route Kind = "rte"
	Activation Kind = "act"
	Request Kind = "req"
	Interaction Kind = "int"
	Idempotency Kind = "idem"
	UsageEvent Kind = "usg"
	PricingRule Kind = "prc"
)

var ErrInvalid = errors.New("invalid platform id")

// New is intentionally incomplete in the Red commit. The first TDD check proves
// the ID contract before the implementation is added.
func New(kind Kind) string { return "" }

func Validate(kind Kind, value string) error { return ErrInvalid }
func KindOf(value string) (Kind, error) { return "", ErrInvalid }
