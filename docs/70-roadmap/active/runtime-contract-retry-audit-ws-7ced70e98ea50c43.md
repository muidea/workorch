# Runtime Contract Retry Audit: ws-7ced70e98ea50c43

## Scope

This is the L1 audit summary for `ws-7ced70e98ea50c43` (`轻量级 LLM 本地代理网关`). It records the execution chain and deviation classes without embedding raw event streams or full LLM payloads.

Raw evidence is indexed in `runtime-contract-retry-evidence-index.md`. Conclusions in this document must cite L2 evidence IDs.

## Workspace State

| Field | Value |
| --- | --- |
| Workspace | `ws-7ced70e98ea50c43` |
| Title | `轻量级 LLM 本地代理网关` |
| Template/Profile | `software-dev` / `software-dev` |
| Active run | `run-001-replan-001-replan-001-replan-001` |
| Workspace state | `waiting_decision` |
| Run state | `blocked_on_decision` |
| Current step | `contract-retry-001-of-planning-001-5` |
| Blocking ticket | `run-001-replan-001-replan-001-replan-001-contract-retry-001-of-planning-001-5-contract-violation-decision` |

Projection was aligned at the audit point for this workspace: `sourceEventRevision=3031`, `projectedSourceRevision=3031`.

Evidence: `ev:service/readyz/001`, `ev:ws7c/snapshot/current/001`.

## Goal / DoD / Coverage

The current active focus is the provider routing and fallback coverage around `cwi-005`. The ledger summary at the audit point was:

| Total | Satisfied | In Progress | Failed | Open |
| --- | --- | --- | --- | --- |
| 89 | 2 | 1 | 26 | 60 |

The immediate gap is not absence of any code: `main.go`, `main_test.go`, and `go.mod` exist. The blocker is repeated failure to produce a valid state-changing repair after the focused validation failure.

Evidence: `ev:ws7c/snapshot/current/001`, `ev:ws7c/artifact/git/001`.

## Run Timeline

| Run | Time Range UTC | Event Count | Main Observations |
| --- | --- | ---: | --- |
| `run-001` | 2026-06-20 15:43:40 to 16:22:32 | 252 | Initial minimal Go proxy skeleton and tests were produced. |
| `run-001-replan-001` | 2026-06-20 16:22:32 to 2026-06-21 00:35:56 | 42 | A focused test patch was attempted but rejected for missing coverage targets. |
| `run-001-replan-001-replan-001` | 2026-06-21 00:35:56 to 02:30:42 | 98 | A retry added coverage targets, but provider/model prefix validation still failed. |
| `run-001-replan-001-replan-001-replan-001` | 2026-06-21 02:30:42 to 02:45:37 | 181 | Repeated contract retry loop ended in decision after five retry requests. |

Evidence: `ev:ws7c/events/all-runs/001`.

## Key Steps

| Run | Step | State | Contract / Failure | Observation |
| --- | --- | --- | --- | --- |
| `run-001` | `021-graph-004-003-validate-focus-cwi-005` | failed | `shell_command_failed` | Focused provider routing test failed. |
| `run-001-replan-001` | `002-graph-001-001-patch-main-test-focus-cwi-005` | failed | `coverage_targets_required` | State-changing patch missed required coverage binding. |
| `run-001-replan-001-replan-001` | `002-graph-001-001-patch-main-test-focus-cwi-005-retry` | completed | none | Retried test patch completed. |
| `run-001-replan-001-replan-001` | `004-repair-001-of-graph-001-002-verify-provider-model-prefix-focus` | failed | `patch_dispatch_invalid` | Runtime rejected `file_write` replacing existing `main.go`. |
| `run-001-replan-001-replan-001-replan-001` | `003-contract-retry-001-of-planning-001-2` | failed | `patch_dispatch_invalid` | Same file-write/existing-target issue remained in retry. |
| `run-001-replan-001-replan-001-replan-001` | `006-contract-retry-001-of-planning-001-5` | failed | `state_changing_required_missing` | LLM returned DoD assessment instead of state-changing output and claimed missing snapshots. |

Evidence: `ev:ws7c/steps/summary/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.

## Todo / ExecutionGraph Mapping

The workspace initially produced a meaningful execution path:

- minimal source files were created
- a focused provider/model prefix test was added
- validation failed against that test
- runtime repair tried to modify `main.go`

The latest run degraded into repeated contract retries. The final output contains no execution graph nodes and three DoD assessment items, even though the repair contract required state-changing output.

Evidence: `ev:ws7c/steps/summary/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.

