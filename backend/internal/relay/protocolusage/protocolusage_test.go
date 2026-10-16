package protocolusage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"testing"
)

func TestLLMFixedExamplesJSON(t *testing.T) {
	tests := []struct {
		name       string
		protocol   LLMProtocol
		body       string
		want       map[Meter]int64
		wantDetail map[string]int64
	}{
		{
			name:     "chat",
			protocol: OpenAIChatCompletions,
			body: `{"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,` +
				`"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}}`,
			want:       map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CachedTokens: 40},
			wantDetail: map[string]int64{"reasoning_tokens": 5},
		},
		{
			name:     "responses",
			protocol: OpenAIResponses,
			body: `{"id":"resp_1","status":"completed","usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,` +
				`"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":5}}}`,
			want:       map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CachedTokens: 40},
			wantDetail: map[string]int64{"reasoning_tokens": 5},
		},
		{
			name:       "gemini",
			protocol:   GoogleGenerateContent,
			body:       `{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":5,"totalTokenCount":125,"cachedContentTokenCount":40}}`,
			want:       map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 25, TotalTokens: 125, CachedTokens: 40},
			wantDetail: map[string]int64{"reasoning_tokens": 5},
		},
		{
			name:       "anthropic",
			protocol:   AnthropicMessages,
			body:       `{"usage":{"input_tokens":10,"cache_creation_input_tokens":30,"cache_read_input_tokens":60,"output_tokens":20}}`,
			want:       map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CachedTokens: 60},
			wantDetail: map[string]int64{"cache_creation_input_tokens": 30},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseLLMJSON(tc.protocol, []byte(tc.body), LLMOptions{})
			assertCounts(t, result, tc.want, Exact)
			for name, value := range tc.wantDetail {
				assertDetail(t, result, name, value, Exact)
			}
		})
	}
}

func TestLLMSSEChunkingMultilineAndDuplicateTerminal(t *testing.T) {
	stream := "event: response.completed\r\n" +
		"id: evt-7\r\n" +
		"data: {\"sequence_number\":7,\"response\":{\"id\":\"resp_1\",\r\n" +
		"data: \"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"total_tokens\":120,\"input_tokens_details\":{\"cached_tokens\":40},\"output_tokens_details\":{\"reasoning_tokens\":5}}}}\r\n\r\n" +
		"event: response.completed\n" +
		"id: evt-7\n" +
		"data: {\"sequence_number\":7,\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"total_tokens\":120}}}\n\n"
	observer := NewLLMSSEObserver(OpenAIResponses, LLMOptions{})
	for _, b := range []byte(stream) {
		if err := observer.Observe([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	result := observer.Finish(true)
	assertCounts(t, result, map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120}, Exact)
}

func TestLLMStreamingProtocolMergeAndCompletion(t *testing.T) {
	tests := []struct {
		name     string
		protocol LLMProtocol
		stream   string
		want     map[Meter]int64
	}{
		{
			name: "chat usage after finish reason", protocol: OpenAIChatCompletions,
			stream: "data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":null}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\n" +
				"data: [DONE]\n\n",
			want: map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
		},
		{
			name: "gemini snapshots not sum", protocol: GoogleGenerateContent,
			stream: "data: {\"usageMetadata\":{\"promptTokenCount\":90,\"candidatesTokenCount\":10,\"thoughtsTokenCount\":2}}\n\n" +
				"data: {\"usageMetadata\":{\"promptTokenCount\":100,\"candidatesTokenCount\":20,\"thoughtsTokenCount\":5,\"totalTokenCount\":125}}\n\n",
			want: map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 25, TotalTokens: 125},
		},
		{
			name: "anthropic cumulative fields", protocol: AnthropicMessages,
			stream: "event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":10,\"cache_creation_input_tokens\":30,\"cache_read_input_tokens\":60,\"output_tokens\":0}}}\n\n" +
				"event: message_delta\ndata: {\"usage\":{\"output_tokens\":20}}\n\n" +
				"event: message_delta\ndata: {\"usage\":{\"output_tokens\":20}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			want: map[Meter]int64{Requests: 1, InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CachedTokens: 60},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observer := NewLLMSSEObserver(tc.protocol, LLMOptions{})
			for offset := 0; offset < len(tc.stream); {
				n := 1 + offset%7
				if n > len(tc.stream)-offset {
					n = len(tc.stream) - offset
				}
				if err := observer.Observe([]byte(tc.stream[offset : offset+n])); err != nil {
					t.Fatal(err)
				}
				offset += n
			}
			assertCounts(t, observer.Finish(true), tc.want, Exact)
		})
	}
}

