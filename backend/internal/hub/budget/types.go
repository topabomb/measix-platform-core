package budget

import (
	"encoding/json"
	"errors"
	"time"

	"measix/platform/ent"
)

type Capability string

const (
	CapabilityModel           Capability = "MODEL"
	CapabilityImageGeneration Capability = "IMAGE_GENERATION"
	CapabilityTTS             Capability = "TTS"
	CapabilityASR             Capability = "ASR"
	CapabilityMCP             Capability = "MCP"
)

type Mode string

const (
	ModeUnlimited Mode = "UNLIMITED"
	ModeLimited   Mode = "LIMITED"
)

type Source string

const (
	SourceDefault  Source = "DEFAULT"
	SourceTemplate Source = "TEMPLATE"
	SourceExplicit Source = "EXPLICIT"
)

type Period string

const (
	PeriodDay      Period = "DAY"
	PeriodWeek     Period = "WEEK"
	PeriodMonth    Period = "MONTH"
	PeriodLifetime Period = "LIFETIME"
)

type Meter string

const (
	MeterRequests          Meter = "REQUESTS"
	MeterRequestedImages   Meter = "REQUESTED_IMAGES"
	MeterInputTokens       Meter = "INPUT_TOKENS"
	MeterOutputTokens      Meter = "OUTPUT_TOKENS"
	MeterCachedTokens      Meter = "CACHED_TOKENS"
	MeterTotalTokens       Meter = "TOTAL_TOKENS"
	MeterCharacters        Meter = "CHARACTERS"
	MeterAudioMilliseconds Meter = "AUDIO_MILLISECONDS"
)

type ClientProtocol string

const (
	ProtocolOpenAIChatCompletions       ClientProtocol = "OPENAI_CHAT_COMPLETIONS"
	ProtocolOpenAIImagesGenerations     ClientProtocol = "OPENAI_IMAGES_GENERATIONS"
	ProtocolOpenAIResponses             ClientProtocol = "OPENAI_RESPONSES"
	ProtocolGoogleGenerateContent       ClientProtocol = "GOOGLE_GENERATE_CONTENT"
	ProtocolAnthropicMessages           ClientProtocol = "ANTHROPIC_MESSAGES"
	ProtocolOpenAIAudioSpeech           ClientProtocol = "OPENAI_AUDIO_SPEECH"
	ProtocolGeminiGenerateContentTTS    ClientProtocol = "GEMINI_GENERATE_CONTENT_TTS"
	ProtocolMiMoChatCompletionsTTS      ClientProtocol = "MIMO_CHAT_COMPLETIONS_TTS"
	ProtocolOpenAIAudioTranscriptions   ClientProtocol = "OPENAI_AUDIO_TRANSCRIPTIONS"
	ProtocolDashScopeHTTPASR            ClientProtocol = "DASHSCOPE_HTTP_ASR"
	ProtocolOpenAIRealtimeTranscription ClientProtocol = "OPENAI_REALTIME_TRANSCRIPTION"
	ProtocolDashScopeRealtimeASR        ClientProtocol = "DASHSCOPE_REALTIME_ASR"
	ProtocolMCPStreamableHTTP           ClientProtocol = "MCP_STREAMABLE_HTTP"
)

type RequestState string

const (
	RequestDenied         RequestState = "DENIED"
	RequestAdmitted       RequestState = "ADMITTED"
	RequestStarted        RequestState = "STARTED"
	RequestReconciliation RequestState = "RECONCILIATION"
	RequestSettled        RequestState = "SETTLED"
	RequestReleased       RequestState = "RELEASED"
	RequestResolved       RequestState = "RESOLVED"
)

type DecisionCode string

const (
	DecisionAllowed          DecisionCode = "ALLOWED"
	DecisionBudgetExhausted  DecisionCode = "BUDGET_EXHAUSTED"
	DecisionMeterUnavailable DecisionCode = "USAGE_METER_UNAVAILABLE"
	DecisionInFlightLimit    DecisionCode = "IN_FLIGHT_LIMIT"
)

