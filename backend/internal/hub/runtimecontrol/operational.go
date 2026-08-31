package runtimecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/device"
	"measix/platform/ent/idempotencyrecord"
	"measix/platform/ent/session"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

func (s *Service) GetActivation(ctx context.Context, activationID string) (ActivationResult, error) {
	return s.loadActivation(ctx, activationID)
}

func (s *Service) ApplyUpstream(ctx context.Context, adminUserID, idempotencyKey, upstreamID string) (ActivationResult, error) {
	if s.Client == nil || s.Upstream == nil || s.Signer == nil || s.Relay == nil || platformid.Validate(platformid.User, adminUserID) != nil || platformid.Validate(platformid.Idempotency, idempotencyKey) != nil || platformid.Validate(platformid.Upstream, upstreamID) != nil {
		return ActivationResult{}, fmt.Errorf("invalid upstream apply request")
	}
	row, err := s.Client.Upstream.Get(ctx, upstreamID)
	if err != nil {
		return ActivationResult{}, err
	}
	targetRevision := int(row.ConfigRevision)
	requestHash := hashOperation(struct {
		UpstreamID string `json:"upstreamId"`
		Revision   int    `json:"revision"`
	}{upstreamID, targetRevision})
	path := "/api/admin/v1/upstreams/" + upstreamID + ":apply"
	if existing, found, err := s.findOperationIdempotent(ctx, adminUserID, path, idempotencyKey, requestHash); err != nil || found {
		return existing, err
	}

	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return ActivationResult{}, err
	}
	controlRevision := int(managed.DesiredControlRevision) + 1
	generation := int(managed.ActiveManagedGeneration)
	content, err := s.activeReleaseContent(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	state, err := s.compileOperationalState(ctx, content, generation, controlRevision, map[string]int{upstreamID: targetRevision})
	if err != nil {
		return ActivationResult{}, err
	}
	descriptor, err := relaystate.DescriptorJSON(state)
	if err != nil {
		return ActivationResult{}, err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		return ActivationResult{}, err
	}
	state.BundleHash = hash
	pendingOperation, err := json.Marshal(struct {
		TargetRevision int `json:"targetRevision"`
	}{targetRevision})
	if err != nil {
		return ActivationResult{}, err
	}
	activationID := platformid.New(platformid.Activation)
	now := s.Now().UTC()

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	rollback := func(cause error) (ActivationResult, error) {
		_ = tx.Rollback()
		return ActivationResult{}, cause
	}
	pending, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Count(ctx)
	if err != nil {
		return rollback(err)
	}
	if pending != 0 {
		return rollback(ErrActivationInProgress)
	}
	freshUpstream, err := tx.Upstream.Get(ctx, upstreamID)
	if err != nil || int(freshUpstream.ConfigRevision) != targetRevision {
		return rollback(ErrActivationInProgress)
	}
	freshManaged, err := tx.ManagedState.Get(ctx, "current")
	if err != nil || freshManaged.DesiredControlRevision+1 != int64(controlRevision) {
		return rollback(ErrActivationInProgress)
	}
	if _, err := tx.Activation.Create().
		SetID(activationID).SetKind("RUNTIME_CONFIG").SetState("APPLYING").
		SetIdempotencyKey(idempotencyKey).SetRequestHash(requestHash).
		SetControlRevision(int64(controlRevision)).SetBundleHash(string(hash)).
		SetTargetDescriptorJSON(descriptor).SetSubjectID(upstreamID).SetPendingOperationJSON(pendingOperation).
		SetCreatedByUserID(adminUserID).SetCreatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.IdempotencyRecord.Create().
		SetAdminUserID(adminUserID).SetMethod("POST").SetNormalizedPath(path).
		SetIdempotencyKey(idempotencyKey).SetRequestHash(requestHash).SetActivationID(activationID).SetCreatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Upstream.UpdateOneID(upstreamID).SetStatus("APPLYING").SetUpdatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.ManagedState.UpdateOneID("current").
		SetDesiredControlRevision(int64(controlRevision)).SetDesiredBundleHash(string(hash)).SetRuntimeStatus("ACTIVATING").
		SetManagedStateRevision(freshManaged.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return ActivationResult{}, err
	}

	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		_ = s.markUnknown(ctx, activationID, "relay_apply_unknown")
		_, _ = s.Client.Upstream.UpdateOneID(upstreamID).SetStatus("DEGRADED").SetUpdatedAt(s.Now().UTC()).Save(ctx)
		return s.loadActivation(ctx, activationID)
	}
	if ack.AppliedControlRevision != controlRevision || string(ack.BundleHash) != string(hash) || ack.ActiveManagedGeneration != generation {
		_ = s.markFailed(ctx, activationID, "relay_ack_mismatch")
		_, _ = s.Client.Upstream.UpdateOneID(upstreamID).SetStatus("DEGRADED").SetUpdatedAt(s.Now().UTC()).Save(ctx)
		return ActivationResult{}, ErrRelayAckMismatch
	}
	if err := s.finalizeUpstreamApply(ctx, activationID, upstreamID, targetRevision, controlRevision, string(hash)); err != nil {
		return ActivationResult{}, err
	}
	return s.loadActivation(ctx, activationID)
}

