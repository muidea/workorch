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

真实 LLM 场景默认使用部署实例已配置的默认 provider；如果默认 provider 发生网络 EOF、限流或不可用，应显式指定
`WORKORCH_LOAD_PROVIDER_ID` 重跑最小样本，用于区分 provider 可用性问题和 workorch 框架问题。当前本地验收建议使用
`local` provider 作为真实 LLM smoke 基线。

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
- `min_success_rate`。
- `pass`。
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
- `curl_error_total`。
- `failure_class_total`。
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
- 可通过 `WORKORCH_LOAD_MCP_SESSION_POLICY=long_lived` 显式验证长会话；默认使用 `transient`，避免反复压测后遗留多批次 stdio helper 进程污染容量结论。
- 可通过 `WORKORCH_LOAD_MIN_SUCCESS_RATE` 设置回归门禁成功率，默认 `1.0`，报告中输出 `pass`。
- 对每个成功 workspace 断言正式 Result `summaryDetail`、Artifact 分类、`workorch://` URI、Artifact 内容读取、Result/Artifact 关联和 timeline `runState`。
- 对失败 workspace 归类输出 `http_4xx`、`http_5xx`、`http_timeout`、`http_empty_reply`、`llm_protocol_error`、`provider_error`、`evidence_integrity_error` 等失败类型。

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
| `/tmp/workorch-loadtest-mock-c100-t300-mcr100-20260428200956` | mock | 100 | 300 | 13 | 287 | 1.61 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c50-t100-mcr100-20260428201837` | mock | 50 | 100 | 16 | 84 | 1.13 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c20-t50-mcr100-20260428203318` | mock | 20 | 50 | 28 | 22 | 1.29 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c10-t20-mcr100-20260428212246-pqagfzbz` | mock | 10 | 20 | 12 | 8 | 0.82 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-c5-t10-mcr100-20260428213804-e37mir89` | mock | 5 | 10 | 10 | 0 | 1.64 | 0 | 0 |
| `/tmp/workorch-loadtest-real-c1-t1-mcr100-20260428214440-v8ttwewc` | real `miniMax` | 1 | 1 | 0 | 1 | 0.00 | 0 | 0 |
| `/tmp/workorch-loadtest-real-local-c1-t1-mcr100-20260428214523-zqmbb9_7` | real `local` | 1 | 1 | 1 | 0 | 0.63 | 0 | 0 |
| `/tmp/workorch-loadtest-mock-transient-smoke-20260428214855-t28cia0u` | mock transient MCP | 1 | 1 | 1 | 0 | 1.14 | 0 | 0 |

结论：

- 当前单执行槽部署可稳定通过 mock 队列压力与真实 LLM 小并发端到端验收。
- 这些结果验证的是排队稳定性和完整业务闭环，不代表多 Run 真正并行容量。
- `server.maxConcurrentRuns=10` 部署下，mock `concurrency=20,total=100` 已验证 workorch 自身并发调度、file backend 和证据链完整性。
- 真实 LLM 在 `concurrency=2/4` 下仍存在 workflow protocol violation，典型表现是跳过第二轮 Decision 直接进入 Approval，或第二轮后继续生成额外 Decision；这类失败应归入 LLM 协议稳定性问题，不作为 workorch 内部并发容量失败。
- `server.maxConcurrentRuns=100` 部署下，mock 压测在 `concurrency=100/50/20` 均未通过；失败主要表现为 workspace create HTTP 500、empty reply，以及 activation/resume 阶段 empty reply。当前服务指标显示 run queue 没有排队，说明瓶颈已从 Run 执行槽转移到 HTTP 写入路径、file backend 并发落盘和读模型回查。
- 在 `.workorch-data` 已积累约 103 万行 JSONL 数据时，`event_record.json`、`tool_call.json`、`step.json`、`runtime_checkpoint.json` 成为主要数据文件；后续提升并发应优先治理持久化与读模型性能，而不是继续单纯提高 `server.maxConcurrentRuns`。
- 在 `.workorch-data` 继续增长后，mock `concurrency=10,total=20` 只达到 60% 成功率；8 个失败全部发生在最终 `POST /runs/{runId}/resume`，curl code 为 52 empty reply，耗时 184s 到 225s。失败样本回查 Run snapshot 均为 `completed`，并且包含 2 个 Decision、1 个 Approval、ToolCall、ValidationResult、RuntimeCheckpoint、accepted Result 和 linked Artifact。因此该类失败应归因为同步 HTTP 长请求超过 `server.writeTimeout=180s` 后连接被关闭，而不是业务状态机失败。
- 同一数据规模下，mock `concurrency=5,total=10` 全部通过；当前 file backend 与同步 resume 模式下，可作为临时可靠对照阈值，但不应视为最终容量目标。该批次 `toolExecutionAndResultMaterialization` 平均 60.23s、p95 69.86s，workspace e2e p95 194.45s。
- 真实 LLM 最小样本使用默认 `miniMax` provider 失败，failureReason 为 `Post "https://aiapi.bluetron.cn/chat/completions": EOF`，未进入 Ticket/Approval 流程；同一部署显式指定 `WORKORCH_LOAD_PROVIDER_ID=local` 后成功完成完整真实 LLM 链路。因此默认 provider EOF 归入 provider 可用性问题，不作为 workorch 框架失败。
- 脚本默认改为 `WORKORCH_LOAD_MCP_SESSION_POLICY=transient` 后，mock transient smoke 仍可完成完整业务闭环，并且不会新增长期驻留的 loadtest MCP helper 进程。早前 `long_lived` 批次遗留进程仍需通过重启服务或后续 MCP 管理能力清理。

