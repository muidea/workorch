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
    `GET /api/v1/workorch/approvals/{approvalId}`
  - workspace / run / decision / approval write and execution endpoints are registered as native `magicEngine/http` routes:
    `POST /api/v1/workorch/workspaces`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/goals`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/dods`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/runs`,
    `POST /api/v1/workorch/runs`,
    `POST /api/v1/workorch/runs/{runId}/start`,
    `POST /api/v1/workorch/runs/{runId}/resume`,
    `POST /api/v1/workorch/decisions/{ticketId}/resolve`,
    `POST /api/v1/workorch/decisions/{ticketId}/reject`,
    `POST /api/v1/workorch/approvals/{approvalId}/approve`,
    `POST /api/v1/workorch/approvals/{approvalId}/reject`
  - slice compatibility endpoints are registered as native `magicEngine/http` routes:
    `GET /api/v1/workorch/slices`,
    `GET /api/v1/workorch/slices/snapshots`,
    `GET /api/v1/workorch/slices/statuses`,
    `GET /api/v1/workorch/slices/{sliceId}/snapshot`,
    `GET /api/v1/workorch/slices/{sliceId}/status`,
    `POST /api/v1/workorch/slices/{sliceId}/runs`
  - `internal/modules/application/workorchd/biz/bootstrap` now uses a `magicCommon/event.Hub` backed workorch event adapter by default
  - request-scoped `RuntimeSession` construction now reuses the application-injected EventHub when available
  - `internal/modules/application/workorchd/biz/bootstrap` now runs request-scoped runtime modules through a local magicCommon plugin manager lifecycle
- shared internal runtime packages: `internal/pkg/{common,events,repository}`
- kernel modules:
  - `internal/modules/kernel/workspace`
  - `internal/modules/kernel/run`
  - `internal/modules/kernel/decision`
  - `internal/modules/kernel/orchestrator`
- block modules:
  - `internal/modules/blocks/approval`
  - `internal/modules/blocks/legacy`

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

### Command

```bash
go run ./application/workorchd
```

Optional file-backed service run:

```bash
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

### Expected service behavior

HTTP endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces/{workspaceId}`
- `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`
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
- `POST /api/v1/workorch/runs/{runId}/start`
- `POST /api/v1/workorch/runs/{runId}/resume`
- all run execution entrypoints now share one in-process FIFO wait queue behind `WORKORCH_SERVER_MAX_CONCURRENT_RUNS`
- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/{ticketId}`
- `POST /api/v1/workorch/decisions/{ticketId}/resolve`
- `POST /api/v1/workorch/decisions/{ticketId}/reject`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`
- `GET /api/v1/workorch/slices`
- `GET /api/v1/workorch/slices/snapshots`
- `GET /api/v1/workorch/slices/statuses`
- `GET /api/v1/workorch/slices/{sliceId}/status`
- `GET /api/v1/workorch/slices/{sliceId}/snapshot`
- `POST /api/v1/workorch/slices/{sliceId}/runs`
- `POST /api/v1/workorch/runs`

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
curl -f http://127.0.0.1:8080/api/v1/workorch/slices
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
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/runs
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs
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
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/snapshots
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/statuses
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/status
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/snapshot
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
- the slice catalog endpoint returns the executable `workorch` slice definitions
- the single-slice run endpoint returns one requested `workorch` slice
- the bulk run endpoint returns 17 current `workorch` slices as JSON
- with the file-backed backend, the workspace list endpoint returns persisted `workorch` workspace instances
- with the file-backed backend, the single-workspace endpoint returns one persisted `workorch` workspace instance
- with the file-backed backend, the batch slice-snapshots endpoint returns all currently persisted `workorch` slice snapshots
- with the file-backed backend, the batch slice-statuses endpoint returns all currently persisted `workorch` slice summaries
- with the file-backed backend, the single-slice status endpoint returns one persisted `workorch` slice status summary when persisted state exists
- with the file-backed backend, the single-slice snapshot endpoint returns one persisted `workorch` slice snapshot when persisted state exists

Formal local verification:

```bash
make verify-service
make verify-service-file
```

`make verify-service-file` now includes a same-data-dir restart pass for persisted workspace / run / decision / approval reads.

### File-backed acceptance verification

Verified with:

```bash
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
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
- slice compatibility endpoints now use native `magicEngine/http` route registration
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
- richer block execution graph
- external tool/runtime adapters beyond minimal legacy adapter
- richer multi-run scheduling beyond the current in-process FIFO wait queue
- observability pipeline beyond current acceptance snapshots

当前已按 MVP 形式交付，后续如需增强应作为新需求处理：

- file-backed repository adapter as the first non-memory persistence path

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
- completed: native slice compatibility routes
- completed: native route registration guard for accepted API endpoints
- completed: guarded HTTP route registration so accepted API endpoints stay explicit native routes
- active todo: none
