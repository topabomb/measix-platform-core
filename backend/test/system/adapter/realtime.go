package adapter

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// A bounded deterministic peer for the two ASR wire protocols. It validates
// configuration and PCM framing; returned text is synthetic, not recognized speech.
func (a *Adapter) handleRealtimeASR(w http.ResponseWriter, r *http.Request) {
	dash := r.URL.Path == "/api-ws/v1/realtime"
	if (!dash && r.URL.Query().Get("intent") != "transcription") || (dash && r.URL.Query().Get("model") == "") {
		http.Error(w, "missing ASR query", http.StatusBadRequest)
		return
	}
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	write := func(event map[string]any) bool { return conn.WriteJSON(event) == nil }
	fail := func(message string) {
		write(map[string]any{"type": "error", "error": map[string]any{"code": "invalid_request_error", "message": message}})
	}
	if !write(map[string]any{"type": "session.created", "session": map[string]any{"id": "session_test"}}) {
		return
	}
	configured := false
	for {
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			return
		}
		if dash && stringField(event, "event_id") == "" {
			fail("event_id required")
			return
		}
		switch event["type"] {
		case "session.update":
			if !validRealtimeSettings(objectField(event, "session"), dash) {
				fail("invalid realtime settings")
				return
			}
			configured = true
			if !write(map[string]any{"type": "session.updated", "session": event["session"]}) {
				return
			}
		case "input_audio_buffer.append":
			pcm, err := base64.StdEncoding.DecodeString(stringField(event, "audio"))
			if !configured || err != nil || len(pcm) == 0 || len(pcm)%2 != 0 {
				fail("configured PCM16 audio required")
				return
			}
			interim := map[string]any{"type": "conversation.item.input_audio_transcription.delta", "item_id": "item_test", "content_index": 0, "delta": "协议"}
			if dash {
				interim = map[string]any{"type": "conversation.item.input_audio_transcription.text", "item_id": "item_test", "text": "协议", "stash": "验证"}
			}
			if !write(interim) || !write(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "item_test", "content_index": 0, "transcript": "协议验证"}) {
				return
			}
		case "session.finish":
			if !dash || !configured {
				fail("session.finish requires configured DashScope session")
				return
			}
			write(map[string]any{"type": "session.finished"})
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			// Complete the closing handshake before closing TCP, including when
			// several transparent proxies sit between this peer and the client.
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			_, _, _ = conn.ReadMessage()
			return
		default:
			fail("unsupported test session event")
			return
		}
	}
}

func objectField(value map[string]any, key string) map[string]any {
	result, _ := value[key].(map[string]any)
	return result
}
func stringField(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}

func validRealtimeSettings(session map[string]any, dash bool) bool {
	input := session
	if dash {
		rate := session["sample_rate"]
		if session["input_audio_format"] != "pcm" || (rate != float64(8000) && rate != float64(16000)) {
			return false
		}
		if _, invalid := session["audio"]; invalid {
			return false
		}
	} else {
		if session["type"] != "transcription" {
			return false
		}
		input = objectField(objectField(session, "audio"), "input")
		format := objectField(input, "format")
		if format["type"] != "audio/pcm" || format["rate"] != float64(24000) || stringField(objectField(input, "transcription"), "model") == "" {
			return false
		}
	}
	vad := objectField(input, "turn_detection")
	threshold, thresholdOK := vad["threshold"].(float64)
	silence, silenceOK := vad["silence_duration_ms"].(float64)
	if vad["type"] != "server_vad" || !thresholdOK || threshold < 0 || threshold > 1 || !silenceOK || silence <= 0 {
		return false
	}
	if !dash {
		prefix, ok := vad["prefix_padding_ms"].(float64)
		if !ok || prefix < 0 {
			return false
		}
	}
	return true
}
