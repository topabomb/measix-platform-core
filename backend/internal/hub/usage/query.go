package usage

import (
	"context"
	"errors"
	"math/big"
	"measix/platform/pkg/platformid"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/predicate"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
)

type Summary struct {
	From                  time.Time
	To                    time.Time
	RequestCount          int
	ForwardedRequestCount int
	RequestBytes          int64
	ResponseBytes         int64
	Meters                []MeterSummary
	Cost                  CostSummary
}

type MeterSummary struct {
	Meter      string
	Quantity   string
	Confidence Completeness
}

type CostSummary struct {
	State    CostState
	Amount   string
	Currency string
}

type ResourceKind string

const (
	ResourceKindProvider ResourceKind = "PROVIDER"
	ResourceKindModel    ResourceKind = "MODEL"
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
	After        string
	From         *time.Time
	To           *time.Time
	UserID       string
	ResourceID   string
	ResourceKind ResourceKind
	UpstreamID   string
	Status       RequestStatus
	Completeness Completeness
}

type RequestView struct {
	RequestID          string
	InteractionID      *string
	DeploymentID       string
	UserID             string
	DeviceID           *string
	ResourceID         string
	RuntimeRouteID     string
	UpstreamID         string
	ManagedGeneration  int
	ControlRevision    int
	StartedAt          time.Time
	CompletedAt        time.Time
	Forwarded          bool
	HTTPStatus         int
	UpstreamHTTPStatus *int
	RequestBytes       int
	ResponseBytes      int
	DurationMs         int
	ErrorClass         *string
}

func requestView(row *ent.RequestUsage) RequestView {
	var upstreamStatus *int
	if row.UpstreamHTTPStatus != nil {
		value := int(*row.UpstreamHTTPStatus)
		upstreamStatus = &value
	}
	return RequestView{
		RequestID: row.RequestID, InteractionID: row.InteractionID, DeploymentID: row.DeploymentID, UserID: row.UserID, DeviceID: row.DeviceID,
		ResourceID: row.ResourceID, RuntimeRouteID: row.RuntimeRouteID, UpstreamID: row.UpstreamID, ManagedGeneration: int(row.ManagedGeneration), ControlRevision: int(row.ControlRevision),
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, Forwarded: row.Forwarded, HTTPStatus: row.HTTPStatus, UpstreamHTTPStatus: upstreamStatus,
		RequestBytes: int(row.RequestBytes), ResponseBytes: int(row.ResponseBytes), DurationMs: int(row.DurationMs), ErrorClass: row.ErrorClass,
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
	return views, nil
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
	return requestView(row), nil
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
	requests, err := tx.RequestUsage.Query().Where(requestusage.And(preds...)).All(ctx)
	if err != nil {
		return Summary{}, err
	}
	result := Summary{From: from, To: to, Cost: CostSummary{State: CostUnknown}}
	for _, row := range requests {
		result.RequestCount++
		if row.Forwarded {
			result.ForwardedRequestCount++
		}
		result.RequestBytes += row.RequestBytes
		result.ResponseBytes += row.ResponseBytes
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
		matched.Where(sql.ColumnsEQ(table.C(requestusage.FieldRequestID), sel.C(semanticusage.FieldRequestID)))
		condition := sql.Exists(matched)
		if filter.UserID == "" && filter.Status == "" {
			anyRequest := sql.Select(table.C(requestusage.FieldRequestID)).From(table).
				Where(sql.ColumnsEQ(table.C(requestusage.FieldRequestID), sel.C(semanticusage.FieldRequestID)))
			unlinked := sql.Not(sql.Exists(anyRequest))
			if filter.Completeness != "" {
				unlinked = sql.And(unlinked, sql.EQ(sel.C(semanticusage.FieldCompleteness), string(filter.Completeness)))
			}
			condition = sql.Or(condition, unlinked)
		}
		sel.Where(condition)
	})
	if filter.ResourceID != "" {
		semanticQ = semanticQ.Where(semanticusage.ResourceIDEQ(filter.ResourceID))
	}
	if filter.UpstreamID != "" {
		semanticQ = semanticQ.Where(semanticusage.UpstreamIDEQ(filter.UpstreamID))
	}
	if filter.ResourceKind != "" {
		semanticQ = semanticQ.Where(semanticusage.ResourceIDHasPrefix(resourceKindPrefix(filter.ResourceKind)))
	}
	semantic, err := semanticQ.All(ctx)
	if err != nil {
		return Summary{}, err
	}
	type accumulator struct {
		quantity     *big.Rat
		completeness Completeness
	}
	meters := map[string]accumulator{}
	costs := map[string]*big.Rat{}
	costState := CostKnown
	for _, row := range semantic {
		q, ok := decimalRat(row.QuantityDecimal)
		if !ok {
			continue
		}
		acc := meters[row.Meter]
		if acc.quantity == nil {
			acc.quantity = new(big.Rat)
			acc.completeness = CompletenessComplete
		}
		acc.quantity.Add(acc.quantity, q)
		rowCompleteness := Completeness(row.Completeness)
		if rowCompleteness == CompletenessUnknown {
			acc.completeness = CompletenessUnknown
		} else if rowCompleteness == CompletenessPartial && acc.completeness == CompletenessComplete {
			acc.completeness = CompletenessPartial
		}
		meters[row.Meter] = acc
		if row.ProviderCost == nil || row.Currency == nil {
			costState = CostPartial
		}
		if row.ProviderCost != nil && row.Currency != nil {
			cost, ok := decimalRat(*row.ProviderCost)
			if ok {
				if costs[*row.Currency] == nil {
					costs[*row.Currency] = new(big.Rat)
				}
				costs[*row.Currency].Add(costs[*row.Currency], cost)
				if rowCompleteness != CompletenessComplete {
					costState = CostPartial
				}
			}
		}
	}
	meterNames := make([]string, 0, len(meters))
	for meter := range meters {
		meterNames = append(meterNames, meter)
	}
	sort.Strings(meterNames)
	for _, meter := range meterNames {
		acc := meters[meter]
		quantity, err := exactDecimal(acc.quantity)
		if err != nil {
			return Summary{}, err
		}
		result.Meters = append(result.Meters, MeterSummary{Meter: meter, Quantity: quantity, Confidence: acc.completeness})
	}
	if len(costs) == 1 {
		for currency, total := range costs {
			amount, err := exactDecimal(total)
			if err != nil {
				return Summary{}, err
			}
			result.Cost = CostSummary{State: costState, Amount: amount, Currency: currency}
		}
	} else if len(costs) > 1 {
		result.Cost = CostSummary{State: CostUnknown}
	}
	return result, nil
}

