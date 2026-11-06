package runtimecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"measix/platform/ent"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/workspace"
	"measix/platform/internal/wire/relaycontrolapi"
)

// Compilation precedes the durable intent transaction. Workspace commands can
// commit in that gap without advancing the global control revision. Re-read
// the complete workspace binding sets inside the same transaction that saves
// the Activation, so a stale permission/key/connection can never become an
// immutable pending descriptor which recovery is correctly unable to replay.
// This only checks local state; it performs no Relay or remote management IO.
func (s *Service) validateWorkspaceIntentTx(ctx context.Context, tx *ent.Tx, descriptor []byte) error {
	var state relaycontrolapi.RuntimeControlState
	if err := json.Unmarshal(descriptor, &state); err != nil {
		return err
	}
	routes := make(map[string]relaycontrolapi.RuntimeRouteSpec)
	for _, route := range state.Routes {
		if route.WorkspaceServiceId != nil {
			routes[route.RuntimeRouteId] = route
		}
	}
	if len(routes) == 0 {
		return nil
	}
	if s.Workspace == nil || s.Workspace.Secrets == nil {
		return fmt.Errorf("%w: workspace authority unavailable", ErrActivationInProgress)
	}
	client := tx.Client()
	secrets := upstream.NewService(client, s.Workspace.Secrets.Box)
	current := workspace.NewService(client, secrets)
	wantDAV, gotDAV := map[string]relaycontrolapi.UserDAVBinding{}, map[string]relaycontrolapi.UserDAVBinding{}
	wantMCP, gotMCP := map[string]relaycontrolapi.UserRuntimeBinding{}, map[string]relaycontrolapi.UserRuntimeBinding{}
	if state.DavBindings != nil {
		for _, binding := range *state.DavBindings {
			binding.Token = ""
			wantDAV[binding.UserId+"/"+binding.ResourceId] = binding
		}
	}
	if state.UserBindings != nil {
		for _, binding := range *state.UserBindings {
			binding.Token = ""
			wantMCP[binding.UserId+"/"+binding.McpServerId] = binding
		}
	}
	for _, resource := range state.ResourceRoutes {
		route, ok := routes[resource.RuntimeRouteId]
		if !ok {
			continue
		}
		var connect, idle int
		if resource.ResourceKind == "WEBDAV" {
			cfg, bindings, err := current.DAVRuntimeBindings(ctx, *route.WorkspaceServiceId, resource.ResourceId)
			if err != nil {
				return fmt.Errorf("%w: DAV binding changed: %v", ErrActivationInProgress, err)
			}
			connect, idle = cfg.ConnectTimeoutMs, cfg.IdleTimeoutMs
			for _, binding := range bindings {
				binding.Token = ""
				gotDAV[binding.UserId+"/"+binding.ResourceId] = binding
			}
		} else {
			cfg, bindings, err := current.RuntimeBindings(ctx, *route.WorkspaceServiceId, resource.ResourceId)
			if err != nil {
				return fmt.Errorf("%w: MCP binding changed: %v", ErrActivationInProgress, err)
			}
			connect, idle = cfg.ConnectTimeoutMs, cfg.IdleTimeoutMs
			for _, binding := range bindings {
				binding.Token = ""
				gotMCP[binding.UserId+"/"+binding.McpServerId] = binding
			}
		}
		if route.TimeoutPolicy.ConnectMs != connect || route.TimeoutPolicy.ResponseHeaderMs != connect || route.TimeoutPolicy.IdleMs != idle {
			return fmt.Errorf("%w: workspace configuration changed", ErrActivationInProgress)
		}
	}
	if !maps.Equal(wantDAV, gotDAV) || !maps.Equal(wantMCP, gotMCP) {
		return fmt.Errorf("%w: workspace bindings changed", ErrActivationInProgress)
	}
	return nil
}
