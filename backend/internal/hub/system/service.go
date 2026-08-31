package system

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/store"
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
	LatestActivation             *runtimecontrol.ActivationResult
	SpoolState                   *string
	SpoolPendingCount            *int
	OldestPendingAgeSeconds      *int
	RequestUsageIngestLagSeconds *int
	SemanticOrphanCount          *int
}

func New(store *store.Store, control *runtimecontrol.Service, buildVersion string) *Service {
	return &Service{Store: store, RuntimeControl: control, BuildVersion: buildVersion, Now: time.Now}
}

// Health is a cheap local dependency probe. Full integrity/history/Relay
// diagnostics belong to authenticated Status and the maintenance check command.
func (s *Service) Health(ctx context.Context) error { return s.Store.DB.PingContext(ctx) }

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
			result.SpoolPendingCount = status.SpoolPendingCount
			result.OldestPendingAgeSeconds = status.OldestPendingAgeSeconds
			if status.SpoolState != nil {
				value := string(*status.SpoolState)
				result.SpoolState = &value
			}
			appliedRevision := status.AppliedControlRevision
			result.AppliedControlRevision = &appliedRevision
			if status.BundleHash != "" {
				hash := status.BundleHash
				result.AppliedBundleHash = &hash
			}
		}
	}
	if s.RuntimeControl != nil {
		result.LatestActivation, err = s.RuntimeControl.LatestActivation(ctx)
		if err != nil {
			return result, err
		}
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
	return client.SemanticUsage.Query().Where(semanticusage.RequestIDNotNil(), func(sel *sql.Selector) {
		request := sql.Table(requestusage.Table)
		sel.Where(sql.Not(sql.Exists(sql.Select(request.C(requestusage.FieldID)).From(request).Where(sql.ColumnsEQ(request.C(requestusage.FieldRequestID), sel.C(semanticusage.FieldRequestID))))))
	}).Count(ctx)
}
