# Runtime Contract Retry Convergence Closure Plan

## Background

Recent live verification covered two active software-dev workspaces:

- `ws-4e83810542ac4d65` (`LSP-Bridge MVP`)
- `ws-7ced70e98ea50c43` (`轻量级 LLM 本地代理网关`)

Both workspaces created run tasks and both exercised runtime retry paths after LLM contract violations. Neither reached final Goal/DoD closure. The observed failures are not caused by TUI refresh, EventHub dispatch loss, or workorchd projection absence. The blockers are in runtime contract enforcement, retry correction contracts, LLM request assembly, and task projection clarity.

This document defines the next code closure plan. It is intentionally business-neutral: WorkOrch core remains responsible for orchestration, contracts, state machine, context, retry, projection, and evidence rules. Concrete software development behavior remains in workspaceTemplate, Soul, profile, capabilities, and workspace-bound data.

## Audit Planning

The detailed full-chain audit planning is maintained in `runtime-contract-retry-full-chain-audit-plan.md`.

Before implementing code changes from this plan, use that audit plan to:

1. Split evidence into L0/L1/L2 artifacts.
2. Map each blocker to a full-chain audit dimension.
3. Cite exact evidence refs for every conclusion.
4. Fill the closure gate for the target batch.
5. Load only the relevant L1 summary and a bounded set of L2 extracts into the implementation context.

This prevents single-blocker fixes from overriding the overall runtime design.

## Current Findings

### ws-4e83810542ac4d65

Observed behavior:

- The workspace produced a real task list and progressed through planned execution graph nodes.
- It produced intermediate deliverables, including `cmd/lsp-bridge/main.go`, a `lsp-bridge` binary, and workspace commits.
- Runtime retry was triggered after LLM contract violations.
- The latest blocker is `concrete_validation_contract_missing`.

Main deviation:

- The prompt contract allows controlled substitute validation using an isolated temporary directory.
- The runtime validation gate still rejected shell validation that writes files under `/tmp`, with:
  `concrete validation evidence graph must not mutate workspace files through shell/command`.
- The model retried the same validation intent with improved metadata, but the same enforcement rejected it repeatedly.

Interpretation:

- The todo/graph path exists and is being executed.
- The retry loop works mechanically.
- The contract text and contract enforcement are inconsistent for controlled substitute validation.

### ws-7ced70e98ea50c43

Observed behavior:

- The workspace initially produced a task list and created a minimal Go proxy skeleton.
- It attempted focused provider/model prefix and fallback verification.
- Runtime retry was triggered repeatedly after contract violations.
- The latest blocker is `state_changing_required_missing`.

Main deviation:

- The latest LLM request contained complete `main.go` and `main_test.go` snapshots.
- The LLM still repeatedly claimed that it lacked current target snapshots or actionable diagnostics.
- It returned DoD assessment instead of the required state-changing output.

Interpretation:

- The early todo/graph path existed and executed.
- The latest run degraded into a contract retry loop rather than a new effective state-changing todo list.
- The issue is not missing target data. It is a context/contract interpretation failure and insufficiently targeted retry correction.

## Full-Chain Audit Artifacts

The current audit is split into the following artifacts:

| Level | Document | Role |
| --- | --- | --- |
| L0 | `runtime-contract-retry-convergence-closure-plan.md` | Decision index, deviation matrix, and closure sequencing. |
| L1 | `runtime-contract-retry-audit-ws-4e83810542ac4d65.md` | Workspace/run execution story for `ws-4e83810542ac4d65`. |
| L1 | `runtime-contract-retry-audit-ws-7ced70e98ea50c43.md` | Workspace/run execution story for `ws-7ced70e98ea50c43`. |
| Batch | `runtime-contract-retry-batch1-state-model-audit.md` | State model, EventHub command, lane, and projection ownership audit. |
| Batch | `runtime-contract-retry-batch2-task-projection-audit.md` | Todo / executionGraph / task projection lifecycle audit. |
| Batch | `runtime-contract-retry-batch3-contract-enforcement-audit.md` | Runtime contract enforcement audit for validation, state-changing, patch/apply, and shell safety. |
| Batch | `runtime-contract-retry-batch4-retry-correction-audit.md` | Contract violation retry correction, budget, escalation, and diagnostic boundary audit. |
| Batch | `runtime-contract-retry-batch5-context-actionability-audit.md` | LLM request partition, current-action, target snapshot actionability, and cache-boundary audit. |
| Batch | `runtime-contract-retry-batch6-evidence-coverage-projection-audit.md` | Evidence acceptance, coverage ledger, artifact completeness, and ViewModel projection audit. |
| Batch | `runtime-contract-retry-batch7-regression-live-verification-audit.md` | Regression, e2e, live verification, boundary search, and resource envelope audit. |
| L2 | `runtime-contract-retry-evidence-index.md` | Bounded raw evidence refs and extraction commands. |

