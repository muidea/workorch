# workorch LLM Driven Workspace Execution Design

## Goal

This document defines the design for making `workorchd` drive a real Workspace
execution loop from `Goal` and `DoD` through an LLM-backed runtime.

The target is not to turn `workorchd` into a chat service. The target is to keep
`workorchd` as a governed asynchronous orchestration service where LLMs produce
structured execution decisions and the existing `Run`, `Step`, `ToolCall`,
`Patch`, `ValidationResult`, `RuntimeCheckpoint`, `Decision Ticket`, and
`Approval` chains remain the source of operational truth.

## Current Gap

The current implementation can create Workspace, define Goal and DoD, activate a
Run, execute runtime steps, persist evidence, block on Decision or Approval, and
expose observability.

The missing product capability is:

- `Goal` and `DoD` are not yet used as planning context for execution.
- The first runtime step is selected by a fixed step kind, not by model planning.
- The legacy runtime adapter returns deterministic payloads for verification.
- Run completion does not yet depend on model-generated DoD assessment.
- There is no LLM provider boundary, Soul selection, Skill loading, MCP tool
  discovery, or persistent Memory recall in the execution loop.

## Design Inputs

The following local projects provide useful implementation references, but
`workorch` should not directly couple to their runtime shape:

- `openclaw`: Soul, Memory, Session, Skill, and MCP product surface.
- `opencode`: Agent, permission, tool registry, session prompt, and LLM loop
  separation.
- `codex`: MCP protocol types, tool registry planning, Memory extraction jobs,
  agent jobs, and pre/post tool hooks.
- `hermes-agent`: `SOUL.md`, toolset grouping, Skill library, Memory tool, and
  long-running agent loop practice.
- `magicCodeAgent`: Go-side prompt registry, LLM provider, MCP server, and tool
  block implementation patterns.

These references should be translated into formal `workorch` business models
and module boundaries.

## Design Principles

- LLM output is advisory and structural. It must become a `Step` output before
  it can affect a Run.
- LLM must not directly mutate Workspace files, call shell, or call MCP tools.
- Orchestrator owns Run state transitions and evidence creation.
- Runtime adapters own external execution details.
- Soul, Skill, MCP, and Memory are first-class business context, not prompt text
  concatenation helpers.
- Missing LLM configuration must fail clearly. It must not silently fall back to
  `legacy/minimal` and report a false completion.
- Memory is durable execution context. It is not a replacement for evidence,
  checkpoint, audit, or knowledge asset records.

## MVP Scope Boundary

The first deliverable is a locally deployable `workorchd` service that can drive
one Workspace execution loop from Goal and DoD through a configured LLM provider.
It must be possible to verify the loop with local HTTP APIs and the embedded Web
console.

Included in the first LLM-driven MVP:

- Workspace execution activation can bind `Goal`, `DoD`, `Soul`, selected
  `Skill` refs, allowed `MCPServer` refs, and recalled `MemoryItem` refs.
- `llm/workspace_execution` becomes a formal runtime step kind.
- The LLM adapter builds planning context, calls the configured provider, parses
  a strict `LLMExecutionDecision`, and returns normalized Step output.
- Orchestrator maps the normalized output to Run state, next Step, Decision,
  Approval, Patch, ToolCall, ValidationResult, RuntimeCheckpoint, and Memory
  write requests.
- Persistent Memory supports create, list by Workspace, and deterministic search
  for subsequent Runs in the same Workspace.
- The embedded Web console exposes Workspace execution start, Decision Queue,
  Workspace Detail, Memory visibility, and LLM observability.

Excluded from the first LLM-driven MVP:

- Full vector Memory search.
- Multi-tenant authorization.
- Distributed execution.
- General external runtime marketplace.
- Allowing LLM providers to directly execute shell, write files, apply patches,
  or invoke MCP tools.
- Silent fallback from LLM-driven execution to `legacy/minimal`.

## Formal Boundary Contract

The implementation must preserve these ownership boundaries.

