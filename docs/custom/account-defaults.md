# 账号添加默认配置（account defaults）

管理端「系统设置 → 账号配置」Tab：配置新建账号表单默认带入的模型白名单与出口代理。

## 功能

- 模型按平台配置白名单（openai / anthropic / gemini / antigravity / grok / kimi / zhipu / deepseek / minimax / opencode_go）。
  - 新建账号打开表单时按当前平台带入；切换平台时重新带入当前平台的默认。
  - 未配置（空列表）= 不限制模型，表单白名单保持为空（支持全部模型）。
  - 带入的是「白名单模式」（`allowedModels`），表单内可手动增删；mapping 模式不做默认。
- 代理带入二选一：
  - **固定 N 个**：手动勾选代理列表（`proxy_fixed_ids`），打开表单时带入全部仍可用的配置代理（按配置顺序，失效的自动跳过）；可选「同时允许本地直连」（`allow_local_egress`）。
  - **随机 1 个**：打开表单时从代理池（状态正常且未过期的全部代理）随机抽 1 个带入；池空则不带入。
  - 均为自动带入、可手动修改；不强制。编辑账号不应用默认值。
- 随机抽样的时机 = 打开新建账号表单时（服务端 `resolve` 接口抽一次），提交前可改。

## 后端

- 模块：`backend/internal/custom/accountdefaults/`
- 数据表：`custom_account_defaults`（单行 JSONB 配置 + 审计字段），独立迁移 `backend/migrations/custom/accountdefaults/`，随服务启动自动执行（advisory lock 串行化，checksum 防篡改），不占主仓迁移编号。
- API（挂 `/api/v1/admin/custom/...` 独立命名空间，管理员鉴权 + 审计日志 + 合规守卫）：
  - `GET /api/v1/admin/custom/account-defaults` 读取配置
  - `PUT /api/v1/admin/custom/account-defaults` 保存配置（校验：固定模式至少一个代理或允许直连）
  - `GET /api/v1/admin/custom/account-defaults/resolve` 表单解析（random 模式服务端抽 1 个）

## 前端

- Tab 内容组件：`frontend/src/custom/accountDefaults/AccountDefaultsSettings.vue`（Card 外层，复用 SettingsView 底部的通用保存按钮）。
- API wrapper：`frontend/src/custom/accountDefaults/api.ts`。
- 主仓薄接入：
  - `SettingsView.vue`：tab key `accountDefaults` + v-show 区块 + import，并通过通用保存按钮触发账号配置保存。
  - `CreateAccountModal.vue`：打开时调 `resolveAccountDefaults()` 带入模型/代理默认（拉取失败静默降级，不阻断建号）。
  - i18n：`admin.settings.tabs.accountDefaults` + `admin.settings.accountDefaults.*`（zh/en）。

## 验证

- 后端：`go test ./internal/custom/accountdefaults/...`（归一化、随机/固定解析、失效代理过滤、校验）。
- 前端：`vue-tsc --noEmit`、`CreateAccountModal.spec.ts`、`SettingsView.spec.ts`、locale key 完整性测试。
- 手工：系统设置 → 账号配置保存后，新建账号确认模型与代理按配置带入且可修改。

## 与主仓接入点

| 类型 | 文件 | 改动 |
|---|---|---|
| 薄接入 | `backend/internal/server/http.go`、`router.go` | 路由注册参数与调用 |
| 薄接入 | `backend/cmd/server/wire.go`、`wire_gen.go` | ProviderSet 注册、ProvideRouter 传参 |
| 薄接入 | `frontend/src/views/admin/SettingsView.vue` | tab 注册与内容区块 |
| 薄接入 | `frontend/src/components/account/CreateAccountModal.vue` | 打开/切平台时带入默认值 |
| 薄接入 | `frontend/src/i18n/locales/{zh,en}/admin/settings.ts` | 文案键追加 |
| 测试适配 | `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`、`SettingsView.spec.ts` | mock/stub 追加 |
