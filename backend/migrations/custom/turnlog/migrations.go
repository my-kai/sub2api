package turnlogmigrations

import "embed"

// FS embeds the isolated turn-log schema so it does not consume upstream migration numbers.
//
//go:embed *.sql
var FS embed.FS
