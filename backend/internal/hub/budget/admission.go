package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/budgetallocation"
	"measix/platform/ent/budgetbucket"
	"measix/platform/ent/budgetlimit"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/deletedprincipal"
	"measix/platform/ent/userbudget"
	"measix/platform/pkg/platformid"
)

type allocationCandidate struct {
	limit   *ent.BudgetLimit
	bucket  *ent.BudgetBucket
	reserve int64
	window  periodWindow
}

func (s *Service) Admit(ctx context.Context, input AdmitInput) (AdmissionDecision, error) {
	normalized, known, supported, requestHash, err := normalizeAdmit(input)
	if err != nil {
		return AdmissionDecision{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return AdmissionDecision{}, err
	}
	defer tx.Rollback()
	deleted, err := tx.DeletedPrincipal.Query().Where(deletedprincipal.IDEQ(normalized.UserID)).Exist(ctx)
	if err != nil {
		return AdmissionDecision{}, err
	}
	if deleted {
		return AdmissionDecision{}, ErrIdentityDeleted
	}

	existing, err := tx.BudgetRequest.Get(ctx, normalized.RequestID)
	if err == nil {
		if existing.RequestHash != requestHash {
			return AdmissionDecision{}, ErrRequestConflict
		}
		var decision AdmissionDecision
		if err := json.Unmarshal(existing.DecisionJSON, &decision); err != nil {
			return AdmissionDecision{}, err
		}
		return decision, nil
	}
	if !ent.IsNotFound(err) {
		return AdmissionDecision{}, err
	}

	mode, source, revision := ModeUnlimited, SourceDefault, int64(0)
	var budgetID *int
	var activatedAt *time.Time
	budgetRow, err := tx.UserBudget.Query().
		Where(userbudget.UserIDEQ(normalized.UserID), userbudget.CapabilityEQ(userbudget.Capability(normalized.Capability))).
		Only(ctx)
	if err == nil {
		mode, source, revision = Mode(budgetRow.Mode), Source(budgetRow.Source), budgetRow.Revision
		budgetID = &budgetRow.ID
		activated := budgetRow.ActivatedAt
		activatedAt = &activated
		if activated.After(normalized.AdmittedAt) {
			// The committed configuration head wins over a repeated/coarse
			// request clock tick; keep admission on the same logical timeline
			// used by immutable limit versions.
			normalized.AdmittedAt = activated
		}
	} else if !ent.IsNotFound(err) {
		return AdmissionDecision{}, err
	}

	var limits []*ent.BudgetLimit
	if budgetID != nil {
		limits, err = tx.BudgetLimit.Query().Where(
			budgetlimit.UserBudgetIDEQ(*budgetID),
			budgetlimit.EffectiveFromLTE(normalized.AdmittedAt),
			budgetlimit.Or(budgetlimit.EffectiveToIsNil(), budgetlimit.EffectiveToGT(normalized.AdmittedAt)),
		).All(ctx)
		if err != nil {
			return AdmissionDecision{}, err
		}
	}
	sort.Slice(limits, func(i, j int) bool {
		if limits[i].Period != limits[j].Period {
			return limits[i].Period < limits[j].Period
		}
		if limits[i].Meter != limits[j].Meter {
			return limits[i].Meter < limits[j].Meter
		}
		return limits[i].ScopeKey < limits[j].ScopeKey
	})

	decision := AdmissionDecision{
		RequestID: normalized.RequestID, Capability: normalized.Capability, Mode: mode, Source: source,
		Revision: revision, ActivatedAt: activatedAt, AsOf: normalized.AdmittedAt,
	}
	candidates := make([]allocationCandidate, 0, len(limits))
	unavailable := make(map[Meter]struct{})
	for _, limit := range limits {
		meter := Meter(limit.Meter)
		if mode == ModeLimited && !supported[meter] {
			unavailable[meter] = struct{}{}
		}
		window, err := windowFor(Period(limit.Period), normalized.AdmittedAt, limit.ScopeStartedAt, s.Location)
		if err != nil {
			return AdmissionDecision{}, err
		}
		bucket, err := findOrCreateBucket(ctx, tx, limit, window, normalized.AdmittedAt)
		if err != nil {
			return AdmissionDecision{}, err
		}
		reserve := int64(0)
		if mode == ModeLimited {
			reserve = known[meter]
			used := bucket.SettledQuantity
			reserved := bucket.ReservedQuantity
			quantity, predictable := known[meter]
			blocked := predictable && used+reserved+quantity > limit.LimitQuantity
			if !predictable {
				blocked = used+reserved >= limit.LimitQuantity
			}
			if blocked {
				decision.BlockingLimits = append(decision.BlockingLimits, BlockingLimit{
					ScopeKey: limit.ScopeKey, Meter: meter, Period: Period(limit.Period), Limit: limit.LimitQuantity,
					Used: used, Reserved: reserved, ScopeStart: window.Start, ResetAt: window.End,
				})
			}
		}
		candidates = append(candidates, allocationCandidate{limit: limit, bucket: bucket, reserve: reserve, window: window})
	}
	for meter := range unavailable {
		decision.Unavailable = append(decision.Unavailable, meter)
	}
	sort.Slice(decision.Unavailable, func(i, j int) bool { return decision.Unavailable[i] < decision.Unavailable[j] })

	inFlight, err := tx.BudgetRequest.Query().Where(
		budgetrequest.UserIDEQ(normalized.UserID),
		budgetrequest.CapabilityEQ(budgetrequest.Capability(normalized.Capability)),
		budgetrequest.StateIn(budgetrequest.State(RequestAdmitted), budgetrequest.State(RequestStarted), budgetrequest.State(RequestReconciliation)),
	).Count(ctx)
	if err != nil {
		return AdmissionDecision{}, err
	}
	decision.InFlightRequests = int64(inFlight)
	switch {
	case len(decision.Unavailable) > 0:
		decision.Code = DecisionMeterUnavailable
	case len(decision.BlockingLimits) > 0:
		decision.Code = DecisionBudgetExhausted
	case s.MaxInFlight > 0 && int64(inFlight) >= s.MaxInFlight:
		decision.Code = DecisionInFlightLimit
	default:
		decision.Allowed = true
		decision.Code = DecisionAllowed
		decision.InFlightRequests++
	}
	decision.ResetAt = overallResetAt(decision.BlockingLimits)
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		return AdmissionDecision{}, err
	}
	state := RequestDenied
	if decision.Allowed {
		state = RequestAdmitted
	}
	if _, err := tx.BudgetRequest.Create().
		SetID(normalized.RequestID).
		SetRequestHash(requestHash).
		SetDeploymentID(normalized.DeploymentID).
		SetUserID(normalized.UserID).
		SetNillableInteractionID(normalized.InteractionID).
		SetNillableDeviceID(normalized.DeviceID).
		SetCapability(budgetrequest.Capability(normalized.Capability)).
		SetResourceID(normalized.ResourceID).
		SetClientProtocol(string(normalized.ClientProtocol)).
		SetUpstreamID(normalized.UpstreamID).
		SetManagedGeneration(normalized.ManagedGeneration).
		SetControlRevision(normalized.ControlRevision).
		SetNillableUserBudgetID(budgetID).
		SetBudgetRevision(revision).
		SetMode(budgetrequest.Mode(mode)).
		SetSource(budgetrequest.Source(source)).
		SetDecisionJSON(decisionJSON).
		SetState(budgetrequest.State(state)).
		SetAdmittedAt(normalized.AdmittedAt).
		SetUpdatedAt(normalized.AdmittedAt).
		Save(ctx); err != nil {
		return AdmissionDecision{}, err
	}
	if decision.Allowed {
		for _, candidate := range candidates {
			if candidate.reserve > 0 {
				if _, err := tx.BudgetBucket.UpdateOneID(candidate.bucket.ID).
					SetReservedQuantity(candidate.bucket.ReservedQuantity + candidate.reserve).
					SetUpdatedAt(normalized.AdmittedAt).
					Save(ctx); err != nil {
					return AdmissionDecision{}, err
				}
			}
			if _, err := tx.BudgetAllocation.Create().
				SetRequestID(normalized.RequestID).
				SetBudgetLimitID(candidate.limit.ID).
				SetBudgetBucketID(candidate.bucket.ID).
				SetScopeKey(candidate.limit.ScopeKey).
				SetPeriod(budgetallocation.Period(candidate.limit.Period)).
				SetMeter(budgetallocation.Meter(candidate.limit.Meter)).
				SetReservedQuantity(candidate.reserve).
				SetCreatedAt(normalized.AdmittedAt).
				SetUpdatedAt(normalized.AdmittedAt).
				Save(ctx); err != nil {
				return AdmissionDecision{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return AdmissionDecision{}, err
	}
	return decision, nil
}

func (s *Service) Start(ctx context.Context, input LifecycleInput) error {
	if err := validateLifecycle(input); err != nil {
		return ErrInvalidTransition
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	request, err := tx.BudgetRequest.Get(ctx, input.RequestID)
	if ent.IsNotFound(err) {
		return ErrRequestNotFound
	}
	if err != nil {
		return err
	}
	if request.LastLifecycleRevision == input.Revision {
		if request.LastLifecycleHash == nil || *request.LastLifecycleHash != input.EventHash {
			return ErrLifecycleRevisionConflict
		}
		return nil
	}
	if input.Revision <= request.LastLifecycleRevision {
		return ErrLifecycleRevisionConflict
	}
	if RequestState(request.State) != RequestAdmitted || input.OccurredAt.Before(request.AdmittedAt) {
		return ErrRequestNotAdmitted
	}
	if _, err := tx.BudgetRequest.UpdateOneID(request.ID).
		SetState(budgetrequest.State(RequestStarted)).
		SetStartedAt(input.OccurredAt.UTC()).
		SetLastLifecycleRevision(input.Revision).
		SetLastLifecycleHash(input.EventHash).
		SetUpdatedAt(input.OccurredAt.UTC()).
		Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Release(ctx context.Context, input ReleaseInput) error {
	if validateLifecycle(input.LifecycleInput) != nil || strings.TrimSpace(input.Reason) == "" {
		return ErrInvalidTransition
	}
	now := input.OccurredAt.UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	request, err := tx.BudgetRequest.Get(ctx, input.RequestID)
	if ent.IsNotFound(err) {
		return ErrRequestNotFound
	}
	if err != nil {
		return err
	}
	if request.LastLifecycleRevision == input.Revision {
		if request.LastLifecycleHash == nil || *request.LastLifecycleHash != input.EventHash {
			return ErrLifecycleRevisionConflict
		}
		return nil
	}
	if input.Revision <= request.LastLifecycleRevision {
		return ErrLifecycleRevisionConflict
	}
	switch RequestState(request.State) {
	case RequestAdmitted:
	case RequestStarted, RequestReconciliation, RequestSettled, RequestResolved:
		return ErrAlreadyForwarded
	default:
		return ErrRequestNotAdmitted
	}
	allocations, err := requestAllocations(ctx, tx, input.RequestID)
	if err != nil {
		return err
	}
	if err := releaseReservations(ctx, tx, allocations, now); err != nil {
		return err
	}
	if _, err := tx.BudgetRequest.UpdateOneID(request.ID).
		SetState(budgetrequest.State(RequestReleased)).
		SetCompletedAt(now).
		SetLastLifecycleRevision(input.Revision).
		SetLastLifecycleHash(input.EventHash).
		SetTerminalReason(strings.TrimSpace(input.Reason)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeAdmit(input AdmitInput) (AdmitInput, map[Meter]int64, map[Meter]bool, string, error) {
	if platformid.Validate(platformid.Request, input.RequestID) != nil || !validSHA256(input.RequestHash) || platformid.Validate(platformid.Deployment, input.DeploymentID) != nil || validateSubject(input.UserID, input.Capability) != nil || input.AdmittedAt.IsZero() || input.ManagedGeneration < 0 || input.ControlRevision < 0 {
		return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
	}
	if input.InteractionID != nil && platformid.Validate(platformid.Interaction, *input.InteractionID) != nil {
		return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
	}
	if input.DeviceID != nil && platformid.Validate(platformid.Device, *input.DeviceID) != nil {
		return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
	}
	kind, err := platformid.KindOf(input.ResourceID)
	if err != nil || !resourceMatchesCapability(kind, input.Capability) || platformid.Validate(platformid.Upstream, input.UpstreamID) != nil || !protocolMatchesCapability(input.ClientProtocol, input.Capability) {
		return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
	}
	input.AdmittedAt = input.AdmittedAt.UTC()
	known := map[Meter]int64{MeterRequests: 1}
	for _, quantity := range input.KnownQuantities {
		if !validMeter(quantity.Meter) || !meterAllowed(input.Capability, quantity.Meter) || quantity.Quantity < 0 {
			return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
		}
		if previous, duplicate := known[quantity.Meter]; duplicate && (quantity.Meter != MeterRequests || previous != quantity.Quantity) {
			return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
		}
		if quantity.Meter == MeterRequests && quantity.Quantity != 1 {
			return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
		}
		known[quantity.Meter] = quantity.Quantity
	}
	supported := map[Meter]bool{MeterRequests: true}
	for _, meter := range input.SupportedMeters {
		if !validMeter(meter) || !meterAllowed(input.Capability, meter) {
			return AdmitInput{}, nil, nil, "", ErrInvalidConfiguration
		}
		supported[meter] = true
	}
	for meter := range known {
		supported[meter] = true
	}
	hash, err := admissionHash(input, known, supported)
	return input, known, supported, hash, err
}

func admissionHash(input AdmitInput, known map[Meter]int64, supported map[Meter]bool) (string, error) {
	type canonical struct {
		RequestID         string          `json:"requestId"`
		RequestHash       string          `json:"requestHash"`
		DeploymentID      string          `json:"deploymentId"`
		UserID            string          `json:"userId"`
		InteractionID     *string         `json:"interactionId,omitempty"`
		DeviceID          *string         `json:"deviceId,omitempty"`
		Capability        Capability      `json:"capability"`
		ResourceID        string          `json:"resourceId"`
		ClientProtocol    ClientProtocol  `json:"clientProtocol"`
		UpstreamID        string          `json:"upstreamId"`
		ManagedGeneration int64           `json:"managedGeneration"`
		ControlRevision   int64           `json:"controlRevision"`
		AdmittedAt        string          `json:"admittedAt"`
		Known             []MeterQuantity `json:"known"`
		Supported         []Meter         `json:"supported"`
	}
	payload := canonical{
		RequestID: input.RequestID, RequestHash: input.RequestHash, DeploymentID: input.DeploymentID,
		UserID: input.UserID, InteractionID: input.InteractionID, DeviceID: input.DeviceID, Capability: input.Capability,
		ResourceID: input.ResourceID, ClientProtocol: input.ClientProtocol, UpstreamID: input.UpstreamID,
		ManagedGeneration: input.ManagedGeneration, ControlRevision: input.ControlRevision,
		AdmittedAt: input.AdmittedAt.UTC().Format(time.RFC3339Nano),
	}
	for meter, quantity := range known {
		payload.Known = append(payload.Known, MeterQuantity{Meter: meter, Quantity: quantity})
	}
	sort.Slice(payload.Known, func(i, j int) bool { return payload.Known[i].Meter < payload.Known[j].Meter })
	for meter := range supported {
		payload.Supported = append(payload.Supported, meter)
	}
	sort.Slice(payload.Supported, func(i, j int) bool { return payload.Supported[i] < payload.Supported[j] })
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func findOrCreateBucket(ctx context.Context, tx *ent.Tx, limit *ent.BudgetLimit, window periodWindow, now time.Time) (*ent.BudgetBucket, error) {
	bucket, err := tx.BudgetBucket.Query().Where(
		budgetbucket.ScopeKeyEQ(limit.ScopeKey),
		budgetbucket.PeriodStartEQ(window.Start),
		budgetbucket.MeterEQ(budgetbucket.Meter(limit.Meter)),
	).Only(ctx)
	if err == nil {
		return bucket, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	return tx.BudgetBucket.Create().
		SetScopeKey(limit.ScopeKey).
		SetPeriod(budgetbucket.Period(limit.Period)).
		SetPeriodStart(window.Start).
		SetNillablePeriodEnd(window.End).
		SetMeter(budgetbucket.Meter(limit.Meter)).
		SetUpdatedAt(now).
		Save(ctx)
}

func resourceMatchesCapability(kind platformid.Kind, capability Capability) bool {
	switch capability {
	case CapabilityModel:
		return kind == platformid.Model
	case CapabilityImageGeneration:
		return kind == platformid.ImageGeneration
	case CapabilityTTS:
		return kind == platformid.TTS
	case CapabilityASR:
		return kind == platformid.ASR
	case CapabilityMCP:
		return kind == platformid.MCP
	default:
		return false
	}
}

func protocolMatchesCapability(protocol ClientProtocol, capability Capability) bool {
	switch capability {
	case CapabilityModel:
		return protocol == ProtocolOpenAIChatCompletions || protocol == ProtocolOpenAIResponses || protocol == ProtocolGoogleGenerateContent || protocol == ProtocolAnthropicMessages
	case CapabilityImageGeneration:
		return protocol == ProtocolOpenAIImagesGenerations
	case CapabilityTTS:
		return protocol == ProtocolOpenAIAudioSpeech || protocol == ProtocolGeminiGenerateContentTTS || protocol == ProtocolMiMoChatCompletionsTTS
	case CapabilityASR:
		return protocol == ProtocolOpenAIAudioTranscriptions || protocol == ProtocolDashScopeHTTPASR || protocol == ProtocolOpenAIRealtimeTranscription || protocol == ProtocolDashScopeRealtimeASR
	case CapabilityMCP:
		return protocol == ProtocolMCPStreamableHTTP
	default:
		return false
	}
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validateLifecycle(input LifecycleInput) error {
	if platformid.Validate(platformid.Request, input.RequestID) != nil || input.Revision <= 0 || input.OccurredAt.IsZero() || !validSHA256(input.EventHash) {
		return ErrInvalidTransition
	}
	return nil
}

func overallResetAt(blockers []BlockingLimit) *time.Time {
	if len(blockers) == 0 {
		return nil
	}
	var latest time.Time
	for _, blocker := range blockers {
		if blocker.ResetAt == nil {
			return nil
		}
		if blocker.ResetAt.After(latest) {
			latest = *blocker.ResetAt
		}
	}
	return &latest
}
