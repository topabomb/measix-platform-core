package adapter_test

import (
	"bufio"
	"encoding/json"
	"measix/platform/test/system/adapter"
	"net/http"
	"strings"
	"testing"
)

func TestAndroidChatStreamHasTerminalChoiceBeforeDone(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	response := doJSON(t, a.URL, http.MethodPost, "/v1/chat/completions", `{"model":"test","messages":[],"stream":true}`)
	defer response.Body.Close()
	terminal := false
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			if !terminal {
				t.Fatal("stream ended without finish_reason")
			}
			return
		}
		var chunk struct {
			Choices []struct {
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatal(err)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && *choice.FinishReason == "stop" {
				terminal = true
			}
		}
	}
	t.Fatalf("stream missing DONE: %v", scanner.Err())
}

func TestAndroidMCPInitializedNotificationHasNoJSONRPCResponse(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	response := doJSON(t, a.URL, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("notification status=%d body=%s", response.StatusCode, readAll(t, response))
	}
	if body := readAll(t, response); len(body) != 0 {
		t.Fatalf("notification must not receive JSON-RPC response: %s", body)
	}
	response, err := http.Get(a.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported SSE GET status=%d", response.StatusCode)
	}
}
