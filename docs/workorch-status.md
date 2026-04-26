# workorch Status

## Current repository state

- module: `github.com/muidea/workorch`
- executable entry: `application/workorchd`
- framework baseline:
  - `application/workorchd/main.go` starts through `magicCommon/framework/application` and explicitly imports the selected `workorchruntime`, `kernel`, and `blocks` units
  - `internal/initiators/workorchruntime` provides repository, event hub, and cross-module service wiring for plugin-loaded modules
  - `internal/modules/application/workorchd` attaches `internal/modules/application/workorchd/service/server.Service` to the `magicCommon/framework/plugin/module` lifecycle
  - HTTP requests pass through `magicEngine/http` `RouteRegistry` / `HTTPServer`; current acceptance API endpoints are registered as explicit native routes
  - `/healthz`, `/readyz`, and `/metrics` are registered as native `magicEngine/http` routes
  - workspace / run read endpoints are registered as native `magicEngine/http` routes:
    `GET /api/v1/workorch/workspaces`,
    `GET /api/v1/workorch/workspaces/{workspaceId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/goals`,
    `GET /api/v1/workorch/goals/{goalId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/dods`,
    `GET /api/v1/workorch/dods/{dodId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/runs` for explicit `405 method not allowed`,
    `GET /api/v1/workorch/runs`,
    `GET /api/v1/workorch/runs/{runId}`,
    `GET /api/v1/workorch/runs/{runId}/snapshot`,
    `GET /api/v1/workorch/runs/{runId}/status`,
    `GET /api/v1/workorch/decisions`,
    `GET /api/v1/workorch/decisions/{ticketId}`,
    `GET /api/v1/workorch/approvals`,
    `GET /api/v1/workorch/approvals/{approvalId}`,
    `GET /api/v1/workorch/souls`,
    `GET /api/v1/workorch/souls/{soulId}`,
    `GET /api/v1/workorch/skills`,
    `GET /api/v1/workorch/skills/{skillId}`,
    `GET /api/v1/workorch/mcp/servers`,
    `GET /api/v1/workorch/mcp/servers/{serverId}`,
    `GET /api/v1/workorch/mcp/servers/{serverId}/tools`,
    `GET /api/v1/workorch/memory?workspaceId={workspaceId}`
  - workspace / run / decision / approval write and execution endpoints are registered as native `magicEngine/http` routes:
    `POST /api/v1/workorch/workspaces`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/goals`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/dods`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/memory`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/runs`,
    `POST /api/v1/workorch/runs/{runId}/start`,
    `POST /api/v1/workorch/runs/{runId}/resume`,
    `POST /api/v1/workorch/decisions/{ticketId}/resolve`,
    `POST /api/v1/workorch/approvals/{approvalId}/approve`,
    `POST /api/v1/workorch/approvals/{approvalId}/reject`,
    `POST /api/v1/workorch/souls`,
    `POST /api/v1/workorch/skills`,
    `POST /api/v1/workorch/mcp/servers`
  - `internal/modules/application/workorchd/biz/bootstrap` now uses a `magicCommon/event.Hub` backed workorch event adapter by default
  - request-scoped `RuntimeSession` construction now reuses the application-injected EventHub when available
  - `internal/modules/application/workorchd/biz/bootstrap` now runs request-scoped runtime modules through a local magicCommon plugin manager lifecycle
- shared internal runtime packages: `internal/pkg/{common,events,repository}`
- kernel modules:
  - `internal/modules/kernel/workspace`
  - `internal/modules/kernel/run`
  - `internal/modules/kernel/decision`
  - `internal/modules/kernel/orchestrator`
  - `internal/modules/kernel/soul`
  - `internal/modules/kernel/memory`
  - `internal/modules/kernel/skill`
  - `internal/modules/kernel/mcp`
- block modules:
  - `internal/modules/blocks/approval`
  - `internal/modules/blocks/legacy`
  - `internal/modules/blocks/llm`
  - `internal/modules/blocks/shell`
- Workspace execution activation now creates business Runs whose first Step is
  `llm/workspace_execution`; `legacy/minimal` remains only as the default for
  explicit legacy/manual run paths.
- Workspace execution activation accepts and persists selected Soul, Skill, MCP
  server, and Memory context on Run execution context; the same context is
  passed to the first LLM Step input.
