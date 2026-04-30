# workorch LLM Driven Development Plan

## Purpose

This document tracks the detailed development plan for making `workorchd`
execute Workspace work from `Goal` and `DoD` through an LLM-backed orchestration
loop.

The source design is:

- `docs/llm-driven-workspace-execution-design.md`

The plan originally scoped the first locally deployable LLM-driven MVP. That
MVP is now completed, and later runtime expansion items are tracked in
`docs/runtime-expansion-plan.md` and
`docs/advanced-runtime-scheduling-plan.md`.
This document is retained as a completed implementation record rather than an
active backlog.

Vector Memory search has been promoted into the completed P1 scope tracked in
`docs/next-stage-p1-development-plan.md`. Still outside the completed local
service delivery scope: distributed scheduling, multi-tenant authorization, and
an external runtime marketplace.

## Tracking Rules

Each item uses one of these statuses:

```text
not_started
in_progress
blocked
completed
deferred
```

Completion requires code, tests, documentation, and local verification unless
the item is explicitly marked as design-only.

## Development Sequence

| ID | Work Item | Status | Deliverable | Acceptance |
| --- | --- | --- | --- | --- |
| L1 | Soul model and repository | completed | Formal `Soul` model, memory/file repository support, module registration | Soul can be created/listed/read; activation binding uses this service in L6 |
| L2 | Memory model and repository | completed | Formal `MemoryItem` model, Workspace-scoped list/search, source citation validation | Memory can be created, searched by Workspace, persisted through file backend restart |
| L3 | Skill and MCP registry baseline | completed | Formal Skill metadata and MCP server registry baseline | Skill and MCP refs can be persisted/read; activation policy binding uses these services in L6 |
| L4 | LLM provider configuration | completed | Provider config, disabled-provider behavior, redaction rules | Provider availability and redacted config are deterministic; runtime failure path is consumed in L5/L7 |
| L5 | `blocks/llm` runtime adapter skeleton | completed | `llm/workspace_execution` adapter module, provider request boundary, schema parser | Adapter returns normalized Step output or schema/provider failure output |
| L6 | Workspace execution activation context | completed | Activation request accepts Goal, DoD, Soul, Skill, MCP selections and recalled Memory refs | Created Run snapshot exposes selected execution context refs |
| L7 | Orchestrator LLM step integration | completed | Runtime registry dispatches `llm/workspace_execution`; normalized output maps to state/evidence | First execution Step is LLM planning and does not use alternate runtime completion |
| L8 | LLM decision to action mapping | completed | Shell, patch, MCP, Decision, Approval, Memory write requests map from `LLMExecutionDecision` | Continue, block, approval, patch, and tool-call paths create expected evidence |
| L9 | DoD assessment completion gate | completed | Run completion requires schema-valid DoD assessment with evidence refs | Run cannot complete without evidence-backed DoD assessment |
| L10 | Memory write and recall loop | completed | Run summary and decision-derived Memory writes; second Run recall | Second Run includes prior relevant Memory in LLM planning context |
| L11 | API surface for Soul, Skill, MCP, Memory | completed | HTTP endpoints for management and Workspace-scoped Memory | API supports the baseline acceptance dataset setup |
| L12 | Web console LLM execution views | completed | Workspace Board activation, Workspace Detail context/evidence, Decision Queue, Observability | Web can start and observe the same flow as API without low-level adapter controls |
| L13 | End-to-end acceptance scripts | completed | Local verification scripts for memory and file repository backends | Happy path, Decision, Approval, provider/schema failure, Memory recall are repeatable |
| L14 | Status and operator documentation | completed | Updated status, deployment, and acceptance documentation | Docs describe config, verification commands, known limits, and current status |
| L15 | LLM decision output stabilization and graph planning | completed | DeepSeek/OpenAI-compatible JSON output contract, one repair retry, formal `executionGraph` in `LLMExecutionDecision` | Schema-invalid provider output is repaired once or fails with diagnostics; graph plans become orchestrator-consumable Step output |
| L16 | Runtime graph follow-up and workspace shell sandbox closure | completed | Graph leaf LLM follow-up dispatch and automatic shell working directory creation | Graph-based real provider Runs can return to LLM for DoD assessment after block execution |
| L17 | MCP HTTP tool-call runtime adapter | completed | `blocks/mcp` runtime unit registered for `mcp/tool_call` with MCP server policy checks and HTTP endpoint invocation | Allowed HTTP tool calls produce Step output and ToolCall evidence; disabled, disallowed, or unsupported transports fail explicitly |
| L18 | Concurrent ready-node graph dispatch | completed | Orchestrator dispatches all dependency-satisfied graph nodes in one scheduler batch | Independent ready nodes execute concurrently, dependent nodes wait for all required predecessors, and Run closure remains coordinator-owned |
| L19 | MCP stdio protocol transport adapter | completed | `blocks/mcp` supports MCP stdio subprocess transport with JSON-RPC `initialize`, `notifications/initialized`, and `tools/call` | Allowed stdio tool calls produce decoded Step output and ToolCall evidence; invalid endpoint, protocol error, timeout, or unsupported transports fail explicitly |

