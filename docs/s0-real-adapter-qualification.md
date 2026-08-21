# S0.1 Real Adapter Qualification Report

> **Checkpoint**: C6 / C7  
> **Status**: NOT EXECUTED — Real endpoint qualification has not been performed.  
> **Date**: 2026-08-20  
> **Architecture baseline**: `topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`  
> **Platform-core commit**: see `docs/s0-freeze-manifest.json` → `platformCoreCommit` (if generated)

## 1. Scope

The S0.1 Gate requires qualification of at least one real OpenAI-compatible
endpoint through the full Hub→Relay→Adapter path, proving that the
deterministic Test Adapter is a faithful stand-in for a real upstream.

Per architecture qualification spec, the qualification unit is
adapter/version + configRevision + profile. Different profiles (Model/TTS/ASR/MCP)
may use different endpoints/adapters; a single adapter is NOT required to
cover all four capabilities.

This report covers:

1. **Deterministic Adapter Qualification** — the in-repo `test/system/adapter`
   implements all four required transport profiles. System tests using the
   deterministic adapter compile and pass at the component level.
2. **Real Endpoint Qualification Procedure** — the documented procedure for
   qualifying a real OpenAI-compatible endpoint.
3. **Qualification Results** — NOT EXECUTED.

## 2. Deterministic Adapter Description

The deterministic Test Adapter (`backend/test/system/adapter/adapter.go`) is a
real HTTP service backed by `httptest.Server`. It is NOT a mock — it uses real
HTTP transport, real multipart parsing, real SSE streaming, and real binary
responses.

| Profile | Endpoint | Transport | Response |
|---|---|---|---|
| Model (request/response) | `POST /v1/chat/completions` | HTTP_REQUEST_RESPONSE | JSON `{choices:[...]}` |
| Model (streaming SSE) | `POST /v1/chat/completions` (stream:true) | HTTP_STREAMING_SSE | `text/event-stream` with 4 chunks + `[DONE]` |
| TTS (binary) | `POST /v1/audio/speech` | HTTP_BINARY_STREAM | `audio/mpeg` (16-byte ID3 header) |
| ASR (multipart) | `POST /v1/audio/transcriptions` | HTTP_MULTIPART | JSON `{text:...}` |
| MCP (Streamable HTTP) | `POST /mcp` | HTTP_REQUEST_RESPONSE | JSON `{jsonrpc:2.0,...}` |

The deterministic adapter implements the OpenAI Chat Completions,
Audio Speech, and Audio Transcriptions protocols with response shapes
(JSON structure, SSE event format, binary content type, multipart parsing)
that match the OpenAI API specification. For MCP, the adapter implements
a minimal JSON-RPC 2.0 protocol flow (initialize → tools/list → tools/call)
sufficient for deterministic testing of the required MCP Streamable HTTP
profile. It does not implement SSE-backed Streamable HTTP sessions or
resource completion; it must not be used to qualify real MCP endpoint
compatibility beyond the basic request/response transport.

## 3. Real Endpoint Qualification Procedure

### 3.1 Prerequisites

- A running Hub + Relay environment (see `scripts/dev-setup.mjs`)
- An OpenAI-compatible API endpoint (OpenAI, Azure OpenAI, Ollama, vLLM, etc.)
- A valid API key for the endpoint

### 3.2 Steps

1. **Create Secret**: Store the API key as a Hub secret:
   ```
   POST /api/admin/v1/secrets
   {"name":"openai-api-key","value":"sk-..."}
   ```

2. **Create Upstream**: Register the endpoint as an upstream:
   ```
   POST /api/admin/v1/upstreams
   {
     "config": {
       "name": "OpenAI",
       "baseUrl": "https://api.openai.com",
       "transportCapabilities": ["HTTP_REQUEST_RESPONSE","HTTP_STREAMING_SSE","HTTP_BINARY_STREAM","HTTP_MULTIPART"],
       "auth": {"type":"BEARER","secretRef":{"secretId":"...","secretVersion":1}},
       "correlationMode": "HEADER_ECHO",
       "usageCapabilityLevel": "LEVEL_1"
     }
   }
   ```

3. **Test Upstream**: Verify connectivity:
   ```
   POST /api/admin/v1/upstreams/{id}:test
   ```

4. **Apply Upstream**: Activate the upstream:
   ```
   POST /api/admin/v1/upstreams/{id}:apply
   ```

5. **Configure Resources**: Create model/TTS/ASR/MCP resources in the draft,
   binding each to the real upstream with appropriate `upstreamModelKey`.

6. **Validate + Publish**: Validate the draft, preview the snapshot, then publish.

7. **Exchange Enrollment**: Get a client access token.

8. **Invoke Runtime**: Use the Test Client to call all four profiles through
   the Relay against the real upstream:
   - Model: `POST /runtime/v1/resources/{modelId}/v1/chat/completions`
   - TTS: `POST /runtime/v1/resources/{ttsId}/v1/audio/speech`
   - ASR: `POST /runtime/v1/resources/{asrId}/v1/audio/transcriptions`
   - MCP: `POST /runtime/v1/resources/{mcpId}/mcp`

9. **Verify Usage**: Check that usage records are complete and contain
   correct semantic meters.

### 3.3 Acceptance Criteria

The real endpoint is qualified when:

- [ ] All four transport profiles succeed through the Relay
- [ ] Streaming SSE delivers ≥2 chunks
- [ ] TTS returns valid audio binary (Content-Type starts with `audio/`)
- [ ] ASR multipart is correctly parsed by the upstream
- [ ] MCP returns valid JSON-RPC response
- [ ] Usage records show correct resource kind, generation, and status
- [ ] No prompt body or secret is leaked in usage detail
- [ ] Stale generation requests return 428 with `forwarded: false`
- [ ] Adapter receives NO request body for denied old-generation calls

## 4. Qualification Results — NOT EXECUTED

**Deterministic Adapter**: Implemented; system tests using it compile and pass
at the component level. These are deterministic T2/T3 evidence, NOT equivalent
to real adapter qualification.

**Real Endpoint Qualification**: NOT EXECUTED. The procedure in §3.2 is the
authoritative guide for operators to execute real endpoint qualification in a
secure environment. No real OpenAI-compatible endpoint has been qualified.

## 5. Gap

- Real OpenAI API qualification has not been executed (requires an API key
  secret that must not enter persistent state or logs per AGENTS.md).
- Deterministic adapter tests are NOT a substitute for real adapter
  qualification per `measix-s0-capability-delivery-system-testing-spec.md` §16.
- The deterministic adapter cannot be used to claim real Adapter qualification
  for C7 Freeze.
