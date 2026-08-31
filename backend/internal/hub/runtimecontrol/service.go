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
	"measix/platform/ent/managedrelease"
	"measix/platform/ent/session"
	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

var (
	ErrIdempotencyConflict  = errors.New("idempotency key reused with different request")
	ErrActivationInProgress = errors.New("another runtime activation is in progress")
	ErrRelayAckMismatch     = errors.New("relay acknowledgement does not match desired state")
)

const defaultMaxRequestBytes = 10 << 20

type PublishRequest struct {
	AdminUserID           string
	IdempotencyKey        string
	ExpectedDraftRevision int
	AcknowledgedWarnings  []string
}

type ActivationResult struct {
	ActivationID            string
	Kind                    string
	State                   string
	ReleaseID               string
	TargetManagedGeneration int
	DesiredControlRevision  int
	BundleHash              string
	CreatedAt               time.Time
	CompletedAt             *time.Time
	ErrorCode               *string
}

type Service struct {
	Client     *ent.Client
	Capability *capability.Service
	Upstream   *upstream.Service
	Signer     *security.AccessSigner
	Relay      RelayClient
	Now        func() time.Time

	// testHooks are test-only deterministic barrier/failure hooks that
	// simulate Hub crashes at precise points in the Publish pipeline.
	// They are nil in production and only set by test code.
	testHooks PublishBarrierHooks
}

func NewService(client *ent.Client, capabilityService *capability.Service, upstreamService *upstream.Service, signer *security.AccessSigner, relay RelayClient) *Service {
	return &Service{Client: client, Capability: capabilityService, Upstream: upstreamService, Signer: signer, Relay: relay, Now: time.Now}
}

// PublishBarrierHooks provides test-only deterministic hooks at the five
// critical points of the Publish pipeline (crash points A through E):
//
//	A: BeforeIntentCommit  — before persistPublishIntent commits.
//	B: AfterIntentCommit   — after intent is durable, before Relay.Apply.
//	C: AfterRelayApplied   — after Relay.Apply returns success (ACK received).
//	D: AfterAck            — after ACK is validated, before finalizePublish.
//	E: AfterFinalize       — after finalizePublish commits, before response.
//
// If a hook returns a non-nil error, Publish simulates a crash at that point
// by returning the error immediately (without proceeding to the next step).
// The Hub DB state is left exactly as it was at that point in the pipeline,
// allowing reconciliation tests to verify recovery.
//
// These hooks are never set in production code.
type PublishBarrierHooks struct {
	BeforeIntentCommit func(ctx context.Context) error // Crash point A
	AfterIntentCommit  func(ctx context.Context) error // Crash point B
	AfterRelayApplied  func(ctx context.Context) error // Crash point C (rarely needed; Relay controls this)
	AfterAck           func(ctx context.Context) error // Crash point D
	AfterFinalize      func(ctx context.Context) error // Crash point E
}

// SetTestBarrierHooks sets test-only barrier hooks. This function must
// never be called from production code.
func (s *Service) SetTestBarrierHooks(hooks PublishBarrierHooks) {
	s.testHooks = hooks
}

func IsIdempotencyConflict(err error) bool { return errors.Is(err, ErrIdempotencyConflict) }

func DescriptorPolicy() string {
	return "descriptor includes identity, routes, immutable secret references and non-secret auth shape; resolved credential values are excluded"
}

