# WorkOrch Loadtest Baseline

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件作为 WorkOrch 并发压测与容量基线入口文档。

## Current Baseline

- 快速回归基线：mock LLM，`concurrency=10,total=50`
- 高压验收基线：mock LLM，`concurrency=20,total=100`
- 真实 LLM 抽检基线：`concurrency=1-2` 的小批量 smoke/queue 验证
- `concurrency>=30` 不再作为常规验收项

## Verification Entry

当前 active loadtest 入口已统一到单一控制器：

```bash
make loadtest-governed
make loadtest-governed-real
```

对应脚本：

- `make loadtest-governed`
  - `scripts/loadtest-governed-workspace-concurrency.sh`
  - 再转发到 `scripts/loadtest-governed-workspace-concurrency.py`
- `make loadtest-governed-real`
  - `WORKORCH_LOAD_LLM_MODE=real ./scripts/loadtest-governed-workspace-concurrency.sh`

结论：

- 当前没有并列的第二套 active loadtest harness
- mock/real 两条轨道只通过环境变量和 provider 选择分流
- 结果口径应以 `summary.json` / `report.md` 的实际输出字段为准

## Script Responsibility

`loadtest-governed-workspace-concurrency.py` 当前同时负责：

- 检查 `readyz`
- 读取 `configuration`
- 采集 `metricsBefore` / `metricsAfter`
- 按批次创建 workspace / goal / DoD / run
- 驱动两轮 Decision、一轮 Approval、一次 MCP tool call
- 回读 snapshot / workspace snapshot / observability / artifact content
- 汇总 success rate、throughput、latency、failure class、timeline integrity
- 输出 `summary.json` 与 `report.md`

这意味着本文件只需要说明：

- 哪些入口可跑
- 该脚本产出哪些稳定指标
- 哪些 records 属于历史轮次/复盘，不再是 active baseline 定义

## Track Split

- deterministic mock LLM capacity baseline
- real LLM end-to-end pressure baseline
- queue / throughput / latency
- evidence integrity and read-model correctness

## Stable Scenario

压测的标准治理闭环应固定覆盖：

- 创建 Workspace
- 创建 Goal / DoD
- 两轮 Decision Ticket
- 一轮 Approval
- 一次 MCP/ToolCall
- 一次 `dodAssessment`
- accepted Result 与 linked Artifact
- snapshot / observability 回查

补充：

- 当前 deterministic harness 会稳定经历：
  - 2 个 Decision Ticket
  - 1 个 Approval
  - 1 个 MCP stdio tool execution
  - accepted Result / linked Artifact
  - snapshot / workspace snapshot / observability / artifact content readback
- 因此该基线验证的是“完整治理型业务闭环并发”，不是普通 HTTP QPS

## Entry Matrix

| 入口 | LLM 模式 | 当前用途 | 主要环境变量 |
| --- | --- | --- | --- |
| `make loadtest-governed` | mock | 日常 deterministic 回归、容量基线、容量曲线抽样 | `WORKORCH_LOAD_TOTAL` / `WORKORCH_LOAD_CONCURRENCY` / `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS` |
| `make loadtest-governed-real` | real | 真实 provider smoke/queue 抽检 | `WORKORCH_LOAD_PROVIDER_ID` / `WORKORCH_LOAD_TOTAL` / `WORKORCH_LOAD_CONCURRENCY` |

当前入口约束：

- `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS`
  - 用于 fail-fast 校验部署实例的 `server.maxConcurrentRuns`
- `WORKORCH_LOAD_LLM_MODE`
  - `mock` 或 `real`
- `WORKORCH_LOAD_PROVIDER_ID`
  - real 模式下可显式指定 provider
- `WORKORCH_LOAD_ARTIFACT_DIR`
  - 覆盖批次产物目录
- `WORKORCH_LOAD_KEEP_ARTIFACTS`
  - 保留产物目录用于复盘

## Metric Contract

当前脚本稳定写出两层指标：

### Summary-Level Metrics

`summary.json` 当前稳定包含：

- `batchId`
- `llmMode`
- `mcpSessionPolicy`
- `total`
- `concurrency`
- `serverMaxConcurrentRuns`
- `workspaceSuccessTotal`
- `workspaceFailedTotal`
- `successRate`
- `minSuccessRate`
- `pass`
- `durationSeconds`
- `throughputWorkspacesPerMin`
- `http4xxTotal`
- `http5xxTotal`
- `curlErrorTotal`
- `llmProtocolErrorTotal`
- `timelineMissingRunStateTotal`
- `metricsBefore`
- `metricsAfter`

