package usage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/semanticusage"
	"measix/platform/ent/user"
	"measix/platform/internal/hub/budget"
	"measix/platform/pkg/platformid"
)

const maxTrendDays = 92

type TrendPoint struct {
	Date                  time.Time
	RequestCount          int
	ForwardedRequestCount int
	Meters                []MeterSummary
	Cost                  CostSummary
}

type Trend struct {
	From, To time.Time
	Timezone string
	Points   []TrendPoint
}

type DistributionItem struct {
	ResourceKind, ClientProtocol string
	ResourceID, ResourceName     string
	RequestCount                 int
	Meters                       []MeterSummary
	Cost                         CostSummary
}

type Distribution struct {
	From, To time.Time
	Items    []DistributionItem
}

type UserUsage struct {
	UserID, DisplayName string
	RequestCount        int
	Meters              []MeterSummary
	UsageMeters         CapabilityMeters
	Budget              []budget.EffectiveState
}

type CapabilityMeters map[budget.Capability][]MeterSummary

type UserUsagePage struct {
	Items      []UserUsage
	NextCursor string
}

type UserBudgetFilter string

const (
	UserBudgetExhausted      UserBudgetFilter = "EXHAUSTED"
	UserBudgetNearLimit      UserBudgetFilter = "NEAR_LIMIT"
	UserBudgetReconciliation UserBudgetFilter = "PENDING_RECONCILIATION"
)

