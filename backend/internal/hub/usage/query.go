package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/device"
	"measix/platform/ent/managedrelease"
	"measix/platform/ent/predicate"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

type Summary struct {
	From                  time.Time
	To                    time.Time
	RequestCount          int
	ForwardedRequestCount int
	RequestCompleteness   RequestCompletenessCounts
	RequestBytes          int64
	ResponseBytes         int64
	Meters                []MeterSummary
	Cost                  CostSummary
}

type RequestCompletenessCounts struct {
	Exact, Partial, Unknown int
}

type MeterSummary struct {
	Meter      string
	Quantity   string
	Confidence Completeness
}

type CostSummary struct {
	State                  CostState
	Amount                 string
	Currency               string
	Amounts                []CurrencyAmount
	PricedRequests         int
	PartialRequests        int
	UnknownRequests        int
	MissingPricingRequests int
	UnknownMeterRequests   int
	Lines                  []CostLine
}

type ResourceKind string

const (
	ResourceKindProvider ResourceKind = "PROVIDER"
	ResourceKindModel    ResourceKind = "MODEL"
	ResourceKindImage    ResourceKind = "IMAGE_GENERATION"
	ResourceKindTTS      ResourceKind = "TTS"
	ResourceKindASR      ResourceKind = "ASR"
	ResourceKindMCP      ResourceKind = "MCP"
)

type RequestStatus string

const (
	RequestStatusSuccess RequestStatus = "SUCCESS"
	RequestStatusError   RequestStatus = "ERROR"
	RequestStatusBlocked RequestStatus = "BLOCKED"
)

// Filter captures the combinable usage read-model filters (Time / User /
// Resource / Resource Kind / Upstream / Status / Completeness).
type Filter struct {
	After          string
	From           *time.Time
	To             *time.Time
	UserID         string
	ResourceID     string
	ResourceKind   ResourceKind
	UpstreamID     string
	Status         RequestStatus
	Completeness   Completeness
	ClientProtocol string
}

type RequestView struct {
	RequestID           string
	InteractionID       *string
	DeploymentID        string
	UserID              string
	DeviceID            *string
	ResourceID          string
	ResourceKind        ResourceKind
	ClientProtocol      string
	ResourceDisplayName string
	// Display identity resolved from the users and devices tables so an audit row
	// can be read without resolving identifiers by hand. Both are display
	// metadata and carry no authority.
	UserDisplayName     string
	DeviceName          string
	RuntimeRouteID      string
	UpstreamID          string
	ManagedGeneration   int
	ControlRevision     int
	StartedAt           time.Time
	CompletedAt         time.Time
	Forwarded           bool
	HTTPStatus          int
	UpstreamHTTPStatus  *int
	RequestBytes        int
	ResponseBytes       int
	DurationMs          int
	ErrorClass          *string
	RequestCompleteness Completeness
	SettlementState     string
	SettlementRevision  int64
	SemanticMeters      []MeterSummary
	Cost                CostSummary
	Budget              *budget.AdmissionDecision
}

func requestView(row *ent.RequestUsage) RequestView {
	var upstreamStatus *int
	if row.UpstreamHTTPStatus != nil {
		value := int(*row.UpstreamHTTPStatus)
		upstreamStatus = &value
	}
	return RequestView{
		RequestID: row.RequestID, InteractionID: row.InteractionID, DeploymentID: row.DeploymentID, UserID: row.UserID, DeviceID: row.DeviceID,
		ResourceID: row.ResourceID, ResourceKind: ResourceKind(row.ResourceKind), ClientProtocol: row.ClientProtocol,
		RuntimeRouteID: row.RuntimeRouteID, UpstreamID: row.UpstreamID, ManagedGeneration: int(row.ManagedGeneration), ControlRevision: int(row.ControlRevision),
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded, HTTPStatus: row.HTTPStatus, UpstreamHTTPStatus: upstreamStatus,
		RequestBytes: int(row.RequestBytes), ResponseBytes: int(row.ResponseBytes), DurationMs: int(row.DurationMs), ErrorClass: row.ErrorClass,
		RequestCompleteness: Completeness(row.RequestCompleteness), SettlementState: row.SettlementState, SettlementRevision: row.SettlementRevision,
	}
}

