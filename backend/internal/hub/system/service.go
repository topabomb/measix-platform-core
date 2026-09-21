package system

import (
	"context"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
	"measix/platform/internal/common/observability"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/store"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/relaycontrolapi"
)

type Service struct {
	Store              *store.Store
	RuntimeControl     *runtimecontrol.Service
	BuildVersion       string
	PortalMode         string
	PortalURL          *string
	PortalUpstream     *string
	Now                func() time.Time
	Telemetry          *observability.Recorder
	DiagnosticsLogDir  string
	mu                 sync.RWMutex
	lastRelayTelemetry *relaycontrolapi.ProcessTelemetry
}

type Status struct {
	BuildVersion                 string
	DBHealth                     string
	SchemaIdentity               string
	RuntimeStatus                string
	ActiveManagedGeneration      int
	ManagedStateRevision         int
	DesiredControlRevision       int
	DesiredBundleHash            *string
	RelayReady                   bool
	RelayBuildVersion            *string
	AppliedControlRevision       *int
	AppliedBundleHash            *string
	LastRelaySeenAt              *time.Time
	CurrentActivation            *runtimecontrol.ActivationResult
	LastActivation               *runtimecontrol.ActivationResult
	SpoolState                   *string
	SpoolPendingCount            *int
	OldestPendingAgeSeconds      *int
	RequestUsageIngestLagSeconds *int
	SemanticOrphanCount          *int
	SemanticUnknownRequestCount  *int
	PortalMode                   string
	PortalURL                    *string
	PortalUpstream               *string
}

func New(store *store.Store, control *runtimecontrol.Service, buildVersion string) *Service {
	return &Service{Store: store, RuntimeControl: control, BuildVersion: buildVersion, PortalMode: "UNAVAILABLE", Now: time.Now}
}

// Health is a cheap local dependency probe. Full integrity/history/Relay
// diagnostics belong to authenticated Status and the maintenance check command.
func (s *Service) Health(ctx context.Context) error { return s.Store.DB.PingContext(ctx) }

func (s *Service) Status(ctx context.Context) (Status, error) {
	result := Status{BuildVersion: s.BuildVersion, SchemaIdentity: maintenance.CurrentSchemaIdentity,
		PortalMode: s.PortalMode, PortalURL: s.PortalURL, PortalUpstream: s.PortalUpstream}
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
			result.RelayBuildVersion = &status.BuildVersion
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
			s.mu.Lock()
			s.lastRelayTelemetry = status.Telemetry
			s.mu.Unlock()
		}
	}
	if s.RuntimeControl != nil {
		result.CurrentActivation, result.LastActivation, err = s.RuntimeControl.ActivationStatus(ctx)
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
	unknown, err := usage.NewService(s.Store.Client).UnknownRequestCount(ctx)
	if err != nil {
		return result, err
	}
	result.SemanticUnknownRequestCount = &unknown
	return result, nil
}

type Telemetry struct {
	CollectedAt time.Time
	Hub         observability.Snapshot
	Relay       *relaycontrolapi.ProcessTelemetry
}

func (s *Service) RecentTelemetry(window time.Duration) Telemetry {
	if window != 15*time.Minute && window != 60*time.Minute {
		window = 15 * time.Minute
	}
	result := Telemetry{CollectedAt: s.Now().UTC()}
	if s.Telemetry != nil {
		result.Hub = s.Telemetry.Snapshot(window)
	}
	s.mu.RLock()
	if s.lastRelayTelemetry != nil {
		copy := *s.lastRelayTelemetry
		cutoff := result.CollectedAt.Truncate(time.Minute).Add(-window + time.Minute)
		copy.Buckets = append([]relaycontrolapi.TelemetryBucket(nil), copy.Buckets...)
		filtered := copy.Buckets[:0]
		for _, bucket := range copy.Buckets {
			if !bucket.Minute.Before(cutoff) {
				filtered = append(filtered, bucket)
			}
		}
		copy.Buckets = filtered
		copy.Summary = summarizeRelayTelemetry(filtered, copy.Summary.InFlight)
		result.Relay = &copy
	}
	s.mu.RUnlock()
	return result
}

func summarizeRelayTelemetry(buckets []relaycontrolapi.TelemetryBucket, inFlight int64) relaycontrolapi.TelemetryMetrics {
	result := relaycontrolapi.TelemetryMetrics{InFlight: inFlight}
	var upstream, denied int64
	for _, bucket := range buckets {
		result.RequestCount += bucket.RequestCount
		result.SuccessCount += bucket.SuccessCount
		result.ClientErrorCount += bucket.ClientErrorCount
		result.ServerErrorCount += bucket.ServerErrorCount
		result.RejectedCount += bucket.RejectedCount
		result.TimeoutCount += bucket.TimeoutCount
		result.CancelledCount += bucket.CancelledCount
		if bucket.DurationP95Ms > result.DurationP95Ms {
			result.DurationP95Ms = bucket.DurationP95Ms
		}
		if bucket.UpstreamErrorCount != nil {
			upstream += *bucket.UpstreamErrorCount
		}
		if bucket.BudgetDeniedCount != nil {
			denied += *bucket.BudgetDeniedCount
		}
	}
	result.UpstreamErrorCount, result.BudgetDeniedCount = &upstream, &denied
	return result
}

func (s *Service) RecentEvents(filter observability.EventFilter) (observability.EventPage, error) {
	return observability.ReadEvents(s.DiagnosticsLogDir, filter)
}

func semanticOrphanCount(ctx context.Context, client *ent.Client) (int, error) {
	return client.SemanticUsage.Query().Where(func(sel *sql.Selector) {
		request := sql.Table(requestusage.Table)
		sel.Where(sql.Not(sql.Exists(sql.Select(request.C(requestusage.FieldID)).From(request).Where(sql.ColumnsEQ(request.C(requestusage.FieldRequestID), sel.C(semanticusage.FieldRequestID))))))
	}).Count(ctx)
}
