# workorch

workorch minimal skeleton, rectified to the go-multi-module-dev module-oriented layout.

## Structure

- `application/workorchd`: executable entry
- `internal/initiators/workorchruntime`: workorch runtime initiator that provides repository, event hub, and cross-module service wiring to plugin-loaded modules
- `internal/modules/application/workorchd`: workorchd plugin module plus `biz` / `service` / `pkg` implementation packages
- `web/workorch-console`: React/TypeScript management console source, built into `workorchd` assets
- `internal/modules/application/workorchd/service/server/consoleassets`: embedded management console build output served by `workorchd`
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: workorch kernel modules
- `internal/modules/blocks/{approval,legacy}`: reusable/block modules
- `internal/pkg/{common,events,repository}`: shared internal primitives
- `docs/`: repo docs
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

Each module is now normalized around the go-multi-module-dev minimum shape:

- `module.go`: module-level constructor/export entry
- `biz/`: business logic
- `service/`: module-facing contract layer or adapter-facing surface
- `pkg/common`: module-local constants
- `pkg/models`: module-local models when the module owns domain data

The application layer is also split by responsibility:

- `application/workorchd/main.go`: thin process entry that starts the magicCommon default service and explicitly imports the selected `workorchruntime` initiator plus the required `kernel` and `blocks` modules
- `internal/modules/application/workorchd`: plugin module registration plus `Setup` / `Run` / `Teardown` bridge for the workorchd HTTP service
- `internal/initiators/workorchruntime`: application runtime dependency provider used by plugin-loaded modules
- `internal/modules/application/workorchd/biz/bootstrap`: backend assembly, repository wiring, and runtime construction
- `internal/modules/application/workorchd/biz/appservice`: application-level business orchestration, workorch workflow assembly, env-backed execution entry, and workorch runtime adapter
- `internal/modules/application/workorchd/pkg/observed`: workorch observed-envelope adapter for acceptance snapshots
- `internal/modules/application/workorchd/biz/demo`: compatibility export entry that forwards to `appservice`
- `internal/modules/application/workorchd/pkg/presenter`: observable output rendering

Inside `demo/`, responsibilities are now intentionally thin:

- `demo.go`: compatibility shim that delegates to `appservice.RunAllWorkorchFromEnv(...)` / `appservice.RunAllWorkorch(...)`

Current boundary:

- `bootstrap.Runtime`: assembled capability export surface via `Exports()` plus lifecycle shutdown
- `bootstrap.Runtime`: now stores a single assembled `Exports` bundle plus module lifecycle teardown, rather than scattering repo/service fields internally
- `bootstrap.Exports`: now groups assembled capabilities into `Repositories`, `Services`, and `Infrastructure`
- `appservice.Runtime`: workorch application runtime contract
- `appservice/runtime_adapter.go`: bridges grouped `bootstrap.Exports` into the workorch runtime contract through grouped runtime dependencies
- `appservice`: now keeps workspace write flows behind `workspace` module service instead of direct repository updates
- `appservice`: now also uses explicit read-side vs action-side runtime faces inside workorch orchestration
- `appservice`: workorch action-side is now further grouped by workspace/run/decision/approval business actions
- `appservice`: workorch read-side is now further grouped by guard checks vs snapshot assembly
- `appservice/runtime_adapter.go`: now also groups its own dependencies into read-side and action-side capabilities instead of flat repo/service fields
- `appservice/runtime_dependencies.go`: now centralizes `bootstrap.Exports` to grouped runtime dependency assembly in one place
- `appservice/runtime_assembly.go`: now centralizes managed runtime adapter plus collector assembly in one place
- `appservice/runtime_session.go`: now centralizes repeated bootstrap-plus-managed-runtime lifecycle handling in one place
- `appservice/entry.go`: now centralizes repeated workorch execution through `WithSession` and `WithBackend` helpers, with `demo` forwarding to the same path
- `appservice/demo` tests: now centralize repeated session setup and backend run patterns through shared test helpers
- `observed/`: owns workorch acceptance `observed` envelope assembly
- `bootstrap.Exports.Infrastructure`: now carries the application `EventHub`
- `internal/pkg/events/workorch_payload.go`: now centralizes reusable workorch event payload keys and builders
- `internal/pkg/events/workorch_event.go`: now centralizes typed workorch event envelope decoding, normalization, topic listing, and observer subscription helpers
- `internal/pkg/events/workorch_version.go`: now centralizes strict workorch event/snapshot version constants and topic-scoped contract lookup
- `observed/workorch_contract.go`: now decodes and validates published workorch payloads before turning them into acceptance snapshots
- `observed/snapshot_contract.go`: now centralizes typed workorch snapshot items while `presenter` handles the final JSON-compatible rendering shape
- `workspace / run / decision / approval`: now publish formal business events into the shared `EventHub`, and `run` now has both `created` and `state changed` events
- `orchestrator`: now also publishes `workspace` runtime state changes when `activeRunId` is bound or cleared
- `observed/collector.go`: now subscribes to workorch event topics and collects observed input without relying on `MemoryHub.Published()`
- `appservice/runtime_managed.go`: now owns explicit collector lifecycle via `ManagedRuntime`
- `appservice/runtime_adapter.go`: now acts as a pure business runtime adapter without collector ownership
- `observed/`: now prefers subscribed/published `workspace / run / step / decision / approval` events when available, treats `observed` as an event-normalization layer rather than a business state source, and falls back to state-derived envelopes only when a matching formal event is absent
- `presenter/`: owns snapshot view mapping and output only

