# workorch Next Stage Demand Candidates

## Purpose

This document keeps future demand candidates separate from completed MVP,
runtime expansion scope, and completed P1 delivery scope. Items here are not
active defects or remaining MVP work. Before implementation, each item must be
promoted into a formal delivery plan with target behavior, acceptance criteria,
API/Web impact, and verification commands.

## Current Promoted Requirement

- none

## Completed P1 Delivery

- `C1 Global Long-Lived Runtime Module Registration`
- `C5 Vector Memory Search`
- `C6 LLM Provider Streaming And Non-Chat APIs`

## Candidate List

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
