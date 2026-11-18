package protocolusage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type LLMProtocol string

const (
	OpenAIChatCompletions LLMProtocol = "OPENAI_CHAT_COMPLETIONS"
	OpenAIResponses       LLMProtocol = "OPENAI_RESPONSES"
	GoogleGenerateContent LLMProtocol = "GOOGLE_GENERATE_CONTENT"
	AnthropicMessages     LLMProtocol = "ANTHROPIC_MESSAGES"

	maxLLMObservationBytes = 2 << 20
	maxSSEDataLines        = 4096
	maxLLMUsageEvents      = 1024
	maxLLMDiagnostics      = 32
	maxLLMDiagnosticBytes  = 512
)

// LLMOptions records profile-specific facts which cannot safely be inferred
// from a vendor protocol name alone.
type LLMOptions struct {
	GeminiThoughtsMayBeAbsent       bool
	AnthropicCacheFieldsMayBeAbsent bool
}

type tokenSnapshot struct {
	input          *int64
	output         *int64
	total          *int64
	cached         *int64
	reasoning      *int64
	cacheCreation  *int64
	providerSource string
}

type llmState struct {
	protocol       LLMProtocol
	options        LLMOptions
	snapshot       tokenSnapshot
	terminal       bool
	usageSeen      bool
	diagnostics    []Diagnostic
	seenEvents     map[string]struct{}
	seenEventBytes int
	observationErr error
}

func ParseLLMJSON(protocol LLMProtocol, body []byte, options LLMOptions) Result {
	state := newLLMState(protocol, options)
	if err := state.consumeJSON("", "", body); err != nil {
		state.addDiagnostic("invalid_usage_payload", err.Error())
	} else {
		state.terminal = true
	}
	return state.result(true)
}

func newLLMState(protocol LLMProtocol, options LLMOptions) *llmState {
	return &llmState{protocol: protocol, options: options, seenEvents: make(map[string]struct{})}
}

func (s *llmState) addDiagnostic(code, message string) {
	if len(message) > maxLLMDiagnosticBytes {
		message = message[:maxLLMDiagnosticBytes]
	}
	for _, diagnostic := range s.diagnostics {
		if diagnostic.Code == code && diagnostic.Message == message {
			return
		}
	}
	if len(s.diagnostics) >= maxLLMDiagnostics {
		return
	}
	s.diagnostics = append(s.diagnostics, Diagnostic{Code: code, Message: message})
}

func (s *llmState) stopObservation() error {
	if s.observationErr == nil {
		s.observationErr = fmt.Errorf("LLM SSE observation exceeded its space limit")
		// Always retain the limit diagnostic, even after many malformed events.
		if len(s.diagnostics) == maxLLMDiagnostics {
			s.diagnostics = s.diagnostics[:maxLLMDiagnostics-1]
		}
		s.addDiagnostic("response_usage_observation_limit", s.observationErr.Error())
		s.seenEvents = nil
		s.seenEventBytes = 0
	}
	return s.observationErr
}

