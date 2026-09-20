package runtime

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/relay/protocolusage"
)

const maxObservedResponseBytes = 2 << 20

type usageObservation struct {
	resource control.Resource

	requestResult protocolusage.Result
	requestStream requestStreamObserver

	responseSSE      *protocolusage.LLMSSEObserver
	responseJSON     bytes.Buffer
	responseMedia    string
	responseEncoding string
	responseOverflow bool
	responseObserved bool

	realtime       *protocolusage.RealtimeASRObserver
	realtimeMu     sync.Mutex
	realtimeFailed bool
}

func (o *usageObservation) llmOptions() protocolusage.LLMOptions {
	if o == nil || o.resource.LLMProfile == nil {
		return protocolusage.LLMOptions{}
	}
	return protocolusage.LLMOptions{
		GeminiThoughtsMayBeAbsent:       o.resource.LLMProfile.GeminiThoughtsMayBeAbsent,
		AnthropicCacheFieldsMayBeAbsent: o.resource.LLMProfile.AnthropicCacheFieldsMayBeAbsent,
	}
}

type requestStreamObserver interface {
	Observe([]byte) error
	Finish(bool) protocolusage.Result
}

func prepareUsageObservation(resource control.Resource, request *http.Request, runtimePath string, maxBytes int64) (*usageObservation, error) {
	observation := &usageObservation{resource: resource}
	protocol := string(resource.ClientProtocol)
	switch string(resource.Kind) {
	case "IMAGE_GENERATION":
		mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if request.Method != http.MethodPost || request.URL.RawQuery != "" || !strings.HasSuffix(runtimePath, "/images/generations") || mediaType != "application/json" || err != nil || resource.ImageProfile == nil {
			return nil, errInvalidImageGenerationRequest
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > maxBytes {
			return nil, errObservedRequestTooLarge
		}
		_ = request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		result, err := protocolusage.ObserveImageGenerationRequest(body, resource.ImageProfile.MaxImagesPerRequest, resource.ImageProfile.AllowedSizes)
		if err != nil {
			return nil, errInvalidImageGenerationRequest
		}
		observation.requestResult = result
	case "TTS":
		body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > maxBytes {
			return nil, errObservedRequestTooLarge
		}
		_ = request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		observation.requestResult = protocolusage.CountTTSCharacters(protocolusage.TTSProtocol(protocol), body)
	case "ASR":
		switch protocol {
		case string(protocolusage.OpenAIRealtimeTranscription), string(protocolusage.DashScopeRealtimeASR):
			observation.realtime = protocolusage.NewRealtimeASRObserver(protocolusage.RealtimeASRProtocol(protocol))
		case "OPENAI_AUDIO_TRANSCRIPTIONS":
			mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/form-data" || parameters["boundary"] == "" {
				// The request may still use an upstream-compatible extension. Expose
				// REQUESTS only so unlimited/request-only policies can proceed while
				// an AUDIO_SECONDS limit is rejected by Hub before forwarding.
				break
			}
			observation.requestStream = newMultipartRequestObserver(parameters["boundary"])
		case "DASHSCOPE_HTTP_ASR":
			observation.requestStream = protocolusage.NewDashScopeRequestObserver()
		}
	}
	return observation, nil
}

var errObservedRequestTooLarge = fmt.Errorf("observed request exceeds runtime limit")
var errInvalidImageGenerationRequest = fmt.Errorf("invalid image generation request")

func (o *usageObservation) supportedMeters() []string {
	switch string(o.resource.Kind) {
	case "MODEL":
		return []string{"REQUESTS", "INPUT_TOKENS", "OUTPUT_TOKENS", "CACHED_TOKENS", "TOTAL_TOKENS"}
	case "TTS":
		meters := []string{"REQUESTS"}
		if measurementExact(o.requestResult, protocolusage.Characters) {
			meters = append(meters, "CHARACTERS")
		}
		return meters
	case "ASR":
		if o.requestStream != nil || o.realtime != nil {
			return []string{"REQUESTS", "AUDIO_SECONDS"}
		}
		return []string{"REQUESTS"}
	case "MCP":
		return []string{"REQUESTS"}
	case "IMAGE_GENERATION":
		return []string{"REQUESTS", "REQUESTED_IMAGES"}
	default:
		return nil
	}
}

