# workorch

workorch is a local-first workspace orchestration agent and service for defining Goals and DoD, activating execution Runs, observing runtime progress, and managing LLM / MCP / memory context through a TUI-first CLI and bundled web console.

Current LLM provider management supports explicit provider capabilities for
chat completions, streaming, and declared non-chat operations. Streaming output
is recorded as Run/Step observability evidence and non-chat operations remain
capability-gated rather than implicitly selected by the orchestrator.

Memory records support embedding metadata and vector readiness fields. Existing
workspace-scoped list/search remains deterministic, and repositories can rank
vector-backed Memory records by similarity when an embedding provider is
configured.

## Structure

- `application/workorch`: executable entry; default mode starts the TUI, and `--server` starts the HTTP/Web service
- `internal/initiators/workorchruntime`: workorch runtime initiator that provides repository, event hub, and cross-module service wiring to plugin-loaded modules
- `internal/modules/application/workorchd`: server-side application module plus `biz` / `service` / `pkg` implementation packages
- `web/workorch-console`: React/TypeScript management console source, built into workorch server assets
- `internal/modules/application/workorchd/service/server/consoleassets`: embedded management console build output served by `workorch --server`
- `internal/modules/kernel/{workspace,run,decision,orchestrator,eventstream,mcp,memory,skill,soul}`: workorch kernel modules
- `internal/modules/kernel/result`: formal Result / Artifact business outcome models
- `internal/modules/blocks/{approval,llm,mcp,shell,ssh}`: reusable/block modules
- `internal/pkg/{common,events,repository}`: shared internal primitives
- `docs/`: repo docs
- `docs/next-stage-result-event-development-plan.md`: completed formal Result / Artifact and persistent Workorch event stream scope
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

Each module is now normalized around the go-multi-module-dev minimum shape:

- `module.go`: module-level constructor/export entry
- `biz/`: business logic
- `service/`: module-facing contract layer or adapter-facing surface
- `pkg/common`: module-local constants
- `pkg/models`: module-local models when the module owns domain data

The application layer is also split by responsibility:

- `application/workorch/main.go`: thin process entry that starts the TUI by default, or starts the magicCommon service with `--server`; it explicitly imports the selected `workorchruntime` initiator plus the required `kernel` and `blocks` modules
- `internal/modules/application/workorchd`: plugin module registration plus `Setup` / `Run` / `Teardown` bridge for the workorch HTTP service
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
- `observed/collector.go`: now subscribes to workorch event topics, while `ManagedRuntime` can also read formal runtime EventHub history when available
- `appservice/runtime_managed.go`: now owns explicit collector lifecycle via `ManagedRuntime`
- `appservice/runtime_adapter.go`: now acts as a pure business runtime adapter without collector ownership
- `observed/`: normalizes subscribed/published `workspace / run / step / decision / approval` events into acceptance snapshots and does not synthesize snapshots from repository state when formal events are missing
- `presenter/`: owns snapshot view mapping and output only

## Workorch semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`, `eventstream`, `mcp`, `memory`, `skill`, `soul`
- models: `result`
- blocks: `approval`, `llm`, `mcp`, `shell`, `ssh`
- shared common/events/repository primitives moved under `internal/pkg`

## Quick Install