func (s *llmState) consumeJSON(eventName, eventID string, data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode %s usage event: %w", s.protocol, err)
	}
	switch s.protocol {
	case OpenAIChatCompletions:
		usage, present, err := rawObject(object, "usage")
		if err != nil {
			return err
		}
		if present {
			s.mergeOpenAIUsage(usage, "openai_chat_usage")
		}
	case OpenAIResponses:
		typeName := stringValue(object, "type")
		if typeName == "" {
			typeName = eventName
		}
		response := object
		if nested, present, err := rawObject(object, "response"); err != nil {
			return err
		} else if present {
			response = nested
		}
		usage, present, err := rawObject(response, "usage")
		if err != nil {
			return err
		}
		terminal := typeName == "response.completed" || typeName == "response.incomplete" || typeName == "response.failed" ||
			stringValue(response, "status") == "completed" || stringValue(response, "status") == "incomplete" || stringValue(response, "status") == "failed"
		// Content deltas do not change meters and need no retained identity.
		// Bound both the count and bytes of identities used for usage deduplication.
		if present || terminal {
			key := eventID + "|" + stringValue(response, "id") + "|" + integerKey(object, "sequence_number") + "|" + typeName
			if key != "|||" {
				if _, duplicate := s.seenEvents[key]; duplicate {
					return nil
				}
				if len(s.seenEvents) >= maxLLMUsageEvents || len(key) > maxLLMObservationBytes-s.seenEventBytes {
					return s.stopObservation()
				}
				s.seenEvents[key] = struct{}{}
				s.seenEventBytes += len(key)
			}
		}
		if present {
			s.mergeOpenAIUsage(usage, "openai_responses_usage")
		}
		if terminal {
			s.terminal = true
		}
	case GoogleGenerateContent:
		usage, present, err := rawObject(object, "usageMetadata")
		if err != nil {
			return err
		}
		if present {
			s.mergeGeminiUsage(usage)
		}
	case AnthropicMessages:
		usageParent := object
		if message, present, err := rawObject(object, "message"); err != nil {
			return err
		} else if present {
			usageParent = message
		}
		usage, present, err := rawObject(usageParent, "usage")
		if err != nil {
			return err
		}
		if present {
			s.mergeAnthropicUsage(usage)
		}
		if eventName == "message_stop" || stringValue(object, "type") == "message_stop" {
			s.terminal = true
		}
	default:
		return fmt.Errorf("unsupported LLM protocol %q", s.protocol)
	}
	return nil
}

func (s *llmState) mergeOpenAIUsage(usage map[string]json.RawMessage, source string) {
	s.usageSeen = true
	s.snapshot.providerSource = source
	mergeNonNegative(usage, "prompt_tokens", &s.snapshot.input, s)
	mergeNonNegative(usage, "input_tokens", &s.snapshot.input, s)
	mergeNonNegative(usage, "completion_tokens", &s.snapshot.output, s)
	mergeNonNegative(usage, "output_tokens", &s.snapshot.output, s)
	mergeNonNegative(usage, "total_tokens", &s.snapshot.total, s)
	if details, present, _ := rawObject(usage, "prompt_tokens_details"); present {
		mergeNonNegative(details, "cached_tokens", &s.snapshot.cached, s)
	}
	if details, present, _ := rawObject(usage, "input_tokens_details"); present {
		mergeNonNegative(details, "cached_tokens", &s.snapshot.cached, s)
	}
	if details, present, _ := rawObject(usage, "completion_tokens_details"); present {
		mergeNonNegative(details, "reasoning_tokens", &s.snapshot.reasoning, s)
	}
	if details, present, _ := rawObject(usage, "output_tokens_details"); present {
		mergeNonNegative(details, "reasoning_tokens", &s.snapshot.reasoning, s)
	}
}

func (s *llmState) mergeGeminiUsage(usage map[string]json.RawMessage) {
	s.usageSeen = true
	s.snapshot.providerSource = "gemini_usage_metadata"
	mergeNonNegative(usage, "promptTokenCount", &s.snapshot.input, s)
	var candidates *int64
	mergeNonNegative(usage, "candidatesTokenCount", &candidates, s)
	mergeNonNegative(usage, "thoughtsTokenCount", &s.snapshot.reasoning, s)
	mergeNonNegative(usage, "totalTokenCount", &s.snapshot.total, s)
	mergeNonNegative(usage, "cachedContentTokenCount", &s.snapshot.cached, s)
	if candidates != nil {
		if s.snapshot.reasoning != nil {
			value := *candidates + *s.snapshot.reasoning
			s.snapshot.output = &value
		} else if s.options.GeminiThoughtsMayBeAbsent {
			value := *candidates
			s.snapshot.output = &value
		} else {
			s.snapshot.output = nil
			s.addDiagnostic("gemini_thoughts_unknown", "thoughtsTokenCount is absent for a profile that has not declared it optional")
		}
	}
}

