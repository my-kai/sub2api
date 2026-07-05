# custom 开票申请与抬头管理

## 入口

- 用户抬头管理页：`/invoice-titles`
- 用户开票申请页：`/invoices`
- 管理端开票管理页：`/admin/custom/invoices`
- 用户接口前缀：`/api/v1/custom/invoices`
- 管理接口前缀：`/api/v1/admin/custom/invoices`
- 测试发件接口：`POST /api/v1/admin/custom/invoice-test-email`，后端生成测试开票信息，不依赖真实申请记录。
- 已开票通知补发接口：`POST /api/v1/admin/custom/invoices/:id/test-email`
- 临时下载接口：`/api/v1/custom/invoice-downloads/:token`

## 行为

- 开票申请只能选择当前用户的余额充值订单。
- 可选订单限定为 `order_type = 'balance'`，且状态为 `COMPLETED`。`PAID`、`RECHARGING` 仍属于支付/充值处理中间态，不能申请开票。
- 一个开票申请可以合并多笔充值订单。
- `pending`、`issued` 状态的申请会占用订单；`rejected` 状态释放订单。
- 申请只选择已有抬头，创建申请时保存抬头快照。
- 当前仅支持企业普通发票：`enterprise_vat_normal`。
- 管理员标记已开票时必须填写发票号码、备注，并上传 PDF。
- 开票完成必须向抬头接收邮箱发送通知邮件，邮件内容使用现有通知邮件模板体系；优先把本次发票 PDF 作为附件发送。
- 管理端列表顶部提供测试发件入口，弹窗选择已开票申请后发送；测试发件复用同一套模板和附件/临时链接规则，不改变申请状态。
- 如果附件邮件发送失败，系统改发无需登录的临时下载链接；链接默认 24 小时过期，过期后不能下载。
- 附件邮件和临时链接邮件都发送失败时，本次开票完成操作失败，申请保持可重试状态。
- 发票文件仅支持单个 PDF，最大 10MB，保存到 `pricing.data_dir/custom/invoices/YYYY/MM/`。

## 数据表

- `custom_invoice_titles`：用户企业发票抬头，支持默认抬头和软删除。
- `custom_invoice_applications`：开票申请、`INV{YYYYMMDD}-{10位无序码}` 申请编号、抬头快照、状态、发票号码、备注和 PDF 文件索引。
- `custom_invoice_application_orders`：申请与充值订单的绑定关系。
- `custom_invoice_schema_migrations`：custom 开票独立迁移记录。

## 主仓接入点

- `backend/internal/server/router.go` 只注册 custom 开票用户/管理员路由。
- `backend/internal/server/http.go`、`backend/cmd/server/wire.go`、`backend/cmd/server/wire_gen.go` 只注入 custom 开票 bundle。
- `frontend/src/router/index.ts` 只追加 custom 开票路由记录。
- `frontend/src/components/layout/AppSidebar.vue` 只追加用户侧和管理员侧菜单入口。

未修改主仓支付订单履约、余额入账、退款、ent schema 和主迁移编号。

## 验证

- 后端：`cd backend && go test ./internal/custom/invoice/... ./internal/service ./internal/server ./cmd/server`
- 前端类型检查：`npx -y pnpm@10.23.0 --dir frontend typecheck`
- 前端 lint：`npx -y pnpm@10.23.0 --dir frontend exec eslint src/custom/invoice src/router/index.ts src/components/layout/AppSidebar.vue --ext .vue,.ts`
- 前端构建：`npx -y pnpm@10.23.0 --dir frontend build`
- Diff 检查：`git diff --check`