| Component | Owns | May Read | Must Not Do |
| --- | --- | --- | --- |
| `application/workorchd` | HTTP/Web contract, request validation, execution activation use cases | Workspace, Goal, DoD, Soul, Skill, MCP catalog, Memory, Run snapshots | Provider-specific LLM calls, MCP protocol calls, direct Run state mutation outside appservice contracts |
| `kernel/workspace` | Workspace lifecycle and workspace projection state | Run summary and formal workspace events through stable contracts | Dispatch runtime steps or mutate evidence directly |
| `kernel/run` | Run, Step, Patch, ToolCall, ValidationResult, RuntimeCheckpoint persistence | Workspace/Goal/DoD references needed for validation | Select LLM provider or execute external tools |
| `kernel/orchestrator` | Run state machine, Step dispatch, evidence mapping, Decision/Approval blocking | Runtime adapter registry, Run context, normalized Step output | Import provider clients, call MCP directly, concatenate prompt context |
| `kernel/decision` | Decision Ticket lifecycle and resolution payloads | Run and evidence refs | Execute follow-up steps directly |
| `kernel/soul` | Soul model, default selection, capability policy | Workspace tags, explicit activation selections | Grant executable permission by itself |
| `kernel/skill` | Skill metadata, trigger matching, selected Skill content | Soul allow-list, Workspace/Goal/DoD text | Execute tools or expand capability beyond policy |
| `kernel/memory` | Memory CRUD, Workspace-scoped search, source citation, memory write policy | Run evidence refs, Decision/Approval resolution, ValidationResult summary | Replace evidence, checkpoint, or artifact persistence |
| `kernel/mcp` | MCP server registry, tool/resource metadata, policy | Soul allow-list, activation selections | Invoke MCP tools directly from model output |
| `blocks/llm` | Provider request assembly, provider call, schema parsing, provider error normalization | Execution context assembled by application/orchestrator | Mutate repositories, execute shell, apply patches, invoke MCP |
| `blocks/mcp` | Approved MCP invocation and output normalization | Approved tool call request and server registry | Select tools from free-form LLM text or bypass Approval |
| Future `blocks/shell` | Approved Workspace-rooted shell execution | Step input and workspace directory | Run outside Workspace policy or decide business state |

Hard rules:

- Business state transitions remain in `kernel/orchestrator` and related kernel
  services.
- External side effects remain in block modules.
- Prompt construction may use business context, but prompt text is not business
  state.
- Evidence records are append-only business observations; Memory records are
  reusable planning context derived from cited sources.
- All module loading must stay explicit through the `workorchd` application
  entry and `internal/modules/...` plugin/module registration pattern.

## Interaction Contract

### Data Exchange Types

The LLM-driven loop uses three boundary payloads.

```text
WorkspaceExecutionContext
- workspace
- goal
- dod
- soul
- selectedSkills
- allowedMCPServers
- mcpToolCatalog
- recalledMemory
- recentEvidence
- executionPolicy
- runSnapshot
```

```text
LLMProviderRequest
- model
- messages
- responseSchema
- timeoutMillis
- redactionPolicy
- traceRefs
```

```text
NormalizedRuntimeOutput
- status
- stage
- nextStep
- nextKind
- nextInput
- patches
- toolCalls
- validationResults
- runtimeCheckpoints
- blockDecisionReason
- blockApprovalReason
- memoryWriteRequests
- llmDecision
```

Only `NormalizedRuntimeOutput` is consumed by `kernel/orchestrator`. The
provider request and raw provider response remain inside `blocks/llm`, except
for redacted diagnostic references stored as evidence or checkpoints.

### Activation Interaction

Execution activation is the boundary between business configuration and runtime
execution.

```text
HTTP/Web request
-> application/workorchd validates workspaceId, goalId, doDId, soulId,
   skillIds, and mcpServerIds
-> kernel/workspace loads Workspace
-> kernel/workspace or appservice loads active Goal and DoD
-> kernel/soul resolves explicit Soul or Workspace default Soul
-> kernel/skill resolves selected and policy-allowed Skills
-> kernel/mcp resolves selected and policy-allowed MCP servers and tool catalog
-> kernel/memory searches Workspace-scoped reusable context
-> kernel/run creates Run with execution context refs
-> kernel/orchestrator starts Run
-> kernel/orchestrator dispatches `llm/workspace_execution`
```

Failure rules:

- Missing Workspace, Goal, or DoD returns a client-visible validation error.
- Missing Soul returns a validation error unless a configured default Soul
  exists.
- Selected Skills or MCP servers not allowed by Soul/policy are rejected before
  Run creation.
- Missing or disabled LLM provider fails activation or the first LLM Step with a
  clear error. It must not create a completed Run through `legacy/minimal`.

### LLM Planning Interaction