func (o *usageObservation) knownMeasurements() []protocolusage.Measurement {
	result := []protocolusage.Measurement{{
		Meter: protocolusage.Requests, Value: quantityOne(), Source: "relay_request", Completeness: protocolusage.Exact,
	}}
	for _, measurement := range o.requestResult.Measurements {
		if measurement.Meter != protocolusage.Requests && measurement.Value != nil && measurement.Completeness == protocolusage.Exact {
			result = append(result, measurement)
		}
	}
	return result
}

func quantityOne() *protocolusage.Quantity {
	value, _ := protocolusage.NewQuantity(1, 1)
	return &value
}

func measurementExact(result protocolusage.Result, meter protocolusage.Meter) bool {
	for _, measurement := range result.Measurements {
		if measurement.Meter == meter {
			return measurement.Value != nil && measurement.Completeness == protocolusage.Exact
		}
	}
	return false
}

func (o *usageObservation) observeRequest(chunk []byte) {
	if o != nil && o.requestStream != nil && len(chunk) > 0 {
		_ = o.requestStream.Observe(chunk)
	}
}

func (o *usageObservation) configureResponse(response *http.Response) {
	if o == nil || string(o.resource.Kind) != "MODEL" {
		return
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	o.responseMedia = mediaType
	o.responseEncoding = strings.TrimSpace(response.Header.Get("Content-Encoding"))
	if mediaType == "text/event-stream" && (o.responseEncoding == "" || strings.EqualFold(o.responseEncoding, "identity")) {
		o.responseSSE = protocolusage.NewLLMSSEObserver(protocolusage.LLMProtocol(o.resource.ClientProtocol), o.llmOptions())
	}
}

func (o *usageObservation) observeResponse(chunk []byte) {
	if o == nil || len(chunk) == 0 || string(o.resource.Kind) != "MODEL" {
		return
	}
	o.responseObserved = true
	if o.responseSSE != nil {
		_ = o.responseSSE.Observe(chunk)
		return
	}
	if o.responseOverflow {
		return
	}
	remaining := maxObservedResponseBytes - o.responseJSON.Len()
	if len(chunk) > remaining {
		o.responseOverflow = true
		o.responseJSON.Reset()
		return
	}
	_, _ = o.responseJSON.Write(chunk)
}

func (o *usageObservation) finish(transportComplete bool) protocolusage.Result {
	result := protocolusage.Result{}
	mergeUsageResult(&result, o.requestResult)
	if o.requestStream != nil {
		mergeUsageResult(&result, o.requestStream.Finish(transportComplete))
	}
	if o.responseSSE != nil {
		mergeUsageResult(&result, o.responseSSE.Finish(transportComplete))
	} else if string(o.resource.Kind) == "MODEL" {
		if o.responseOverflow {
			mergeUsageResult(&result, unknownModelResult("response_usage_observation_limit"))
		} else if o.responseObserved {
			body, err := decodeObservedResponse(o.responseEncoding, o.responseJSON.Bytes())
			if err != nil {
				mergeUsageResult(&result, unknownModelResult("response_content_decode_failed"))
			} else if o.responseMedia == "text/event-stream" {
				observer := protocolusage.NewLLMSSEObserver(protocolusage.LLMProtocol(o.resource.ClientProtocol), o.llmOptions())
				if err := observer.Observe(body); err != nil {
					mergeUsageResult(&result, unknownModelResult("invalid_usage_payload"))
				} else {
					mergeUsageResult(&result, observer.Finish(transportComplete))
				}
			} else {
				mergeUsageResult(&result, protocolusage.ParseLLMJSON(protocolusage.LLMProtocol(o.resource.ClientProtocol), body, o.llmOptions()))
			}
		} else {
			mergeUsageResult(&result, unknownModelResult("response_usage_missing"))
		}
	}
	if o.realtime != nil {
		o.realtimeMu.Lock()
		realtime := o.realtime.Finish(transportComplete && !o.realtimeFailed)
		o.realtimeMu.Unlock()
		mergeUsageResult(&result, realtime)
	}
	if !hasMeter(result, protocolusage.Requests) {
		result.Measurements = append(result.Measurements, protocolusage.Measurement{
			Meter: protocolusage.Requests, Value: quantityOne(), Source: "provider_request", Completeness: protocolusage.Exact,
		})
	}
	return result
}

// decodeObservedResponse decodes a private observation copy. The original
// response bytes and Content-Encoding header continue through ReverseProxy
// unchanged; usage accounting must never become a protocol-transform owner.
func decodeObservedResponse(encoding string, body []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return body, nil
	case "gzip", "x-gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		decompressed, readErr := io.ReadAll(io.LimitReader(reader, maxObservedResponseBytes+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(decompressed) > maxObservedResponseBytes {
			return nil, errObservedRequestTooLarge
		}
		return decompressed, nil
	default:
		return nil, fmt.Errorf("unsupported response content encoding %q", encoding)
	}
}

