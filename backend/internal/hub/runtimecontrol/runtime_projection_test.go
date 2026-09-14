package runtimecontrol_test

import (
	"context"
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
				content.Mcp = []adminapi.McpDefinition{{McpServerId: mcpID, DisplayName: "MCP", ClientProtocol: "MCP_STREAMABLE_HTTP", AuthOwnership: "ENTERPRISE_MANAGED", RuntimePath: "/mcp"}}
				for _, id := range []string{ttsID, asrID, mcpID} {
					binding := content.Bindings[0]
					binding.ResourceId, binding.RuntimeRouteId = id, platformid.New(platformid.Route)
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
				if len(state.ResourceRoutes) != 0 || len(state.Routes) != 0 {
					t.Errorf("%s exposed disabled resources: %v", stage, state.ResourceRoutes)
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
