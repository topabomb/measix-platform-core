// generate-client-fixtures projects the authored S0.2 profile through the real
// compiler. Only the current unpublished protocol is supported.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
)

type object = map[string]any
type wireCase struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
	Valid  bool   `json:"valid"`
	Value  any    `json:"value"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func clone(value any) object {
	b, e := json.Marshal(value)
	must(e)
	var result object
	must(json.Unmarshal(b, &result))
	return result
}

func main() {
	root := filepath.Join("..", "api", "fixtures")
	out := filepath.Join(root, "client-integration")
	must(os.MkdirAll(out, 0755))
	write := func(name string, value any) {
		raw, err := json.MarshalIndent(value, "", "  ")
		must(err)
		must(os.WriteFile(filepath.Join(out, name), append(raw, '\n'), 0644))
	}
	raw, err := os.ReadFile(filepath.Join(root, "draft", "s02-client-profile.json"))
	must(err)
	var content adminapi.ManagedDraftContent
	must(json.Unmarshal(raw, &content))
	deployment := "dep_550e8400-e29b-41d4-a716-446655440000"
	user := "usr_550e8400-e29b-41d4-a716-446655440000"
	device := "dev_550e8400-e29b-41d4-a716-446655440000"
	session := "ses_550e8400-e29b-41d4-a716-446655440000"
	at := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	snapshot, _, err := capability.NewService(nil).CompileSnapshot(capability.SnapshotInput{DeploymentID: deployment, ReleaseID: "rel_550e8400-e29b-41d4-a716-446655440000", ManagedGeneration: 42, Content: content, PublishedAt: at, PublishedByUserID: user})
	must(err)
	write("snapshot-v4.json", snapshot)
	denied := content
	denied.Policy.AllowLocalProviders = false
	denied.Policy.AllowLocalTts = false
	denied.Policy.AllowLocalAsr = false
	denied.Policy.AllowLocalMcp = false
	denied.Policy.AllowLocalAssistants = false
	deniedSnapshot, _, err := capability.NewService(nil).CompileSnapshot(capability.SnapshotInput{DeploymentID: deployment, ReleaseID: "rel_660e8400-e29b-41d4-a716-446655440000", ManagedGeneration: 43, Content: denied, PublishedAt: at, PublishedByUserID: user})
	must(err)
	write("snapshot-v4-denied.json", deniedSnapshot)
	// Reference validity is a semantic check, separate from JSON shape. These
	// public projection recipes intentionally have no private runtime bindings.
	references := []object{{"name": "complete-references", "expectedCode": "", "content": content}}
	mutations := []struct {
		name, code string
		change     func(object)
	}{
		{"assistant-missing-model", "invalid_model_ref", func(v object) {
			v["assistants"].([]any)[0].(map[string]any)["modelId"] = "mdl_99999999-9999-4999-8999-999999999999"
		}},
		{"assistant-disabled-model", "invalid_model_ref", func(v object) { v["models"].([]any)[0].(map[string]any)["enabled"] = false }},
		{"assistant-disabled-mcp", "invalid_mcp_ref", func(v object) { v["mcp"].([]any)[0].(map[string]any)["enabled"] = false }},
		{"starter-disabled-assistant", "invalid_assistant_ref", func(v object) { v["assistants"].([]any)[0].(map[string]any)["enabled"] = false }},
		{"default-missing-model", "invalid_default_model", func(v object) {
			v["policy"].(map[string]any)["defaultModelId"] = "mdl_99999999-9999-4999-8999-999999999999"
		}},
		{"default-disabled-tts", "invalid_default_tts", func(v object) { v["tts"].([]any)[0].(map[string]any)["enabled"] = false }},
		{"default-disabled-asr", "invalid_default_asr", func(v object) { v["asr"].([]any)[0].(map[string]any)["enabled"] = false }},
	}
	for _, mutation := range mutations {
		value := clone(content)
		mutation.change(value)
		references = append(references, object{"name": mutation.name, "expectedCode": mutation.code, "content": value})
	}
	write("reference-cases.json", references)
	cases := []wireCase{}
	add := func(name, schema string, valid bool, value any) {
		cases = append(cases, wireCase{name, schema, valid, value})
	}
	add("v4-full", "ManagedSnapshot", true, snapshot)
	add("v4-policy-denied", "ManagedSnapshot", true, deniedSnapshot)
	for _, flag := range []string{"allowLocalProviders", "allowLocalTts", "allowLocalAsr", "allowLocalMcp", "allowLocalAssistants"} {
		for _, mutation := range []string{"missing", "null", "string"} {
			value := clone(snapshot)
			policy := value["policy"].(map[string]any)
			switch mutation {
			case "missing":
				delete(policy, flag)
			case "null":
				policy[flag] = nil
			case "string":
				policy[flag] = "true"
			}
			add("v4-policy-"+flag+"-"+mutation, "ManagedSnapshot", false, value)
		}
	}
	for _, field := range []string{"assistants", "starters"} {
		value := clone(snapshot)
		delete(value, field)
		add("v4-missing-"+field, "ManagedSnapshot", false, value)
	}
	value := clone(snapshot)
	value["schemaVersion"] = 5
	add("gateway-version-not-s02", "ManagedSnapshot", false, value)
	value = clone(snapshot)
	value["models"].([]any)[0].(map[string]any)["credential"] = "forbidden-synthetic-secret"
	add("v4-secret-leak", "ManagedSnapshot", false, value)
	for _, field := range []string{"upstreamUrl", "runtimeRouteId", "resolvedCredential"} {
		value := clone(snapshot)
		value["models"].([]any)[0].(map[string]any)[field] = "forbidden-synthetic-internal-value"
		add("v4-internal-"+field, "ManagedSnapshot", false, value)
	}
	discovery := object{"product": "MEASIX_AGENT_PLATFORM", "protocolVersion": "1", "deploymentId": deployment, "deploymentName": "S0.2 Integration", "clientApiBase": "/api/client/v1", "runtimeApiBase": "/runtime/v1", "supportedSnapshotSchemaVersions": []int{4}}
	enrollmentRequest := object{"code": "synthetic-single-use-code", "installationId": "ins_550e8400-e29b-41d4-a716-446655440000", "deviceName": "Android contract fixture", "appVersion": "0.0.20", "platform": "ANDROID"}
	enrollment := object{"deploymentId": deployment, "userId": user, "deviceId": device, "sessionId": session, "accessToken": "synthetic.access.token", "accessTokenExpiresAt": at.Add(15 * time.Minute).Format(time.RFC3339), "refreshToken": "synthetic-refresh-token", "refreshExpiresAt": at.Add(7 * 24 * time.Hour).Format(time.RFC3339), "sessionIdleExpiresAt": at.Add(7 * 24 * time.Hour).Format(time.RFC3339)}
	ready := object{"runtimeStatus": "READY", "activeManagedGeneration": 42, "managedStateRevision": 77, "syncRequired": false, "runtimeBlocked": false}
	pending := object{"runtimeStatus": "READY", "activeManagedGeneration": 0, "managedStateRevision": 1, "syncRequired": true, "runtimeBlocked": true}
	syncRequired := object{"runtimeStatus": "READY", "activeManagedGeneration": 43, "targetManagedGeneration": 43, "managedStateRevision": 78, "syncRequired": true, "runtimeBlocked": true}
	bootstrapState := clone(ready)
	bootstrapState["syncRequired"], bootstrapState["runtimeBlocked"], bootstrapState["targetManagedGeneration"] = true, true, 42
	bootstrap := object{"deployment": object{"deploymentId": deployment, "name": "S0.2 Integration"}, "user": object{"userId": user, "displayName": "Contract User"}, "device": object{"deviceId": device, "status": "ACTIVE"}, "session": object{"sessionId": session, "expiresAt": enrollment["sessionIdleExpiresAt"], "sessionIdleExpiresAt": enrollment["sessionIdleExpiresAt"]}, "supportedSnapshotSchemaVersions": []int{4}, "managedState": bootstrapState}
	refresh := object{"accessToken": "synthetic.rotated.access", "accessTokenExpiresAt": at.Add(time.Hour + 15*time.Minute).Format(time.RFC3339), "refreshToken": "synthetic-rotated-refresh", "refreshExpiresAt": at.Add(time.Hour + 7*24*time.Hour).Format(time.RFC3339), "sessionIdleExpiresAt": at.Add(time.Hour + 7*24*time.Hour).Format(time.RFC3339)}
	for _, entry := range []struct {
		name, schema string
		value        any
	}{{"discovery", "Discovery", discovery}, {"enrollment-request", "EnrollmentExchangeRequest", enrollmentRequest}, {"enrollment-response", "EnrollmentExchangeResponse", enrollment}, {"bootstrap", "Bootstrap", bootstrap}, {"managed-ready", "ManagedState", ready}, {"managed-pending", "ManagedState", pending}, {"managed-sync-required", "ManagedState", syncRequired}, {"refresh-response", "RefreshResponse", refresh}} {
		write(entry.name+".json", entry.value)
		add(entry.name, entry.schema, true, entry.value)
	}
	write("cases.json", cases)
	response := func(status int, body any) object { return object{"status": status, "body": body} }
	httpExamples := []object{
		{"name": "discovery", "method": "GET", "path": "/.well-known/measix", "response": response(200, discovery)},
		{"name": "enrollment", "method": "POST", "path": "/api/client/v1/enrollments/exchange", "body": enrollmentRequest, "response": response(201, enrollment)},
		{"name": "bootstrap", "method": "GET", "path": "/api/client/v1/bootstrap", "headers": object{"Authorization": "Bearer synthetic.access.token"}, "response": response(200, bootstrap)},
		{"name": "refresh", "method": "POST", "path": "/api/client/v1/sessions/refresh", "headers": object{"Idempotency-Key": "idem_550e8400-e29b-41d4-a716-446655440000"}, "body": object{"refreshToken": "synthetic-refresh-token"}, "response": response(200, refresh)},
		{"name": "snapshot", "method": "GET", "path": "/api/client/v1/managed/snapshots/42", "headers": object{"Authorization": "Bearer synthetic.access.token"}, "response": object{"status": 200, "headers": object{"ETag": "\"" + snapshot.SnapshotHash + "\""}, "bodyFile": "snapshot-v4.json"}},
		{"name": "snapshot-not-modified", "method": "GET", "path": "/api/client/v1/managed/snapshots/42", "headers": object{"Authorization": "Bearer synthetic.access.token", "If-None-Match": "\"" + snapshot.SnapshotHash + "\""}, "response": object{"status": 304, "headers": object{"ETag": "\"" + snapshot.SnapshotHash + "\""}, "body": nil}},
		{"name": "logout", "method": "POST", "path": "/api/client/v1/sessions/logout", "body": object{"refreshToken": "synthetic-rotated-refresh"}, "response": response(204, nil)},
	}
	write("http-examples.json", httpExamples)
	receptions := []object{}
	for _, name := range []string{"accept", "wrong-deployment", "wrong-generation", "wrong-etag", "wrong-body-hash"} {
		context := object{"deploymentId": deployment, "generation": 42, "etag": "\"" + snapshot.SnapshotHash + "\""}
		value := clone(snapshot)
		switch name {
		case "wrong-deployment":
			context["deploymentId"] = "dep_99999999-9999-4999-8999-999999999999"
		case "wrong-generation":
			context["generation"] = 43
		case "wrong-etag":
			context["etag"] = "\"sha256:0000000000000000000000000000000000000000000000000000000000000000\""
		case "wrong-body-hash":
			value["snapshotHash"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		}
		receptions = append(receptions, object{"name": name, "valid": name == "accept", "context": context, "snapshot": value})
	}
	write("snapshot-reception-cases.json", receptions)
	base := "https://platform.example.invalid/runtime/v1/resources/"
	headers := object{"Authorization": "Bearer synthetic.access.token", "X-Measix-Managed-Generation": "42", "X-Measix-Interaction-Id": "int_550e8400-e29b-41d4-a716-446655440000"}
	write("runtime-examples.json", []object{
		{"resourceId": snapshot.Models[0].ModelId, "protocol": snapshot.Providers[0].ClientProtocol, "method": "POST", "url": base + snapshot.Models[0].ModelId + snapshot.Models[0].RuntimePath, "headers": headers, "contentType": "application/json", "body": object{"model": snapshot.Models[0].UpstreamModelKey, "messages": []object{{"role": "user", "content": "Hello"}}, "stream": true, "stream_options": object{"include_usage": true}}, "responseKind": "SSE"},
		{"resourceId": snapshot.Tts[0].TtsId, "protocol": snapshot.Tts[0].ClientProtocol, "method": "POST", "url": base + snapshot.Tts[0].TtsId + snapshot.Tts[0].RuntimePath, "headers": headers, "contentType": "application/json", "body": object{"model": snapshot.Tts[0].UpstreamModelKey, "voice": snapshot.Tts[0].Voice, "input": "Hello"}, "responseKind": "BINARY_AUDIO"},
		{"resourceId": snapshot.Asr[0].AsrId, "protocol": snapshot.Asr[0].ClientProtocol, "method": "POST", "url": base + snapshot.Asr[0].AsrId + snapshot.Asr[0].RuntimePath, "headers": headers, "contentType": "multipart/form-data", "fields": object{"model": snapshot.Asr[0].UpstreamModelKey, "language": snapshot.Asr[0].Language, "file": "<client audio bytes>"}, "responseKind": "JSON"},
		{"resourceId": snapshot.Mcp[0].McpServerId, "protocol": snapshot.Mcp[0].ClientProtocol, "method": "POST", "url": base + snapshot.Mcp[0].McpServerId + snapshot.Mcp[0].RuntimePath, "headers": headers, "contentType": "application/json", "body": object{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, "precondition": "MCP session initialized by client SDK; add negotiated protocol/session headers and Accept application/json, text/event-stream", "responseKind": "JSON_OR_SSE"},
	})
	// HTTP response fields are schema examples, not live credentials or a substitute for handler tests.
	write("provenance.json", object{"generated": true, "input": "api/fixtures/draft/s02-client-profile.json", "compiler": "capability.CompileSnapshot", "snapshotSchemaVersion": 4, "fixtureTime": at.Format(time.RFC3339), "credentials": "synthetic-not-usable", "httpEvidence": "See docs/testing.md; examples do not prove a live exchange"})
}
