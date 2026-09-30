// Package usagetarget validates the versioned immutable wire target shared by
// admission and settlement. It has no Hub persistence or business dependency.
package usagetarget

import (
	"encoding/json"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func Valid(upstream, kind string, version int, target *usageingestapi.WorkspaceTarget) bool {
	if target == nil {
		return version == 0 && platformid.Validate(platformid.Upstream, upstream) == nil
	}
	if upstream != "" || kind != "MCP" || version != 2 || platformid.Validate(platformid.WorkspaceService, target.WorkspaceServiceId) != nil || platformid.Validate(platformid.AgentSpace, target.AgentSpaceId) != nil || target.BindingRevision < 1 {
		return false
	}
	name := target.RemoteUsername
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '_' || c == '.' || c == '-') {
			continue
		}
		return false
	}
	return true
}
func JSON(target *usageingestapi.WorkspaceTarget) []byte {
	if target == nil {
		return nil
	}
	b, _ := json.Marshal(target)
	return b
}
func Upstream(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
