# Runtime Contract Retry Full-Chain Audit Plan

## Purpose

This document defines how to audit workspace execution end to end without overflowing the LLM context or turning live workspace observations into one-off fixes. It supports the implementation plan in `runtime-contract-retry-convergence-closure-plan.md`.

The audit must be complete enough to explain the whole workspace execution chain, but each code closure step must load only the smallest evidence package needed for that step.

## Core Rule

Do not repair WorkOrch from a single live blocker. Every code change must first map the blocker to a full-chain audit category, cite evidence, and identify whether the fix belongs to core runtime, workspaceTemplate/Soul/profile policy, projection/ViewModel, or client display.

Live workspaces are samples. They must not introduce business-specific rules into core.

## Audit Artifact Levels

### L0: Index And Decision Layer

Purpose:

- Provide the default document loaded for discussion and implementation planning.
- Summarize conclusions, deviation classes, batch status, and evidence refs.
- Avoid raw event dumps, full request payloads, SSE content, and large checkpoint JSON.

Expected content:

- Workspace summary matrix.
- Deviation matrix.
- Batch checklist.
- Evidence refs pointing to L1/L2.
- Current implementation decision and next action.

Recommended file:

- `docs/70-roadmap/active/runtime-contract-retry-convergence-closure-plan.md`

### L1: Workspace / Run Summary Layer

Purpose:

- Preserve full execution story per workspace and run without raw payload expansion.
- Allow reviewers to understand execution progress, retry, decision, and artifact changes without opening large files.

Expected content per workspace:

- Workspace identity, active run, template/profile, current state.
- Run timeline.
- Key step table.
- Todo / executionGraph / dispatched step mapping.
- Contract violation timeline.
- Decision / approval timeline.
- Artifact and git state.
- Coverage ledger summary.
- Context/actionability observations.

Recommended files:

- `docs/70-roadmap/active/runtime-contract-retry-audit-ws-4e83810542ac4d65.md`
- `docs/70-roadmap/active/runtime-contract-retry-audit-ws-7ced70e98ea50c43.md`

### L2: Evidence Index Layer

Purpose:

- Preserve traceability to raw data without loading raw data by default.
- Record exact paths, event revisions, request/response files, checkpoint files, and extraction commands.

Expected content:

- Evidence ID.
- Claim supported.
- Workspace/run/step.
- Raw file path.
- Event revision or checkpoint id.
- Extraction command.
- Short extracted value.
- Whether the evidence is required for a closure batch.

Recommended file:

- `docs/70-roadmap/active/runtime-contract-retry-evidence-index.md`

## Current Audit Status

The full-chain audit artifacts have been materialized. The audit phase is no longer a request to collect unbounded raw data; it is now the control surface for selecting a bounded closure gate.

| Artifact | Current Status | Notes |
| --- | --- | --- |
| L0 convergence plan | complete for current deviations | Use as the default entry point for sequencing and allowed closure order. |
| L1 workspace summaries | complete for the two sampled workspaces | Use only the relevant workspace summary unless comparing a shared deviation. |
| L2 evidence index | complete for current cited evidence | New evidence may be added only as bounded refs with extraction commands. |
| Batch 1 state model audit | code-closed for audited gates | Projection notification, accepted-command ViewModel path, canonical topic delivery policy, owner-scoped notify lanes, and run/workspace/approval/decision/governance/orchestrator event-subsystem notify publishers are covered. Broader notify call-site migration remains owner-specific and conditional on idempotency/order/replay proof. |
| Batch 2 task projection audit | code-closed | Closure record covers first-class planned/rejected/retry task projection. |
| Batch 3 contract enforcement audit | code-closed | Closure record covers runtime-owned validation scratch boundary, shell safety, state-changing gate, and patch rules. |
| Batch 4 retry correction audit | code-closed | Closure record covers violation-specific correction profiles, forced-narrow retry, and escalation provenance. |
| Batch 5 context/actionability audit | code-closed | Closure record covers C4 available target snapshots, ignored-snapshot classification, and stable C0 prefix tests. |
| Batch 6 evidence/coverage/projection audit | code-closed for audited projection clarity gate | Closure record preserves accepted/rejected evidence boundaries, strict coverage, task/retry ViewModel clarity, and active run result/artifact projection into workspace views. |
| Batch 7 regression/live verification audit | partially verified | Unit/boundary/evidence, CI-safe managed e2e, mock resource loadtest, live projection observation with bounded workspace summaries, live goal-convergence gate tooling, and real-runtime preflight passed; positive live goal convergence and full real-provider resource verification remain pending. |

