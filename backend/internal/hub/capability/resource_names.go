package capability

import (
	"encoding/json"
	"measix/platform/internal/wire/clientapi"
)

func SnapshotResourceNames(raw []byte) (map[string]string, error) {
	var snapshot clientapi.ManagedSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, r := range snapshot.Providers {
		names[r.ProviderId] = r.DisplayName
	}
	for _, r := range snapshot.Models {
		names[r.ModelId] = r.DisplayName
	}
	if snapshot.ImageGenerators != nil {
		for _, r := range *snapshot.ImageGenerators {
			names[r.ImageId] = r.DisplayName
		}
	}
	for _, r := range snapshot.Tts {
		names[r.TtsId] = r.DisplayName
	}
	for _, r := range snapshot.Asr {
		names[r.AsrId] = r.DisplayName
	}
	for _, r := range snapshot.Mcp {
		names[r.McpServerId] = r.DisplayName
	}
	return names, nil
}
