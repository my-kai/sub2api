package giftcreditmigrations

import "embed"

// FS embeds only the custom gift-credit SQL files in this directory.
//
// The main migration runner owns backend/migrations/*.sql. Keeping this
// filesystem separate avoids upstream migration-number conflicts.
//
//go:embed *.sql
var FS embed.FS
