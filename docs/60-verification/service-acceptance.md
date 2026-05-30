# WorkOrch Service Acceptance

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件作为 WorkOrch 服务级验收入口文档。

## Responsibility Boundary

- `../50-product-interface/deployment.md` 负责如何启动、如何挂载配置、如何选择 backend
- 本文负责如何证明当前服务行为、读写路径和运行态投影符合预期
- 完整资源面与端点分组以 `../50-product-interface/service-api.md` 为准

换句话说：

- 若问题是“该用哪种启动方式、配置怎么挂载、backend 怎么切换”
  - 看 `../50-product-interface/deployment.md`
- 若问题是“启动之后哪些行为必须通过验证、由哪些脚本或 fixture 证明”
  - 看本文

## Acceptance Layers

### Layer 1. Service Startup

验证：

- 进程可启动
- `healthz` / `readyz` / `metrics` 可访问
- 当前配置目录 / backend 被正确加载

### Layer 2. Minimal Business Flow

验证：

- create workspace
- define goal
- define DoD
- create run
- start run

### Layer 3. File-Backed Persistence

验证：

- workspace / run / decision / approval / snapshot / observability 可持久化重读
- terminal run 重启后仍可查询
- runtime payload、event stream、Result / Artifact、paginated detail 路径可读取
- 重复 `start` / `resume` 语义稳定

### Layer 4. Deployed Verification Entry

通过 `make verify-service` / `make verify-service-file` 对已部署实例执行最小行为复核。

## Verification Entry Matrix

| Layer | Entry | 目的 |
| --- | --- | --- |
| Service startup | `go run ./application/workorch --server -config-dir config` | 验证本地默认服务可启动 |
| File backend startup | `go run ./application/workorch --server -config-dir config/file` | 验证 file backend 服务可启动 |
| Minimal service | `curl /healthz` / `readyz` / `metrics` | 验证进程、路由和指标可见 |
| Minimal business | workspace/goal/dod/run/start 最小链 | 验证最短业务主链 |
| Deployed service | `make verify-service` / `make verify-service-file` | 验证已部署实例的最小行为 |
| Template/Soul | `make verify-workspace-template-soul` | 验证 template / Soul / workspace setup |
| LLM mock | `make verify-llm` / `make verify-llm-file` | 验证部署实例上的 mock LLM 执行链 |
| LLM real | `make verify-real-llm` | 验证已部署实例接入真实 provider |

## Script Mapping

当前已知脚本 / target 对应关系：

- `make verify-service`
  - 对应 `scripts/verify-workorch-http.sh`
  - 面向当前部署实例的基础 service acceptance
- `make verify-service-file`
  - `verify-service` 的 deployed-instance alias
- `make verify-workspace-template-soul`
  - 对应 `scripts/verify-workspace-template-soul.sh`
  - template / Soul 管理与 template-based workspace setup
- `make verify-llm`
  - 对应 `scripts/verify-llm-workspace-execution.sh`
  - mock provider + 部署实例上的 LLM-driven workspace execution
- `make verify-real-llm`
  - 对应 `scripts/verify-real-llm-workspace-execution.sh`
  - 真实 provider + 部署实例上的最小执行链

更专门的业务链路脚本例如：

- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-requirement-development-orchestration.sh`
- `verify-workspace-web-search-e2e.sh`
- `verify-auto-governance-ticket-approval.sh`

不属于“服务级最小验收”，而属于 template/profile/capability 特化链路验收。

## API / Behavior Mapping

### Minimal Service

- `healthz`
- `readyz`
- `metrics`

用于证明服务可启动、可路由、可暴露指标。

### Minimal Business Flow

对应资源：

- `POST /workspaces`
- `POST /workspaces/:workspaceId/goals`
- `POST /workspaces/:workspaceId/dods`
- `POST /workspaces/:workspaceId/runs`
- `POST /runs/:runId/start`

用于证明最短业务主链可运行。

### File Backend Persistence Focus

重点资源：

- `GET /workspaces/:workspaceId`
- `GET /workspaces/:workspaceId/snapshot`
- `GET /runs/:runId/snapshot`
- `GET /runs/:runId/status`
- `GET /decisions`
- `GET /approvals`
- `GET /runtime/payloads`
- `GET /artifacts/content`
- `GET /events`

用于证明 file backend 的持久化重读、detail 打开与事件重建方向成立。

### Deployed Instance Focus

重点脚本：

- `make verify-service`
- `make verify-service-file`
- `make verify-workspace-template-soul`
- `make verify-llm`
- `make verify-real-llm`

这些脚本应只连接现有服务实例，不应隐式 build/start/restart。

## Script-To-Resource Coverage Matrix

| Script / Target | 主要覆盖资源或行为 |
| --- | --- |
| `make verify-service` | `healthz` / `readyz` / `metrics` / `console` / `configuration` / `workspaces` / `runs` / `events?limit=20` |
| `make verify-service-file` | `verify-service` 的 deployed-instance alias；不启动服务、不切换 backend |
| `make docker-verify-file` / `scripts/verify-workorch-container.sh` | file backend 启动、重启与最小业务主链 |
| `make verify-workspace-template-soul` | souls CRUD / workspace templates CRUD / soul bindings / template-based setup |
| `make verify-llm` | configuration/llm / memory seed / execution activations / runs / decisions / approvals / LLM-driven execution loop |
| `make verify-real-llm` | 真实 provider 下的最小 LLM 执行链 |
| `verify-software-dev-goal-dod-e2e.sh` | software-dev Goal/DoD coverage / repair / result / artifact |
| `verify-software-requirement-development-orchestration.sh` | 更复杂的软件开发 orchestration 链 |
| `verify-workspace-web-search-e2e.sh` | web/search 与 web/fetch 相关执行链 |
| `verify-auto-governance-ticket-approval.sh` | auto governance decision / approval / trace / eventstream |
| `verify-tui-long-output-e2e.sh` | TUI 长输出与结果展示契约 |

### Coverage Reading Rule

使用该矩阵时应区分三类覆盖：

- direct
  - 脚本直接断言该 API / 资源 / 状态
- indirect
  - 脚本通过更高层链路间接覆盖，例如 e2e 中由 workspace execution 顺带覆盖 run snapshot
- none
  - 当前脚本尚未覆盖，需要后续补专门断言

## Endpoint-Level Acceptance Direction

当前服务级验收已经形成“脚本 direct + handler fixture direct + 容器/启动 direct”
三层证据，不再把以下端点视为“只有间接覆盖”：

- service startup
  - `healthz`
  - `readyz`
  - `metrics`
- workspace/read model
  - `workspace`
  - `workspace snapshot`
  - `task-progress`
  - `context-bundle`
  - `task-decompositions`
  - `memory-recalls`
- run/read model
  - `run snapshot`
  - `run status`
  - `run observability`
  - `task-progress`
  - `context-bundle`
  - `task-decompositions`
  - `memory-recalls`
- governance
  - `decisions`
  - `approvals`
- raw/detail
  - `events`
  - `events/stream`
  - `runtime/payloads`
  - `artifacts/content`

### Current Direct Assertion Focus

| Endpoint | 直接断言关注点 |
| --- | --- |
| `GET /runs/:runId/status` | `state`、`blockingTicketId`、`blockingApprovalId`、`failureReason`、`waitMatched/waitElapsedMs` |
| `GET /runs/:runId/snapshot` | `run`、`outcome`、`taskProgress`、`results`、`artifacts` 聚合 |
| `GET /runs/:runId/observability` | `timeline`、`evidence`、`llmUsageSummary`、`closureTraces` |
| `GET /runs/:runId/task-progress` | `taskProgress.state`、`milestones`、`blockers`、`recovery` |
| `GET /runs/:runId/context-bundle` | `contextBundle.sections`、`recentEvidence`、`compressedContext` |
| `GET /runs/:runId/task-decompositions` | `taskDecompositions` 列表和节点总数/状态摘要 |
| `GET /runs/:runId/memory-recalls` | `memoryRecallTraces` 列表和 source/stage 基本字段 |
| `GET /workspaces/:workspaceId/snapshot` | `workspace`、`outcome`、`runs`、`taskProgress` 聚合 |
| `GET /workspaces/:workspaceId/task-progress` | `taskProgress.state`、`blockers`、`recovery` |
| `GET /workspaces/:workspaceId/context-bundle` | workspace 级 `sections` / `compressedContext` |
| `GET /workspaces/:workspaceId/task-decompositions` | workspace 级 decomposition 汇总 |
| `GET /workspaces/:workspaceId/memory-recalls` | workspace 级 recall trace 汇总 |
| `GET /runtime/payloads` | `ref` 成功读取与非法 `ref` 错误方向 |
| `GET /artifacts/content` | `uri` 成功读取与非法 `uri` 错误方向 |
| `GET /events` | `limit` / `cursorAfterId` / filter 生效方向 |
| `GET /events/stream` | SSE 连接、过滤和关闭行为 |
| `GET /decisions` | governance 列表分页、workspace/run 过滤方向 |
| `GET /approvals` | governance 列表分页、workspace/run 过滤方向 |

### Direct Assertion Status

| Endpoint | 当前状态 | 当前直接证据 |
| --- | --- | --- |
| `GET /runs/:runId/status` | direct | `http_test.go` wait/status fixture + `verify-workorch-container.sh` |
| `GET /runs/:runId/snapshot` | direct | `http_test.go` snapshot / replay / cache fixture + `verify-workorch-container.sh` |
| `GET /runs/:runId/observability` | direct | `http_test.go` observability / replay / inflight cache fixture |
| `GET /runs/:runId/task-progress` | direct | `http_test.go` native-read fixture |
| `GET /runs/:runId/context-bundle` | direct | `http_test.go` native-read fixture |
| `GET /runs/:runId/task-decompositions` | direct | `http_test.go` native-read fixture |
| `GET /runs/:runId/memory-recalls` | direct | `http_test.go` recall trace fixture |
| `GET /workspaces/:workspaceId/snapshot` | direct | `http_test.go` workspace snapshot fixture |
| `GET /workspaces/:workspaceId/task-progress` | direct | `http_test.go` native-read fixture |
| `GET /workspaces/:workspaceId/context-bundle` | direct | `http_test.go` native-read fixture |
| `GET /workspaces/:workspaceId/task-decompositions` | direct | `http_test.go` native-read fixture |
| `GET /workspaces/:workspaceId/memory-recalls` | direct | `http_test.go` recall trace fixture |
| `GET /decisions` | direct | `http_test.go` decision list/filter fixture |
| `GET /approvals` | direct | `http_test.go` approval list/filter fixture |
| `GET /runtime/payloads` | direct | `http_test.go` raw ref fixture |
| `GET /artifacts/content` | direct | `http_test.go` artifact content fixture |
| `GET /events` | direct | `verify-workorch-http.sh` + `http_test.go` event replay fixture |
| `GET /events/stream` | direct | `http_test.go` SSE fixture |

### Evidence Source Layers

服务级验收当前已经形成三层直接证据：

- deployed script
  - `verify-workorch-http.sh`
  - `verify-workspace-template-soul.sh`
  - `verify-llm-workspace-execution.sh`
- route / handler fixture
  - `internal/modules/application/workorchd/service/server/http_test.go`
- container / startup fixture
  - `scripts/verify-workorch-container.sh`

三层分工：

- deployed script 证明“现有服务实例”对外可见行为成立
- route / handler fixture 证明 endpoint 参数、错误码和读模型装配方向成立
- container / startup fixture 证明服务从启动到最小运行链的组合行为成立

## Startup Baseline

启动命令本身以 `../50-product-interface/deployment.md` 为准。

本文只把它们当作验收前置条件使用：

- local startup baseline
  - 服务应能按 deployment 文档中的本地默认方式启动
- file-backend startup baseline
  - 服务应能按 deployment 文档中的 file-backend 方式启动

## Minimal Service Verification

```bash
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8080/readyz
curl -f http://127.0.0.1:8080/metrics | grep workorch_http_requests_total
```

## Minimal Business Verification

```bash
WORKSPACE_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d '{\"title\":\"demo workspace\",\"description\":\"demo workspace description\"}')"
WORKSPACE_ID="$(printf '%s\n' \"${WORKSPACE_JSON}\" | sed -n 's/.*\"id\": \"\\([^\"]*\\)\".*/\\1/p' | head -n1)"
test -n "${WORKSPACE_ID}"
GOAL_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/goals \
  -H 'Content-Type: application/json' \
  -d '{\"text\":\"deliver local MVP orchestration\"}')"