```text
kernel/orchestrator
-> builds runtime dispatch request with execution context refs
-> blocks/llm loads context through provided service interfaces
-> blocks/llm assembles LLMProviderRequest
-> LLM provider returns structured response
-> blocks/llm validates response schema
-> blocks/llm returns NormalizedRuntimeOutput
-> kernel/orchestrator records Step result and applies state transition rules
```

The LLM may ask for action, Decision, Approval, Memory write, or DoD assessment.
It does not get direct access to repository handles, shell executors, patch
engines, or MCP clients.

### Action Interaction

Action requests always move through Orchestrator and block adapters.

```text
LLMExecutionDecision.shellCommands
-> NormalizedRuntimeOutput.nextKind = shell/command
-> kernel/orchestrator dispatches shell Step
-> shell block executes inside Workspace root
-> kernel/orchestrator records ToolCall and ValidationResult evidence
```

```text
LLMExecutionDecision.patchRequests
-> NormalizedRuntimeOutput.patches
-> kernel/orchestrator validates and applies patch engine policy
-> kernel/run records Patch evidence
-> kernel/orchestrator dispatches follow-up validation Step when requested
```

```text
LLMExecutionDecision.mcpToolCalls
-> NormalizedRuntimeOutput.nextKind = mcp/tool_call
-> kernel/orchestrator verifies policy and Approval need
-> blocks/mcp invokes approved MCP tool
-> kernel/run records ToolCall evidence
```

### Human Governance Interaction

Decision and Approval are separate contracts.

```text
LLMExecutionDecision.requiredDecision
-> NormalizedRuntimeOutput.blockDecisionReason
-> kernel/orchestrator creates Decision Ticket
-> Run enters blocked_on_decision
-> Web Decision Queue shows context, choices, and evidence refs
-> user resolves Decision Ticket
-> ResumeRun appends resolution context
-> next `llm/workspace_execution` sees resolution in recentEvidence
```

```text
LLMExecutionDecision.requiredApproval
-> NormalizedRuntimeOutput.blockApprovalReason
-> kernel/orchestrator creates Approval
-> Run enters blocked_on_approval
-> Web Decision Queue shows requested action, risk, and evidence refs
-> user approves or rejects Approval
-> ResumeRun continues or fails according to Approval result
```

Decision resolution types required by the business loop:

```text
replan
reject_current_path
provide_missing_info
```

Approval resolution types required by the business loop:

```text
approved
rejected
```

### Memory Interaction

Memory is read before planning and written after meaningful evidence exists.

```text
Before LLM planning
-> kernel/memory searches by workspaceId, goal tags, DoD tags, memoryType,
   source quality, and keyword relevance
-> blocks/llm receives selected Memory summaries with source refs
```

```text
After evidence exists
-> kernel/orchestrator or appservice proposes Memory write requests from
   Decision, Approval, ValidationResult, Patch, ToolCall, or Run summary
-> kernel/memory validates source refs and visibility
-> kernel/memory persists MemoryItem
-> future Runs can recall the MemoryItem
```

Rules:

- LLM-proposed `memoryWrites` are requests, not final records.
- Memory without source refs is allowed only for explicit user-provided context.
- RuntimeCheckpoint is used for recovery and observability; Memory is used for
  future planning.

### Web And API Interaction

The embedded Web console must interact only through HTTP APIs.

```text
Workspace Board
-> list Workspace projections and current Run state
-> start Workspace execution with Goal, DoD, Soul, Skills, and MCP selections
```

```text
Workspace Detail
-> show Goal, DoD, selected Soul, selected Skills, allowed MCP servers,
   recalled Memory, Runs, Steps, and evidence
```

```text
Decision Queue
-> show Decision Tickets and Approvals with reason, requested action, choices,
   risk, and evidence refs
-> submit Decision or Approval resolution
```

```text
Runtime Observability
-> show LLM planning Step, redacted decision summary, next action, DoD
   assessment, ToolCall/Patch/ValidationResult refs, Memory writes, and errors
```

Web must not expose raw provider secrets, raw prompt logs containing secrets, or
direct low-level adapter execution controls as primary user actions.

## Business Objects

### Soul

`Soul` defines execution stance, policy, and capability boundaries for a
Workspace execution.

Suggested fields:

```text
Soul
- id
- name
- description
- businessDomains
- systemInstruction
- planningPolicy
- decisionPolicy
- approvalPolicy
- validationPolicy
- allowedSkillRefs
- allowedMCPServerRefs
- allowedToolKinds
- outputContract
- createdAt
- updatedAt
```

