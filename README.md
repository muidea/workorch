# workorch

workorch is a local-first workspace orchestration agent and service for defining Goals and DoD, activating execution Runs, observing runtime progress, and managing LLM / MCP / memory context through a TUI-first CLI and bundled web console.

**Production runtime path = Runtime V2 only.** Application constructs a typed `RuntimeBootstrap` and issues Start/Resume; `kernel/orchestrator` is the sole convergence authority (`DriveRuntimeV2` + reducer + sealed dispatch + evidence commit). Capability blocks only execute and return typed results; Result stores already-judged outcomes. Design contract: `docs/00-core/runtime-stage-contract.md`. Implementation SoT: `internal/modules/kernel/orchestrator/readme.md` and `internal/pkg/workorchruntimecontract`.

Current LLM provider management supports explicit provider capabilities for
chat completions, streaming, and declared non-chat operations. Streaming output
is recorded as observability evidence and non-chat operations remain
capability-gated rather than implicitly selected by the orchestrator.

Memory records support embedding metadata and vector readiness fields. Existing
workspace-scoped list/search remains deterministic, and repositories can rank
vector-backed Memory records by similarity when an embedding provider is
configured.

## Structure

- `application/workorch`: executable entry; default mode starts the TUI, and `--server` starts the HTTP/Web service
- `internal/services/workorch`: process-level framework service for startup mode selection, local/remote backend wiring, CLI, TUI, and agent session adapters
- `internal/initiators/workorchruntime`: repository-provider initiator for plugin-loaded owners; it does not export EventHub, runtime policy, or bootstrap facades
- `internal/modules/application/workorchd`: server-side application module (`biz` / `service` / `pkg`); constructs typed bootstrap, projections, HTTP/SSE — does **not** own Runtime V2 drive
- `web/workorch-console`: React/TypeScript management console source
- `web/consoleassets`: embedded management console build output served by `workorch --server`
- `internal/modules/kernel/{orchestrator,governance,system}`: kernel coordination modules
  - **orchestrator**: Runtime V2 convergence authority
  - **governance** / **system**: policy coordination and startup defaults
- `internal/modules/blocks/{workspace,run,result,decision,approval,memory,skill,soul,mcpregistry,eventstream,file,git,llm,mcp,shell,ssh,web}`: stateful owners + capability blocks
  - **run**: Run / Step / Runtime V2 aggregate persistence owner
  - **eventstream**: audit EventRecord / high-watermark owner
- `internal/pkg/{common,events,eventcontract,runtimeevent,repository,workorchclient,workorchport,workorchdeps,workorchrepo,workorchruntimecontract,workorchruntimepolicy,...}`: shared primitives and **Runtime V2 typed contracts**
- `docs/`: authoritative design docs (`docs/README.md` navigation; `docs/01-overview/current-state.md` fact entry)
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

Each Module/Block follows the go-multi-module-dev minimum shape:

- `module.go` / `plugin.go`: constructor and EventHub registration
- `biz/`: business logic (orchestrator includes `biz/runtimev2/`)
- `service/`: HTTP/CLI/session/protocol adapters only; owner-to-owner I/O remains typed EventHub commands/results
- `pkg/`: models, events, common constants
- `readme.md`: module interaction contract (implementation SoT for that module)

### Runtime V2 boundary (current)

- **Application** (`workorchd`): HTTP/CLI/SSE/projection; builds `RuntimeBootstrap`; Start/Resume only; no drive queue; Decision resolution does **not** re-issue Resume
- **Orchestrator**: sole `DriveRuntimeV2` owner — facts, authority transaction, prerequisite, candidate seal, sealed dispatch, operation evidence, governance wait, terminal projection
- **Run owner**: CAS-persists Runtime V2 aggregate; does not choose next action
- **Capability blocks** (llm/file/git/shell/ssh/web/mcp): execute typed operations or produce candidates; do not judge coverage / Result
- **Stateful blocks** (workspace/result/decision/approval/memory/…): own formal aggregates via EventHub
- **Eventstream**: audit `EventRecord` only — not a business recovery entry
- Cross-component I/O: EventHub + concrete command/data/result only (no cross-owner service/repo, no `map[string]any` bridge back into authority)
- V1 StagePacket / NextAction / DispatchNextStep / ExecutionContextPatch / auto replan successor: **removed** from production path

## Workorch semantics retained

- modules: `application/workorchd`, `kernel/orchestrator`, `kernel/governance`, `kernel/system`
- blocks (owners): `workspace`, `run`, `result`, `decision`, `approval`, `memory`, `skill`, `soul`, `mcpregistry`, `eventstream`
- blocks (capabilities): `llm`, `file`, `git`, `shell`, `ssh`, `web`, `mcp`
- shared contracts under `internal/pkg` (especially `workorchruntimecontract`)

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