GOAL_ID="$(printf '%s\n' \"${GOAL_JSON}\" | sed -n 's/.*\"id\": \"\\([^\"]*\\)\".*/\\1/p' | head -n1)"
DOD_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/dods \
  -H 'Content-Type: application/json' \
  -d '{\"items\":[\"workspace created\",\"run completed\"]}')"
DOD_ID="$(printf '%s\n' \"${DOD_JSON}\" | sed -n 's/.*\"id\": \"\\([^\"]*\\)\".*/\\1/p' | head -n1)"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"run-local-verify\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/run-local-verify/start
```

## File-Backed Acceptance Focus

file backend 需要额外验证：

- workspace / run / decision / approval / snapshot / observability 可持久化重读
- terminal run 重启后仍可查询
- runtime payload、event stream、Result / Artifact 和 paginated detail 路径可读取
- 重复 `start` / `resume` 语义稳定

## Deployed Verification Entry

```bash
make verify-service
make verify-service-file
```

说明：

- 两个目标都应连接当前部署实例
- `make verify-service-file` 只是别名，不负责启动或重启服务
- 若实例尚未启动，先按 `../50-product-interface/deployment.md` 完成启动，再执行这些验收入口
- 需要验证 file backend 启动、重启和持久化重读时，使用 `make docker-verify-file`
  或直接调用 `WORKORCH_VERIFY_BACKEND=file ./scripts/verify-workorch-container.sh`

## File Backend Vs Deployed Instance

file backend 更关注：

- 本地持久化布局
- 重启后 replay / snapshot / observability 可重建
- raw payload 与 artifact content 可显式读取

deployed instance 更关注：

- 当前实例的路由面、最小业务主链和外部可见行为
- 脚本只连接现有服务，不隐式 build/start/restart

对应收口边界：

- file backend 层优先回答“持久化之后能否被正确重读、翻页、回放和 raw 打开”
- deployed instance 层优先回答“当前服务对外暴露的端点、脚本、控制台和最小执行链是否成立”

## Test Baseline

```bash
go test ./...
```

当前基线覆盖重点包括：

- workspace / run happy path
- blocked / resume semantics
- file backend persistence / reload
- formal event contract
- route registration and service startup integration
- request-scoped runtime session / event hub integration

## File Backend Direct Focus

当前 file backend 相关 direct fixture 已覆盖的重点包括：

- `GET /runs/:runId/snapshot`
  - event replay 后 snapshot 可重建
- `GET /runs/:runId/observability`
  - event replay 后 observability 可重建
- `GET /runtime/payloads`
  - `ref` 能打开 raw payload，非法或缺失 ref 会失败
- `GET /artifacts/content`
  - `uri` 能打开 artifact 正文，非法或缺失 uri 会失败
- `GET /events`
  - event list 可作为 replay 输入面
- `GET /events/stream`
  - SSE 作为在线观察面，不替代 file replay

## Paginated Detail Assertion Matrix

当前 paginated detail page 已有稳定 direct fixture，可明确整理为下表：

| Endpoint | 直接 query 断言 | 直接 page 断言 | 当前直接证据 |
| --- | --- | --- | --- |
| `GET /runs/:runId/steps` | `runId=run-native-read`、`cursorAfterId=step-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=step-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/tool-calls` | `runId=run-native-read`、`cursorAfterId=tool-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=tool-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/runtime-checkpoints` | `runId=run-native-read`、`cursorAfterId=checkpoint-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=checkpoint-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/results` | `runId=run-native-read`、`cursorAfterId=result-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=result-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/patches` | `runId=run-native-read`、`cursorAfterId=patch-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=patch-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/validation-results` | `runId=run-native-read`、`cursorAfterId=validation-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=validation-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /runs/:runId/artifacts` | `runId=run-native-read`、`cursorAfterId=artifact-native-1`、`limit=2` | `page.limit=2`、`page.cursorAfterId=artifact-native-1`、`nextCursorAfterId` 前进 | `http_test.go` native-read fixture |
| `GET /decisions` | `workspaceId=ws-native-read`、`runId=run-native-read`、`cursorAfterId=ticket-native-cursor`、`limit=2` | `limit=2`、`cursorAfterId=ticket-native-cursor`、`nextCursorAfterId=ticket-native-read` | `http_test.go` decision page fixture |
| `GET /approvals` | `workspaceId=ws-native-read`、`runId=run-native-read`、`cursorAfterId=approval-native-cursor`、`limit=2` | `limit=2`、`cursorAfterId=approval-native-cursor`、`nextCursorAfterId=approval-native-read` | `http_test.go` approval page fixture |

补充规则：

- run detail page 当前 HTTP 层统一透传 `cursorAfterId` / `limit`
- governance page 同时验证 workspace/run filter 与 cursor pagination
- `events` 也属于分页资源，但其 `limit` 额外受 default/max clamp 保护，见下节

## Event / Raw Content Assertion Matrix

| Endpoint | 直接断言 | 当前直接证据 |
| --- | --- | --- |
| `GET /events` | 默认 `limit`、显式 `limit`、超大 `limit` clamp、负值回退默认值 | `http_test.go` `eventRecordQueryFromRequest` fixture + `verify-workorch-http.sh` |
| `GET /events?workspaceId=...` | workspace filter 在 file-backend restart 后仍可读出 `workspace created / goal defined / dod defined / run created` | `http_test.go` event replay fixture |
| `GET /events?runId=...` | run filter 在 file-backend restart 后仍可读出 replayed run event | `http_test.go` event replay fixture + LLM e2e scripts |
| `GET /events/stream` | SSE 连接、过滤、空闲保持、关闭行为 | `http_test.go` SSE fixture |
| `GET /runtime/payloads` | 合法 `ref` 返回 raw content 与 `X-Workorch-Raw-Ref`；非法 `ref` 失败 | `http_test.go` raw ref fixture |
| `GET /artifacts/content` | 合法 `uri` 返回 artifact body 与 `X-Workorch-Artifact-URI`；非法 `uri` 失败 | `http_test.go` artifact content fixture |
| `GET /artifacts/content` with cached snapshot metadata | 可复用 cached artifact metadata 打开正文 | `http_test.go` cached artifact fixture |

## File Backend Reload Matrix

当前 file-backend reload 已可整理为“首启写入 -> 关闭 -> 重启重读”的明确验收矩阵：

| 验收主题 | 首启阶段动作 | 重启后断言 | 当前直接证据 |
| --- | --- | --- | --- |
| workspace / goal / DoD / run replay | 创建 workspace、goal、DoD、run | `GET /events?workspaceId=...` 仍能读到 persisted workspace/goal/dod/run topics | `http_test.go` event replay fixture |
| run event replay -> snapshot | 创建 run 后关闭服务 | `GET /runs/:runId/snapshot` 仍含 replayed `eventRecords` | `http_test.go` event replay fixture |
| run event replay -> observability | 创建 run 后关闭服务 | `GET /runs/:runId/observability` timeline 仍包含 persisted run event | `http_test.go` event replay fixture |
| raw payload reopening | file backend 写入 raw request payload 文件 | `GET /runtime/payloads?ref=...` 重建后仍可按受控 `ref` 打开 | `http_test.go` raw payload fixture |
| artifact reopening | file backend 写入 artifact metadata + file | `GET /artifacts/content?uri=...` 重建后仍可按受控 `uri` 打开 | `http_test.go` artifact content fixture |
| cached artifact metadata reopening | snapshot cache 中已有 artifact metadata | `GET /artifacts/content` 可直接复用 cached metadata 打开 | `http_test.go` cached artifact fixture |
| completed / failed / resolved governance state persistence | file backend 容器脚本完成 completed run、resolved decision、approved/rejected approval | 重启后 run/decision/approval 仍保持 terminal / resolved state | `verify-workorch-container.sh` file-backend path |

## File Backend Vs Deployed Verification Mapping

同名资源在 file-backend reload 与 deployed verification 中的分工当前可明确为：

| 资源/行为 | file-backend 主要回答 | deployed verification 主要回答 | 当前主要证据 |
| --- | --- | --- | --- |
| `healthz` / `readyz` / `metrics` | 重启后服务仍可恢复最小可用性 | 当前实例对外暴露健康与指标 | `verify-workorch-http.sh` + `verify-workorch-container.sh` |
| `configuration` | 挂载配置能正确重载 | 当前实例配置对外可读 | `verify-workorch-http.sh` + `verify-workorch-container.sh` |
| `workspaces` / `goals` / `dods` / `runs` | 持久化后对象仍能重读 | 当前实例最小业务主链可用 | `verify-workorch-container.sh` + `http_test.go` |
| `run snapshot` / `run status` / `run observability` | replay 后聚合读模型能重建 | 当前实例运行态投影对外可见 | `http_test.go` replay/native-read fixture + `verify-workorch-container.sh` |
| `decisions` / `approvals` | resolved/open/pending state 重启后不丢失 | 当前实例治理链可读可处理 | `verify-workorch-container.sh` + `http_test.go` |
| `events` | replay 输入面是否完整、limit/filter 是否稳定 | 当前实例事件列表对外可见 | `http_test.go` replay/limit fixture + `verify-workorch-http.sh` |
| `events/stream` | 不承担 replay，但不因 file backend 破坏在线观察 | 当前实例 SSE 行为可建立 | `http_test.go` SSE fixture |
| `runtime/payloads` / `artifacts/content` | 重启后 raw/detail 打开是否稳定 | 当前实例 raw/detail 下载入口可用 | `http_test.go` raw/artifact fixture |

## Remaining Gaps By Layer

### Service API Direct Acceptance

当前缺口已经不再是“没有 direct 覆盖”。

本轮已补入：

- paginated detail page 的 cursor / limit 断言矩阵
- file-backend reload 与 deployed verification 的同名端点对照表

### File Backend Reload Acceptance

当前仍缺：

- 某些 detail page 在“真正重启后再次翻页”的逐页实例样本还没有专门 deployed script
- raw payload ref 与 artifact uri 的跨脚本样本仍主要集中在 handler/file-backend fixture，而不是 deployed shell 脚本

## Detailing Boundary

本文在服务级验收层已经给出稳定入口、资源面和 direct evidence 层次。

后续若继续补写，主要只应是：

- detail page 的 deployed shell 级样例
- `events/stream` 的 deployed 断线补偿样例
- `make verify-*` 到 API/read-model/resource 的更细覆盖索引

这些补细不会改变本文当前的验收边界和责任划分。

## Source Migration Note

本文件主要从以下旧文档分离而来：

- `../50-product-interface/deployment.md`
- `../01-overview/current-state.md`
