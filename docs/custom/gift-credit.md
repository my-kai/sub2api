# 赠送余额说明

## 口径

- `users.balance` 继续表示普通余额。
- 赠送余额单独存储，每次发放生成一条 grant，并拥有独立过期时间。
- AI 调用消费时先抵扣未过期赠送余额，剩余金额继续走原普通余额扣费逻辑。
- 首轮不修改原普通余额扣费的并发控制、幂等 key、结算时机和 cache 扣减策略。
- `gift_balance` 表示当前未过期赠送余额，`available_balance` 表示普通余额与赠送余额合计后的可用额度。

## 存储

- `custom_gift_credit_grants`：每次赠送余额发放记录。
- `custom_gift_credit_deductions`：每次请求从哪些 grant 扣了多少。
- `custom_gift_credit_user_balances`：用户赠送余额聚合快照，用于请求准入 O(1) 查询。
- `custom_gift_credit_schema_migrations`：赠送余额独立迁移记录。

## 迁移

- 赠送余额基础表：`backend/migrations/custom/giftcredit/001_gift_credit.sql`。
- 赠送余额来源约束：`backend/migrations/custom/giftcredit/002_require_grant_source_id.sql`。
- 活动配置补充有效期字段：`backend/migrations/custom/activity/003_custom_activity_gift_credit_validity.sql`。
- 活动配置显式有效期约束：`backend/migrations/custom/activity/004_remove_gift_validity_default.sql`。
- custom 迁移独立于主仓 `backend/migrations/**`，避免和上游迁移编号冲突。

## 来源

- `activity_reward`：活动奖励。
- `admin_grant`：管理员手动赠送余额。
- `promo_code`：优惠码兑换赠送余额。

## 发放入口

### 管理员手动调整余额

- 管理端用户余额弹窗支持选择入账类型：
  - `balance`：进入普通余额，沿用原余额增加逻辑。
  - `gift`：进入赠送余额，生成独立 grant。
- `gift` 只支持增加余额，不支持扣减；扣减仍只能走普通余额原逻辑。
- 选择 `gift` 时必须显式传入大于 0 的 `gift_validity_days`；未传或传 0 会直接失败，不做默认有效期。

### 优惠码

- 优惠码新增和编辑支持 `credit_type`：
  - `balance`：兑换后进入普通余额。
  - `gift`：兑换后进入赠送余额。
- `credit_type=gift` 时必须显式传入大于 0 的 `gift_validity_days`，并用它计算 grant 过期时间。
- 为避免修改主仓优惠码表结构，优惠码额度类型元数据暂存于 `notes` 的 HTML comment 中；前端展示时会剥离这段元数据。

### 红包雨活动

- 红包雨领取奖励进入赠送余额，不再直接增加普通余额。
- 活动配置新增 `gift_validity_days`，用于控制每次领取生成 grant 的过期时间。
- 历史普通余额不迁移；新领取记录按赠送余额规则生效。

## 性能约束

- 请求准入优先读可用余额缓存或 `custom_gift_credit_user_balances`。
- 不在 AI 流式 chunk 中查询赠送余额。
- grant 明细只在发放、扣减、管理员明细和过期刷新时访问。
- API Key 鉴权快照会读取 O(1) 聚合赠送余额，避免普通余额为 0 但赠送余额可用时被提前拦截。
- AI 扣费只在最终 usage billing 阶段先扣赠送余额；赠送余额不足时，剩余金额继续进入原普通余额扣费流程。

## 配置

- `CUSTOM_GIFT_CREDIT_TABLE_PREFIX`：可选，给赠送余额相关表增加统一前缀；活动领取写入赠送余额时也使用该前缀。
- `CUSTOM_GIFT_CREDIT_MIGRATION_TIMEOUT`：可选，控制赠送余额启动迁移超时时间，默认 `30s`。

## 回滚

- 回滚业务代码后，普通余额仍按旧逻辑工作。
- custom giftcredit 表可保留，不影响 `users.balance`。
- 若需要把未用赠送余额转回普通余额，必须单独执行明确 SQL 或管理脚本，不做隐式迁移。