## Detailed Work Items

### L1 Soul Model And Repository

Scope:

- Add `internal/modules/kernel/soul`.
- Define `Soul` business model with capability boundaries.
- Add repository contracts for create, get, list, and update.
- Add memory and file backend persistence.
- Register the module explicitly through the `workorchd` application loading path.

Acceptance:

- `Soul` records can be created and read through service contracts.
- File backend restart preserves `Soul` records.
- `Soul` service can reject disallowed Skill, MCP server, or tool refs before
  Run creation.
- Workspace execution activation consumes the `Soul` service in L6.

Status:

- completed
- implementation: `internal/modules/kernel/soul`
- repository: `internal/pkg/repository.SoulRepository`
- backends: `internal/pkg/repository/memory.NewSoulRepository`,
  `internal/pkg/repository/file.NewSoulRepository`
- runtime loading: explicit `workorchd` import and `workorchruntime`
  `RepositorySet` / `SoulService` injection
- verification:
  `go test ./internal/modules/kernel/soul/... -count 1`,
  `go test ./internal/pkg/repository/... -count 1`,
  `go test ./internal/initiators/workorchruntime/... ./application/workorchd -count 1`

### L2 Memory Model And Repository

Scope:

- Add `internal/modules/kernel/memory`.
- Define `MemoryItem` with `workspaceId`, `runId`, `sourceType`,
  `sourceRef`, `memoryType`, `summary`, `tags`, `importance`,
  `confidence`, and `visibility`.
- Add deterministic Workspace-scoped search by keyword, tag, and memory type.
- Validate source refs for execution-derived Memory.

Acceptance:

- User-provided `project_context` Memory can be created without execution source
  refs.
- Execution-derived Memory requires source refs.
- First Run can write summary Memory.
- Second Run can recall prior Workspace Memory.

Status:

- completed
- implementation: `internal/modules/kernel/memory`
- repository: `internal/pkg/repository.MemoryItemRepository`
- backends: `internal/pkg/repository/memory.NewMemoryItemRepository`,
  `internal/pkg/repository/file.NewMemoryItemRepository`
- runtime loading: explicit `workorchd` import and `workorchruntime`
  `RepositorySet` / `MemoryService` injection
- current boundary: Run summary write and second Run recall are implemented in
  L10 using this service
- verification:
  `go test ./internal/modules/kernel/memory/... -count 1`,
  `go test ./internal/pkg/repository/... -count 1`,
  `go test ./internal/initiators/workorchruntime/... ./application/workorchd -count 1`

### L3 Skill And MCP Registry Baseline

Scope:

- Add `internal/modules/kernel/skill`.
- Add `internal/modules/kernel/mcp`.
- Define Skill metadata and MCP server metadata.
- Support minimal create, get, list, and policy validation.
- MCP invocation is not required in this item.

Acceptance:

- Activation rejects unknown Skill or MCP server refs.
- Activation rejects Skill or MCP refs not allowed by selected Soul.
- Activation can pass allowed Skill content and MCP tool catalog refs into Run
  execution context.

Status:

- completed
- implementation: `internal/modules/kernel/skill`,
  `internal/modules/kernel/mcp`