Audit completion means the deviation is understood, scoped, and mapped to a gate. It does not mean implementation is complete or that live workspace convergence has been proven.

## Audit-To-Code Readiness

Code closure must enter through the first allowed gate for the relevant batch. The gate must cite L2 evidence refs, define expected and actual behavior, identify owner, and list tests before implementation starts.

| Batch | First Allowed Closure | Entry Condition | Required Verification |
| --- | --- | --- | --- |
| 1 | closed for audited gates | B1-G1 delivery policy, owner-scoped notify lane policy, projection notification async path, and accepted-command projection semantics have closure evidence. | EventHub unit tests and boundary search. |
| 2 | closed | Planned/retry/rejected task state gap was selected and code-closed. | Projection unit tests without TUI/Web inference. |
| 3 | closed | Controlled substitute validation scratch boundary and related contract gates were selected and code-closed. | Runtime contract tests for scratch accept/reject, shell safety, state-changing gate, and patch rules. |
| 4 | closed | Retry profile gap was selected and code-closed. | Retry classifier/profile tests and repeated same-rule forced-narrow retry tests. |
| 5 | closed | Current-action snapshot actionability gap was selected and code-closed. | Request rendering tests proving C4 actionability and stable C0 prefix. |
| 6 | code-closed for audited projection clarity gate | Projection/evidence tests did not require weakening accepted evidence or strict coverage; workspace projection now carries active run result/artifact linkage. | Coverage/evidence/result/artifact projection tests. |
| 7 | `B7-G3` / full `B7-G4` remaining | Implementation closure records exist and partial verification has bounded output; live goal-convergence gate tooling exists, but positive live goal convergence and full real-provider resource verification still need bounded passing output. | Bounded command output from selected live goal-convergence/full real-resource tiers. |

The default closure order is:

1. Completed: `B3-G1` and `B3-G4` for runtime-owned validation scratch boundary and shell safety.
2. Completed: `B4-G1` and `B4-G2` for violation-specific correction profiles and forced-narrow retry.
3. Completed: `B5-G1` and `B5-G2` for C4 available target snapshots and ignored-snapshot classification.
4. Completed: `B2-G1` and `B2-G2` for first-class planned task/retry/rejected projection.
5. Code-closed for audited projection clarity gate: `B6-G3` and `B6-G4`; reopen only if later focused tests prove accepted artifact/projection ambiguity.
6. Partially complete: `B7-G1`, `B7-G2`, mock-resource `B7-G4`, live-projection `B7-G3`, live goal-convergence gate tooling, and real-runtime preflight have bounded output; positive live goal convergence and full real-provider resource verification remain open.

## Evidence Reference Format

Use stable evidence refs in all L0/L1/L2 documents:

```text
ev:<workspaceShort>/<runShort>/<stepShort>/<kind>/<ordinal>
```

Examples:

```text
ev:ws4e/run4/ctr-validation-003/output/001
ev:ws7c/run4/ctr-statechange-005/request/001
ev:ws7c/run4/ctr-statechange-005/output/001
```

Each evidence ref must include:

- `claim`: what the evidence proves.
- `counterEvidence`: optional evidence that disproves a model claim.
- `sourcePath`: local file path.
- `extractCommand`: command used to extract the relevant fragment.
- `owner`: core, template/Soul/profile, projection, client, or external provider.
- `closureBatch`: batch number that consumes the evidence.

## Full-Chain Audit Dimensions

Every workspace run must be audited against the same dimensions.