## 11. 风险与边界

- 压测脚本必须只访问当前部署实例，不自行启动或停止 `workorchd`。
- CI 不执行真实 LLM 压测。
- 真实 LLM 压测必须有总量上限，避免不可控费用。
- Mock LLM 不能作为真实 provider 稳定性的证明。
- 真实 LLM 压测不能作为 workorch 内部容量的唯一依据。
- 压测不能在通用 runtime 中加入具体业务 fallback。
- 压测数据应通过批次前缀标识，便于清理与复盘。
- 常规容量压测默认使用 transient MCP session；长会话压测必须作为单独场景记录，并在压测后通过重启或后续管理能力清理遗留 stdio helper 进程。

## 12. 稳定性诊断规则

当前阶段应通过测试把问题定位到明确层级，而不是只看最终成功率。

诊断分层：

- HTTP 入口层：记录 method、path、curl code、HTTP status、response body、stderr、elapsedMs。
- repository / file backend 层：用 Go 并发测试隔离仓储写入行为，避免把 HTTP 或 Run 调度混入判断。
- Run 调度层：观察 `workorch_run_requests_in_flight`、`workorch_run_queue_waiting`、`workorch_run_queue_total`。
- 业务状态机层：校验 Run snapshot、Workspace snapshot、status、observability、Ticket、Approval、ToolCall、ValidationResult、RuntimeCheckpoint、Result、Artifact。
- LLM 协议层：统计 workflow protocol violation、schema parse / repair error、provider HTTP error。

已补充：

- 压测控制器失败时输出 `http-error.json`，保留 HTTP status、body preview、curl stderr 和请求耗时。
- `TestWorkspaceRepositorySharedInstanceConcurrentCreate` 验证共享 file repository 实例的并发写入安全性。
- `TestWorkspaceRepositoryIndependentInstancesConcurrentCreateStress` 默认跳过，设置 `WORKORCH_STRESS_TEST=1` 后用于验证多个 file repository 实例指向同一文件时的并发写入风险。

已复现的关键规则：

- 共享 file repository 实例可以通过实例内 mutex 保护同一文件写入。
- 多个 file repository 实例并发写同一文件会破坏 JSON 持久化文件，例如出现 `invalid character '"' after top-level value`。
- 因此 file backend 的稳定性要求是：运行中必须复用同一组 repository 实例；如果未来存在跨进程、多服务实例、或未走 shared runtime 的 fallback 路径，必须引入跨实例文件锁或升级持久化后端。

## 13. 2026-05-01 压测复盘与优化

当前部署实例：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮先修正压测控制器与运行期语义不一致的问题：Decision resolve 与 Approval approve 已经触发后台续跑，压测脚本不再显式调用 `/runs/{runId}/resume`，而是按 Run status 轮询阻塞对象切换和终态完成。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e p95 | snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501012005-as45pjvy` | mock | 2 | 5 | 5 | 0 | 18.49 | 5.74s | 0.22s | 0.22s | 小规模完整业务闭环通过 |
| `/tmp/workorch-loadtest-load-20260501012027-7avmcd7f` | mock | 5 | 20 | 20 | 0 | 20.03 | 19.26s | 1.07s | 0.88s | 中等并发完整业务闭环通过 |
| `/tmp/workorch-loadtest-load-20260501012135-afnyn31c` | mock | 10 | 50 | 50 | 0 | 14.38 | 54.11s | 2.50s | 2.35s | 高并发业务正确，但读模型和持久化延迟明显放大 |
| `/tmp/workorch-loadtest-load-20260501012523-adj084hg` | real | 1 | 2 | 1 | 1 | 0.22 | 187.07s | 0.79s | 0.62s | 真实 LLM 样本可完成，但存在模型跳过第二轮 Decision 的协议波动 |

关键结论：

- Mock 压测已经覆盖两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact 和 timeline runState，业务正确性通过。
- `server.maxConcurrentRuns=100` 下，失败不再来自调度槽不足；在 mock `concurrency=10,total=50` 下吞吐反而低于 `concurrency=5,total=20`，说明主要瓶颈转向 file backend 与读模型回查。
- 状态推进阶段延迟随数据规模增长明显放大：`toolExecutionAndResultMaterialization` 从并发 2 的 p95 1.45s 上升到并发 10 的 p95 18.46s。
- Snapshot / observability 查询延迟也随数据规模上升：snapshot p95 从 0.22s 上升到 2.50s，observability p95 从 0.22s 上升到 2.35s。
- 真实 LLM 的失败样本进入 `blocked_on_approval`，但压测协议期望第二轮 Decision；这属于 provider 输出协议稳定性问题，不应作为 workorch 内部并发容量失败。

已落实优化：

- 在 repository 层补充正式窄查询能力：`RunRepository.ListByWorkspaceID`、`DecisionTicketRepository.ListByRunID/ListByWorkspaceID`、`ApprovalRepository.ListByRunID/ListByWorkspaceID`。
- 在 workspace-scoped file backend 中按 workspace/run 根目录读取相关数据，避免 workspace snapshot 扫描所有 workspace 的 Run/Ticket/Approval。
- 在 application read model 中让 `/runs/{runId}/status` 走轻量状态读取，不再构造完整 Run snapshot。
- `readWorkspaceSnapshot`、`decisionsForRun`、`approvalsForRun` 改用 repository 窄查询，保持业务组合在 appservice，数据选择在 repository，未把业务解析下沉到 orchestrator 或 runtime block。

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=50 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

优化方向：

- repository/file：继续补齐 workspace/run scoped index，减少 `Get(id)` 和 `ListByRunID(id)` 对全量 run roots 的扫描。
- application/read-model：为 Workspace Board 提供摘要专用查询，避免列表页读取完整 snapshot、timeline 和 artifact 明细。
- eventstream：对观测时间线增加按 run/workspace 的增量 cursor 和 limit，避免每次全量重建 timeline。
- mcp block：对 transient 与 long_lived session 分别压测；若选择 long_lived，需要同步实现可观测 session 生命周期和清理能力。
- llm block：真实 LLM 压测需单独统计 provider latency、schema repair 次数和 workflow protocol violation，不应混入框架吞吐基准。

## 14. 2026-05-01 部署后复测与第二轮优化

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮复测基于已部署实例，覆盖完整业务闭环：Workspace 创建、Goal/DoD、两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501020545-an6do_9m` | mock | 10 | 50 | 50 | 0 | 25.04 | 23.01s | 37.34s | 2.54s | 2.38s | 第一轮读模型窄查询优化后，同规模吞吐明显提升 |
| `/tmp/workorch-loadtest-load-20260501020937-gbhh7kl7` | mock | 20 | 100 | 100 | 0 | 10.10 | 116.85s | 160.38s | 10.49s | 8.47s | 业务正确性稳定，但已超过效率拐点 |