- repositories: `internal/pkg/repository.SkillRepository`,
  `internal/pkg/repository.MCPServerRepository`
- backends: `internal/pkg/repository/memory.NewSkillRepository`,
  `internal/pkg/repository/memory.NewMCPServerRepository`,
  `internal/pkg/repository/file.NewSkillRepository`,
  `internal/pkg/repository/file.NewMCPServerRepository`
- runtime loading: explicit `workorchd` imports and `workorchruntime`
  `RepositorySet` / `SkillService` / `MCPService` injection
- current boundary: activation policy validation and context binding are
  implemented in L6; MCP invocation is outside L3
- verification:
  `go test ./internal/modules/kernel/skill/... ./internal/modules/kernel/mcp/... -count 1`,
  `go test ./internal/pkg/repository/... -count 1`,
  `go test ./internal/initiators/workorchruntime/... ./application/workorchd -count 1`

### L4 LLM Provider Configuration

Scope:

- Add formal provider config fields:
  `[llm].defaultProvider`, `[llm.providers.<id>].provider`,
  `[llm.providers.<id>].baseURL`, `[llm.providers.<id>].apiKey`,
  `[llm.providers.<id>].model`, `[llm.providers.<id>].timeout`, and
  `[llm.providers.<id>].disabled`.
- Add provider availability validation.
- Add redaction helper for logs, evidence, checkpoints, and Web/API payloads.

Acceptance:

- Disabled or missing provider fails LLM-driven execution clearly.
- No provider secret appears in snapshots, Memory, RuntimeCheckpoint, or logs
  intended for users.
- Failure path does not dispatch shell, patch, MCP, or alternate runtime steps.

Status:

- completed
- implementation: `internal/modules/blocks/llm/biz/provider_config.go`,
  `internal/modules/blocks/llm/pkg/common`,
  `internal/modules/blocks/llm/pkg/models`
- current boundary: provider availability and redaction are implemented; the
  actual runtime Step failure path consumes this config in L5/L7
- verification:
  `go test ./internal/modules/blocks/llm/... -count 1`,
  `go test ./... -count 1`

### L5 `blocks/llm` Runtime Adapter Skeleton

Scope:

- Add `internal/modules/blocks/llm`.
- Register runtime adapter kind `llm/workspace_execution`.
- Define `WorkspaceExecutionContext`, `LLMProviderRequest`,
  `LLMExecutionDecision`, and `NormalizedRuntimeOutput`.
- Implement strict schema validation for provider output.

Acceptance:

- Valid provider output becomes normalized Step output.
- Invalid JSON or schema-invalid fields fail the Step with validation details.
- The adapter does not mutate repositories or execute external actions.

Status:

- completed
- implementation: `internal/modules/blocks/llm`
- registered step kind: `llm/workspace_execution`
- provider boundary: `ProviderRequest`, `Provider`, `RuntimeAdapter`
- schema boundary: `LLMExecutionDecision` with strict JSON parsing and
  `completionState` validation
- response stabilization: provider content is normalized only for transport
  formatting, including markdown fence removal and first JSON object extraction;
  business schema validation remains strict with unknown fields rejected
- repair boundary: schema-invalid provider output triggers exactly one LLM
  repair retry with the raw output preview and parse error; failed repair keeps
  the original parse failure as the Step failure and exposes repair diagnostics
- real provider client: OpenAI-compatible Chat Completions HTTP client is
  created from magicCommon `framework/configuration` LLM section when provider
  config is ready
- normalized output covers provider unavailable, provider error, schema invalid,
  execution graph plan, shell plan, patch requests, MCP plan, Decision block,
  Approval block, Memory write requests, and DoD assessment output
- current boundary: the first MVP used OpenAI-compatible `/chat/completions`;
  provider-declared streaming and non-chat operations were completed later in
  the P1 delivery scope
- verification:
  `go test ./internal/modules/blocks/llm/... ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/kernel/orchestrator/... ./application/workorchd -count 1`,
  `go test ./... -count 1`

### L6 Workspace Execution Activation Context

Scope:

- Add or extend activation API for selecting `goalId`, `doDId`, `soulId`,
  `skillIds`, and `mcpServerIds`.
- Persist execution context refs on Run creation or Run snapshot.
- Add recalled Memory ids to the first LLM planning context.