func (s *Service) Trend(ctx context.Context, filter Filter, location *time.Location) (Trend, error) {
	if location == nil || filter.UserID != "" && platformid.Validate(platformid.User, filter.UserID) != nil {
		return Trend{}, ErrInvalidBatch
	}
	from, to, err := boundedAnalyticsWindow(s.Now(), filter.From, filter.To)
	if err != nil {
		return Trend{}, err
	}
	result := Trend{From: from, To: to, Timezone: location.String(), Points: []TrendPoint{}}
	type trendWindow struct {
		Date     time.Time
		From, To time.Time
	}
	windows := make([]trendWindow, 0, maxTrendDays+1)
	localStart := from.In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	for day.Before(to.In(location)) {
		next := day.AddDate(0, 0, 1)
		windowFrom, windowTo := day.UTC(), next.UTC()
		if windowFrom.Before(from) {
			windowFrom = from
		}
		if windowTo.After(to) {
			windowTo = to
		}
		windows = append(windows, trendWindow{Date: day, From: windowFrom, To: windowTo})
		result.Points = append(result.Points, TrendPoint{Date: day, Meters: []MeterSummary{}})
		day = next
	}
	filter.From, filter.To = &from, &to
	if err := filter.Validate(); err != nil {
		return Trend{}, err
	}
	if len(windows) == 0 {
		return result, nil
	}
	bucketExpression := func(column string) string {
		var expression strings.Builder
		expression.WriteString("CASE")
		for index, window := range windows {
			fmt.Fprintf(&expression, " WHEN julianday(%s) >= julianday('%s') AND julianday(%s) < julianday('%s') THEN %d", column, window.From.Format(time.RFC3339Nano), column, window.To.Format(time.RFC3339Nano), index)
		}
		expression.WriteString(" ELSE -1 END")
		return expression.String()
	}

	type requestAggregate struct {
		Bucket         int `json:"bucket"`
		Count          int `json:"count"`
		ForwardedCount int `json:"forwarded_count"`
	}
	var requestRows []requestAggregate
	if err := s.Client.RequestUsage.Query().Where(requestusage.And(requestFilterPreds(filter)...)).
		Aggregate(
			func(selector *sql.Selector) string {
				bucket := bucketExpression(selector.C(requestusage.FieldCompletedAt))
				selector.GroupBy(bucket)
				return sql.As(bucket, "bucket")
			},
			ent.Count(),
			func(selector *sql.Selector) string {
				column := selector.C(requestusage.FieldForwarded)
				return sql.As("SUM(CASE WHEN "+column+" THEN 1 ELSE 0 END)", "forwarded_count")
			},
		).Scan(ctx, &requestRows); err != nil {
		return Trend{}, err
	}
	for _, row := range requestRows {
		if row.Bucket >= 0 && row.Bucket < len(result.Points) {
			result.Points[row.Bucket].RequestCount = row.Count
			result.Points[row.Bucket].ForwardedRequestCount = row.ForwardedCount
		}
	}

	type semanticAggregate struct {
		Bucket        int    `json:"bucket"`
		Meter         string `json:"meter"`
		QuantityUnits int64  `json:"quantity_units"`
		PartialCount  int    `json:"partial_count"`
		UnknownCount  int    `json:"unknown_count"`
	}
	requests := sql.Select().From(sql.Table(requestusage.Table))
	predicates := requestFilterPreds(filter)
	var semanticRows []semanticAggregate
	query := s.Client.SemanticUsage.Query().Where(func(selector *sql.Selector) {
		for _, predicate := range predicates {
			predicate(requests)
		}
		selector.Join(requests).On(selector.C(semanticusage.FieldRequestID), requests.C(requestusage.FieldRequestID))
		selector.Where(sql.ColumnsEQ(selector.C(semanticusage.FieldSettlementRevision), requests.C(requestusage.FieldSettlementRevision)))
		selector.GroupBy(bucketExpression(requests.C(requestusage.FieldCompletedAt)))
	})
	if err := query.GroupBy(semanticusage.FieldMeter).Aggregate(
		func(*sql.Selector) string {
			return sql.As(bucketExpression(requests.C(requestusage.FieldCompletedAt)), "bucket")
		},
		quantityUnitsAggregate,
		partialCountAggregate,
		unknownCountAggregate,
	).Scan(ctx, &semanticRows); err != nil {
		return Trend{}, err
	}
	for _, row := range semanticRows {
		if row.Bucket >= 0 && row.Bucket < len(result.Points) {
			result.Points[row.Bucket].Meters = append(result.Points[row.Bucket].Meters, meterSummary(row.Meter, row.QuantityUnits, row.PartialCount, row.UnknownCount))
		}
	}
	for index := range result.Points {
		sort.Slice(result.Points[index].Meters, func(i, j int) bool {
			return result.Points[index].Meters[i].Meter < result.Points[index].Meters[j].Meter
		})
	}
	byDay := make(map[string]*costAccumulator, len(result.Points))
	if err := s.analyzeCosts(ctx, filter, func(view RequestView, priced CostBreakdown) error {
		day := view.CompletedAt.In(location).Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = &costAccumulator{}
		}
		return byDay[day].add(priced)
	}); err != nil {
		return Trend{}, err
	}
	for i := range result.Points {
		bucket := byDay[result.Points[i].Date.Format("2006-01-02")]
		if bucket == nil {
			bucket = &costAccumulator{}
		}
		cost, err := bucket.summary()
		if err != nil {
			return Trend{}, err
		}
		result.Points[i].Cost = cost
	}
	return result, nil
}