与上一轮同规模 `concurrency=10,total=50` 对比：

| 指标 | 优化前 `/tmp/workorch-loadtest-load-20260501012135-afnyn31c` | 优化后 `/tmp/workorch-loadtest-load-20260501020545-an6do_9m` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 14.38 workspace/min | 25.04 workspace/min | +74.1% |
| 总耗时 | 208.55s | 119.83s | -42.5% |
| e2e avg | 41.18s | 23.01s | -44.1% |
| e2e p95 | 54.11s | 37.34s | -31.0% |
| queueWaitProxy p95 | 8.23s | 6.95s | -15.6% |
| decisionRound1 p95 | 10.08s | 6.84s | -32.2% |
| decisionRound2 p95 | 10.95s | 6.51s | -40.5% |
| toolExecutionAndResultMaterialization p95 | 18.46s | 11.96s | -35.2% |

关键约束：

- `concurrency=10,total=50` 已能稳定通过，并且吞吐优于上一轮；第一轮优化有效。
- `concurrency=20,total=100` 虽然 100% 通过，但吞吐下降到 `10.10 workspace/min`，说明继续增加 client 并发会导致 file backend 和读模型访问竞争，整体效率反而下降。
- 在 `concurrency=20,total=100` 下，snapshot p95 达到 `10.49s`，observability p95 达到 `8.47s`；这些读路径已经成为用户侧过程可视化的主要约束。
- Run 队列指标在压测前后均为 0，说明瓶颈不在 `maxConcurrentRuns` 调度槽，而在 repository/file 与 read-model 访问放大。

本轮继续落实的第二轮代码优化：

- 在 workspace-scoped file store 中增加 runID 到 run root 的内存定位缓存。
- `RunRepository.Get` 优先通过 runID 定位 run 目录，避免跨所有 workspace/run 目录扫描。
- `StepRepository.ListByRun`、`ToolCallRepository.ListByRun`、`PatchRepository.ListByRun`、`ValidationResultRepository.ListByRun`、`RuntimeCheckpointRepository.ListByRun`、`ResultRepository.ListByRun`、`ArtifactRepository.ListByRun` 改为按 runID 直接读取单个 run 目录。
- `DecisionTicketRepository.ListByRunID`、`ApprovalRepository.ListByRunID` 改为按 runID 直接读取单个 run 目录。
- `EventRecordRepository.List` 在 query 包含 runID 时只读取对应 run 目录和 system event 文件，不再扫描所有 run 目录。
- 该优化仍位于 `internal/modules/application/workorchd/biz/bootstrap` 的 file repository adapter 层，不改变 kernel/orchestrator、runtime block 或业务模型边界。

验证：

```bash
go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/pkg/repository -count=1
go test ./... -count=1 -p 1
```

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=50 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh

WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=20 \
WORKORCH_LOAD_TOTAL=100 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

后续优化建议：

- 若第二轮 run root 缓存复测后 `snapshot/observability` p95 仍超过 3s，应继续推进 read-model summary endpoint，专门服务 Workspace Board 和运行过程列表，避免 UI 常规刷新读取完整 evidence 明细。
- 若 `toolExecutionAndResultMaterialization` p95 仍超过 10s，应对 Result/Artifact/EventRecord 写入做批量化或 append-only compaction，减少单 run 完成阶段的多文件写放大。
- 若真实 LLM 并发继续扩展，应单独限制 provider 并发与重试策略，把 provider latency/protocol violation 与 workorch 内部容量指标分开记录。