var (
	ErrInvalidConfiguration       = errors.New("invalid budget configuration")
	ErrRevisionConflict           = errors.New("budget revision conflict")
	ErrRequestConflict            = errors.New("budget request id reused with different content")
	ErrRequestNotFound            = errors.New("budget request not found")
	ErrRequestNotAdmitted         = errors.New("budget request was not admitted")
	ErrRequestNotStarted          = errors.New("budget request was not started")
	ErrAlreadyForwarded           = errors.New("budget request may already have been forwarded")
	ErrLifecycleRevisionConflict  = errors.New("budget lifecycle revision conflict")
	ErrSettlementRevisionConflict = errors.New("settlement revision conflict")
	ErrInvalidSettlement          = errors.New("invalid budget settlement")
	ErrInvalidTransition          = errors.New("invalid budget request transition")
	ErrReconciliationNotOpen      = errors.New("budget reconciliation is not open")
	ErrIdentityDeleted            = errors.New("enterprise identity was deleted")
	ErrTemplateNotFound           = errors.New("budget template not found")
	ErrTemplateAssigned           = errors.New("budget template is assigned")
	ErrAssignmentNotFound         = errors.New("budget template assignment not found")
)

const DefaultMaxInFlight int64 = 32

type Service struct {
	Client      *ent.Client
	Location    *time.Location
	Now         func() time.Time
	MaxInFlight int64
}

// NewService fixes natural budget periods to one deployment IANA timezone.
// Callers must rebuild the service explicitly to change that deployment setting;
// active buckets retain their already captured UTC bounds.
func NewService(client *ent.Client, ianaTimeZone string) (*Service, error) {
	if client == nil || ianaTimeZone == "" {
		return nil, ErrInvalidConfiguration
	}
	location, err := time.LoadLocation(ianaTimeZone)
	if err != nil {
		return nil, err
	}
	return &Service{
		Client:      client,
		Location:    location,
		Now:         time.Now,
		MaxInFlight: DefaultMaxInFlight,
	}, nil
}

type LimitSpec struct {
	Meter Meter `json:"meter"`
	Limit int64 `json:"limit"`
}

type ScopeInput struct {
	ScopeKey string      `json:"scopeKey,omitempty"`
	Period   Period      `json:"period"`
	Limits   []LimitSpec `json:"limits"`
}

type PutBudgetInput struct {
	UserID           string
	Capability       Capability
	ExpectedRevision int64
	Mode             Mode
	Scopes           []ScopeInput
	ActorUserID      string
	Reason           string
}

type ScopeView struct {
	ScopeKey      string      `json:"scopeKey"`
	Period        Period      `json:"period"`
	EffectiveFrom time.Time   `json:"effectiveFrom"`
	Limits        []LimitSpec `json:"limits"`
}

type BudgetView struct {
	UserID      string      `json:"userId"`
	Capability  Capability  `json:"capability"`
	Mode        Mode        `json:"mode"`
	Source      Source      `json:"source"`
	Revision    int64       `json:"revision"`
	ActivatedAt *time.Time  `json:"activatedAt,omitempty"`
	Scopes      []ScopeView `json:"scopes"`
}

type MeterQuantity struct {
	Meter    Meter `json:"meter"`
	Quantity int64 `json:"quantity"`
}

type AdmitInput struct {
	RequestID         string
	RequestHash       string
	DeploymentID      string
	UserID            string
	InteractionID     *string
	DeviceID          *string
	Capability        Capability
	ResourceID        string
	ClientProtocol    ClientProtocol
	UpstreamID        string
	ManagedGeneration int64
	ControlRevision   int64
	AdmittedAt        time.Time
	KnownQuantities   []MeterQuantity
	SupportedMeters   []Meter
}

type BlockingLimit struct {
	ScopeKey   string     `json:"scopeKey"`
	Meter      Meter      `json:"meter"`
	Period     Period     `json:"period"`
	Limit      int64      `json:"limit"`
	Used       int64      `json:"used"`
	Reserved   int64      `json:"reserved"`
	ScopeStart time.Time  `json:"scopeStart"`
	ResetAt    *time.Time `json:"resetAt,omitempty"`
}

type AdmissionDecision struct {
	RequestID        string          `json:"requestId"`
	Capability       Capability      `json:"capability"`
	Allowed          bool            `json:"allowed"`
	Code             DecisionCode    `json:"code"`
	Mode             Mode            `json:"mode"`
	Source           Source          `json:"source"`
	Revision         int64           `json:"revision"`
	InFlightRequests int64           `json:"inFlightRequests"`
	ActivatedAt      *time.Time      `json:"activatedAt,omitempty"`
	BlockingLimits   []BlockingLimit `json:"blockingLimits"`
	Unavailable      []Meter         `json:"unavailableMeters,omitempty"`
	ResetAt          *time.Time      `json:"resetAt,omitempty"`
	AsOf             time.Time       `json:"asOf"`
}