Acceptance:

- Activation validates every selected object before Run creation.
- Run snapshot exposes selected Goal, DoD, Soul, Skills, MCP servers, and
  recalled Memory refs.
- Missing required context returns a client-visible validation error.

Status:

- completed
- activation command/API now accepts `goalId`, `doDId`, `soulId`, `skillIds`,
  `mcpServerIds`, and `memoryIds`
- selected Soul, Skills, MCP servers, and Memory are validated before Run
  creation
- selected Skill required MCP server refs must be present in the activation
  selection
- selected Soul capability policy validates selected Skill refs, MCP server
  refs, and required tool kinds
- when `memoryIds` is omitted, activation recalls the first Workspace Memory
  items as planning context
- Run snapshot and first LLM Step input expose the persisted execution context
- verification:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count 1`,
  `npm run build`

### L7 Orchestrator LLM Step Integration

Scope:

- Ensure LLM-driven activation dispatches `llm/workspace_execution` as first
  Step.
- Prevent LLM-driven execution from silently completing through any fallback
  runtime.
- Map provider/schema failure into failed Step evidence and stable Run state.

Acceptance:

- First Step kind is `llm/workspace_execution`.
- Provider-unavailable and schema-invalid paths are observable in Run snapshot.
- No false completed Run occurs when LLM provider is not usable.

Status:

- completed
- Run creation now supports `InitialStepKind` and persistent
  `ExecutionContext` for business-driven execution.
- Workspace execution activation creates Runs with
  `llm/workspace_execution` as the first Step kind.
- Orchestrator defaults business activation to `llm/workspace_execution` and
  does not register an alternate runtime fallback.
- Runtime output `nextInput` is carried into the next dispatched Step so LLM
  decisions can hand concrete command/tool input to follow-up block execution.
- Provider-unavailable LLM execution fails the Step and Run with observable
  `llm_provider_unavailable` output instead of completing through an alternate
  runtime path.
- verification:
  `go test ./internal/modules/kernel/run/... ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/biz/appservice -count 1`

### L8 LLM Decision To Action Mapping

Scope:

- Map `shellCommands` to `shell/command`.
- Map `patchRequests` to patch application evidence.
- Map `mcpToolCalls` to `mcp/tool_call`.
- Map `requiredDecision` to Decision Ticket.
- Map `requiredApproval` to Approval.
- Map `memoryWrites` to Memory write requests.

Acceptance:

- Continue action dispatches only approved next Step kinds.
- Decision block creates Decision Ticket and enters `blocked_on_decision`.
- Approval block creates Approval and enters `blocked_on_approval`.
- Rejected Approval does not produce ToolCall or Patch for the rejected action.

Status:

- completed
- `LLMExecutionDecision` now normalizes shell commands, MCP tool-call
  requests, patch requests, Memory write requests, required Decision,
  required Approval, completed DoD assessment, and failed LLM decisions into
  stable runtime Step output.
- Required Decision output carries `blockDecisionKind`,
  `blockDecisionQuestion`, and `blockDecisionContext`; the orchestrator
  persists these fields into the created Decision Ticket.
- Failed LLM decisions now fail the Step and Run with `llm_failed` evidence
  instead of being treated as a completed plan.
- Unknown runtime kinds no longer fall back to an alternate runtime; they fail with
  `runtime_adapter_unavailable`.
- Memory write requests require `memoryType` and `summary` at schema parsing
  time.
- `mcp/tool_call` now has a concrete `blocks/mcp` runtime adapter in L17; this
  item owns only the LLM decision-to-runtime mapping boundary.
- verification:
  `go test ./internal/modules/blocks/llm/... -count 1`,
  `go test ./internal/modules/kernel/orchestrator/... -count 1`

### L9 DoD Assessment Completion Gate

Scope:

- Require `dodAssessment` before LLM-driven Run completion.
- Validate each satisfied DoD item has evidence refs.
- Fail or block completion when DoD assessment is missing or unsupported.

Acceptance:

- Run cannot complete without DoD assessment.
- Satisfied DoD item without evidence refs is rejected.
- Completed Run snapshot exposes DoD assessment and evidence refs.

Status:

- completed
- `blocks/llm` now exposes completed `dodAssessment` as first-class Step
  output while retaining the full raw `llmDecision` payload.
- LLM-driven Runs with a `doDId` cannot transition to `completed` unless the
  terminal Step output contains a satisfied DoD assessment.
- Missing, unsatisfied, malformed, or evidence-free satisfied DoD assessment
  items fail the Step and Run through the runtime completion gate.
- The gate applies to LLM-driven execution and leaves non-LLM runtime kinds to
  their own explicit contracts.
- verification:
  `go test ./internal/modules/blocks/llm/... -count 1`,
  `go test ./internal/modules/kernel/orchestrator/... -count 1`

### L10 Memory Write And Recall Loop

Scope:

- Persist `run_summary`, `execution_lesson`, `decision`,
  `dod_feedback`, and `tool_feedback` Memory where applicable.
- Include recalled Memory summaries and source refs in subsequent LLM context.
- Expose Memory references in Run snapshots and Web observability.

Acceptance:

- Completed Run writes summary Memory with source refs.
- Decision resolution can create Memory when material.
- Second Run in same Workspace recalls prior relevant Memory.

Status:

- completed
- Orchestrator now persists normalized `memoryWrites` from runtime Step output
  into Workspace Memory records with Run source refs.
- Terminal LLM DoD assessment output with a summary also creates a
  `run_summary` Memory item.
- Created Memory IDs are written back to Step output as `memoryRefs` for Run
  snapshot and observability.
- Subsequent Workspace execution activation already recalls Workspace Memory
  into the first LLM Step context when `memoryIds` is omitted.
- current boundary: Decision resolution can create durable Memory through the
  existing Memory API when material; automatic resolution-to-Memory synthesis
  is not required for this MVP item.
- verification:
  `go test ./internal/modules/kernel/orchestrator/... -count 1`,
  `go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd -count 1`

### L11 API Surface For Soul, Skill, MCP, Memory

Scope:

- Add management endpoints for Soul, Skill, MCP server, and Memory.
- Add Workspace-scoped Memory endpoints.
- Keep provider secrets out of HTTP responses.

Acceptance:

- Baseline acceptance dataset can be created through HTTP APIs.
- Management endpoints support local verification and Web console operations.
- Invalid requests return clear validation errors.

Status:

- completed
- API:
  `POST /api/v1/workorch/souls`,
  `GET /api/v1/workorch/souls`,
  `GET /api/v1/workorch/souls/{soulId}`,
  `POST /api/v1/workorch/skills`,
  `GET /api/v1/workorch/skills`,
  `GET /api/v1/workorch/skills/{skillId}`,
  `POST /api/v1/workorch/mcp/servers`,
  `PUT /api/v1/workorch/mcp/servers/{serverId}`,
  `GET /api/v1/workorch/mcp/servers`,
  `GET /api/v1/workorch/mcp/servers/{serverId}`,
  `GET /api/v1/workorch/mcp/servers/{serverId}/tools`,
  `POST /api/v1/workorch/workspaces/{workspaceId}/memory`,
  `GET /api/v1/workorch/memory?workspaceId={workspaceId}`
- implementation: `internal/modules/application/workorchd/biz/appservice/context_registry_service.go`,
  `internal/modules/application/workorchd/service/server/http.go`
- current boundary: API creates and reads baseline context data only; LLM
  activation context binding remains L6, and repeatable E2E scripts remain L13
- verification:
  `go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count 1`,
  `go test ./... -count 1`

### L12 Web Console LLM Execution Views

Scope:

- Add Workspace Board activation controls for Goal, DoD, Soul, Skill, and MCP
  selections.
- Extend Workspace Detail with selected execution context, recalled Memory,
  Steps, and evidence.
- Extend Decision Queue for LLM-generated Decision and Approval context.
- Extend Runtime Observability for LLM planning, normalized decisions, DoD
  assessment, and Memory writes.

Acceptance:

- Web can start LLM-driven execution through HTTP APIs.
- Web can resolve Decision and Approval without direct adapter controls.
- Web exposes enough evidence to verify progress and failure cause.

Status:

- completed
- Workspace Console activation now supports selecting Goal, DoD, Soul, Skills,
  MCP servers, and Memory through HTTP APIs.
- Decision Queue exposes structured Decision context and supported resolution
  actions.
- Run observability exposes execution context, Step output stage, next runtime
  kind, Memory refs, and DoD assessment evidence.
- Embedded console assets were rebuilt into `workorchd`.
- verification:
  `npm run build`

### L13 End-To-End Acceptance Scripts

Scope:

- Add repeatable local scripts for the five acceptance flows:
  happy path, Decision resume, Approval rejection, provider/schema failure, and
  Memory recall.
- Cover memory backend and file backend restart verification.

Acceptance:

- Scripts fail on missing evidence, missing DoD assessment, unexpected runtime
  fallback, missing Memory recall, or leaked provider secrets.
- Deployed-instance verification preserves business objects and relationships
  through API reads; restart-read verification is an operator-managed deployment
  check, not something e2e scripts perform by starting or stopping `workorchd`.

Status:

- completed
- Added `scripts/verify-llm-workspace-execution.sh`, which connects to the
  currently deployed `workorchd` instance and starts only a local
  OpenAI-compatible mock provider.
- Added `make verify-llm` and `make verify-llm-file`.
- The script covers happy path, Decision resume, Approval rejection,
  schema-invalid provider output, and Memory write/recall through the deployed
  service API.
- LLM-created default Decision and Approval IDs are now URL-safe so HTTP
  detail and resolution routes can operate on them directly.
- verification:
  `make verify-llm`,
  `make verify-llm-file`,
  `make verify-real-llm`

### L14 Status And Operator Documentation

Scope:

- Update `docs/workorch-status.md`.
- Update deployment and acceptance docs with LLM provider config.
- Document known MVP limits and non-goals.

Acceptance:

- Status document reflects current completed and active item.
- Local operator can configure provider and run acceptance checks from docs.
- Documentation does not describe unfinished items as completed.

Status:

- completed
- Updated `docs/workorch-status.md` and this plan to reflect completed
  LLM-driven MVP work items.
- Updated `docs/deployment.md` with TOML runtime configuration, LLM provider
  configuration, mock-provider acceptance targets, supported fields, and MVP
  protocol boundary.
- Updated `config/application.toml`, `config/file/application.toml`, and
  `.env.workorchd.example` with the current configuration entrypoints.
- verification:
  `make verify-llm`,
  `make verify-llm-file`,
  `make verify-real-llm`

### L17 MCP HTTP Tool-Call Runtime Adapter

Scope:

- Add `internal/modules/blocks/mcp`.
- Register runtime adapter kind `mcp/tool_call`.
- Validate selected MCP server existence, enablement, and `allowedTools`.
- Invoke HTTP/HTTPS MCP-style tool endpoints with workorch trace refs and tool
  input.
- Keep this item scoped to HTTP/HTTPS endpoint invocation; MCP `stdio` is
  delivered separately in L19.

Acceptance:

- Allowed HTTP tool calls complete and expose decoded response payload in Step
  output.
- Disabled servers, disallowed tools, unsupported transports, invalid endpoint,
  timeout, and non-2xx HTTP responses fail deterministically.
- Orchestrator records the result through the existing ToolCall and
  ValidationResult evidence path.

Status:

- completed
- implementation: `internal/modules/blocks/mcp`
- runtime loading: explicit `workorchd` import, `workorchruntime`
  `MCPRuntimeAdapter` injection, and orchestrator runtime registry registration
- current boundary for this item: supported invocation transport is HTTP/HTTPS
  endpoint call; MCP `stdio` is tracked and completed in L19
- verification:
  `go test ./internal/modules/blocks/mcp/... ./internal/initiators/workorchruntime/... ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/kernel/orchestrator/... -count 1`

### L18 Concurrent Ready-Node Graph Dispatch

Scope:

- Extend `kernel/orchestrator` graph traversal from single ready-node dispatch
  to scheduler batches.
- Dispatch all ready graph nodes whose dependencies are satisfied and branch
  policy matches.
- Prevent parallel graph nodes from independently advancing or closing the Run;
  the coordinator decides the next batch, graph follow-up, or terminal state
  after the batch completes.

Acceptance:

- Two independent graph nodes execute concurrently before a dependent merge
  node.
- The merge node sees both predecessor node IDs in `graphCompletedNodes`.
- Existing graph branch, Decision, Approval, LLM follow-up, and DoD completion
  paths remain valid.

Status:

- completed
- implementation: `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
- verification:
  `go test ./internal/modules/kernel/orchestrator/... -count 1`

