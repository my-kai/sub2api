# custom 公告 Markdown 图片上传

## 入口

- 管理端公告新增/编辑页：`/admin/announcements`
- 图片上传接口：`POST /api/v1/admin/custom/announcements/images`
- 图片访问接口：`GET /api/v1/custom/announcements/images/:filename`

## 行为

- 公告内容输入框使用 Markdown 编辑器。
- 复制图片后在编辑器中粘贴，会上传图片并自动插入 `![image](url)`。
- 图片支持 PNG、JPEG、GIF、WebP，单张最大 8MB。
- 图片按随机文件名保存到 `pricing.data_dir/custom/announcements/images`。

## 主仓接入点

- `backend/internal/server/router.go` 只注册 custom 公告图片路由。
- `frontend/src/views/admin/AnnouncementsView.vue` 只替换公告内容输入组件。
- 现有公告 CRUD、展示和已读逻辑保持不变。

## 验证

- 后端单测：`GOWORK=off go test ./internal/custom/announcements/... ./internal/server/...`
- 前端类型检查：`pnpm --dir frontend exec vue-tsc --noEmit`
