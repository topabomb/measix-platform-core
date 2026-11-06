package protocolusage

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestSSEObservationBoundsAndIncompleteUsage(t *testing.T) {
	for _, name := range []string{"unfinished-line", "multiline-event", "empty-data-lines", "usage-history", "usage-history-bytes"} {
		t.Run(name, func(t *testing.T) {
			o := NewLLMSSEObserver(OpenAIResponses, LLMOptions{})
			var observationErr error
			switch name {
			case "unfinished-line":
				observationErr = o.Observe(bytes.Repeat([]byte("x"), 8<<20))
			case "multiline-event":
				observationErr = o.Observe([]byte(strings.Repeat("data: "+strings.Repeat("x", 8192)+"\n", 1024)))
			case "empty-data-lines":
				observationErr = o.Observe([]byte(strings.Repeat("data:\n", 100000)))
			case "usage-history":
				for i := 0; i < 20000 && observationErr == nil; i++ {
					observationErr = o.Observe([]byte(fmt.Sprintf("data: {\"type\":\"response.completed\",\"sequence_number\":%d,\"response\":{\"id\":\"r\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n", i)))
				}
			case "usage-history-bytes":
				for i := 0; i < 32 && observationErr == nil; i++ {
					observationErr = o.Observe([]byte(fmt.Sprintf("id: %d-%s\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n", i, strings.Repeat("x", 256<<10))))
				}
			}
			if observationErr == nil {
				t.Error("unbounded observation did not report a limit")
			}
			if len(o.buffer) > 2<<20 || len(o.dataLines) > 4096 || len(o.state.seenEvents) > 4096 {
				t.Errorf("observation retained unbounded state: buffer=%d lines=%d history=%d", len(o.buffer), len(o.dataLines), len(o.state.seenEvents))
			}
			// A later terminal event cannot turn an incomplete observation into EXACT.
			_ = o.Observe([]byte("data: [DONE]\n\n"))
			result := o.Finish(true)
			if mustMeasurement(t, result, TotalTokens).Completeness == Exact || len(result.Diagnostics) == 0 {
				t.Fatalf("observation overflow presented as complete usage: %+v", result)
			}
		})
	}
}

func TestSSELongResponsesStreamKeepsOnlyMeteringHistory(t *testing.T) {
	o := NewLLMSSEObserver(OpenAIResponses, LLMOptions{})
	for i := 0; i < 20000; i++ {
		if err := o.Observe([]byte(fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"sequence_number\":%d,\"delta\":\"%s\"}\n\n", i, strings.Repeat("x", 512)))); err != nil {
			t.Fatal(err)
		}
	}
	if len(o.state.seenEvents) > 1 {
		t.Fatalf("content-only events retained for metering: %d", len(o.state.seenEvents))
	}
	if err := o.Observe([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n")); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, o.Finish(true), map[Meter]int64{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, Exact)
}

func TestSSEDiagnosticsRemainBounded(t *testing.T) {
	o := NewLLMSSEObserver(OpenAIChatCompletions, LLMOptions{})
	for i := 0; i < 20000; i++ {
		_ = o.Observe([]byte(fmt.Sprintf("data: %c-invalid-json\n\n", '!'+i%90)))
	}
	result := o.Finish(true)
	if len(result.Diagnostics) == 0 || len(result.Diagnostics) > 32 {
		t.Fatalf("unbounded diagnostics: %d", len(result.Diagnostics))
	}
	if m := mustMeasurement(t, result, TotalTokens); m.Completeness != Unknown || m.Value != nil {
		t.Fatalf("invalid events silently became known zero: %+v", m)
	}
}