Before changing code, load the L0 document, the relevant batch section, one L1 summary, and only the L2 evidence records needed by the target claim.

## Deviation Matrix

| Deviation ID | Description | Evidence | Owner | Closure Batch | Status |
| --- | --- | --- | --- | --- | --- |
| `D-CONTROLLED-SUBSTITUTE-ENFORCEMENT` | Controlled substitute validation graph declares bounded temporary fixtures, but runtime rejects it as shell mutation. | `ev:ws4e/run4/ctr-validation-003/output/001` | core runtime contract / validation gate | 3 | closed; closure record in Batch 3 |
| `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY` | Controlled substitute validation lacks an explicit runtime scratch boundary, so shell write safety is decided by temporary-target command heuristics. | `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:code/contract/shell-safety/001` | core runtime contract / validation gate | 3 | closed; closure record in Batch 3 |
| `D-AVAILABLE-SNAPSHOT-IGNORED` | State-changing repair claims target snapshots are missing although complete snapshots are present in the request. | `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`, `ev:ws7c/run4/ctr-statechange-005/request-prompt/001` | core LLM request assembly / retry classifier | 5 | closed; closure record in Batch 5 |
| `D-RETRY-CORRECTION-TOO-BROAD` | Repeated same-scope violations receive retries but do not become sufficiently narrow and corrective. | `ev:ws4e/events/all-runs/001`, `ev:ws7c/events/all-runs/001`, `ev:code/retry/recovery-projection/001` | core retry correction | 4 | closed; closure record in Batch 4 |
| `D-PLANNED-TASK-PROJECTION-GAP` | Execution graph, raw steps, tool calls, validation refs, retries, and decisions are projected as mixed items rather than first-class planned tasks. | `ev:ws4e/snapshot/current/001`, `ev:ws7c/snapshot/current/001` | projection / ViewModel | 2 | code-closed; closure record in Batch 2 |
| `D-PATCH-CONTRACT-RECOVERY-GAP` | Existing target `file_write` rejection is correct, but recovery does not consistently force exact patch modes. | `ev:ws7c/steps/summary/001` | core patch contract / retry correction | 3 | closed through Batch 3 enforcement plus Batch 4 correction profiles |
| `D-ARTIFACT-SNAPSHOT-INCOMPLETE` | One workspace produced commits and artifacts while the other has untracked skeleton files and no accepted version snapshot; final closure must use accepted result/evidence/artifact linkage rather than raw file existence. | `ev:ws4e/artifact/git/001`, `ev:ws7c/artifact/git/001`, `ev:code/evidence/artifact-recording/001`, `ev:code/projection/result-artifact-view/001` | core evidence / coverage / projection boundary | 6 | code-closed for audited projection clarity gate; workspace projection carries active run result/artifact linkage |

## Batch Audit Status