| Dimension | Expected Question | Typical Evidence |
| --- | --- | --- |
| Activation | Was workspace/template/Soul/capability/Goal/DoD bound correctly? | workspace snapshot, activation JSON, run executionContext |
| Gap / Todo | Was a gap confirmation and executable todo/executionGraph produced? | LLM response, step output, taskProgress |
| Dispatch | Did todo nodes become dispatched runtime steps? | `/step/dispatched` events, task projection |
| Execution | Did each step execute, fail, reject, or complete? | step output, runtime progress events |
| Evidence | Did output produce accepted evidence refs? | result/artifact/evidence events, checkpoints |
| Coverage | Did coverage ledger change correctly? | snapshot coverageSummary, checkpoint coverageWorkItems |
| Contract | Did LLM output satisfy requiredOutput and stage contract? | output.json, contractViolation |
| Retry | Was violation classified and retried with narrower correction? | contract violation events, retry request step |
| Decision | Did budget exhaustion open decision only when automation was exhausted? | ticket events, snapshot decisions |
| Artifact | Were workspace files/commits/artifacts produced correctly? | deliverable tree, git status/log, artifact events |
| Projection | Did workorchd ViewModel reflect true backend state? | snapshot taskProgress/runtimeExecution |
| Resource | Did CPU/memory/IO remain bounded? | process stats, watchdog diagnostics |

## Claim Template

Use this template for every audit conclusion:

```markdown
### <Claim ID>

- Claim:
- Expected behavior:
- Actual behavior:
- Evidence:
- Counter-evidence:
- Deviation type:
- Owner:
- Closure batch:
- Required code/doc/test action:
```

Example:

```markdown
### C-WS7C-STATECHANGE-SNAPSHOT-001

- Claim: The model claimed target snapshots were missing during state-changing repair.
- Expected behavior: If complete snapshots are present, retry must require patch/apply or a real blocker.
- Actual behavior: LLM returned DoD assessment and claimed missing snapshots.
- Evidence: ev:ws7c/run4/ctr-statechange-005/output/001
- Counter-evidence: ev:ws7c/run4/ctr-statechange-005/request/001
- Deviation type: available_snapshot_ignored
- Owner: core LLM request assembly and contract retry classifier
- Closure batch: 4
- Required code/doc/test action: Add snapshot-aware retry classifier and correction contract.
```

## Context Budget Policy

Default LLM context for code closure must load:

- L0 plan.
- The current batch checklist.
- The relevant L1 workspace/run summary.
- Up to 3-5 L2 evidence extracts.

Do not load by default:

- Full `events-*.jsonl`.
- Full `request.json` larger than the exact extracted fields needed.
- Full `response.sse`.
- Full checkpoint directory.
- Full workspace file tree.

When raw evidence is needed, extract only the required fields with `jq`, `rg`, `find`, or `git` commands and record the extraction command in the evidence index.

## Context-Safe Audit Packet

Every audit or implementation turn must start from a bounded packet. The packet is intentionally small so the review can stay complete without loading every event, checkpoint, request, or workspace file into the LLM context.

| Packet | Required Files | Evidence Limit | Use When |
| --- | --- | --- | --- |
| `P0-index` | L0 closure plan, full-chain audit plan | No raw evidence | Deciding scope, sequencing, or whether code closure is allowed. |
| `P1-workspace` | L0, one L1 workspace summary, evidence index | Up to 5 L2 refs | Explaining one live workspace execution story. |
| `P2-batch-audit` | L0, target batch audit, evidence index | Up to 8 L2 refs | Auditing a batch and filling gates. |
| `P3-implementation` | L0, target batch gate, selected code refs | Up to 5 L2 refs | Making a bounded code change after a gate is complete. |
| `P4-verification` | L0, closed batch records, test/e2e outputs | Bounded command output only | Verifying code closure and live execution behavior. |

Packet rules:

- Do not load both workspace L1 summaries unless comparing a shared deviation.
- Do not load full request logs; extract only stable prefix fields, current-action fields, target snapshot summaries, or the exact prompt section being audited.
- Do not load full event streams; extract timeline rows, latest revisions, and event topics required for the claim.
- Do not load full deliverable trees; extract git status/log, artifact refs, and bounded file summaries.
- If a conclusion needs more than the evidence limit, split it into another claim and another packet.

## Audit Control Loop

Use the same control loop for every remaining batch:

1. Select exactly one batch and one packet type.
2. List the claims that will be audited in that batch.
3. Add or verify L2 evidence refs before writing conclusions.
4. Fill each closure gate with expected behavior, actual behavior, owner, proposed change, boundary check, and required tests.
5. Only then start code closure for that gate.
6. After code closure, append a closure record and rerun the evidence reference integrity check.

Reference integrity check:

```bash
refs=$(mktemp)
defs=$(mktemp)
rg -o 'e[v]:[A-Za-z0-9_./-]+' docs/70-roadmap/active/runtime-contract-retry-*.md | awk -F'e[v]:' '{print "e" "v" ":" $2}' | sort -u > "$refs"
rg -n '^### e[v]:' docs/70-roadmap/active/runtime-contract-retry-evidence-index.md | sed 's/^.*### //' | sort -u > "$defs"
echo 'missing definitions:'
comm -23 "$refs" "$defs"
echo 'unused definitions:'
comm -13 "$refs" "$defs"
```

## Batch Plan

### Batch 1: Full-Chain State Model Audit

Goal:

- Confirm the expected state chain from activation to decision.
- Define the state model that all later closure must preserve.

Audit outputs:

- State transition matrix.
- Expected event topics per transition.
- Known deviations from live workspaces.

Consumes dimensions:

- Activation, dispatch, execution, decision, projection.

### Batch 2: Todo / ExecutionGraph Audit

Goal:

- Confirm how LLM todo/executionGraph is created, persisted, dispatched, retried, rejected, and projected.

Audit outputs:

- Todo-to-step mapping.
- Planned task projection requirements.
- Retry task representation requirements.

Consumes dimensions:

- Gap / Todo, dispatch, execution, projection.

### Batch 3: Contract Enforcement Audit

Goal:

- Align contract wording, requiredOutput, and runtime enforcement.

Audit outputs:

- Controlled substitute validation policy.
- State-changing output policy.
- Patch/apply policy.
- Validation shell policy.

Consumes dimensions:

- Contract, evidence, artifact.

### Batch 4: Retry Correction Audit

Goal:

- Ensure each violation type has targeted retry behavior.

Audit outputs:

- Violation taxonomy.
- Retry correction contract by violation type.
- Budget escalation rules.

Consumes dimensions:

- Contract, retry, decision.

### Batch 5: Context And Actionability Audit

Goal:

- Ensure LLM request is complete, ordered, actionable, and not overloaded by competing historical data.

Audit outputs:

- Required current-action fields.
- Available snapshot summary.
- Context noise reduction rules.
- Cache/context budget guardrails.

Consumes dimensions:

- Gap / Todo, contract, retry, context/actionability.

### Batch 6: Evidence And Coverage Audit

Goal:

- Ensure accepted evidence binds to coverage work items and moves the ledger.

Audit outputs:

- Evidence acceptance matrix.
- Rejected evidence taxonomy.
- Coverage ledger update rules.
- Projection evidence ownership matrix.
- Artifact snapshot completeness rules.

Consumes dimensions:

- Evidence, coverage, artifact, projection.

Required packet:

- `P2-batch-audit`
- L0 closure plan.
- `runtime-contract-retry-batch6-evidence-coverage-projection-audit.md`
- Evidence index entries for result recording, artifact recording, coverage ledger, projection, and workspace artifact/git state.

Gate outline:

- `B6-G1`: accepted/rejected evidence ownership.
- `B6-G2`: coverage ledger movement after accepted evidence.
- `B6-G3`: artifact snapshot completeness.
- `B6-G4`: ViewModel projection owns coverage, artifact, retry, and decision display state.

### Batch 7: Regression And Live Verification

Goal:

- Prove the closure does not rely on live workspace-specific assumptions.

Audit outputs:

- Unit test list.
- E2E script list.
- Live verification checklist.
- Core/template boundary search results.
- Resource observation checklist.
- TUI/Web projection-only verification checklist.

Consumes dimensions:

- All dimensions.

Required packet:

- `P4-verification`
- L0 closure plan.
- `runtime-contract-retry-batch7-regression-live-verification-audit.md`
- Closure records from the batches being verified.
- Bounded test, e2e, CPU, memory, IO, and live workspace summaries.

Gate outline:

- `B7-G1`: unit regression coverage per closed gate.
- `B7-G2`: e2e coverage for command, projection, retry, and decision flows.
- `B7-G3`: live verification with no workspace-specific core assumptions.
- `B7-G4`: resource envelope remains bounded during multi-workspace execution.

## Required L1 Workspace Summary Sections

Each workspace audit summary must include:

1. Workspace identity and current state.
2. Goal/DoD summary and coverage ledger summary.
3. Run timeline table.
4. Key step table.
5. Todo / executionGraph / step mapping.
6. Contract violation and retry timeline.
7. Decision and approval timeline.
8. Artifact and git state timeline.
9. LLM request/actionability observations.
10. Projection observations.
11. Resource observations.
12. Final deviation matrix.