## 15. 2026-05-01 run root 缓存部署后复测

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮复测基于已部署实例，部署内容包含 runID -> run root 缓存和 `ListByRun/ListByRunID` 单 run 目录读取优化。所有压测仍覆盖两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501034758-z_2mb4xg` | mock | 10 | 50 | 50 | 0 | 93.56 | 6.05s | 7.75s | 0.30s | 0.28s | 当前最优效率区间，业务闭环稳定 |
| `/tmp/workorch-loadtest-load-20260501034840-2yxmyil9` | mock | 20 | 100 | 100 | 0 | 35.39 | 32.87s | 42.31s | 1.66s | 1.47s | 高压稳定性通过，但效率明显下降 |
| `/tmp/workorch-loadtest-load-20260501035210-d8puc3pl` | mock | 15 | 75 | 75 | 0 | 30.85 | 28.61s | 32.49s | 0.90s | 0.86s | 在已累积更多 file 数据后仍稳定，但吞吐低于 c10 |

与上一轮部署后同规模对比：

| 指标 | c10/t50 优化前 `/tmp/workorch-loadtest-load-20260501020545-an6do_9m` | c10/t50 优化后 `/tmp/workorch-loadtest-load-20260501034758-z_2mb4xg` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 25.04 workspace/min | 93.56 workspace/min | +273.6% |
| 总耗时 | 119.83s | 32.07s | -73.2% |
| e2e avg | 23.01s | 6.05s | -73.7% |
| e2e p95 | 37.34s | 7.75s | -79.2% |
| snapshot p95 | 2.54s | 0.30s | -88.2% |
| observability p95 | 2.38s | 0.28s | -88.3% |

| 指标 | c20/t100 优化前 `/tmp/workorch-loadtest-load-20260501020937-gbhh7kl7` | c20/t100 优化后 `/tmp/workorch-loadtest-load-20260501034840-2yxmyil9` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 10.10 workspace/min | 35.39 workspace/min | +250.4% |
| 总耗时 | 593.88s | 169.56s | -71.4% |
| e2e avg | 116.85s | 32.87s | -71.9% |
| e2e p95 | 160.38s | 42.31s | -73.6% |
| snapshot p95 | 10.49s | 1.66s | -84.2% |
| observability p95 | 8.47s | 1.47s | -82.7% |

关键约束：

- run root 缓存优化显著降低了读模型和运行证据回查成本，`snapshot/observability` 已从 10 秒级下降到 1 秒级以内或附近。
- 当前最优吞吐出现在 `concurrency=10,total=50`，达到 `93.56 workspace/min`。
- `concurrency=15/20` 仍能 100% 完成业务目标，但吞吐明显下降，说明继续增加客户端并发会触发 file backend 多文件写入、事件写入、Result/Artifact 物化和后台续跑竞争。
- `toolExecutionAndResultMaterialization` 仍是最大阶段：c20/t100 p95 为 `15.98s`，c15/t75 p95 为 `11.88s`，后续优化应优先处理完成阶段的写入和证据物化。

本轮追加代码优化：

- `EventRecordRepository.List` 在 query 包含 runID 时不再读取 system event 文件，避免 Run snapshot/observability 的无效系统事件扫描。
- `ToolCallRepository.ListByStep`、`PatchRepository.ListByStep`、`ValidationResultRepository.ListByStep` 从 stepID 推导 runID 后直接定位单个 run 目录，避免未来按 step 查询时退化为全量 run root 扫描。
- 该补丁仍位于 `internal/modules/application/workorchd/biz/bootstrap` file repository adapter 层，不改变业务模型、不修改 orchestrator、不让 runtime block 解析业务目标。

验证：

```bash
go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/pkg/repository -count=1
go test ./... -count=1 -p 1
```

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=50 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

后续优化建议：

- application/read-model：为 Workspace Board 和运行过程列表增加 summary 专用接口，避免 UI 高频刷新触发完整 evidence 明细读取。
- repository/file：对 Result、Artifact、EventRecord 完成阶段写入做批量提交或 append-only compaction，降低 `toolExecutionAndResultMaterialization` 尾延迟。
- runtime/orchestrator：保持后台续跑语义，不恢复同步长请求；继续通过 metrics 观察 in-flight 与 queue，但不要把调度器改成解析业务目标的优化点。
- blocks/mcp：如需要进一步提升工具阶段吞吐，单独压测 transient 与 long_lived session policy，长会话必须配套生命周期清理和隔离。

## 16. 2026-05-01 event/step 查询优化部署后复测

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮复测基于已部署实例，部署内容包含 run root 缓存、run 级 event 查询跳过 system event 文件、`ListByStep` 通过 stepID 定位 run root 等优化。压测仍覆盖完整业务闭环：两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501035945-72ra_5xi` | mock | 10 | 50 | 50 | 0 | 101.51 | 5.64s | 7.18s | 0.28s | 0.25s | 当前最佳效率区间继续提升 |
| `/tmp/workorch-loadtest-load-20260501040028-674z9h5s` | mock | 20 | 100 | 100 | 0 | 48.82 | 23.26s | 35.18s | 1.27s | 1.18s | 高压吞吐和尾延迟继续改善 |

