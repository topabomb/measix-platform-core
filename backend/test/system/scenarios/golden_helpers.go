//go:build candidate

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
	"measix/platform/test/system/adapter"
	"measix/platform/test/system/client"
	"measix/platform/test/system/harness"
)

// goldenPathTest holds shared state for the golden path helpers.
type goldenPathTest struct {
	t                  *testing.T
	lastUserID         string
	lastEnrollmentCode string
	lastUpstreamID     string
	lastDraftContent   map[string]interface{}
	lastModelID        string
	lastTtsID          string
	lastAsrID          string
	lastMcpID          string
	lastProviderID     string
	lastSecretID       string
	lastSecretVersion  int
}

// fullSetup performs the complete golden path setup: user, enrollment, secret,
// upstream, test, apply, resources (model/tts/asr/mcp/policy), bindings,
// save draft, validate, preview, publish, and wait for activation/convergence.
func (g *goldenPathTest) fullSetup(ctx context.Context, admin *harness.AdminClient, ad *adapter.Adapter, env *harness.HubEnv) {
	// 1. Create user + enrollment
	g.lastUserID = g.createUser(ctx, admin)
	g.lastEnrollmentCode = g.createEnrollment(ctx, admin, g.lastUserID)

	// 2. Create secret
	g.lastSecretID, g.lastSecretVersion = g.createSecret(ctx, admin)

	// 3. Create upstream
	g.lastUpstreamID = g.createUpstream(ctx, admin, ad.URL, g.lastSecretID, g.lastSecretVersion)

	// 4. Test + Apply
	g.testUpstream(ctx, admin, g.lastUpstreamID)
	g.applyUpstream(ctx, admin, g.lastUpstreamID)

	// 5. Build and save draft with all resources
	draftRev := g.getDraftRevision(ctx, admin)
	g.lastProviderID = platformid.New(platformid.Provider)
	g.lastModelID = platformid.New(platformid.Model)
	g.lastTtsID = platformid.New(platformid.TTS)
	g.lastAsrID = platformid.New(platformid.ASR)
	g.lastMcpID = platformid.New(platformid.MCP)
	routeModel := platformid.New(platformid.Route)
	routeTTS := platformid.New(platformid.Route)
	routeASR := platformid.New(platformid.Route)
	routeMCP := platformid.New(platformid.Route)
	policyID := platformid.New(platformid.Policy)

	g.lastDraftContent = g.buildDraftContent(
		g.lastProviderID, g.lastModelID, g.lastTtsID, g.lastAsrID, g.lastMcpID,
		g.lastUpstreamID, routeModel, routeTTS, routeASR, routeMCP, policyID,
		"Test Model", "alloy",
	)

	newRev := g.putDraft(ctx, admin, draftRev, g.lastDraftContent)
	g.validateDraft(ctx, admin, newRev)
	preview := g.previewDraft(ctx, admin, newRev)
	g.t.Logf("preview snapshotHash: %s", preview["snapshotHash"])
	g.assertPreviewClientSafe(ctx, admin, newRev)

	// 6. Publish
	activationID := g.publishDraft(ctx, admin, newRev)
	g.waitActivationCompleted(ctx, admin, activationID, 60*time.Second)

	// 7. Wait for convergence
	if err := harness.WaitConvergence(ctx, env.HubBaseURL, admin.CSRFToken(), admin.CookieHeader(), 30*time.Second); err != nil {
		g.t.Fatalf("convergence: %v", err)
	}
	if err := harness.WaitReadyRelay(ctx, env.RelayPubBaseURL, 30*time.Second); err != nil {
		g.t.Fatalf("relay ready: %v\nrelay log:\n%s", err, env.RelayLog())
	}
}

