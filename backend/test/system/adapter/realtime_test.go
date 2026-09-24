package adapter_test

import (
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"measix/platform/test/system/adapter"
)

func TestDashScopeWaitsForCloseAcknowledgement(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(a.URL, "http://", "ws://", 1)+"/api-ws/v1/realtime?model=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetCloseHandler(func(int, string) error { return nil })
	read := func() {
		t.Helper()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"event_id":"setup","type":"session.update","session":{"input_audio_format":"pcm","sample_rate":16000,"turn_detection":{"type":"server_vad","threshold":0,"silence_duration_ms":400}}}`)); err != nil {
		t.Fatal(err)
	}
	read()
	if err := conn.WriteJSON(map[string]any{"event_id": "finish", "type": "session.finish"}); err != nil {
		t.Fatal(err)
	}
	read()
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("close frame: %v", err)
	}
	raw := conn.UnderlyingConn()
	_ = raw.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, err = raw.Read(make([]byte, 1))
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("TCP closed before client acknowledgement: %v", err)
	}
	if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

// These are protocol-shaped synthetic sessions, not speech recognition accuracy tests.
func TestRealtimeASRSessions(t *testing.T) {
	for _, tc := range []struct{ name, path, settings, interim string }{
		{"openai", "/v1/realtime?intent=transcription", `{"type":"session.update","session":{"type":"transcription","audio":{"input":{"format":{"type":"audio/pcm","rate":24000},"transcription":{"model":"gpt-4o-transcribe","language":"zh"},"turn_detection":{"type":"server_vad","threshold":0.5,"prefix_padding_ms":300,"silence_duration_ms":500}}}}}`, "conversation.item.input_audio_transcription.delta"},
		{"dashscope", "/api-ws/v1/realtime?model=qwen3-asr-flash-realtime", `{"event_id":"evt_setup","type":"session.update","session":{"input_audio_format":"pcm","sample_rate":16000,"input_audio_transcription":{"language":"zh"},"turn_detection":{"type":"server_vad","threshold":0,"silence_duration_ms":400}}}`, "conversation.item.input_audio_transcription.text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := adapter.New()
			defer a.Close()
			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(a.URL, "http://", "ws://", 1)+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			read := func(want string) map[string]any {
				t.Helper()
				var event map[string]any
				if err := conn.ReadJSON(&event); err != nil {
					t.Fatal(err)
				}
				if event["type"] != want {
					t.Fatalf("event=%v want=%s", event, want)
				}
				return event
			}
			read("session.created")
			if err := conn.WriteMessage(websocket.TextMessage, []byte(tc.settings)); err != nil {
				t.Fatal(err)
			}
			read("session.updated")
			// 100 ms of mono PCM16. No real microphone data or account credential.
			if err := conn.WriteJSON(map[string]any{"event_id": "evt_audio", "type": "input_audio_buffer.append", "audio": base64.StdEncoding.EncodeToString(make([]byte, 4800))}); err != nil {
				t.Fatal(err)
			}
			read(tc.interim)
			if got := read("conversation.item.input_audio_transcription.completed")["transcript"]; got != "协议验证" {
				t.Fatalf("transcript=%v", got)
			}
			if tc.name == "dashscope" {
				if err := conn.WriteJSON(map[string]any{"event_id": "evt_finish", "type": "session.finish"}); err != nil {
					t.Fatal(err)
				}
				read("session.finished")
			}
		})
	}
}

func TestRealtimeASRRejectsInvalidClientEvents(t *testing.T) {
	for _, tc := range []struct{ name, path, event string }{
		{"missing-event-id", "/api-ws/v1/realtime?model=qwen3-asr-flash-realtime", `{"type":"session.update","session":{}}`},
		{"wrong-sample-rate", "/api-ws/v1/realtime?model=qwen3-asr-flash-realtime", `{"event_id":"evt_setup","type":"session.update","session":{"input_audio_format":"pcm","sample_rate":24000,"turn_detection":{"type":"server_vad","threshold":0,"silence_duration_ms":400}}}`},
		{"audio-before-settings", "/v1/realtime?intent=transcription", `{"type":"input_audio_buffer.append","audio":"AAABAA=="}`},
		{"wrong-openai-format", "/v1/realtime?intent=transcription", `{"type":"session.update","session":{"type":"transcription","audio":{"input":{"format":{"type":"audio/pcm","rate":16000},"transcription":{"model":"gpt-4o-transcribe"},"turn_detection":{"type":"server_vad","threshold":0.5,"prefix_padding_ms":300,"silence_duration_ms":500}}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := adapter.New()
			defer a.Close()
			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(a.URL, "http://", "ws://", 1)+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			var event map[string]any
			if err := conn.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(tc.event)); err != nil {
				t.Fatal(err)
			}
			if err := conn.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event["type"] != "error" {
				t.Fatalf("invalid event accepted: %v", event)
			}
		})
	}
}
