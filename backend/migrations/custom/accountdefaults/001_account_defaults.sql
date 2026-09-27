-- 账号添加默认配置（二开 custom_account_defaults）
--
-- 存储「添加账号」表单的默认带入配置：
--   models_by_platform   按平台预选的模型白名单（空数组/缺失 = 该平台不限制模型）
--   proxy_mode           代理带入模式：fixed（固定 N 个，从 proxy_fixed_ids 带入）/ random（随机 1 个）
--   proxy_fixed_ids      固定模式下的代理 ID 列表（顺序即带入顺序）
--   allow_local_egress   固定模式下是否允许带入「本地直连」出口（默认关闭）
--   updated_at / updated_by 审计字段
CREATE TABLE IF NOT EXISTS custom_account_defaults (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT
);

INSERT INTO custom_account_defaults (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;