## Workorch semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`
- blocks: `approval`, `legacy`
- shared common/events/repository primitives moved under `internal/pkg`

## Development workflow

This repository is still a single `go.mod` codebase. Here, `go-multi-module-dev` means module-oriented directory organization and dependency boundaries inside one repository, not a split into multiple Go modules yet.

Common entrypoints:

```bash
make fmt
make fmt-check
make vet
make test
make test-race
make console-build
make build
make run
make run-file
make verify-service
make verify-service-file
make ci
```

Notes:

- `make build` writes the binary to `./bin/workorchd`
- `make console-build` rebuilds the embedded management console assets from `web/workorch-console`
- `make verify-service` and `make verify-service-file` are the formal local acceptance checks for the HTTP service
- `make verify-service-file` includes file-backed restart-read verification for persisted workspace / run / decision / approval state
- `make run-file` runs the service with the file-backed repository backend
- default memory-backed service state is shared within the running `workorchd` process; use `make run-file` for restart persistence
- file-backed service execution is repeatable against the same data directory for the shipped workorch IDs
- `make ci` is the local equivalent of the GitHub Actions baseline

## Workorch Service

Start the service:

```bash
make run
# or
go run ./application/workorchd
```

Optional file-backed service run:

```bash
make run-file
# or
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

Container build and run:

```bash
docker build -t workorchd:local .
docker run --rm -p 8080:8080 workorchd:local
```

Optional file-backed container run:

```bash
mkdir -p .workorch-data
chmod 0777 .workorch-data
docker run --rm \
  -p 8080:8080 \
  -e WORKORCH_REPOSITORY_BACKEND=file \
  -e WORKORCH_DATA_DIR=/var/lib/workorch \
  -v "$(pwd)/.workorch-data:/var/lib/workorch" \
  workorchd:local
