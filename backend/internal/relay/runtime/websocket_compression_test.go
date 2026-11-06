package runtime

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"strings"
	"testing"
)

func TestDeflateContextTakeoverPreservesRollingDictionary(t *testing.T) {
	messages := [][]byte{[]byte(`{"type":"review","value":"` + strings.Repeat("dictionary-unique-", 80) + `"}`), []byte(`{"type":"small"}`)}
	messages = append(messages, messages[0])
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	var observed []byte
	failed := false
	observer := newWebSocketFrameObserver(false, true, false,
		func(message []byte) { observed = append([]byte(nil), message...) },
		func() { failed = true })
	for index, message := range messages {
		compressed.Reset()
		if _, err := writer.Write(message); err != nil {
			t.Fatal(err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatal(err)
		}
		payload := append([]byte(nil), compressed.Bytes()...)
		if !bytes.HasSuffix(payload, []byte{0, 0, 255, 255}) {
			t.Fatal("missing permessage-deflate flush trailer")
		}
		payload = payload[:len(payload)-4]
		header := []byte{0xc1, byte(len(payload))}
		if len(payload) >= 126 {
			header = []byte{0xc1, 126, 0, 0}
			binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
		}
		observer.Observe(append(header, payload...))
		if failed || !bytes.Equal(observed, message) {
			t.Fatalf("valid compressed message %d rejected or changed: failed=%v observedBytes=%d wantBytes=%d compressedBytes=%d", index+1, failed, len(observed), len(message), len(payload))
		}
	}
}

func TestDeflateRejectsInvalidAndOversizedObservation(t *testing.T) {
	var encoded bytes.Buffer
	writer, err := flate.NewWriter(&encoded, flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(bytes.Repeat([]byte("a"), maxObservedWebSocketMessage+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte(nil), encoded.Bytes()[:encoded.Len()-4]...)
	_ = writer.Close()
	for _, payload := range [][]byte{{0xff, 0xff}, payload} {
		o := newWebSocketFrameObserver(false, true, true, func([]byte) { t.Fatal("invalid observation delivered") }, func() {})
		if _, err := o.inflate(payload); err == nil {
			t.Fatal("invalid/oversized compressed observation accepted")
		}
		if len(o.dictionary) != 0 {
			t.Fatal("failed observation retained dictionary")
		}
	}
}
