// Package protocolusage contains protocol-shaped, transport-independent usage
// observers. It deliberately has no dependency on Relay runtime, Hub, or
// persistence so callers can observe bytes without transferring ownership of
// the underlying request or response.
package protocolusage

import "fmt"

type Meter string

const (
	Requests     Meter = "REQUESTS"
	InputTokens  Meter = "INPUT_TOKENS"
	OutputTokens Meter = "OUTPUT_TOKENS"
	TotalTokens  Meter = "TOTAL_TOKENS"
	CachedTokens Meter = "CACHED_TOKENS"
	Characters   Meter = "CHARACTERS"
	AudioSeconds Meter = "AUDIO_SECONDS"
)

type Completeness string

const (
	Exact   Completeness = "EXACT"
	Partial Completeness = "PARTIAL"
	Unknown Completeness = "UNKNOWN"
)

// Quantity is an exact non-negative rational value. Counts use denominator 1;
// audio duration uses sample frames / sample rate and is rounded only by the
// eventual settlement owner.
type Quantity struct {
	Numerator   int64
	Denominator int64
}

func NewQuantity(numerator, denominator int64) (Quantity, error) {
	if numerator < 0 || denominator <= 0 {
		return Quantity{}, fmt.Errorf("invalid non-negative quantity %d/%d", numerator, denominator)
	}
	if numerator == 0 {
		return Quantity{Numerator: 0, Denominator: 1}, nil
	}
	g := gcd(numerator, denominator)
	return Quantity{Numerator: numerator / g, Denominator: denominator / g}, nil
}

func count(value int64) *Quantity {
	q, _ := NewQuantity(value, 1)
	return &q
}

func rational(numerator, denominator int64) *Quantity {
	q, err := NewQuantity(numerator, denominator)
	if err != nil {
		return nil
	}
	return &q
}

func (q Quantity) add(other Quantity) Quantity {
	n := q.Numerator*other.Denominator + other.Numerator*q.Denominator
	d := q.Denominator * other.Denominator
	result, _ := NewQuantity(n, d)
	return result
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

type Measurement struct {
	Meter        Meter
	Value        *Quantity
	Source       string
	Completeness Completeness
}

type Diagnostic struct {
	Code    string
	Message string
}

// Detail is provider-reported context which is useful for diagnosis but is
// not an independently deductible platform meter. In particular, reasoning
// tokens are already included in OUTPUT_TOKENS.
type Detail struct {
	Name         string
	Value        Quantity
	Source       string
	Completeness Completeness
}

type Result struct {
	Measurements []Measurement
	Details      []Detail
	Diagnostics  []Diagnostic
}

func (r *Result) set(measurement Measurement) {
	for i := range r.Measurements {
		if r.Measurements[i].Meter == measurement.Meter {
			r.Measurements[i] = measurement
			return
		}
	}
	r.Measurements = append(r.Measurements, measurement)
}

func (r *Result) diagnostic(code, message string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: code, Message: message})
}

func requestResult(source string) Result {
	return Result{Measurements: []Measurement{{Meter: Requests, Value: count(1), Source: source, Completeness: Exact}}}
}

func MCPRequestResult() Result {
	return requestResult("relay_request")
}
