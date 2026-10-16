package protocolusage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type RealtimeASRProtocol string

const (
	OpenAIRealtimeTranscription RealtimeASRProtocol = "OPENAI_REALTIME_TRANSCRIPTION"
	DashScopeRealtimeASR        RealtimeASRProtocol = "DASHSCOPE_REALTIME_ASR"
)

// RealtimeASRObserver consumes already reconstructed WebSocket text messages.
// Raw frame fragmentation, masking, control-frame interleaving, and negotiated
// compression remain the Relay transport integration owner's responsibility.
type RealtimeASRObserver struct {
	protocol     RealtimeASRProtocol
	sampleRate   int64
	configured   bool
	duration     Quantity
	hasDuration  bool
	unknownAudio bool
	invalidAudio bool
	diagnostics  []Diagnostic
}

func NewRealtimeASRObserver(protocol RealtimeASRProtocol) *RealtimeASRObserver {
	return &RealtimeASRObserver{protocol: protocol, duration: Quantity{Denominator: 1}}
}

func (o *RealtimeASRObserver) ObserveClientMessage(message []byte) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(message, &event); err != nil {
		o.diagnostic("invalid_realtime_message", err.Error())
		return
	}
	switch stringValue(event, "type") {
	case "input_audio_buffer.append":
		var audio string
		if err := json.Unmarshal(event["audio"], &audio); err != nil || audio == "" {
			o.diagnostic("realtime_audio_missing", "append event has no base64 audio")
			o.invalidAudio = true
			return
		}
		pcm, err := base64.StdEncoding.DecodeString(audio)
		if err != nil {
			o.diagnostic("invalid_base64_audio", err.Error())
			o.invalidAudio = true
			return
		}
		if o.sampleRate == 0 {
			o.unknownAudio = true
			o.diagnostic("realtime_format_unconfirmed", "audio arrived before a valid server-confirmed PCM format")
			return
		}
		frames := int64(len(pcm) / 2)
		q, _ := NewQuantity(frames, o.sampleRate)
		if o.hasDuration {
			o.duration = o.duration.add(q)
		} else {
			o.duration = q
			o.hasDuration = true
		}
		if len(pcm)%2 != 0 {
			o.invalidAudio = true
			o.diagnostic("incomplete_pcm16_sample", "audio append ends inside a PCM16 sample")
		}
	}
}

func (o *RealtimeASRObserver) ObserveServerMessage(message []byte) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(message, &event); err != nil {
		o.diagnostic("invalid_realtime_message", err.Error())
		return
	}
	if stringValue(event, "type") != "session.updated" {
		return
	}
	session, present, err := rawObject(event, "session")
	if err != nil || !present {
		o.diagnostic("realtime_session_missing", "session.updated has no session object")
		return
	}
	rate, valid := o.confirmedRate(session)
	if !valid {
		o.diagnostic("unsupported_realtime_format", fmt.Sprintf("server confirmed an unsupported %s audio format", o.protocol))
		return
	}
	o.sampleRate = rate
	o.configured = true
}

func (o *RealtimeASRObserver) confirmedRate(session map[string]json.RawMessage) (int64, bool) {
	switch o.protocol {
	case OpenAIRealtimeTranscription:
		audio, ok, _ := rawObject(session, "audio")
		if !ok {
			return 0, false
		}
		input, ok, _ := rawObject(audio, "input")
		if !ok {
			return 0, false
		}
		format, ok, _ := rawObject(input, "format")
		if !ok || stringValue(format, "type") != "audio/pcm" {
			return 0, false
		}
		var rate int64
		if json.Unmarshal(format["rate"], &rate) != nil || rate != 24000 {
			return 0, false
		}
		return rate, true
	case DashScopeRealtimeASR:
		if stringValue(session, "input_audio_format") != "pcm" {
			return 0, false
		}
		var rate int64
		if json.Unmarshal(session["sample_rate"], &rate) != nil || (rate != 8000 && rate != 16000) {
			return 0, false
		}
		return rate, true
	default:
		return 0, false
	}
}

func (o *RealtimeASRObserver) diagnostic(code, message string) {
	o.diagnostics = append(o.diagnostics, Diagnostic{Code: code, Message: message})
}

func (o *RealtimeASRObserver) Finish(normalClose bool) Result {
	result := requestResult("websocket_connection")
	result.Diagnostics = append(result.Diagnostics, o.diagnostics...)
	measurement := Measurement{Meter: AudioSeconds, Source: "submitted_pcm16_samples", Completeness: Unknown}
	if o.hasDuration || o.configured {
		value := o.duration
		measurement.Value = &value
		measurement.Completeness = Exact
		if !normalClose || o.unknownAudio || o.invalidAudio {
			measurement.Completeness = Partial
		}
	}
	result.set(measurement)
	return result
}
