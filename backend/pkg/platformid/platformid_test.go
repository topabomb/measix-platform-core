package platformid_test

import (
	"measix/platform/pkg/platformid"
	"testing"
)

func TestCanonicalKindsGenerateAndValidateUUIDv4(t *testing.T) {
	kinds := []platformid.Kind{platformid.Deployment, platformid.User, platformid.Device, platformid.Enrollment, platformid.Session, platformid.Installation, platformid.Provider, platformid.Model, platformid.TTS, platformid.ASR, platformid.MCP, platformid.Policy, platformid.Draft, platformid.Release, platformid.Upstream, platformid.Secret, platformid.Route, platformid.Activation, platformid.Request, platformid.Interaction, platformid.Idempotency, platformid.UsageEvent, platformid.PricingRule}
	for _, kind := range kinds {
		id := platformid.New(kind)
		if err := platformid.Validate(kind, id); err != nil {
			t.Fatalf("Validate(%q,%q): %v", kind, id, err)
		}
		got, err := platformid.KindOf(id)
		if err != nil || got != kind {
			t.Fatalf("KindOf(%q)=%q,%v; want %q", id, got, err, kind)
		}
	}
}
func TestRejectsWrongPrefixAndNonV4UUID(t *testing.T) {
	cases := []struct {
		kind platformid.Kind
		id   string
	}{{platformid.User, "dev_550e8400-e29b-41d4-a716-446655440000"}, {platformid.User, "usr_550e8400-e29b-11d4-a716-446655440000"}, {platformid.User, "usr_550E8400-e29b-41d4-a716-446655440000"}, {platformid.User, "usr_not-a-uuid"}}
	for _, tc := range cases {
		if err := platformid.Validate(tc.kind, tc.id); err == nil {
			t.Fatalf("Validate(%q,%q) unexpectedly succeeded", tc.kind, tc.id)
		}
	}
}