func (s *Service) Distribution(ctx context.Context, filter Filter) (Distribution, error) {
	from, to, err := boundedAnalyticsWindow(s.Now(), filter.From, filter.To)
	if err != nil {
		return Distribution{}, err
	}
	filter.From, filter.To = &from, &to
	if err := filter.Validate(); err != nil {
		return Distribution{}, err
	}
	type group struct {
		ResourceKind      string `json:"resource_kind"`
		ClientProtocol    string `json:"client_protocol"`
		ResourceID        string `json:"resource_id"`
		ManagedGeneration int64  `json:"managed_generation"`
		Count             int    `json:"count"`
	}
	var groups []group
	query := s.Client.RequestUsage.Query().Where(requestusage.And(requestFilterPreds(filter)...))
	if err := query.GroupBy(requestusage.FieldResourceKind, requestusage.FieldClientProtocol, requestusage.FieldResourceID).
		Aggregate(ent.Count(), func(selector *sql.Selector) string {
			return sql.As("MAX("+selector.C(requestusage.FieldManagedGeneration)+")", "managed_generation")
		}).Scan(ctx, &groups); err != nil {
		return Distribution{}, err
	}
	type semanticAggregate struct {
		ResourceKind   string `json:"resource_kind"`
		ClientProtocol string `json:"client_protocol"`
		ResourceID     string `json:"resource_id"`
		Meter          string `json:"meter"`
		QuantityUnits  int64  `json:"quantity_units"`
		PartialCount   int    `json:"partial_count"`
		UnknownCount   int    `json:"unknown_count"`
	}
	requests := sql.Select().From(sql.Table(requestusage.Table))
	predicates := requestFilterPreds(filter)
	var semanticRows []semanticAggregate
	semanticQuery := s.Client.SemanticUsage.Query().Where(func(selector *sql.Selector) {
		for _, predicate := range predicates {
			predicate(requests)
		}
		selector.Join(requests).On(selector.C(semanticusage.FieldRequestID), requests.C(requestusage.FieldRequestID))
		selector.Where(sql.ColumnsEQ(selector.C(semanticusage.FieldSettlementRevision), requests.C(requestusage.FieldSettlementRevision)))
		selector.GroupBy(requests.C(requestusage.FieldResourceKind), requests.C(requestusage.FieldClientProtocol), requests.C(requestusage.FieldResourceID))
	})
	if err := semanticQuery.GroupBy(semanticusage.FieldMeter).Aggregate(
		func(*sql.Selector) string { return sql.As(requests.C(requestusage.FieldResourceKind), "resource_kind") },
		func(*sql.Selector) string {
			return sql.As(requests.C(requestusage.FieldClientProtocol), "client_protocol")
		},
		func(*sql.Selector) string { return sql.As(requests.C(requestusage.FieldResourceID), "resource_id") },
		quantityUnitsAggregate,
		partialCountAggregate,
		unknownCountAggregate,
	).Scan(ctx, &semanticRows); err != nil {
		return Distribution{}, err
	}
	type distributionKey struct{ resourceKind, protocol, resourceID string }
	metersByGroup := make(map[distributionKey][]MeterSummary, len(groups))
	for _, row := range semanticRows {
		key := distributionKey{row.ResourceKind, row.ClientProtocol, row.ResourceID}
		metersByGroup[key] = append(metersByGroup[key], meterSummary(row.Meter, row.QuantityUnits, row.PartialCount, row.UnknownCount))
	}
	views := make([]RequestView, len(groups))
	for index, current := range groups {
		views[index] = RequestView{ResourceID: current.ResourceID, ManagedGeneration: int(current.ManagedGeneration)}
	}
	if err := s.resourceNames(ctx, views); err != nil {
		return Distribution{}, err
	}
	result := Distribution{From: from, To: to, Items: []DistributionItem{}}
	for index, current := range groups {
		key := distributionKey{current.ResourceKind, current.ClientProtocol, current.ResourceID}
		meters := metersByGroup[key]
		if meters == nil {
			meters = []MeterSummary{}
		}
		sort.Slice(meters, func(i, j int) bool { return meters[i].Meter < meters[j].Meter })
		result.Items = append(result.Items, DistributionItem{
			ResourceKind: current.ResourceKind, ClientProtocol: current.ClientProtocol, ResourceID: current.ResourceID,
			ResourceName: views[index].ResourceDisplayName, RequestCount: current.Count, Meters: meters,
		})
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].RequestCount != result.Items[j].RequestCount {
			return result.Items[i].RequestCount > result.Items[j].RequestCount
		}
		return result.Items[i].ResourceID < result.Items[j].ResourceID
	})
	byResource := make(map[distributionKey]*costAccumulator, len(result.Items))
	if err := s.analyzeCosts(ctx, filter, func(view RequestView, priced CostBreakdown) error {
		key := distributionKey{string(view.ResourceKind), view.ClientProtocol, view.ResourceID}
		if byResource[key] == nil {
			byResource[key] = &costAccumulator{}
		}
		return byResource[key].add(priced)
	}); err != nil {
		return Distribution{}, err
	}
	for i := range result.Items {
		item := &result.Items[i]
		bucket := byResource[distributionKey{item.ResourceKind, item.ClientProtocol, item.ResourceID}]
		if bucket == nil {
			bucket = &costAccumulator{}
		}
		cost, err := bucket.summary()
		if err != nil {
			return Distribution{}, err
		}
		item.Cost = cost
	}
	return result, nil
}

