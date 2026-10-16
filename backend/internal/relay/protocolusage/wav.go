package protocolusage

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type wavStage uint8

const (
	wavRIFF wavStage = iota
	wavChunkHeader
	wavChunkData
	wavChunkPadding
)

// WAVScanner incrementally observes RIFF/WAVE PCM16 bytes. It buffers only
// fixed-size headers and the bounded fmt chunk; audio payload bytes are counted
// and discarded as they pass through.
type WAVScanner struct {
	stage          wavStage
	buffer         []byte
	chunkID        [4]byte
	chunkRemaining uint32
	chunkSize      uint32
	fmtBuffer      []byte
	fmtSeen        bool
	dataSeen       bool
	dataBytes      int64
	sampleRate     uint32
	channels       uint16
	blockAlign     uint16
	bitsPerSample  uint16
	invalid        bool
	diagnostics    []Diagnostic
}

func NewWAVScanner() *WAVScanner {
	return &WAVScanner{stage: wavRIFF}
}

func (s *WAVScanner) Observe(chunk []byte) error {
	for {
		switch s.stage {
		case wavRIFF:
			needed := 12 - len(s.buffer)
			if needed > len(chunk) {
				needed = len(chunk)
			}
			s.buffer = append(s.buffer, chunk[:needed]...)
			chunk = chunk[needed:]
			if len(s.buffer) < 12 {
				return nil
			}
			if !bytes.Equal(s.buffer[:4], []byte("RIFF")) || !bytes.Equal(s.buffer[8:12], []byte("WAVE")) {
				s.fail("invalid_wav_header", "audio is not RIFF/WAVE")
				return nil
			}
			s.buffer = s.buffer[:0]
			s.stage = wavChunkHeader
		case wavChunkHeader:
			needed := 8 - len(s.buffer)
			if needed > len(chunk) {
				needed = len(chunk)
			}
			s.buffer = append(s.buffer, chunk[:needed]...)
			chunk = chunk[needed:]
			if len(s.buffer) < 8 {
				return nil
			}
			copy(s.chunkID[:], s.buffer[:4])
			s.chunkSize = binary.LittleEndian.Uint32(s.buffer[4:8])
			s.chunkRemaining = s.chunkSize
			s.buffer = s.buffer[:0]
			if bytes.Equal(s.chunkID[:], []byte("fmt ")) && s.chunkSize > 4096 {
				s.fail("wav_fmt_too_large", fmt.Sprintf("fmt chunk has unsupported size %d", s.chunkSize))
			}
			s.stage = wavChunkData
		case wavChunkData:
			if s.chunkRemaining == 0 {
				s.finishChunk()
				continue
			}
			if len(chunk) == 0 {
				return nil
			}
			consume := int(s.chunkRemaining)
			if consume > len(chunk) {
				consume = len(chunk)
			}
			if bytes.Equal(s.chunkID[:], []byte("fmt ")) {
				s.fmtBuffer = append(s.fmtBuffer, chunk[:consume]...)
			} else if bytes.Equal(s.chunkID[:], []byte("data")) {
				s.dataSeen = true
				s.dataBytes += int64(consume)
			}
			chunk = chunk[consume:]
			s.chunkRemaining -= uint32(consume)
		case wavChunkPadding:
			if len(chunk) == 0 {
				return nil
			}
			chunk = chunk[1:]
			s.stage = wavChunkHeader
		}
		if len(chunk) == 0 && !(s.stage == wavChunkData && s.chunkRemaining == 0) {
			return nil
		}
	}
}

func (s *WAVScanner) finishChunk() {
	if bytes.Equal(s.chunkID[:], []byte("fmt ")) {
		s.parseFormat()
	}
	if s.chunkSize%2 == 1 {
		s.stage = wavChunkPadding
	} else {
		s.stage = wavChunkHeader
	}
}

func (s *WAVScanner) parseFormat() {
	if len(s.fmtBuffer) < 16 {
		s.fail("wav_fmt_truncated", "fmt chunk is shorter than 16 bytes")
		return
	}
	format := binary.LittleEndian.Uint16(s.fmtBuffer[0:2])
	s.channels = binary.LittleEndian.Uint16(s.fmtBuffer[2:4])
	s.sampleRate = binary.LittleEndian.Uint32(s.fmtBuffer[4:8])
	s.blockAlign = binary.LittleEndian.Uint16(s.fmtBuffer[12:14])
	s.bitsPerSample = binary.LittleEndian.Uint16(s.fmtBuffer[14:16])
	s.fmtSeen = true
	if format != 1 || s.bitsPerSample != 16 || s.channels == 0 || s.sampleRate == 0 || s.blockAlign != s.channels*2 {
		s.fail("unsupported_wav_format", fmt.Sprintf("requires PCM16 with consistent channels/rate/blockAlign, got format=%d channels=%d rate=%d align=%d bits=%d", format, s.channels, s.sampleRate, s.blockAlign, s.bitsPerSample))
	}
	s.fmtBuffer = nil
}

func (s *WAVScanner) fail(code, message string) {
	s.invalid = true
	s.diagnostics = append(s.diagnostics, Diagnostic{Code: code, Message: message})
}

func (s *WAVScanner) Finish(transportComplete bool) Result {
	result := Result{Diagnostics: append([]Diagnostic(nil), s.diagnostics...)}
	truncated := s.stage == wavRIFF || (s.stage == wavChunkHeader && len(s.buffer) != 0) ||
		(s.stage == wavChunkData && s.chunkRemaining != 0) || s.stage == wavChunkPadding
	if truncated {
		result.diagnostic("wav_truncated", "RIFF/WAVE ended before the declared chunk boundary")
	}
	measurement := Measurement{Meter: AudioSeconds, Source: "pcm16_samples", Completeness: Unknown}
	if s.fmtSeen && s.dataSeen && s.blockAlign > 0 && s.sampleRate > 0 {
		frames := s.dataBytes / int64(s.blockAlign)
		measurement.Value = rational(frames, int64(s.sampleRate))
		measurement.Completeness = Exact
		if !transportComplete || truncated || s.invalid || s.dataBytes%int64(s.blockAlign) != 0 {
			measurement.Completeness = Partial
		}
		if s.dataBytes%int64(s.blockAlign) != 0 {
			result.diagnostic("wav_incomplete_sample", "data chunk ends inside a PCM sample frame")
		}
	}
	result.set(measurement)
	return result
}