- Missing or disabled LLM provider configuration fails the LLM Step and Run
  with observable runtime evidence instead of silently completing through the
  legacy adapter.
- Ready LLM provider configuration now creates a real OpenAI-compatible
  Chat Completions HTTP client inside `blocks/llm`; `openai` defaults to
  `https://api.openai.com/v1`, while `compatible` requires
  `[llm.providers.<id>].baseURL`.
- OpenAI-compatible LLM calls use JSON object mode and a strict
  `workorch.llmExecutionDecision.v1` contract; provider formatting drift is
  normalized only for markdown fences and first JSON object extraction.
- Schema-invalid LLM output now triggers exactly one provider repair retry. A
  successful repair exposes `llmRepair` metadata on Step output; a failed repair
  keeps the original schema failure and exposes parse/repair diagnostics.
- `workorchd` now loads runtime parameters through magicCommon
  `framework/configuration` via `workorchd -config-dir <dir>`;
  `config/application.toml` is the default local template and
  `config/file/application.toml` is the file-backed local template.
- Web Console Configuration now updates LLM provider definitions through
  `PUT /api/v1/workorch/configuration/llm`; `workorchd` persists the update to
  `application.toml`, reloads framework configuration, redacts API keys in read
  responses, and `blocks/llm` resolves provider settings at Step execution time.
- LLM decision output now maps to runtime actions and gates: shell command
  dispatch, MCP tool-call dispatch, execution graph planning, patch
  evidence, Memory write request evidence, structured Decision Ticket fields,
  Approval gates, DoD assessment, and explicit failed LLM decisions.
- `LLMExecutionDecision.executionGraph` is now a formal strict model; valid
  graph plans become orchestrator-consumable Step output while graph traversal
  and ready-node concurrent execution are owned by `kernel/orchestrator`.
- Block execution graph dispatch now runs all currently ready graph nodes
  concurrently when their dependencies are satisfied; follow-up graph batches
  are coordinated centrally so individual parallel steps do not race to close
  or advance the Run.
- Graph leaf completion now returns to `llm/workspace_execution` for final DoD
  assessment when an LLM-driven Run still requires DoD evidence, instead of
  failing immediately after the last tool block.
- The shell block now creates the Workspace working directory before executing
  a command, so default generated Workspace paths are usable by runtime blocks.
- `blocks/mcp` now registers `mcp/tool_call` as a runtime adapter. It validates
  MCP server enablement and `allowedTools`, invokes HTTP/HTTPS endpoints or
  MCP `stdio` JSON-RPC tool calls, and records output through the standard Step
  / ToolCall / ValidationResult evidence path. Unsupported transports still
  fail explicitly instead of falling back to `legacy/minimal`.
- LLM-driven Runs that bind a DoD now require a satisfied, evidence-backed
  `dodAssessment` before completion; missing or invalid assessments fail the
  terminal Step and Run.
- Runtime `memoryWrites` and terminal LLM run summaries now persist as
  Workspace Memory records with Run source refs; created Memory IDs are exposed
  on Step output as `memoryRefs` for later snapshots and recall.
- Embedded Web Console can start LLM-driven Workspace execution with
  Goal/DoD/Soul/Skill/MCP/Memory context and observe execution context,
  Decision context, Step output, DoD assessment, and Memory refs.
- LLM-driven Workspace execution has repeatable local acceptance scripts:
  `make verify-llm` and `make verify-llm-file`, backed by a local
  OpenAI-compatible mock provider, plus `make verify-real-llm` for an already
  running service with a configured real provider.
- MCP `stdio` runtime execution has repeatable local acceptance scripts:
  `make verify-mcp-stdio` and `make verify-mcp-stdio-file`, which create a
  governed MCP server, invoke it through an LLM execution graph, and verify
  restart-read continuity for the file backend.
- `make verify-real-mcp-stdio` verifies an already running `workorchd` instance
  by temporarily adding a local mock provider through the configuration API and
  executing a governed MCP `stdio` tool call in the deployed process.
- MVP verification is grouped by target:
  `make verify-mvp` for local memory-backed checks,
  `make verify-mvp-file` for file-backed restart-read checks, and
  `make verify-real-mvp` for an already running deployed service.

## Delivered in workorch

### Slice A: normal completion chain

Implemented and runnable:

1. create workspace
2. create run
3. start run
4. runtime orchestrator delegates to legacy minimal runtime adapter
5. adapter produces one step
6. step completes
7. run reaches `completed`

