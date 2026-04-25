# workorch Status

## Current repository state

- module: `github.com/muidea/workorch`
- executable entry: `application/workorchd`
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
- `application/workorchd/observed/collector.go` subscribes only to the formal workorch topic set and stores normalized event input
- `application/workorchd/observed/workorch_contract.go` maps published business events to `/workspace|run|step|decision|approval/snapshot` items
- `application/workorchd/observed/workorch.go` only falls back to state-derived snapshot items when the current subject has no matching published event

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
- `POST /api/v1/workorch/workspaces/{workspaceId}/runs`
- `GET /api/v1/workorch/runs`
- `GET /api/v1/workorch/runs/{runId}`
- `GET /api/v1/workorch/runs/{runId}/snapshot`
- `GET /api/v1/workorch/runs/{runId}/status`
- `POST /api/v1/workorch/runs/{runId}/start`
- `POST /api/v1/workorch/runs/{runId}/resume`
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
WORKSPACE_ID="ws-demo-$(date +%s)"
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8080/readyz
curl -f http://127.0.0.1:8080/metrics | grep workorch_http_requests_total
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d "{\"workspaceId\":\"${WORKSPACE_ID}\",\"title\":\"demo workspace\"}"
curl -f http://127.0.0.1:8080/api/v1/workorch/slices
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
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"goal-demo\",\"doDId\":\"dod-demo\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\"}"
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
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\"}"
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

- the workspace create endpoint creates one formal `workorch` workspace instance
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
- repeatable file-backed demo execution against the same data directory
- file-backed terminal read models remain stable across reloads
- file-backed `workorchd` restart preserves terminal run, decision, approval, and workspace snapshot reads
- corrupt file-backed persisted JSON boundary returns explicit errors
- workorch formal business event contract now covers `workspace / run / step / decision / approval`
- run-created and enriched run/workspace event payloads are covered by `events` / `run` / `observed` tests

## Explicit non-goals of current workorch

Not implemented yet:

- real event bus integration
- richer block execution graph
- external tool/runtime adapters beyond minimal legacy adapter
- multi-run scheduling / queue semantics beyond the current max-concurrency guard
- observability pipeline beyond demo snapshots

Implemented in minimal form, but not yet expanded to fuller persistence semantics:

- file-backed repository adapter as the first non-memory persistence path

## MVP backlog

当前遗留目标已经整理到正式待办清单：

- `docs/mvp-backlog.md`

后续推进以这份 backlog 为准，按实例接口、阻塞处理闭环、仓储契约测试、持久化语义和事件总线边界逐项收口。

当前进度：

- `B1 workspace 实例接口` 已完成
- `B2 run 实例接口` 已完成
- `B3 run 启动与恢复接口` 已完成
- `B4 decision 工单查询与处理接口` 已完成
- `B5 approval 工单查询与处理接口` 已完成
- `B6 实例读模型补齐` 已完成
- `S1 仓储后端共享契约测试` 已完成
- 下一项：`S2 file 后端持久化语义补强`