Install the latest release from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/muidea/workorch/main/scripts/install-latest.sh | bash
```

Or with custom options:

```bash
export WORKORCH_VERSION=v0.1.0
export WORKORCH_INSTALL_DIR=/usr/local/bin
curl -fsSL https://raw.githubusercontent.com/muidea/workorch/main/scripts/install-latest.sh | bash
```

The script auto-detects OS and architecture (linux/darwin, amd64/arm64) and downloads the matching binary.

## Development workflow

This repository is still a single `go.mod` codebase. Here, `go-multi-module-dev` means module-oriented directory organization and dependency boundaries inside one repository, not a split into multiple Go modules yet.

### Core design contract

`docs/workorch-runtime-stage-contract-neutrality-design.md` is the authoritative design contract for future WorkOrch functionality work. Any change that affects WorkOrch runtime behavior, orchestration flow, LLM execution, memory / recall, soul / skill / tool / MCP capability selection, repair / replan, verification / acceptance, or profile mapping must be designed, implemented, and verified against that document.

If older docs, local closure plans, or feature-specific proposals conflict with the stage contract and neutrality design, the stage contract takes precedence. Feature-specific behavior must remain in profile / capability layers and must not be hard-coded into the core runtime.

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

- `make build` writes the binary to `./bin/workorch`
- `make console-build` rebuilds the embedded management console assets from `web/workorch-console`
- `make verify-service` and `make verify-service-file` verify the currently deployed `workorch --server` HTTP service at `WORKORCH_BASE_URL` without starting or stopping it
- `make verify-service-file` is kept as an alias for the same deployed-instance check
- `make run-file` runs the service with the file-backed repository backend
- default memory-backed service state is shared within the running `workorch --server` process; use `make run-file` for restart persistence
- file-backed service execution is repeatable against the same data directory for the shipped workorch IDs
- `make ci` is the local equivalent of the GitHub Actions baseline

## Release

Build cross-platform release binaries:

```bash
make release
```

This produces `dist/workorch-{tag}-{os}-{arch}.tar.gz` for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, plus a `checksums.txt`.

Run full pre-release checks before building:

```bash
make release-check
```

Release version is derived from `git describe --tags --always`. Override with `RELEASE_TAG`:

```bash
make release RELEASE_TAG=v0.2.0
```

Manual install from a local release build:

```bash
tar -xzf dist/workorch-*-linux-amd64.tar.gz -C /usr/local/bin/
```

## Built-in Templates and Souls

workorch ships with 6 built-in workspace templates and 36 per-stage Souls, loaded automatically on first startup:

| Template | Description |
|----------|-------------|
| `generic-governed` | Full decision + approval governance, unrestricted tools |
| `weather-query` | MCP weather tool query with decision + approval gates |
| `software-dev` | Software development: git, build, test, lint, diff |
| `code-review` | Read-only code analysis with security/best-practice checks |
| `doc-generator` | Technical documentation generation from source code |
| `data-analysis` | Data query, Python/R scripting, statistical modeling |

Each template has 6 Souls (planning, execution, decision, approval, validation, artifact). Users can customize templates and Souls after creation through the Web Console or API. See `docs/builtin-templates-and-souls-design.md` for details.

## Workorch Service

Start the service:

```bash
make run
# or
go run ./application/workorch --server -config-dir config
```

Optional file-backed service run:

```bash
make run-file
# or
go run ./application/workorch --server -config-dir config/file
```

Container build and run:

```bash
docker build -t workorch:local .
docker run --rm -p 8080:8080 workorch:local
```

Optional file-backed container run:

```bash
mkdir -p .workorch-data
chmod 0777 .workorch-data
docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/config/container-file/application.toml:/etc/workorch/application.toml:ro" \
  -v "$(pwd)/.workorch-data:/var/lib/workorch" \
  workorch:local
