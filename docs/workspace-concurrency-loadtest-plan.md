# Workspace Concurrency Load Test Plan

本文固化 workorch 的 workspace 并发压测方案。压测目标不是验证普通 HTTP
QPS，而是验证完整业务闭环在并发 workspace 下的吞吐、排队、稳定性和证据完整性。

## 1. 压测目标

压测必须覆盖以下正式业务流程：

- 创建 Workspace。
- 创建 Goal 和 DoD。
- 启动 Run。
- 处理多轮 Decision Ticket。
- 处理 Approval。
- 执行 MCP / ToolCall。
- 形成 DoD assessment。
- 生成 Result 和 Artifact。
- 回查 Run snapshot、Workspace snapshot、Run status、Run observability。

核心问题：

- workorch 在指定 `server.maxConcurrentRuns` 下可稳定支撑多少并发 workspace。
- 队列等待、Run 执行、Ticket/Approval 人工治理、ToolCall、Result 生成分别消耗多少时间。
- file backend 在并发写入下是否保持数据一致性。
- 运行过程与证据是否完整，尤其是 `step` / `step_stream` timeline 的 `runState`。

## 2. 压测分层

压测分为两条轨道，结果必须分开解释。

### 2.1 Deterministic Mock LLM 容量压测

用途：

- 测 workorch 自身容量。
- 避免真实 LLM provider 限流、网络抖动、费用和模型不稳定污染结论。
- 作为容量曲线和回归基线。

覆盖能力：

- 调度队列。
- file backend。
- 多轮 Decision Ticket。
- Approval。
- MCP stdio ToolCall。
- RuntimeCheckpoint、ValidationResult、Result、Artifact。
- Snapshot / observability 读模型。

### 2.2 Real LLM 真实端到端压测

用途：

- 验证真实 LLM provider 与 workorch 的端到端稳定性。
- 验证提示词约束在并发和多轮治理下仍能稳定产出协议化响应。
- 观察真实 provider 延迟、协议错误、限流和网络错误。

真实 LLM 结果不能单独作为 workorch 内部容量结论。报告必须区分：

- workorch 问题：队列卡死、resume 不继续、状态不一致、证据缺失、Result/Artifact 缺失、file backend 写入异常。
- LLM 问题：JSON 不合法、跳过 Ticket/Approval、未按协议返回 executionGraph 或 dodAssessment。
- 外部系统问题：provider 429/5xx、网络超时、MCP 外部依赖失败。

## 3. 标准业务场景

每个压测 workspace 使用同一个治理型业务流程。

1. 创建 Workspace，标题包含压测批次和序号。
2. 创建 Goal，要求严格按以下阶段执行。
3. 第一轮 LLM 返回 `blocked_on_decision`，生成 Ticket 1，询问目标城市。
4. 压测控制器 resolve Ticket 1，补充 `country`、`province`、`city`、`lookupName`。
5. Resume Run。
6. 第二轮 LLM 返回 `blocked_on_decision`，生成 Ticket 2，询问输出格式、风险确认或查询范围。
7. 压测控制器 resolve Ticket 2，补充输出格式和确认信息。
8. Resume Run。
9. 第三轮 LLM 返回 `blocked_on_approval`，生成 Approval。
10. 压测控制器 approve Approval。
11. Resume Run。
12. LLM 返回 executionGraph，包含一个 MCP stdio ToolCall。
13. MCP 工具返回确定性业务结果。
14. LLM 基于 previousStepOutput 返回 `completed` 和 `dodAssessment`。
15. workorch 生成 accepted Result 和 linked Artifact。
16. 压测控制器回查并断言所有数据。

DoD 必须要求最终结果包含：

- 两轮 Decision Ticket 处理记录。
- Approval 处理记录。
- ToolCall 证据。
- ValidationResult。
- RuntimeCheckpoint。
- DoD assessment。
- Result 和 Artifact。

## 4. Mock LLM 场景

Mock LLM 必须是确定性的，根据 `stepId`、历史消息或上下文阶段返回固定协议响应。

阶段响应：

- `step-1`: 返回 `completionState=blocked_on_decision` 和 `requiredDecision`。
- `llm-after-decision`: 如果只解决 Ticket 1，返回第二个 `requiredDecision`。
- `llm-after-decision-2`: 返回 `completionState=blocked_on_approval` 和 `requiredApproval`。
- `llm-after-approval`: 返回 executionGraph，包含 MCP stdio tool call。
- `zz-llm-after-graph-*`: 返回 `completionState=completed`、`dodAssessment.satisfied=true`、`evidenceRefs`。

Mock MCP 工具必须本地执行，避免外部网络依赖。工具输出应包含：

- `query`。
- `current` 或业务结果主体。
- `source`。
- `observedAt`。
- 可验证的结构化字段。

## 5. Real LLM 场景

真实 LLM 场景使用部署实例已配置的默认 provider，当前本地验收默认使用 `local`。

真实 LLM prompt 必须明确：