### Slice B: decision-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create decision ticket
4. run transitions to blocked-on-decision state
5. resolve ticket
6. resume run
7. runtime orchestrator runs minimal slice
8. run reaches `completed`

### Slice C: approval-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create approval gate
4. run transitions to blocked-on-approval state
5. approve gate
6. resume run
7. runtime orchestrator runs minimal slice
8. run reaches `completed`

### Slice D: approval-rejection terminal path

Implemented and runnable:

1. create workspace
2. create run
3. create approval gate
4. run transitions to blocked-on-approval state
5. reject gate
6. resume run
7. run reaches `failed` without dispatching a runtime step

## Formal Event Boundary

Current formal business events:

- `/workspace/created`
- `/workspace/state/changed`
- `/goal/defined`
- `/dod/defined`
- `/run/created`
- `/run/state/changed`
- `/step/dispatched`
- `/step/state/changed`
- `/ticket/created`
- `/ticket/resolved`
- `/approval/created`
- `/approval/resolved`

Current internal event consumption boundary:

- `internal/modules/...` publish formal workorch business events into the shared `EventHub`
- `internal/modules/application/workorchd/pkg/observed/collector.go` subscribes only to the formal workorch topic set and stores normalized event input
- `internal/modules/application/workorchd/pkg/observed/workorch_contract.go` maps published business events to `/workspace|run|step|decision|approval/snapshot` items
- `internal/modules/application/workorchd/pkg/observed/workorch.go` only falls back to state-derived snapshot items when the current subject has no matching published event
- Current default runtime `EventHub` is backed by `magicCommon/event.Hub` through `internal/pkg/events.MagicHub`.
- The HTTP service injects the application-level EventHub into request contexts, and appservice backend sessions reuse that hub instead of always creating a per-request hub.
- The business-facing `events.Hub` interface remains as a narrow workorch adapter boundary while module lifecycle is migrated.

## Acceptance

The current service MVP is closed. The next product-aligned MVP acceptance scope is tracked in `docs/next-mvp-acceptance.md`.

### Command

```bash
go run ./application/workorchd -config-dir config
```

Optional file-backed service run:

```bash
go run ./application/workorchd -config-dir config/file
```

### Expected service behavior

HTTP endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /api/v1/workorch/configuration`
- `PUT /api/v1/workorch/configuration/llm`
- `POST /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces/{workspaceId}`
- `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`
- `POST /api/v1/workorch/workspaces/{workspaceId}/execution-activations`
- `POST /api/v1/workorch/workspaces/{workspaceId}/goals`
- `GET /api/v1/workorch/workspaces/{workspaceId}/goals`
- `GET /api/v1/workorch/goals/{goalId}`
- `POST /api/v1/workorch/workspaces/{workspaceId}/dods`
- `GET /api/v1/workorch/workspaces/{workspaceId}/dods`
- `GET /api/v1/workorch/dods/{dodId}`
- `POST /api/v1/workorch/workspaces/{workspaceId}/runs`
- `GET /api/v1/workorch/runs`
- `GET /api/v1/workorch/runs/{runId}`
- `GET /api/v1/workorch/runs/{runId}/snapshot`
- `GET /api/v1/workorch/runs/{runId}/status`
- `GET /api/v1/workorch/runs/{runId}/observability`
- `POST /api/v1/workorch/runs/{runId}/start`
- `POST /api/v1/workorch/runs/{runId}/resume`
- all run execution entrypoints now share one in-process FIFO wait queue behind `[server].maxConcurrentRuns`
- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/{ticketId}`
- `POST /api/v1/workorch/decisions/{ticketId}/resolve`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`
- `POST /api/v1/workorch/souls`
- `GET /api/v1/workorch/souls`
- `GET /api/v1/workorch/souls/{soulId}`
- `POST /api/v1/workorch/skills`
- `GET /api/v1/workorch/skills`
- `GET /api/v1/workorch/skills/{skillId}`
- `POST /api/v1/workorch/mcp/servers`
- `GET /api/v1/workorch/mcp/servers`
- `GET /api/v1/workorch/mcp/servers/{serverId}`
- `GET /api/v1/workorch/mcp/servers/{serverId}/tools`
- `POST /api/v1/workorch/workspaces/{workspaceId}/memory`
- `GET /api/v1/workorch/memory?workspaceId={workspaceId}`

Minimal verification:

```bash
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8080/readyz
curl -f http://127.0.0.1:8080/metrics | grep workorch_http_requests_total
WORKSPACE_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d '{"title":"demo workspace","description":"demo workspace description"}')"
WORKSPACE_ID="$(printf '%s\n' "${WORKSPACE_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
test -n "${WORKSPACE_ID}"
GOAL_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/goals \
  -H 'Content-Type: application/json' \
  -d '{"text":"deliver local MVP orchestration"}')"
