package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// A strict external boundary for the current stateless Responses profile.
// See https://developers.openai.com/api/reference/resources/responses/methods/create.
// It deliberately rejects a Chat Completions body sent to this endpoint.
func (a *Adapter) handleResponses(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	body := fact.BodyJSON
	model, _ := body["model"].(string)
	input, _ := body["input"].([]interface{})
	if r.Method != http.MethodPost || model == "" || len(input) == 0 || body["stream"] != true || body["store"] != false || body["messages"] != nil || body["previous_response_id"] != nil || body["conversation"] != nil {
		http.Error(w, "expected stateless streaming Responses input", http.StatusBadRequest)
		return
	}
	tools, _ := body["tools"].([]interface{})
	toolName := ""
	if len(tools) > 0 {
		tool, _ := tools[0].(map[string]interface{})
		toolName, _ = tool["name"].(string)
		if toolName == "" || tool["type"] != "function" {
			http.Error(w, "expected Responses function tool", http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	sequence := 0
	emit := func(kind string, event map[string]interface{}) {
		event["type"], event["sequence_number"] = kind, sequence
		sequence++
		data, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
	}
	response := map[string]interface{}{"id": "resp_fixture", "object": "response", "created_at": 1750000000, "status": "in_progress", "model": model, "output": []interface{}{}, "store": false}
	emit("response.created", map[string]interface{}{"response": response})
	var item map[string]interface{}
	if toolName != "" {
		item = map[string]interface{}{"type": "function_call", "id": "fc_fixture", "call_id": "call_fixture", "name": toolName, "arguments": "", "status": "in_progress"}
		emit("response.output_item.added", map[string]interface{}{"output_index": 0, "item": item})
		emit("response.function_call_arguments.delta", map[string]interface{}{"output_index": 0, "item_id": "fc_fixture", "delta": "{}"})
		item["arguments"], item["status"] = "{}", "completed"
		emit("response.function_call_arguments.done", map[string]interface{}{"output_index": 0, "item_id": "fc_fixture", "arguments": "{}"})
	} else {
		item = map[string]interface{}{"type": "message", "id": "msg_fixture", "role": "assistant", "status": "in_progress", "content": []interface{}{}}
		emit("response.output_item.added", map[string]interface{}{"output_index": 0, "item": item})
		part := map[string]interface{}{"type": "output_text", "text": "", "annotations": []interface{}{}}
		emit("response.content_part.added", map[string]interface{}{"output_index": 0, "content_index": 0, "item_id": "msg_fixture", "part": part})
		emit("response.output_text.delta", map[string]interface{}{"output_index": 0, "content_index": 0, "item_id": "msg_fixture", "delta": "Protocol verified"})
		part["text"] = "Protocol verified"
		emit("response.output_text.done", map[string]interface{}{"output_index": 0, "content_index": 0, "item_id": "msg_fixture", "text": part["text"]})
		emit("response.content_part.done", map[string]interface{}{"output_index": 0, "content_index": 0, "item_id": "msg_fixture", "part": part})
		item["content"], item["status"] = []interface{}{part}, "completed"
	}
	emit("response.output_item.done", map[string]interface{}{"output_index": 0, "item": item})
	response["status"], response["output"] = "completed", []interface{}{item}
	response["usage"] = map[string]interface{}{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}
	emit("response.completed", map[string]interface{}{"response": response})
}