| Batch | Scope | Audit Artifact | Status | Next Gate |
| --- | --- | --- | --- | --- |
| 1 | Full-chain state model, EventHub command/notify flow, projection ownership | `runtime-contract-retry-batch1-state-model-audit.md` | code-closed for audited gates; projection async path, accepted-command ViewModel path, canonical delivery policy, owner-scoped notify lanes, and audited notify publishers covered | future notify call-site migration only by owner-specific idempotency/order/replay proof |
| 2 | Todo / executionGraph / task projection | `runtime-contract-retry-batch2-task-projection-audit.md` | code-closed | closure record in Batch 2 |
| 3 | Contract enforcement: controlled substitute, state-changing, patch/apply, validation shell | `runtime-contract-retry-batch3-contract-enforcement-audit.md` | code-closed with supplemental convergence scratch closure | closure record in Batch 3 |
| 4 | Retry correction by violation type | `runtime-contract-retry-batch4-retry-correction-audit.md` | code-closed | closure record in Batch 4 |
| 5 | Context and actionability | `runtime-contract-retry-batch5-context-actionability-audit.md` | code-closed | closure record in Batch 5 |
| 6 | Evidence, coverage ledger, artifacts, and projection | `runtime-contract-retry-batch6-evidence-coverage-projection-audit.md` | code-closed for audited projection clarity gate | reopen only if focused projection/evidence tests prove a new ambiguity |
| 7 | Regression, e2e, live verification, and resource envelope | `runtime-contract-retry-batch7-regression-live-verification-audit.md` | unit/boundary/evidence verified; CI-safe managed deployed e2e, mock resource loadtest, live projection observation, live goal-convergence gate tooling, real-runtime preflight, and live replan command main path passed; latest post-replan live outputs exposed evidence-acquisition scratch, coverageFocus/actionPackage target-set, runtime MCP capability-context, undeclared MCP server, non-substantive state-change quality gaps, shell bash-feature validation compatibility, structured coverage/evidence state-change focus, replan successor budget inheritance, and runtime validation scratch preparation defects that are code-closed locally through structured metadata and state-machine rules; post-managed live observation `ev:verify/live-convergence/20260622-007` failed on historical open decisions with healthy projection/resource signals; fresh explicit replan observation `ev:verify/live-convergence/20260622-008` exposed exhausted convergence budget inheritance, code-closed by `ev:code/replan-budget-reset/20260622-009`; current packet managed CI-safe verification passed after budget reset in `ev:verify/managed-ci-safe/20260622-010`; post-redeploy live observation `ev:verify/live-convergence/20260622-011` exposed missing runtime scratch preparation, code-closed by `ev:code/runtime-validation-scratch-prepare/20260622-012`; live `mcp_capability_denied` on `lsp_status` exposed an incomplete software-dev LSP profile tool surface, code-closed by `ev:code/software-dev-lsp-tool-surface/20260622-019`, with bootstrap/backend defaults upsert coverage added in `ev:code/bootstrap-system-defaults/20260622-020`; positive live goal convergence/full real-resource still pending after redeploy | `B7-G3` positive goal convergence after redeploy and explicit replan/resume, full real-provider resource verification |

## Full-Chain Audit Coverage Review

The audit has been checked against every execution dimension from activation to verification. This table is the completion checklist for the audit phase; it does not claim implementation closure.

| Dimension | Audit Location | Evidence Basis | Audit Result | Remaining Closure |
| --- | --- | --- | --- | --- |
| Activation | Batch 1, both L1 summaries | `ev:code/scheduler/workspace-run-request/001`, workspace snapshots | Command entry is EventHub-driven and workspace-scoped. | Keep command accepted state separate from owner run state if a state/projection change is selected. |
| Gap / Todo | Batch 2, both L1 summaries | `ev:code/projection/task-derived/001`, live step summaries | Todo/task state is first-class in projection through planned task and retry projections. | No code closure unless a focused projection defect is found. |
| Dispatch | Batch 1 and Batch 2 | `ev:code/orchestrator/dispatch-request/001`, `ev:code/projection/task-node-mapping/001` | Dispatch is event-driven and idempotent; lifecycle metadata now covers rejected/retry task states. | Preserve workspace/run/step scoped idempotency and projection ownership. |
| Execution | Batch 1 | `ev:code/runtime/block-executor/001` | Runtime block ACK/progress/terminal-event pattern is aligned. | No immediate closure unless a later gate finds backpressure or lost terminal events. |
| Evidence | Batch 3 and Batch 6 | `ev:code/evidence/result-recording/001`, `ev:code/evidence/rejected-result-boundary/001` | Accepted, rejected, slice, artifact, and memory boundaries are distinct. | Preserve rejected diagnostic boundary while adding retry/projection clarity. |
| Coverage | Batch 6 | `ev:code/coverage/work-item-ledger/001`, `ev:code/coverage/acceptance-gate/001` | Coverage gate is intentionally strict and should not be weakened. | Trace missing evidence back to contract/retry/context gaps first. |
| Contract | Batch 3 | `ev:code/contract/concrete-validation/001`, `ev:code/contract/shell-safety/001` | Runtime-owned validation scratch boundary, state-changing gate, blocker structure gate, and patch snapshot rules are code-closed. | Keep strict enforcement; do not weaken invalid-output rejection. |
| Retry | Batch 4 | `ev:code/retry/violation-taxonomy/001`, `ev:code/retry/recovery-projection/001` | Retry routing is event-driven and budgeted; violation-specific correction profiles and forced-narrow retry are code-closed. | Preserve targeted retry evidence before decision escalation. |
| Decision | Batch 1 and Batch 4 | `ev:code/retry/escalation-decision/001`, snapshots | Budget exhaustion opens scoped decisions; should occur after targeted retry. | Preserve decision only after automation evidence is exhausted. |
| Artifact | Batch 6 and L1 summaries | `ev:code/evidence/artifact-recording/001`, git evidence refs | Artifact existence is not enough; accepted result/evidence linkage is the authority. | `B6-G3` only if projection tests prove ambiguity remains. |
| Projection | Batch 1, Batch 2, Batch 6 | `ev:code/projection/manager/001`, task/evidence projection refs | workorchd is the ViewModel owner; planned task/retry/evidence/result/artifact projection is code-closed for the audited packet. | TUI/Web must continue consuming projection without business-state inference. |
| Resource | Batch 7 | `ev:verify/resource/real-runtime-load/001`, `ev:service/process/001` | CI-safe mock resource loadtest and real-runtime preflight passed for the current closure packet; real-provider runtime/resource execution remains pending. | Run full `verify-real-runtime-closure` only with configured external provider credentials and bounded output. |
| Boundary | Batch 7 and skill rules | `ev:verify/boundary-contract/001` | Core/template boundary has a verifier and docs constraints. | Run boundary search for every code closure. |