### L19 MCP Stdio Protocol Transport Adapter

Scope:

- Extend `internal/modules/blocks/mcp` without changing `kernel/orchestrator`
  or `kernel/mcp` ownership.
- Start the configured `MCPServer.endpoint` command as a local stdio
  subprocess when `transport=stdio`.
- Exchange newline-delimited UTF-8 JSON-RPC messages over stdin/stdout.
- Send `initialize`, then `notifications/initialized`, then `tools/call` with
  `{name, arguments}`.
- Preserve the existing MCP server enablement and `allowedTools` policy gate.

Acceptance:

- Allowed stdio tool calls complete and expose decoded JSON-RPC `result` in Step
  output.
- Invalid endpoint, subprocess launch failure, JSON-RPC error, timeout, and
  unsupported transports fail deterministically.
- Orchestrator records the result through the existing ToolCall and
  ValidationResult evidence path.

Status:

- completed
- implementation: `internal/modules/blocks/mcp/biz/mcp_runtime_adapter.go`
- current boundary: initial stdio scope covered per-call subprocess
  `initialize` + `tools/call`; later MCP resources, prompts, streaming evidence,
  and long-lived stdio session reuse are completed in
  `docs/advanced-runtime-scheduling-plan.md`. External runtime marketplace
  integration remains future work.
