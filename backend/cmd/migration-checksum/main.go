// Command migration-checksum regenerates atlas.sum with the pinned Atlas library.
// It does not apply or adopt any production migration history.
package main

import (
	"ariga.io/atlas/sql/migrate"
	"flag"
	"log"
)

func main() {
	path := flag.String("dir", "migrations", "migration directory")
	flag.Parse()
	dir, err := migrate.NewLocalDir(*path)
	if err != nil {
		log.Fatal(err)
	}
	sum, err := dir.Checksum()
	if err != nil {
		log.Fatal(err)
	}
	if err := migrate.WriteSumFile(dir, sum); err != nil {
		log.Fatal(err)
	}
}
