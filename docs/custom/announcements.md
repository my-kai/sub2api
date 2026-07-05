# custom 公告 Markdown 图片上传

## 入口

- 管理端公告新增/编辑页：`/admin/announcements`
- 图片上传接口：`POST /api/v1/admin/custom/announcements/images`
- 图片访问接口：`GET /api/v1/custom/announcements/images/:filename`

## 行为

- 公告内容输入框使用 Markdown 编辑器。
- 复制图片后在编辑器中粘贴，会上传图片并自动插入 `![image](url)`。
- 使用编辑器工具栏里的链接按钮插入链接时，可选择“当前页打开”或“新窗口打开”；编辑器预览区点击已有链接可修改链接地址、链接文字和打开方式。链接文字可留空，留空时使用链接地址作为展示文本。
- 图片工具栏隐藏默认的“添加链接”菜单项，避免与公告链接配置入口重复。
- 链接打开方式随 Markdown 内容一起保存：当前页打开使用标准 Markdown 链接，新窗口打开使用带 `target="_blank"` 和 `rel="noopener noreferrer"` 的 HTML 链接。
- 图片支持 PNG、JPEG、GIF、WebP，单张最大 8MB。
- 图片按随机文件名保存到 `pricing.data_dir/custom/announcements/images`。

## 主仓接入点

- `backend/internal/server/router.go` 只注册 custom 公告图片路由。
- `frontend/src/views/admin/AnnouncementsView.vue` 只替换公告内容输入组件。
- `frontend/src/custom/announcements/linkOpenMode.ts` 负责公告 Markdown 链接打开方式的生成、定位和替换。
- `frontend/src/custom/announcements/renderMarkdown.ts` 负责公告展示时的 Markdown 渲染和链接属性清理。
- 现有公告 CRUD、展示和已读逻辑保持不变。

## 验证

- 前端单测：`pnpm --dir frontend exec vitest run src/custom/announcements/__tests__/linkOpenMode.spec.ts`
- 后端单测：`GOWORK=off go test ./internal/custom/announcements/... ./internal/server/...`
- 前端类型检查：`pnpm --dir frontend exec vue-tsc --noEmit`
