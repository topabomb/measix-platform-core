// Package migrations exposes the single current initialization schema to binary
// diagnostics and tests. Production initialization remains Atlas-owned.
package migrations

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
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

func CurrentIdentity() string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(CurrentSQL())))
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
