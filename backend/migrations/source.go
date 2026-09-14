// Package migrations exposes the single current initialization schema to binary
// diagnostics and tests. Production initialization remains Atlas-owned.
package migrations

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed *.sql
var Source embed.FS

func Names() []string {
	names, err := fs.Glob(Source, "*.sql")
	if err != nil || len(names) == 0 {
		panic("missing embedded migrations")
	}
	return names
}

func CurrentRevision() string {
	names := Names()
	return strings.TrimSuffix(names[len(names)-1], ".sql")
}

func CurrentSQL() string {
	names := Names()
	if len(names) != 1 {
		panic("exactly one current initialization schema is required")
	}
	data, err := Source.ReadFile(names[0])
	if err != nil {
		panic(err)
	}
	return string(data)
}
