package runtimecontrol_test

import (
	"context"
	"reflect"
	"testing"

	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// RLY-ADM-004: a retained binding never grants access to a disabled resource.
func TestDisabledResourcesStayUnroutableAcrossActivations(t *testing.T) {
	for _, providerDisabled := range []bool{false, true} {
		name := "resources"
		if providerDisabled {
			name = "provider"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			_, svc, relay, server, _, adminID, upstreamID, _ := newRuntimeControlEnv(t)
			defer server.Close()
			draft, err := svc.Capability.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			content := draft.Content
			content.Policy.DefaultModelId = nil
			content.Providers[0].Enabled = !providerDisabled
			content.Models[0].Enabled = providerDisabled
			if !providerDisabled {
				ttsID, asrID, mcpID := platformid.New(platformid.TTS), platformid.New(platformid.ASR), platformid.New(platformid.MCP)
				content.Tts = []adminapi.TtsDefinition{{TtsId: ttsID, DisplayName: "TTS", UpstreamModelKey: "tts", ClientProtocol: "OPENAI_AUDIO_SPEECH", RuntimePath: "/v1/audio/speech"}}
				content.Asr = []adminapi.AsrDefinition{{AsrId: asrID, DisplayName: "ASR", UpstreamModelKey: "asr", ClientProtocol: "OPENAI_AUDIO_TRANSCRIPTIONS", RuntimePath: "/v1/audio/transcriptions"}}
				content.Mcp = []adminapi.McpDefinition{{ToolAccessMode: new(adminapi.McpDefinitionToolAccessMode("ALL")), AllowedTools: &[]adminapi.McpToolGrant{}, McpServerId: mcpID, DisplayName: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", RuntimePath: "/mcp"}}
				for _, id := range []string{ttsID, asrID, mcpID} {
					binding := content.Bindings[0]
					binding.ResourceId, binding.RuntimeRouteId = id, platformid.New(platformid.Route)
					if id == asrID {
						binding.TransportPolicy = "HTTP_MULTIPART"
					}
					switch id {
					case ttsID:
						binding.AllowedPathPrefixes = []string{"/v1/audio/speech"}
					case asrID:
						binding.AllowedPathPrefixes = []string{"/v1/audio/transcriptions"}
					case mcpID:
						binding.AllowedPathPrefixes = []string{"/mcp"}
						binding.AllowedMethods = []string{"POST", "GET", "DELETE"}
					}
					content.Bindings = append(content.Bindings, binding)
				}
			}
			draft, err = svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, content)
			if err != nil {
				t.Fatal(err)
			}
			first := publishAndFinalize(t, svc, adminID, draft.DraftRevision)
			assertClosed := func(stage string) {
				t.Helper()
				state := relay.Current()
				if state == nil {
					t.Fatal("relay not ready")
				}
				if len(state.Resources) != 0 || len(state.Routes) != 0 {
					t.Errorf("%s exposed disabled resources: %v", stage, state.Resources)
				}
			}
			assertClosed("publish")
			if _, err := svc.ApplyUpstream(ctx, adminID, platformid.New(platformid.Idempotency), upstreamID); err != nil {
				t.Fatal(err)
			}
			assertClosed("upstream apply")
			if _, err := svc.Republish(ctx, adminID, platformid.New(platformid.Idempotency), first.ReleaseID); err != nil {
				t.Fatal(err)
			}
			assertClosed("republish")
		})
	}
}

func TestPublishTimeoutPolicyOverridesWholeUpstreamDefault(t *testing.T) {
	ctx := context.Background()
	_, svc, relay, server, _, adminID, upstreamID, _ := newRuntimeControlEnv(t)
	defer server.Close()
	up, err := svc.Upstream.GetUpstream(ctx, upstreamID)
	if err != nil {
		t.Fatal(err)
	}
	general := adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 300000, IdleMs: 120000}
	up.Config.TimeoutDefaults = general
	up, err = svc.Upstream.UpdateUpstream(ctx, adminID, upstreamID, up.ConfigRevision, up.Config)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture owns the active revision; publishing still crosses the real Hub -> Relay HTTP boundary.
	if err := svc.Client.Upstream.UpdateOneID(upstreamID).SetActiveConfigRevision(int64(up.ConfigRevision)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		override *adminapi.TimeoutPolicy
		want     adminapi.TimeoutPolicy
	}{
		{"inherit", nil, general},
		{"local", &adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 300000, IdleMs: 120000}, adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 300000, IdleMs: 120000}},
		{"images", &adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 600000, IdleMs: 120000}, adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 600000, IdleMs: 120000}},
		{"mimo", &adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 60000, IdleMs: 60000}, adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 60000, IdleMs: 60000}},
		{"firecrawl", &adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 60000, IdleMs: 120000}, adminapi.TimeoutPolicy{ConnectMs: 5000, ResponseHeaderMs: 60000, IdleMs: 120000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft, err := svc.Capability.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			draft.Content.Bindings[0].TimeoutPolicy = tc.override
			draft, err = svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
			if err != nil {
				t.Fatal(err)
			}
			publishAndFinalize(t, svc, adminID, draft.DraftRevision)
			got := relay.Current().Routes[draft.Content.Bindings[0].RuntimeRouteId].TimeoutPolicy
			if got.ConnectMs != tc.want.ConnectMs || got.ResponseHeaderMs != tc.want.ResponseHeaderMs || got.IdleMs != tc.want.IdleMs || !reflect.DeepEqual(got.OverallMs, tc.want.OverallMs) {
				t.Fatalf("applied timeout = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPublishPreservesOverallTimeout(t *testing.T) {
	ctx := context.Background()
	_, svc, relay, server, _, adminID, _, _ := newRuntimeControlEnv(t)
	defer server.Close()
	draft, err := svc.Capability.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	overall := 15000
	draft.Content.Bindings[0].TimeoutPolicy = &adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 2000, IdleMs: 3000, OverallMs: &overall}
	draft, err = svc.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	publishAndFinalize(t, svc, adminID, draft.DraftRevision)
	route := relay.Current().Routes[draft.Content.Bindings[0].RuntimeRouteId]
	if route.TimeoutPolicy.OverallMs == nil || *route.TimeoutPolicy.OverallMs != overall {
		t.Fatalf("published overall timeout = %v, want %d", route.TimeoutPolicy.OverallMs, overall)
	}
}
