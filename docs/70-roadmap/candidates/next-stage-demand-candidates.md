# workorch Next Stage Demand Candidates

## Purpose

This document keeps future demand candidates separate from completed MVP,
runtime expansion scope, and completed P1 delivery scope. Items here are not
active defects or remaining MVP work. Before implementation, each item must be
promoted into a formal delivery plan with target behavior, acceptance criteria,
API/Web impact, and verification commands.

## Current Promoted Requirement

- `Formal Business Result And Artifact Model`
- `Persistent Workorch Event Stream`

Active delivery plan:

- `../../90-archive/completed/result-event-development-plan.md`

## Completed P1 Delivery

- `C1 Global Long-Lived Runtime Module Registration`
- `C5 Vector Memory Search`
- `C6 LLM Provider Streaming And Non-Chat APIs`

## Candidate List

### C8. Formal Business Result And Artifact Model

Status: promoted

Target direction:

- Promote final Workspace / Run outcome from `dodAssessment.summary` into a
  first-class business Result.
- Attach generated deliverables, file outputs, external references, and
  evidence-backed outputs as formal Artifacts.
- Keep ToolCall, Patch, ValidationResult, RuntimeCheckpoint, Memory, and events
  as evidence/context rather than final business outcome.

Acceptance:

- Tracked in `../../90-archive/completed/result-event-development-plan.md`.

### C9. Persistent Workorch Event Stream

Status: promoted

Target direction:

- Persist formal Workorch business events emitted through the existing EventHub
  boundary.
- Support audit/replay queries by workspace, run, topic, subject, and cursor.
- Allow observability and Workspace detail to use persisted event timeline data
  without synthesizing missing events from repository state.

Acceptance:

- Tracked in `../../90-archive/completed/result-event-development-plan.md`.

### C2. Distributed Scheduling And External Worker Fleets

Target direction:

- Move beyond local priority-aware run scheduling toward remote workers,
  distributed queues, worker heartbeats, and retry policy.

Acceptance to define before development:

- Worker ownership model and Run state transition contract.
- Failure, timeout, retry, and cancellation semantics.
- Metrics and observability for queued, assigned, running, and terminal work.

### C3. Multi-Tenant Authorization And Quota

Target direction:

- Add tenant/project/user boundaries, authorization checks, and execution quota.

Acceptance to define before development:

- Request identity model.
- Resource ownership and visibility rules.
- Quota enforcement for Run scheduling and runtime tool usage.

### C4. External Runtime Marketplace

Target direction:

- Manage external runtime/tool adapters as governed installable capabilities.
- Keep adapter execution behind the existing runtime adapter contract and
  Decision/Approval gates.

Acceptance to define before development:

- Adapter metadata, trust policy, and enablement workflow.
- Runtime capability publication to LLM planning context.
- Safe failure behavior for missing, disabled, or rejected adapters.

### C7. Product Collaboration Surface

Target direction:

- Expand beyond single-operator local control into enterprise collaboration,
  knowledge assets, templates, and Sub-Agent orchestration.

Acceptance to define before development:

- Formal business models and ownership boundaries.
- Web control-plane workflow.
- Interaction with Workspace, Run, Decision, Approval, and Memory models.

### C10. LLM Context Budget Advanced Follow-ups

Status: deferred-candidate

Source: 2026-07-16 docs cleanup demoted
`90-archive/completed/llm-context-budget-todo.md` out of active. Baseline
bytes budget / timeout retry / CTX-01/CTX-02 already closed in code.

Target direction:

- Provider failover advanced policy (beyond single-target sealed ceiling).
- Real tokenizer / provider-native token usage precision across models.
- Broader multi-entry governance semantic normalization for context budget.
- Differentiated governance for `context-too-large` (vs generic failure).

Acceptance to define before development:

- Must not reopen sealed Runtime V2 provider/model ceiling authority.
- Keep provider-neutral C0–C5 cache plan; no map-based authority rebuild.
- Promote only with formal plan + verifier coverage before re-entering active.

Historical process note:

- `../../90-archive/completed/llm-context-budget-todo.md`