func (s *Service) ListRequests(ctx context.Context, filter Filter, limit int) ([]RequestView, error) {
	if limit < 1 || limit > 201 {
		limit = 50
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	q := s.Client.RequestUsage.Query()
	if preds := requestFilterPreds(filter); len(preds) > 0 {
		q = q.Where(requestusage.And(preds...))
	}
	if filter.After != "" {
		timestamp, id, ok := strings.Cut(filter.After, "|")
		completed, err := time.Parse(time.RFC3339Nano, timestamp)
		if !ok || err != nil || platformid.Validate(platformid.Request, id) != nil {
			return nil, ErrInvalidBatch
		}
		q = q.Where(requestusage.Or(requestusage.CompletedAtLT(completed), requestusage.And(requestusage.CompletedAtEQ(completed), requestusage.RequestIDGT(id))))
	}
	rows, err := q.Order(ent.Desc(requestusage.FieldCompletedAt), ent.Asc(requestusage.FieldRequestID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]RequestView, 0, len(rows))
	for _, row := range rows {
		views = append(views, requestView(row))
	}
	return views, s.enrich(ctx, views)
}

// enrich resolves the display names a request carries but does not own: the
// resource name from the release snapshot, and the identity of the user and
// device. List and detail both go through here so the two views cannot disagree
// about the same request.
func (s *Service) enrich(ctx context.Context, views []RequestView) error {
	if err := s.resourceNames(ctx, views); err != nil {
		return err
	}
	if err := s.identityNames(ctx, views); err != nil {
		return err
	}
	return s.usageFacts(ctx, views)
}

// usageFacts attaches only the current settlement revision and the immutable
// admission decision. Corrections remain in the ledger for audit but never
// appear twice in a request projection.
func (s *Service) usageFacts(ctx context.Context, views []RequestView) error {
	if len(views) == 0 {
		return nil
	}
	requestIDs := make([]string, 0, len(views))
	revisions := make(map[string]int64, len(views))
	for _, view := range views {
		requestIDs = append(requestIDs, view.RequestID)
		revisions[view.RequestID] = view.SettlementRevision
	}
	semanticRows, err := s.Client.SemanticUsage.Query().Where(semanticusage.RequestIDIn(requestIDs...)).All(ctx)
	if err != nil {
		return err
	}
	meters := make(map[string][]MeterSummary, len(views))
	for _, row := range semanticRows {
		if revisions[row.RequestID] != row.SettlementRevision {
			continue
		}
		meters[row.RequestID] = append(meters[row.RequestID], MeterSummary{
			Meter: row.Meter, Quantity: row.QuantityDecimal, Confidence: Completeness(row.Completeness),
		})
	}
	for id := range meters {
		sort.Slice(meters[id], func(i, j int) bool { return meters[id][i].Meter < meters[id][j].Meter })
	}
	budgetRows, err := s.Client.BudgetRequest.Query().Where(budgetrequest.IDIn(requestIDs...)).All(ctx)
	if err != nil {
		return err
	}
	decisions := make(map[string]*budget.AdmissionDecision, len(budgetRows))
	for _, row := range budgetRows {
		var decision budget.AdmissionDecision
		if err := json.Unmarshal(row.DecisionJSON, &decision); err != nil {
			return err
		}
		decisions[row.ID] = &decision
	}
	for i := range views {
		views[i].SemanticMeters = meters[views[i].RequestID]
		if views[i].SemanticMeters == nil {
			views[i].SemanticMeters = []MeterSummary{}
		}
		views[i].Budget = decisions[views[i].RequestID]
	}
	rules, err := s.Client.PricingRule.Query().All(ctx)
	if err != nil {
		return err
	}
	for i := range views {
		priced, err := priceRequest(views[i], rules)
		if err != nil {
			return err
		}
		views[i].Cost = priced.CostSummary
	}
	return nil
}

// identityNames resolves who and which device each request belongs to, in bulk so
// that a page of results costs two queries rather than one per row. Names come
// from the authoritative users and devices tables, never from the usage row. The
// references are enforced by the schema (`user_id` is NOT NULL, `device_id`
// nullable, both with a foreign key), so a present identifier always resolves;
// a request without a device simply has no device name.
func (s *Service) identityNames(ctx context.Context, views []RequestView) error {
	if len(views) == 0 {
		return nil
	}
	userIDs := make([]string, 0, len(views))
	deviceIDs := make([]string, 0, len(views))
	seenUser := make(map[string]bool, len(views))
	seenDevice := make(map[string]bool, len(views))
	for _, view := range views {
		if view.UserID != "" && !seenUser[view.UserID] {
			userIDs = append(userIDs, view.UserID)
			seenUser[view.UserID] = true
		}
		if view.DeviceID != nil && *view.DeviceID != "" && !seenDevice[*view.DeviceID] {
			deviceIDs = append(deviceIDs, *view.DeviceID)
			seenDevice[*view.DeviceID] = true
		}
	}
	userNames := make(map[string]string, len(userIDs))
	if len(userIDs) > 0 {
		rows, err := s.Client.User.Query().Where(user.IDIn(userIDs...)).All(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			userNames[row.ID] = row.DisplayName
		}
	}
	deviceNames := make(map[string]string, len(deviceIDs))
	if len(deviceIDs) > 0 {
		rows, err := s.Client.Device.Query().Where(device.IDIn(deviceIDs...)).All(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			deviceNames[row.ID] = row.Name
		}
	}
	for i := range views {
		views[i].UserDisplayName = userNames[views[i].UserID]
		if views[i].DeviceID != nil {
			views[i].DeviceName = deviceNames[*views[i].DeviceID]
		}
	}
	return nil
}

// Release snapshots are immutable. Batch by generation instead of querying each
// request or substituting names from the current editable draft.
func (s *Service) resourceNames(ctx context.Context, views []RequestView) error {
	if len(views) == 0 {
		return nil
	}
	generations := make([]int64, 0, len(views))
	seen := make(map[int64]bool)
	for _, view := range views {
		generation := int64(view.ManagedGeneration)
		if !seen[generation] {
			generations = append(generations, generation)
			seen[generation] = true
		}
	}
	releases, err := s.Client.ManagedRelease.Query().Where(managedrelease.ManagedGenerationIn(generations...)).All(ctx)
	if err != nil {
		return err
	}
	names := make(map[int]map[string]string, len(releases))
	for _, release := range releases {
		var snapshot clientapi.ManagedSnapshot
		if err := json.Unmarshal(release.SnapshotJSON, &snapshot); err != nil {
			return err
		}
		resources := make(map[string]string)
		for _, provider := range snapshot.Providers {
			resources[provider.ProviderId] = provider.DisplayName
		}
		for _, model := range snapshot.Models {
			resources[model.ModelId] = model.DisplayName
		}
		if snapshot.ImageGenerators != nil {
			for _, image := range *snapshot.ImageGenerators {
				resources[image.ImageId] = image.DisplayName
			}
		}
		for _, speech := range snapshot.Tts {
			resources[speech.TtsId] = speech.DisplayName
		}
		for _, speech := range snapshot.Asr {
			resources[speech.AsrId] = speech.DisplayName
		}
		for _, tool := range snapshot.Mcp {
			resources[tool.McpServerId] = tool.DisplayName
		}
		names[int(release.ManagedGeneration)] = resources
	}
	for i := range views {
		views[i].ResourceDisplayName = names[views[i].ManagedGeneration][views[i].ResourceID]
	}
	return nil
}

// requestFilterPreds returns the combinable predicates for a Filter over the
// RequestUsage entity, including the same half-open time window as summaries.
func requestFilterPreds(filter Filter) []predicate.RequestUsage {
	preds := []predicate.RequestUsage{}
	if filter.From != nil {
		preds = append(preds, requestusage.CompletedAtGTE(filter.From.UTC()))
	}
	if filter.To != nil {
		preds = append(preds, requestusage.CompletedAtLT(filter.To.UTC()))
	}
	if filter.Completeness != "" {
		preds = append(preds, requestCompletenessPred(filter.Completeness))
	}
	if filter.UserID != "" {
		preds = append(preds, requestusage.UserIDEQ(filter.UserID))
	}
	if filter.ResourceID != "" {
		preds = append(preds, requestusage.ResourceIDEQ(filter.ResourceID))
	}
	if filter.ResourceKind != "" {
		preds = append(preds, requestusage.ResourceIDHasPrefix(resourceKindPrefix(filter.ResourceKind)))
	}
	if filter.UpstreamID != "" {
		preds = append(preds, requestusage.UpstreamIDEQ(filter.UpstreamID))
	}
	if filter.ClientProtocol != "" {
		preds = append(preds, requestusage.ClientProtocolEQ(filter.ClientProtocol))
	}
	if filter.Status != "" {
		preds = append(preds, requestStatusPred(filter.Status))
	}
	return preds
}

func resourceKindPrefix(kind ResourceKind) string {
	switch kind {
	case ResourceKindProvider:
		return "prv_"
	case ResourceKindModel:
		return "mdl_"
	case ResourceKindImage:
		return "img_"
	case ResourceKindTTS:
		return "tts_"
	case ResourceKindASR:
		return "asr_"
	case ResourceKindMCP:
		return "mcp_"
	default:
		return ""
	}
}

func requestStatusPred(status RequestStatus) predicate.RequestUsage {
	switch status {
	case RequestStatusError:
		return requestusage.And(
			requestusage.ForwardedEQ(true),
			requestusage.HTTPStatusGTE(400),
		)
	case RequestStatusBlocked:
		return requestusage.ForwardedEQ(false)
	default: // SUCCESS
		return requestusage.And(
			requestusage.ForwardedEQ(true),
			requestusage.HTTPStatusLT(400),
		)
	}
}

func (s *Service) GetRequest(ctx context.Context, requestID string) (RequestView, error) {
	row, err := s.Client.RequestUsage.Query().Where(requestusage.RequestIDEQ(requestID)).Only(ctx)
	if ent.IsNotFound(err) {
		return RequestView{}, ErrRequestNotFound
	}
	if err != nil {
		return RequestView{}, err
	}
	views := []RequestView{requestView(row)}
	err = s.enrich(ctx, views)
	return views[0], err
}

// GetRequests resolves request audit projections in one batch. Reconciliation
// uses this to show the same user, device and immutable resource names as the
// ordinary request list without issuing one query per row.
func (s *Service) GetRequests(ctx context.Context, requestIDs []string) (map[string]RequestView, error) {
	unique := make([]string, 0, len(requestIDs))
	seen := make(map[string]struct{}, len(requestIDs))
	for _, requestID := range requestIDs {
		if platformid.Validate(platformid.Request, requestID) != nil {
			return nil, ErrInvalidBatch
		}
		if _, ok := seen[requestID]; ok {
			continue
		}
		seen[requestID] = struct{}{}
		unique = append(unique, requestID)
	}
	result := make(map[string]RequestView, len(unique))
	if len(unique) == 0 {
		return result, nil
	}
	rows, err := s.Client.RequestUsage.Query().Where(requestusage.RequestIDIn(unique...)).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]RequestView, 0, len(rows))
	for _, row := range rows {
		views = append(views, requestView(row))
	}
	if err := s.enrich(ctx, views); err != nil {
		return nil, err
	}
	for _, view := range views {
		result[view.RequestID] = view
	}
	return result, nil
}

