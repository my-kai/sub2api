-- 已执行的 001 迁移受 custom checksum 保护；通过新增迁移扩展转入记录金额精度，避免修改其内容导致生产启动失败。
ALTER TABLE custom_ai_gateway_admin_transfers
    ALTER COLUMN amount_usd TYPE NUMERIC(20,8);