Audit coverage conclusion: every full-chain dimension has an audited owner, evidence basis, expected/actual result, and next closure route. Implementation closure records now exist for the audited Batch 1-6 gates and selected Batch 7 tiers have bounded output. The remaining intentionally unclosed area is Batch 7 live/runtime verification: positive live goal convergence and full real-provider resource verification still require actual bounded execution output.

## Audit Completion Matrix

This matrix is the entry point for continuing from audit to code closure. It records the current audit status, the first allowed implementation gate, and the minimum verification that must be attached to the closure record.

| Batch | Audit Status | First Allowed Closure | Verification Required Before Declaring Closure |
| --- | --- | --- | --- |
| 1 | code-closed for audited gates | no automatic call-site migration; owner-specific notify migrations may use `EventHubDeliveryModeForTopic` after idempotency/order/replay review | EventHub unit tests and boundary search. |
| 2 | code-closed | closed by Batch 2 record | Projection unit tests prove planned/rejected/retry task states without TUI/Web inference. |
| 3 | code-closed | closed by Batch 3 record | Runtime contract tests for scratch accept/reject, shell safety, state-changing gate, and patch snapshot rules passed. |
| 4 | code-closed | closed by Batch 4 record | Retry classifier/profile tests, repeated same-rule forced-narrow retry tests, escalation provenance tests passed. |
| 5 | code-closed | closed by Batch 5 record | LLM prompt/context tests prove C4 actionability and stable C0 prefix; full `go test ./... -count=1` passed. |
| 6 | code-closed for audited projection clarity gate | closed by Batch 6 projection closure record | Coverage/evidence/result/artifact projection tests passed; strict coverage and rejected diagnostic boundaries preserved; workspace projection carries active run result/artifact linkage. |
| 7 | unit/boundary/evidence verified; CI-safe managed deployed e2e, mock resource loadtest, live projection observation with bounded `workspaceSummaries`, live goal-convergence gate tooling, and real-runtime preflight passed; latest deployed continuation observation found current LLM provider-call progress with bounded resources and no scheduler/projection/EventHub defect evidence; provider-returned output produced local code closures for structured coverage/evidence state-change focus and shell bash-feature validation compatibility; post-managed observation found both live workspaces still waiting on historical open decisions; explicit fresh replan observation found replan successor budget inheritance, now code-closed locally; managed CI-safe verification passed after budget reset; post-redeploy live observation found runtime validation scratch was contracted but not prepared before runtime request publication, now code-closed locally; positive live goal convergence/full real-resource pending | no additional code closure without a new audited defect after redeploy plus explicit replan/resume; verification-only gate for current packet | Actual bounded command output from selected live goal-convergence/full real-resource tiers after redeploy and explicit replan/resume. |

Audit completion means the deviation is understood and has a documented gate. It does not mean implementation or runtime convergence is complete.

## Code Closure Entry Order

Recommended closure order:

1. Completed: `B3-G1` and `B3-G4` introduced runtime-owned validation scratch boundary while preserving shell safety; supplemental closure ensures convergence `need_validation` also carries `validationScratchDir`.
2. Completed: `B4-G1` and `B4-G2` added violation-specific correction profiles and forced-narrow retry.
3. Completed: `B5-G1` and `B5-G2` surfaced available target snapshots in C4 current-action and classified ignored snapshots.
4. Completed: `B2-G1` and `B2-G2` make planned task/retry/rejected state first-class in projection.
5. Completed: `B1-G1` adds the canonical EventHub delivery policy for sync commands and async-preferred notifications, plus contract-aware publication for run/workspace/approval/decision/governance/orchestrator event-subsystem notifications without changing command/result response paths; B1 notify lane policy now uses owner/workspace/run scoped state lanes instead of global topic lanes.
6. Conditional: `B6-G3` and `B6-G4` only if later tests show artifact completeness or projection ambiguity remains after Batch 2 closure.
7. Partially complete: `B7-G1` unit regression, boundary search, and evidence integrity have bounded outputs for the current code closure packet.
8. Completed for CI-safe scope: `B7-G2` managed deployed e2e passed for HTTP, workspace template/Soul, mock LLM, MCP stdio, and the contract retry exhaustion decision path.
9. Completed for CI-safe resource scope: `B7-G4` managed mock loadtest passed for two concurrent governed workspaces with accepted results, artifacts, projection reads, and zero transport/protocol/timeline errors.
10. Partially complete: `B7-G3` live projection observation passed for the deployed 8080 service and now emits bounded `workspaceSummaries`; the observer also has a `WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1` gate that emits structured goal-convergence failures. A live `replan` resolve of the two open decisions proved the governance-continuation main path can create fresh running runs, but the post-replan outputs still failed contract convergence. The latest local code closures clarify that evidence-acquisition and supplemental validation shell/tool nodes may not write workspace deliverable files, mirror `validationScratchDir` into evidence-acquisition contracts, keep `repairActionPackage.coverageTargetSet` aligned with the current `coverageFocus`, preserve current runtime MCP capability context in LLM inputs, reject undeclared MCP server IDs, reject structurally declared non-substantive state changes without natural-language placeholder/deferral parsing in core, dedupe coverage targets by normalized requirement identity, normalize bash-only pipeline status syntax in `sh -c` validation scripts to bash execution, reset replan successor convergence budgets, prepare runtime-owned validation scratch directories before runtime request publication, and avoid charging no-delta budget to completed non-validation graph nodes before downstream validation/result evidence can move coverage. Current packet CI-safe managed verification passed in `ev:verify/managed-ci-safe/20260622-010`; post-redeploy live observation `ev:verify/live-convergence/20260622-011` exposed the scratch preparation gap now closed by `ev:code/runtime-validation-scratch-prepare/20260622-012`; fresh post-replan observation exposed the intermediate graph-node budget gap now closed by `ev:code/convergence-budget-intermediate-graph-nodes/20260623-031`. Positive convergence now requires redeploy plus explicit replan/resume followed by fresh bounded observation.
11. Partially complete: real-runtime preflight passed for the deployed 8080 service, but full real-provider load/resource verification still requires external provider credentials and bounded execution output.

Every closure must add a closure record to the target batch document and must rerun evidence reference integrity checks.

## Closure Gate

No code change should start until the target deviation has a filled gate:

| Field | Required Content |
| --- | --- |
| Chain node | Exact activation, todo, dispatch, execution, contract, retry, decision, projection, or evidence node. |
| Deviation type | One of the deviation IDs above or a newly added audited deviation. |
| Evidence refs | L2 IDs, not raw file dumps. |
| Owner | core, template/Soul/profile, projection/ViewModel, client, or external. |
| Expected behavior | Design-level behavior. |
| Actual behavior | Observed behavior from L1/L2. |
| Proposed change | Minimal change that addresses the audited deviation. |
| Boundary check | Why the change is business-neutral if it touches core. |
| Required tests | Regression tests that fail before the change and pass after it. |

## Target State

The closure target is:

1. A workspace should produce an explicit executable task plan from LLM execution output.
2. workorchd should project that plan as a stable task list with plan, dispatch, completion, rejection, retry, and decision state.
3. Runtime should execute valid graph nodes and reject invalid outputs before side effects.
4. Runtime retry should narrow the correction contract according to the violation type.
5. If required data is already present in the request, LLM output that claims it is missing should be classified as a contract violation with targeted feedback.
6. Controlled substitute validation should be allowed when it is explicitly bounded, non-destructive, and outside the workspace deliverable tree.
7. Budget exhaustion should still open a decision ticket, but only after targeted retry has been attempted.

## Audit-To-Closure Operating Plan

The remaining work must be executed as batch gates, not as single-workspace fixes. Each batch has a fixed audit payload, implementation payload, and verification payload.

