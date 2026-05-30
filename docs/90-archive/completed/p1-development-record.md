# workorch Completed P1 Development Record

## Goal

This document records the completed P1 demand candidates delivered after the
completed MVP, LLM-driven execution, runtime
expansion, advanced runtime scheduling, and decision/approval batch governance
work.

The target remains a locally deployable `workorchd` service first. The P1 scope
must improve runtime context quality, provider interaction capability, and
long-lived runtime lifecycle control without introducing distributed worker
fleet complexity.

## Scope

### P1-1. LLM Provider Streaming And Non-Chat APIs

Status: completed

Target:

- Extend the LLM provider contract beyond synchronous OpenAI-compatible
  `/chat/completions` without breaking existing providers.
- Support provider-declared capabilities for streaming and optional non-chat
  operations.
- Persist streamed provider events as formal Step output and observability
  timeline data.
- Keep the final `LLMExecutionDecision` schema strict. Streaming may expose
  incremental text/events, but Run state changes still depend on a validated
  final decision object.

Acceptance:

- Existing synchronous provider verification remains green.
- A streaming-capable provider can emit incremental events that are visible in
  Run observability.
- Streaming failure before a valid final decision fails the Step clearly.
- Non-chat API support is capability-gated and cannot be selected implicitly by
  the orchestrator.

### P1-2. Vector Memory Search

Status: completed

Target:

- Add semantic Memory retrieval while preserving the current deterministic
  workspace-scoped Memory list/search behavior.
- Introduce explicit embedding provider configuration and failure semantics.
- Store and refresh Memory vectors without replacing existing Memory records.
- Expose retrieval evidence so Run snapshots and observability can show which
  Memory items influenced planning context.

Acceptance:

- Existing Memory create/list/search APIs continue to work without embeddings.
- When embedding configuration is available, Workspace execution can retrieve
  semantically relevant Memory items.
- Missing or failing embedding provider degrades explicitly according to policy;
  it must not silently invent Memory context.
- File backend restart preserves Memory vector metadata or rebuilds it through a
  documented deterministic path.

### P1-3. Global Long-Lived Runtime Module Registration

Status: completed

Target:

- Evaluate and implement a formal lifecycle path for long-lived workorch runtime
  units using `magicCommon/framework/plugin/module.Register` where appropriate.
- Keep `application/workorch` as the explicit owner of which modules are loaded
  for this service.
- Avoid duplicating request-scoped runtime session behavior; long-lived runtime
  units must have clear ownership, startup, teardown, health, and failure
  semantics.

Acceptance:

- Long-lived runtime units have deterministic startup and teardown order.
- Startup failure is visible through readiness and does not panic later in
  request handling.
- Request-scoped runtime sessions continue to work for existing APIs.
- `make verify-real-mvp` remains green after the lifecycle adjustment.

## Delivery Order

1. `P1-3 Global Long-Lived Runtime Module Registration`
2. `P1-1 LLM Provider Streaming And Non-Chat APIs`
3. `P1-2 Vector Memory Search`

The order is intentional. Runtime lifecycle boundaries should be settled before
adding longer-lived provider streams or vector index services. Provider
streaming then establishes the event/observability pattern that vector Memory
retrieval can reuse for evidence exposure.

## Non-Goals For This P1 Scope

- Distributed scheduling, external worker fleets, and remote execution workers.
- Multi-tenant authorization and quota.
- External runtime/tool adapter marketplace.
- Enterprise collaboration, templates, knowledge asset marketplace, and
  Sub-Agent orchestration.

## Progress

- completed: `P1-1 LLM Provider Streaming And Non-Chat APIs`
  Implementation completed: LLM provider configuration now exposes
  explicit `chatCompletions`, `streaming`, and `nonChatOperations`
  capabilities through config, API, and Web Console editing. `blocks/llm`
  uses streaming only when the selected provider declares it, persists streamed
  deltas into Step output and Run observability timeline as `streamEvents` /
  `step_stream` items, fails pre-decision stream errors clearly, and rejects
  undeclared non-chat operations instead of implicitly selecting them.
- completed: `P1-2 Vector Memory Search`
  First implementation slice completed: Memory now has formal embedding
  metadata and vector fields, memory/file repositories clone and persist vector
  data, repository search supports vector similarity ordering with `MinScore`,
  and API/Web responses expose `vectorReady` and `vectorDimension` without
  replacing existing deterministic workspace-scoped Memory list/search.
  Second implementation slice completed: LLM configuration now has an explicit
  `defaultEmbeddingProvider`, Web Console can manage it alongside provider
  capabilities, `blocks/llm` implements capability-gated OpenAI-compatible
  `/embeddings`, and workspace execution activation performs semantic Memory
  recall when an embedding provider is configured. Run execution context now
  records `memoryRetrieval` evidence including provider, operation, query hash,
  vector dimension, selected Memory IDs, scores, and explicit degradation
  reasons when semantic recall cannot run.
  Closure slice completed: `scripts/verify-real-vector-memory-recall.sh` and
  `make verify-real-vector-memory` provide a repeatable deployed-service
  verification path that temporarily registers a capability-gated embedding
  provider, verifies `/embeddings`-driven vector Memory recall, checks
  `memoryRetrieval` evidence, and restores the original LLM configuration.
- completed: `P1-3 Global Long-Lived Runtime Module Registration`
  First implementation slice completed: `workorchd` HTTP state now prefers a
  non-owning `RuntimeSession` built from the application-level
  `workorch/runtime` initiator exports. Direct test/tool construction still
  falls back to request-scoped runtime sessions when no application initiator is
  available. The lifecycle path is now covered by a server test that starts the
  registered runtime initiator and modules through magicCommon plugin lifecycle,
  proves HTTP state uses the application runtime, and verifies HTTP state close
  does not teardown the long-lived runtime.

## Verification Baseline

Each P1 item must keep these baseline checks green unless its plan explicitly
defines a replacement:

```bash
go test ./... -count 1
npm --prefix web/workorch-console run build
make build
make verify-real-mvp
```

`make verify-real-mvp` now includes real LLM execution, MCP stdio graph
execution, and deployed vector Memory recall verification.