Boundary:

- Soul may influence how the LLM reasons and which capabilities are allowed.
- Soul must not be treated as executable permission by itself.
- Final tool permission still belongs to policy and approval checks.

### Skill

`Skill` is reusable execution knowledge. It can provide instructions,
references, examples, and constraints.

Suggested fields:

```text
Skill
- id
- name
- version
- description
- triggerRules
- instruction
- referenceRefs
- requiredToolKinds
- requiredMCPServerRefs
- constraints
- createdAt
- updatedAt
```

Boundary:

- Skill is loaded into planning context.
- Skill does not execute tools directly.
- Skill cannot expand permissions beyond Soul and policy.
- Skill usage should be recorded in Run evidence or Memory when it affects the
  execution path.

### MCP Server

`MCPServer` represents an external tool or resource provider.

Suggested fields:

```text
MCPServer
- id
- name
- transport
- endpoint
- enabled
- allowedTools
- allowedResources
- defaultApprovalPolicy
- timeoutMillis
- createdAt
- updatedAt
```

Boundary:

- `kernel/mcp` owns registry, discovery metadata, and policy.
- `blocks/mcp` owns concrete invocation and result normalization.
- LLM may request an MCP call by structured action only.
- MCP call result must be recorded as `ToolCall` evidence.

### Persistent Memory

`Memory` stores durable context for future planning. It is designed for recall,
not for raw execution replay.

Suggested fields:

```text
MemoryItem
- id
- workspaceId
- runId
- sourceType
- sourceRef
- memoryType
- content
- summary
- tags
- importance
- confidence
- visibility
- createdAt
- updatedAt
```

Suggested `memoryType` values:

```text
decision
execution_lesson
project_context
dod_feedback
user_preference
skill_feedback
tool_feedback
run_summary
```

Boundary:

- `RuntimeCheckpoint` records execution state for recovery and observability.
- `ToolCall`, `Patch`, and `ValidationResult` record evidence.
- `MemoryItem` records reusable context for future planning.
- `KnowledgeAsset` should later cover documents, templates, design outputs, and
  durable project artifacts.
- Memory must cite its source object when derived from a Run, Decision,
  Approval, ValidationResult, Patch, or ToolCall.

### LLM Execution Decision

LLM output must be parsed into a strict contract.

```text
LLMExecutionDecision
- thoughtSummary
- plan
- nextAction
- requiredDecision
- requiredApproval
- patchRequests
- mcpToolCalls
- shellCommands
- memoryWrites
- dodAssessment
- completionState
```

Allowed `completionState` values:

```text
continue
blocked_on_decision
blocked_on_approval
completed
failed
```

Boundary:

- Free-form model text is not accepted as a Run transition input.
- Invalid output fails the Step with a validation result.
- The Orchestrator maps the parsed decision into existing output keys such as
  `nextStep`, `nextKind`, `patches`, `blockDecisionReason`, and
  `blockApprovalReason`.

## Module Boundaries

Planned formal module layout:

```text
internal/modules/kernel/soul
internal/modules/kernel/skill
internal/modules/kernel/memory
internal/modules/kernel/mcp
internal/modules/blocks/llm
internal/modules/blocks/mcp
```

### `kernel/soul`

Responsibilities:

- Store and query Soul definitions.
- Select default Soul for Workspace execution.
- Validate that selected Soul allows requested Skills and MCP servers.

Not responsible for:

- Calling LLM providers.
- Executing tools.
- Mutating Run state.

### `kernel/skill`

Responsibilities:

- Store and query Skill metadata.
- Resolve candidate Skills from Goal, DoD, Workspace tags, and Soul policy.
- Provide selected Skill content to prompt assembly.

Not responsible for:

- Tool execution.
- MCP invocation.
- Approval bypass.

### `kernel/memory`

Responsibilities:

- Persist Memory items.
- Search relevant Memory for a Workspace and Goal.
- Record Memory derived from Decision, Approval, Validation, and Run summary.

Not responsible for:

- Evidence persistence.
- Runtime recovery.
- Full document or artifact management.

### `kernel/mcp`

Responsibilities:

- Store MCP server definitions.
- Discover tool and resource metadata.
- Apply MCP server and tool policy.

Not responsible for:

- Direct model prompting.
- Direct Run state transition.

### `blocks/llm`

