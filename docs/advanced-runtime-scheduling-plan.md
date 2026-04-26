# workorch Advanced Runtime And Scheduling Plan

## Goal

This delivery round extends the already closed MVP with two formal capability
areas:

- advanced MCP interaction: resources, prompts, streaming evidence, and
  long-lived stdio sessions
- richer local multi-run scheduling beyond the current FIFO queue

The target remains a locally deployable `workorchd` service. Distributed
scheduling, external runtime marketplace, multi-tenant quota, and remote worker
fleet management are not included in this round.

## Scope

### A1. MCP Resources And Prompts

Status: completed

Target:

- Extend MCP server metadata with allowed prompts.
- Expose MCP resources and prompts from server configuration in read APIs.
- Add runtime step kinds for `mcp/resource_read` and `mcp/prompt_get`.
- Enforce allow-list policy for tools, resources, and prompts before dispatch.
- Support both HTTP/HTTPS MCP endpoint adapters and stdio JSON-RPC adapters.

Acceptance:

- Creating an MCP server can include `allowedTools`, `allowedResources`, and
  `allowedPrompts`.
- `GET /api/v1/workorch/mcp/servers/{serverId}/resources` returns configured
  resource refs.
- `GET /api/v1/workorch/mcp/servers/{serverId}/prompts` returns configured
  prompt refs.
- Runtime graph nodes using `mcp/resource_read` and `mcp/prompt_get` can
  complete and produce ToolCall evidence.

### A2. MCP Streaming Evidence

Status: completed

Target:

- Allow MCP runtime step input to request streaming collection.
- For HTTP/HTTPS endpoints, accept server-sent-event or newline JSON streams and
  persist collected stream events in Step output.
- For stdio JSON-RPC, collect JSON-RPC notifications observed before the final
  response as stream events.

Acceptance:

- Tool/resource/prompt output includes `streamEvents` when streaming evidence is
  emitted.
- Non-streaming requests keep the existing response shape.

### A3. MCP Long-Lived Stdio Session

Status: completed

Target:

- Add a per-server session policy.
- Keep existing per-call stdio process behavior as default.
- When `sessionPolicy` is `long_lived`, reuse one initialized stdio JSON-RPC
  process per MCP server inside the MCP block runtime adapter.
- Close long-lived sessions during block teardown.

Acceptance:

- Two calls to the same long-lived stdio server reuse one initialized process.
- A failed long-lived process is evicted and recreated on the next call.
- Existing transient stdio behavior remains valid by default.

### A4. Priority-Aware Local Run Scheduling

Status: completed

Target:

- Replace the HTTP execution FIFO wait queue with a local priority queue.
- Keep existing behavior as the default priority.
- Allow start/resume/activation requests to specify priority by query, header,
  or optional JSON body.
- Preserve `server.maxConcurrentRuns` enforcement and current cancellation
  semantics.

Acceptance:

- Higher-priority queued run starts before lower-priority queued run.
- Equal priority preserves FIFO order.
- Queue metrics continue to report waiting, total, and in-flight runs.

## Progress

- completed: `A1 MCP Resources And Prompts`
  - MCP server metadata now includes `allowedPrompts` and `sessionPolicy`.
  - MCP server APIs expose configured tools, resources, and prompts.
  - Runtime step kinds `mcp/tool_call`, `mcp/resource_read`, and
    `mcp/prompt_get` share the MCP runtime adapter with allow-list policy
    enforcement.
- completed: `A2 MCP Streaming Evidence`
  - HTTP MCP endpoints can return SSE or newline JSON stream events.
  - Stdio JSON-RPC notifications observed before the final response are exposed
    as `streamEvents`.
- completed: `A3 MCP Long-Lived Stdio Session`
  - `sessionPolicy: "long_lived"` reuses one initialized stdio process per MCP
    server inside the MCP runtime adapter.
  - MCP block teardown closes managed stdio sessions.
- completed: `A4 Priority-Aware Local Run Scheduling`
  - Local run execution waiters are selected by priority and FIFO sequence.
  - Activation supports body `priority`; start/resume support `priority` query
    and `X-Workorch-Run-Priority` header.
- completed: `A5 Web Console Runtime Management`
  - Web console adds a formal `Runtime Control` entry for MCP server capability
    management and local run scheduling observability.
  - The console can create MCP servers with tool/resource/prompt allow-lists,
    session policy, timeout, approval policy, and enabled state.
  - Workspace execution activation can specify run queue priority.
  - Run observability exposes MCP operation, server, target, and streaming
    evidence fields from Step output.
  - Removed stale Web slice API bindings from the console source.
- completed: `A6 Runtime Acceptance Gap Closure`
  - Run priority is now a first-class Run field persisted by workspace execution
    activation and exposed through Run responses and snapshots.
  - Run outcome metrics now count returned Run terminal state instead of merely
    counting successful HTTP dispatch requests.
  - LLM planning contract now explicitly forbids invented `workorch` shell CLI
    usage, and decision validation rejects shell commands that reference that
    unavailable runtime command.
  - LLM provider requests now include a formal `runtimeCapabilities` catalog
    for supported runtime block kinds, read-only local service APIs, and shell
    policy.
  - LLM decision validation rejects shell commands and execution graph shell
    nodes that call undeclared local `workorchd` HTTP endpoints such as `/runs`,
    `/observability`, or `/metrics?run=...`.

## Closure Status

- none in this delivery round
