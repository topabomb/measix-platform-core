package workspace

import (
	"context"
	"time"

	remoteapi "measix/platform/internal/hub/agentspace"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// Resources is an admin observation, not a file lease or lifecycle transition.
// Keep remote IO outside transactions and never update the durable projection.
func (s *Service) Resources(ctx context.Context, userID, spaceID string, authorize func(context.Context) error) (adminapi.WorkspaceResources, error) {
	var out adminapi.WorkspaceResources
	if platformid.Validate(platformid.AgentSpace, spaceID) != nil {
		return out, ErrInvalid
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return out, err
		}
	}
	row, err := s.Client.AgentSpace.Get(ctx, userID)
	if err != nil {
		return out, err
	}
	if row.AgentSpaceID != spaceID {
		return out, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	service, err := s.Client.WorkspaceService.Get(ctx, row.WorkspaceServiceID)
	if err != nil {
		return out, err
	}
	if service.ActiveConfigRevision == nil {
		return out, ErrUnavailable
	}
	cfg, err := s.config(ctx, service.ID, *service.ActiveConfigRevision)
	if err != nil {
		return out, err
	}
	client, err := s.adapter(ctx, cfg)
	if err != nil {
		return out, err
	}
	// Covers all bounded upstream collection stages, independently of file IO.
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, readErr := client.Resources(readCtx, row.RemoteUsername, spaceID)
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return adminapi.WorkspaceResources{}, err
		}
	}
	fresh, err := s.Client.AgentSpace.Get(ctx, userID)
	if err != nil {
		return adminapi.WorkspaceResources{}, err
	}
	if fresh.AgentSpaceID != spaceID {
		return adminapi.WorkspaceResources{}, &remoteapi.Error{Code: "workspace_space_mismatch"}
	}
	current, err := s.Client.WorkspaceService.Get(ctx, service.ID)
	if err != nil {
		return adminapi.WorkspaceResources{}, err
	}
	if fresh.BindingRevision != row.BindingRevision || fresh.State != row.State || fresh.Intent != row.Intent || current.ConfigRevision != service.ConfigRevision || current.ActiveConfigRevision == nil || *current.ActiveConfigRevision != *service.ActiveConfigRevision || current.Enabled != service.Enabled {
		return adminapi.WorkspaceResources{}, ErrConflict
	}
	return out, readErr
}