func (s *Service) Publish(ctx context.Context, request PublishRequest) (ActivationResult, error) {
	if s.Client == nil || s.Capability == nil || s.Upstream == nil || s.Signer == nil || s.Relay == nil {
		return ActivationResult{}, fmt.Errorf("runtime control service is not configured")
	}
	if platformid.Validate(platformid.User, request.AdminUserID) != nil || platformid.Validate(platformid.Idempotency, request.IdempotencyKey) != nil || request.ExpectedDraftRevision < 1 {
		return ActivationResult{}, fmt.Errorf("invalid publish request")
	}
	requestHash, err := publishRequestHash(request)
	if err != nil {
		return ActivationResult{}, err
	}
	if existing, found, err := s.findIdempotent(ctx, request.AdminUserID, request.IdempotencyKey, requestHash); err != nil {
		return ActivationResult{}, err
	} else if found {
		return existing, nil
	}

	validation, err := s.Capability.ValidateDraft(ctx, request.ExpectedDraftRevision)
	if err != nil {
		return ActivationResult{}, err
	}
	if !validation.Valid {
		return ActivationResult{}, capability.ErrInvalidDraft
	}
	if !warningsAcknowledged(validation.Warnings, request.AcknowledgedWarnings) {
		return ActivationResult{}, capability.ErrInvalidDraft
	}
	draft, err := s.Capability.GetDraft(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	if draft.DraftRevision != request.ExpectedDraftRevision {
		return ActivationResult{}, capability.ErrRevisionConflict
	}

	managed, err := s.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		return ActivationResult{}, err
	}
	previousDesiredRevision := managed.DesiredControlRevision
	previousRuntimeStatus := managed.RuntimeStatus
	var previousDesiredHash *string
	if managed.DesiredBundleHash != nil {
		value := *managed.DesiredBundleHash
		previousDesiredHash = &value
	}
	generation, err := s.nextGeneration(ctx)
	if err != nil {
		return ActivationResult{}, err
	}
	releaseID := platformid.New(platformid.Release)
	activationID := platformid.New(platformid.Activation)
	now := s.Now().UTC()
	snapshot, snapshotHash, err := s.Capability.CompileSnapshot(capability.SnapshotInput{
		DeploymentID: s.Signer.DeploymentID, ReleaseID: releaseID, ManagedGeneration: generation,
		Content: draft.Content, PublishedAt: now, PublishedByUserID: request.AdminUserID,
	})
	if err != nil {
		return ActivationResult{}, err
	}
	controlRevision := int(managed.DesiredControlRevision) + 1
	state, err := s.compileState(ctx, draft.Content, generation, controlRevision)
	if err != nil {
		return ActivationResult{}, err
	}
	descriptorJSON, err := relaystate.DescriptorJSON(state)
	if err != nil {
		return ActivationResult{}, err
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		return ActivationResult{}, err
	}
	state.BundleHash = hash

	releaseContentJSON, err := json.Marshal(draft.Content)
	if err != nil {
		return ActivationResult{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return ActivationResult{}, err
	}

	// Crash point A: before intent durable commit.
	if s.testHooks.BeforeIntentCommit != nil {
		if err := s.testHooks.BeforeIntentCommit(ctx); err != nil {
			return ActivationResult{}, err
		}
	}

	if err := s.persistPublishIntent(ctx, publishIntent{
		Request: request, RequestHash: requestHash, ReleaseID: releaseID, ActivationID: activationID,
		Generation: generation, ControlRevision: controlRevision, BundleHash: string(hash), DescriptorJSON: descriptorJSON,
		ReleaseContentJSON: releaseContentJSON, SnapshotJSON: snapshotJSON, SnapshotHash: snapshotHash, Now: now,
	}); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) || errors.Is(err, ErrActivationInProgress) {
			return ActivationResult{}, err
		}
		if existing, found, lookupErr := s.findIdempotent(ctx, request.AdminUserID, request.IdempotencyKey, requestHash); lookupErr == nil && found {
			return existing, nil
		}
		return ActivationResult{}, err
	}

	// Crash point B: intent is durable, before Relay.Apply.
	if s.testHooks.AfterIntentCommit != nil {
		if err := s.testHooks.AfterIntentCommit(ctx); err != nil {
			// Intent is already committed; return the activation as UNKNOWN.
			_ = s.markUnknown(ctx, activationID, "crash_after_intent_commit")
			return s.loadActivation(ctx, activationID)
		}
	}

	ack, err := s.Relay.Apply(ctx, state)
	if err != nil {
		if code, rejected := relayValidationRejected(err); rejected {
			if rejectErr := s.rejectPublish(ctx, activationID, releaseID, code, previousDesiredRevision, previousDesiredHash, previousRuntimeStatus); rejectErr != nil {
				return ActivationResult{}, rejectErr
			}
			return s.loadActivation(ctx, activationID)
		}
		_ = s.markUnknown(ctx, activationID, "relay_apply_unknown")
		return s.loadActivation(ctx, activationID)
	}
	// Crash point C: Relay applied successfully (ACK received),
	// but Hub may lose the ACK before processing it.
	if s.testHooks.AfterRelayApplied != nil {
		if err := s.testHooks.AfterRelayApplied(ctx); err != nil {
			_ = s.markUnknown(ctx, activationID, "crash_after_relay_applied")
			return s.loadActivation(ctx, activationID)
		}
	}

	if ack.AppliedControlRevision != controlRevision || string(ack.BundleHash) != string(hash) || ack.ActiveManagedGeneration != generation {
		_ = s.markFailed(ctx, activationID, "relay_ack_mismatch")
		return ActivationResult{}, ErrRelayAckMismatch
	}
	// Crash point D: ACK validated, before finalizePublish.
	if s.testHooks.AfterAck != nil {
		if err := s.testHooks.AfterAck(ctx); err != nil {
			_ = s.markUnknown(ctx, activationID, "crash_after_ack")
			return s.loadActivation(ctx, activationID)
		}
	}

	if err := s.finalizePublish(ctx, activationID, releaseID, generation, controlRevision, string(hash)); err != nil {
		return ActivationResult{}, err
	}

	// Crash point E: finalize committed, before response returned to Admin.
	if s.testHooks.AfterFinalize != nil {
		if err := s.testHooks.AfterFinalize(ctx); err != nil {
			// Everything is persisted; return the activation as COMPLETED
			// so the caller sees the finalized state even though the
			// "response was lost". A retry with the same idempotency key
			// must return the same result.
			return s.loadActivation(ctx, activationID)
		}
	}

	return s.loadActivation(ctx, activationID)
}