func (s *Service) ListUsers(ctx context.Context, filter Filter, budgetService *budget.Service, budgetFilter UserBudgetFilter, pageSize int, cursor string) (UserUsagePage, error) {
	if budgetService == nil || pageSize < 1 || pageSize > 100 || cursor != "" && platformid.Validate(platformid.User, cursor) != nil || !validUserBudgetFilter(budgetFilter) {
		return UserUsagePage{}, ErrInvalidBatch
	}
	from, to, err := boundedAnalyticsWindow(s.Now(), filter.From, filter.To)
	if err != nil {
		return UserUsagePage{}, err
	}
	filter.From, filter.To = &from, &to
	if err := filter.Validate(); err != nil {
		return UserUsagePage{}, err
	}
	type group struct {
		UserID string `json:"user_id"`
		Count  int    `json:"count"`
	}
	selected := make([]group, 0, pageSize+1)
	statesByUser := make(map[string][]budget.EffectiveState)
	scanCursor := cursor
	for len(selected) < pageSize+1 {
		predicates := requestFilterPreds(filter)
		if scanCursor != "" {
			predicates = append(predicates, requestusage.UserIDGT(scanCursor))
		}
		var groups []group
		if err := s.Client.RequestUsage.Query().
			Where(requestusage.And(predicates...)).
			Order(ent.Asc(requestusage.FieldUserID)).
			Limit(100).
			GroupBy(requestusage.FieldUserID).
			Aggregate(ent.Count()).
			Scan(ctx, &groups); err != nil {
			return UserUsagePage{}, err
		}
		if len(groups) == 0 {
			break
		}
		batchIDs := make([]string, 0, len(groups))
		for _, value := range groups {
			batchIDs = append(batchIDs, value.UserID)
		}
		batchStates, err := budgetService.UserStatesBatch(ctx, batchIDs)
		if err != nil {
			return UserUsagePage{}, err
		}
		for _, value := range groups {
			states := batchStates[value.UserID]
			if userBudgetMatches(states, budgetFilter) {
				selected = append(selected, value)
				statesByUser[value.UserID] = states
				if len(selected) == pageSize+1 {
					break
				}
			}
		}
		scanCursor = groups[len(groups)-1].UserID
		if len(groups) < 100 {
			break
		}
	}
	visible := selected[:min(pageSize, len(selected))]
	userIDs := make([]string, 0, len(visible))
	for _, value := range visible {
		userIDs = append(userIDs, value.UserID)
	}
	names := make(map[string]string, len(userIDs))
	if len(userIDs) > 0 {
		rows, err := s.Client.User.Query().Where(user.IDIn(userIDs...)).All(ctx)
		if err != nil {
			return UserUsagePage{}, err
		}
		for _, row := range rows {
			names[row.ID] = row.DisplayName
		}
	}
	metersByUser, err := s.metersByUsers(ctx, filter, userIDs)
	if err != nil {
		return UserUsagePage{}, err
	}
	usageMetersByUser, err := s.usageMetersByUsers(ctx, userIDs)
	if err != nil {
		return UserUsagePage{}, err
	}
	page := UserUsagePage{Items: []UserUsage{}}
	for _, value := range visible {
		meters := metersByUser[value.UserID]
		if meters == nil {
			meters = []MeterSummary{}
		}
		page.Items = append(page.Items, UserUsage{UserID: value.UserID, DisplayName: names[value.UserID], RequestCount: value.Count, Meters: meters, UsageMeters: usageMetersByUser[value.UserID], Budget: statesByUser[value.UserID]})
	}
	if len(selected) > pageSize && len(visible) > 0 {
		page.NextCursor = visible[len(visible)-1].UserID
	}
	return page, nil
}

func validUserBudgetFilter(filter UserBudgetFilter) bool {
	switch filter {
	case "", UserBudgetExhausted, UserBudgetNearLimit, UserBudgetReconciliation:
		return true
	default:
		return false
	}
}

