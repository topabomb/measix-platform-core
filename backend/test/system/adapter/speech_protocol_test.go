package adapter

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSpeechProtocolsRejectWrongWireAndReturnPCM(t *testing.T) {
	a := New()
	defer a.Close()
	for _, tc := range []struct {
		name, path, body, expected string
		status                     int
	}{
		{"mimo", "/v1/chat/completions", `{"model":"mimo-v2.5-tts","messages":[{"role":"assistant","content":"hello"}],"audio":{"voice":"mimo_default","format":"pcm16"},"stream":true}`, `"audio":{"data":"AAABAP//AAA="}`, 200},
		{"mimo-design", "/v1/chat/completions", `{"model":"mimo-v2.5-tts-voicedesign","messages":[{"role":"user","content":"Warm voice"},{"role":"assistant","content":"hello"}],"audio":{"format":"pcm16"},"stream":true}`, "[DONE]", 200},
		{"mimo-wrong-role", "/v1/chat/completions", `{"model":"mimo-v2.5-tts","messages":[{"role":"user","content":"hello"}],"audio":{"voice":"mimo_default","format":"pcm16"},"stream":true}`, "", 400},
		{"mimo-missing-design", "/v1/chat/completions", `{"model":"mimo-v2.5-tts-voicedesign","messages":[{"role":"assistant","content":"hello"}],"audio":{"format":"pcm16"},"stream":true}`, "", 400},
		{"gemini", "/v1beta/models/gemini-2.5-flash-preview-tts:generateContent", `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"responseModalities":["AUDIO"],"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Kore"}}}}}`, `"data":"AAABAP//AAA="`, 200},
		{"gemini-openai-body", "/v1beta/models/gemini-2.5-flash-preview-tts:generateContent", `{"model":"tts","input":"hello","voice":"alloy"}`, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(a.URL+tc.path, "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status || !strings.Contains(string(body), tc.expected) {
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
		})
	}
}
