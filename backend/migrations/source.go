// Package migrations exposes the same immutable SQL files to binary diagnostics
// and tests. Production application of migrations remains Atlas-owned.
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

func SQLAfter(revision string) string {
	var out strings.Builder
	for _, name := range Names() {
		if strings.TrimSuffix(name, ".sql") <= revision {
			continue
		}
		data, err := Source.ReadFile(name)
		if err != nil {
			panic(err)
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	return out.String()
}