Responsibilities:

- Build provider request from Workspace execution context.
- Call configured LLM provider.
- Parse and validate `LLMExecutionDecision`.
- Normalize provider errors into Step failure output.

Not responsible for:

- Shell execution.
- File patch application.
- MCP tool execution.
- Run state transition.

### `blocks/mcp`

Responsibilities:

- Execute approved MCP tool calls.
- Read approved MCP resources.
- Normalize MCP output into `ToolCall` evidence payloads.

Not responsible for:

- Tool selection by model.
- Soul or Skill selection.
- Persistent Memory policy.

### `kernel/orchestrator`

Responsibilities remain:

- Start, resume, validate, complete, fail, cancel, or supersede Run.
- Dispatch Step through registered runtime adapters.
- Create `ToolCall`, `Patch`, `ValidationResult`, and `RuntimeCheckpoint`.
- Create `Decision Ticket` or `Approval` when runtime output requests it.

New boundary:

- Orchestrator may dispatch `llm/workspace_execution` as a Step kind.
- Orchestrator must not import or depend on concrete provider clients.
- Orchestrator consumes only normalized Step output.

### `application/workorchd`

Responsibilities:

- Bind Workspace execution activation to selected Goal, DoD, Soul, Skills, MCP
  servers, and Memory recall.
- Expose HTTP APIs and Web views for configuring those objects.
- Keep the service deployable with memory and file repository backends.

Not responsible for:

- Provider-specific LLM behavior.
- MCP protocol implementation.
- Business model persistence internals.

## Interaction Flow

### Workspace Execution Activation

```text
POST /api/v1/workorch/workspaces/{workspaceId}/execution-activations
-> appservice loads Workspace
-> appservice selects Goal and DoD
-> appservice selects Soul
-> appservice resolves allowed Skills
-> appservice searches relevant Memory
-> appservice discovers allowed MCP tools/resources
-> appservice creates Run
-> RunService starts Run
-> Orchestrator dispatches llm/workspace_execution step
```

The first LLM step input should include:

```text
workspace
goal
dod
soul
skills
memory
mcpToolCatalog
recentEvidence
executionPolicy
```

### LLM Planning Step

```text
Orchestrator
-> RuntimeAdapterRegistry
-> blocks/llm adapter
-> provider request
-> LLMExecutionDecision
-> normalized Step output
-> Orchestrator applies output
```

The LLM adapter output should never directly update repositories. It returns a
structured Step output only.

### Tool Execution

If LLM requests shell:

```text
LLMExecutionDecision.shellCommands
-> Step output nextKind=shell/command
-> Orchestrator dispatches shell step
-> blocks/shell executes command
-> ToolCall and ValidationResult evidence recorded
```

If LLM requests patch:

```text
LLMExecutionDecision.patchRequests
-> Step output patches
-> Orchestrator applyRuntimePatches
-> Patch evidence recorded
```

If LLM requests MCP:

```text
LLMExecutionDecision.mcpToolCalls
-> Step output nextKind=mcp/tool_call
-> blocks/mcp invokes MCP tool
-> ToolCall evidence recorded
```

### Decision and Approval

If LLM needs human input:

```text
LLMExecutionDecision.requiredDecision
-> Step output blockDecisionReason
-> Orchestrator creates Decision Ticket
-> Run enters blocked_on_decision
```

If LLM needs permission for risky action:

```text
LLMExecutionDecision.requiredApproval
-> Step output blockApprovalReason
-> Orchestrator creates Approval
-> Run enters blocked_on_approval
```

Resume remains unchanged:

```text
Decision/Approval resolved
-> ResumeRun
-> Orchestrator dispatches next step
-> LLM receives updated evidence and resolution context
```

### Memory Read and Write

Read timing:

```text
Before LLM planning
-> search same Workspace Memory
-> filter by Goal/DoD tags and source quality
-> include top relevant Memory in LLM context
```

Write timing:

```text
Decision resolved -> decision/user_preference memory
Validation failed -> dod_feedback/tool_feedback memory
Run completed -> run_summary/execution_lesson memory
Run failed -> execution_lesson memory
Skill used -> skill_feedback memory when material
User adds context -> project_context/user_preference memory
```

Memory write must be explicit. The LLM may propose `memoryWrites`, but
`kernel/memory` decides whether to persist them based on policy and source.

## Runtime Output Contract