## Required L2 Evidence Index Fields

| Field | Required | Notes |
| --- | --- | --- |
| evidenceId | yes | Stable evidence ref id using the `ev:<workspace>/<run>/<step>/<kind>/<ordinal>` format |
| claimId | yes | L0/L1 claim id |
| workspaceId | yes | Full workspace id |
| runId | yes | Full run id |
| stepId | optional | Required for step-level evidence |
| evidenceKind | yes | event, output, request, response, checkpoint, artifact, git, process |
| sourcePath | yes | Local path or API endpoint |
| revision | optional | Event revision or checkpoint id |
| extractCommand | yes | Exact command used |
| extractedSummary | yes | Short bounded summary |
| owner | yes | core/template/projection/client/external |
| closureBatch | yes | Batch number |
| loadByDefault | yes | Usually `false` for L2 raw evidence |

## Closure Gate Before Code Changes

Before any code change, fill this gate in the relevant batch notes:

```markdown
- Chain node:
- Deviation type:
- Evidence refs:
- Owner:
- Expected behavior:
- Actual behavior:
- Proposed change:
- Why this is not a business-specific core rule:
- Required tests:
```

If any field is missing, do not change code yet.

## Boundary Guardrails

- Do not add live workspace IDs to core tests except as fixture names in audit/e2e data.
- Do not hardcode LSP, provider gateway, Go, `main.go`, or `main_test.go` behavior in kernel/orchestrator.
- Use structured metadata such as `validationEvidence`, `availableTargetSnapshots`, `coverageTargets`, and `violationType`.
- Keep dynamic repair/retry fields out of C0 stable prompt sections.
- Contract violation outputs remain rejected diagnostic evidence until accepted by runtime; they must not become accepted result/artifact or high-value memory.

## Initial Evidence Seeds

These are starting points for L2 extraction, not complete evidence records:

- `ws-4e83810542ac4d65`
  - Latest blocker: `concrete_validation_contract_missing`
  - Sample output path: `.workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-001-replan-001-replan-001-replan-001/steps/run-001-replan-001-replan-001-replan-001/017-contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi/output.json`
  - Sample request path: `.workorch-data/internal/workspaces/ws-4e83810542ac4d65/runs/run-001-replan-001-replan-001-replan-001/llm-calls/run-001-replan-001-replan-001-replan-001/007-contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi/request.json`
- `ws-7ced70e98ea50c43`
  - Latest blocker: `state_changing_required_missing`
  - Sample output path: `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/run-001-replan-001-replan-001-replan-001/steps/run-001-replan-001-replan-001-replan-001/006-contract-retry-001-of-planning-001-5/output.json`
  - Sample request path: `.workorch-data/internal/workspaces/ws-7ced70e98ea50c43/runs/run-001-replan-001-replan-001-replan-001/llm-calls/run-001-replan-001-replan-001-replan-001/006-contract-retry-001-of-planning-001-5/request.json`

## Implementation Order

The initial audit material is already in place. Continue from the current state rather than restarting collection.

Completed:

1. L1 workspace summaries were created for the two sampled workspaces.
2. The L2 evidence index was created with bounded extracts.
3. The L0 convergence plan was updated with deviation IDs linked to evidence refs.
4. Batch 1 through Batch 6 audits were completed.
5. Batch 7 verification inventory was completed and selected verification tiers now have bounded output; live goal-convergence gate tooling exists, while positive live goal convergence and full real-provider resource verification remain pending.

Remaining implementation workflow:

1. Select exactly one still-open closure or verification gate from the L0 `Code Closure Entry Order`.
2. For a code gate, load packet `P2-batch-audit` for gate filling, then packet `P3-implementation` for code closure.
3. Fill the target closure gate in the batch audit document before editing code.
4. Implement only the minimal change required by that gate.
5. Add or update tests that fail on the audited deviation and pass after the change.
6. Run boundary search to prove core remains business-neutral.
7. Append a closure record to the batch audit document.
8. Rerun the evidence reference integrity check.
9. For the current state, use Batch 7 as the active verification gate and attach bounded live goal-convergence or real-provider resource output before claiming runtime convergence.

## Rolling Audit Execution Plan

This section turns the planning method into a repeatable execution workflow for code closure. The audit documents named below already exist; each pass now means filling the relevant closure gate, implementing the gated change, and adding verification records.