- 第一轮不得调用工具，必须生成 Ticket 1。
- Ticket 1 解决后不得调用工具，必须生成 Ticket 2。
- Ticket 2 解决后不得调用工具，必须生成 Approval。
- Approval 通过后必须调用指定 MCP server 的指定 tool。
- 工具返回后必须返回 `completed` 和 `dodAssessment`。
- 不允许使用 shell fallback。
- `dodAssessment.summary` 必须暴露最终业务结果。

真实 LLM 压测控制器必须统计协议错误：

- 未生成 Ticket。
- Ticket 数量不足。
- 未生成 Approval。
- Approval 前调用工具。
- 未调用指定 MCP tool。
- 未生成 DoD assessment。
- Result / Artifact 缺失。

## 6. 压测矩阵

### 6.1 当前部署基线

当前配置常见为：

```text
repository.backend = file
server.maxConcurrentRuns = 1
```

在 `maxConcurrentRuns=1` 下，压测重点是队列稳定性和单执行槽吞吐。

| 轮次 | LLM | client 并发 workspace | 总 workspace | 目标 |
| --- | --- | ---: | ---: | --- |
| mock-baseline | mock | 1 | 10 | 校验压测控制器正确性 |
| mock-queue-1 | mock | 5 | 50 | 验证单执行槽排队稳定性 |
| mock-queue-2 | mock | 10 | 100 | 验证 file backend 持续写入与队列等待 |
| real-smoke | real | 1 | 3 | 验证真实 LLM 多轮治理稳定 |
| real-queue | real | 2 | 10 | 验证真实 LLM 下队列与多 workspace 串行处理 |
| real-pressure | real | 5 | 20 | 观察 provider 延迟、协议错误和队列等待 |

### 6.2 并发容量曲线

需要调整 `server.maxConcurrentRuns` 并重新部署后执行。

| 轮次 | maxConcurrentRuns | LLM | client 并发 workspace | 总 workspace |
| --- | ---: | --- | ---: | ---: |
| capacity-2-mock | 2 | mock | 5 / 10 / 20 | 100 |
| capacity-4-mock | 4 | mock | 10 / 20 / 40 | 200 |
| capacity-8-mock | 8 | mock | 20 / 50 / 100 | 500 |
| capacity-2-real | 2 | real | 4 | 20 |
| capacity-4-real | 4 | real | 8 | 40 |

真实 LLM 容量压测应控制规模，避免费用和 provider 限流影响判断。

容量曲线压测必须设置 `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS`，让控制器在执行前校验当前部署实例的
`server.maxConcurrentRuns`。如果部署实例仍为单执行槽，压测应 fail-fast，避免把排队压测误判为真实并行容量。

## 7. 指标

压测报告必须输出：

- `workspace_total`。
- `workspace_success_total`。
- `workspace_failed_total`。
- `success_rate`。
- `server_max_concurrent_runs`。
- `throughput_workspaces_per_min`。
- `workspace_e2e_latency_p50/p95/p99`。
- `queue_wait_latency_p50/p95/p99`。
- `decision_round1_latency_p50/p95/p99`。
- `decision_round2_latency_p50/p95/p99`。
- `approval_latency_p50/p95/p99`。
- `tool_execution_latency_p50/p95/p99`。
- `result_materialization_latency_p50/p95/p99`。
- `snapshot_read_latency_p50/p95/p99`。
- `observability_read_latency_p50/p95/p99`。
- `http_4xx_total`。
- `http_5xx_total`。
- `workorch_run_queue_waiting`。
- `workorch_run_queue_total`。
- `workorch_run_requests_in_flight`。
- `metrics_before` / `metrics_after`。

真实 LLM 额外输出：

- `llm_call_count_per_workspace`。
- `llm_latency_p50/p95/p99`。
- `llm_protocol_error_total`。
- `provider_http_error_total`。
- `provider_rate_limit_total`。
- `provider_timeout_total`。
- `ticket_round_count`。
- `approval_round_count`。
- `token_usage_total`，如果 provider 返回 usage。
- `cost_estimate`，如果 token usage 和单价可得。

## 8. 成功标准

Mock 容量压测通过标准：

- 成功率 `>= 99%`。
- 所有成功 workspace 最终为 `completed`。
- 每个成功 workspace 至少有 2 个 resolved Decision Ticket。
- 每个成功 workspace 至少有 1 个 approved Approval。
- 每个成功 workspace 至少有 1 个 completed ToolCall。
- 每个成功 workspace 至少有 1 个 passed ValidationResult。
- 每个成功 workspace 有 RuntimeCheckpoint。
- 每个成功 workspace 有 accepted Result 和 linked Artifact。
- `step` / `step_stream` timeline 缺失 `runState` 数量为 `0`。
- file backend 落盘数据与 API 读模型一致。
- 失败 workspace 必须有可定位的 failureReason 或错误证据。

真实 LLM 压测通过标准：