与上一轮同规模对比：

| 指标 | c10/t50 上一轮 `/tmp/workorch-loadtest-load-20260501034758-z_2mb4xg` | c10/t50 本轮 `/tmp/workorch-loadtest-load-20260501035945-72ra_5xi` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 93.56 workspace/min | 101.51 workspace/min | +8.5% |
| 总耗时 | 32.07s | 29.55s | -7.9% |
| e2e avg | 6.05s | 5.64s | -6.7% |
| e2e p95 | 7.75s | 7.18s | -7.3% |
| snapshot p95 | 0.30s | 0.28s | -5.2% |
| observability p95 | 0.28s | 0.25s | -11.9% |

| 指标 | c20/t100 上一轮 `/tmp/workorch-loadtest-load-20260501034840-2yxmyil9` | c20/t100 本轮 `/tmp/workorch-loadtest-load-20260501040028-674z9h5s` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 35.39 workspace/min | 48.82 workspace/min | +37.9% |
| 总耗时 | 169.56s | 122.91s | -27.5% |
| e2e avg | 32.87s | 23.26s | -29.2% |
| e2e p95 | 42.31s | 35.18s | -16.9% |
| snapshot p95 | 1.66s | 1.27s | -23.6% |
| observability p95 | 1.47s | 1.18s | -19.7% |
| toolExecutionAndResultMaterialization p95 | 15.98s | 13.39s | -16.2% |

当前关键约束：

- `c10/t50` 已达到 `101.51 workspace/min`，继续作为当前推荐本地 file backend 压测并发基线。
- `c20/t100` 仍 100% 正确完成，但 e2e p95 为 `35.18s`，说明高并发下完成阶段仍有写入/物化放大。
- `toolExecutionAndResultMaterialization` 仍是最大阶段，c20/t100 p95 为 `13.39s`；主要风险点是 Result、Artifact、EventRecord、Checkpoint 等 run 级 JSON 文件多次重写。
- snapshot/observability 已稳定在 1 秒级左右，不再是当前首要瓶颈。

本轮新增代码优化：

- `internal/pkg/repository/file.saveMap` 从 `json.MarshalIndent` 改为 `json.Marshal`，减少 file backend 每次写入的数据量、序列化 CPU 和磁盘 IO。
- 该优化只改变 file repository 的落盘编码格式，不改变模型结构、仓储接口或业务语义；历史缩进 JSON 仍可由 `json.Unmarshal` 读取。
- 优化边界在 `internal/pkg/repository/file`，不涉及 orchestrator、runtime block 或业务目标解析。

验证：

```bash
go test ./internal/pkg/repository/file ./internal/pkg/repository ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./... -count=1 -p 1
```

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=50 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh

WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=20 \
WORKORCH_LOAD_TOTAL=100 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

后续优化建议：

- 若紧凑 JSON 部署后 `toolExecutionAndResultMaterialization` p95 仍超过 10s，应进一步评估 run 级 EventRecord append-only JSONL 或 per-run event index，避免每次 append 都重写完整 event map。
- 若 UI 高频刷新继续产生压力，应优先开发 Workspace Board summary read model，而不是在页面端继续读取完整 snapshot。
- 若需要继续提升 c20 以上吞吐，应把 file backend 视为 MVP 本地部署后端上限，并准备 SQLite/Badger 等正式持久化后端方案。