### Audit Pass A: Contract Enforcement

- Batch: 3.
- Status: code-closed; keep this pass as the contract enforcement reference.
- Required documents: L0 plan, Batch 3 audit, evidence index, one relevant L1 workspace summary.
- Required evidence limit: `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`, and no more than five code evidence refs.
- Questions:
  - Does requiredOutput match enforcement?
  - Does shell validation have an explicit path boundary?
  - Does state-changing repair reject read-only assessment correctly?
  - Are patch/apply safe-write exceptions documented and testable?
- Output before code: Filled `B3-G1` to `B3-G4` gates.
- Closure result: runtime-owned validation scratch boundary for controlled substitute validation was implemented with shell safety preserved.

### Audit Pass B: Retry Correction

- Batch: 4.
- Status: code-closed; keep this pass as the retry correction reference.
- Required documents: L0 plan, Batch 4 audit, Batch 3 audit summary, evidence index.
- Required evidence limit: retry timeline summaries and violation-feedback code refs only.
- Questions:
  - Is each contract violation classified by rule and target scope?
  - Does same-type retry become narrower after repetition?
  - Does budget escalation occur only after targeted retry is exhausted?
  - Are rejected outputs kept as diagnostic evidence only?
- Output before code: Violation taxonomy, correction contract matrix, and retry escalation gate.
- Closure result: violation-specific correction profiles and repeated same-rule forced-narrow retry were implemented.

### Audit Pass C: Context And Actionability

- Batch: 5.
- Status: code-closed; keep this pass as the context/actionability reference.
- Required documents: L0 plan, Batch 5 audit, relevant L1 summary, bounded request extracts.
- Required evidence limit: current-action fields, available snapshot summary, selected context ordering, and cache partition refs.
- Questions:
  - Can the LLM reconstruct current Goal/DoD, current requested action, available snapshots, accepted/rejected evidence, and unresolved gaps?
  - Are large historical records compacted without losing actionable source refs?
  - Are dynamic retry fields kept out of C0?
  - Is the request prompt too noisy or contradictory for the current step?
- Output before code: Current-action schema, context budget policy, and request rendering tests.
- Closure result: C4 available target snapshot summary and `available_snapshot_ignored` classification were implemented.

### Audit Pass D: Evidence, Coverage, And Projection

- Batch: 6.
- Status: code-closed for the audited projection clarity gate; reopen only if later focused projection/evidence tests prove a new gap.
- Required documents: L0 plan, Batch 6 audit, artifact/git evidence, projection evidence.
- Required evidence limit: accepted/rejected evidence refs, coverage ledger summary, planned task projection summary.
- Questions:
  - Which outputs become accepted evidence, rejected evidence, results, artifacts, or memory?
  - How does coverage ledger move after evidence is accepted?
  - Does workorchd projection show planned tasks, retries, decisions, artifacts, and coverage without client-side inference?
  - Are artifact snapshots complete enough to support final closure?
- Output before code: Evidence acceptance matrix, coverage ledger matrix, and projection ViewModel gate.
- Closure result: accepted/rejected result and strict coverage boundaries remain authoritative; workspace projection now carries active run result/artifact linkage so clients do not infer closure from raw workspace files or git state.

### Audit Pass E: Regression And Live Verification

- Batch: 7.
- Status: partially verified; live goal convergence and full real-provider resource verification remain pending.
- Required documents: L0 plan and all closed batch summaries.
- Required evidence limit: unit test output, selected e2e output, process CPU/memory/IO snapshot, and bounded workspace verification summary.
- Questions:
  - Do unit and e2e tests cover each closed gate?
  - Do live workspaces progress without relying on workspace-specific rules?
  - Are CPU/IO costs bounded during multi-workspace execution?
  - Do TUI/Web display only workorchd-owned projection state?
- Output: Regression checklist and final verification report.
- Execution rule: do not claim Batch 7 or runtime convergence complete until actual bounded live goal-convergence and real-provider resource outputs are attached to closure records.

## Per-Batch Closure Record Template

Every batch implementation must append a closure record to the batch audit document:

```markdown
## Closure Record

- Closed gate:
- Code change refs:
- Tests added:
- Tests run:
- Boundary search run:
- Core/template boundary result:
- Remaining risk:
- Follow-up batch:
```

If the closure record cannot be filled, the batch remains audit-only and code changes should not be described as complete.
