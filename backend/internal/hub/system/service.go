package system

import (
	"context"
	"time"

	"measix/platform/backend/ent"
	"measix/platform/backend/ent/activation"
	"measix/platform/backend/ent/requestusage"
	"measix/platform/backend/internal/hub/maintenance"
	"measix/platform/backend/internal/hub/runtimecontrol"
	"measix/platform/backend/internal/hub/store"
	"measix/platform/backend/internal/wire/relaycontrolapi"
)

type Service struct {
	Store          *store.Store
	RuntimeControl *runtimecontrol.Service
	BuildVersion   string
	Now            func() time.Time
}

type Status struct {
	BuildVersion                 string
	DBHealth                     string
	MigrationRevision            string
	RuntimeStatus                string
	ActiveManagedGeneration      int
	ManagedStateRevision         int
	DesiredControlRevision       int
	DesiredBundleHash            *string
	RelayReady                   bool
	AppliedControlRevision       *int
	AppliedBundleHash            *string
	LastRelaySeenAt              *time.Time
	LatestActivation             *ent.Activation
	RequestUsageIngestLagSeconds *int
	SemanticOrphanCount          *int
}

func New(store *store.Store, control *runtimecontrol.Service, buildVersion string) *Service {
	return &Service{Store: store, RuntimeControl: control, BuildVersion: buildVersion, Now: time.Now}
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	result := Status{BuildVersion: s.BuildVersion, MigrationRevision: maintenance.CurrentSchemaRevision}
	if _, err := maintenance.Check(ctx, s.Store.DB); err != nil {
		result.DBHealth = "DEGRADED"
	} else {
		result.DBHealth = "OK"
	}
	managed, err := s.Store.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return result, err
	}
	result.RuntimeStatus = managed.RuntimeStatus
	result.ActiveManagedGeneration = int(managed.ActiveManagedGeneration)
	result.ManagedStateRevision = int(managed.ManagedStateRevision)
	result.DesiredControlRevision = int(managed.DesiredControlRevision)
	result.DesiredBundleHash = managed.DesiredBundleHash

	if s.RuntimeControl != nil && s.RuntimeControl.Relay != nil {
		status, err := s.RuntimeControl.Relay.Status(ctx)
		if err == nil {
			now := s.Now().UTC()
			result.LastRelaySeenAt = &now
			result.RelayReady = status.Ready
			appliedRevision := status.AppliedControlRevision
			result.AppliedControlRevision = &appliedRevision
			if status.BundleHash != "" {
				hash := status.BundleHash
				result.AppliedBundleHash = &hash
			}
		}
	}
	latest, err := s.Store.Client.Activation.Query().Order(ent.Desc(activation.FieldCreatedAt)).First(ctx)
	if err == nil {
		result.LatestActivation = latest
	} else if !ent.IsNotFound(err) {
		return result, err
	}
	usageRow, err := s.Store.Client.RequestUsage.Query().Order(ent.Desc(requestusage.FieldIngestedAt)).First(ctx)
	if err == nil {
		lag := int(usageRow.IngestedAt.Sub(usageRow.CompletedAt).Seconds())
		if lag < 0 {
			lag = 0
		}
		result.RequestUsageIngestLagSeconds = &lag
	} else if !ent.IsNotFound(err) {
		return result, err
	}
	orphan, err := semanticOrphanCount(ctx, s.Store.Client)
	if err != nil {
		return result, err
	}
	result.SemanticOrphanCount = &orphan
	return result, nil
}

func semanticOrphanCount(ctx context.Context, client *ent.Client) (int, error) {
	rows, err := client.SemanticUsage.Query().All(ctx)
	if err != nil {
		return 0, err
	}
	orphan := 0
	for _, row := range rows {
		if row.RequestID == nil {
			continue
		}
		exists, err := client.RequestUsage.Query().Where(requestusage.RequestIDEQ(*row.RequestID)).Exist(ctx)
		if err != nil {
			return 0, err
		}
		if !exists {
			orphan++
		}
	}
	return orphan, nil
}

var _ relaycontrolapi.ControlStatus