| Stage | Required Input | Output Before Code | Code Closure Allowed When | Verification |
| --- | --- | --- | --- | --- |
| Batch 3 | L0, Batch 3 audit, `ws4e` controlled substitute evidence, contract code refs | Filled `B3-G1` to `B3-G4` gates | The target change maps to one filled gate and preserves core/template boundary | Runtime contract unit tests for scratch, shell safety, state-changing, and patch rules |
| Batch 4 | Batch 3 results, violation feedback code refs, retry event timeline | Violation taxonomy and retry correction matrix | Each violation type has expected retry contract and escalation rule | Orchestrator retry tests for same-type repeated violations |
| Batch 5 | Batch 4 retry matrix, request/actionability evidence, context/cache docs | Current-action and context payload contract | Required LLM context fields are complete, ordered, bounded, and C0-stable | LLM request rendering tests and bounded request-log checks |
| Batch 6 | Evidence/coverage docs, artifact/git evidence, projection evidence | Evidence acceptance and coverage ledger matrix | Accepted/rejected evidence ownership is explicit | Coverage ledger and projection tests |
| Batch 7 | All previous batch gates | Regression checklist and live verification script | All affected unit tests and selected e2e pass | Fresh or resumed workspace verification with bounded resource observation |

For each stage:

1. Load only L0, the target batch audit, one relevant L1 workspace summary, and 3-5 L2 evidence records.
2. Fill the closure gate before implementation.
3. Implement the smallest code change that satisfies the gate.
4. Add tests that fail on the audited deviation and pass after the change.
5. Run boundary search to prove core did not absorb template-specific rules.
6. Update this document and the evidence index with the actual closure result.

No stage may use live workspace ID, filename, programming language, provider, LSP, gateway, or template title as core behavior. Live evidence remains an audit sample only.

## Context-Safe Execution Packets

The audit and implementation context must be loaded as packets, not as raw workspace dumps.

| Packet | Contents | Maximum Evidence Load | Purpose |
| --- | --- | --- | --- |
| `P0-index` | This L0 plan and `runtime-contract-retry-full-chain-audit-plan.md` | none | Decide scope and next batch. |
| `P1-workspace` | L0, one L1 workspace summary, evidence index | 5 L2 refs | Explain one workspace execution story. |
| `P2-batch-audit` | L0, target batch audit, evidence index | 8 L2 refs | Fill closure gates. |
| `P3-implementation` | Filled batch gate, selected code refs, relevant L2 refs | 5 L2 refs | Make a bounded code change. |
| `P4-verification` | Closed batch records, test/e2e/resource summaries | bounded command output | Verify closure and live behavior. |

Rules:

- If more evidence is required, split the claim instead of loading more raw data.
- Full event streams, full request payloads, full checkpoint directories, and full deliverable trees are never loaded by default.
- Every new conclusion must cite L2 evidence refs or explicitly mark itself as an open question.
- Code closure starts only after the target gate is filled in the batch audit document.

## Closure Scope

### In Scope

- Runtime contract gate for concrete validation evidence.
- Controlled substitute scratch directory semantics.
- Contract violation classification.
- Retry correction contract generation.
- LLM request current-action and available snapshot signal.
- Task/todo projection from execution graph nodes.
- Regression tests for runtime contracts and retry behavior.

### Out of Scope

- TUI-specific business state inference.
- Hardcoding LSP, provider routing, Go, or any specific business target in kernel/orchestrator.
- Weakening Goal/DoD coverage semantics to accept unverifiable output.
- Treating these live workspace IDs as permanent product behavior.

## Work Items

### 1. Controlled Substitute Validation Contract

Problem:

Runtime contract text says controlled substitute validation may use an isolated temporary directory, but enforcement rejects shell commands that create files even when they are outside the workspace.

Required changes:

- Define a runtime-owned validation scratch directory field, for example `validationScratchDir`.
- Include `validationScratchDir` in validation-stage LLM request contracts when controlled substitutes are allowed.
- Allow `shell/command` validation nodes to create files only when all of the following are true:
  - `input.validationEvidence.mode == "controlled_substitute"`
  - `input.validationEvidence.observationType == "controlled_substitute_observation"`
  - `input.validationEvidence.substituteFor` or `input.validationEvidence.substitute` is present
  - all detected write targets are inside the runtime-provided scratch directory
  - no write target is inside the workspace deliverable directory
- Keep the existing rejection for workspace file mutation through shell validation.

Expected result:

- `ws-4e...` style validation graphs can proceed when they write only isolated temporary fixtures.
- Workspace file mutation still requires `patch/apply`.

Ownership:

- Core runtime contract and validation gate.

### 2. Available Snapshot Aware State-Changing Retry

Problem:

The LLM can claim that target snapshots are missing even when `failureTargetRepairContext`, `patchRepairContext`, or `targetSnapshots` already contain complete current content.

Required changes:

- Add an explicit `availableTargetSnapshots` summary to current-action for state-changing repair.
- Include each target with:
  - `targetRef`
  - `contentComplete`
  - `safeForFileWrite`
  - allowed patch modes
  - `contentHash`
  - source field path