func TestLLMMissingAndInterruptedAreNotZero(t *testing.T) {
	missing := ParseLLMJSON(OpenAIChatCompletions, []byte(`{"choices":[]}`), LLMOptions{})
	for _, meter := range []Meter{InputTokens, OutputTokens, TotalTokens} {
		m := mustMeasurement(t, missing, meter)
		if m.Completeness != Unknown || m.Value != nil {
			t.Fatalf("%s = %+v, want unknown", meter, m)
		}
	}

	observer := NewLLMSSEObserver(OpenAIChatCompletions, LLMOptions{})
	if err := observer.Observe([]byte("data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\n")); err != nil {
		t.Fatal(err)
	}
	partial := observer.Finish(false)
	assertCounts(t, partial, map[Meter]int64{InputTokens: 100, OutputTokens: 20, TotalTokens: 120}, Partial)
}

func TestLLMTotalDerivationAndContradictionDiagnostic(t *testing.T) {
	derived := ParseLLMJSON(OpenAIChatCompletions, []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":20}}`), LLMOptions{})
	assertCounts(t, derived, map[Meter]int64{InputTokens: 100, OutputTokens: 20, TotalTokens: 120}, Exact)

	contradiction := ParseLLMJSON(OpenAIChatCompletions, []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":119}}`), LLMOptions{})
	assertCounts(t, contradiction, map[Meter]int64{TotalTokens: 119}, Exact)
	if !hasDiagnostic(contradiction, "token_total_mismatch") {
		t.Fatalf("missing total mismatch diagnostic: %+v", contradiction.Diagnostics)
	}

	negative := ParseLLMJSON(OpenAIChatCompletions, []byte(`{"usage":{"prompt_tokens":-1,"completion_tokens":20}}`), LLMOptions{})
	if !hasDiagnostic(negative, "invalid_token_value") || mustMeasurement(t, negative, InputTokens).Completeness != Unknown {
		t.Fatalf("negative tokens accepted: %+v", negative)
	}
}

func TestProfileSpecificAbsentTokenFieldsStayUnknownByDefault(t *testing.T) {
	gemini := ParseLLMJSON(GoogleGenerateContent, []byte(`{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"totalTokenCount":120}}`), LLMOptions{})
	if mustMeasurement(t, gemini, OutputTokens).Completeness != Unknown {
		t.Fatalf("Gemini thoughts silently defaulted to zero: %+v", gemini)
	}
	geminiVerified := ParseLLMJSON(GoogleGenerateContent, []byte(`{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"totalTokenCount":120}}`), LLMOptions{GeminiThoughtsMayBeAbsent: true})
	assertCounts(t, geminiVerified, map[Meter]int64{OutputTokens: 20, TotalTokens: 120}, Exact)

	claude := ParseLLMJSON(AnthropicMessages, []byte(`{"usage":{"input_tokens":10,"output_tokens":20}}`), LLMOptions{})
	if mustMeasurement(t, claude, InputTokens).Completeness != Unknown {
		t.Fatalf("Anthropic cache fields silently defaulted to zero: %+v", claude)
	}
	claudeVerified := ParseLLMJSON(AnthropicMessages, []byte(`{"usage":{"input_tokens":10,"output_tokens":20}}`), LLMOptions{AnthropicCacheFieldsMayBeAbsent: true})
	assertCounts(t, claudeVerified, map[Meter]int64{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}, Exact)
}

func TestTTSCharacterTargetsUseUnicodeCodePoints(t *testing.T) {
	const text = "你😀 A"
	tests := []struct {
		protocol TTSProtocol
		body     string
	}{
		{OpenAIAudioSpeech, `{"input":"你😀 A","instructions":"not counted"}`},
		{GeminiGenerateContentTTS, `{"contents":[{"role":"user","parts":[{"text":"你😀"},{"text":" A"}]}],"generationConfig":{"speechConfig":{"voiceConfig":{}}}}`},
		{MiMoChatCompletionsTTS, `{"messages":[{"role":"user","content":"style"},{"role":"assistant","content":"old"},{"role":"assistant","content":"你😀 A"}]}`},
	}
	_ = text
	for _, tc := range tests {
		result := CountTTSCharacters(tc.protocol, []byte(tc.body))
		assertCounts(t, result, map[Meter]int64{Requests: 1, Characters: 4}, Exact)
	}
}

func TestWAVScannerArbitraryChunksUnknownChunkAndPadding(t *testing.T) {
	wav := pcm16WAV(16000, 1, 32000, true)
	for chunkSize := 1; chunkSize <= 97; chunkSize += 13 {
		t.Run(fmt.Sprintf("chunk-%d", chunkSize), func(t *testing.T) {
			scanner := NewWAVScanner()
			for offset := 0; offset < len(wav); offset += chunkSize {
				end := offset + chunkSize
				if end > len(wav) {
					end = len(wav)
				}
				if err := scanner.Observe(wav[offset:end]); err != nil {
					t.Fatal(err)
				}
			}
			result := scanner.Finish(true)
			assertQuantity(t, result, AudioSeconds, 1, 1, Exact)
		})
	}
}

func TestWAVScannerTruncationKeepsConfirmedPartialSamples(t *testing.T) {
	wav := pcm16WAV(16000, 1, 32000, false)
	scanner := NewWAVScanner()
	if err := scanner.Observe(wav[:len(wav)-16000]); err != nil {
		t.Fatal(err)
	}
	result := scanner.Finish(false)
	assertQuantity(t, result, AudioSeconds, 1, 2, Partial)
}

func TestOpenAIMultipartCountsOnlyFileWAV(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("model", "whisper-1")
	part, err := w.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(pcm16WAV(16000, 1, 32000, true))
	_ = w.WriteField("prompt", strings.Repeat("x", 4000))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	result := ObserveOpenAIMultipart(&shortReader{data: body.Bytes(), size: 3}, w.Boundary())
	assertCounts(t, result, map[Meter]int64{Requests: 1}, Exact)
	assertQuantity(t, result, AudioSeconds, 1, 1, Exact)
}

func TestOpenAIMultipartMultipleFilesCannotBeExact(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, name := range []string{"first.wav", "second.wav"} {
		part, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(pcm16WAV(16000, 1, 32000, true))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	result := ObserveOpenAIMultipart(bytes.NewReader(body.Bytes()), w.Boundary())
	measurement := mustMeasurement(t, result, AudioSeconds)
	if measurement.Completeness != Partial {
		t.Fatalf("multiple files completeness = %s, want %s", measurement.Completeness, Partial)
	}
	if !hasDiagnostic(result, "multiple_audio_files") {
		t.Fatalf("multiple files diagnostic missing: %+v", result.Diagnostics)
	}
}

func TestDashScopeDataURIBase64Chunks(t *testing.T) {
	wav := pcm16WAV(16000, 1, 32000, true)
	uri := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav)
	observer := NewDataURIWAVObserver()
	for offset := 0; offset < len(uri); {
		n := 1 + offset%11
		if n > len(uri)-offset {
			n = len(uri) - offset
		}
		if err := observer.Observe([]byte(uri[offset : offset+n])); err != nil {
			t.Fatal(err)
		}
		offset += n
	}
	result := observer.Finish(true)
	assertQuantity(t, result, AudioSeconds, 1, 1, Exact)
}

func TestDashScopeRequestStreamsDataURIWithoutBufferingAudio(t *testing.T) {
	wav := pcm16WAV(16000, 1, 32000, true)
	request := `{"model":"fun-asr","input":{"messages":[{"role":"user","content":[{"input_audio":{"data":"data:audio/wav;base64,` +
		base64.StdEncoding.EncodeToString(wav) + `"}}]}]},"parameters":{"format":"wav","sample_rate":16000}}`
	observer := NewDashScopeRequestObserver()
	for _, value := range []byte(request) {
		if err := observer.Observe([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	result := observer.Finish(true)
	assertCounts(t, result, map[Meter]int64{Requests: 1}, Exact)
	assertQuantity(t, result, AudioSeconds, 1, 1, Exact)
}

func TestDashScopeRequestIgnoresDataURIOutsideProtocolPath(t *testing.T) {
	wav := pcm16WAV(16000, 1, 32000, false)
	request := `{"metadata":{"input_audio":{"data":"data:audio/wav;base64,` + base64.StdEncoding.EncodeToString(wav) + `"}},"input":{"messages":[]}}`
	observer := NewDashScopeRequestObserver()
	if err := observer.Observe([]byte(request)); err != nil {
		t.Fatal(err)
	}
	result := observer.Finish(true)
	if mustMeasurement(t, result, AudioSeconds).Completeness != Unknown || !hasDiagnostic(result, "dashscope_audio_missing") {
		t.Fatalf("out-of-path audio was accepted: %+v", result)
	}
}

func TestRealtimeASRApplicationMessageObservers(t *testing.T) {
	pcm := make([]byte, 48000) // 1 second of mono PCM16 at 24 kHz.
	openAI := NewRealtimeASRObserver(OpenAIRealtimeTranscription)
	openAI.ObserveClientMessage([]byte(`{"type":"session.update","session":{"type":"transcription","audio":{"input":{"format":{"type":"audio/pcm","rate":24000}}}}}`))
	openAI.ObserveServerMessage([]byte(`{"type":"session.updated","session":{"type":"transcription","audio":{"input":{"format":{"type":"audio/pcm","rate":24000}}}}}`))
	openAI.ObserveClientMessage([]byte(`{"type":"input_audio_buffer.append","audio":"` + base64.StdEncoding.EncodeToString(pcm) + `"}`))
	openAI.ObserveClientMessage([]byte(`{"type":"input_audio_buffer.clear"}`))
	result := openAI.Finish(true)
	assertCounts(t, result, map[Meter]int64{Requests: 1}, Exact)
	assertQuantity(t, result, AudioSeconds, 1, 1, Exact)

	dash := NewRealtimeASRObserver(DashScopeRealtimeASR)
	dash.ObserveClientMessage([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":16000}}`))
	dash.ObserveServerMessage([]byte(`{"type":"session.updated","session":{"input_audio_format":"pcm","sample_rate":16000}}`))
	dash.ObserveClientMessage([]byte(`{"type":"input_audio_buffer.append","audio":"` + base64.StdEncoding.EncodeToString(make([]byte, 16000)) + `"}`))
	dash.ObserveClientMessage([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":24000}}`))
	dash.ObserveServerMessage([]byte(`{"type":"error","error":{"message":"unsupported"}}`))
	dash.ObserveClientMessage([]byte(`{"type":"input_audio_buffer.append","audio":"` + base64.StdEncoding.EncodeToString(make([]byte, 16000)) + `"}`))
	assertQuantity(t, dash.Finish(false), AudioSeconds, 1, 1, Partial)
}

func TestRealtimeAudioBeforeConfirmedConfigurationIsUnknown(t *testing.T) {
	observer := NewRealtimeASRObserver(OpenAIRealtimeTranscription)
	observer.ObserveClientMessage([]byte(`{"type":"input_audio_buffer.append","audio":"AAAA"}`))
	result := observer.Finish(false)
	m := mustMeasurement(t, result, AudioSeconds)
	if m.Completeness != Unknown || m.Value != nil {
		t.Fatalf("audio = %+v, want unknown", m)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("missing diagnostic for unconfigured audio")
	}
}

func TestRealtimeConfirmedSessionWithNoAudioIsExactZero(t *testing.T) {
	observer := NewRealtimeASRObserver(DashScopeRealtimeASR)
	observer.ObserveServerMessage([]byte(`{"type":"session.updated","session":{"input_audio_format":"pcm","sample_rate":8000}}`))
	assertQuantity(t, observer.Finish(true), AudioSeconds, 0, 1, Exact)
}

func TestMCPOnlyCountsRequest(t *testing.T) {
	result := MCPRequestResult()
	if len(result.Measurements) != 1 {
		t.Fatalf("measurements = %+v", result.Measurements)
	}
	assertCounts(t, result, map[Meter]int64{Requests: 1}, Exact)
}

func assertCounts(t *testing.T, result Result, want map[Meter]int64, completeness Completeness) {
	t.Helper()
	for meter, value := range want {
		m := mustMeasurement(t, result, meter)
		if m.Completeness != completeness || m.Value == nil || m.Value.Numerator != value || m.Value.Denominator != 1 {
			t.Fatalf("%s = %+v, want %d exactness %s", meter, m, value, completeness)
		}
	}
}

func assertQuantity(t *testing.T, result Result, meter Meter, numerator, denominator int64, completeness Completeness) {
	t.Helper()
	m := mustMeasurement(t, result, meter)
	if m.Completeness != completeness || m.Value == nil || m.Value.Numerator != numerator || m.Value.Denominator != denominator {
		t.Fatalf("%s = %+v, want %d/%d %s", meter, m, numerator, denominator, completeness)
	}
}

func assertDetail(t *testing.T, result Result, name string, value int64, completeness Completeness) {
	t.Helper()
	for _, detail := range result.Details {
		if detail.Name == name {
			if detail.Value.Numerator != value || detail.Value.Denominator != 1 || detail.Completeness != completeness {
				t.Fatalf("detail %s = %+v, want %d %s", name, detail, value, completeness)
			}
			return
		}
	}
	t.Fatalf("missing detail %s in %+v", name, result.Details)
}

func mustMeasurement(t *testing.T, result Result, meter Meter) Measurement {
	t.Helper()
	for _, measurement := range result.Measurements {
		if measurement.Meter == meter {
			return measurement
		}
	}
	t.Fatalf("missing %s in %+v", meter, result.Measurements)
	return Measurement{}
}

func hasDiagnostic(result Result, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func pcm16WAV(sampleRate uint32, channels uint16, dataBytes int, unknownChunk bool) []byte {
	var chunks bytes.Buffer
	_ = binary.Write(&chunks, binary.LittleEndian, [4]byte{'f', 'm', 't', ' '})
	_ = binary.Write(&chunks, binary.LittleEndian, uint32(16))
	_ = binary.Write(&chunks, binary.LittleEndian, uint16(1))
	_ = binary.Write(&chunks, binary.LittleEndian, channels)
	_ = binary.Write(&chunks, binary.LittleEndian, sampleRate)
	blockAlign := channels * 2
	_ = binary.Write(&chunks, binary.LittleEndian, sampleRate*uint32(blockAlign))
	_ = binary.Write(&chunks, binary.LittleEndian, blockAlign)
	_ = binary.Write(&chunks, binary.LittleEndian, uint16(16))
	if unknownChunk {
		_, _ = chunks.Write([]byte("JUNK"))
		_ = binary.Write(&chunks, binary.LittleEndian, uint32(3))
		_, _ = chunks.Write([]byte{1, 2, 3, 0}) // odd chunks include one padding byte.
	}
	_, _ = chunks.Write([]byte("data"))
	_ = binary.Write(&chunks, binary.LittleEndian, uint32(dataBytes))
	_, _ = chunks.Write(make([]byte, dataBytes))

	var wav bytes.Buffer
	_, _ = wav.Write([]byte("RIFF"))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(chunks.Len()+4))
	_, _ = wav.Write([]byte("WAVE"))
	_, _ = wav.Write(chunks.Bytes())
	return wav.Bytes()
}

type shortReader struct {
	data []byte
	size int
}

func (r *shortReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	if len(p) > r.size {
		p = p[:r.size]
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