func (s *llmState) mergeAnthropicUsage(usage map[string]json.RawMessage) {
	s.usageSeen = true
	s.snapshot.providerSource = "anthropic_usage"
	var baseInput, creation, read *int64
	mergeNonNegative(usage, "input_tokens", &baseInput, s)
	mergeNonNegative(usage, "cache_creation_input_tokens", &creation, s)
	mergeNonNegative(usage, "cache_read_input_tokens", &read, s)
	mergeNonNegative(usage, "output_tokens", &s.snapshot.output, s)
	if baseInput != nil {
		if creation != nil && read != nil {
			value := *baseInput + *creation + *read
			s.snapshot.input = &value
			s.snapshot.cacheCreation = creation
			s.snapshot.cached = read
		} else if s.options.AnthropicCacheFieldsMayBeAbsent {
			value := *baseInput
			if creation != nil {
				value += *creation
				s.snapshot.cacheCreation = creation
			}
			if read != nil {
				value += *read
				s.snapshot.cached = read
			}
			s.snapshot.input = &value
		} else {
			s.addDiagnostic("anthropic_cache_fields_unknown", "cache input fields are absent for a profile that has not declared them optional")
		}
	}
}

func mergeNonNegative(object map[string]json.RawMessage, key string, target **int64, state *llmState) {
	raw, ok := object[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 {
		state.addDiagnostic("invalid_token_value", fmt.Sprintf("%s must be a non-negative integer", key))
		return
	}
	*target = &value
}

func (s *llmState) result(transportComplete bool) Result {
	result := requestResult("provider_request")
	result.Diagnostics = append(result.Diagnostics, s.diagnostics...)
	terminal := s.terminal
	if s.protocol == GoogleGenerateContent && transportComplete && s.usageSeen {
		terminal = true
	}
	knownCompleteness := Partial
	if terminal && transportComplete && s.observationErr == nil {
		knownCompleteness = Exact
	}
	if !s.usageSeen {
		knownCompleteness = Unknown
	}
	input, output, total := s.snapshot.input, s.snapshot.output, s.snapshot.total
	if total == nil && input != nil && output != nil {
		value := *input + *output
		total = &value
	}
	if total != nil && input != nil && output != nil && *total != *input+*output {
		result.diagnostic("token_total_mismatch", fmt.Sprintf("provider total %d differs from explained input plus output %d", *total, *input+*output))
	}
	setTokenMeasurement(&result, InputTokens, input, s.snapshot.providerSource, knownCompleteness)
	setTokenMeasurement(&result, OutputTokens, output, s.snapshot.providerSource, knownCompleteness)
	setTokenMeasurement(&result, TotalTokens, total, s.snapshot.providerSource, knownCompleteness)
	if s.snapshot.cached != nil {
		setTokenMeasurement(&result, CachedTokens, s.snapshot.cached, s.snapshot.providerSource, knownCompleteness)
	}
	if s.snapshot.reasoning != nil {
		result.Details = append(result.Details, Detail{Name: "reasoning_tokens", Value: *count(*s.snapshot.reasoning), Source: s.snapshot.providerSource, Completeness: knownCompleteness})
	}
	if s.snapshot.cacheCreation != nil {
		result.Details = append(result.Details, Detail{Name: "cache_creation_input_tokens", Value: *count(*s.snapshot.cacheCreation), Source: s.snapshot.providerSource, Completeness: knownCompleteness})
	}
	return result
}

func setTokenMeasurement(result *Result, meter Meter, value *int64, source string, completeness Completeness) {
	measurement := Measurement{Meter: meter, Source: source, Completeness: completeness}
	if value != nil {
		measurement.Value = count(*value)
	} else {
		measurement.Completeness = Unknown
	}
	result.set(measurement)
}

func rawObject(parent map[string]json.RawMessage, key string) (map[string]json.RawMessage, bool, error) {
	raw, ok := parent[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, false, fmt.Errorf("%s must be an object: %w", key, err)
	}
	return object, true, nil
}

func stringValue(object map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}

func integerKey(object map[string]json.RawMessage, key string) string {
	if raw, ok := object[key]; ok {
		return string(bytes.TrimSpace(raw))
	}
	return ""
}

type LLMSSEObserver struct {
	state      *llmState
	buffer     []byte
	eventName  string
	eventID    string
	dataLines  []string
	eventBytes int
	finished   bool
}

func NewLLMSSEObserver(protocol LLMProtocol, options LLMOptions) *LLMSSEObserver {
	return &LLMSSEObserver{state: newLLMState(protocol, options)}
}

func (o *LLMSSEObserver) Observe(chunk []byte) error {
	if o.finished {
		return fmt.Errorf("SSE observer already finished")
	}
	if o.state.observationErr != nil {
		return o.state.observationErr
	}
	for len(chunk) > 0 {
		index := bytes.IndexByte(chunk, '\n')
		length := len(chunk)
		if index >= 0 {
			length = index
		}
		if length > maxLLMObservationBytes-len(o.buffer) {
			return o.stopObservation()
		}
		needed := len(o.buffer) + length
		if needed > cap(o.buffer) {
			capacity := max(needed, min(maxLLMObservationBytes, max(4096, 2*cap(o.buffer))))
			buffer := make([]byte, len(o.buffer), capacity)
			copy(buffer, o.buffer)
			o.buffer = buffer
		}
		o.buffer = append(o.buffer, chunk[:length]...)
		if index < 0 {
			break
		}
		chunk = chunk[index+1:]
		line := o.buffer
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if err := o.consumeLine(string(line)); err != nil {
			return o.stopObservation()
		}
		o.buffer = o.buffer[:0]
	}
	return nil
}

func (o *LLMSSEObserver) stopObservation() error {
	o.buffer, o.dataLines = nil, nil
	o.eventName, o.eventID, o.eventBytes = "", "", 0
	return o.state.stopObservation()
}

func (o *LLMSSEObserver) consumeLine(line string) error {
	if line == "" {
		return o.dispatch()
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}
	if len(line)+1 > maxLLMObservationBytes-o.eventBytes {
		return o.stopObservation()
	}
	o.eventBytes += len(line) + 1
	field, value, found := strings.Cut(line, ":")
	if !found {
		value = ""
	}
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	switch field {
	case "event":
		o.eventName = value
	case "id":
		o.eventID = value
	case "data":
		if len(o.dataLines) >= maxSSEDataLines {
			return o.stopObservation()
		}
		o.dataLines = append(o.dataLines, value)
	}
	return nil
}

func (o *LLMSSEObserver) dispatch() error {
	o.eventBytes = 0
	if len(o.dataLines) == 0 {
		o.eventName = ""
		return nil
	}
	data := strings.Join(o.dataLines, "\n")
	if strings.TrimSpace(data) == "[DONE]" {
		o.state.terminal = true
	} else if err := o.state.consumeJSON(o.eventName, o.eventID, []byte(data)); err != nil {
		if o.state.observationErr != nil {
			return o.stopObservation()
		}
		o.state.addDiagnostic("invalid_sse_usage_event", err.Error())
	}
	o.eventName = ""
	// Release line strings rather than retaining response content in the backing array.
	o.dataLines = nil
	return nil
}

func (o *LLMSSEObserver) Finish(transportComplete bool) Result {
	if !o.finished {
		if len(o.buffer) > 0 {
			line := o.buffer
			if line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			_ = o.consumeLine(string(line))
		}
		if o.state.observationErr == nil {
			_ = o.dispatch()
		}
		o.buffer, o.dataLines, o.state.seenEvents = nil, nil, nil
		o.eventName, o.eventID = "", ""
		o.finished = true
	}
	return o.state.result(transportComplete)
}
