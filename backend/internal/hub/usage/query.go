package usage

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"time"

	"measix/platform/ent"
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

func (s *Service) ListRequests(ctx context.Context, limit int) ([]RequestView, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Client.RequestUsage.Query().Order(ent.Desc(requestusage.FieldCompletedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]RequestView, 0, len(rows))
	for _, row := range rows {
		views = append(views, requestView(row))
	}
	return views, nil
}

func (s *Service) GetRequest(ctx context.Context, requestID string) (RequestView, error) {
	row, err := s.Client.RequestUsage.Query().Where(requestusage.RequestIDEQ(requestID)).Only(ctx)
	if ent.IsNotFound(err) {
		return RequestView{}, ErrInvalidBatch
	}
	if err != nil {
		return RequestView{}, err
	}
	return requestView(row), nil
}

func (s *Service) Summary(ctx context.Context, from, to time.Time) (Summary, error) {
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return Summary{}, ErrInvalidBatch
	}
	requests, err := s.Client.RequestUsage.Query().Where(
		requestusage.CompletedAtGTE(from.UTC()),
		requestusage.CompletedAtLT(to.UTC()),
	).All(ctx)
	if err != nil {
		return Summary{}, err
	}
	result := Summary{From: from.UTC(), To: to.UTC(), Cost: CostSummary{State: CostUnknown}}
	for _, row := range requests {
		result.RequestCount++
		if row.Forwarded {
			result.ForwardedRequestCount++
		}
		result.RequestBytes += row.RequestBytes
		result.ResponseBytes += row.ResponseBytes
	}

	semantic, err := s.Client.SemanticUsage.Query().Where(
		semanticusage.OccurredAtGTE(from.UTC()),
		semanticusage.OccurredAtLT(to.UTC()),
	).All(ctx)
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