## 17. 2026-05-01 紧凑 JSON 部署后复测与 workspace index 优化

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮复测基于已部署实例，部署内容包含 `saveMap` 紧凑 JSON 写入。压测仍覆盖完整业务闭环：两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | snapshot p95 | workspace snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501040659-x2ep4yu1` | mock | 10 | 50 | 50 | 0 | 101.60 | 5.47s | 6.69s | 0.28s | 0.26s | 0.23s | c10 仍为当前最高效率区间，业务闭环稳定 |
| `/tmp/workorch-loadtest-load-20260501040822-84qs1lsl` | mock | 20 | 100 | 100 | 0 | 42.49 | 27.17s | 35.94s | 1.56s | 1.82s | 1.47s | 高压业务正确性通过，但吞吐低于上一轮 |

与上一轮同规模对比：

| 指标 | c10/t50 上一轮 `/tmp/workorch-loadtest-load-20260501035945-72ra_5xi` | c10/t50 本轮 `/tmp/workorch-loadtest-load-20260501040659-x2ep4yu1` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 101.51 workspace/min | 101.60 workspace/min | +0.1% |
| e2e avg | 5.64s | 5.47s | -3.0% |
| e2e p95 | 7.18s | 6.69s | -6.8% |
| snapshot p95 | 0.28s | 0.28s | 基本持平 |
| observability p95 | 0.25s | 0.23s | -9.1% |

| 指标 | c20/t100 上一轮 `/tmp/workorch-loadtest-load-20260501040028-674z9h5s` | c20/t100 本轮 `/tmp/workorch-loadtest-load-20260501040822-84qs1lsl` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 48.82 workspace/min | 42.49 workspace/min | -13.0% |
| e2e avg | 23.26s | 27.17s | +16.8% |
| e2e p95 | 35.18s | 35.94s | +2.2% |
| snapshot p95 | 1.27s | 1.56s | +23.2% |
| workspace snapshot p95 | 未记录 | 1.82s | 本轮暴露为读模型约束 |
| observability p95 | 1.18s | 1.47s | +25.0% |
| toolExecutionAndResultMaterialization p95 | 13.39s | 11.75s | -12.3% |

当前判断：

- `saveMap` 紧凑 JSON 对 c10 基本持平，对 c20 的完成阶段物化有局部收益，`toolExecutionAndResultMaterialization` p95 从 `13.39s` 降到 `11.75s`。
- c20 整体吞吐下降，且 workspace snapshot、snapshot、observability p95 上升，不能把紧凑 JSON 判定为整体容量提升；更可能是当前 file backend 数据量累计后，全局索引和读模型读取被放大。
- 分阶段 p95 显示 `workspace_create=0.98s`、`goal_create=1.21s`、`dod_create=1.03s`、`approval_to_completed=11.75s`、`workspace_snapshot_read=1.82s`，说明瓶颈已经从单纯 Run 证据读取扩展到 workspace 索引和全局文件写入。
- 当前本地 `.workorch-data/system/workspace_index.json` 样本包含 `150` 个 workspace entry，缩进 JSON 为 `41562` bytes，紧凑 JSON 为 `34054` bytes，约减少 `18.1%` 索引写入体积。
- 本轮仍保持 100% 成功率、0 HTTP 4xx/5xx、0 curl error、0 LLM protocol error、0 timeline missing runState，业务正确性符合压测要求。

本轮新增代码优化：

- `internal/modules/application/workorchd/biz/bootstrap.workspaceScopedFileStore` 增加 `workspace_index.json` 进程内缓存，避免每次 workspace create/update/list/delete 都重新读取和反序列化完整索引。
- `workspace_index.json` 写入从 `json.MarshalIndent` 改为 `json.Marshal`，降低全局索引文件写入大小、序列化 CPU 和磁盘 IO。
- 该优化位于 application/workorchd 的 file repository adapter 层，只处理本地 file backend 的索引读写性能；不改变 kernel/orchestrator、runtime block、业务模型或业务目标解析边界。

验证：

```bash
go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/pkg/repository/file -count=1
go test ./... -count=1 -p 1
```

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=10 \
WORKORCH_LOAD_TOTAL=50 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh

WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=20 \
WORKORCH_LOAD_TOTAL=100 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

下一步优化备选：

- 若 workspace index 缓存复测后 c20/t100 的 create/update 阶段 p95 仍超过 1s，应将全局 `workspace_index.json` 进一步拆成 append-only index event 或 per-workspace index entry，避免全局文件随 workspace 数量增长反复重写。
- 若 `approval_to_completed` p95 仍超过 10s，应推进 run 级 EventRecord append-only JSONL 或批量写入，减少完成阶段 Result、Artifact、EventRecord、Checkpoint 多文件重写。
- 若 UI 高频读取继续影响压测，应新增 Workspace Board summary read model，让总览页读取轻量投影，而不是完整 Run/Workspace snapshot。

## 18. 2026-05-01 workspace index 缓存部署后复测与 file map 缓存优化

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
```

本轮复测基于已部署实例，部署内容包含 `workspace_index.json` 进程内缓存和紧凑写入。压测继续覆盖两轮 Decision、一次 Approval、MCP tool call、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。

已完成压测：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | approval_to_completed p95 | snapshot p95 | workspace snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501042732-f896s2eh` | mock | 10 | 50 | 50 | 0 | 100.99 | 5.68s | 8.24s | 3.45s | 0.34s | 0.44s | 0.26s | c10 仍稳定，吞吐与上一轮基本持平 |
| `/tmp/workorch-loadtest-load-20260501042810-84cymxg3` | mock | 20 | 100 | 100 | 0 | 65.70 | 17.46s | 23.40s | 7.75s | 0.98s | 1.08s | 0.96s | c20 明显改善，是当前 file backend 推荐压测上限 |
| `/tmp/workorch-loadtest-load-20260501043011-sda957zc` | mock | 30 | 150 | 150 | 0 | 22.28 | 77.99s | 120.81s | 37.71s | 5.75s | 5.60s | 5.03s | c30 正确性通过，但效率严重退化 |

与上一轮 `c20/t100` 对比：

| 指标 | index 缓存前 `/tmp/workorch-loadtest-load-20260501040822-84qs1lsl` | index 缓存后 `/tmp/workorch-loadtest-load-20260501042810-84cymxg3` | 变化 |
| --- | ---: | ---: | ---: |
| 成功率 | 100% | 100% | 持平 |
| 吞吐 | 42.49 workspace/min | 65.70 workspace/min | +54.6% |
| e2e avg | 27.17s | 17.46s | -35.8% |
| e2e p95 | 35.94s | 23.40s | -34.9% |
| workspace_create p95 | 0.98s | 0.65s | -33.9% |
| goal_create p95 | 1.21s | 0.65s | -46.4% |
| dod_create p95 | 1.03s | 0.73s | -28.5% |
| approval_to_completed p95 | 11.75s | 7.75s | -34.1% |
| snapshot p95 | 1.56s | 0.98s | -37.4% |
| workspace snapshot p95 | 1.82s | 1.08s | -40.8% |
| observability p95 | 1.47s | 0.96s | -34.7% |

当前判断：

- workspace index 缓存对高并发有明确收益，`c20/t100` 吞吐提升 `54.6%`，且所有关键 p95 均下降。
- `c30/t150` 虽然 100% 完成业务目标，但吞吐下降到 `22.28 workspace/min`，e2e p95 达到 `120.81s`；说明 file backend 在当前实现下不适合继续提高 client 并发。
- `c30` 下所有阶段同步放大，最大瓶颈仍是 `approval_to_completed` p95 `37.71s`，同时 create/update/read p95 也达到 4-6 秒级；主要约束是 JSON map 文件反复读取、反序列化和重写造成的 IO/CPU 放大。
- 当前推荐容量基线：本地 file backend、mock LLM、transient MCP 下以 `concurrency=20,total=100` 作为高压验收基线；`concurrency=10,total=50` 作为快速回归基线。

本轮新增代码优化：

- `internal/pkg/repository/file` 为 JSON map 文件增加进程内解码缓存，`loadMap` 首次读取后缓存已解码 map，`saveMap` 成功写盘后同步刷新缓存。
- 缓存返回 map 副本，避免调用方直接修改缓存对象；各 repository 仍保持自身读写锁。
- 该优化位于 file repository 基础仓储层，不改变 repository 接口、业务模型、orchestrator、runtime block 或业务目标解析边界。
- 边界约束：file backend 仍按当前服务进程作为本地数据管理者设计；不把外部进程直接修改 JSON 文件后的即时可见性作为高并发路径优化目标。

验证：

```bash
go test ./internal/pkg/repository/file ./internal/pkg/repository ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./... -count=1 -p 1
```

待重新部署后复测：

```bash
WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=20 \
WORKORCH_LOAD_TOTAL=100 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh

