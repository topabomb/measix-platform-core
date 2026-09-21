package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxTailBytes = 2 << 20
	maxLineBytes = 4 << 10
	maxEvents    = 200
)

type Event struct {
	Time            time.Time `json:"time"`
	Service         string    `json:"service"`
	Level           string    `json:"level"`
	Event           string    `json:"event"`
	Message         string    `json:"message"`
	RequestID       string    `json:"requestId,omitempty"`
	InteractionID   string    `json:"interactionId,omitempty"`
	ActivationID    string    `json:"activationId,omitempty"`
	DeploymentID    string    `json:"deploymentId,omitempty"`
	ControlRevision *int64    `json:"controlRevision,omitempty"`
	ResourceID      string    `json:"resourceId,omitempty"`
	DurationMs      *int64    `json:"durationMs,omitempty"`
	HTTPStatus      *int      `json:"httpStatus,omitempty"`
	Outcome         string    `json:"outcome,omitempty"`
	ErrorCode       string    `json:"errorCode,omitempty"`
	Truncated       bool      `json:"truncated,omitempty"`
}

type EventFilter struct {
	Service     string
	Level       string
	Event       string
	Correlation string
	Limit       int
}

type EventPage struct {
	Items     []Event `json:"items"`
	Truncated bool    `json:"truncated"`
}

type logFile struct {
	name, service string
	stderr        bool
}

var diagnosticFiles = []logFile{
	{name: "hub.jsonl", service: "HUB"},
	{name: "hub.stderr.log", service: "HUB", stderr: true},
	{name: "relay.jsonl", service: "RELAY"},
	{name: "relay.stderr.log", service: "RELAY", stderr: true},
}

func ReadEvents(logDir string, filter EventFilter) (EventPage, error) {
	if strings.TrimSpace(logDir) == "" {
		return EventPage{}, fmt.Errorf("diagnostics log directory is not configured")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxEvents {
		limit = maxEvents
	}
	filter.Service = strings.ToUpper(filter.Service)
	filter.Level = strings.ToUpper(filter.Level)
	var all []Event
	truncated := false
	for _, file := range diagnosticFiles {
		if filter.Service != "" && filter.Service != file.service {
			continue
		}
		lines, partial, observedAt, err := tailLines(filepath.Join(logDir, file.name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return EventPage{}, err
		}
		truncated = truncated || partial
		for _, line := range lines {
			event, ok := parseEvent(line, file, observedAt)
			if !ok || !matchesEvent(event, filter) {
				continue
			}
			all = append(all, event)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time.After(all[j].Time) })
	if len(all) > limit {
		all = all[:limit]
		truncated = true
	}
	return EventPage{Items: all, Truncated: truncated}, nil
}

func tailLines(path string) ([][]byte, bool, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, time.Time{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, time.Time{}, err
	}
	start := info.Size() - maxTailBytes
	truncated := start > 0
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, false, time.Time{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTailBytes))
	if err != nil {
		return nil, false, time.Time{}, err
	}
	lines := bytes.Split(data, []byte{'\n'})
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, truncated, info.ModTime().UTC(), nil
}

func parseEvent(line []byte, file logFile, observedAt time.Time) (Event, bool) {
	if len(line) == 0 {
		return Event{}, false
	}
	lineTruncated := false
	if len(line) > maxLineBytes {
		line = line[:maxLineBytes]
		lineTruncated = true
	}
	if file.stderr {
		return Event{Time: observedAt, Service: file.service, Level: "ERROR", Event: "process.stderr", Message: redactText(string(line)), Truncated: lineTruncated}, true
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(line, &row); err != nil {
		return Event{Time: observedAt, Service: file.service, Level: "ERROR", Event: "process.stdout_unparseable", Message: "Unparseable process output", Truncated: lineTruncated}, true
	}
	event := Event{Service: file.service, Truncated: lineTruncated}
	decodeString(row, "level", &event.Level)
	decodeString(row, "event", &event.Event)
	decodeString(row, "msg", &event.Message)
	decodeString(row, "requestId", &event.RequestID)
	decodeString(row, "interactionId", &event.InteractionID)
	decodeString(row, "activationId", &event.ActivationID)
	decodeString(row, "deploymentId", &event.DeploymentID)
	decodeString(row, "resourceId", &event.ResourceID)
	decodeString(row, "outcome", &event.Outcome)
	decodeString(row, "errorCode", &event.ErrorCode)
	_ = json.Unmarshal(row["time"], &event.Time)
	if event.Time.IsZero() {
		event.Time = observedAt
	}
	decodeInt64(row, "controlRevision", &event.ControlRevision)
	decodeInt64(row, "durationMs", &event.DurationMs)
	var status *int64
	decodeInt64(row, "httpStatus", &status)
	if status != nil {
		value := int(*status)
		event.HTTPStatus = &value
	}
	event.Level = strings.ToUpper(bounded(event.Level))
	event.Event = bounded(event.Event)
	event.Message = redactText(event.Message)
	if event.Event == "" {
		event.Event = "process.stdout"
	}
	return event, true
}

func decodeString(row map[string]json.RawMessage, key string, target *string) {
	var value string
	if json.Unmarshal(row[key], &value) == nil {
		*target = bounded(value)
	}
}

func decodeInt64(row map[string]json.RawMessage, key string, target **int64) {
	var value int64
	if json.Unmarshal(row[key], &value) == nil {
		*target = &value
	}
}

func matchesEvent(event Event, filter EventFilter) bool {
	if filter.Level != "" && event.Level != filter.Level {
		return false
	}
	if filter.Event != "" && event.Event != filter.Event {
		return false
	}
	if filter.Correlation != "" {
		needle := strings.ToLower(filter.Correlation)
		values := []string{event.RequestID, event.InteractionID, event.ActivationID, event.DeploymentID, event.ResourceID}
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), needle) {
				return true
			}
		}
		return false
	}
	return true
}
