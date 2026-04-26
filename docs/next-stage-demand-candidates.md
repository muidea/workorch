# workorch Next Stage Demand Candidates

## Purpose

This document keeps future demand candidates separate from completed MVP,
runtime expansion scope, and the active P1 development plan. Items here are not
active defects or remaining MVP work. Before implementation, each item must be
promoted into a formal delivery plan with target behavior, acceptance criteria,
API/Web impact, and verification commands.

## Current Active Requirement

- active P1 plan: `docs/next-stage-p1-development-plan.md`

## Promoted To Active P1 Plan

- `C1 Global Long-Lived Runtime Module Registration`
- `C5 Vector Memory Search`
- `C6 LLM Provider Streaming And Non-Chat APIs`

## Candidate List

### C1. Global Long-Lived Runtime Module Registration

Status: promoted to `docs/next-stage-p1-development-plan.md`

Target direction:

- Evaluate whether long-lived workorch runtime units should use global
  `magicCommon/framework/plugin/module.Register` registration rather than the
  current explicit application-selected loading path.
- Keep `application/workorchd` in control of which modules are loaded for this
  service.

Acceptance to define before development:

- Clear module startup, teardown, and failure behavior.
- No regression in request-scoped runtime session behavior.
- Existing `make verify-real-mvp` remains green.

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

### C5. Vector Memory Search

Status: promoted to `docs/next-stage-p1-development-plan.md`

Target direction:

- Add semantic Memory retrieval while preserving existing deterministic
  workspace-scoped Memory list/search behavior.

Acceptance to define before development:

- Embedding provider configuration and failure behavior.
- Index build/update strategy.
- Snapshot and observability references for retrieved Memory.

### C6. LLM Provider Streaming And Non-Chat APIs

Status: promoted to `docs/next-stage-p1-development-plan.md`

Target direction:

- Extend the LLM provider contract beyond synchronous
  OpenAI-compatible `/chat/completions`.

Acceptance to define before development:

- Streaming event schema and persistence.
- Provider capability detection or explicit configuration.
- Backward-compatible behavior for current synchronous providers.

### C7. Product Collaboration Surface

Target direction:

- Expand beyond single-operator local control into enterprise collaboration,
  knowledge assets, templates, and Sub-Agent orchestration.

Acceptance to define before development:

- Formal business models and ownership boundaries.
- Web control-plane workflow.
- Interaction with Workspace, Run, Decision, Approval, and Memory models.