GOAL_ID="$(printf '%s\n' "${GOAL_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
DOD_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/dods \
  -H 'Content-Type: application/json' \
  -d '{"items":["workspace created","run completed"]}')"
DOD_ID="$(printf '%s\n' "${DOD_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d '{"runId":"run-local-verify","goalId":"'"${GOAL_ID}"'","doDId":"'"${DOD_ID}"'"}'
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/run-local-verify/start
```

File-backed persisted-state verification:

```bash
RUN_ID="run-demo-$(date +%s)"
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/snapshot
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/decisions
curl -f "http://127.0.0.1:8080/api/v1/workorch/decisions?workspaceId=${WORKSPACE_ID}"
curl -f "http://127.0.0.1:8080/api/v1/workorch/decisions?runId=${BLOCKED_RUN_ID}"
TICKET_ID="ticket-run-runtime-decision-block-demo-runtime-decision"
curl -f http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}/resolve
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/resume
APPROVAL_RUN_ID="run-runtime-approval-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/approvals
curl -f "http://127.0.0.1:8080/api/v1/workorch/approvals?workspaceId=${WORKSPACE_ID}"
curl -f "http://127.0.0.1:8080/api/v1/workorch/approvals?runId=${APPROVAL_RUN_ID}"
APPROVAL_ID="approval-run-runtime-approval-block-demo-runtime-approval"
curl -f http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}/approve
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/resume
curl -f http://127.0.0.1:8080/api/v1/workorch/runs
curl -f "http://127.0.0.1:8080/api/v1/workorch/runs?workspaceId=${WORKSPACE_ID}"
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}
```

- the workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `workingDirectory` to `<service-working-directory>/workspaces/<generated-workspace-id>` when omitted
- with the file-backed backend, the run create endpoint creates one formal `workorch` run instance for the requested workspace
- with the file-backed backend, the run list endpoint returns persisted `workorch` run instances and supports `workspaceId` filtering
- with the file-backed backend, the single-run endpoint returns one persisted `workorch` run instance
- with the file-backed backend, the run start endpoint advances one persisted `workorch` run instance and returns its updated terminal or blocked state
- with the file-backed backend, the decision list endpoint returns persisted decision tickets and supports `workspaceId` / `runId` filtering
- with the file-backed backend, the decision get endpoint returns one persisted decision ticket and its resolution fields when present
- with the file-backed backend, the decision resolve and reject endpoints close one formal decision gate instance
- with the file-backed backend, the run resume endpoint can continue after a decision resolve or fail terminally after a decision reject
- with the file-backed backend, the approval list endpoint returns persisted approval items and supports `workspaceId` / `runId` filtering
- with the file-backed backend, the approval get endpoint returns one persisted approval item
- with the file-backed backend, the approval approve and reject endpoints close one formal approval gate instance
- with the file-backed backend, the run resume endpoint can continue after approval approve or fail terminally after approval reject
- with the file-backed backend, the run snapshot endpoint returns one aggregated run read model keyed by `runId`
- with the file-backed backend, the run status endpoint returns one summarized run status keyed by `runId`
- with the file-backed backend, the workspace snapshot endpoint returns one aggregated workspace read model keyed by `workspaceId`
- with the file-backed backend, the workspace list endpoint returns persisted `workorch` workspace instances
- with the file-backed backend, the single-workspace endpoint returns one persisted `workorch` workspace instance

Formal local verification:

```bash
make verify-service
make verify-service-file
```

`make verify-service-file` now includes a same-data-dir restart pass for persisted workspace / run / decision / approval reads.

### File-backed acceptance verification

Verified with:

```bash
go run ./application/workorchd -config-dir config/file
```

Observed persisted files:

- `.workorch-data/workspace.json`
- `.workorch-data/run.json`
- `.workorch-data/step.json`
- `.workorch-data/ticket.json`
- `.workorch-data/approval.json`
- `.workorch-data/resolution.json`

Verified terminal states:

- runnable / decision-blocked-resume / approval-blocked-resume runs persisted as `completed`
- approval-rejected-terminal run persisted as `failed`
- repeated file-backed service execution against the same data directory is supported for the shipped workorch IDs
- completed / failed terminal runs remain queryable after `workorchd` restart on the same file-backed data directory
- repeated `start` / `resume` on terminal file-backed runs now has stable semantics: `completed` `start` / `resume` and `failed` `resume` stay idempotent, while `failed` `start` returns `409 conflict`
- corrupt persisted `workspace.json` / `run.json` now has explicit repository test coverage and is not silently overwritten
- formal `run` events now include `created` plus self-contained `goalId` / `doDId` / `failureReason` payload fields
- formal `workspace state changed` events now include `title`, so event consumers do not need to reconstruct it from state
- `workorchd` now starts and stops through `magicCommon/framework/application`, with HTTP attached through the workorchd plugin module and kernel/block units selected by application imports
- HTTP dispatch now passes through `magicEngine/http`, and current acceptance API endpoints are registered as explicit native routes
- system endpoints `/healthz`, `/readyz`, and `/metrics` now use native `magicEngine/http` route registration
- workspace / run / decision / approval read, write, and execution endpoints now use native `magicEngine/http` route registration
- default runtime event dispatch now uses a `magicCommon/event.Hub` backed adapter and terminates it during `Runtime.Shutdown`
- request-backed runtime sessions now reuse the `magicCommon/framework/application` EventHub injected during service startup
- request-scoped runtime modules now register into a local magicCommon plugin manager for `Run` ordering, while shutdown preserves workorch's context-aware teardown error semantics

### Test baseline

```bash
go test ./...
```

Validated coverage currently includes:

- workspace happy path
- run happy path
- orchestrator happy path
- repository memory path
- repository file-backed path
- shared repository bundle contract across `memory` / `file`
- blocked/resume semantics
- decision ticket resolve conflict guard
- approval resolve conflict guard
- approval reject -> terminal failed path
- application-level smoke test for `workorchd`
- repeatable file-backed service execution against the same data directory
- file-backed terminal read models remain stable across reloads
- file-backed `workorchd` restart preserves terminal run, decision, approval, and workspace snapshot reads
- corrupt file-backed persisted JSON boundary returns explicit errors
- workorch formal business event contract now covers `workspace / run / step / decision / approval`
- run-created and enriched run/workspace event payloads are covered by `events` / `run` / `observed` tests
- framework entry adaptation is covered by `application/workorchd` and `internal/modules/application/workorchd/service/server` tests plus `make verify-service-file`
- magicCommon-backed event adapter is covered by `internal/pkg/events` and appservice / observed integration tests
- application-level EventHub injection is covered by appservice runtime-session tests and service-level verification
- local plugin lifecycle integration is covered by bootstrap lifecycle tests
- native magicEngine system routes are covered by server tests and `make verify-service-file`
- native magicEngine acceptance API routes are covered by server tests and `make verify-service-file`
- accepted API native route registration is now guarded by a route registry test, and wildcard `/**` fallback registration is explicitly forbidden

## Out Of Current Delivery Scope

以下内容不是当前已完成的 MVP / framework alignment 交付范围内的活跃待办项；如后续启动，需要重新拆成正式业务需求：

- global `magicCommon/framework/plugin/module.Register` usage for long-lived workorch runtime units
- richer multi-run scheduling beyond the current in-process FIFO wait queue

当前已按 MVP 形式交付，后续如需增强应作为新需求处理：

- file-backed repository adapter as the first non-memory persistence path

## Runtime Expansion Scope

runtime expansion 交付范围已完成，范围与验收记录见：

- `docs/runtime-expansion-plan.md`

当前活跃项：

- active: none

已完成：

- completed: `R1 External Runtime And Tool Adapter Contract`
- completed: `R2 Controlled Shell Sandbox`
- completed: `R3 Generalized Diff Patch Apply Engine`
- completed: `R4 Block Execution Graph`
- completed: `R5 Observability Pipeline`

## LLM Driven Workspace Execution Design

LLM 驱动 Workspace 执行的设计文档已补齐，范围、边界与交互契约见：

- `docs/llm-driven-workspace-execution-design.md`
- `docs/llm-driven-development-plan.md`

当前状态：

- design: completed
- development plan: completed
- implementation: completed
- active tracking: `docs/llm-driven-development-plan.md`

该设计明确后续实现边界：

- Soul / Skill / MCP / Persistent Memory 是正式业务上下文，不是简单 prompt 拼接
- LLM 只输出结构化 `LLMExecutionDecision`，不直接执行 shell、patch 或 MCP tool
- Orchestrator 继续消费标准 Step output 并负责 Run 状态、Evidence、Decision、Approval
- application/workorchd 只负责 HTTP/Web、执行激活和上下文装配，不承载 provider 或 MCP 协议实现
- blocks/llm 只负责 provider request、provider call、schema parsing 和 normalized Step output
- Web Console 只通过 HTTP API 交互，不暴露 provider secret 或低层 adapter 直接执行入口
- 未配置 LLM provider 时，LLM-driven execution 必须明确失败，不能回落到 `legacy/minimal` 形成假完成

该设计同步定义了后续实现验收的数据交互流程：

- happy path: Goal/DoD/Soul/Skill/MCP/Memory 绑定后，通过 `llm/workspace_execution`、具体 block action、DoD assessment 和 Memory write 完成 Run
- decision path: LLM 请求业务补充信息时创建 Decision Ticket，resolve 后 resume 并把 resolution 放入后续 planning context
- approval path: 风险动作进入 Approval，reject 后不得产生对应 ToolCall 或 Patch
- failure path: provider 缺失或 schema-invalid 时 Step 明确失败，不派发外部动作，不回落到 `legacy/minimal`
- memory recall path: 第一轮 Run 生成 Memory，第二轮 Run 可召回并在 snapshot/observability 中可见

当前细化开发项：

- L1 Soul model and repository: completed
- L2 Memory model and repository: completed
- L3 Skill and MCP registry baseline: completed
- L4 LLM provider configuration: completed
- L5 `blocks/llm` runtime adapter skeleton: completed
- L6 Workspace execution activation context: completed
- L7 Orchestrator LLM step integration: completed
- L8 LLM decision to action mapping: completed
- L9 DoD assessment completion gate: completed
- L10 Memory write and recall loop: completed
- L11 API surface for Soul, Skill, MCP, Memory: completed
- L12 Web console LLM execution views: completed
- L13 End-to-end acceptance scripts: completed
- L14 Status and operator documentation: completed
- L15 LLM decision output stabilization and graph planning: completed
- L16 Runtime graph follow-up and workspace shell sandbox closure: completed
- L17 MCP HTTP tool-call runtime adapter: completed
- L18 Concurrent ready-node graph dispatch: completed
- L19 MCP stdio protocol transport adapter: completed

## MVP Closure Record

MVP 功能收口记录见：

- `docs/mvp-backlog.md`

MVP 功能清单已收口，不再作为活跃待办清单使用。

当前进度：

- `B1 workspace 实例接口` 已完成
- `B2 run 实例接口` 已完成
- `B3 run 启动与恢复接口` 已完成
- `B4 decision 工单查询与处理接口` 已完成
- `B5 approval 工单查询与处理接口` 已完成
- `B6 实例读模型补齐` 已完成
- `S1 仓储后端共享契约测试` 已完成
- `S2 file 后端持久化语义补强` 已完成
- `S3 最小事件总线路径正式化` 已完成
- 当前活跃待办项：无

## Framework Alignment Closure Record

当前 framework 对齐进度：

- completed: `application/workorchd` 通过 `magicCommon/framework/application` 启动
- completed: `internal/modules/application/workorchd/service/server.Service` 接入 `magicCommon/framework/service.Service` 生命周期
- completed: 默认 runtime EventHub 接入 `magicCommon/event.Hub` backed adapter
- completed: request-scoped runtime modules 接入本地 magicCommon plugin manager 生命周期
- completed: HTTP dispatch 进入 `magicEngine/http` `RouteRegistry` / `HTTPServer`
- completed: native system routes for `healthz` / `readyz` / `metrics`
- completed: native workspace / run read routes
- completed: native decision / approval read routes
- completed: native workspace / run / decision / approval write and execution routes
- completed: native route registration guard for accepted API endpoints
- completed: guarded HTTP route registration so accepted API endpoints stay explicit native routes
- active todo: none