The LLM adapter should translate `LLMExecutionDecision` into the existing
orchestrator output contract.

Examples:

Continue with shell:

```json
{
  "status": "completed",
  "stage": "llm_plan",
  "nextStep": "run-1/shell-1",
  "nextKind": "shell/command",
  "nextInput": {
    "command": "go",
    "args": ["test", "./..."]
  },
  "llmDecision": {
    "completionState": "continue",
    "thoughtSummary": "Need to validate the service before claiming completion."
  }
}
```

Apply patch:

```json
{
  "status": "completed",
  "stage": "llm_patch_plan",
  "patches": [
    {
      "patchType": "unified_diff",
      "targetRef": "README.md",
      "diff": "--- README.md\n+++ README.md\n@@ ...",
      "summary": "document local verification command"
    }
  ],
  "nextStep": "run-1/validate-1",
  "nextKind": "shell/command"
}
```

Block on decision:

```json
{
  "status": "completed",
  "stage": "llm_decision_required",
  "blockDecisionReason": "Two implementation paths affect public API shape.",
  "blockDecisionTicketId": "ticket-run-1-api-shape",
  "nextStep": "run-1/continue-after-decision",
  "nextKind": "llm/workspace_execution"
}
```

Complete:

```json
{
  "status": "completed",
  "stage": "llm_dod_assessment",
  "llmDecision": {
    "completionState": "completed",
    "dodAssessment": {
      "satisfied": true,
      "items": [
        {
          "item": "service deploys locally",
          "state": "satisfied",
          "evidenceRefs": ["validation-result-run-1"]
        }
      ]
    }
  },
  "memoryWrites": [
    {
      "memoryType": "run_summary",
      "summary": "Local deployment verified through workorchd HTTP checks."
    }
  ]
}
```

## Provider Configuration

Suggested environment contract for local MVP:

```text
WORKORCH_LLM_PROVIDER=openai|compatible
WORKORCH_LLM_BASE_URL=
WORKORCH_LLM_API_KEY=
WORKORCH_LLM_MODEL=
WORKORCH_LLM_TIMEOUT=
WORKORCH_LLM_DISABLED=false
```

Provider rules:

- Empty or disabled provider makes `llm/workspace_execution` unavailable.
- Unavailable LLM must fail with a clear error.
- Provider errors must become failed Step evidence.
- Secrets must not be returned in HTTP responses, ToolCall output, logs, Memory,
  or RuntimeCheckpoint payloads.

## API Surface

Initial API candidates:

```text
GET  /api/v1/workorch/souls
POST /api/v1/workorch/souls
GET  /api/v1/workorch/souls/{soulId}

GET  /api/v1/workorch/skills
POST /api/v1/workorch/skills
GET  /api/v1/workorch/skills/{skillId}

GET  /api/v1/workorch/memory?workspaceId={workspaceId}
POST /api/v1/workorch/workspaces/{workspaceId}/memory

GET  /api/v1/workorch/mcp/servers
POST /api/v1/workorch/mcp/servers
GET  /api/v1/workorch/mcp/servers/{serverId}/tools
```

Execution activation should accept optional context selection:

```json
{
  "goalId": "goal-1",
  "doDId": "dod-1",
  "soulId": "soul-engineering-delivery",
  "skillIds": ["go-multi-module-dev"],
  "mcpServerIds": ["repo-context"]
}
```

## Web Interaction

The embedded console should evolve without exposing low-level runtime internals
as primary navigation.

Required views:

- Workspace Console: select Soul, selected Skills, and allowed MCP servers when
  starting execution.
- Workspace Detail: show Memory used by the selected Run.
- Decision Queue: show LLM-generated decision context and referenced evidence.
- Runtime Observability: show LLM planning step, tool actions, DoD assessment,
  and Memory writes.

Optional administration views:

- Soul Management.
- Skill Management.
- MCP Server Management.
- Workspace Memory.

## Persistence

Memory and file repository backends should support all new models:

```text
soul.json
skill.json
memory_item.json
mcp_server.json
```

Repository contracts should include:

```text
Create
Get
Update
List
ListByWorkspace where applicable
Search where applicable
```

For Memory, first implementation may use deterministic keyword and tag search.
Vector search is explicitly a later enhancement.

## Security and Governance