- verification:
  `go test ./internal/modules/blocks/mcp/... -count 1`,
  `make verify-mcp-stdio`,
  `make verify-mcp-stdio-file`,
  `make verify-real-mcp-stdio`
- grouped MVP verification:
  `make verify-mvp`,
  `make verify-mvp-file`,
  `make verify-real-mvp`

## Recommended Development Rounds

### Round 1: Business Context Foundation

Target items:

- L1 Soul model and repository.
- L2 Memory model and repository.
- L3 Skill and MCP registry baseline.
- L11 minimal API surface for creating the baseline dataset.

Exit criteria:

- Baseline acceptance dataset can be created and read back.
- File backend restart preserves Soul, Skill, MCP server, and Memory records.

### Round 2: LLM Runtime Boundary

Target items:

- L4 LLM provider configuration.
- L5 `blocks/llm` runtime adapter skeleton.
- L7 Orchestrator LLM step integration.

Exit criteria:

- `llm/workspace_execution` is registered and dispatchable.
- Missing provider and schema-invalid provider output fail safely.
- LLM-driven flow cannot complete through an unregistered fallback runtime.

### Round 3: Workspace Execution Loop

Target items:

- L6 Workspace execution activation context.
- L8 LLM decision to action mapping.
- L9 DoD assessment completion gate.
- L10 Memory write and recall loop.

