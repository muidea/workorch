# S11.3 Approval And Decision E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [S1.6 TUI/REST Command Path Parity](./goal-dod-nextaction-s1-6-command-path-parity.md)
- [S5.2 Pending Approval NextAction](./goal-dod-nextaction-s5-2-pending-approval-nextaction.md)
- [S5.3 Open Decision NextAction](./goal-dod-nextaction-s5-3-open-decision-nextaction.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)
- [S11.2 Dual Workspace Resume/Replan E2E](./goal-dod-nextaction-s11-2-dual-workspace-resume-replan-e2e.md)

## Contract Audit

- Scope: deployed or managed WorkOrch service resolving runtime decision and approval governance objects, then continuing execution through the workorchd scheduler owner path.
- Observed state: server unit tests cover decision resolve, decision replan command projection, approval approve/reject, conflict handling, and governance continuation scheduling. Container verification covers decision and approval continuation in a Docker scenario. The P11 managed e2e chain did not yet have a dedicated deployed-service script that validates both decision and approval resolution under the S11.1 managed harness.
- Expected state: a run blocked on decision or approval is not resumed by TUI/Web local state. The REST resolution endpoint updates the governance owner state, publishes a scoped governance continuation command, refreshes projection, and the scheduler triggers the post-resolution execution step. A decision replan resolution must project a scoped `replan` run command with the resolving decision id so the command owner can schedule the next run path.
- Deviation: S11.2 validates generic resume/replan command scope, but it does not create real governance objects and does not prove decision/approval resolution continues a blocked run in a deployed service. Existing single-package tests do not prove the deployed HTTP + LLM + governance continuation loop works under the managed harness.
- Owner: e2e. This slice validates existing workorchd governance continuation behavior and does not change core runtime, template, Soul, TUI, Web, or LLM context rendering.
- Event path: mock LLM blocks a run with `blocked_on_decision` or `blocked_on_approval` -> workorchd persists the ticket/approval -> REST resolve/approve endpoint updates governance state -> workorchd publishes scoped governance continuation -> scheduler resumes the run for provide_missing_info/approval or projects the scoped replan command for replan -> mock LLM receives the post-resolution step request for direct continuation paths.
- Ledger impact: none. The script validates governance state and run state only. It does not alter coverage ledger, evidence acceptance rules, or completion policy.
- Stage Packet impact: none. The mock provider returns deterministic governance and completion candidates; this slice does not modify prompt partitioning or cache behavior.
- Contract gap: missing managed/deployed e2e coverage that real decision and approval governance objects can be resolved and followed by scheduler-driven continuation without endpoint-local state transitions.
- Proposed change: add `scripts/verify-approval-decision-resolution-e2e.sh`, expose it as `make verify-approval-decision-resolution-e2e`, include it in `verify-e2e-full`, and document the S11.3 contract.
- Boundary check: the change is limited to e2e script, Makefile wiring, and roadmap documentation. No production core code or template-specific behavior is added.
- Verification: shell syntax check, managed harness execution, full Go tests, `git diff --check`, and boundary search over the changed e2e/doc files.

## E2E Contract

- The script must run against `WORKORCH_BASE_URL` so it can be executed under the managed harness or against an already deployed service.
- The script starts a local OpenAI-compatible mock provider and temporarily configures WorkOrch to use it, restoring the previous LLM configuration on exit.
- The script creates separate temporary workspaces for decision continuation, decision replan, and approval continuation so one active blocked run cannot interfere with another governance path; all workspaces are deleted with `purge=true` during cleanup.
- The script validates:
  - a decision blocked run can be resolved with `provide_missing_info` and triggers the post-decision execution step;
  - a decision blocked run can be resolved with `replan` and projects a scoped `replan` command containing the resolving decision id and a positive command revision;
  - an approval blocked run can be approved and triggers the post-approval execution step;
  - decision and approval list/get endpoints expose the scoped governance object before resolution;
  - resolved/approved governance objects no longer leave the direct continuation paths without a scheduler-visible continuation step.
- The script must not depend on TUI/Web state or local client-side inference.

## Acceptance

- `verify-approval-decision-resolution-e2e` passes against the managed harness.
- The script records workspace id, original run ids, ticket/approval ids, observed continuation steps, and the replan command projection in a summary JSON artifact.
- Existing production governance continuation behavior is validated without production code changes.

## Verification

- `bash -n scripts/verify-approval-decision-resolution-e2e.sh`
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-approval-decision-resolution-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=1 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" scripts/verify-approval-decision-resolution-e2e.sh Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-3-approval-decision-e2e.md`