type LifecycleInput struct {
	RequestID  string
	Revision   int64
	OccurredAt time.Time
	EventHash  string
}

type ReleaseInput struct {
	LifecycleInput
	Reason string
}

type SettlementInput struct {
	RequestID  string
	Revision   int64
	Meters     []MeterQuantity
	Complete   bool
	ReportedBy string
}

type SettlementOutcome string

const (
	OutcomeSettled        SettlementOutcome = "SETTLED"
	OutcomeCorrected      SettlementOutcome = "CORRECTED"
	OutcomeReconciliation SettlementOutcome = "RECONCILIATION"
)

type SettlementResult struct {
	RequestID string
	Revision  int64
	Outcome   SettlementOutcome
	Duplicate bool
}

type ResolutionAction string

const (
	ResolutionConfirmUsage     ResolutionAction = "CONFIRM_USAGE"
	ResolutionReleaseUncertain ResolutionAction = "RELEASE_UNCERTAIN"
)

type ResolveInput struct {
	RequestID   string
	Action      ResolutionAction
	Revision    int64
	Meters      []MeterQuantity
	ActorUserID string
	Reason      string
}

type EffectiveStatus string

const (
	StatusUnlimited      EffectiveStatus = "UNLIMITED"
	StatusAvailable      EffectiveStatus = "AVAILABLE"
	StatusExhausted      EffectiveStatus = "EXHAUSTED"
	StatusReconciliation EffectiveStatus = "RECONCILIATION"
)

type EffectiveLimitState struct {
	ScopeKey  string     `json:"scopeKey"`
	Meter     Meter      `json:"meter"`
	Period    Period     `json:"period"`
	Limit     int64      `json:"limit"`
	Used      int64      `json:"used"`
	Reserved  int64      `json:"reserved"`
	Remaining int64      `json:"remaining"`
	Overage   int64      `json:"overage"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

type EffectiveState struct {
	Budget            BudgetView            `json:"budget"`
	Status            EffectiveStatus       `json:"status"`
	Limits            []EffectiveLimitState `json:"limits"`
	BlockingLimits    []EffectiveLimitState `json:"blockingLimits"`
	InFlightRequests  int64                 `json:"inFlightRequests"`
	HasReconciliation bool                  `json:"hasReconciliation"`
	ResetAt           *time.Time            `json:"resetAt,omitempty"`
	AsOf              time.Time             `json:"asOf"`
}

type AuditQuery struct {
	UserID     string
	Capability *Capability
	PageSize   int
	Cursor     string
}

type AuditRecord struct {
	ID          int             `json:"id"`
	UserID      string          `json:"userId"`
	Capability  Capability      `json:"capability"`
	Revision    int64           `json:"revision"`
	ActorUserID string          `json:"actorUserId"`
	Action      string          `json:"action"`
	Reason      string          `json:"reason"`
	Before      json.RawMessage `json:"before,omitempty"`
	After       json.RawMessage `json:"after"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type AuditPage struct {
	Items      []AuditRecord `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type ReconciliationQuery struct {
	UserID     *string
	Capability *Capability
	PageSize   int
	Cursor     string
}

type ReconciliationAllocation struct {
	ScopeKey            string `json:"scopeKey"`
	Period              Period `json:"period"`
	Meter               Meter  `json:"meter"`
	Reserved            int64  `json:"reserved"`
	Observed            int64  `json:"observed"`
	ReservationReleased bool   `json:"reservationReleased"`
	Resolved            bool   `json:"resolved"`
}

type ReconciliationRecord struct {
	ID                     int                        `json:"id"`
	RequestID              string                     `json:"requestId"`
	UserID                 string                     `json:"userId"`
	Capability             Capability                 `json:"capability"`
	ResourceID             string                     `json:"resourceId"`
	ClientProtocol         ClientProtocol             `json:"clientProtocol"`
	RequestState           RequestState               `json:"requestState"`
	Reason                 string                     `json:"reason"`
	LastSettlementRevision int64                      `json:"lastSettlementRevision"`
	Allocations            []ReconciliationAllocation `json:"allocations"`
	AdmittedAt             time.Time                  `json:"admittedAt"`
	StartedAt              *time.Time                 `json:"startedAt,omitempty"`
	OpenedAt               time.Time                  `json:"openedAt"`
	UpdatedAt              time.Time                  `json:"updatedAt"`
}

type ReconciliationPage struct {
	Items      []ReconciliationRecord `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
}
