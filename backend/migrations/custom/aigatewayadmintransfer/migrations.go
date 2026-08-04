// Package aigatewayadmintransfermigrations 嵌入 ai-gateway 管理员直连来源扣款的独立迁移文件。
package aigatewayadmintransfermigrations

import "embed"

// FS 只包含本 custom 领域 SQL，避免进入主仓迁移编号序列。
//
//go:embed *.sql
var FS embed.FS
