package runtime

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxObservedWebSocketMessage = 4 << 20

// upgradedStream preserves the original WebSocket byte stream while two
// direction-specific parsers reconstruct text messages for realtime usage.
type upgradedStream struct {
	io.ReadWriteCloser
	requestBytes, responseBytes atomic.Int64
	timedOut, exceeded          atomic.Bool
	limit                       int64
	idle                        time.Duration
	mu                          sync.Mutex
	timer                       *time.Timer
	closed                      bool
	lastActivity                time.Time
	clientFrames                *webSocketFrameObserver
	serverFrames                *webSocketFrameObserver
}

func newUpgradedStream(conn io.ReadWriteCloser, idle time.Duration, limit int64, observation *usageObservation, extensions string) *upgradedStream {
	compression := parsePerMessageDeflate(extensions)
	s := &upgradedStream{
		ReadWriteCloser: conn, idle: idle, limit: limit, lastActivity: time.Now(),
		clientFrames: newWebSocketFrameObserver(true, compression.enabled, compression.clientNoContextTakeover, observation.observeRealtimeClient, observation.markRealtimeFailure),
		serverFrames: newWebSocketFrameObserver(false, compression.enabled, compression.serverNoContextTakeover, observation.observeRealtimeServer, observation.markRealtimeFailure),
	}
	s.mu.Lock()
	s.timer = time.AfterFunc(idle, s.checkIdle)
	s.mu.Unlock()
	return s
}

func (s *upgradedStream) checkIdle() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	remaining := s.idle - time.Since(s.lastActivity)
	if remaining > 0 {
		s.timer.Reset(remaining)
		s.mu.Unlock()
		return
	}
	s.timedOut.Store(true)
	s.mu.Unlock()
	_ = s.Close()
}

func (s *upgradedStream) activity() {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

func (s *upgradedStream) Read(p []byte) (int, error) {
	n, err := s.ReadWriteCloser.Read(p)
	if n > 0 {
		s.responseBytes.Add(int64(n))
		s.activity()
		s.serverFrames.Observe(p[:n])
	}
	return n, err
}

func (s *upgradedStream) Write(p []byte) (int, error) {
	if int64(len(p)) > s.limit-s.requestBytes.Load() {
		s.exceeded.Store(true)
		_ = s.Close()
		return 0, errors.New("WebSocket request byte limit exceeded")
	}
	n, err := s.ReadWriteCloser.Write(p)
	if n > 0 {
		s.requestBytes.Add(int64(n))
		s.activity()
		s.clientFrames.Observe(p[:n])
	}
	return n, err
}

func (s *upgradedStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.timer.Stop()
	s.mu.Unlock()
	return s.ReadWriteCloser.Close()
}

type perMessageDeflate struct {
	enabled, clientNoContextTakeover, serverNoContextTakeover bool
}

func parsePerMessageDeflate(value string) perMessageDeflate {
	for _, extension := range strings.Split(value, ",") {
		parts := strings.Split(extension, ";")
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "permessage-deflate") {
			continue
		}
		result := perMessageDeflate{enabled: true}
		for _, parameter := range parts[1:] {
			name := strings.ToLower(strings.TrimSpace(strings.SplitN(parameter, "=", 2)[0]))
			switch name {
			case "client_no_context_takeover":
				result.clientNoContextTakeover = true
			case "server_no_context_takeover":
				result.serverNoContextTakeover = true
			}
		}
		return result
	}
	return perMessageDeflate{}
}

type webSocketFrameObserver struct {
	expectMasked, compression, noContextTakeover bool
	buffer, message, dictionary                  []byte
	fragmented, compressed, failed               bool
	messageOpcode                                byte
	onText                                       func([]byte)
	onFailure                                    func()
}

func newWebSocketFrameObserver(masked, compression, noContextTakeover bool, onText func([]byte), failure func()) *webSocketFrameObserver {
	return &webSocketFrameObserver{expectMasked: masked, compression: compression, noContextTakeover: noContextTakeover, onText: onText, onFailure: failure}
}