Exit criteria:

- Happy path reaches evidence-backed completion.
- Decision and Approval paths block, resolve, and resume correctly.
- Second Run can recall prior Memory.

### Round 4: Management Surface And Acceptance

Target items:

- L12 Web console LLM execution views.
- L13 end-to-end acceptance scripts.
- L14 status and operator documentation.

Exit criteria:

- API and Web both verify the same business flow.
- Acceptance scripts cover memory and file backends.
- Status document accurately tracks completed items and confirms no active
  leftovers for this scope.

### Round 5: Real Provider Stabilization

Target items:

- L15 LLM decision output stabilization and graph planning.
- L16 Runtime graph follow-up and workspace shell sandbox closure.
- L17 MCP HTTP tool-call runtime adapter.
- L18 Concurrent ready-node graph dispatch.

Exit criteria:

- DeepSeek/OpenAI-compatible JSON mode uses a strict decision contract.
- Provider formatting drift is normalized only at the transport boundary.
- Schema-invalid output triggers one repair retry and then either produces a
  valid decision or fails with parse and repair diagnostics.
- `executionGraph` plans are emitted as normalized Step output for the
  orchestrator-owned runtime graph chain.
- Graph leaf completion returns to LLM DoD assessment.
- HTTP/HTTPS MCP tool calls are policy-checked and recorded as runtime
  evidence.
