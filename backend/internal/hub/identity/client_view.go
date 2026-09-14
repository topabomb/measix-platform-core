package identity

import (
	"context"
	"time"
)

type DiscoveryView struct {
	DeploymentID   string
	DeploymentName string
}

type ManagedStateView struct {
	ActiveManagedGeneration int
	ManagedStateRevision    int
	RuntimeStatus           string
}

type BootstrapView struct {
	Principal        AccessPrincipal
	DeploymentName   string
	UserDisplayName  string
	DeviceStatus     string
	SessionExpiresAt time.Time
	ManagedState     ManagedStateView
}

func (s *Service) Discovery(ctx context.Context) (DiscoveryView, error) {
	deployment, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return DiscoveryView{}, err
	}
	return DiscoveryView{DeploymentID: deployment.ID, DeploymentName: deployment.Name}, nil
}

func (s *Service) ManagedState(ctx context.Context) (ManagedStateView, error) {
	state, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return ManagedStateView{}, err
	}
	return ManagedStateView{
		ActiveManagedGeneration: int(state.ActiveManagedGeneration),
		ManagedStateRevision:    int(state.ManagedStateRevision),
		RuntimeStatus:           state.RuntimeStatus,
	}, nil
}

func (s *Service) BootstrapView(ctx context.Context, accessToken string) (BootstrapView, error) {
	principal, err := s.AuthenticateAccess(ctx, accessToken)
	if err != nil {
		return BootstrapView{}, err
	}
	deployment, err := s.Client.Deployment.Get(ctx, principal.DeploymentID)
	if err != nil {
		return BootstrapView{}, err
	}
	u, err := s.Client.User.Get(ctx, principal.UserID)
	if err != nil {
		return BootstrapView{}, err
	}
	d, err := s.Client.Device.Get(ctx, principal.DeviceID)
	if err != nil {
		return BootstrapView{}, err
	}
	se, err := s.Client.Session.Get(ctx, principal.SessionID)
	if err != nil {
		return BootstrapView{}, err
	}
	state, err := s.ManagedState(ctx)
	if err != nil {
		return BootstrapView{}, err
	}
	return BootstrapView{
		Principal:        principal,
		DeploymentName:   deployment.Name,
		UserDisplayName:  u.DisplayName,
		DeviceStatus:     d.Status,
		SessionExpiresAt: se.ExpiresAt,
		ManagedState:     state,
	}, nil
}