var ErrPricingRevisionConflict = errors.New("pricing revision conflict")

func requestCompletenessPred(value Completeness) predicate.RequestUsage {
	return func(sel *sql.Selector) {
		exists := func(confidence string, negate bool) *sql.Predicate {
			table := sql.Table(semanticusage.Table)
			q := sql.Select(table.C(semanticusage.FieldID)).From(table).Where(sql.ColumnsEQ(table.C(semanticusage.FieldRequestID), sel.C(requestusage.FieldRequestID)))
			if confidence != "" {
				if negate {
					q.Where(sql.NEQ(table.C(semanticusage.FieldCompleteness), confidence))
				} else {
					q.Where(sql.EQ(table.C(semanticusage.FieldCompleteness), confidence))
				}
			}
			return sql.Exists(q)
		}
		switch value {
		case CompletenessComplete:
			sel.Where(sql.And(exists("", false), sql.Not(exists(string(CompletenessComplete), true))))
		case CompletenessPartial:
			sel.Where(sql.And(exists(string(CompletenessPartial), false), sql.Not(exists(string(CompletenessUnknown), false))))
		default:
			sel.Where(sql.Or(sql.Not(exists("", false)), exists(string(CompletenessUnknown), false)))
		}
	}
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
	case "", ResourceKindProvider, ResourceKindModel, ResourceKindTTS, ResourceKindASR, ResourceKindMCP:
	default:
		return ErrInvalidBatch
	}
	if f.UserID != "" && platformid.Validate(platformid.User, f.UserID) != nil {
		return ErrInvalidBatch
	}
	if f.UpstreamID != "" && platformid.Validate(platformid.Upstream, f.UpstreamID) != nil {
		return ErrInvalidBatch
	}
	return nil
}