func userBudgetMatches(states []budget.EffectiveState, filter UserBudgetFilter) bool {
	if filter == "" {
		return true
	}
	health := UserBudgetFilter("")
	for _, state := range states {
		if state.Status == budget.StatusReconciliation {
			health = UserBudgetReconciliation
			break
		}
		if state.Status == budget.StatusExhausted {
			health = UserBudgetExhausted
			continue
		}
		if health == "" && state.Budget.Mode == budget.ModeLimited {
			for _, limit := range state.Limits {
				// Near-limit is an Admin presentation category, not an admission
				// threshold. Eighty percent is exact integer arithmetic and only
				// applies while the limit is not already exhausted.
				if limit.Limit > 0 && limit.Used+limit.Reserved < limit.Limit &&
					(limit.Used+limit.Reserved)*5 >= limit.Limit*4 {
					health = UserBudgetNearLimit
					break
				}
			}
		}
	}
	return health == filter
}

// metersByUsers aggregates the current settlement revision for every user in
// one page. The join applies the same request filters as the surrounding user
// query, so the Admin projection remains exact without one Summary call per
// user.
func (s *Service) metersByUsers(ctx context.Context, filter Filter, userIDs []string) (map[string][]MeterSummary, error) {
	result := make(map[string][]MeterSummary, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	requests := sql.Select().From(sql.Table(requestusage.Table))
	predicates := append(requestFilterPreds(filter), requestusage.UserIDIn(userIDs...))
	type aggregate struct {
		UserID        string `json:"user_id"`
		Meter         string `json:"meter"`
		QuantityUnits int64  `json:"quantity_units"`
		PartialCount  int    `json:"partial_count"`
		UnknownCount  int    `json:"unknown_count"`
	}
	var rows []aggregate
	query := s.Client.SemanticUsage.Query().Where(func(selector *sql.Selector) {
		for _, predicate := range predicates {
			predicate(requests)
		}
		selector.Join(requests).On(
			selector.C(semanticusage.FieldRequestID),
			requests.C(requestusage.FieldRequestID),
		)
		selector.Where(sql.ColumnsEQ(
			selector.C(semanticusage.FieldSettlementRevision),
			requests.C(requestusage.FieldSettlementRevision),
		))
		selector.GroupBy(requests.C(requestusage.FieldUserID))
	})
	if err := query.GroupBy(semanticusage.FieldMeter).Aggregate(
		func(*sql.Selector) string {
			return sql.As(requests.C(requestusage.FieldUserID), "user_id")
		},
		quantityUnitsAggregate,
		partialCountAggregate,
		unknownCountAggregate,
	).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], meterSummary(row.Meter, row.QuantityUnits, row.PartialCount, row.UnknownCount))
	}
	for userID := range result {
		sort.Slice(result[userID], func(i, j int) bool { return result[userID][i].Meter < result[userID][j].Meter })
	}
	return result, nil
}

