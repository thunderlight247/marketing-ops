// Package migrations embeds the SQL migration files so the migrate command is
// a single self-contained binary.
package migrations

import "embed"

// FS holds every *.sql file in this directory.
//
//go:embed *.sql
var FS embed.FS