func (o *webSocketFrameObserver) Observe(chunk []byte) {
	if o == nil || o.failed || len(chunk) == 0 {
		return
	}
	o.buffer = append(o.buffer, chunk...)
	for o.consumeFrame() {
	}
}

func (o *webSocketFrameObserver) consumeFrame() bool {
	if len(o.buffer) < 2 {
		return false
	}
	first, second := o.buffer[0], o.buffer[1]
	fin, rsv1, opcode := first&0x80 != 0, first&0x40 != 0, first&0x0f
	masked := second&0x80 != 0
	if masked != o.expectMasked || first&0x30 != 0 || (rsv1 && (!o.compression || opcode == 0)) {
		o.fail()
		return false
	}
	offset := 2
	payloadLength := uint64(second & 0x7f)
	switch payloadLength {
	case 126:
		if len(o.buffer) < offset+2 {
			return false
		}
		payloadLength = uint64(binary.BigEndian.Uint16(o.buffer[offset:]))
		offset += 2
	case 127:
		if len(o.buffer) < offset+8 {
			return false
		}
		payloadLength = binary.BigEndian.Uint64(o.buffer[offset:])
		if payloadLength>>63 != 0 {
			o.fail()
			return false
		}
		offset += 8
	}
	if opcode >= 8 && (!fin || payloadLength > 125) {
		o.fail()
		return false
	}
	var mask []byte
	if masked {
		if len(o.buffer) < offset+4 {
			return false
		}
		mask = o.buffer[offset : offset+4]
		offset += 4
	}
	if payloadLength > maxObservedWebSocketMessage || uint64(len(o.buffer)-offset) < payloadLength {
		if payloadLength > maxObservedWebSocketMessage {
			o.fail()
		}
		return false
	}
	payload := append([]byte(nil), o.buffer[offset:offset+int(payloadLength)]...)
	o.buffer = o.buffer[offset+int(payloadLength):]
	for index := range payload {
		if masked {
			payload[index] ^= mask[index%4]
		}
	}
	if opcode >= 8 {
		return len(o.buffer) > 0
	}
	if opcode == 1 || opcode == 2 {
		if o.fragmented {
			o.fail()
			return false
		}
		o.fragmented, o.compressed = !fin, rsv1
		o.messageOpcode = opcode
		o.message = append(o.message[:0], payload...)
	} else if opcode == 0 {
		if !o.fragmented {
			o.fail()
			return false
		}
		o.message = append(o.message, payload...)
		o.fragmented = !fin
	} else {
		o.fail()
		return false
	}
	if len(o.message) > maxObservedWebSocketMessage {
		o.fail()
		return false
	}
	if fin {
		message := o.message
		if o.compressed {
			var err error
			message, err = o.inflate(message)
			if err != nil {
				o.fail()
				return false
			}
		}
		if o.messageOpcode == 1 {
			o.onText(message)
		}
		o.message, o.fragmented, o.compressed = o.message[:0], false, false
		o.messageOpcode = 0
	}
	return len(o.buffer) > 0
}

func (o *webSocketFrameObserver) inflate(compressed []byte) ([]byte, error) {
	payload := append(append([]byte(nil), compressed...), 0x00, 0x00, 0xff, 0xff)
	var reader io.ReadCloser
	if len(o.dictionary) > 0 && !o.noContextTakeover {
		reader = flate.NewReaderDict(bytes.NewReader(payload), o.dictionary)
	} else {
		reader = flate.NewReader(bytes.NewReader(payload))
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, maxObservedWebSocketMessage+1))
	if err != nil || len(decoded) > maxObservedWebSocketMessage {
		return nil, errors.New("invalid or oversized permessage-deflate payload")
	}
	if o.noContextTakeover {
		o.dictionary = nil
	} else if len(decoded) > 32768 {
		o.dictionary = append(o.dictionary[:0], decoded[len(decoded)-32768:]...)
	} else {
		o.dictionary = append(o.dictionary[:0], decoded...)
	}
	return decoded, nil
}

func (o *webSocketFrameObserver) fail() {
	if !o.failed {
		o.failed = true
		o.onFailure()
	}
}