- Independent ready graph nodes run concurrently and dependent graph nodes wait
  for all required predecessors.
- MCP stdio tool calls are policy-checked, executed through the standard
  newline-delimited JSON-RPC flow, and recorded as runtime evidence.

## Current Progress

| ID | Status | Notes |
| --- | --- | --- |
| L1 | completed | Soul model, repository contracts, memory/file backends, service validation, module registration, and runtime injection completed. Activation binding remains L6. |
| L2 | completed | MemoryItem model, repository contracts, memory/file backends, deterministic Workspace search, source validation, module registration, and runtime injection completed. Run summary write and recall integration remain L10. |
| L3 | completed | Skill and MCPServer models, repository contracts, memory/file backends, services, module registration, and runtime injection completed. Activation policy binding is implemented in L6, HTTP/HTTPS MCP invocation in L17, and MCP stdio invocation in L19. |
| L4 | completed | LLM provider env config, disabled/unavailable state, timeout validation, supported provider validation, and redaction helper completed. Runtime dispatch failure path remains L5/L7. |
| L5 | completed | LLM runtime adapter, OpenAI-compatible HTTP provider client, provider request boundary, strict decision parsing, normalized output, module registration, runtime helper injection, and bootstrap/orchestrator registration completed. |
| L6 | completed | Workspace execution activation now binds selected Soul, Skill, MCP server, and Memory context into Run execution context and first LLM Step input; disallowed or missing selections are rejected before Run creation. |
| L7 | completed | LLM-driven activation dispatches `llm/workspace_execution` first, propagates execution context and `nextInput`, and fails visibly when provider config is unavailable instead of falling back to an alternate runtime path. |
| L8 | completed | LLM decisions now map to shell, MCP boundary, patches, Memory write requests, structured Decision Tickets, Approval gates, DoD assessment, and explicit failed decisions; unregistered runtime kinds fail instead of using alternate runtime fallback. |
| L9 | completed | LLM-driven Runs with DoD now require a satisfied evidence-backed DoD assessment before completion; missing or invalid assessments fail the Step and Run. |
| L10 | completed | Runtime `memoryWrites` and terminal run summaries now persist as Workspace Memory with Run source refs; created IDs are exposed as `memoryRefs`, and later activation can recall Workspace Memory into LLM context. |
| L11 | completed | Minimal HTTP API surface for Soul, Skill, MCPServer, and Workspace Memory completed; baseline acceptance dataset can be created through workorchd APIs. |
| L12 | completed | Web can start LLM-driven execution with Goal/DoD/Soul/Skill/MCP/Memory context, inspect structured Decision context, and observe Step output including DoD assessment and Memory refs. |
| L13 | completed | Added repeatable local LLM acceptance scripts for memory and file backends with a local OpenAI-compatible mock provider; covers happy path, Decision resume, Approval rejection, schema failure, Memory recall, and file restart-read. |
| L14 | completed | Status, deployment, environment template, verification commands, provider config, and current LLM MVP boundaries are documented. |
| L15 | completed | LLM output now supports markdown/JSON-object normalization, one provider repair retry, strict `executionGraph` validation, graph Step output, and repair diagnostics for real provider validation. |
| L16 | completed | Runtime graph leaf completion now dispatches an LLM follow-up for DoD assessment, and shell sandbox creates missing Workspace working directories before command execution. |
| L17 | completed | `blocks/mcp` now registers `mcp/tool_call`, validates MCP server policy, invokes HTTP/HTTPS tool endpoints, and fails unsupported transports explicitly. |
| L18 | completed | Orchestrator now dispatches all currently ready graph nodes concurrently and advances the graph only after the batch completes. |
| L19 | completed | `blocks/mcp` now supports MCP `stdio` subprocess transport with JSON-RPC `initialize`, `notifications/initialized`, and `tools/call`; later advanced MCP resources, prompts, streaming, and session reuse are tracked as completed in `docs/advanced-runtime-scheduling-plan.md`. |
