package modelratemigrations

import "embed"

// FS embeds the independent model-rate migration set.
//
// The custom runtime tracks these files separately from the upstream migration
// sequence so upstream merges do not create migration-number conflicts.
//
//go:embed *.sql
var FS embed.FS