func (o *usageObservation) observeRealtimeClient(message []byte) {
	if o == nil || o.realtime == nil {
		return
	}
	o.realtimeMu.Lock()
	o.realtime.ObserveClientMessage(message)
	o.realtimeMu.Unlock()
}

func (o *usageObservation) observeRealtimeServer(message []byte) {
	if o == nil || o.realtime == nil {
		return
	}
	o.realtimeMu.Lock()
	o.realtime.ObserveServerMessage(message)
	o.realtimeMu.Unlock()
}

func (o *usageObservation) markRealtimeFailure() {
	if o == nil || o.realtime == nil {
		return
	}
	o.realtimeMu.Lock()
	o.realtimeFailed = true
	o.realtimeMu.Unlock()
}

func mergeUsageResult(target *protocolusage.Result, source protocolusage.Result) {
	for _, incoming := range source.Measurements {
		replaced := false
		for index, existing := range target.Measurements {
			if existing.Meter == incoming.Meter {
				target.Measurements[index] = incoming
				replaced = true
				break
			}
		}
		if !replaced {
			target.Measurements = append(target.Measurements, incoming)
		}
	}
	target.Details = append(target.Details, source.Details...)
	target.Diagnostics = append(target.Diagnostics, source.Diagnostics...)
}

func hasMeter(result protocolusage.Result, meter protocolusage.Meter) bool {
	for _, measurement := range result.Measurements {
		if measurement.Meter == meter {
			return true
		}
	}
	return false
}

func unknownModelResult(code string) protocolusage.Result {
	result := protocolusage.Result{Diagnostics: []protocolusage.Diagnostic{{Code: code, Message: "model usage could not be observed completely"}}}
	for _, meter := range []protocolusage.Meter{protocolusage.InputTokens, protocolusage.OutputTokens, protocolusage.TotalTokens} {
		result.Measurements = append(result.Measurements, protocolusage.Measurement{Meter: meter, Completeness: protocolusage.Unknown, Source: "provider_response"})
	}
	return result
}

type multipartRequestObserver struct {
	writer *io.PipeWriter
	done   chan protocolusage.Result
	once   sync.Once
	result protocolusage.Result
}

func newMultipartRequestObserver(boundary string) *multipartRequestObserver {
	reader, writer := io.Pipe()
	observer := &multipartRequestObserver{writer: writer, done: make(chan protocolusage.Result, 1)}
	go func() {
		observer.done <- protocolusage.ObserveOpenAIMultipart(reader, boundary)
		_ = reader.Close()
	}()
	return observer
}

func (o *multipartRequestObserver) Observe(chunk []byte) error {
	_, err := o.writer.Write(chunk)
	return err
}

func (o *multipartRequestObserver) Finish(complete bool) protocolusage.Result {
	o.once.Do(func() {
		if complete {
			_ = o.writer.Close()
		} else {
			_ = o.writer.CloseWithError(io.ErrUnexpectedEOF)
		}
		o.result = <-o.done
	})
	return o.result
}

var _ requestStreamObserver = (*multipartRequestObserver)(nil)
var _ requestStreamObserver = (*protocolusage.DashScopeRequestObserver)(nil)
