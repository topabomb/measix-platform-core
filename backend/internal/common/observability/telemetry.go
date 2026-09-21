package observability

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

const bucketCount = 60

var durationBounds = [...]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

type Bucket struct {
	Minute                 time.Time `json:"minute"`
	RequestCount           int64     `json:"requestCount"`
	SuccessCount           int64     `json:"successCount"`
	ClientErrorCount       int64     `json:"clientErrorCount"`
	ServerErrorCount       int64     `json:"serverErrorCount"`
	RejectedCount          int64     `json:"rejectedCount"`
	TimeoutCount           int64     `json:"timeoutCount"`
	CancelledCount         int64     `json:"cancelledCount"`
	UpstreamErrorCount     int64     `json:"upstreamErrorCount,omitempty"`
	BudgetDeniedCount      int64     `json:"budgetDeniedCount,omitempty"`
	ActivationFailureCount int64     `json:"activationFailureCount,omitempty"`
	ReconcileFailureCount  int64     `json:"reconcileFailureCount,omitempty"`
	DurationP95Ms          int64     `json:"durationP95Ms"`
	InFlightPeak           int64     `json:"inFlightPeak"`
	histogram              [len(durationBounds) + 1]int64
}

type Snapshot struct {
	StartedAt time.Time `json:"startedAt"`
	InFlight  int64     `json:"inFlight"`
	Summary   Bucket    `json:"summary"`
	Buckets   []Bucket  `json:"buckets"`
}

type Outcome struct {
	Status        int
	Duration      time.Duration
	Rejected      bool
	Timeout       bool
	Cancelled     bool
	UpstreamError bool
	BudgetDenied  bool
}

type Recorder struct {
	mu        sync.Mutex
	startedAt time.Time
	now       func() time.Time
	buckets   [bucketCount]Bucket
	inFlight  atomic.Int64
}

func NewRecorder(now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{startedAt: now().UTC(), now: now}
}

func (r *Recorder) Begin() func(Outcome) {
	current := r.inFlight.Add(1)
	r.mu.Lock()
	bucket := r.bucketLocked(r.now())
	if current > bucket.InFlightPeak {
		bucket.InFlightPeak = current
	}
	r.mu.Unlock()
	return func(outcome Outcome) {
		r.inFlight.Add(-1)
		r.Record(outcome)
	}
}

func (r *Recorder) Record(outcome Outcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket := r.bucketLocked(r.now())
	bucket.RequestCount++
	switch {
	case outcome.Status >= 500:
		bucket.ServerErrorCount++
	case outcome.Status >= 400:
		bucket.ClientErrorCount++
	default:
		bucket.SuccessCount++
	}
	if outcome.Rejected {
		bucket.RejectedCount++
	}
	if outcome.Timeout {
		bucket.TimeoutCount++
	}
	if outcome.Cancelled {
		bucket.CancelledCount++
	}
	if outcome.UpstreamError {
		bucket.UpstreamErrorCount++
	}
	if outcome.BudgetDenied {
		bucket.BudgetDeniedCount++
	}
	millis := outcome.Duration.Milliseconds()
	index := sort.Search(len(durationBounds), func(i int) bool { return millis <= durationBounds[i] })
	bucket.histogram[index]++
	bucket.DurationP95Ms = percentile95(bucket.histogram)
}

func (r *Recorder) IncrementActivationFailure() {
	r.increment(func(b *Bucket) { b.ActivationFailureCount++ })
}
func (r *Recorder) IncrementReconcileFailure() {
	r.increment(func(b *Bucket) { b.ReconcileFailureCount++ })
}

func (r *Recorder) increment(change func(*Bucket)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(r.bucketLocked(r.now()))
}

func (r *Recorder) Snapshot(window time.Duration) Snapshot {
	minutes := int(window / time.Minute)
	if minutes != 15 && minutes != 60 {
		minutes = 60
	}
	nowMinute := r.now().UTC().Truncate(time.Minute)
	cutoff := nowMinute.Add(-time.Duration(minutes-1) * time.Minute)
	r.mu.Lock()
	buckets := make([]Bucket, 0, minutes)
	var summary Bucket
	for i := range r.buckets {
		bucket := r.buckets[i]
		if !bucket.Minute.IsZero() && !bucket.Minute.Before(cutoff) && !bucket.Minute.After(nowMinute) {
			summary.RequestCount += bucket.RequestCount
			summary.SuccessCount += bucket.SuccessCount
			summary.ClientErrorCount += bucket.ClientErrorCount
			summary.ServerErrorCount += bucket.ServerErrorCount
			summary.RejectedCount += bucket.RejectedCount
			summary.TimeoutCount += bucket.TimeoutCount
			summary.CancelledCount += bucket.CancelledCount
			summary.UpstreamErrorCount += bucket.UpstreamErrorCount
			summary.BudgetDeniedCount += bucket.BudgetDeniedCount
			summary.ActivationFailureCount += bucket.ActivationFailureCount
			summary.ReconcileFailureCount += bucket.ReconcileFailureCount
			for index, count := range bucket.histogram {
				summary.histogram[index] += count
			}
			bucket.histogram = [len(durationBounds) + 1]int64{}
			buckets = append(buckets, bucket)
		}
	}
	summary.DurationP95Ms = percentile95(summary.histogram)
	summary.histogram = [len(durationBounds) + 1]int64{}
	r.mu.Unlock()
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Minute.Before(buckets[j].Minute) })
	return Snapshot{StartedAt: r.startedAt, InFlight: r.inFlight.Load(), Summary: summary, Buckets: buckets}
}

func (r *Recorder) bucketLocked(at time.Time) *Bucket {
	minute := at.UTC().Truncate(time.Minute)
	index := int(minute.Unix()/60) % bucketCount
	if index < 0 {
		index += bucketCount
	}
	if !r.buckets[index].Minute.Equal(minute) {
		r.buckets[index] = Bucket{Minute: minute}
	}
	return &r.buckets[index]
}

func percentile95(histogram [len(durationBounds) + 1]int64) int64 {
	var total int64
	for _, count := range histogram {
		total += count
	}
	if total == 0 {
		return 0
	}
	target := (total*95 + 99) / 100
	var cumulative int64
	for i, count := range histogram {
		cumulative += count
		if cumulative >= target {
			if i < len(durationBounds) {
				return durationBounds[i]
			}
			return durationBounds[len(durationBounds)-1]
		}
	}
	return 0
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}
func (w *statusWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

func HTTPMiddleware(recorder *Recorder, logger Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if recorder == nil {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/live" || r.URL.Path == "/ready" {
				writer := &statusWriter{ResponseWriter: w}
				started := time.Now()
				next.ServeHTTP(writer, r)
				if writer.status >= 400 {
					logger.RequestCompleted(r.Context(), r.URL.Path, r.Method, writer.status, time.Since(started))
				}
				return
			}
			started := time.Now()
			finish := recorder.Begin()
			writer := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(writer, r)
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			duration := time.Since(started)
			finish(Outcome{
				Status: status, Duration: duration,
				Rejected: status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests,
				Timeout:  status == http.StatusGatewayTimeout, Cancelled: r.Context().Err() != nil,
				UpstreamError: status == http.StatusBadGateway || status == http.StatusGatewayTimeout,
				BudgetDenied:  status == http.StatusTooManyRequests,
			})
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			logger.RequestCompleted(r.Context(), route, r.Method, status, duration)
		})
	}
}