func (s *Service) finalizeUpstreamApply(ctx context.Context, activationID, upstreamID string, targetRevision, controlRevision int, bundleHash string) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil || managed.DesiredControlRevision != int64(controlRevision) || managed.DesiredBundleHash == nil || *managed.DesiredBundleHash != bundleHash {
		return ErrRelayAckMismatch
	}
	now := s.Now().UTC()
	if _, err := tx.Upstream.UpdateOneID(upstreamID).SetActiveConfigRevision(int64(targetRevision)).SetStatus("ACTIVE").SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("COMPLETED").SetCompletedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SetManagedStateRevision(managed.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) findOperationIdempotent(ctx context.Context, adminUserID, path, key, requestHash string) (ActivationResult, bool, error) {
	record, err := s.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.AdminUserIDEQ(adminUserID), idempotencyrecord.MethodEQ("POST"),
		idempotencyrecord.NormalizedPathEQ(path), idempotencyrecord.IdempotencyKeyEQ(key),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return ActivationResult{}, false, nil
	}
	if err != nil {
		return ActivationResult{}, false, err
	}
	if record.RequestHash != requestHash {
		return ActivationResult{}, false, ErrIdempotencyConflict
	}
	if record.ActivationID == nil {
		return ActivationResult{}, false, fmt.Errorf("idempotency record has no activation")
	}
	result, err := s.loadActivation(ctx, *record.ActivationID)
	return result, err == nil, err
}

func (s *Service) activeReleaseContent(ctx context.Context) (adminapi.ManagedDraftContent, error) {
	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return adminapi.ManagedDraftContent{}, err
	}
	if managed.ActiveReleaseID == nil || managed.ActiveManagedGeneration == 0 {
		return adminapi.ManagedDraftContent{
			Providers: []adminapi.ProviderDefinition{}, Models: []adminapi.ModelDefinition{}, Tts: []adminapi.TtsDefinition{},
			Asr: []adminapi.AsrDefinition{}, Mcp: []adminapi.McpDefinition{}, Bindings: []adminapi.RuntimeBindingDefinition{},
			Policy: adminapi.ManagedPolicy{PolicyId: platformid.New(platformid.Policy)},
		}, nil
	}
	release, err := s.Client.ManagedRelease.Get(ctx, *managed.ActiveReleaseID)
	if err != nil {
		return adminapi.ManagedDraftContent{}, err
	}
	var content adminapi.ManagedDraftContent
	if err := json.Unmarshal(release.ReleaseContentJSON, &content); err != nil {
		return adminapi.ManagedDraftContent{}, err
	}
	return content, nil
}