## Contract Violation And Retry Timeline

| Run | Recorded Violations | Retry Requests | Final State |
| --- | ---: | ---: | --- |
| `run-001` | 0 | 0 | continued via replan |
| `run-001-replan-001` | 0 | 0 | focused patch rejected |
| `run-001-replan-001-replan-001` | 1 | 1 | continued via replan |
| `run-001-replan-001-replan-001-replan-001` | 6 | 5 | escalated decision |

The latest violation type is `repair_no_state_changing_output` / `state_changing_required_missing`, with failure class `dod_coverage_missing`.

Evidence: `ev:ws7c/events/all-runs/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.

## Decision / Approval

The current decision exists because the same target scope exhausted repair budget and repeated state-changing contract violations. The escalation mechanism worked, but the preceding retry correction did not narrow the model toward the already available target snapshots and allowed patch modes.

Evidence: `ev:ws7c/snapshot/current/001`.

## Artifact And Git State

The workspace contains uncommitted source skeleton files and no git commits:

- untracked `go.mod`, 27 bytes
- untracked `main.go`, 2162 bytes
- untracked `main_test.go`, 3844 bytes
- git log: no commits yet

This confirms early state-changing output happened, but no version-control snapshot was accepted.

Evidence: `ev:ws7c/artifact/git/001`.

## LLM Actionability

The latest LLM output claims that it lacks the current target snapshot text and enough diagnostics to safely patch. That claim conflicts with the latest request evidence:

- `main.go` was present with `contentComplete=true`, `safeForFileWrite=true`, size `2162`, and source `patch_repair_context`.
- `main_test.go` was present with `contentComplete=true`, `safeForFileWrite=true`, size `3844`, and sources `failure_target_current_workspace_file` / `current_workspace_file`.
- The request contained many references to `main_test.go` and `main.go`.

The deviation is not missing request data. It is that the state-changing retry contract does not make available snapshots primary and does not classify "snapshot missing" claims as a specific violation when snapshots are present.

Evidence: `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.

## Projection

The projection shows `waiting_decision` / `blocked_on_decision`, but planned tasks and retry attempts remain mixed with raw step/tool/validation refs. A consumer can see the workspace is blocked, but not cleanly see the intended todo list, which tasks were rejected, and which retry correction path was exhausted.

Evidence: `ev:ws7c/snapshot/current/001`.

## Resource

At audit time the shared `./bin/workorch` process was running with about `7.9%` CPU and `387936 KiB` RSS. No current high CPU or projection rebuild loop was observed in this sample.

Evidence: `ev:service/process/001`.

## Deviation Matrix

| Claim ID | Deviation | Owner | Evidence | Closure Batch |
| --- | --- | --- | --- | --- |
| `C-WS7C-STATECHANGE-SNAPSHOT-001` | LLM claimed target snapshots were missing although complete snapshots were present in the request. | core LLM request assembly / retry classifier | `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/output/001` | 5 |
| `C-WS7C-RETRY-NARROWING-001` | Repeated same-scope retry did not force a narrow patch correction based on available snapshots and allowed patch modes. | core retry correction | `ev:ws7c/events/all-runs/001` | 4 |
| `C-WS7C-PATCH-CONTRACT-001` | Existing target replacement through `file_write` was rejected, but retry did not consistently move to exact patch modes. | core patch contract / retry correction | `ev:ws7c/steps/summary/001` | 3 |
| `C-WS7C-PLANNED-TASK-PROJECTION-001` | The user-facing projection does not show a clean todo/retry task plan separate from raw runtime items. | projection / ViewModel | `ev:ws7c/snapshot/current/001` | 2 |

## Closure Gate Inputs

- Chain node: state-changing repair retry after focused validation failure.
- Deviation type: `available_snapshot_ignored`.
- Evidence refs: `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Owner: core LLM request assembly, contract violation classifier, retry correction generator.
- Expected behavior: when complete target snapshots are present, retry requires `patch/apply`, allowed state-changing output, or a real blocker that is not "snapshot missing".
- Actual behavior: LLM returned DoD assessment and claimed missing snapshots despite complete snapshots in the request.
- Proposed change: add available snapshot summary near current-action, classify false missing-snapshot claims as `available_snapshot_ignored`, and generate a narrow correction contract.
- Why this is not business-specific: the rule applies to any target file or resource snapshot; `main.go` and `main_test.go` are evidence samples only.
- Required tests: request with complete snapshots plus output claiming missing snapshot is rejected with the new classifier; retry correction cites available targets and allowed patch modes.
