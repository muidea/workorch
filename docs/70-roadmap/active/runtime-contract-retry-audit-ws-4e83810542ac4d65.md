# Runtime Contract Retry Audit: ws-4e83810542ac4d65

## Scope

This is the L1 audit summary for `ws-4e83810542ac4d65` (`LSP-Bridge MVP`). It records the execution chain and deviation classes without embedding raw event streams or full LLM payloads.

Raw evidence is indexed in `runtime-contract-retry-evidence-index.md`. Conclusions in this document must cite L2 evidence IDs.

## Workspace State

| Field | Value |
| --- | --- |
| Workspace | `ws-4e83810542ac4d65` |
| Title | `LSP-Bridge MVP` |
| Template/Profile | `software-dev` / `software-dev` |
| Active run | `run-001-replan-001-replan-001-replan-001` |
| Workspace state | `waiting_decision` |
| Run state | `blocked_on_decision` |
| Current step | `contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi` |
| Blocking ticket | `run-001-replan-001-replan-001-replan-001-contract-retry-001-of-graph-003-001-validate-focus-cwi-009-operability-s-probe-operabi-contract-violation-decision` |

Projection was aligned at the audit point: `sourceEventRevision=9281`, `projectedSourceRevision=9281`.

Evidence: `ev:service/readyz/001`, `ev:ws4e/snapshot/current/001`.

## Goal / DoD / Coverage

The current active focus is `cwi-012`, which requires validation evidence for multi-language definition behavior. The ledger summary at the audit point was:

| Total | Satisfied | Evidence Ready | Failed | Open |
| --- | --- | --- | --- | --- |
| 40 | 6 | 6 | 23 | 5 |

The immediate gap is missing accepted `validation` evidence for `cwi-012`. The latest rejected evidence chain contains prior Go/runtime probes and current controlled-substitute validation attempts.

Evidence: `ev:ws4e/snapshot/current/001`.

## Run Timeline

| Run | Time Range UTC | Event Count | Main Observations |
| --- | --- | ---: | --- |
| `run-001` | 2026-06-20 15:43:35 to 16:22:25 | 224 | Initial graph dispatch, stream progress, patches, two contract retry events. |
| `run-001-replan-001` | 2026-06-20 16:22:25 to 2026-06-21 00:35:50 | 398 | More graph execution, validation probes, provider failure and retry. |
| `run-001-replan-001-replan-001` | 2026-06-21 00:35:50 to 02:30:27 | 241 | Result acceptance and patch application occurred; later validation failed. |
| `run-001-replan-001-replan-001-replan-001` | 2026-06-21 02:30:27 to 02:50:14 | 235 | Controlled substitute validation retried and escalated to decision. |

Evidence: `ev:ws4e/events/all-runs/001`.

## Key Steps

| Run | Step | State | Contract / Failure | Observation |
| --- | --- | --- | --- | --- |
| `run-001` | `021-contract-retry-...-2` | failed | `repair_focus_mismatch` | Retry demanded state-changing or validation action rather than read-only assessment. |
| `run-001-replan-001` | `022-contract-retry-...-3` | completed | none | Produced bounded runtime evidence graph. |
| `run-001-replan-001-replan-001` | `026-graph-004-002-validate-focus-cwi-005-unsaved-sync-diagnostics` | failed | `shell_command_failed` | Runtime validation still failed for focused evidence. |
| `run-001-replan-001-replan-001-replan-001` | `015-follow-up-...probe_operability_surface_after_init` | failed | `concrete_validation_contract_missing` | First controlled-substitute validation graph rejected by runtime gate. |
| `run-001-replan-001-replan-001-replan-001` | `016-contract-retry-...operability-surface` | failed | `concrete_validation_contract_missing` | Same violation repeated. |
| `run-001-replan-001-replan-001-replan-001` | `017-contract-retry-...probe-operabi` | failed | `concrete_validation_contract_missing` | Latest retry still rejected even though shell node declares controlled substitute metadata. |

Evidence: `ev:ws4e/steps/summary/001`, `ev:ws4e/run4/ctr-validation-003/output/001`.

## Todo / ExecutionGraph Mapping

The workspace did produce executable graph nodes and the runtime dispatched them. The latest blocker graph contains 11 nodes:

- one `shell/command` node, `prepare_multilang_definition_fixtures`
- ten `mcp/tool_call` nodes for initialization and definition probes

The shell node declares:

- `validationEvidence.mode=controlled_substitute`
- `validationEvidence.observationType=controlled_substitute_observation`
- substitute path pattern under `/tmp/workorch-cwi-012-*`
- coverage target bound to the multi-language definition DoD item

