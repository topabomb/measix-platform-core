package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func emitNativeEvent(w http.ResponseWriter, event string, value any) {
	data, _ := json.Marshal(value)
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flush, ok := w.(http.Flusher); ok {
		flush.Flush()
	}
}

func (a *Adapter) handleGeminiModel(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	body := fact.BodyJSON
	contents, _ := body["contents"].([]any)
	if r.Method != http.MethodPost || r.URL.Query().Get("alt") != "sse" || len(contents) == 0 || body["messages"] != nil {
		http.Error(w, "expected Gemini contents and alt=sse", http.StatusBadRequest)
		return
	}
	for _, item := range contents {
		content, _ := item.(map[string]any)
		parts, _ := content["parts"].([]any)
		if len(parts) == 0 {
			http.Error(w, "missing Gemini parts", http.StatusBadRequest)
			return
		}
	}
	part := map[string]any{"text": "Protocol verified"}
	if tools, _ := body["tools"].([]any); len(tools) > 0 {
		tool, _ := tools[0].(map[string]any)
		declarations, _ := tool["functionDeclarations"].([]any)
		if len(declarations) == 0 {
			http.Error(w, "missing Gemini function declarations", http.StatusBadRequest)
			return
		}
		declaration, _ := declarations[0].(map[string]any)
		name, _ := declaration["name"].(string)
		if name == "" {
			http.Error(w, "missing Gemini function name", http.StatusBadRequest)
			return
		}
		part = map[string]any{"functionCall": map[string]any{"name": name, "args": map[string]any{}}}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	emitNativeEvent(w, "", map[string]any{"candidates": []any{map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": []any{part}}, "finishReason": "STOP"}}, "usageMetadata": map[string]any{"promptTokenCount": 10, "candidatesTokenCount": 3, "totalTokenCount": 13}})
}

func (a *Adapter) handleClaude(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	body := fact.BodyJSON
	model, _ := body["model"].(string)
	maxTokens, _ := body["max_tokens"].(float64)
	messages, _ := body["messages"].([]any)
	if r.Method != http.MethodPost || r.Header.Get("anthropic-version") != "2023-06-01" || model == "" || maxTokens <= 0 || len(messages) == 0 || body["stream"] != true {
		http.Error(w, "expected streaming Anthropic Messages request", http.StatusBadRequest)
		return
	}
	block := map[string]any{"type": "text", "text": ""}
	delta := map[string]any{"type": "text_delta", "text": "Protocol verified"}
	stopReason := "end_turn"
	if tools, _ := body["tools"].([]any); len(tools) > 0 {
		tool, _ := tools[0].(map[string]any)
		name, _ := tool["name"].(string)
		if name == "" || tool["input_schema"] == nil {
			http.Error(w, "expected Anthropic tool schema", http.StatusBadRequest)
			return
		}
		block = map[string]any{"type": "tool_use", "id": "toolu_fixture", "name": name, "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": "{}"}
		stopReason = "tool_use"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	emitNativeEvent(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 10, "output_tokens": 0}}})
	emitNativeEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": block})
	emitNativeEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": delta})
	emitNativeEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	emitNativeEvent(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 3}})
	emitNativeEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}