WORKORCH_LOAD_LLM_MODE=mock \
WORKORCH_LOAD_CONCURRENCY=30 \
WORKORCH_LOAD_TOTAL=150 \
WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS=100 \
WORKORCH_LOAD_MIN_SUCCESS_RATE=1.0 \
./scripts/loadtest-governed-workspace-concurrency.sh
```

后续优化建议：

- 如果 file map 缓存部署后 `c30` 仍吞吐低于 `c20`，不要继续扩大并发，应把 `c20` 固化为 file backend 的当前推荐上限。
- 如果 `approval_to_completed` p95 仍超过 10s，应推进 run 级 EventRecord append-only JSONL 或批量写入，避免完成阶段多次重写 run 级 JSON map。
- 如果 snapshot/observability p95 仍超过 1s，应新增 Workspace Board summary read model，让 Web 高频读取轻量投影。

## 19. 2026-05-01 并发压测阶段性收尾结论

本轮并发压测阶段性收尾。结论适用于当前 MVP 本地部署形态：

```text
repository.backend = file
server.maxConcurrentRuns = 100
LLM 容量基线 = mock
MCP sessionPolicy = transient
```

最终验收基线：

| 用途 | LLM | client 并发 | 总量 | 通过标准 | 说明 |
| --- | --- | ---: | ---: | --- | --- |
| 快速回归 | mock | 10 | 50 | 成功率 100%，无 HTTP/LLM/timeline 异常 | 用于日常代码变更后快速确认完整治理闭环 |
| 高压验收 | mock | 20 | 100 | 成功率 100%，无 HTTP/LLM/timeline 异常 | 当前 file backend 推荐容量上限 |
| 真实端到端抽检 | real | 1-2 | 小批量 | 成功完成业务闭环，单独记录 provider 协议问题 | 不作为 workorch 内部容量基准，不进入 CI |

不再作为常规验收项：

| 场景 | 结论 | 原因 |
| --- | --- | --- |
| `concurrency >= 30` 的常规容量压测 | 阶段性停止 | `c30/t150` 虽 100% 完成，但吞吐下降到 `22.28 workspace/min`，e2e p95 达到 `120.81s` |
| 单纯继续提高 `server.maxConcurrentRuns` | 阶段性停止 | Run 队列执行槽不是当前主瓶颈；瓶颈在 file backend JSON map 读写、Result/Artifact/EventRecord 物化和读模型回查 |
| 用真实 LLM 作为容量基准 | 阶段性停止 | provider latency、限流和协议稳定性会污染 workorch 内部容量判断 |

已确认能力：

- Mock 容量压测完整覆盖两轮 Decision Ticket、一次 Approval、MCP ToolCall、ValidationResult、RuntimeCheckpoint、accepted Result、linked Artifact、Run snapshot、Workspace snapshot、observability timeline。
- 在当前 file backend 下，`c20/t100` 是业务正确性与效率的平衡点；最近有效批次 `/tmp/workorch-loadtest-load-20260501042810-84cymxg3` 达到 `65.70 workspace/min`。
- `c30/t150` 验证了业务正确性和稳定性仍可保持，但效率严重退化，因此不应作为 MVP 常规验收目标。
- 本轮优化保持模块边界：性能治理集中在 `internal/pkg/repository/file`、`internal/modules/application/workorchd/biz/bootstrap` 和 read-model 使用路径，不向 `kernel/orchestrator` 或 runtime blocks 注入具体业务解析逻辑。

已识别的下一阶段性能专项，而非本轮并发压测遗留：

- 持久化事件流：将 run 级 `EventRecord` 从 JSON map 多次重写推进为 append-only JSONL、per-run event index 或正式持久化后端。
- 正式持久化后端：若目标继续提升 `c20` 以上吞吐，应引入 SQLite、Badger、Postgres 等后端，而不是继续压榨 file backend。
- 轻量读模型：为 Workspace Board / 运行过程列表增加 summary read model，避免 Web 高频刷新完整 snapshot / observability。
- 完成阶段批量物化：对 Result、Artifact、RuntimeCheckpoint、EventRecord 的完成阶段写入做批量提交或异步投影，降低 `approval_to_completed` p95。

收尾决策：

- 当前并发压测作为 MVP file backend 容量验证已完成。
- 后续普通功能开发只需执行快速回归 `c10/t50` 或高压验收 `c20/t100`。
- 只有当持久化后端、事件流或 summary read model 发生实质变更时，才重新开启容量曲线压测。
- `c30+` 后续只作为专项性能实验，不作为常规验收阻塞项。

## 20. 2026-05-01 QC29/QC30 后全量回归与并发复测

部署参数：

```text
repository.backend = file
server.maxConcurrentRuns = 100
LLM 容量基线 = mock
MCP sessionPolicy = transient
```

本轮复测目标是在继续单 Workspace 优化前，确认最近的 Approval /
Decision governance 读复用优化没有影响整体功能和并发能力。

全量回归结果：

- `go test ./... -count=1 -p 1` 通过。
- `make verify-boundary-contract` 通过；期间清理了 Web 通用占位文案中的具体业务词，避免 runtime/Web 边界检查误报。
- `make verify-deployed-full` 通过，覆盖已部署实例的健康检查、Workspace Template / Soul、mock LLM、MCP stdio、SkillHub、真实 LLM、真实天气查询、多轮 Ticket + Approval、真实 MCP stdio、向量 Memory recall。
- 真实 LLM provider：`miniMax`。
- 真实多轮治理样本：`workspace=ws-2574db0bb97bdaf4`，`run=run-e0ad2a8a4f40a6f4`，最终 `completed`，包含 1 个 Ticket、1 个 Approval、5 个 Step、5 个 ToolCall、5 个 ValidationResult、17 个 RuntimeCheckpoint、1 个 Result、2 个 Artifact。

并发复测结果：

| 批次 | LLM | client 并发 | 总量 | 成功 | 失败 | 吞吐 workspace/min | e2e avg | e2e p95 | approval_to_completed p95 | snapshot p95 | workspace snapshot p95 | observability p95 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/tmp/workorch-loadtest-load-20260501081203-dozcd5u8` | mock | 10 | 30 | 30 | 0 | 305.59 | 1.78s | 2.04s | 0.44s | 0.23s | 0.16s | 0.16s | 快速并发冒烟通过，完整治理闭环稳定 |
| `/tmp/workorch-loadtest-load-20260501081248-iu5vtgtp` | mock | 20 | 50 | 50 | 0 | 114.89 | 9.33s | 12.26s | 3.20s | 1.26s | 0.74s | 0.80s | 中等样本通过，暴露高压下 file backend 尾延迟 |
| `/tmp/workorch-loadtest-load-20260501081428-ux6t_kpd` | mock | 20 | 100 | 100 | 0 | 68.13 | 17.06s | 23.22s | 5.31s | 1.69s | 1.56s | 1.39s | 高压验收基线通过，最近修改未造成并发回归 |