### Latency / Diagnostic Metrics

脚本还稳定输出：

- `e2eLatency`
  - `count` / `avg_ms` / `p50_ms` / `p95_ms` / `p99_ms` / `max_ms`
- `stageLatency`
  - 各阶段原始 latency map
- `businessLatency`
  - `queueWaitProxy`
  - `decisionRound1`
  - `decisionRound2`
  - `approval`
  - `toolExecutionAndResultMaterialization`
  - `snapshotRead`
  - `observabilityRead`
- `llmCallCountPerWorkspace`
- `failureClassTotal`
- `slowestWorkspaces`
- `stageSamples`
- `results`

### Metrics From `/metrics`

当前脚本显式采集的 Prometheus 指标口径只有：

- `workorch_run_queue_waiting`
- `workorch_run_queue_total`
- `workorch_run_requests_in_flight`
- `workorch_server_max_concurrent_runs`

解释边界：

- 这些指标用于解释队列压力和服务并发配置
- 它们不是 success/failure 的唯一判断依据
- 压测结论仍以 workspace 级闭环成功率和证据完整性为主

## Failure Classification Contract

当前脚本会把失败归入稳定 failure class：

- `http_4xx`
- `http_5xx`
- `http_timeout`
- `http_empty_reply`
- `curl_error`
- `llm_protocol_error`
- `provider_error`
- `evidence_integrity_error`
- `business_state_error`

解释规则：

- provider / 网络 /协议错误必须和 WorkOrch 内部状态错误分开统计
- accepted Result / Artifact 缺失、evidence 缺失、timeline 缺失等进入
  `evidence_integrity_error`
- 无法归因到 provider 或 HTTP 的业务闭环异常进入 `business_state_error`

## Acceptance Gates

mock 基线通过标准：

- 成功率 `>=99%`
- 所有成功 workspace 最终为 `completed`
- 每个成功 workspace 至少包含 2 个 resolved Decision Ticket、1 个 approved Approval、1 个 completed ToolCall、1 个 passed ValidationResult
- timeline 中 `runState` 缺失数必须为 `0`
- accepted Result 与 linked Artifact 必须存在

real LLM 基线通过标准：

- `real-smoke` 成功率 `100%`
- `real-queue` 成功率 `>=90%`，且失败必须可归因
- provider 协议错误、限流和网络错误必须与 WorkOrch 内部故障分开统计

## Current Recommendation

- 当前 file backend 的常规验收上限维持在 `c20/t100`
- `c30+` 仅保留为专项性能实验，不再作为日常验收阻塞项
- 若持久化后端、事件流或 summary read model 发生实质变化，应重新开启容量曲线压测

## Records Layering

当前 verification / archive 分层应固定为：

- `loadtest-baseline.md`
  - 只承接当前 active baseline、入口、指标口径、门禁标准
- `../90-archive/records/workspace-concurrency-loadtest-record.md`
  - 承接历史并发轮次、容量曲线结果、阶段性结论与复盘
- `../90-archive/records/single-workspace-performance-optimization-record.md`
  - 承接 `concurrency=1` 单工作空间延迟优化记录，不等于并发容量基线

这意味着：

- baseline 文档不重复粘贴每一轮历史结果
- records 文档不重新定义当前门禁阈值
- 单工作空间性能优化记录只作为局部 latency 复盘，不替代并发 capacity baseline

## Script And Record Mapping

| 入口/文档 | 当前职责 |
| --- | --- |
| `scripts/loadtest-governed-workspace-concurrency.py` | 产出 active loadtest summary / report / raw artifacts |
| `make loadtest-governed` | mock baseline 入口 |
| `make loadtest-governed-real` | real-provider smoke/queue 入口 |
| `loadtest-baseline.md` | 当前基线门禁与指标口径 |
| `../90-archive/records/workspace-concurrency-loadtest-record.md` | 历史轮次、并发实验与复盘 |
| `../90-archive/records/single-workspace-performance-optimization-record.md` | 单工作空间 latency 优化复盘 |

## Historical Note

旧状态文档和旧压测方案中的阶段性结论已迁入本文件；
完整场景定义、压测矩阵、轮次结果与复盘记录已迁入 archive records。

补充：

- 更细场景设计源文档仍可追溯到 `../workspace-concurrency-loadtest-plan.md`
- 但 active 验收与当前阈值应以本文件为准，而不是继续回到旧规划文档解释

## Source Documents

- `../90-archive/records/workspace-concurrency-loadtest-record.md`
