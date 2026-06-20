package activitymigrations

import "embed"

// FS embeds only the custom activity SQL files in this directory.
//
// The main migration runner still owns backend/migrations/*.sql. Keeping this
// embedded filesystem separate lets the custom activity runtime apply its own
// schema without changing upstream migration discovery.
//
//go:embed *.sql
var FS embed.FS