`docs/00-core/runtime-stage-contract.md` is the authoritative design contract for future WorkOrch functionality work. Any change that affects WorkOrch runtime behavior, orchestration flow, LLM execution, memory / recall, soul / skill / tool / MCP capability selection, repair / replan, verification / acceptance, or profile mapping must be designed, implemented, and verified against that document.

If older docs, local closure plans, or feature-specific proposals conflict with the stage contract and neutrality design, the stage contract takes precedence. Feature-specific behavior must remain in profile / capability layers and must not be hard-coded into the core runtime.

Common entrypoints:

```bash
make deps
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

- `make deps` downloads Go modules and installs Web console dependencies before local builds
- Go module preparation uses `go mod download`; set `GO=/path/to/go` or `GO_MOD_DOWNLOAD_FLAGS=...` when a machine needs custom toolchain/proxy settings
- Web console dependency preparation uses `npm ci` when `web/workorch-console/package-lock.json` exists, so fresh checkouts and stale `node_modules` are rebuilt from the lockfile; set `NPM=/path/to/npm` or `NPM_CI_FLAGS=...` for local registry/proxy flags
- `make build` writes the binary to `./bin/workorch`
- `make console-build` rebuilds the embedded management console assets from `web/workorch-console`
- `make verify-service` and `make verify-service-file` verify the currently deployed `workorch --server` HTTP service at `WORKORCH_BASE_URL` without starting or stopping it
- `make verify-service-file` is kept as an alias for the same deployed-instance check
- `make run-file` runs the service with the default file-backed repository backend
- default service state is file-backed and persists under `repository.dataDir`
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

Each template has 6 Souls (planning, execution, decision, approval, validation, artifact). Users can customize templates and Souls after creation through the Web Console or API. See `docs/40-profiles-capabilities/builtin-templates-and-souls.md` for details.

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
mkdir -p .workorch-runtime
chmod 0777 .workorch-runtime
docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/config/container-file/application.toml:/etc/workorch/application.toml:ro" \
  -v "$(pwd)/.workorch-runtime:/var/lib/workorch" \
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
defaultDialogLanguage = ""

[repository]
backend = "file"
# repository.dataDir is WorkOrch internal state. For file-backed containers,
# config/container-file/application.toml uses /var/lib/workorch/.workorch-data
# so default deliverables can live beside it under /var/lib/workorch/.workorch-workspaces.
dataDir = "/var/lib/workorch/.workorch-data"

[runtime]
defaultSoulId = ""
maxRuntimeRepairAttempts = 10
maxProviderRetryAttempts = 10
shellCommandTimeout = "60s"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = ""
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[governance.autoTicket]
enabled = true
minConfidence = 0.82
maxPerRun = 5

[governance.autoApproval]
enabled = true
minConfidence = 0.9
maxPerRun = 3
allowReadOnly = true
allowWorkspaceWrite = true
requireHumanFor = ["secret_access", "external_write", "destructive_operation", "production_operation"]

[governance.budget]
maxConsecutiveFailures = 2
maxRepeatedSamePrompt = 1
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

The workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `deliverableDirectory` to `.workorch-workspaces/<generated-workspace-id>` for the file backend when the client omits it. Runtime state is stored under `repository.dataDir/internal/workspaces/<workspace-id>`. The workspace snapshot endpoint aggregates one workspace together with its related runs and blocking items. The run create, list, single-run, snapshot, status, and observability endpoints provide formal `workorch` run instance management. The run start endpoint synchronously advances a created `run` and returns the updated state, which may already be `completed`, `failed`, `blocked_on_decision`, or `blocked_on_approval`. The run resume endpoint returns `409 conflict` with `blocked` when the required decision or approval has not yet been resolved. The decision and approval endpoints provide formal query and processing paths for blocked runs. All run execution entrypoints share one in-process priority-aware wait queue behind `server.maxConcurrentRuns`. If `[llm].defaultEmbedding` is configured with an explicit provider/model target that supports the `embeddings` operation, workspace execution activation uses `/embeddings` for vector Memory recall and records `memoryRetrieval` evidence on the Run execution context. When defaultEmbedding is unset or embedding fails, activation falls back to workspace memory list baseline selection.

Container-oriented deployment details are in `docs/50-product-interface/deployment.md`, and `config/application.toml` provides the baseline runtime configuration template.

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
make deps
make fmt
make vet
make test
make build
make ci
```

## Docs

正式导航入口：

- `docs/README.md`：文档地图与阅读顺序
- `docs/01-overview/current-state.md`：当前生产事实与权威分层
- `docs/00-core/runtime-stage-contract.md`：中立 runtime 合约（设计 SoT）
- `docs/70-roadmap/active/overview.md`：仅真实 residual 未关项
- `docs/70-roadmap/candidates/next-stage-demand-candidates.md`：延期产品候选
- `docs/90-archive/completed/overview.md`：已完成过程计划（历史检索，不约束新实现）

实现 SoT：`internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract`。