- In state-changing repair, if complete snapshots exist, current-action must explicitly say:
  `Do not claim target snapshot is missing; use the supplied complete snapshot or return a real blocker.`
- Add a contract violation classifier such as `available_snapshot_ignored` when output claims missing snapshot but the request contains complete snapshot content.
- Retry feedback for this classifier must point to the exact available targets and require `patch/apply`, accepted state-changing shell/tool operation, or real blocker.

Expected result:

- `ws-7c...` style loops stop repeating false "snapshot missing" assessments.
- Retry becomes actionable instead of repeating the same broad state-changing contract.

Ownership:

- Core LLM request assembly, contract violation classifier, retry correction generator.

### 3. Retry Correction Contracts By Violation Type

Problem:

Runtime retry currently works, but repeated retries often restate the broad contract. This is insufficient when the model repeats a specific violation.

Required changes:

- For `concrete_validation_contract_missing`, produce a correction contract that includes:
  - allowed validation node kinds
  - required validationEvidence fields
  - runtime-provided scratch directory
  - examples of invalid and valid validation shell usage
- For `state_changing_required_missing`, produce a correction contract that includes:
  - available target snapshots
  - required patch mode
  - forbidden return of DoD assessment without state-changing output
  - fallback to real blocker only when the supplied data is genuinely insufficient
- For `patch_dispatch_invalid`, produce a correction contract that includes:
  - invalid patch reason
  - allowed patch forms
  - whether `file_write` is forbidden for existing target
  - required coverageTargets and strategy reassessment fields
- After the second same-type violation on the same target scope, switch to a narrower forced correction contract instead of a broad retry.

Expected result:

- Retry remains strict but becomes more helpful and less repetitive.

Ownership:

- Core orchestrator contract recovery.

### 4. Planned Task Projection

Problem:

The system creates executionGraph nodes, but UI consumers mainly see a mixed list of steps, tool calls, validation results, and decisions. This makes it hard to confirm whether a workspace has a coherent todo plan or is only retrying.

Required changes:

- Persist LLM executionGraph nodes as planned run tasks before dispatch where possible.
- Project task state separately from raw step/event state:
  - `planned`
  - `dispatched`
  - `running`
  - `completed`
  - `failed`
  - `rejected`
  - `retrying`
  - `blocked_on_decision`
- Link each projected task to:
  - graph node id
  - runtime step id
  - coverage target refs
  - validation/result refs
  - retry source step when applicable
- Mark contract retry steps as retry tasks with violation type and retry attempt.

Expected result:

- workorchd remains the ViewModel owner.
- TUI/Web can show true planned todo progress without reconstructing business state.

Ownership:

- Core read model / projection / appservice ViewModel.

### 5. Context Noise Reduction For State-Changing Repair

Problem:

State-changing repair requests include many valid sections, but the repair action can be drowned by historical failures, broad DoD coverage, forbidden actions, and older rejected outputs.

Required changes:

- Keep `repairActionPackage` as the primary actionable contract.
- Convert older failure history into compact evidence summaries.
- Keep complete target snapshots near current-action, not only buried in repair context.
- Avoid repeating full DoD assessment outputs in retry request body unless directly needed.
- Ensure contractViolationRecovery is diagnostic evidence, not a competing instruction source.

Expected result:

- LLM has less room to choose "cannot safely patch" when the necessary patch target is already supplied.

Ownership:

- Core LLM context assembly and context budget normalization.

### 6. Regression Tests

Required tests:

- Controlled substitute validation:
  - shell writes inside runtime scratch directory are accepted when evidence metadata is complete.
  - shell writes inside workspace deliverable directory are rejected.
  - shell writes outside both workspace and scratch directory are rejected.
- Snapshot-aware retry:
  - when complete `main.go`/`main_test.go` style snapshots exist, output claiming missing snapshot is classified as `available_snapshot_ignored`.
  - retry correction includes available target refs and allowed patch modes.
- State-changing contract inheritance:
  - contract retry preserves `stateChangingOutputRequired=true`.
- Patch contract:
  - existing file cannot be replaced through `file_write` when exact snapshot exists unless policy explicitly permits it.
  - `text_replace` and `unified_diff` with coverageTargets pass validation.
- Projection:
  - executionGraph nodes appear in task projection before or at dispatch.
  - contract retry appears as retrying/rejected/blocked state with violation metadata.

Suggested validation commands:

```bash
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/blocks/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./... -count=1
```

## Sequencing

