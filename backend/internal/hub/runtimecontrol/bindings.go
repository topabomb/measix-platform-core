package runtimecontrol

import (
	"sort"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
)

type resourceMeterProfile struct {
	kind, protocol string
	audio          *resourceAudioProfile
	llm            *resourceLLMProfile
	modelMapping   *resourceModelMapping
	image          *resourceImageProfile
}

type resourceModelMapping struct {
	publishedModelKey, upstreamModelKey    string
	clientRuntimePath, upstreamRuntimePath string
}

type resourceImageProfile struct {
	maxImagesPerRequest int
	allowedSizes        []string
}

type resourceLLMProfile struct {
	geminiThoughtsMayBeAbsent       bool
	anthropicCacheFieldsMayBeAbsent bool
}

type resourceAudioProfile struct {
	encoding    string
	sampleRates []int
}

// Only enabled resources contribute routes. Saved bindings remain in the Release
// so a later explicit Publish can re-enable them without losing configuration.
func enabledBindings(content adminapi.ManagedDraftContent) []adminapi.RuntimeBindingDefinition {
	providers := make(map[string]bool, len(content.Providers))
	for _, provider := range content.Providers {
		providers[provider.ProviderId] = provider.Enabled
	}
	enabled := make(map[string]bool)
	for _, model := range content.Models {
		enabled[model.ModelId] = model.Enabled && providers[model.ProviderId]
	}
	if content.ImageGenerators != nil {
		for _, image := range *content.ImageGenerators {
			enabled[image.ImageId] = image.Enabled
		}
	}
	for _, tts := range content.Tts {
		enabled[tts.TtsId] = tts.Enabled && tts.ClientProtocol != adminapi.TtsDefinitionClientProtocolSYSTEMTTS
	}
	for _, asr := range content.Asr {
		enabled[asr.AsrId] = asr.Enabled
	}
	for _, mcp := range content.Mcp {
		enabled[mcp.McpServerId] = mcp.Enabled
	}
	bindings := make([]adminapi.RuntimeBindingDefinition, 0, len(content.Bindings))
	for _, binding := range content.Bindings {
		if enabled[binding.ResourceId] {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

func meterProfiles(content adminapi.ManagedDraftContent) (map[string]resourceMeterProfile, error) {
	imageCount := 0
	if content.ImageGenerators != nil {
		imageCount = len(*content.ImageGenerators)
	}
	profiles := make(map[string]resourceMeterProfile, len(content.Models)+imageCount+len(content.Tts)+len(content.Asr)+len(content.Mcp))
	providerProtocols := make(map[string]string, len(content.Providers))
	for _, provider := range content.Providers {
		providerProtocols[provider.ProviderId] = string(provider.ClientProtocol)
	}
	for _, model := range content.Models {
		reasoning := false
		for _, capability := range model.Capabilities {
			if capability == adminapi.REASONING {
				reasoning = true
			}
		}
		protocol := providerProtocols[model.ProviderId]
		publishedKey := capability.EffectivePublishedModelKey(model)
		clientPath, err := capability.ModelClientRuntimePath(adminapi.ProviderDefinitionClientProtocol(protocol), model.RuntimePath, model.UpstreamModelKey, publishedKey)
		if err != nil {
			return nil, err
		}
		profiles[model.ModelId] = resourceMeterProfile{kind: "MODEL", protocol: protocol, modelMapping: &resourceModelMapping{
			publishedModelKey: publishedKey, upstreamModelKey: model.UpstreamModelKey,
			clientRuntimePath: clientPath, upstreamRuntimePath: model.RuntimePath,
		}, llm: &resourceLLMProfile{
			geminiThoughtsMayBeAbsent:       protocol == "GOOGLE_GENERATE_CONTENT" && !reasoning,
			anthropicCacheFieldsMayBeAbsent: protocol == "ANTHROPIC_MESSAGES",
		}}
	}
	if content.ImageGenerators != nil {
		for _, image := range *content.ImageGenerators {
			sizes := append([]string(nil), image.AllowedSizes...)
			sort.Strings(sizes)
			profiles[image.ImageId] = resourceMeterProfile{
				kind: "IMAGE_GENERATION", protocol: string(image.ClientProtocol),
				image: &resourceImageProfile{maxImagesPerRequest: image.MaxImagesPerRequest, allowedSizes: sizes},
			}
		}
	}
	for _, tts := range content.Tts {
		if tts.ClientProtocol != adminapi.TtsDefinitionClientProtocolSYSTEMTTS {
			profiles[tts.TtsId] = resourceMeterProfile{kind: "TTS", protocol: string(tts.ClientProtocol)}
		}
	}
	for _, asr := range content.Asr {
		profile := resourceMeterProfile{kind: "ASR", protocol: string(asr.ClientProtocol)}
		profile.audio = asrAudioProfile(asr)
		profiles[asr.AsrId] = profile
	}
	for _, mcp := range content.Mcp {
		profiles[mcp.McpServerId] = resourceMeterProfile{kind: "MCP", protocol: string(mcp.ClientProtocol)}
	}
	return profiles, nil
}

func asrAudioProfile(asr adminapi.AsrDefinition) *resourceAudioProfile {
	encoding := "WAV_PCM16_LE"
	rates := []int{}
	switch asr.ClientProtocol {
	case adminapi.AsrDefinitionClientProtocolOPENAIREALTIMETRANSCRIPTION:
		encoding, rates = "PCM16_LE", []int{24000}
	case adminapi.AsrDefinitionClientProtocolDASHSCOPEREALTIMEASR:
		encoding, rates = "PCM16_LE", []int{8000, 16000}
	case adminapi.AsrDefinitionClientProtocolOPENAIAUDIOTRANSCRIPTIONS:
		rates = []int{16000, 24000}
	case adminapi.AsrDefinitionClientProtocolDASHSCOPEHTTPASR:
		rates = []int{16000}
	}
	if asr.SampleRate != nil {
		rates = []int{int(*asr.SampleRate)}
	}
	return &resourceAudioProfile{encoding: encoding, sampleRates: rates}
}
