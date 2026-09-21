package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

const maxFieldLength = 512

var forbiddenKeys = map[string]struct{}{
	"authorization": {}, "cookie": {}, "set-cookie": {}, "token": {}, "access_token": {}, "refresh_token": {},
	"password": {}, "secret": {}, "csrf": {}, "prompt": {}, "body": {}, "claims": {}, "toolref": {}, "username": {}, "display_name": {},
}

var sensitiveText = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[^\s]+`),
	regexp.MustCompile(`(?i)(token|password|secret|cookie|csrf)[=:]\s*[^\s,;]+`),
	regexp.MustCompile(`https?://[^\s]+`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
}

type safeHandler struct{ next slog.Handler }

func NewLogger(output io.Writer, service, buildVersion string) *slog.Logger {
	base := slog.NewJSONHandler(output, nil).WithAttrs([]slog.Attr{
		slog.String("service", bounded(service)),
		slog.String("buildVersion", bounded(buildVersion)),
	})
	return slog.New(safeHandler{next: base})
}

func (h safeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}
func (h safeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return safeHandler{next: h.next.WithAttrs(safeAttrs(attrs))}
}
func (h safeHandler) WithGroup(name string) slog.Handler {
	return safeHandler{next: h.next.WithGroup(bounded(name))}
}
func (h safeHandler) Handle(ctx context.Context, record slog.Record) error {
	copy := slog.NewRecord(record.Time, record.Level, bounded(record.Message), record.PC)
	hasEvent := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "event" {
			hasEvent = true
		}
		copy.AddAttrs(safeAttr(attr))
		return true
	})
	if !hasEvent {
		copy.AddAttrs(slog.String("event", "log.message"))
	}
	return h.next.Handle(ctx, copy)
}

func safeAttrs(attrs []slog.Attr) []slog.Attr {
	result := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		result[i] = safeAttr(attr)
	}
	return result
}

func safeAttr(attr slog.Attr) slog.Attr {
	key := strings.ToLower(strings.ReplaceAll(attr.Key, "-", "_"))
	if _, forbidden := forbiddenKeys[key]; forbidden || containsForbiddenKey(key) {
		return slog.String(attr.Key, "[redacted]")
	}
	value := attr.Value.Resolve()
	if key == "error" {
		if value.Kind() == slog.KindAny {
			if err, ok := value.Any().(error); ok {
				return slog.String(attr.Key, SafeError(err))
			}
		}
		return slog.String(attr.Key, "[error details redacted]")
	}
	if value.Kind() == slog.KindGroup {
		return slog.Group(attr.Key, anySlice(safeAttrs(value.Group()))...)
	}
	if value.Kind() == slog.KindString {
		return slog.String(attr.Key, redactText(value.String()))
	}
	if value.Kind() == slog.KindAny {
		if err, ok := value.Any().(error); ok {
			return slog.String(attr.Key, SafeError(err))
		}
		return slog.String(attr.Key, bounded(fmt.Sprint(value.Any())))
	}
	return slog.Attr{Key: attr.Key, Value: value}
}

func anySlice(attrs []slog.Attr) []any {
	values := make([]any, len(attrs))
	for i := range attrs {
		values[i] = attrs[i]
	}
	return values
}

func containsForbiddenKey(key string) bool {
	for value := range forbiddenKeys {
		if strings.Contains(key, value) {
			return true
		}
	}
	return false
}

func bounded(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return ' '
		}
		return r
	}, value)
	if len(value) <= maxFieldLength {
		return value
	}
	return value[:maxFieldLength] + "…"
}

func redactText(value string) string {
	for _, pattern := range sensitiveText {
		value = pattern.ReplaceAllString(value, "[redacted]")
	}
	return bounded(value)
}

func SafeError(err error) string {
	if err == nil {
		return ""
	}
	return bounded(fmt.Sprintf("%T", err))
}

type Logger struct{ Log *slog.Logger }

func (l Logger) RequestCompleted(ctx context.Context, route, method string, status int, duration time.Duration) {
	if l.Log == nil {
		return
	}
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	} else if status >= 400 {
		level = slog.LevelWarn
	}
	l.Log.Log(ctx, level, "request completed", "event", "http.request_completed", "route", bounded(route), "method", method, "httpStatus", status, "durationMs", duration.Milliseconds())
}