- Soul defines allowed capability scope.
- Skill cannot grant permission.
- MCP server and tool policy gates external calls.
- Shell and patch remain Workspace-rooted.
- Risky actions require Approval according to policy.
- Every external action becomes `ToolCall` evidence.
- Every file mutation becomes `Patch` evidence.
- Model output must be schema-validated before use.
- Secrets must be redacted from prompt logs, Memory, and evidence.

## Acceptance Data Interaction Flow

The LLM-driven MVP must be verified through deterministic data interaction
flows, not only by checking whether a Run eventually reaches a terminal state.
Each acceptance flow must prove input binding, state transition, evidence
creation, projection visibility, and persistence.

### Baseline Acceptance Dataset

Use a small fixed dataset so repeated local verification can compare the same
objects and transitions.

```text
Workspace
- title: "local llm execution acceptance"
- description: "verify Goal and DoD driven workorch execution"

Goal
- text: "create a local verification artifact and prove it satisfies DoD"

DoD
- items:
  - "LLM planning step is recorded"
  - "at least one concrete action is executed through a block adapter"
  - "DoD assessment references evidence"
  - "run summary is persisted as Memory"

Soul
- id: "soul-engineering-delivery"
- allowedSkillRefs: ["skill-local-go-service"]
- allowedMCPServerRefs: ["mcp-local-context"]
- allowedToolKinds: ["shell/command", "patch/apply", "mcp/tool_call"]

Skill
- id: "skill-local-go-service"
- instruction: "Prefer small verifiable steps and cite evidence before completion."

Memory
- memoryType: "project_context"
- summary: "This workspace is verified through local workorchd HTTP APIs."

MCPServer
- id: "mcp-local-context"
- enabled: true
- allowedTools: ["repo.summary"]
```

### Flow A: Happy Path To Completion

Purpose: prove Goal, DoD, Soul, Skill, MCP catalog, and Memory are bound into a
Run and result in evidence-backed completion.

```text
1. Create Workspace
   -> expect Workspace has generated id, work directory, description, and state

2. Create Goal and DoD under Workspace
   -> expect Goal and DoD list endpoints return the newly created records

3. Create Soul, Skill, MCPServer, and initial Memory
   -> expect each record can be read back through its management endpoint

4. Start execution activation with goalId, doDId, soulId, skillIds, mcpServerIds
   -> expect Run is created and first Step kind is llm/workspace_execution

5. Inspect Run snapshot
   -> expect snapshot references Workspace, Goal, DoD, Soul, Skills, MCP servers,
      recalled Memory, and recent evidence

6. LLM returns completionState=continue with a concrete next action
   -> expect Step completed, RuntimeCheckpoint recorded, next Step dispatched

7. Concrete action executes through shell, patch, or MCP block
   -> expect ToolCall or Patch evidence exists and is linked to the Run

8. LLM returns DoD assessment with evidenceRefs
   -> expect Run transitions to completed only when every satisfied DoD item has
      at least one evidence reference

9. Memory write request is accepted
   -> expect run_summary or execution_lesson MemoryItem references the completed
      Run or its evidence

10. Restart service with file repository backend
    -> expect Workspace, Run, Step, evidence, and Memory can still be read
```

Minimum observable API checks:

```text
GET /api/v1/workorch/workspaces/{workspaceId}/snapshot
GET /api/v1/workorch/runs/{runId}/snapshot
GET /api/v1/workorch/runs/{runId}/status
GET /api/v1/workorch/memory?workspaceId={workspaceId}
```

### Flow B: Decision Blocking And Resume

Purpose: prove LLM requests for missing business information are governed by
Decision Ticket and do not mutate execution state implicitly.

```text
1. Start execution with a Goal that intentionally misses a required detail
   -> LLM returns requiredDecision

2. Orchestrator creates Decision Ticket
   -> expect Run state is blocked_on_decision
   -> expect ticket has resolutionTypes including replan, reject_current_path,
      and provide_missing_info

3. Resolve Decision Ticket with provide_missing_info
   -> expect ticket state is resolved and resolution payload is persisted

4. Resume Run
   -> expect next llm/workspace_execution input includes Decision resolution as
      recent evidence

5. Continue to concrete action or completion
   -> expect no duplicate unresolved Decision Ticket remains for the same block
```

### Flow C: Approval Blocking And Rejection

Purpose: prove risky actions require explicit Approval and rejection results in
a stable terminal path.

