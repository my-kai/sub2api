package accountdefaultsmigrations

import "embed"

// FS embeds the isolated account-defaults schema so it does not consume upstream
// migration numbers.
//
//go:embed *.sql
var FS embed.FS