- `real-smoke`: 成功率 `100%`。
- `real-queue`: 成功率 `>= 90%`，失败必须可归因。
- `real-pressure`: 不强制作为发布门禁，主要用于观察真实 provider 端到端上限。
- 所有成功 workspace 必须满足与 mock 相同的数据完整性断言。
- 真实 LLM 协议错误必须单独列出，不与 workorch 内部故障混算。

## 9. 实施计划

### LTP1. 压测控制器

已新增：

```text
scripts/loadtest-governed-workspace-concurrency.py
scripts/loadtest-governed-workspace-concurrency.sh
```

能力：

- 参数化 base URL、总量、client 并发、LLM 模式、artifact 目录。
- 启动本地 mock LLM 和 mock MCP。
- 并发创建 workspace 并驱动完整治理流程。
- 每个 workspace 独立记录 artifacts。
- 输出 JSON 和 Markdown 报告。
- 报告当前部署实例的 `serverMaxConcurrentRuns`、压测前后 metrics。
- 可通过 `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS` 在容量曲线压测前校验服务端执行槽，避免把单执行槽排队压测误判为真实并行压测。

当前入口：

```bash
make loadtest-governed
make loadtest-governed-real
```

或直接使用脚本参数：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=2 \
WORKORCH_LOAD_TOTAL=3 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

真实 LLM smoke：

```bash
WORKORCH_LOAD_LLM_MODE=real \
WORKORCH_LOAD_CONCURRENCY=1 \
WORKORCH_LOAD_TOTAL=1 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

### LTP2. Mock 容量基线

先实现并执行：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=1 \
WORKORCH_LOAD_TOTAL=10 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

验收控制器正确性。

### LTP3. Mock 队列压力

在当前 `maxConcurrentRuns=1` 下执行：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=5 \
WORKORCH_LOAD_TOTAL=50 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

再提升到 `concurrency=10,total=100`。

### LTP4. Real LLM 真实链路基线

执行：

```bash
WORKORCH_LOAD_LLM_MODE=real \
WORKORCH_LOAD_CONCURRENCY=1 \
WORKORCH_LOAD_TOTAL=3 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

然后执行：

```bash
WORKORCH_LOAD_LLM_MODE=real \
WORKORCH_LOAD_CONCURRENCY=2 \
WORKORCH_LOAD_TOTAL=10 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

### LTP5. 并发容量曲线

调整 `server.maxConcurrentRuns` 后重新部署，再按矩阵执行 mock 和 real 压测。

容量压测建议显式设置期望执行槽：

```bash
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=2 \
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=100 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

## 10. 当前基线结果

当前部署实例：

```text
repository.backend = file
server.maxConcurrentRuns = 1
```

已完成基线：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | 协议错误 | timeline runState 缺失 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `/tmp/workorch-loadtest-real-c2-t3-20260428183022` | real | 2 | 3 | 3 | 0 | 1.30 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c10-t30-20260428183558` | mock | 10 | 30 | 30 | 0 | 4.69 | 0 | 0 |
| `/tmp/workorch-loadtest-real-c2-t5-20260428184232` | real | 2 | 5 | 5 | 0 | 0.99 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c20-t100-mcr10-20260428185911` | mock | 20 | 100 | 100 | 0 | 8.84 | 0 | 0 |
| `/tmp/workorch-loadtest-real-c4-t20-mcr10-20260428191046` | real | 4 | 20 | 18 | 2 | 1.55 | 0 | 0 |
| `/tmp/workorch-loadtest-real-c4-t20-mcr10-retry-20260428192538` | real | 4 | 20 | 16 | 4 | 1.64 | 0 | 0 |
| `/tmp/workorch-loadtest-real-c2-t10-mcr10-20260428193626` | real | 2 | 10 | 8 | 2 | 1.45 | 2 | 0 |

结论：

- 当前单执行槽部署可稳定通过 mock 队列压力与真实 LLM 小并发端到端验收。
- 这些结果验证的是排队稳定性和完整业务闭环，不代表多 Run 真正并行容量。
- `server.maxConcurrentRuns=10` 部署下，mock `concurrency=20,total=100` 已验证 workorch 自身并发调度、file backend 和证据链完整性。
- 真实 LLM 在 `concurrency=2/4` 下仍存在 workflow protocol violation，典型表现是跳过第二轮 Decision 直接进入 Approval，或第二轮后继续生成额外 Decision；这类失败应归入 LLM 协议稳定性问题，不作为 workorch 内部并发容量失败。

## 11. 风险与边界

- 压测脚本必须只访问当前部署实例，不自行启动或停止 `workorchd`。
- CI 不执行真实 LLM 压测。
- 真实 LLM 压测必须有总量上限，避免不可控费用。
- Mock LLM 不能作为真实 provider 稳定性的证明。
- 真实 LLM 压测不能作为 workorch 内部容量的唯一依据。
- 压测不能在通用 runtime 中加入具体业务 fallback。
- 压测数据应通过批次前缀标识，便于清理与复盘。