```text
1. LLM requests an action that violates auto-approval policy
   -> Orchestrator creates Approval
   -> expect Run state is blocked_on_approval

2. Reject Approval
   -> expect Approval state is rejected

3. Resume Run
   -> expect Run fails or replans according to policy, but the rejected action
      is not executed

4. Inspect evidence
   -> expect no ToolCall or Patch exists for the rejected action
```

### Flow D: Provider And Schema Failure

Purpose: prove LLM integration fails safely and observably.

```text
1. Disable or omit provider configuration
   -> start LLM-driven execution
   -> expect activation or first Step fails with a clear provider-unavailable
      error
   -> expect Run does not complete through legacy/minimal

2. Configure provider to return invalid JSON or schema-invalid fields
   -> expect llm/workspace_execution Step fails with schema validation details
   -> expect ValidationResult or RuntimeCheckpoint records the failure
   -> expect no shell, patch, or MCP action is dispatched
```

### Flow E: Memory Recall In Second Run

Purpose: prove Memory is durable planning context and not only a write-only
record.

```text
1. Complete first Run and persist run_summary Memory
   -> expect MemoryItem sourceRef points to Run or evidence

2. Start second Run in the same Workspace
   -> expect llm/workspace_execution context includes relevant MemoryItem ids

3. Inspect Run snapshot and LLM planning diagnostics
   -> expect recalled Memory is visible by id and summary, with source refs
```

### Stability Invariants

Every acceptance flow must satisfy these invariants:

- A Run cannot complete without a DoD assessment.
- A satisfied DoD item must reference at least one evidence object.
- A failed or schema-invalid LLM Step must not dispatch external actions.
- Decision and Approval blocks must be visible through both API snapshots and
  Web Decision Queue.
- Memory writes derived from execution must cite a source object.
- Provider secrets must never appear in snapshots, evidence payloads, Memory,
  RuntimeCheckpoint, Web payloads, or logs intended for users.
- Re-running the same flow with file repository backend after restart must
  preserve the business objects and their relationships.

### Web Verification

The Web console acceptance path should mirror the API flow:

```text
Workspace Board
-> create/select Workspace
-> start execution with Goal, DoD, Soul, Skills, and MCP selections

Workspace Detail
-> verify active Run, selected context, recalled Memory, Steps, and evidence

Decision Queue
-> resolve Decision or Approval and verify Run state updates

Runtime Observability
-> verify LLM planning Step, normalized decision, action evidence, DoD
   assessment, and Memory writes
```

Web verification is not a replacement for API verification. It proves the
management surface can observe and operate the same business flow without
calling lower-level adapters directly.

## MVP Acceptance

LLM-driven execution is acceptable when:

- A Workspace can bind Goal, DoD, Soul, selected Skills, allowed MCP servers,
  and relevant Memory.
- Starting Workspace execution dispatches an `llm/workspace_execution` Step.
- The LLM planning context includes Goal text, DoD items, Soul instruction,
  selected Skill content, relevant Memory, and MCP tool catalog.
- The LLM returns a schema-validated `LLMExecutionDecision`.
- The Run can continue by dispatching at least one concrete action through an
  existing adapter such as `shell/command` or patch application.
- The Run can block on Decision and resume with resolution context.
- The Run can block on Approval and resume after approval.
- The Run can complete only after a DoD assessment references evidence.
- Meaningful execution summary is persisted as Memory.
- A second Run in the same Workspace can recall prior Memory.
- Memory and file backends both preserve LLM execution records after restart.
- If LLM provider is not configured, LLM-driven activation fails clearly and
  does not complete through `legacy/minimal`.

## Non-Goals

- Full vector search for Memory.
- Multi-tenant authorization.
- Distributed scheduling.
- Full external runtime marketplace.
- Replacing existing ToolCall, Patch, ValidationResult, or RuntimeCheckpoint
  evidence with Memory.
- Allowing LLM to execute shell, patch, or MCP calls directly.

## Implementation Order

Recommended order:

1. Add formal model and repository contracts for Soul and Memory.
2. Add LLM provider configuration and `blocks/llm` adapter skeleton.
3. Add `llm/workspace_execution` Step kind with strict output schema parsing.
4. Add prompt assembly from Workspace, Goal, DoD, Soul, Skills, Memory, and MCP
   catalog.
5. Add Memory write path for Run summary and Decision resolution.
6. Add MCP registry and later `blocks/mcp` invocation.
7. Add Web controls for Soul selection, Memory visibility, and LLM execution
   observability.

This order keeps the business loop valid before expanding external tool reach.