func (s *Service) UsageMetersByCapability(ctx context.Context, userID string) (CapabilityMeters, error) {
	if platformid.Validate(platformid.User, userID) != nil {
		return nil, ErrInvalidBatch
	}
	byUser, err := s.usageMetersByUsers(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	result := byUser[userID]
	if result == nil {
		result = CapabilityMeters{}
	}
	return result, nil
}

// usageMetersByUsers is the retained, all-time usage projection. It is
// deliberately separate from EffectiveState.Limits, whose used/reserved
// values only describe the currently active budget windows.
func (s *Service) usageMetersByUsers(ctx context.Context, userIDs []string) (map[string]CapabilityMeters, error) {
	result := make(map[string]CapabilityMeters, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	requests := sql.Select().From(sql.Table(requestusage.Table))
	type aggregate struct {
		UserID        string `json:"user_id"`
		ResourceKind  string `json:"resource_kind"`
		Meter         string `json:"meter"`
		QuantityUnits int64  `json:"quantity_units"`
		PartialCount  int    `json:"partial_count"`
		UnknownCount  int    `json:"unknown_count"`
	}
	var rows []aggregate
	query := s.Client.SemanticUsage.Query().Where(func(selector *sql.Selector) {
		requestusage.UserIDIn(userIDs...)(requests)
		selector.Join(requests).On(selector.C(semanticusage.FieldRequestID), requests.C(requestusage.FieldRequestID))
		selector.Where(sql.ColumnsEQ(selector.C(semanticusage.FieldSettlementRevision), requests.C(requestusage.FieldSettlementRevision)))
		selector.GroupBy(requests.C(requestusage.FieldUserID), requests.C(requestusage.FieldResourceKind))
	})
	if err := query.GroupBy(semanticusage.FieldMeter).Aggregate(
		func(*sql.Selector) string { return sql.As(requests.C(requestusage.FieldUserID), "user_id") },
		func(*sql.Selector) string { return sql.As(requests.C(requestusage.FieldResourceKind), "resource_kind") },
		quantityUnitsAggregate,
		partialCountAggregate,
		unknownCountAggregate,
	).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		capability := budget.Capability(row.ResourceKind)
		if capability != budget.CapabilityModel && capability != budget.CapabilityTTS && capability != budget.CapabilityASR && capability != budget.CapabilityMCP {
			continue
		}
		if result[row.UserID] == nil {
			result[row.UserID] = CapabilityMeters{}
		}
		result[row.UserID][capability] = append(result[row.UserID][capability], meterSummary(row.Meter, row.QuantityUnits, row.PartialCount, row.UnknownCount))
	}
	for _, capabilities := range result {
		for capability := range capabilities {
			sort.Slice(capabilities[capability], func(i, j int) bool { return capabilities[capability][i].Meter < capabilities[capability][j].Meter })
		}
	}
	return result, nil
}

func quantityUnitsAggregate(selector *sql.Selector) string {
	return sql.As(sql.Sum(selector.C(semanticusage.FieldQuantityUnits)), "quantity_units")
}

func partialCountAggregate(selector *sql.Selector) string {
	column := selector.C(semanticusage.FieldCompleteness)
	return sql.As("SUM(CASE WHEN "+column+" = 'PARTIAL' THEN 1 ELSE 0 END)", "partial_count")
}

func unknownCountAggregate(selector *sql.Selector) string {
	column := selector.C(semanticusage.FieldCompleteness)
	return sql.As("SUM(CASE WHEN "+column+" = 'UNKNOWN' THEN 1 ELSE 0 END)", "unknown_count")
}

func meterSummary(meter string, quantityUnits int64, partialCount, unknownCount int) MeterSummary {
	confidence := CompletenessComplete
	if unknownCount > 0 {
		confidence = CompletenessUnknown
	} else if partialCount > 0 {
		confidence = CompletenessPartial
	}
	quantity := fmt.Sprintf("%d", quantityUnits)
	if meter == "AUDIO_SECONDS" {
		seconds, millis := quantityUnits/1000, quantityUnits%1000
		quantity = fmt.Sprintf("%d", seconds)
		if millis != 0 {
			quantity = strings.TrimRight(fmt.Sprintf("%d.%03d", seconds, millis), "0")
		}
	}
	return MeterSummary{Meter: meter, Quantity: quantity, Confidence: confidence}
}

func boundedAnalyticsWindow(now time.Time, from, to *time.Time) (time.Time, time.Time, error) {
	// SQLite's julianday aggregation has millisecond precision. Keep the
	// implicit live-window upper bound one millisecond past Now so a request
	// completed exactly at Now remains in the final bucket.
	windowTo := now.UTC().Add(time.Millisecond)
	if to != nil {
		windowTo = to.UTC()
	}
	windowFrom := windowTo.AddDate(0, 0, -30)
	if from != nil {
		windowFrom = from.UTC()
	}
	if !windowFrom.Before(windowTo) || windowTo.Sub(windowFrom) > maxTrendDays*24*time.Hour+time.Hour {
		return time.Time{}, time.Time{}, ErrInvalidBatch
	}
	return windowFrom, windowTo, nil
}
