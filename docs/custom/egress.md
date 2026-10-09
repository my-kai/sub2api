# 账号多出口配置

## 配置字段

账号的 `extra` JSON 保存以下字段：

- `egress_proxy_ids`：按稳定顺序保存已选代理 ID。
- `egress_include_local`：是否包含本地直连，默认 `true`。

本地出口的展示和审计值固定为 `local`。代理出口使用代理记录的 `host` 展示，槽位身份使用 `proxy:<id>`，避免同 host 的不同代理配置互相覆盖。历史账号未配置新字段时继续读取兼容的 `proxy_id`。

## 并发与容量

请求在调度阶段先从账号出口集合轮询选择一个出口，再按“账号 + 出口身份”占用 Redis 槽位。每个出口的上限都等于账号并发上限，不做平均分配；账号并发为 5、配置 3 个出口时总容量为 15，每行仍显示该出口的 `当前占用/5`。

旧账号级槽位不迁移、不猜测出口归属，继续自然释放；新请求使用出口级槽位。出口满载只阻塞该出口，其他出口仍可接收请求。

## 请求与使用记录

网关、OpenAI 网关、图片、嵌入、Alpha Search、OAuth/配额刷新和账号测试等出站路径复用请求级出口绑定。使用记录写入本次实际使用的代理 `host` 或 `local`；历史记录的 `NULL` 保持为空，前端显示 `-`。

## 失败语义

- 账号配置写入仍校验代理存在、状态和有效期，不接受空代理列表且关闭直连的配置。
- 运行时读取按出口隔离不存在、已删除、停用或过期的代理；其他有效代理以及已明确配置的 `local` 仍可使用，不自动补本地。
- 无有效出口时，返回的账号/调度快照标记为不可调度；保留原代理 ID、分组与平台，不写回账号状态，避免单个失效出口阻塞整个调度桶和 outbox 消费。
- 无可用出口或出口槽位等待超时时显式返回并发/调度错误。
- 代理连接失败遵循现有请求失败和 failover 语义；一次请求的出口绑定不会因重试在日志中漂移。

## 接入位置

- 后端统一出口解析：`backend/internal/custom/egress/`。
- 账号配置接口：管理员账号创建、编辑和批量编辑接口的 `egress_proxy_ids`、`egress_include_local`。
- 容量响应：账号 DTO 的 `egress_capacities`，每行包含 `host`、`capacity` 和 `current_concurrency`。
- 使用记录：`usage_logs.egress_host` 及管理员/普通用户 Usage 页面和导出。

## 调度快照重建失败修复

### 接入与兼容性

- `backend/internal/custom/egress/runtime.go`：统一代理可用性和出口集合解析，不依赖 service 包。
- 主仓薄接入为 `backend/internal/repository/account_repo.go` 的 `loadProxies`、统一出口转换适配器及 `GetByIDs` 批量加载路径；单账号读取、分组/平台查询和批量账号事件使用同一策略。
- 历史 `proxy_id` 缺失或失效时仍保留绑定，不解释为本地直连；`proxy_fallback_origin_id` 只是展示来源，不参与当前出口可用性判断。
- 管理端读取继续保留失效出口 ID、已有代理名称的“（失效）”标记和数据库中的 `schedulable` 设置，以便修复。恢复有效出口后，下一次快照重建重新计算可调度性。
- 数据库读取错误和格式错误仍向上传播，不将存储故障掩盖为成功的部分快照。
- 不新增 API、配置、数据表、迁移或依赖；不修改调度算法、网关、登录或计费文件。

### 验证

使用项目 Go 1.27 工具链，可在 `backend/` 执行：

```bash
# 常规测试
GOTOOLCHAIN=go1.27.0 go test ./internal/custom/egress ./internal/repository -count=1

# 出口与调度快照针对性回归
GOTOOLCHAIN=go1.27.0 go test -tags=unit ./internal/custom/egress ./internal/repository \
  -run 'Test(ProxyUsable|ResolvePool|AccountsToService|AccountRuntimeReads|GetByIDs_LegacyEgress|Scheduler|ListSchedulable)' -count=1

# 新增出口回归的竞态检查
GOTOOLCHAIN=go1.27.0 go test -race -tags=unit ./internal/custom/egress ./internal/repository \
  -run 'Test(ProxyUsable|ResolvePool|AccountsToService_.*Egress|AccountRuntimeReads|GetByIDs_LegacyEgress|SchedulerSnapshot_Egress)' -count=1

# 相关生产包编译和静态检查
GOTOOLCHAIN=go1.27.0 go build ./internal/custom/egress ./internal/repository ./internal/service
GOTOOLCHAIN=go1.27.0 go vet ./internal/custom/egress ./internal/repository ./internal/service
```

以上验证已通过，竞态检查首次超时后重跑通过。

覆盖停用、过期、删除、缺失代理、混合有效出口、显式直连、历史绑定、管理端展示、数据库错误传播、代理恢复，以及 Redis 单平台/forced 快照和完整账号缓存的不可调度保护。

完整 unit 套件未通过，存在以下已用 HEAD 原代码复现的基线问题，未顺带修改这些文件：

- 服务层：`gateway_record_usage_test.go` 引用已移除的 `RecordUsageWithLongContext` / `RecordUsageLongContextInput`；`openai_alpha_search_billing_test.go` 未适配 `snapshotFromAPIKey` 的两个返回值。
- 仓储层：`TestUsageLogStaticInsertShape_PlaceholdersMatchArgTypes` 发现 SQL 占位符 62 个、参数类型 63 个；`TestPrepareUsageLogInsert_RequestedReasoningEffortArgWiring` 断言失败。

服务层另外使用临时 Go overlay 排除上述长上下文测试函数及 Alpha Search 计费测试文件，保留其他测试辅助函数，执行现有 `Scheduler` / `Outbox` / 出口相关回归通过。overlay 只影响验证环境，不修改仓库测试文件；该结果不代表完整套件通过。

未执行真实 PostgreSQL/Redis 集成测试、完整后端制品构建或生产发布；本次缓存回归使用 miniredis，仓储查询使用 sqlmock。

### 发布与回退

1. 使用原部署方式构建、发布后端并重启服务；无需 SQL 或环境变量调整。
2. 重启会触发已有初始快照重建；观察原报错是否停止，以及已有 outbox 消费进度是否继续推进。修复本身不启用代理 10，也不改变它的账号绑定。
3. 若所有出口均不可用，需在管理端恢复代理或调整出口配置；只有原本启用本地出口的账号允许直连。
4. 后续合并上游时，主要冲突点为 `account_repo.go` 的批量账号读取和出口转换区域；保留 custom helper 的接入即可，无调度热点文件改动。
5. 回退使用上一版后端制品；没有数据迁移。旧版本会恢复“失效出口可能阻塞重建”的行为，回退前需先修复失效绑定。