```

The mounted data directory must be writable by the non-root `workorch` user inside the container.

Default listen address:

```bash
WORKORCH_SERVER_ADDR=:8080
```

Optional service runtime configuration:

```bash
WORKORCH_SERVER_READ_HEADER_TIMEOUT=5s
WORKORCH_SERVER_READ_TIMEOUT=15s
WORKORCH_SERVER_WRITE_TIMEOUT=30s
WORKORCH_SERVER_IDLE_TIMEOUT=60s
WORKORCH_SERVER_SHUTDOWN_TIMEOUT=5s
WORKORCH_SERVER_MAX_CONCURRENT_RUNS=1
```

When the concurrent run limit is reached, execution requests enter an in-process FIFO wait queue and continue after a slot becomes available. If the caller cancels the request while waiting, the service returns `408 Request Timeout`.

Available endpoints:

- `GET /console`
- `GET /console/`
- `GET /console/*`
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

Minimal deployment verification:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/metrics
WORKSPACE_JSON="$(curl -sS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d '{"title":"demo workspace","description":"demo workspace description"}')"
WORKSPACE_ID="$(printf '%s\n' "${WORKSPACE_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
test -n "${WORKSPACE_ID}"
curl http://127.0.0.1:8080/api/v1/workorch/slices
curl -X POST http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/runs
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs
```

Persisted-state verification for the file-backed backend:

```bash
RUN_ID="run-demo-$(date +%s)"
curl http://127.0.0.1:8080/api/v1/workorch/workspaces
curl http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}
curl http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/snapshot
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"goal-demo\",\"doDId\":\"dod-demo\"}"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\"}"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/start
curl http://127.0.0.1:8080/api/v1/workorch/decisions
curl "http://127.0.0.1:8080/api/v1/workorch/decisions?workspaceId=${WORKSPACE_ID}"
curl "http://127.0.0.1:8080/api/v1/workorch/decisions?runId=${BLOCKED_RUN_ID}"
TICKET_ID="ticket-run-runtime-decision-block-demo-runtime-decision"
curl http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}
curl -X POST http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}/resolve
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/resume
APPROVAL_RUN_ID="run-runtime-approval-block-demo"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\"}"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/start
curl http://127.0.0.1:8080/api/v1/workorch/approvals
curl "http://127.0.0.1:8080/api/v1/workorch/approvals?workspaceId=${WORKSPACE_ID}"
curl "http://127.0.0.1:8080/api/v1/workorch/approvals?runId=${APPROVAL_RUN_ID}"
APPROVAL_ID="approval-run-runtime-approval-block-demo-runtime-approval"
curl http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}
curl -X POST http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}/approve
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/resume
curl http://127.0.0.1:8080/api/v1/workorch/runs
curl "http://127.0.0.1:8080/api/v1/workorch/runs?workspaceId=${WORKSPACE_ID}"
curl http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}
curl http://127.0.0.1:8080/api/v1/workorch/slices/snapshots
curl http://127.0.0.1:8080/api/v1/workorch/slices/statuses
curl http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/status
curl http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/snapshot
```

Deployment-oriented validation:

```bash
WORKORCH_SERVER_ADDR=:8080 \
WORKORCH_SERVER_MAX_CONCURRENT_RUNS=1 \
go run ./application/workorchd
```

Then verify:

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
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/runs
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs
```

For file-backed verification, also run:

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

The workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `workingDirectory` to `<service-working-directory>/workspaces/<generated-workspace-id>` when the client omits it. The workspace list and single-workspace read endpoints are persisted read endpoints, so they are intended for the file-backed repository backend rather than the stateless per-request memory backend. The workspace snapshot endpoint aggregates one workspace together with its related runs and blocking items. The run create, list, and single-run read endpoints provide formal `workorch` run instance management; because create depends on an existing workspace and the service creates a fresh runtime session per request, the persisted run list and read endpoints are also mainly intended for the file-backed repository backend. The run snapshot and run status endpoints provide formal `runId`-based read models without requiring `sliceId`. The run start endpoint synchronously advances a created `run` and returns the updated state, which may already be `completed`, `failed`, `blocked_on_decision`, or `blocked_on_approval`. The run resume endpoint returns `409 conflict` with `blocked` when the required decision or approval has not yet been resolved. The decision list and single-ticket endpoints provide formal query paths for decision gate instances, and the resolve/reject endpoints complete the external decision gate closure for blocked runs. The approval list and single-approval endpoints provide the same formal query and processing path for approval gates, and the approve/reject endpoints now complete the external approval gate closure for blocked runs. The slice catalog endpoint returns the current executable `workorch` slice definitions with stable IDs. The batch slice-snapshots endpoint returns all currently available persisted slice snapshots. The batch slice-statuses endpoint returns all currently available persisted slice summaries. The single-slice run endpoint executes one requested `workorch` business slice. The single-slice status and snapshot endpoints read persisted state for one requested `workorch` slice, so the persisted read endpoints are intended for the file-backed repository backend rather than the stateless per-request memory backend. The bulk run endpoint returns the current 17 `workorch` slices as JSON, including workspace/run/step/blocking snapshots and observed items. All run execution entrypoints now share one in-process FIFO wait queue behind `WORKORCH_SERVER_MAX_CONCURRENT_RUNS`, including formal `run start` and `run resume`.

Container-oriented deployment details are in `docs/deployment.md`, and `.env.workorchd.example` provides the baseline runtime configuration template.

Formal verification entrypoints:

```bash
make verify-service
make verify-service-file
make docker-verify
make docker-verify-file
```

`make verify-service-file` and `make docker-verify-file` both verify the full file-backed MVP closure, including service restart and persisted read-model continuity.

## Development targets

```bash
make fmt
make vet
make test
make build
make ci
```

## Docs

- `docs/structure.md`: layout normalization mapping
- `docs/workorch-status.md`: delivered scope, acceptance baseline, and current closure status
- `docs/mvp-backlog.md`: closed MVP scope, completed items, and closure criteria used