func (s *Service) Summary(ctx context.Context, filter Filter) (Summary, error) {
	if err := filter.Validate(); err != nil {
		return Summary{}, err
	}
	from, to := time.Unix(0, 0).UTC(), s.Now().UTC().Add(time.Nanosecond)
	if filter.From != nil {
		from = filter.From.UTC()
	}
	if filter.To != nil {
		to = filter.To.UTC()
	}
	if !from.Before(to) {
		return Summary{}, ErrInvalidBatch
	}
	preds := append(
		requestFilterPreds(filter),
		requestusage.CompletedAtGTE(from),
		requestusage.CompletedAtLT(to),
	)
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return Summary{}, err
	}
	defer tx.Rollback()
	requestQuery := tx.RequestUsage.Query().Where(requestusage.And(preds...))
	requestCount, err := requestQuery.Clone().Count(ctx)
	if err != nil {
		return Summary{}, err
	}
	forwardedCount, err := requestQuery.Clone().Where(requestusage.ForwardedEQ(true)).Count(ctx)
	if err != nil {
		return Summary{}, err
	}
	var totals []struct {
		RequestBytes  int64 `json:"request_bytes"`
		ResponseBytes int64 `json:"response_bytes"`
	}
	if err := requestQuery.Clone().Aggregate(
		func(selector *sql.Selector) string {
			return sql.As("COALESCE("+sql.Sum(selector.C(requestusage.FieldRequestBytes))+",0)", "request_bytes")
		},
		func(selector *sql.Selector) string {
			return sql.As("COALESCE("+sql.Sum(selector.C(requestusage.FieldResponseBytes))+",0)", "response_bytes")
		},
	).Scan(ctx, &totals); err != nil {
		return Summary{}, err
	}
	result := Summary{From: from, To: to, RequestCount: requestCount, ForwardedRequestCount: forwardedCount, Cost: CostSummary{State: CostUnknown}}
	if len(totals) == 1 {
		result.RequestBytes, result.ResponseBytes = totals[0].RequestBytes, totals[0].ResponseBytes
	}
	for _, bucket := range []struct {
		state Completeness
		count *int
	}{
		{CompletenessComplete, &result.RequestCompleteness.Exact},
		{CompletenessPartial, &result.RequestCompleteness.Partial},
		{CompletenessUnknown, &result.RequestCompleteness.Unknown},
	} {
		count, err := tx.RequestUsage.Query().Where(preds...).Where(requestCompletenessPred(bucket.state)).Count(ctx)
		if err != nil {
			return Summary{}, err
		}
		*bucket.count = count
	}
	semanticQ := tx.SemanticUsage.Query().Where(
		semanticusage.OccurredAtGTE(from),
		semanticusage.OccurredAtLT(to),
	)
	// Correlate in SQL: no unbounded ID parameter list, and linked meters use
	// the same whole-request completeness as the request list.
	semanticQ = semanticQ.Where(func(sel *sql.Selector) {
		table := sql.Table(requestusage.Table)
		matched := sql.Select(table.C(requestusage.FieldRequestID)).From(table)
		for _, pred := range preds {
			pred(matched)
		}
		matched.Where(sql.And(
			sql.ColumnsEQ(table.C(requestusage.FieldRequestID), sel.C(semanticusage.FieldRequestID)),
			sql.ColumnsEQ(table.C(requestusage.FieldSettlementRevision), sel.C(semanticusage.FieldSettlementRevision)),
		))
		sel.Where(sql.Exists(matched))
	})
	// Ingest normalizes count meters to integer units and AUDIO_SECONDS to
	// integer milliseconds. Aggregate in SQLite so analytics never loads every
	// retained settlement row merely to add it in process memory.
	type semanticAggregate struct {
		Meter         string `json:"meter"`
		QuantityUnits int64  `json:"quantity_units"`
		PartialCount  int    `json:"partial_count"`
		UnknownCount  int    `json:"unknown_count"`
	}
	var semantic []semanticAggregate
	if err := semanticQ.GroupBy(semanticusage.FieldMeter).Aggregate(
		func(selector *sql.Selector) string {
			return sql.As(sql.Sum(selector.C(semanticusage.FieldQuantityUnits)), "quantity_units")
		},
		func(selector *sql.Selector) string {
			column := selector.C(semanticusage.FieldCompleteness)
			return sql.As("SUM(CASE WHEN "+column+" = 'PARTIAL' THEN 1 ELSE 0 END)", "partial_count")
		},
		func(selector *sql.Selector) string {
			column := selector.C(semanticusage.FieldCompleteness)
			return sql.As("SUM(CASE WHEN "+column+" = 'UNKNOWN' THEN 1 ELSE 0 END)", "unknown_count")
		},
	).Scan(ctx, &semantic); err != nil {
		return Summary{}, err
	}
	sort.Slice(semantic, func(i, j int) bool { return semantic[i].Meter < semantic[j].Meter })
	for _, row := range semantic {
		confidence := CompletenessComplete
		if row.UnknownCount > 0 {
			confidence = CompletenessUnknown
		} else if row.PartialCount > 0 {
			confidence = CompletenessPartial
		}
		quantity := fmt.Sprintf("%d", row.QuantityUnits)
		if row.Meter == "AUDIO_SECONDS" {
			quantity = millisecondsAsSeconds(row.QuantityUnits)
		}
		result.Meters = append(result.Meters, MeterSummary{Meter: row.Meter, Quantity: quantity, Confidence: confidence})
	}
	costFilter := filter
	costFilter.From, costFilter.To = &from, &to
	var costs costAccumulator
	if err := (&Service{Client: tx.Client()}).analyzeCosts(ctx, costFilter, func(_ RequestView, priced CostBreakdown) error {
		return costs.add(priced)
	}); err != nil {
		return Summary{}, err
	}
	result.Cost, err = costs.summary()
	if err != nil {
		return Summary{}, err
	}
	return result, nil
}