错误统计：

| 批次 | HTTP 4xx | HTTP 5xx | curl error | LLM protocol error | timeline missing runState | failure class |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `load-20260501081203` | 0 | 0 | 0 | 0 | 0 | `{}` |
| `load-20260501081248` | 0 | 0 | 0 | 0 | 0 | `{}` |
| `load-20260501081428` | 0 | 0 | 0 | 0 | 0 | `{}` |

高压验收批次 `load-20260501081428` 的每个 Workspace 均包含：

- 2 个 Decision Ticket。
- 1 个 Approval。
- 5 个 LLM Step。
- 6 个 Step / ToolCall / ValidationResult。
- 21 个 RuntimeCheckpoint。
- 1 个 accepted Result。
- 2 个 linked Artifact。
- timeline `runState` 完整。

结论：

- 最近的 `Approval ensure loaded Approval reuse` 与 `Decision ensure loaded Ticket reuse` 没有引入功能回归或并发回归。
- 当前已部署实例在 file backend、`maxConcurrentRuns=100`、mock LLM、transient MCP 下仍满足既定高压验收基线 `c20/t100`。
- 与前期最佳 `c20/t100` 批次相比，本轮吞吐 `68.13 workspace/min` 高于收尾基线 `65.70 workspace/min`，e2e p95 `23.22s` 与历史 `23.40s` 基本持平。
- 当前主要性能约束仍是高并发下 file backend 写入/读模型回查放大，`approval_to_completed` p95 `5.31s`、snapshot p95 `1.69s`、workspace snapshot p95 `1.56s`、observability p95 `1.39s`。这不属于 QC29/QC30 引入的新问题。
- 可以进入下一步单 Workspace 从 Goal 到 DoD 通过的效率优化；该优化应继续使用单 workspace governed 基线量化，不应与本轮并发容量复测混淆。
