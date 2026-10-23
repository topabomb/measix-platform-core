package agentspace

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"measix/platform/internal/wire/adminapi"
)

// Resources consumes only the pinned release's documented summary. User.Get
// deliberately does not parse this optional observation during control actions.
func (c *Client) Resources(ctx context.Context, name, space string) (adminapi.WorkspaceResources, error) {
	var out adminapi.WorkspaceResources
	user, err := c.Get(ctx, name)
	if err != nil {
		return out, err
	}
	if user.AgentSpaceID != space {
		return out, &Error{Code: "workspace_space_mismatch"}
	}
	if len(user.Resources) == 0 || string(user.Resources) == "null" {
		return out, &Error{Code: "resource_summary_unavailable"}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(user.Resources, &fields) != nil || json.Unmarshal(fields["agentSpaceId"], &out.AgentSpaceId) != nil {
		return out, &Error{Code: "invalid_remote_response"}
	}
	if out.AgentSpaceId != space {
		return adminapi.WorkspaceResources{}, &Error{Code: "workspace_space_mismatch"}
	}
	invalid := "invalid_resource_sample"
	// Decode independent observations separately so a malformed metric cannot
	// hide healthy ones. All values still use the generated Admin wire types.
	runtimeErr := json.Unmarshal(fields["runtime"], &out.Runtime)
	runtimeValid := out.Runtime.Value != nil && slices.Contains([]string{"running", "stopped", "paused", "transitioning", "notfound", "unknown"}, *out.Runtime.Value)
	if runtimeErr != nil || !validObservation(out.Runtime.Status, out.Runtime.ObservedAt, out.Runtime.Value != nil) || out.Runtime.Value != nil && !runtimeValid {
		out.Runtime = adminapi.WorkspaceRuntimeObservation{Status: "error", Reason: &invalid}
	}
	if json.Unmarshal(fields["memory"], &out.Memory) != nil || !validObservation(out.Memory.Status, out.Memory.ObservedAt, out.Memory.Value != nil) || out.Memory.Value != nil && !validResourceNumber(*out.Memory.Value) {
		out.Memory = adminapi.WorkspaceMemoryObservation{Status: "error", Reason: &invalid}
	}
	diskValid := json.Unmarshal(fields["disk"], &out.Disk) == nil
	if out.Disk.Value != nil {
		var disk struct {
			Value map[string]json.RawMessage
		}
		diskValid = diskValid && json.Unmarshal(fields["disk"], &disk) == nil && validResourceNumber(out.Disk.Value.UsedBytes) && validResourceNumber(out.Disk.Value.AvailableBytes)
		for _, key := range []string{"usedBytes", "availableBytes"} {
			diskValid = diskValid && len(disk.Value[key]) > 0 && string(disk.Value[key]) != "null"
		}
	}
	if !validObservation(out.Disk.Status, out.Disk.ObservedAt, out.Disk.Value != nil) || !diskValid {
		out.Disk = adminapi.WorkspaceDiskObservation{Status: "error", Reason: &invalid}
	}
	var allocation map[string]json.RawMessage
	if json.Unmarshal(fields["allocation"], &allocation) != nil {
		allocation = nil
	}
	for key, a := range map[string]*adminapi.WorkspaceResourceAllocation{"cpuCores": &out.Allocation.CpuCores, "memoryLimitBytes": &out.Allocation.MemoryLimitBytes, "workspaceCapacityBytes": &out.Allocation.WorkspaceCapacityBytes} {
		if json.Unmarshal(allocation[key], a) != nil || !a.Source.Valid() || a.Source == "unknown" && a.Value != nil || a.Source != "unknown" && (a.Value == nil || !validResourceNumber(*a.Value) || *a.Value == 0) {
			*a = adminapi.WorkspaceResourceAllocation{Source: "unknown", Error: &invalid}
		}
		a.Error = safeResourceReason(a.Error)
	}
	out.Runtime.Reason = safeResourceReason(out.Runtime.Reason)
	out.Memory.Reason = safeResourceReason(out.Memory.Reason)
	out.Disk.Reason = safeResourceReason(out.Disk.Reason)
	return out, nil
}

func validResourceNumber(n int64) bool { return n >= 0 && n <= 9007199254740991 }
func validObservation(status adminapi.WorkspaceObservationStatus, at *int64, hasValue bool) bool {
	switch status {
	case "current", "historical":
		return hasValue && at != nil && *at > 0 && *at <= time.Now().Add(time.Minute).UnixMilli()
	case "unavailable", "error":
		return !hasValue && at == nil
	default:
		return false
	}
}

// Do not pass arbitrary upstream diagnostic text (paths, URLs or credentials).
func safeResourceReason(reason *string) *string {
	if reason == nil {
		return nil
	}
	if slices.Contains([]string{"stopped", "paused", "transitioning", "notfound", "unknown", "account_inactive", "account_changed", "not_sampled", "sampling_disabled", "guest_memory_unavailable", "stale_msb_snapshot", "collection_busy", "collection_timeout", "workspace_stat_failed", "runtime_changed", "inspection_failed", "status_unavailable", "metrics_unavailable", "run_identity_unavailable", "workspace_disk_unverified", "runtime_exited", "sample_time_unavailable", "invalid_memory_sample", "configuration_unavailable", "workspace_capacity_unavailable", "workspace_disk_mismatch", "workspace_profile_unavailable", "workspace_volume_unavailable", "workspace_volume_unregistered", "invalid_resource_sample"}, *reason) {
		return reason
	}
	value := "observation_unavailable"
	return &value
}
