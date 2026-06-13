package imagegenmigrations

import "embed"

// FS embeds only the custom image generation SQL files in this directory.
//
// The main migration runner still embeds backend/migrations/*.sql only; this
// package gives the custom runner an explicit release-build source without
// changing the upstream migration discovery rule.
//
//go:embed *.sql
var FS embed.FS
