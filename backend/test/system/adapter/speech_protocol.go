package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Eight deterministic PCM bytes, not synthesized speech or provider quality evidence.
const speechPCM = "AAABAP//AAA="

func (a *Adapter) handleMiMoSpeech(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	body := fact.BodyJSON
	model, _ := body["model"].(string)
	audio, _ := body["audio"].(map[string]any)
	messages, _ := body["messages"].([]any)
	valid := r.Method == http.MethodPost && body["stream"] == true && audio["format"] == "pcm16" && len(messages) > 0
	description := false
	for i, item := range messages {
		message, _ := item.(map[string]any)
		content, _ := message["content"].(string)
		if strings.TrimSpace(content) == "" {
			valid = false
		}
		if i == len(messages)-1 {
			valid = valid && message["role"] == "assistant"
		} else {
			valid = valid && message["role"] == "user"
			description = description || strings.TrimSpace(content) != ""
		}
	}
	voice, _ := audio["voice"].(string)
	if strings.Contains(model, "voicedesign") {
		_, hasVoice := audio["voice"]
		valid = valid && description && !hasVoice
	} else {
		valid = valid && strings.TrimSpace(voice) != ""
	}
	if !valid {
		http.Error(w, "invalid MiMo speech request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"audio\":{\"data\":\"%s\"}}}]}\n\ndata: [DONE]\n\n", speechPCM)
}

func (a *Adapter) handleGeminiSpeech(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	body := fact.BodyJSON
	config, _ := body["generationConfig"].(map[string]any)
	modalities, _ := config["responseModalities"].([]any)
	speech, _ := config["speechConfig"].(map[string]any)
	voice, _ := speech["voiceConfig"].(map[string]any)
	prebuilt, _ := voice["prebuiltVoiceConfig"].(map[string]any)
	name, _ := prebuilt["voiceName"].(string)
	contents, _ := body["contents"].([]any)
	text := ""
	if len(contents) > 0 {
		content, _ := contents[0].(map[string]any)
		parts, _ := content["parts"].([]any)
		if len(parts) > 0 {
			part, _ := parts[0].(map[string]any)
			text, _ = part["text"].(string)
		}
	}
	if r.Method != http.MethodPost || len(modalities) != 1 || modalities[0] != "AUDIO" || strings.TrimSpace(name) == "" || strings.TrimSpace(text) == "" {
		http.Error(w, "invalid Gemini speech request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": "audio/L16;codec=pcm;rate=24000", "data": speechPCM}}}}}}})
}
