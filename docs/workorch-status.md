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
  - configuration / workspace / run read endpoints are registered as native `magicEngine/http` routes:
    `GET /api/v1/workorch/configuration`,
    `GET /api/v1/workorch/workspaces`,
    `GET /api/v1/workorch/workspaces/{workspaceId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/task-progress`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/task-decompositions`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/context-bundle`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/goals`,
    `GET /api/v1/workorch/goals/{goalId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/dods`,
    `GET /api/v1/workorch/dods/{dodId}`,
    `GET /api/v1/workorch/workspaces/{workspaceId}/runs` for explicit `405 method not allowed`,
    `GET /api/v1/workorch/runs`,
    `GET /api/v1/workorch/runs/{runId}`,
    `GET /api/v1/workorch/runs/{runId}/snapshot`,
    `GET /api/v1/workorch/runs/{runId}/status`,
    `GET /api/v1/workorch/runs/{runId}/observability`,
    `GET /api/v1/workorch/runs/{runId}/task-progress`,
    `GET /api/v1/workorch/runs/{runId}/task-decompositions`,
    `GET /api/v1/workorch/runs/{runId}/context-bundle`,
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
    `PUT /api/v1/workorch/mcp/servers/{serverId}`,
    `GET /api/v1/workorch/mcp/servers/{serverId}/tools`,
    `GET /api/v1/workorch/mcp/servers/{serverId}/resources`,
    `GET /api/v1/workorch/mcp/servers/{serverId}/prompts`,
    `GET /api/v1/workorch/memory?workspaceId={workspaceId}`
  - configuration / workspace / run / decision / approval write and execution endpoints are registered as native `magicEngine/http` routes:
    `PUT /api/v1/workorch/configuration/llm`,
    `POST /api/v1/workorch/workspaces`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/goals`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/dods`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/memory`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/runs`,
    `POST /api/v1/workorch/workspaces/{workspaceId}/execution-activations`,
    `POST /api/v1/workorch/runs/{runId}/start`,
    `POST /api/v1/workorch/runs/{runId}/resume`,
    `POST /api/v1/workorch/decisions/{ticketId}/resolve`,
    `POST /api/v1/workorch/decisions/resolve-batch`,
    `POST /api/v1/workorch/approvals/{approvalId}/approve`,
    `POST /api/v1/workorch/approvals/{approvalId}/reject`,
    `POST /api/v1/workorch/approvals/resolve-batch`,
    `POST /api/v1/workorch/souls`,
    `PUT /api/v1/workorch/souls/{soulId}`,
    `POST /api/v1/workorch/skills`,
    `POST /api/v1/workorch/mcp/servers`,
    `PUT /api/v1/workorch/mcp/servers/{serverId}`
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
  - `internal/modules/blocks/llm`
  - `internal/modules/blocks/mcp`
  - `internal/modules/blocks/shell`
  - `internal/modules/blocks/ssh`
- Workspace execution activation now creates business Runs whose first Step is
  `llm/workspace_execution`; no alternate runtime fallback is registered.
- Workspace execution activation accepts and persists selected Soul, Skill, MCP
  server, and Memory context on Run execution context; the same context is
  passed to the first LLM Step input.
- Missing or disabled LLM provider configuration fails the LLM Step and Run
  with observable runtime evidence instead of silently completing through an
  alternate runtime path.
- Ready LLM provider configuration now creates a real OpenAI-compatible
  Chat Completions HTTP client inside `blocks/llm`; `openai` defaults to
  `https://api.openai.com/v1`, while `compatible` requires
  `[llm.providers.<id>].baseURL`.
- LLM provider configuration now declares formal provider capabilities:
  `chatCompletions`, `streaming`, and `nonChatOperations`. Web Console can edit
  these fields, configuration reads expose them, and runtime selection will not
  implicitly use undeclared non-chat operations.
- OpenAI-compatible LLM calls use JSON object mode and a strict
  `workorch.llmExecutionDecision.v1` contract; provider formatting drift is
  normalized only for markdown fences and first JSON object extraction.
- Streaming-capable LLM providers are invoked only when the selected provider
  explicitly declares `streaming = true`; streamed deltas are persisted into
  Step output as `streamEvents`, and stream failures before a valid final
  decision fail the Step with an explicit `llm_provider_stream_error` stage.
- Vector Memory recall is capability-gated through
  `[llm].defaultEmbeddingProvider` plus provider `nonChatOperations =
  ["embeddings"]`. When configured, workspace execution activation calls the
  OpenAI-compatible `/embeddings` operation, searches workspace Memory by vector
  similarity, and stores `memoryRetrieval` evidence on Run execution context.
  Missing or failing embedding providers degrade explicitly with a reason and
  do not silently select deterministic fallback Memory.
- Run observability timeline now includes Step output items and explicit
  `step_stream` entries for streamed provider deltas, so streaming evidence is
  visible through `/api/v1/workorch/runs/{runId}/observability`, not only the
  Run snapshot.
- LLM provider requests include a formal `runtimeCapabilities` catalog covering
  available runtime block kinds, declared read-only local service APIs, and
  shell policy. LLM decisions are rejected if shell commands or execution graph
  shell nodes call undeclared local `workorchd` endpoints such as `/runs`,
  `/observability`, or `/metrics?run=...`.
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
- `blocks/mcp` now registers `mcp/tool_call`, `mcp/resource_read`, and
  `mcp/prompt_get` as runtime adapter operations. It validates MCP server
  enablement plus `allowedTools`, `allowedResources`, and `allowedPrompts`,
  invokes HTTP/HTTPS endpoints or MCP `stdio` JSON-RPC operations, collects
  streaming evidence as `streamEvents`, and records output through the standard
  Step / ToolCall / ValidationResult evidence path. The `long_lived` session
  policy reuses initialized stdio sessions per MCP server; unsupported
  transports fail explicitly.
- LLM-driven Runs that bind a DoD now require a satisfied, evidence-backed
  `dodAssessment` before completion; missing or invalid assessments fail the
  terminal Step and Run.
- Runtime `memoryWrites` and terminal LLM run summaries now persist as
  Workspace Memory records with Run source refs; created Memory IDs are exposed
  on Step output as `memoryRefs` for later snapshots and recall.
- Memory records now include formal embedding metadata and vector readiness
  fields. Memory repositories preserve vector data, support vector similarity
  ordering through repository search, and API/Web responses expose
  `vectorReady` plus `vectorDimension` while preserving existing deterministic
  list/search behavior.
- Embedded Web Console can start LLM-driven Workspace execution with
  Goal/DoD/Soul/Skill/MCP/Memory context and observe execution context,
  Decision context, Step output, DoD assessment, and Memory refs.
- Embedded Web Console now uses Chinese business-facing labels and converged
  control-plane entries: business overview, workspace console, governance
  queue, runtime control, system configuration, and service status. Workspace
  detail exposes the main business models and process visualization from Goal
  and DoD through Run governance, runtime evidence, and terminal closure.
- LLM-driven Workspace execution has repeatable deployed-instance acceptance
  scripts: `make verify-llm` and `make verify-llm-file` both connect to the
  current `workorchd` service through `WORKORCH_BASE_URL` and use only a local
  OpenAI-compatible mock provider as an external dependency. Scripts no longer
  build, start, stop, or restart `workorchd`.
- Decision Queue supports batch resolution for multiple open Decision Tickets
  through the same formal resolution types as single-ticket resolution:
  `provide_missing_info`, `replan`, and `reject_current_path`.
- MCP `stdio` runtime execution has repeatable deployed-instance acceptance
  scripts: `make verify-mcp-stdio` and `make verify-mcp-stdio-file` both connect
  to the current `workorchd` service, temporarily register a governed MCP
  server, invoke it through an LLM execution graph, and restore temporary LLM
  provider configuration on exit.
- `make verify-real-mcp-stdio` verifies an already running `workorchd` instance
  by temporarily adding a local mock provider through the configuration API and
  executing a governed MCP `stdio` tool call in the deployed process.
- `make verify-real-vector-memory` verifies an already running `workorchd`
  instance by temporarily adding an embedding-capable mock provider through the
  configuration API, executing `/embeddings`-driven vector Memory recall, and
  checking `memoryRetrieval` evidence on the Run execution context.
- `make verify-real-weather` verifies an already running `workorchd` instance
  with the configured real LLM provider and a managed local MCP `stdio` weather
  endpoint. It queries Open-Meteo for Zhejiang Hangzhou current weather and
  checks the resulting ToolCall, ValidationResult, RuntimeCheckpoint, and DoD
  evidence in the Run snapshot.
- `make verify-weather-ticket-approval` verifies an already running
  `workorchd` instance with the configured real LLM provider and a temporary
  MCP `stdio` weather tool. It exercises the full governance chain for a
  specified city weather query: Decision Ticket creation and resolution,
  Approval creation and approval, MCP weather ToolCall execution, and final DoD
  evidence closure.
- `make verify-weather-ticket-approval-mock` is reserved for CI-style mock LLM
  checks. Local MVP validation must prefer the real LLM target above.
- MVP verification is grouped by dependency type:
  `make verify-mvp` verifies the current deployed service with service smoke,
  real LLM execution, real LLM weather query, and real LLM weather governance
  flows; `make verify-mvp-file` is kept as an alias for the
  same deployed-instance check. `make verify-ci-mvp-file` is the CI-safe
  grouped check with mock LLM/MCP flows.
- Boundary hardening is now executable: `make verify-boundary-contract` scans
  reusable runtime layers and Web source to prevent concrete business fallback
  logic from leaking into `kernel/orchestrator`, `kernel/run`, `blocks/llm`,
  `blocks/mcp`, or Web outcome rendering. Business-specific weather acceptance
  terms remain allowed only in e2e scripts, tests, and documentation.
- Workspace data layout has a formal next-step plan in
  `docs/workspace-data-layout-plan.md`: `.workorch-data` becomes the single
  persistence root, Workspace directories own `working`, `artifacts`, and
  Run-scoped evidence, while software source files and blog drafts remain in
  `working` and accepted delivery snapshots are stored under `artifacts`.
- Workspace default directory allocation now follows the data layout plan for
  the file backend: when `workingDirectory` is omitted, workorch creates
  `repository.dataDir/workspaces/{workspaceId}/working` and prepares sibling
  `artifacts` and `runs` directories. Explicit `workingDirectory` remains a
  user-managed workspace path and is created as the working root only.
- File backend Artifact storage now writes available text/report Artifact
  summaries under `repository.dataDir/workspaces/{workspaceId}/artifacts/{runId}`
  and stores a `workorch://workspaces/{workspaceId}/artifacts/{runId}/...` URI
  in Artifact metadata. The orchestrator still records only formal Artifact
  metadata; directory and URI materialization stay in the application bootstrap
  storage adapter.
- Artifact content can now be downloaded through
  `GET /api/v1/workorch/artifacts/content?uri=...`; the endpoint resolves only
  registered `workorch://workspaces/{workspaceId}/artifacts/{runId}/...` URIs
  and the Web Console exposes download actions in Artifact tables.
- File backend metadata now follows the Workspace data layout: Workspace,
  Goal, DoD, and MemoryItem are stored under
  `repository.dataDir/workspaces/{workspaceId}`; Run, Step, ToolCall, Patch,
  ValidationResult, RuntimeCheckpoint, Result, Artifact, Decision Ticket,
  Resolution, Approval, and Run-bound EventRecord are stored under
  `repository.dataDir/workspaces/{workspaceId}/runs/{runId}`; Soul, global
  Skill, MCP Server, and service-level EventRecord entries remain under
  `repository.dataDir/system`. Workspace list and directory lookup are backed
  by `repository.dataDir/system/workspace_index.json`, while Workspace metadata
  remains under `workspaces/{workspaceId}/workspace.json` and Workspace Soul
  bindings are persisted separately under
  `workspaces/{workspaceId}/config/soul_bindings.json`.
- Workspace task-stage Soul routing has a formal plan in
  `docs/workspace-soul-routing-plan.md`: each Workspace can define default and
  stage-specific Soul bindings, Run activation can override them for one Run,
  and the effective priority is run stage override, workspace stage binding,
  run default, workspace default, system default, then explicit unbound.
- Workspace task-stage Soul routing is now implemented for MVP use:
  Workspace stores `defaultSoulId` and stage `soulBindings`, Web Console can
  edit Workspace bindings and per-Run overrides, activation writes effective
  `executionContext.soulRouting`, and LLM provider requests expose
  `currentSoulStage`, `currentSoulBinding`, and `currentSoul` for the active
  planning / execution / decision / approval / validation context. LLM Step
  input can also explicitly select any formal Soul stage through `soulStage`,
  including `artifact`, without moving routing policy into the LLM block.
  `[runtime].defaultSoulId` is now the system default Soul source for the
  `system_default` priority slot and is exposed in the service configuration
  response / Web Console.
- Soul definition content is now managed through `soul.md`: structured Soul
  fields keep identity and capability boundaries, while the full persona,
  stage policies, and output contract are stored as Markdown under
  `repository.dataDir/system/souls/{soulId}/soul.md` for the file backend and
  exposed through API field `soulMd`.
- Soul capability enforcement now returns actionable startup diagnostics for
  denied Skill, MCP server, or Tool kind selections while preserving
  `ErrInvalidArgument` semantics for callers and HTTP `400` responses.

## Delivered in workorch

### Slice A: LLM-driven completion chain

Implemented and runnable:

1. create Workspace
2. define Goal
3. define DoD
4. activate Workspace execution with selected context
5. orchestrator dispatches `llm/workspace_execution`
6. runtime blocks produce ToolCall / Patch / ValidationResult / RuntimeCheckpoint evidence as required
7. DoD assessment is accepted and the Run reaches `completed`

### Slice B: decision-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create decision ticket
4. run transitions to blocked-on-decision state
5. resolve ticket
6. resume run
7. runtime orchestrator continues from the blocked Step
8. run reaches `completed`

### Slice C: approval-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create approval gate
4. run transitions to blocked-on-approval state
5. approve gate
6. resume run
7. runtime orchestrator continues from the blocked Step
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
- `/result/accepted`
- `/artifact/created`

Current internal event consumption boundary:

- `internal/modules/...` publish formal workorch business events into the shared `EventHub`
- `internal/modules/application/workorchd/pkg/observed/collector.go` subscribes only to the formal workorch topic set and stores normalized event input
- `internal/modules/application/workorchd/pkg/observed/workorch_contract.go` maps published business events to `/workspace|run|step|decision|approval/snapshot` items
- `internal/modules/application/workorchd/pkg/observed/workorch.go` only emits snapshot items from matching published business events; missing formal events remain visible as missing observed evidence
- Current default runtime `EventHub` is backed by `magicCommon/event.Hub` through `internal/pkg/events.MagicHub`.
- The HTTP service injects the application-level EventHub into request contexts, and appservice backend sessions reuse that hub instead of always creating a per-request hub.
- Observed runtime history remains runtime-local, while the formal `kernel/eventstream`
  path persists normalized Workorch EventHub records for file-backed replay.
  File-backed entity reload still must not synthesize missing events from repository state.
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
- all run execution entrypoints now share one in-process priority-aware wait queue behind `[server].maxConcurrentRuns`; equal priority preserves FIFO order
- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/{ticketId}`
- `POST /api/v1/workorch/decisions/{ticketId}/resolve`
- `POST /api/v1/workorch/decisions/resolve-batch`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`
- `POST /api/v1/workorch/approvals/resolve-batch`
- `POST /api/v1/workorch/souls`
- `PUT /api/v1/workorch/souls/{soulId}`
- `GET /api/v1/workorch/souls`
- `GET /api/v1/workorch/souls/{soulId}`
- `POST /api/v1/workorch/workspace-templates`
- `PUT /api/v1/workorch/workspace-templates/{templateId}`
- `GET /api/v1/workorch/workspace-templates`
- `POST /api/v1/workorch/skills`
- `GET /api/v1/workorch/skills`
- `GET /api/v1/workorch/skills/{skillId}`
- `POST /api/v1/workorch/mcp/servers`
- `PUT /api/v1/workorch/mcp/servers/{serverId}`
- `GET /api/v1/workorch/mcp/servers`
- `GET /api/v1/workorch/mcp/servers/{serverId}`
- `GET /api/v1/workorch/mcp/servers/{serverId}/tools`
- `GET /api/v1/workorch/mcp/servers/{serverId}/resources`
- `GET /api/v1/workorch/mcp/servers/{serverId}/prompts`
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

- the workspace create endpoint creates one formal `workorch` workspace instance, accepts an optional `description`, and defaults `workingDirectory` to `repository.dataDir/workspaces/<generated-workspace-id>/working` for the file backend when omitted
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

Formal deployed-instance verification:

```bash
make verify-service
make verify-service-file
```

Both targets connect to the current deployed service through `WORKORCH_BASE_URL`.
`make verify-service-file` is retained as an alias and does not start, stop, or
restart `workorchd`.

### File-backed acceptance verification

Verified with:

```bash
go run ./application/workorchd -config-dir config/file
```

Observed persisted files:

- `.workorch-data/system/workspace_index.json`
- `.workorch-data/system/souls/{soulId}/metadata.json`
- `.workorch-data/system/souls/{soulId}/soul.md`
- `.workorch-data/system/workspace_templates.json`
- `.workorch-data/system/skill.json`
- `.workorch-data/system/mcp_server.json`
- `.workorch-data/system/mcp_capability.json`
- `.workorch-data/system/mcp-endpoints/{mcpServerId}/{helper}`
- `.workorch-data/workspaces/{workspaceId}/workspace.json`
- `.workorch-data/workspaces/{workspaceId}/config/soul_bindings.json`
- `.workorch-data/workspaces/{workspaceId}/goals.json`
- `.workorch-data/workspaces/{workspaceId}/dods.json`
- `.workorch-data/workspaces/{workspaceId}/memory_item.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/run.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/step.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/ticket.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/approval.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/resolution.json`
- `.workorch-data/workspaces/{workspaceId}/runs/{runId}/artifact.json`

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
- framework entry adaptation is covered by `application/workorchd` and `internal/modules/application/workorchd/service/server` tests plus deployed-instance `make verify-service-file`
- magicCommon-backed event adapter is covered by `internal/pkg/events` and appservice / observed integration tests
- application-level EventHub injection is covered by appservice runtime-session tests and service-level verification
- local plugin lifecycle integration is covered by bootstrap lifecycle tests
- native magicEngine system routes are covered by server tests and `make verify-service-file`
- native magicEngine acceptance API routes are covered by server tests and `make verify-service-file`
- accepted API native route registration is now guarded by a route registry test, and wildcard `/**` fallback registration is explicitly forbidden

## Out Of Current Delivery Scope

以下内容不是当前已完成的 MVP / framework alignment / P1 交付范围内的活跃待办项。P1 已完成并记录在
`docs/next-stage-p1-development-plan.md`；其余候选项继续保留在
`docs/next-stage-demand-candidates.md`：

- distributed scheduling, external worker fleets, and multi-tenant execution quota

已完成的后续增强不再列为待办：

- file-backed repository adapter as the first non-memory persistence path
- priority-aware local run scheduling behind `[server].maxConcurrentRuns`
- MCP resources, prompts, streaming evidence, and long-lived stdio session reuse
- LLM provider streaming and non-chat provider APIs
- vector Memory search and real embedding-backed recall verification
- Decision batch resolution and Approval batch resolution

## Completed P1 Scope

P1 交付范围与验收记录见：

- `docs/next-stage-p1-development-plan.md`

当前活跃项：

- active: none

已完成：

- completed: `P1-1 LLM Provider Streaming And Non-Chat APIs`
- completed: `P1-2 Vector Memory Search`
- completed: `P1-3 Global Long-Lived Runtime Module Registration`

## Active Next Stage Scope

下一轮需求开发清单已晋级，范围与验收计划见：

- `docs/next-stage-result-event-development-plan.md`

当前活跃项：

- active: none

RE1 当前进度：

- completed: Result / Artifact 正式模型、仓储、file/memory 持久化与 runtime/bootstrap 导出。
- completed: LLM-driven Run 在满足正式 DoD assessment 后自动生成 accepted Result。
- completed: Run Snapshot 与 Workspace Snapshot 通过 read-model 暴露 Result / Artifact。
- completed: Web Console 的目标成果区优先消费正式 Result / Artifact，运行证据仍展示
  DoD assessment、Patch、ToolCall、ValidationResult 与 RuntimeCheckpoint。

RE2 当前进度：

- completed: EventRecord 正式模型、memory/file 仓储与 repository contract。
- completed: `kernel/eventstream` 模块订阅正式 Workorch EventHub 事件并持久化。
- completed: Run / Workspace Snapshot 与 Run Observability 已接入持久化事件记录。
- completed: HTTP 查询接口 `GET /api/v1/workorch/events` 支持按 workspace、run、topic、
  subject、cursor、limit 查询。
- completed: file-backed 服务重启后的事件流 replay 验证，覆盖 `/events` 查询、
  Run snapshot eventRecords 和 Run observability persisted event timeline。

边界：

- Result / Artifact 用于表达正式业务成果，不替代 ToolCall、Patch、ValidationResult、
  RuntimeCheckpoint、Memory 或 event evidence。
- Persistent Event Stream 用于审计与回放，不成为 Run / Workspace 状态拥有者。
- 通用 runtime 层和 Web 仍不得根据具体业务目标、城市、天气、工具 payload 或 provider
  特定字段合成最终业务结果。

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

## Advanced Runtime And Scheduling Scope

advanced runtime and scheduling 交付范围已完成，范围与验收记录见：

- `docs/advanced-runtime-scheduling-plan.md`

当前活跃项：

- active: none

已完成：

- completed: `A1 MCP Resources And Prompts`
- completed: `A2 MCP Streaming Evidence`
- completed: `A3 MCP Long-Lived Stdio Session`
- completed: `A4 Priority-Aware Local Run Scheduling`
- completed: `A5 Web Console Runtime Management`
- completed: `A6 Runtime Acceptance Gap Closure`

## Decision Arbitration Scope

决策与裁决能力继续按本地服务闭环推进，当前完成项：

- completed: `Decision Batch Resolution`
  Decision Queue can resolve multiple open Decision Tickets in one operation
  after prevalidating ticket existence, open state, supported resolution type,
  and duplicate IDs.

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
- 未配置 LLM provider 时，LLM-driven execution 必须明确失败，不能回落到旧运行时形成假完成
- kernel/orchestrator、kernel/run、blocks/llm、blocks/mcp 和 Web Console 不得根据具体
  Workspace 目标、城市、天气、工具 payload 字段或 provider 特定结构合成业务完成结果
- ToolCall/MCP 输出只能作为 evidence；最终目标成果必须来自正式 `Result` /
  `Artifact`，历史 `dodAssessment.summary` 只作为验收证据来源

该设计同步定义了后续实现验收的数据交互流程：

- happy path: Goal/DoD/Soul/Skill/MCP/Memory 绑定后，通过 `llm/workspace_execution`、具体 block action、DoD assessment 和 Memory write 完成 Run
- decision path: LLM 请求业务补充信息时创建 Decision Ticket，resolve 后 resume 并把 resolution 放入后续 planning context
- approval path: 风险动作进入 Approval，reject 后不得产生对应 ToolCall 或 Patch
- failure path: provider 缺失或 schema-invalid 时 Step 明确失败，不派发外部动作，不回落到旧运行时
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

## SkillHub Capability Integration

Skill 能力来源已从单一 workorch 内置仓储扩展为 workorch 读模型消费 `skill-hub`
管理结果，边界如下：

- `skill-hub` 继续负责 Skill 生命周期管理，包括 use/apply/status/repo sync/feedback
- workorch 只读取 `skill-hub status --json` 输出并映射为 Skill 读模型和 Run execution context
- 全局 Skill 通过 `skill-hub status --global --json --agent codex` 暴露为 `skillhub:global:<skillId>`
- Workspace 本地 Skill 通过在 `workspace.workingDirectory` 执行 `skill-hub status --json`
  暴露为 `skillhub:workspace:<workspaceId>:<skillId>`
- Web Console 在 Workspace 上下文下加载 Skill 列表，选择器展示来源、作用域、版本和状态

当前进度：

- completed: SkillHub command adapter and status JSON mapping
- completed: Skill read model fields for source/scope/status/path/hash/agent/managedBy
- completed: `GET /api/v1/workorch/skills?workspaceId=...` exposes repository, global SkillHub, and Workspace SkillHub skills
- completed: Workspace execution activation resolves repository Skill and SkillHub skill refs into Run context
- completed: Web activation Skill selector uses Workspace-aware Skill list and source-aware labels
- completed: unit tests for global SkillHub, Workspace SkillHub, and execution selection resolution
- completed: Goal/DoD-based automatic Skill selection using explicit trigger rules and Skill name/source ID matching
- completed: Skill-required MCP server refs are auto-bound into Workspace execution activation and still validated through MCP enabled state
- completed: selected Skill usage writes generic `skill_feedback` Memory after satisfied DoD completion
- completed: deployed-instance e2e script `scripts/verify-skillhub-workspace-execution.sh`
  covers global SkillHub Skill, Workspace SkillHub Skill, repository Skill required MCP auto-binding,
  Run snapshot context, accepted Result/Artifact, and `skill_feedback` Memory
- completed: SkillHub e2e uses a verification Soul to scope `allowedSkillRefs`
  to the current global, workspace, and repository Skill set, so repeated
  deployed-instance runs are not affected by historical repository Skill data
- completed: deployed-instance full regression target `make verify-deployed-full`
  includes service smoke, mock LLM governance, SkillHub, MCP stdio, real LLM,
  real weather, weather Decision/Approval governance, and vector Memory checks
- completed: LLM decision parser keeps top-level `LLMExecutionDecision` fields strict
  while ignoring unsupported nested governance extra fields from real providers

当前边界：

- Web 不直接触发 `skill-hub use/apply/feedback`，Skill 生命周期仍由 `skill-hub` 管理
- 自动选择不使用模糊业务语义推断，只根据 Skill `triggerRules` 和 Skill 名称/source ID
  与 Goal/DoD 文本的通用匹配，避免框架层解析具体业务目标
- `skill-hub feedback` 仍不由 workorch 自动触发；运行效果先进入 workorch `skill_feedback`
  Memory，Skill 内容归档继续由 skill-hub 生命周期命令处理

## Task Progress And Recovery Projection

本轮围绕 LLM 上下文压缩和多轮任务恢复的前置能力，先落地只读 `TaskProgress`
业务投影。该投影不作为执行状态机的新事实源，只从 Workspace / Run / Step、
Decision / Approval、ToolCall / Patch / ValidationResult、RuntimeCheckpoint、
Result / Artifact 等已持久化模型推导任务进度、阻塞项、证据引用和恢复建议。

当前进度：

- completed: Run snapshot 和 Run observability 暴露 `taskProgress`
- completed: `GET /api/v1/workorch/runs/{runId}/task-progress`
- completed: `GET /api/v1/workorch/workspaces/{workspaceId}/task-progress`
- completed: Web Workspace Detail 展示任务进度里程碑、阻塞项和恢复建议
- completed: 单元测试覆盖正式 Result / Artifact 完成态、Decision 阻塞恢复、Workspace 活跃 Run 恢复投影
- completed: `ContextBundle` 只读上下文恢复包，汇总身份信息、执行上下文、
  `TaskProgress`、最近证据、压缩候选和恢复检查点
- completed: `CompressedContext` 确定性压缩摘要，保留恢复建议、阻塞项、
  里程碑、最近证据摘要、正式结果引用和源证据引用
- completed: `GET /api/v1/workorch/runs/{runId}/context-bundle`
- completed: `GET /api/v1/workorch/workspaces/{workspaceId}/context-bundle`
- completed: `TaskDecomposition` 只读任务拆分投影，从 Run step 输出中的
  `executionGraph` 和后续 graph node 执行步骤推导拆分图、节点依赖、节点状态和证据引用
- completed: `GET /api/v1/workorch/runs/{runId}/task-decompositions`
- completed: `GET /api/v1/workorch/workspaces/{workspaceId}/task-decompositions`
- completed: Run start / resume 前自动将 `compressedContext` 写入 Run
  `executionContext`，后续 `llm/workspace_execution` 可直接读取压缩恢复上下文
- completed: Web Workspace Detail 展示上下文分区、压缩上下文、最近证据和压缩候选
- completed: Web Workspace Detail 展示任务拆分图、节点依赖、节点执行状态和节点输入证据
- completed: 单元测试覆盖上下文包、压缩上下文、恢复检查点、压缩候选推导，
  start / resume 前的压缩上下文注入，以及 execution graph 到任务拆分投影的映射
- completed: deployed-instance e2e coverage now checks Run `task-progress`,
  Run `context-bundle`, persisted `executionContext.compressedContext`,
  event stream terminal records, and completed `formal_outputs` through
  `make verify-llm`
- completed: real LLM deployed-instance e2e now checks `task-progress`,
  `context-bundle`, `task-decompositions`, compressed context identity,
  formal Result / Artifact, and event stream records through
  `make verify-real-llm`

当前边界：

- `TaskProgress` 只读取业务模型，不直接驱动调度或修改 Run 状态
- `TaskDecomposition` 只读取 LLM execution graph 和已落库 Step evidence，
  不作为 scheduler、dispatcher 或 block execution graph 的新事实源
- `CompressedContext` 当前采用确定性压缩，不调用 LLM 进行二次摘要
- LLM 参与的进一步语义压缩仍作为后续增量开发
- 投影规则只使用通用状态、证据和治理模型，不解析具体 Workspace Goal 业务语义

## MVP Closure Record

MVP 功能收口记录见：

- `docs/mvp-backlog.md`

MVP 功能清单已收口，不再作为活跃待办清单使用。

## Workspace Concurrency Load Test Plan

workspace 并发压测方案已固化，见：

- `docs/workspace-concurrency-loadtest-plan.md`

压测采用 mock LLM 容量压测和 real LLM 真实端到端压测双轨设计，业务场景必须覆盖多轮
Decision Ticket、Approval、MCP / ToolCall、RuntimeCheckpoint、ValidationResult、Result、
Artifact 与 observability 回查。

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
