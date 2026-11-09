package protocolusage

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

type TTSProtocol string

const (
	OpenAIAudioSpeech        TTSProtocol = "OPENAI_AUDIO_SPEECH"
	GeminiGenerateContentTTS TTSProtocol = "GEMINI_GENERATE_CONTENT_TTS"
	MiMoChatCompletionsTTS   TTSProtocol = "MIMO_CHAT_COMPLETIONS_TTS"
)

func CountTTSCharacters(protocol TTSProtocol, body []byte) Result {
	result := requestResult("provider_request")
	var characterCount int64
	var found bool
	var err error
	switch protocol {
	case OpenAIAudioSpeech:
		var request struct {
			Input *string `json:"input"`
		}
		if decodeErr := json.Unmarshal(body, &request); decodeErr != nil {
			err = decodeErr
		} else if request.Input != nil {
			characterCount = int64(utf8.RuneCountInString(*request.Input))
			found = true
		}
	case GeminiGenerateContentTTS:
		var request struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text *string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if decodeErr := json.Unmarshal(body, &request); decodeErr != nil {
			err = decodeErr
		} else {
			for _, content := range request.Contents {
				if content.Role != "" && content.Role != "user" {
					continue
				}
				for _, part := range content.Parts {
					if part.Text != nil {
						characterCount += int64(utf8.RuneCountInString(*part.Text))
						found = true
					}
				}
			}
		}
	case MiMoChatCompletionsTTS:
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if decodeErr := json.Unmarshal(body, &request); decodeErr != nil {
			err = decodeErr
		} else {
			for index := len(request.Messages) - 1; index >= 0; index-- {
				message := request.Messages[index]
				if message.Role != "assistant" {
					continue
				}
				var content string
				if decodeErr := json.Unmarshal(message.Content, &content); decodeErr == nil {
					characterCount = int64(utf8.RuneCountInString(content))
					found = true
				}
				break
			}
		}
	default:
		err = fmt.Errorf("unsupported TTS protocol %q", protocol)
	}
	if err != nil {
		result.diagnostic("invalid_tts_request", err.Error())
	}
	measurement := Measurement{Meter: Characters, Source: "request_text", Completeness: Unknown}
	if found {
		measurement.Value = count(characterCount)
		measurement.Completeness = Exact
	} else if err == nil {
		result.diagnostic("tts_text_unavailable", "the protocol target text field is absent or unsupported")
	}
	result.set(measurement)
	return result
}