type publishIntent struct {
	Request            PublishRequest
	RequestHash        string
	ReleaseID          string
	ActivationID       string
	Generation         int
	ControlRevision    int
	BundleHash         string
	DescriptorJSON     []byte
	ReleaseContentJSON []byte
	SnapshotJSON       []byte
	SnapshotHash       string
	Now                time.Time
}

func (s *Service) persistPublishIntent(ctx context.Context, intent publishIntent) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}

	existing, err := tx.IdempotencyRecord.Query().Where(
		idempotencyrecord.AdminUserIDEQ(intent.Request.AdminUserID),
		idempotencyrecord.MethodEQ(httpMethodPublish),
		idempotencyrecord.NormalizedPathEQ(publishPath),
		idempotencyrecord.IdempotencyKeyEQ(intent.Request.IdempotencyKey),
	).Only(ctx)
	if err == nil {
		if existing.RequestHash != intent.RequestHash {
			return rollback(ErrIdempotencyConflict)
		}
		return rollback(fmt.Errorf("idempotency record already exists"))
	}
	if !ent.IsNotFound(err) {
		return rollback(err)
	}

	pending, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Count(ctx)
	if err != nil {
		return rollback(err)
	}
	if pending != 0 {
		return rollback(ErrActivationInProgress)
	}
	draftRow, err := tx.ManagedDraft.Query().Only(ctx)
	if err != nil {
		return rollback(err)
	}
	if draftRow.DraftRevision != int64(intent.Request.ExpectedDraftRevision) {
		return rollback(capability.ErrRevisionConflict)
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		return rollback(err)
	}
	if managed.DesiredControlRevision+1 != int64(intent.ControlRevision) {
		return rollback(ErrActivationInProgress)
	}

	if _, err := tx.ManagedRelease.Create().
		SetID(intent.ReleaseID).
		SetManagedGeneration(int64(intent.Generation)).
		SetStatus("STAGED").
		SetReleaseContentJSON(intent.ReleaseContentJSON).
		SetSnapshotSchemaVersion(1).
		SetSnapshotJSON(intent.SnapshotJSON).
		SetSnapshotHash(intent.SnapshotHash).
		SetSourceDraftRevision(int64(intent.Request.ExpectedDraftRevision)).
		SetCreatedByUserID(intent.Request.AdminUserID).
		SetCreatedAt(intent.Now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Activation.Create().
		SetID(intent.ActivationID).
		SetKind("PUBLISH").
		SetState("APPLYING").
		SetIdempotencyKey(intent.Request.IdempotencyKey).
		SetRequestHash(intent.RequestHash).
		SetControlRevision(int64(intent.ControlRevision)).
		SetBundleHash(intent.BundleHash).
		SetTargetGeneration(int64(intent.Generation)).
		SetTargetDescriptorJSON(intent.DescriptorJSON).
		SetSubjectID(intent.ReleaseID).
		SetCreatedByUserID(intent.Request.AdminUserID).
		SetCreatedAt(intent.Now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.IdempotencyRecord.Create().
		SetAdminUserID(intent.Request.AdminUserID).
		SetMethod(httpMethodPublish).
		SetNormalizedPath(publishPath).
		SetIdempotencyKey(intent.Request.IdempotencyKey).
		SetRequestHash(intent.RequestHash).
		SetActivationID(intent.ActivationID).
		SetCreatedAt(intent.Now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.ManagedState.UpdateOneID("current").
		SetDesiredControlRevision(int64(intent.ControlRevision)).
		SetDesiredBundleHash(intent.BundleHash).
		SetRuntimeStatus("ACTIVATING").
		SetManagedStateRevision(managed.ManagedStateRevision + 1).
		SetUpdatedAt(intent.Now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

const (
	httpMethodPublish = "POST"
	publishPath       = "/api/admin/v1/draft:publish"
)

func (s *Service) finalizePublish(ctx context.Context, activationID, releaseID string, generation, controlRevision int, bundleHash string) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	row, err := tx.Activation.Get(ctx, activationID)
	if err != nil {
		return rollback(err)
	}
	if row.State == "COMPLETED" {
		return tx.Commit()
	}
	if row.ControlRevision != int64(controlRevision) || row.BundleHash != bundleHash || row.TargetGeneration == nil || *row.TargetGeneration != int64(generation) {
		return rollback(ErrRelayAckMismatch)
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		return rollback(err)
	}
	if managed.DesiredControlRevision != int64(controlRevision) || managed.DesiredBundleHash == nil || *managed.DesiredBundleHash != bundleHash {
		return rollback(ErrRelayAckMismatch)
	}
	if _, err := tx.ManagedRelease.Update().Where(managedrelease.StatusEQ("ACTIVE")).SetStatus("SUPERSEDED").Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.ManagedRelease.UpdateOneID(releaseID).SetStatus("ACTIVE").Save(ctx); err != nil {
		return rollback(err)
	}
	now := s.Now().UTC()
	if _, err := tx.ManagedState.UpdateOneID("current").
		SetActiveReleaseID(releaseID).
		SetActiveManagedGeneration(int64(generation)).
		SetRuntimeStatus("READY").
		SetManagedStateRevision(managed.ManagedStateRevision + 1).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("COMPLETED").SetCompletedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func (s *Service) rejectPublish(ctx context.Context, activationID, releaseID, code string, previousRevision int64, previousHash *string, previousRuntimeStatus string) error {
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("FAILED").SetErrorCode(code).SetCompletedAt(now).Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.ManagedRelease.UpdateOneID(releaseID).SetStatus("ACTIVATION_FAILED").Save(ctx); err != nil {
		return rollback(err)
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		return rollback(err)
	}
	update := tx.ManagedState.UpdateOneID("current").
		SetDesiredControlRevision(previousRevision).
		SetRuntimeStatus(previousRuntimeStatus).
		SetManagedStateRevision(managed.ManagedStateRevision + 1).
		SetUpdatedAt(now)
	if previousHash == nil {
		update.ClearDesiredBundleHash()
	} else {
		update.SetDesiredBundleHash(*previousHash)
	}
	if _, err := update.Save(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func (s *Service) markUnknown(ctx context.Context, activationID, code string) error {
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("UNKNOWN").SetErrorCode(code).Save(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").SetRuntimeStatus("DEGRADED").SetManagedStateRevision(managed.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Service) markFailed(ctx context.Context, activationID, code string) error {
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Activation.UpdateOneID(activationID).SetState("FAILED").SetErrorCode(code).SetCompletedAt(now).Save(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	managed, err := tx.ManagedState.Get(ctx, "current")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ManagedState.UpdateOneID("current").SetRuntimeStatus("DEGRADED").SetManagedStateRevision(managed.ManagedStateRevision + 1).SetUpdatedAt(now).Save(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Service) findIdempotent(ctx context.Context, adminUserID, key, requestHash string) (ActivationResult, bool, error) {
	record, err := s.Client.IdempotencyRecord.Query().Where(
		idempotencyrecord.AdminUserIDEQ(adminUserID),
		idempotencyrecord.MethodEQ(httpMethodPublish),
		idempotencyrecord.NormalizedPathEQ(publishPath),
		idempotencyrecord.IdempotencyKeyEQ(key),
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

func (s *Service) loadActivation(ctx context.Context, activationID string) (ActivationResult, error) {
	row, err := s.Client.Activation.Get(ctx, activationID)
	if err != nil {
		return ActivationResult{}, err
	}
	return activationView(row), nil
}

func activationView(row *ent.Activation) ActivationResult {
	result := ActivationResult{
		ActivationID: row.ID, Kind: row.Kind, State: row.State, DesiredControlRevision: int(row.ControlRevision),
		BundleHash: row.BundleHash, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt, ErrorCode: row.ErrorCode,
	}
	if row.Kind == "PUBLISH" && row.SubjectID != nil {
		result.ReleaseID = *row.SubjectID
	}
	if row.TargetGeneration != nil {
		result.TargetManagedGeneration = int(*row.TargetGeneration)
	}
	return result
}

func (s *Service) nextGeneration(ctx context.Context) (int, error) {
	latest, err := s.Client.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).First(ctx)
	if ent.IsNotFound(err) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return int(latest.ManagedGeneration) + 1, nil
}

func publishRequestHash(request PublishRequest) (string, error) {
	warnings := append([]string(nil), request.AcknowledgedWarnings...)
	sort.Strings(warnings)
	payload, err := json.Marshal(struct {
		ExpectedDraftRevision int      `json:"expectedDraftRevision"`
		AcknowledgedWarnings  []string `json:"acknowledgedWarnings"`
	}{request.ExpectedDraftRevision, warnings})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func warningsAcknowledged(warnings []adminapi.ValidationIssue, acknowledged []string) bool {
	if len(warnings) == 0 {
		return true
	}
	seen := make(map[string]struct{}, len(acknowledged))
	for _, code := range acknowledged {
		seen[code] = struct{}{}
	}
	for _, warning := range warnings {
		if _, ok := seen[warning.Code]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) compileState(ctx context.Context, content adminapi.ManagedDraftContent, generation, revision int) (relaycontrolapi.RuntimeControlState, error) {
	jwk := s.Signer.PublicJWK()
	key := relaycontrolapi.PublicJwk{
		Kty: relaycontrolapi.OKP, Crv: relaycontrolapi.Ed25519, Alg: relaycontrolapi.EdDSA, Use: relaycontrolapi.Sig,
		Kid: stringValue(jwk["kid"]), X: stringValue(jwk["x"]),
	}
	if key.Kid == "" || key.X == "" {
		return relaycontrolapi.RuntimeControlState{}, fmt.Errorf("invalid signing key")
	}

	disabledUsers, err := s.Client.User.Query().Where(user.StatusEQ("DISABLED")).All(ctx)
	if err != nil {
		return relaycontrolapi.RuntimeControlState{}, err
	}
	revokedDevices, err := s.Client.Device.Query().Where(device.StatusEQ("REVOKED")).All(ctx)
	if err != nil {
		return relaycontrolapi.RuntimeControlState{}, err
	}
	revokedSessions, err := s.Client.Session.Query().Where(session.StatusEQ("REVOKED")).All(ctx)
	if err != nil {
		return relaycontrolapi.RuntimeControlState{}, err
	}
	principal := relaycontrolapi.PrincipalState{
		DisabledUserIds: make([]string, 0, len(disabledUsers)), RevokedDeviceIds: make([]string, 0, len(revokedDevices)), RevokedSessionIds: make([]string, 0, len(revokedSessions)),
	}
	for _, value := range disabledUsers {
		principal.DisabledUserIds = append(principal.DisabledUserIds, value.ID)
	}
	for _, value := range revokedDevices {
		principal.RevokedDeviceIds = append(principal.RevokedDeviceIds, value.ID)
	}
	for _, value := range revokedSessions {
		principal.RevokedSessionIds = append(principal.RevokedSessionIds, value.ID)
	}

	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: revision, ActiveManagedGeneration: generation, DeploymentId: s.Signer.DeploymentID,
		AuthKeys: []relaycontrolapi.PublicJwk{key}, PrincipalState: principal,
		ResourceRoutes: []relaycontrolapi.ResourceRoute{}, Routes: []relaycontrolapi.RuntimeRouteSpec{}, Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: defaultMaxRequestBytes},
	}

	upstreamSpecs := map[string]relaycontrolapi.RuntimeUpstreamSpec{}
	for _, binding := range content.Bindings {
		row, err := s.Client.Upstream.Get(ctx, binding.UpstreamId)
		if err != nil {
			return relaycontrolapi.RuntimeControlState{}, err
		}
		if row.ActiveConfigRevision == nil || row.Status != "ACTIVE" {
			return relaycontrolapi.RuntimeControlState{}, fmt.Errorf("upstream %s is not active", row.ID)
		}
		config, err := loadUpstreamRevision(ctx, s.Client, row.ID, int(*row.ActiveConfigRevision))
		if err != nil {
			return relaycontrolapi.RuntimeControlState{}, err
		}
		if _, exists := upstreamSpecs[row.ID]; !exists {
			spec, err := s.compileUpstream(ctx, row.ID, config)
			if err != nil {
				return relaycontrolapi.RuntimeControlState{}, err
			}
			upstreamSpecs[row.ID] = spec
		}
		timeout := config.TimeoutDefaults
		if binding.TimeoutPolicy != nil {
			timeout = *binding.TimeoutPolicy
		}
		state.ResourceRoutes = append(state.ResourceRoutes, relaycontrolapi.ResourceRoute{ResourceId: binding.ResourceId, RuntimeRouteId: binding.RuntimeRouteId})
		state.Routes = append(state.Routes, relaycontrolapi.RuntimeRouteSpec{
			RuntimeRouteId: binding.RuntimeRouteId, UpstreamId: binding.UpstreamId,
			AllowedMethods: append([]string(nil), binding.AllowedMethods...), AllowedPathPrefixes: append([]string(nil), binding.AllowedPathPrefixes...),
			TransportPolicy: relaycontrolapi.RuntimeRouteSpecTransportPolicy(binding.TransportPolicy),
			TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: timeout.ConnectMs, ResponseHeaderMs: timeout.ResponseHeaderMs, IdleMs: timeout.IdleMs},
		})
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

func (s *Service) compileUpstream(ctx context.Context, upstreamID string, config adminapi.UpstreamConfig) (relaycontrolapi.RuntimeUpstreamSpec, error) {
	spec := relaycontrolapi.RuntimeUpstreamSpec{
		UpstreamId: upstreamID, BaseUrl: config.BaseUrl, Enabled: true,
		TransportCapabilities: stringSlice(config.TransportCapabilities),
	}
	refs, err := upstream.SecretRefs(config.Auth)
	if err != nil {
		return relaycontrolapi.RuntimeUpstreamSpec{}, err
	}
	var ref *upstream.SecretRef
	if len(refs) != 0 {
		ref = &refs[0]
		spec.SecretRef = &relaycontrolapi.SecretRef{SecretId: ref.SecretID, SecretVersion: ref.Version}
	}
	switch config.Auth.Type {
	case adminapi.UpstreamAuthTypeNONE:
		spec.Auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}}
	case adminapi.UpstreamAuthTypeBEARER:
		secret, err := s.Upstream.ResolveSecret(ctx, ref.SecretID, ref.Version)
		if err != nil {
			return relaycontrolapi.RuntimeUpstreamSpec{}, err
		}
		spec.Auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BEARER, AdditionalProperties: map[string]interface{}{"token": string(secret)}}
	case adminapi.UpstreamAuthTypeSTATICHEADER:
		secret, err := s.Upstream.ResolveSecret(ctx, ref.SecretID, ref.Version)
		if err != nil {
			return relaycontrolapi.RuntimeUpstreamSpec{}, err
		}
		header := ""
		if config.Auth.HeaderName != nil {
			header = *config.Auth.HeaderName
		}
		spec.Auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.STATICHEADER, AdditionalProperties: map[string]interface{}{"headerName": header, "value": string(secret)}}
	case adminapi.UpstreamAuthTypeBASIC:
		secret, err := s.Upstream.ResolveSecret(ctx, ref.SecretID, ref.Version)
		if err != nil {
			return relaycontrolapi.RuntimeUpstreamSpec{}, err
		}
		username := ""
		if config.Auth.Username != nil {
			username = *config.Auth.Username
		}
		spec.Auth = relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.BASIC, AdditionalProperties: map[string]interface{}{"username": username, "password": string(secret)}}
	default:
		return relaycontrolapi.RuntimeUpstreamSpec{}, upstream.ErrInvalidConfig
	}
	return spec, nil
}

func stringSlice(caps []adminapi.UpstreamConfigTransportCapabilities) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}

func loadUpstreamRevision(ctx context.Context, client *ent.Client, upstreamID string, revision int) (adminapi.UpstreamConfig, error) {
	row, err := client.UpstreamConfigRevision.Query().Where(
		upstreamconfigrevision.UpstreamIDEQ(upstreamID),
		upstreamconfigrevision.RevisionEQ(int64(revision)),
	).Only(ctx)
	if err != nil {
		return adminapi.UpstreamConfig{}, err
	}
	var config adminapi.UpstreamConfig
	if err := json.Unmarshal(row.ConfigJSON, &config); err != nil {
		return adminapi.UpstreamConfig{}, err
	}
	return config, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