func (s *Service) compileOperationalState(ctx context.Context, content adminapi.ManagedDraftContent, generation, revision int, upstreamOverrides map[string]int) (relaycontrolapi.RuntimeControlState, error) {
	jwk := s.Signer.PublicJWK()
	key := relaycontrolapi.PublicJwk{Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA, Use: relaycontrolapi.Sig, Kid: stringValue(jwk["kid"]), X: stringValue(jwk["x"])}
	if key.Kid == "" || key.X == "" {
		return relaycontrolapi.RuntimeControlState{}, fmt.Errorf("invalid signing key")
	}
	principal, err := s.principalState(ctx)
	if err != nil {
		return relaycontrolapi.RuntimeControlState{}, err
	}
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: revision, ActiveManagedGeneration: generation, DeploymentId: s.Signer.DeploymentID,
		AuthKeys: []relaycontrolapi.PublicJwk{key}, PrincipalState: principal,
		ResourceRoutes: []relaycontrolapi.ResourceRoute{}, Routes: []relaycontrolapi.RuntimeRouteSpec{}, Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: defaultMaxRequestBytes},
	}
	upstreamSpecs := map[string]relaycontrolapi.RuntimeUpstreamSpec{}
	ensureUpstream := func(id string) (adminapi.UpstreamConfig, error) {
		row, err := s.Client.Upstream.Get(ctx, id)
		if err != nil {
			return adminapi.UpstreamConfig{}, err
		}
		revision := 0
		if override, ok := upstreamOverrides[id]; ok {
			revision = override
		} else if row.ActiveConfigRevision != nil {
			revision = int(*row.ActiveConfigRevision)
		}
		if revision < 1 {
			return adminapi.UpstreamConfig{}, fmt.Errorf("upstream %s has no active config", id)
		}
		config, err := loadUpstreamRevision(ctx, s.Client, id, revision)
		if err != nil {
			return adminapi.UpstreamConfig{}, err
		}
		if _, exists := upstreamSpecs[id]; !exists {
			spec, err := s.compileUpstream(ctx, id, config)
			if err != nil {
				return adminapi.UpstreamConfig{}, err
			}
			upstreamSpecs[id] = spec
		}
		return config, nil
	}
	for _, binding := range content.Bindings {
		config, err := ensureUpstream(binding.UpstreamId)
		if err != nil {
			return relaycontrolapi.RuntimeControlState{}, err
		}
		timeout := config.TimeoutDefaults
		if binding.TimeoutPolicy != nil {
			timeout = *binding.TimeoutPolicy
		}
		state.ResourceRoutes = append(state.ResourceRoutes, relaycontrolapi.ResourceRoute{ResourceId: binding.ResourceId, RuntimeRouteId: binding.RuntimeRouteId})
		route := relaycontrolapi.RuntimeRouteSpec{
			RuntimeRouteId: binding.RuntimeRouteId, UpstreamId: binding.UpstreamId,
			AllowedMethods: append([]string(nil), binding.AllowedMethods...), AllowedPathPrefixes: append([]string(nil), binding.AllowedPathPrefixes...),
			TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy(binding.TransportPolicy),
			TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: timeout.ConnectMs, ResponseHeaderMs: timeout.ResponseHeaderMs, IdleMs: timeout.IdleMs},
		}
		route.TimeoutPolicy.OverallMs = timeout.OverallMs
		state.Routes = append(state.Routes, route)
	}
	for id := range upstreamOverrides {
		if _, err := ensureUpstream(id); err != nil {
			return relaycontrolapi.RuntimeControlState{}, err
		}
	}
	ids := make([]string, 0, len(upstreamSpecs))
	for id := range upstreamSpecs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state.Upstreams = append(state.Upstreams, upstreamSpecs[id])
	}
	return state, nil
}

func (s *Service) principalState(ctx context.Context) (relaycontrolapi.PrincipalState, error) {
	disabledUsers, err := s.Client.User.Query().Where(user.StatusEQ("DISABLED")).All(ctx)
	if err != nil {
		return relaycontrolapi.PrincipalState{}, err
	}
	revokedDevices, err := s.Client.Device.Query().Where(device.StatusEQ("REVOKED")).All(ctx)
	if err != nil {
		return relaycontrolapi.PrincipalState{}, err
	}
	revokedSessions, err := s.Client.Session.Query().Where(session.StatusEQ("REVOKED")).All(ctx)
	if err != nil {
		return relaycontrolapi.PrincipalState{}, err
	}
	principal := relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}}
	for _, value := range disabledUsers {
		principal.DisabledUserIds = append(principal.DisabledUserIds, value.ID)
	}
	for _, value := range revokedDevices {
		principal.RevokedDeviceIds = append(principal.RevokedDeviceIds, value.ID)
	}
	for _, value := range revokedSessions {
		principal.RevokedSessionIds = append(principal.RevokedSessionIds, value.ID)
	}
	sort.Strings(principal.DisabledUserIds)
	sort.Strings(principal.RevokedDeviceIds)
	sort.Strings(principal.RevokedSessionIds)
	return principal, nil
}

func hashOperation(value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var _ = upstream.ErrInvalidConfig
var _ = time.Now
var _ = errors.Is

func (s *Service) LatestActivation(ctx context.Context) (*ActivationResult, error) {
	row, err := s.Client.Activation.Query().Order(ent.Desc(activation.FieldCreatedAt), ent.Desc(activation.FieldID)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view := activationView(row)
	return &view, nil
}
