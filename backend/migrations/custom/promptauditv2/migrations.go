package promptauditv2migrations

import "embed"

// FS contains only the prompt-audit-v2 schema. The custom runtime applies it
// with an isolated checksum table so upstream migration numbering stays intact.
//
//go:embed *.sql
var FS embed.FS