```

The mounted data directory must be writable by the non-root `workorch` user inside the container.

Runtime configuration is loaded from TOML:

```toml
[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
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
- `POST /api/v1/workorch/decisions/resolve-batch` with `ticketIds` and one shared `resolutionType`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`
- `POST /api/v1/workorch/approvals/resolve-batch` with `approvalIds` and one shared `action`: `approve` or `reject`

The Web Console Configuration page can update LLM providers. The service writes
the change to `application.toml`, reloads magicCommon framework configuration,
and the next LLM runtime step resolves providers from the latest config without
restarting `workorch --server`. Read responses redact provider API keys.

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
go run ./application/workorch --server -config-dir config
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

The workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `workingDirectory` to `repository.dataDir/workspaces/<generated-workspace-id>/working` for the file backend when the client omits it. The workspace snapshot endpoint aggregates one workspace together with its related runs and blocking items. The run create, list, single-run, snapshot, status, and observability endpoints provide formal `workorch` run instance management. The run start endpoint synchronously advances a created `run` and returns the updated state, which may already be `completed`, `failed`, `blocked_on_decision`, or `blocked_on_approval`. The run resume endpoint returns `409 conflict` with `blocked` when the required decision or approval has not yet been resolved. The decision and approval endpoints provide formal query and processing paths for blocked runs. All run execution entrypoints share one in-process priority-aware wait queue behind `server.maxConcurrentRuns`. If `[llm].defaultEmbeddingProvider` references a provider declaring `nonChatOperations = ["embeddings"]`, workspace execution activation uses `/embeddings` for vector Memory recall and records `memoryRetrieval` evidence on the Run execution context.

Container-oriented deployment details are in `docs/deployment.md`, and `config/application.toml` provides the baseline runtime configuration template.

Formal verification entrypoints:

```bash
make verify-service
make verify-service-file
make verify-workspace-template-soul
make verify-workspace-template-soul-file
make verify-llm
make verify-llm-file
make verify-mcp-stdio
make verify-mcp-stdio-file
make verify-ci-mvp
make verify-ci-mvp-file
make verify-mvp
make verify-mvp-file
make verify-real-llm
make verify-real-mcp-stdio
make verify-real-vector-memory
make verify-real-weather
make verify-weather-ticket-approval
make verify-weather-ticket-approval-mock
make verify-real-mvp
make docker-verify
make docker-verify-file
```

All `verify-*` e2e scripts target the currently deployed `workorch --server` instance through `WORKORCH_BASE_URL` and do not build, start, stop, or restart the service. `make verify-workspace-template-soul` covers deployed Soul management, Workspace Template management, and template-based Workspace setup with Soul bindings. Local MVP acceptance uses the configured real LLM provider: `make verify-mvp` runs service smoke, real LLM execution, real LLM weather query, and real LLM weather governance checks against that deployed instance; `make verify-mvp-file` is an alias for the same check. CI uses only mock-provider checks through `make verify-ci-mvp` / `make verify-ci-mvp-file` to avoid external LLM credentials and provider/network false negatives. `make verify-real-vector-memory` targets an already running `workorch --server`, temporarily registers an embedding-capable mock provider, verifies vector Memory recall evidence, and restores the original LLM configuration. `make verify-real-weather` uses the configured real LLM provider plus a managed local MCP `stdio` weather endpoint to query Zhejiang Hangzhou current weather from Open-Meteo and validate the Run snapshot evidence. `make verify-weather-ticket-approval` uses the configured real LLM provider plus a managed local MCP `stdio` weather endpoint to verify a specified city weather query through Decision Ticket and Approval gates. The default managed endpoint root is `.workorch-data/system/mcp-endpoints`, and it can be overridden with `WORKORCH_MCP_ENDPOINT_DIR`. `make verify-weather-ticket-approval-mock` is the deterministic mock LLM variant reserved for CI-style checks. `make verify-real-mvp` targets an already running `workorch --server`; it combines the configured real LLM check, deployed MCP `stdio` runtime check, and deployed vector Memory recall check.

Governed workspace concurrency load tests also target the currently deployed instance. `make loadtest-governed` uses a deterministic mock LLM to measure workorch scheduling, Ticket / Approval, MCP, evidence, and file-backed read-model capacity. The current MVP file backend acceptance baseline is `concurrency=10,total=50` for quick regression and `concurrency=20,total=100` for high-pressure acceptance; `concurrency>=30` is no longer a regular acceptance target because the latest `c30/t150` run kept 100% correctness but had severe throughput and tail-latency regression. `make loadtest-governed-real` uses the configured real LLM provider for small-scale end-to-end pressure checks; do not run the real target in CI or treat it as the internal capacity baseline. Capacity runs can set `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS` so the controller fails fast when the deployed `server.maxConcurrentRuns` is lower than the intended parallel execution slot count. Load-test MCP helper endpoints also use `WORKORCH_MCP_ENDPOINT_DIR` so persisted MCP server definitions do not point at deleted `/tmp` files.

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
- `docs/mcp-capability-management-design.md`: MCP tool/resource/prompt capability recognition, management, binding, and evidence design
- `docs/workspace-concurrency-loadtest-plan.md`: workspace concurrency load test scope and completed pressure-test findings for mock and real LLM governed execution
- `docs/run-closure-quality-improvement-plan.md`: active quality-improvement tracker for stable Run closure, strict DoD acceptance, recovery, efficiency, task decomposition, and Result / Artifact summary quality
- `docs/next-stage-p1-development-plan.md`: completed P1 runtime context, provider capability, vector Memory, and lifecycle scope
- `docs/next-stage-demand-candidates.md`: future demand candidates that are not active MVP leftovers
