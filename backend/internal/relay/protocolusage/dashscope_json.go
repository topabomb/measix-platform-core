package protocolusage

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

type jsonPhase uint8

const (
	jsonObjectKey jsonPhase = iota
	jsonObjectColon
	jsonObjectValue
	jsonObjectComma
	jsonArrayValue
	jsonArrayComma
)

type jsonFrame struct {
	kind       byte
	phase      jsonPhase
	key        string
	path       string
	inputAudio bool
}

// DashScopeRequestObserver incrementally locates
// input.messages[].content[].input_audio.data without materializing the JSON
// body or its Base64 string. The selected Data URI is decoded directly into a
// WAVScanner.
type DashScopeRequestObserver struct {
	stack          []jsonFrame
	rootStarted    bool
	rootComplete   bool
	inString       bool
	stringIsKey    bool
	stringIsTarget bool
	stringBuffer   []byte
	escaped        bool
	unicodeDigits  []byte
	inPrimitive    bool
	dataURI        *DataURIWAVObserver
	targets        int
	invalid        bool
	diagnostics    []Diagnostic
}

func NewDashScopeRequestObserver() *DashScopeRequestObserver {
	return &DashScopeRequestObserver{}
}

func (o *DashScopeRequestObserver) Observe(chunk []byte) error {
	for index := 0; index < len(chunk); {
		consumed, err := o.consumeByte(chunk[index])
		if err != nil {
			o.invalid = true
			o.diagnostics = append(o.diagnostics, Diagnostic{Code: "invalid_dashscope_request", Message: err.Error()})
			return err
		}
		if consumed {
			index++
		}
	}
	return nil
}

func (o *DashScopeRequestObserver) consumeByte(value byte) (bool, error) {
	if o.inString {
		return true, o.consumeStringByte(value)
	}
	if o.inPrimitive {
		if isJSONDelimiter(value) {
			o.inPrimitive = false
			o.completeValue()
			return false, nil
		}
		return true, nil
	}
	if isJSONSpace(value) {
		return true, nil
	}
	switch value {
	case '"':
		isKey := o.expectsKey()
		if !isKey && !o.expectsValue() {
			return true, fmt.Errorf("unexpected JSON string")
		}
		o.inString = true
		o.stringIsKey = isKey
		o.stringIsTarget = !isKey && o.currentValueIsAudioData()
		o.stringBuffer = o.stringBuffer[:0]
		if o.stringIsTarget {
			o.targets++
			if o.targets == 1 {
				o.dataURI = NewDataURIWAVObserver()
			} else {
				o.diagnostics = append(o.diagnostics, Diagnostic{Code: "multiple_dashscope_audio_inputs", Message: "more than one input_audio.data field is not a supported metering profile"})
				o.stringIsTarget = false
			}
		}
		o.markRootStarted()
		return true, nil
	case '{', '[':
		if !o.expectsValue() {
			return true, fmt.Errorf("unexpected JSON container start %q", value)
		}
		root := len(o.stack) == 0
		path := o.currentValuePath()
		inputAudio := path == "$.input.messages[].content[].input_audio"
		phase := jsonObjectKey
		if value == '[' {
			phase = jsonArrayValue
		}
		o.stack = append(o.stack, jsonFrame{kind: value, phase: phase, path: path, inputAudio: inputAudio})
		if root {
			o.rootStarted = true
		}
		return true, nil
	case '}':
		if len(o.stack) == 0 || o.stack[len(o.stack)-1].kind != '{' {
			return true, fmt.Errorf("unexpected object end")
		}
		phase := o.stack[len(o.stack)-1].phase
		if phase != jsonObjectKey && phase != jsonObjectComma {
			return true, fmt.Errorf("object ended while expecting a value")
		}
		o.stack = o.stack[:len(o.stack)-1]
		o.completeValue()
		return true, nil
	case ']':
		if len(o.stack) == 0 || o.stack[len(o.stack)-1].kind != '[' {
			return true, fmt.Errorf("unexpected array end")
		}
		phase := o.stack[len(o.stack)-1].phase
		if phase != jsonArrayValue && phase != jsonArrayComma {
			return true, fmt.Errorf("array ended while expecting a value")
		}
		o.stack = o.stack[:len(o.stack)-1]
		o.completeValue()
		return true, nil
	case ':':
		if len(o.stack) == 0 || o.stack[len(o.stack)-1].kind != '{' || o.stack[len(o.stack)-1].phase != jsonObjectColon {
			return true, fmt.Errorf("unexpected colon")
		}
		o.stack[len(o.stack)-1].phase = jsonObjectValue
		return true, nil
	case ',':
		if len(o.stack) == 0 {
			return true, fmt.Errorf("unexpected comma")
		}
		frame := &o.stack[len(o.stack)-1]
		if frame.kind == '{' && frame.phase == jsonObjectComma {
			frame.phase = jsonObjectKey
			frame.key = ""
			return true, nil
		}
		if frame.kind == '[' && frame.phase == jsonArrayComma {
			frame.phase = jsonArrayValue
			return true, nil
		}
		return true, fmt.Errorf("unexpected comma")
	default:
		if !o.expectsValue() {
			return true, fmt.Errorf("unexpected JSON primitive")
		}
		o.markRootStarted()
		o.inPrimitive = true
		return true, nil
	}
}