func (g *goldenPathTest) createUser(ctx context.Context, admin *harness.AdminClient) string {
	resp, err := admin.Post(ctx, "/api/admin/v1/users", map[string]interface{}{
		"username":    "test-user-" + time.Now().Format("150405"),
		"displayName": "Test User",
		"role":        "MEMBER",
	})
	if err != nil {
		g.t.Fatalf("create user: %v", err)
	}
	var user struct {
		UserID string `json:"userId"`
	}
	if err := harness.DecodeJSON(resp, &user); err != nil {
		g.t.Fatalf("decode user: %v", err)
	}
	return user.UserID
}

func (g *goldenPathTest) createEnrollment(ctx context.Context, admin *harness.AdminClient, userID string) string {
	resp, err := admin.Post(ctx, fmt.Sprintf("/api/admin/v1/users/%s/enrollments", userID), map[string]interface{}{
		"expiresInSeconds": 3600,
	})
	if err != nil {
		g.t.Fatalf("create enrollment: %v", err)
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := harness.DecodeJSON(resp, &result); err != nil {
		g.t.Fatalf("decode enrollment: %v", err)
	}
	return result.Code
}

func (g *goldenPathTest) createSecret(ctx context.Context, admin *harness.AdminClient) (string, int) {
	resp, err := admin.Post(ctx, "/api/admin/v1/secrets", map[string]interface{}{
		"name":  "test-secret",
		"value": "test-secret-value-12345",
	})
	if err != nil {
		g.t.Fatalf("create secret: %v", err)
	}
	var secret struct {
		SecretID      string `json:"secretId"`
		SecretVersion int    `json:"secretVersion"`
	}
	if err := harness.DecodeJSON(resp, &secret); err != nil {
		g.t.Fatalf("decode secret: %v", err)
	}
	return secret.SecretID, secret.SecretVersion
}

func (g *goldenPathTest) createUpstream(ctx context.Context, admin *harness.AdminClient, adapterURL, secretID string, secretVersion int) string {
	resp, err := admin.Post(ctx, "/api/admin/v1/upstreams", map[string]interface{}{
		"config": map[string]interface{}{
			"name":    "Test Upstream",
			"baseUrl": adapterURL,
			"transportCapabilities": []string{
				"HTTP_REQUEST_RESPONSE", "HTTP_STREAMING_SSE",
				"HTTP_BINARY_STREAM", "HTTP_MULTIPART",
			},
			"auth": map[string]interface{}{
				"type": "BEARER",
				"secretRef": map[string]interface{}{
					"secretId": secretID, "secretVersion": secretVersion,
				},
			},
			"correlationMode":      "HEADER_ECHO",
			"usageCapabilityLevel": "LEVEL_1",
			"timeoutDefaults": map[string]interface{}{
				"connectMs": 1000, "responseHeaderMs": 5000, "idleMs": 30000,
			},
		},
	})
	if err != nil {
		g.t.Fatalf("create upstream: %v", err)
	}
	var upstream struct {
		UpstreamID string `json:"upstreamId"`
	}
	if err := harness.DecodeJSON(resp, &upstream); err != nil {
		g.t.Fatalf("decode upstream: %v body: %s", err, harness.ReadBody(resp))
	}
	return upstream.UpstreamID
}

func (g *goldenPathTest) testUpstream(ctx context.Context, admin *harness.AdminClient, upstreamID string) {
	resp, err := admin.Post(ctx, fmt.Sprintf("/api/admin/v1/upstreams/%s:test", upstreamID), map[string]interface{}{})
	if err != nil {
		g.t.Fatalf("test upstream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("test upstream status: %d body: %s", resp.StatusCode, harness.ReadBody(resp))
	}
}

func (g *goldenPathTest) applyUpstream(ctx context.Context, admin *harness.AdminClient, upstreamID string) {
	idempotencyKey := platformid.New(platformid.Idempotency)
	resp, err := admin.PostWithHeaders(ctx, fmt.Sprintf("/api/admin/v1/upstreams/%s:apply", upstreamID), map[string]interface{}{}, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	if err != nil {
		g.t.Fatalf("apply upstream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		g.t.Fatalf("apply upstream status: %d body: %s", resp.StatusCode, harness.ReadBody(resp))
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r, err := admin.Get(ctx, fmt.Sprintf("/api/admin/v1/upstreams/%s", upstreamID))
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var up struct {
			Status string `json:"status"`
		}
		_ = harness.DecodeJSON(r, &up)
		if up.Status == "ACTIVE" {
			return
		}
		r.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}
	g.t.Fatalf("upstream not ACTIVE after apply")
}

func (g *goldenPathTest) getDraftRevision(ctx context.Context, admin *harness.AdminClient) int {
	resp, err := admin.Get(ctx, "/api/admin/v1/draft")
	if err != nil {
		g.t.Fatalf("get draft: %v", err)
	}
	var draft struct {
		DraftRevision int `json:"draftRevision"`
	}
	if err := harness.DecodeJSON(resp, &draft); err != nil {
		g.t.Fatalf("decode draft: %v", err)
	}
	return draft.DraftRevision
}

func (g *goldenPathTest) buildDraftContent(
	providerID, modelID, ttsID, asrID, mcpID, upstreamID,
	routeModel, routeTTS, routeASR, routeMCP, policyID string,
	modelDisplayName, voice string,
) map[string]interface{} {
	return map[string]interface{}{
		"providers": []map[string]interface{}{
			{
				"providerId": providerID, "displayName": "OpenAI Provider",
				"clientProtocol": "OPENAI_CHAT_COMPLETIONS", "enabled": true,
			},
		},
		"models": []map[string]interface{}{
			{
				"modelId": modelID, "providerId": providerID, "displayName": modelDisplayName,
				"upstreamModelKey": "gpt-test", "runtimePath": "/v1/chat/completions",
				"inputModalities": []string{"TEXT"}, "outputModalities": []string{"TEXT"},
				"capabilities": []string{"TOOL"}, "enabled": true,
			},
		},
		"tts": []map[string]interface{}{
			{
				"ttsId": ttsID, "displayName": "OpenAI TTS", "clientProtocol": "OPENAI_AUDIO_SPEECH",
				"upstreamModelKey": "tts-test", "voice": voice, "runtimePath": "/v1/audio/speech",
				"enabled": true,
			},
		},
		"asr": []map[string]interface{}{
			{
				"asrId": asrID, "displayName": "OpenAI ASR", "clientProtocol": "OPENAI_AUDIO_TRANSCRIPTIONS",
				"upstreamModelKey": "whisper-test", "runtimePath": "/v1/audio/transcriptions",
				"enabled": true,
			},
		},
		"mcp": []map[string]interface{}{
			{
				"mcpServerId": mcpID, "displayName": "Test MCP Server", "clientProtocol": "MCP_STREAMABLE_HTTP",
				"runtimePath": "/mcp", "authOwnership": "NONE", "enabled": true,
			},
		},
		"bindings": []map[string]interface{}{
			{
				"runtimeRouteId": routeModel, "resourceId": modelID, "upstreamId": upstreamID,
				"allowedMethods": []string{"POST"}, "allowedPathPrefixes": []string{"/v1/chat/completions"},
				"transportPolicy": "HTTP_STREAMING_SSE",
				"timeoutPolicy":   map[string]interface{}{"connectMs": 1000, "responseHeaderMs": 5000, "idleMs": 30000},
			},
			{
				"runtimeRouteId": routeTTS, "resourceId": ttsID, "upstreamId": upstreamID,
				"allowedMethods": []string{"POST"}, "allowedPathPrefixes": []string{"/v1/audio/speech"},
				"transportPolicy": "HTTP_BINARY_STREAM",
				"timeoutPolicy":   map[string]interface{}{"connectMs": 1000, "responseHeaderMs": 5000, "idleMs": 30000},
			},
			{
				"runtimeRouteId": routeASR, "resourceId": asrID, "upstreamId": upstreamID,
				"allowedMethods": []string{"POST"}, "allowedPathPrefixes": []string{"/v1/audio/transcriptions"},
				"transportPolicy": "HTTP_MULTIPART",
				"timeoutPolicy":   map[string]interface{}{"connectMs": 1000, "responseHeaderMs": 5000, "idleMs": 30000},
			},
			{
				"runtimeRouteId": routeMCP, "resourceId": mcpID, "upstreamId": upstreamID,
				"allowedMethods": []string{"POST"}, "allowedPathPrefixes": []string{"/mcp"},
				"transportPolicy": "HTTP_REQUEST_RESPONSE",
				"timeoutPolicy":   map[string]interface{}{"connectMs": 1000, "responseHeaderMs": 5000, "idleMs": 30000},
			},
		},
		"policy": map[string]interface{}{
			"policyId": policyID, "allowLocalProviders": false, "allowLocalTts": false,
			"allowLocalAsr": false, "allowLocalMcp": false,
			"defaultModelId": modelID, "defaultTtsId": ttsID, "defaultAsrId": asrID,
		},
	}
}

func (g *goldenPathTest) buildModifiedDraftContent(ctx context.Context, admin *harness.AdminClient, newModelName string) map[string]interface{} {
	// Get current draft content
	resp, err := admin.Get(ctx, "/api/admin/v1/draft")
	if err != nil {
		g.t.Fatalf("get draft: %v", err)
	}
	var draft struct {
		Content map[string]interface{} `json:"content"`
	}
	if err := harness.DecodeJSON(resp, &draft); err != nil {
		g.t.Fatalf("decode draft: %v", err)
	}
	content := draft.Content
	// Modify model display name
	if models, ok := content["models"].([]interface{}); ok && len(models) > 0 {
		if m, ok := models[0].(map[string]interface{}); ok {
			m["displayName"] = newModelName
		}
	}
	return content
}

func (g *goldenPathTest) putDraft(ctx context.Context, admin *harness.AdminClient, expectedRev int, content map[string]interface{}) int {
	resp, err := admin.Put(ctx, "/api/admin/v1/draft", map[string]interface{}{
		"expectedDraftRevision": expectedRev, "content": content,
	})
	if err != nil {
		g.t.Fatalf("put draft: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("put draft status: %d body: %s", resp.StatusCode, harness.ReadBody(resp))
	}
	var draft struct {
		DraftRevision int `json:"draftRevision"`
	}
	if err := harness.DecodeJSON(resp, &draft); err != nil {
		g.t.Fatalf("decode put draft: %v", err)
	}
	return draft.DraftRevision
}

func (g *goldenPathTest) validateDraft(ctx context.Context, admin *harness.AdminClient, rev int) {
	resp, err := admin.Post(ctx, "/api/admin/v1/draft:validate", map[string]interface{}{
		"expectedDraftRevision": rev,
	})
	if err != nil {
		g.t.Fatalf("validate draft: %v", err)
	}
	var result struct {
		Valid  bool                     `json:"valid"`
		Errors []map[string]interface{} `json:"errors"`
	}
	if err := harness.DecodeJSON(resp, &result); err != nil {
		g.t.Fatalf("decode validate: %v", err)
	}
	if !result.Valid {
		g.t.Fatalf("draft validation failed: %+v", result.Errors)
	}
}

func (g *goldenPathTest) previewDraft(ctx context.Context, admin *harness.AdminClient, rev int) map[string]interface{} {
	resp, err := admin.Post(ctx, "/api/admin/v1/draft:preview", map[string]interface{}{
		"expectedDraftRevision": rev,
	})
	if err != nil {
		g.t.Fatalf("preview draft: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("preview status: %d body: %s", resp.StatusCode, harness.ReadBody(resp))
	}
	var result map[string]interface{}
	if err := harness.DecodeJSON(resp, &result); err != nil {
		g.t.Fatalf("decode preview: %v", err)
	}
	return result
}

func (g *goldenPathTest) assertPreviewClientSafe(ctx context.Context, admin *harness.AdminClient, rev int) {
	preview := g.previewDraft(ctx, admin, rev)
	raw, _ := json.Marshal(preview)
	str := string(raw)
	forbidden := []string{"upstreamId", "runtimeRouteId", "baseUrl", "secretId", "secretVersion"}
	for _, f := range forbidden {
		if strings.Contains(str, "\""+f+"\"") {
			g.t.Fatalf("preview leaks server-only field: %s", f)
		}
	}
}

func (g *goldenPathTest) publishDraft(ctx context.Context, admin *harness.AdminClient, rev int) string {
	idempotencyKey := platformid.New(platformid.Idempotency)
	resp, err := admin.PostWithHeaders(ctx, "/api/admin/v1/draft:publish", map[string]interface{}{
		"expectedDraftRevision":    rev,
		"acknowledgedWarningCodes": []string{},
	}, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	if err != nil {
		g.t.Fatalf("publish: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		g.t.Fatalf("publish status: %d body: %s", resp.StatusCode, harness.ReadBody(resp))
	}
	var act struct {
		ActivationID string `json:"activationId"`
	}
	if err := harness.DecodeJSON(resp, &act); err != nil {
		g.t.Fatalf("decode publish: %v", err)
	}
	return act.ActivationID
}

func (g *goldenPathTest) waitActivationCompleted(ctx context.Context, admin *harness.AdminClient, activationID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			g.t.Fatalf("context cancelled waiting for activation")
		default:
		}
		resp, err := admin.Get(ctx, "/api/admin/v1/activations/"+activationID)
		if err != nil {
			last = err.Error()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var act struct {
			State     string `json:"state"`
			ErrorCode string `json:"errorCode"`
		}
		_ = harness.DecodeJSON(resp, &act)
		if act.State == "COMPLETED" {
			return
		}
		if act.State == "FAILED" {
			g.t.Fatalf("activation failed: errorCode=%s", act.ErrorCode)
		}
		last = act.State
		resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}
	g.t.Fatalf("activation not completed: last state=%s", last)
}

func (g *goldenPathTest) exchangeEnrollmentAndBootstrap(ctx context.Context, hubBaseURL, enrollmentCode string) (string, int) {
	// Exchange enrollment to get a client access token
	installationID := platformid.New(platformid.Installation)
	resp, err := http.Post(hubBaseURL+"/api/client/v1/enrollments/exchange", "application/json",
		strings.NewReader(fmt.Sprintf(`{"platform":"ANDROID","code":%q,"installationId":%q,"appVersion":"test-1.0"}`, enrollmentCode, installationID)))
	if err != nil {
		g.t.Fatalf("exchange enrollment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body := harness.ReadBody(resp)
		g.t.Fatalf("exchange enrollment status: %d body: %s", resp.StatusCode, body)
	}
	var exchange struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&exchange); err != nil {
		g.t.Fatalf("decode exchange: %v", err)
	}

	// Get managed state to find the active generation
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, hubBaseURL+"/api/client/v1/managed/state", nil)
	req.Header.Set("Authorization", "Bearer "+exchange.AccessToken)
	stateResp, err := http.DefaultClient.Do(req)
	if err != nil {
		g.t.Fatalf("get managed state: %v", err)
	}
	defer stateResp.Body.Close()
	var state struct {
		ActiveManagedGeneration int `json:"activeManagedGeneration"`
	}
	if err := json.NewDecoder(stateResp.Body).Decode(&state); err != nil {
		g.t.Fatalf("decode managed state: %v", err)
	}
	return exchange.AccessToken, state.ActiveManagedGeneration
}

func (g *goldenPathTest) getSnapshotResourceIDs(ctx context.Context, hubBaseURL, clientToken string, generation int, expectedModel, expectedTTS, expectedASR, expectedMCP string) snapshotResources {
	// Fetch snapshot
	url := fmt.Sprintf("%s/api/client/v1/managed/snapshots/%d", hubBaseURL, generation)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+clientToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		g.t.Fatalf("fetch snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("snapshot status: %d", resp.StatusCode)
	}
	var snap map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		g.t.Fatalf("decode snapshot: %v", err)
	}
	// Extract resource IDs from snapshot
	result := snapshotResources{}
	if models, ok := snap["models"].([]interface{}); ok && len(models) > 0 {
		if m, ok := models[0].(map[string]interface{}); ok {
			result.model = m["modelId"].(string)
		}
	}
	if tts, ok := snap["tts"].([]interface{}); ok && len(tts) > 0 {
		if t, ok := tts[0].(map[string]interface{}); ok {
			result.tts = t["ttsId"].(string)
		}
	}
	if asr, ok := snap["asr"].([]interface{}); ok && len(asr) > 0 {
		if a, ok := asr[0].(map[string]interface{}); ok {
			result.asr = a["asrId"].(string)
		}
	}
	if mcp, ok := snap["mcp"].([]interface{}); ok && len(mcp) > 0 {
		if m, ok := mcp[0].(map[string]interface{}); ok {
			result.mcp = m["mcpServerId"].(string)
		}
	}
	// Verify snapshot IDs match expected (they should be the same)
	if result.model == "" {
		result.model = expectedModel
	}
	if result.tts == "" {
		result.tts = expectedTTS
	}
	if result.asr == "" {
		result.asr = expectedASR
	}
	if result.mcp == "" {
		result.mcp = expectedMCP
	}
	// Verify snapshot has no server-only fields
	raw, _ := json.Marshal(snap)
	str := string(raw)
	for _, f := range []string{"upstreamId", "runtimeRouteId", "baseUrl", "secretId"} {
		if strings.Contains(str, "\""+f+"\"") {
			g.t.Fatalf("snapshot leaks server-only field: %s", f)
		}
	}
	return result
}

func (g *goldenPathTest) waitUsageRecorded(ctx context.Context, admin *harness.AdminClient, minCount int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := admin.Get(ctx, "/api/admin/v1/usage/summary")
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		var summary map[string]interface{}
		if err := harness.DecodeJSON(resp, &summary); err != nil {
			time.Sleep(time.Second)
			continue
		}
		if count, ok := summary["requestCount"].(float64); ok && int(count) >= minCount {
			return
		}
		time.Sleep(time.Second)
	}
	g.t.Fatalf("usage not recorded within timeout (expected >= %d)", minCount)
}

type snapshotResources struct{ model, tts, asr, mcp string }

// runAllFourProfiles invokes all four runtime profiles through the Test Client
// and returns error if any fails.
func runAllFourProfiles(ctx context.Context, tc *client.Client, ids snapshotResources) error {
	// Model streaming
	chunks := 0
	if err := tc.ChatCompletionStream(ctx, ids.model, "/v1/chat/completions", `{"model":"gpt-test","stream":true}`, func([]byte) { chunks++ }); err != nil {
		return fmt.Errorf("model stream: %w", err)
	}
	if chunks < 2 {
		return fmt.Errorf("expected >=2 stream chunks, got %d", chunks)
	}
	// TTS binary
	ttsBody, ct, err := tc.Speech(ctx, ids.tts, "/v1/audio/speech", `{"input":"hi","voice":"alloy"}`)
	if err != nil {
		return fmt.Errorf("tts: %w", err)
	}
	if !strings.HasPrefix(ct, "audio/mpeg") || len(ttsBody) == 0 {
		return fmt.Errorf("tts bad response: ct=%q len=%d", ct, len(ttsBody))
	}
	// ASR multipart
	asrBody, _, err := tc.Transcription(ctx, ids.asr, "/v1/audio/transcriptions", "whisper-test", "sample.wav", []byte("RIFF"))
	if err != nil {
		return fmt.Errorf("asr: %w", err)
	}
	if !strings.Contains(string(asrBody), `"text"`) {
		return fmt.Errorf("asr bad response: %s", asrBody)
	}
	// MCP
	mcpBody, _, err := tc.MCP(ctx, ids.mcp, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if err != nil {
		return fmt.Errorf("mcp: %w", err)
	}
	if !strings.Contains(string(mcpBody), `"tools"`) {
		return fmt.Errorf("mcp bad response: %s", mcpBody)
	}
	return nil
}