func millisecondsAsSeconds(value int64) string {
	seconds, millis := value/1000, value%1000
	if millis == 0 {
		return fmt.Sprintf("%d", seconds)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%03d", seconds, millis), "0")
}

var ErrPricingRevisionConflict = errors.New("pricing revision conflict")

// UnknownRequestCount covers the retained request ledger using the same
// whole-request completeness rules as the usage list and its filters.
func (s *Service) UnknownRequestCount(ctx context.Context) (int, error) {
	return s.Client.RequestUsage.Query().Where(requestCompletenessPred(CompletenessUnknown)).Count(ctx)
}

func requestCompletenessPred(value Completeness) predicate.RequestUsage {
	return requestusage.RequestCompletenessEQ(string(value))
}

var ErrRequestNotFound = errors.New("usage request not found")

func (f Filter) Validate() error {
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return ErrInvalidBatch
	}
	switch f.Status {
	case "", RequestStatusSuccess, RequestStatusError, RequestStatusBlocked:
	default:
		return ErrInvalidBatch
	}
	switch f.Completeness {
	case "", CompletenessComplete, CompletenessPartial, CompletenessUnknown:
	default:
		return ErrInvalidBatch
	}
	switch f.ResourceKind {
	case "", ResourceKindProvider, ResourceKindModel, ResourceKindImage, ResourceKindTTS, ResourceKindASR, ResourceKindMCP:
	default:
		return ErrInvalidBatch
	}
	if f.UserID != "" && platformid.Validate(platformid.User, f.UserID) != nil {
		return ErrInvalidBatch
	}
	if f.UpstreamID != "" && platformid.Validate(platformid.Upstream, f.UpstreamID) != nil {
		return ErrInvalidBatch
	}
	if f.ClientProtocol != "" && !validClientProtocol(f.ClientProtocol) {
		return ErrInvalidBatch
	}
	return nil
}

func validClientProtocol(value string) bool {
	switch value {
	case "OPENAI_CHAT_COMPLETIONS", "OPENAI_RESPONSES", "GOOGLE_GENERATE_CONTENT", "ANTHROPIC_MESSAGES",
		"OPENAI_IMAGES_GENERATIONS", "DASHSCOPE_MULTIMODAL_GENERATION",
		"OPENAI_AUDIO_SPEECH", "GEMINI_GENERATE_CONTENT_TTS", "MIMO_CHAT_COMPLETIONS_TTS",
		"OPENAI_AUDIO_TRANSCRIPTIONS", "DASHSCOPE_HTTP_ASR", "OPENAI_REALTIME_TRANSCRIPTION",
		"DASHSCOPE_REALTIME_ASR", "MCP_STREAMABLE_HTTP":
		return true
	default:
		return false
	}
}