func (o *DashScopeRequestObserver) consumeStringByte(value byte) error {
	if len(o.unicodeDigits) > 0 {
		if !isHex(value) {
			return fmt.Errorf("invalid JSON unicode escape")
		}
		o.unicodeDigits = append(o.unicodeDigits, value)
		if len(o.unicodeDigits) == 4 {
			code, _ := strconv.ParseUint(string(o.unicodeDigits), 16, 16)
			var encoded [utf8.UTFMax]byte
			n := utf8.EncodeRune(encoded[:], rune(code))
			if err := o.consumeDecodedStringBytes(encoded[:n]); err != nil {
				return err
			}
			o.unicodeDigits = o.unicodeDigits[:0]
		}
		return nil
	}
	if o.escaped {
		o.escaped = false
		switch value {
		case '"', '\\', '/':
			return o.consumeDecodedStringBytes([]byte{value})
		case 'b':
			return o.consumeDecodedStringBytes([]byte{'\b'})
		case 'f':
			return o.consumeDecodedStringBytes([]byte{'\f'})
		case 'n':
			return o.consumeDecodedStringBytes([]byte{'\n'})
		case 'r':
			return o.consumeDecodedStringBytes([]byte{'\r'})
		case 't':
			return o.consumeDecodedStringBytes([]byte{'\t'})
		case 'u':
			o.unicodeDigits = make([]byte, 0, 4)
			return nil
		default:
			return fmt.Errorf("invalid JSON escape")
		}
	}
	if value == '\\' {
		o.escaped = true
		return nil
	}
	if value == '"' {
		o.inString = false
		if o.stringIsKey {
			if len(o.stack) == 0 || o.stack[len(o.stack)-1].kind != '{' {
				return fmt.Errorf("object key outside an object")
			}
			frame := &o.stack[len(o.stack)-1]
			frame.key = string(o.stringBuffer)
			frame.phase = jsonObjectColon
		} else {
			o.completeValue()
		}
		return nil
	}
	if value < 0x20 {
		return fmt.Errorf("unescaped control character in JSON string")
	}
	return o.consumeDecodedStringBytes([]byte{value})
}

func (o *DashScopeRequestObserver) consumeDecodedStringBytes(value []byte) error {
	if o.stringIsTarget {
		return o.dataURI.Observe(value)
	}
	if o.stringIsKey {
		if len(o.stringBuffer)+len(value) > 128 {
			return fmt.Errorf("JSON object key exceeds 128 bytes")
		}
		o.stringBuffer = append(o.stringBuffer, value...)
	}
	return nil
}

func (o *DashScopeRequestObserver) expectsKey() bool {
	return len(o.stack) > 0 && o.stack[len(o.stack)-1].kind == '{' && o.stack[len(o.stack)-1].phase == jsonObjectKey
}

func (o *DashScopeRequestObserver) expectsValue() bool {
	if len(o.stack) == 0 {
		return !o.rootStarted
	}
	phase := o.stack[len(o.stack)-1].phase
	return phase == jsonObjectValue || phase == jsonArrayValue
}

func (o *DashScopeRequestObserver) currentValueIsAudioData() bool {
	if len(o.stack) == 0 {
		return false
	}
	frame := o.stack[len(o.stack)-1]
	return frame.kind == '{' && frame.inputAudio && frame.phase == jsonObjectValue && frame.key == "data"
}

func (o *DashScopeRequestObserver) currentValuePath() string {
	if len(o.stack) == 0 {
		return "$"
	}
	frame := o.stack[len(o.stack)-1]
	if frame.kind == '{' && frame.phase == jsonObjectValue {
		return frame.path + "." + frame.key
	}
	if frame.kind == '[' && frame.phase == jsonArrayValue {
		return frame.path + "[]"
	}
	return ""
}

func (o *DashScopeRequestObserver) markRootStarted() {
	if len(o.stack) == 0 {
		o.rootStarted = true
	}
}

func (o *DashScopeRequestObserver) completeValue() {
	if len(o.stack) == 0 {
		o.rootComplete = true
		return
	}
	frame := &o.stack[len(o.stack)-1]
	if frame.kind == '{' {
		frame.phase = jsonObjectComma
	} else {
		frame.phase = jsonArrayComma
	}
}

func (o *DashScopeRequestObserver) Finish(transportComplete bool) Result {
	if o.inPrimitive {
		o.inPrimitive = false
		o.completeValue()
	}
	if o.inString || len(o.stack) != 0 || !o.rootComplete {
		o.invalid = true
		o.diagnostics = append(o.diagnostics, Diagnostic{Code: "dashscope_json_truncated", Message: "request JSON ended before a complete root value"})
	}
	result := requestResult("provider_request")
	result.Diagnostics = append(result.Diagnostics, o.diagnostics...)
	if o.dataURI == nil {
		result.set(Measurement{Meter: AudioSeconds, Source: "pcm16_samples", Completeness: Unknown})
		result.diagnostic("dashscope_audio_missing", "no input_audio.data Data URI was found")
		return result
	}
	audio := o.dataURI.Finish(transportComplete && !o.invalid && o.targets == 1)
	result.Diagnostics = append(result.Diagnostics, audio.Diagnostics...)
	for _, measurement := range audio.Measurements {
		result.set(measurement)
	}
	return result
}

func isJSONSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func isJSONDelimiter(value byte) bool {
	return isJSONSpace(value) || value == ',' || value == ']' || value == '}'
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}