1. Completed: Batch 3 scratch-boundary closure (`B3-G1`) with state-changing/patch gates kept strict.
2. Completed: Batch 4 violation-specific retry correction audit and closure.
3. Completed: Batch 5 request actionability and context noise audit and closure.
4. Completed for audited projection clarity gate: Batch 6 evidence, coverage, artifact, and projection audit and closure.
5. Completed for focused Batch 7 live blockers:
   - replan successor budget reset (`ev:code/replan-budget-reset/20260622-009`)
   - runtime validation scratch preparation (`ev:code/runtime-validation-scratch-prepare/20260622-012`)
   - run progress lifecycle preservation for parallel graph execution (`ev:code/run-progress-lifecycle-preserve/20260622-014`)
   - startup owner-state reconciliation for historical active-run governance blockers (`ev:code/startup-governance-blocker-reconcile/20260622-015`)
   - global decision page scoped resolution projection for duplicate ticket IDs (`ev:code/global-decision-page-scoped-resolution/20260622-017`)
   - global governance page scoped cursors for duplicate ticket/approval IDs (`ev:code/global-governance-page-scoped-cursor/20260622-018`)
   - stale patch `baseHash mismatch` refresh repair budget and action contract (`ev:code/patch-basehash-refresh-repair/20260622-021`, live-verified for that blocker by `ev:verify/live-patch-basehash-refresh/20260622-022`)
   - patch application failure narrow retry contract for stale `oldText`/diff/content after dispatch (`ev:code/patch-application-failure-narrow-retry/20260622-023`)
   - LLM-visible target snapshot compaction safety, so compacted snapshots cannot claim complete safe file-write or exact patch baseline (`ev:code/llm-target-snapshot-compaction-safety/20260622-024`)
   - scoped root-cause strategy reassessment, so repeated failed-surface repair may accept a substantive patch to a declared sub-scope inside the failed resource without weakening broad same-surface rejection (`ev:code/scoped-root-cause-strategy-reassessment/20260622-025`)
   - focus-bound evidence graph normalization for non-`patch/apply` evidence acquisition nodes (`ev:code/focus-bound-evidence-graph-normalization/20260623-028`)
   - convergence-budget protection for pending graph plans and stale patch conflicts (`ev:code/convergence-budget-graph-pending-and-stale-patch/20260623-029`)
   - correction-contract governance blocker shape narrowing (`ev:code/contract-correction-governance-blocker-shape/20260623-030`)
   - convergence-budget protection for successful intermediate graph nodes without validation evidence (`ev:code/convergence-budget-intermediate-graph-nodes/20260623-031`)
   - requiredOutput focus contract preservation after evidence/validation contract rebuilds (`ev:code/focus-contract-required-output-preservation/20260623-032`)
   - graph-node primary diagnostic dependency evidence reuse after a completed dependency already addressed the diagnostic (`ev:code/graph-node-primary-diagnostic-dependency-evidence/20260623-033`)
   - resource-shape alignment context from inherited structured diagnostics and target scope (`ev:code/shape-alignment-context-from-inherited-diagnostics/20260623-034`)
   - already-satisfied stale patch recovery bypasses repair-exhaustion and duplicate-failure decision gates (`ev:code/already-satisfied-stale-patch-bypasses-exhaustion/20260623-035`)
   - stale patch `baseHash mismatch` with readable current target bypasses repair-exhaustion and duplicate-failure gates into current-snapshot patch refresh (`ev:code/stale-basehash-patch-refresh-bypasses-exhaustion/20260623-036`)
   - final repair output-decision refresh preserves active coverage focus binding after convergence/output-mode projections rebuild requiredOutput (`ev:code/final-repair-output-decision-refresh-preserves-focus/20260623-037`)
6. Continue Batch 7 regression and live verification.
7. Re-run live workspace verification against fresh or resumed workspaces after the preceding gates are closed.

## Verification Criteria

The closure is acceptable when:

- A controlled substitute validation graph that writes only to runtime scratch dir is accepted and executed.
- A state-changing retry with complete target snapshots either produces valid patch output or returns a real blocker, not a false missing-snapshot assessment.
- Contract retry attempts are visible in projection as retry work, not only as failed LLM steps.
- Repeated same-type violations receive narrower correction contracts.
- TUI/Web can display the projected todo list without deriving business state client-side.
- Core code remains business-neutral; no LSP, provider gateway, Go-only, or live workspace ID special case is introduced.

## Open Questions

- Should the runtime scratch directory be per step, per run, or per workspace?
- Should scratch directory cleanup be immediate after validation, after run completion, or retention-policy driven for audit?
- Should `file_write` for existing small files ever be allowed when `safeForFileWrite=true`, or should existing files always require `text_replace` / `unified_diff`?
- Should retry budgets be per violation type, per target scope, or shared across the whole run?
