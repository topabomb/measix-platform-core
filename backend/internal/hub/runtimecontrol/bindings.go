package runtimecontrol

import "measix/platform/internal/wire/adminapi"

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
	for _, tts := range content.Tts {
		enabled[tts.TtsId] = tts.Enabled && tts.ClientProtocol != adminapi.SYSTEMTTS
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
