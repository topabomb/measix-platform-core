package protocolusage

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
)

// ObserveOpenAIMultipart streams only the file form part through the WAV
// scanner. Multipart framing and all other form fields are excluded.
func ObserveOpenAIMultipart(reader io.Reader, boundary string) Result {
	result := requestResult("provider_request")
	scanner := NewWAVScanner()
	foundFile := false
	complete := true
	multipartReader := multipart.NewReader(reader, boundary)
	for {
		part, err := multipartReader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.diagnostic("invalid_multipart", err.Error())
			complete = false
			break
		}
		if part.FormName() != "file" {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		if foundFile {
			result.diagnostic("multiple_audio_files", "only the first file part is used for platform audio duration")
			complete = false
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		foundFile = true
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := part.Read(buffer)
			if n > 0 {
				_ = scanner.Observe(buffer[:n])
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				result.diagnostic("audio_part_read_failed", readErr.Error())
				complete = false
				break
			}
		}
		_ = part.Close()
	}
	if !foundFile {
		result.diagnostic("audio_file_missing", "multipart request contains no file part")
		complete = false
	}
	wavResult := scanner.Finish(complete)
	result.Diagnostics = append(result.Diagnostics, wavResult.Diagnostics...)
	for _, measurement := range wavResult.Measurements {
		result.set(measurement)
	}
	return result
}

// DataURIWAVObserver accepts fragments of a single Data URI string. This is
// the bounded-memory primitive used by a streaming JSON string observer for
// DashScope input.messages[].content[].input_audio.data.
type DataURIWAVObserver struct {
	prefix      []byte
	prefixDone  bool
	encoded     []byte
	scanner     *WAVScanner
	invalid     bool
	diagnostics []Diagnostic
}

func NewDataURIWAVObserver() *DataURIWAVObserver {
	return &DataURIWAVObserver{scanner: NewWAVScanner()}
}

func (o *DataURIWAVObserver) Observe(chunk []byte) error {
	if o.invalid {
		return nil
	}
	if !o.prefixDone {
		comma := -1
		for index, value := range chunk {
			if value == ',' {
				comma = index
				break
			}
		}
		if comma < 0 {
			o.prefix = append(o.prefix, chunk...)
			if len(o.prefix) > 256 {
				o.fail("data_uri_prefix_too_large", "Data URI metadata exceeds 256 bytes")
			}
			return nil
		}
		o.prefix = append(o.prefix, chunk[:comma]...)
		metadata := strings.ToLower(string(o.prefix))
		if !strings.HasPrefix(metadata, "data:") || !strings.Contains(metadata, ";base64") {
			o.fail("unsupported_data_uri", "audio data must be a base64 Data URI")
			return nil
		}
		o.prefixDone = true
		chunk = chunk[comma+1:]
	}
	for _, value := range chunk {
		if value == '\r' || value == '\n' || value == ' ' || value == '\t' {
			continue
		}
		o.encoded = append(o.encoded, value)
	}
	return o.decodeAvailable(false)
}

func (o *DataURIWAVObserver) decodeAvailable(final bool) error {
	usable := len(o.encoded) / 4 * 4
	if !final && usable == len(o.encoded) && usable >= 4 && o.encoded[usable-1] != '=' {
		// Keeping one quantum makes it impossible to mistake a non-final quantum
		// for the end when the next observation carries padding.
		usable -= 4
	}
	if final {
		usable = len(o.encoded)
	}
	if usable == 0 {
		return nil
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(usable))
	n, err := base64.StdEncoding.Decode(decoded, o.encoded[:usable])
	if err != nil {
		o.fail("invalid_base64_audio", err.Error())
		return err
	}
	if err := o.scanner.Observe(decoded[:n]); err != nil {
		return err
	}
	o.encoded = append(o.encoded[:0], o.encoded[usable:]...)
	return nil
}

func (o *DataURIWAVObserver) fail(code, message string) {
	o.invalid = true
	o.diagnostics = append(o.diagnostics, Diagnostic{Code: code, Message: message})
}

func (o *DataURIWAVObserver) Finish(transportComplete bool) Result {
	if !o.prefixDone {
		o.fail("incomplete_data_uri", "Data URI has no metadata separator")
	}
	if !o.invalid {
		if err := o.decodeAvailable(true); err != nil {
			transportComplete = false
		}
	}
	result := o.scanner.Finish(transportComplete && !o.invalid)
	result.Diagnostics = append(o.diagnostics, result.Diagnostics...)
	return result
}

func (o *DataURIWAVObserver) String() string {
	return fmt.Sprintf("DataURIWAVObserver(prefix=%t, buffered=%d)", o.prefixDone, len(o.encoded))
}
