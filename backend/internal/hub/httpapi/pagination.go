package httpapi

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// Cursors are opaque positions, bound to the endpoint and active filter set.
// They grant no authority; each page is authenticated and filtered normally.
func pageParams(w http.ResponseWriter, r *http.Request, requestedLimit *int, cursor *string) (int, string, bool) {
	limit := 50
	if requestedLimit != nil {
		limit = *requestedLimit
	}
	if limit < 1 || limit > 200 {
		writeProblem(w, 400, "invalid_request", "Invalid page limit")
		return 0, "", false
	}
	if cursor == nil {
		return limit, "", true
	}
	if len(*cursor) > 4096 {
		writeProblem(w, 400, "invalid_request", "Invalid cursor")
		return 0, "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*cursor)
	parts := strings.SplitN(string(decoded), "\x00", 2)
	if err != nil || len(parts) != 2 || parts[0] != pageScope(r) || parts[1] == "" {
		writeProblem(w, 400, "invalid_request", "Invalid cursor")
		return 0, "", false
	}
	return limit, parts[1], true
}

func pageScope(r *http.Request) string {
	query := r.URL.Query()
	query.Del("cursor")
	query.Del("limit")
	return r.URL.Path + "?" + query.Encode()
}

func pageResult[T any](r *http.Request, rows []T, limit int, key func(T) string) ([]T, *string) {
	if len(rows) <= limit {
		return rows, nil
	}
	rows = rows[:limit]
	cursor := base64.RawURLEncoding.EncodeToString([]byte(pageScope(r) + "\x00" + key(rows[len(rows)-1])))
	return rows, &cursor
}