The runtime still rejected the graph with a generic "must not mutate workspace files through shell/command" error. The deviation is not absence of todo generation; it is mismatch between controlled-substitute prompt contract and runtime enforcement.

Evidence: `ev:ws4e/run4/ctr-validation-003/output/001`.

## Contract Violation And Retry Timeline

| Run | Recorded Violations | Retry Requests | Final State |
| --- | ---: | ---: | --- |
| `run-001` | 2 | 2 | continued via replan |
| `run-001-replan-001` | 4 | 3 | continued via replan |
| `run-001-replan-001-replan-001` | 0 | 0 | continued via replan |
| `run-001-replan-001-replan-001-replan-001` | 3 | 2 | escalated decision |

The latest repeated violation type is `validation_evidence_required` / `concrete_validation_contract_missing`, with failure class `runtime_failed`.

Evidence: `ev:ws4e/events/all-runs/001`, `ev:ws4e/run4/ctr-validation-003/output/001`.

## Decision / Approval

The current decision exists because repeated same-signature validation contract failures exhausted automatic correction. This is a valid escalation point only if the contract itself is internally consistent. The current evidence suggests the contract is inconsistent for controlled substitute validation, so the next code closure should target contract enforcement before treating the workspace as a business-level blocker.

Evidence: `ev:ws4e/snapshot/current/001`.

## Artifact And Git State

The workspace produced real artifacts and commits:

- Commit `421ec47` `Expose runtime status probe surface`
- Commit `27b9622` `Add executable unsaved sync validation contract`
- `cmd/lsp-bridge/main.go`, 9157 bytes
- `lsp-bridge`, 2636836 bytes
- untracked `go.mod`
- untracked `lsp-bridge`

This confirms the run had meaningful state-changing output before the latest validation loop.

Evidence: `ev:ws4e/artifact/git/001`.

## LLM Actionability

The latest LLM output was actionable in structure: it supplied a validation graph, declared controlled-substitute metadata, and bound the focus to `cwi-012`. The runtime rejection did not distinguish workspace mutation from isolated temporary fixture creation. The missing piece is a runtime-owned scratch directory contract and enforcement rule.

Evidence: `ev:ws4e/run4/ctr-validation-003/output/001`.

## Projection

The projection shows the correct terminal state (`waiting_decision` / `blocked_on_decision`) and includes task progress, but it still presents mixed raw steps, tool calls, validation refs, result refs, and governance refs. It does not provide a clean planned-task view that separates planned graph nodes, dispatched steps, retry steps, and blocking decisions.

Evidence: `ev:ws4e/snapshot/current/001`.

## Resource

At audit time the shared `./bin/workorch` process was running with about `7.9%` CPU and `387936 KiB` RSS. No current high CPU or projection rebuild loop was observed in this sample.

Evidence: `ev:service/process/001`.

## Deviation Matrix

| Claim ID | Deviation | Owner | Evidence | Closure Batch |
| --- | --- | --- | --- | --- |
| `C-WS4E-CONTROLLED-SUBSTITUTE-001` | Controlled substitute validation is allowed by prompt contract but rejected by runtime shell mutation gate. | core runtime contract / validation gate | `ev:ws4e/run4/ctr-validation-003/output/001` | 3 |
| `C-WS4E-PLANNED-TASK-PROJECTION-001` | Execution graph nodes are visible only as mixed step/tool/progress items, not a first-class planned task projection. | projection / ViewModel | `ev:ws4e/snapshot/current/001` | 2 |
| `C-WS4E-RETRY-NARROWING-001` | Repeated same-type validation contract failures escalated, but correction did not resolve the contract/enforcement mismatch. | core retry correction | `ev:ws4e/events/all-runs/001` | 4 |

## Closure Gate Inputs

- Chain node: validation contract enforcement after LLM graph output.
- Deviation type: `controlled_substitute_enforcement_mismatch`.
- Evidence refs: `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:ws4e/events/all-runs/001`.
- Owner: core runtime contract and validation gate.
- Expected behavior: shell validation may create files only inside runtime-provided scratch directory when controlled-substitute metadata is complete.
- Actual behavior: shell validation was rejected with a generic mutation error even though it targeted `/tmp` fixture creation.
- Proposed change: define and enforce a runtime-owned validation scratch directory contract.
- Why this is not business-specific: the rule is generic validation evidence semantics; it does not encode LSP, language, Go, or the live workspace ID.
- Required tests: accept scratch-dir writes, reject workspace writes, reject writes outside scratch and workspace.
