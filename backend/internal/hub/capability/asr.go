package capability

import (
	"measix/platform/internal/wire/adminapi"
	"strings"
)

func validateASRSettings(value adminapi.AsrDefinition) (string, string) {
	if value.ClientProtocol == adminapi.AsrDefinitionClientProtocolOPENAIAUDIOTRANSCRIPTIONS || value.ClientProtocol == adminapi.AsrDefinitionClientProtocolDASHSCOPEHTTPASR {
		if value.SampleRate != nil || value.VadThreshold != nil || value.SilenceDurationMs != nil || value.PrefixPaddingMs != nil || value.Prompt != nil {
			return "clientProtocol", "HTTP transcription must not contain realtime settings"
		}
		return "", ""
	}
	if !value.ClientProtocol.Valid() {
		return "", ""
	}
	openAI := value.ClientProtocol == adminapi.AsrDefinitionClientProtocolOPENAIREALTIMETRANSCRIPTION
	if value.SampleRate == nil || (openAI && *value.SampleRate != 24000) || (!openAI && *value.SampleRate != 8000 && *value.SampleRate != 16000) {
		return "sampleRate", "select a PCM sample rate supported by the ASR protocol"
	}
	if value.VadThreshold == nil || *value.VadThreshold < 0 || *value.VadThreshold > 1 {
		return "vadThreshold", "VAD threshold must be between 0 and 1"
	}
	if value.SilenceDurationMs == nil || *value.SilenceDurationMs <= 0 {
		return "silenceDurationMs", "silence duration must be positive"
	}
	if openAI {
		if value.PrefixPaddingMs == nil || *value.PrefixPaddingMs < 0 {
			return "prefixPaddingMs", "OpenAI realtime requires non-negative prefix padding"
		}
		if value.Prompt != nil && strings.TrimSpace(*value.Prompt) == "" {
			return "prompt", "omit the prompt or enter non-empty text"
		}
	} else if value.PrefixPaddingMs != nil || value.Prompt != nil {
		return "clientProtocol", "DashScope must not contain OpenAI-only settings"
	}
	return "", ""
}
