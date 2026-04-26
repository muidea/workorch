# workorch

workorch is a local-first workspace orchestration service for defining Goals and DoD, activating execution Runs, observing runtime progress, and managing LLM / MCP / memory context through a bundled web console.

## Structure

- `application/workorchd`: executable entry
- `internal/initiators/workorchruntime`: workorch runtime initiator that provides repository, event hub, and cross-module service wiring to plugin-loaded modules
- `internal/modules/application/workorchd`: workorchd plugin module plus `biz` / `service` / `pkg` implementation packages
- `web/workorch-console`: React/TypeScript management console source, built into `workorchd` assets
- `internal/modules/application/workorchd/service/server/consoleassets`: embedded management console build output served by `workorchd`
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: workorch kernel modules
- `internal/modules/blocks/{approval,legacy,llm,mcp,shell}`: reusable/block modules
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
- `internal/modules/application/workorchd/biz/appservice`: application-level business orchestration, service read/write flows, runtime activation, and runtime adapter
- `internal/modules/application/workorchd/pkg/observed`: workorch observed-envelope adapter for acceptance snapshots
- `internal/modules/application/workorchd/pkg/presenter`: observable output rendering

Current boundary:

- `bootstrap.Runtime`: assembled capability export surface via `Exports()` plus lifecycle shutdown
- `bootstrap.Runtime`: now stores a single assembled `Exports` bundle plus module lifecycle teardown, rather than scattering repo/service fields internally
- `bootstrap.Exports`: now groups assembled capabilities into `Repositories`, `Services`, and `Infrastructure`
- `appservice.Runtime`: workorch application runtime contract
- `appservice/runtime_adapter.go`: bridges grouped `bootstrap.Exports` into the workorch runtime contract through grouped runtime dependencies
- `appservice`: now keeps workspace write flows behind `workspace` module service instead of direct repository updates
- `appservice`: uses explicit read-side vs action-side runtime faces inside workorch orchestration
- `appservice`: action-side runtime ports are grouped by workspace/run/decision/approval business actions
- `appservice`: workorch read-side is now further grouped by guard checks vs snapshot assembly
- `appservice/runtime_adapter.go`: now also groups its own dependencies into read-side and action-side capabilities instead of flat repo/service fields
- `appservice/runtime_dependencies.go`: now centralizes `bootstrap.Exports` to grouped runtime dependency assembly in one place
- `appservice/runtime_assembly.go`: now centralizes managed runtime adapter plus collector assembly in one place
- `appservice/runtime_session.go`: now centralizes repeated bootstrap-plus-managed-runtime lifecycle handling in one place
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
go run ./application/workorchd -config-dir config
```

Optional file-backed service run:

```bash
make run-file
# or
go run ./application/workorchd -config-dir config/file
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
  -v "$(pwd)/config/container-file/application.toml:/etc/workorch/application.toml:ro" \
  -v "$(pwd)/.workorch-data:/var/lib/workorch" \
  workorchd:local
```

The mounted data directory must be writable by the non-root `workorch` user inside the container.

Runtime configuration is loaded from TOML:

```toml
[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "memory"
dataDir = "/var/lib/workorch"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = ""
baseURL = ""
apiKey = ""
model = ""
timeout = "60s"
disabled = false
```

When the concurrent run limit is reached, execution requests enter an in-process priority-aware wait queue and continue after a slot becomes available. Equal-priority requests keep FIFO order. If the caller cancels the request while waiting, the service returns `408 Request Timeout`.

Available endpoints:

- `GET /console`
- `GET /console/`
- `GET /console/*`
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
- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/{ticketId}`
- `POST /api/v1/workorch/decisions/{ticketId}/resolve` with `resolutionType`: `provide_missing_info`, `reject_current_path`, or `replan`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`

The Web Console Configuration page can update LLM providers. The service writes
the change to `application.toml`, reloads magicCommon framework configuration,
and the next LLM runtime step resolves providers from the latest config without
restarting `workorchd`. Read responses redact provider API keys.

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
GOAL_JSON="$(curl -sS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/goals \
  -H 'Content-Type: application/json' \
  -d '{"text":"deliver local MVP orchestration"}')"
GOAL_ID="$(printf '%s\n' "${GOAL_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
DOD_JSON="$(curl -sS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/dods \
  -H 'Content-Type: application/json' \
  -d '{"items":["workspace created","run completed"]}')"
DOD_ID="$(printf '%s\n' "${DOD_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d '{"runId":"run-local-verify","goalId":"'"${GOAL_ID}"'","doDId":"'"${DOD_ID}"'"}'
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/run-local-verify/start
```

Persisted-state verification for the file-backed backend:

```bash
RUN_ID="run-demo-$(date +%s)"
curl http://127.0.0.1:8080/api/v1/workorch/workspaces
curl http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}
curl http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/snapshot
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
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
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
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
```

Deployment-oriented validation:

```bash
go run ./application/workorchd -config-dir config
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
```

For file-backed verification, also run:

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

The workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `workingDirectory` to `<service-working-directory>/workspaces/<generated-workspace-id>` when the client omits it. The workspace snapshot endpoint aggregates one workspace together with its related runs and blocking items. The run create, list, single-run, snapshot, status, and observability endpoints provide formal `workorch` run instance management. The run start endpoint synchronously advances a created `run` and returns the updated state, which may already be `completed`, `failed`, `blocked_on_decision`, or `blocked_on_approval`. The run resume endpoint returns `409 conflict` with `blocked` when the required decision or approval has not yet been resolved. The decision and approval endpoints provide formal query and processing paths for blocked runs. All run execution entrypoints share one in-process priority-aware wait queue behind `server.maxConcurrentRuns`.

Container-oriented deployment details are in `docs/deployment.md`, and `config/application.toml` provides the baseline runtime configuration template.

Formal verification entrypoints:

```bash
make verify-service
make verify-service-file
make verify-llm
make verify-llm-file
make verify-mcp-stdio
make verify-mcp-stdio-file
make verify-mvp
make verify-mvp-file
make verify-real-llm
make verify-real-mcp-stdio
make verify-real-mvp
make docker-verify
make docker-verify-file
```

`make verify-mvp` runs the local memory-backed MVP verification bundle. `make verify-mvp-file` runs the file-backed bundle with restart-read continuity. `make verify-real-mvp` targets an already running `workorchd`; it combines the configured real LLM check and the deployed MCP `stdio` runtime check.

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
- `docs/next-mvp-acceptance.md`: next MVP scope aligned with the WorkOrch product planning baseline
- `docs/runtime-expansion-plan.md`: completed runtime adapter, shell, patch, graph, and observability scope
- `docs/advanced-runtime-scheduling-plan.md`: completed advanced MCP and priority scheduling scope
