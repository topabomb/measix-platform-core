/**
 * devmigrate messages that mean "this database was initialized from a different
 * revision of the current SQL". This repository keeps one current initialization
 * SQL and never upgrades data, so such a database is obsolete development state
 * and is deleted and recreated rather than migrated.
 *
 * The list is matched against devmigrate's own output, so
 * `scripts/dev-setup-obsolete.test.mjs` asserts every marker still appears
 * verbatim in `backend/cmd/devmigrate/main.go`. Rewording a Go message without
 * updating this list would silently disable the replacement path. A database
 * that is merely unrecognized is deliberately absent: that state is left for its
 * owner to resolve rather than deleted.
 */
export const OBSOLETE_DATABASE_MARKERS = [
  'non-current database',
  'non-current schema/checksum',
  'non-current initialization record',
]

export function isObsoleteDatabaseError(output) {
  return OBSOLETE_DATABASE_MARKERS.some(marker => String(output ?? '').includes(marker))
}
