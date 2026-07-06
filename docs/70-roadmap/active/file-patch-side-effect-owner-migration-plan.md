# File Patch Side Effect Owner Migration Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-06

## Contract Audit

- Scope: F-07 file / patch side-effect owner migration, covering `blocks/file`, `kernel/orchestrator`, patch evidence, approval, coverage ledger, projection and replay.
- Observed state: `docs/40-profiles-capabilities/runtime-capabilities/file-diff-patch.md` already states `patch/preview` and `patch/apply` should be executed by `blocks/file`, while `docs/30-module-architecture/blocks-runtime-adapters.md` still records the historical exception that `patch/*` is executed by `kernel/orchestrator` controlled patch executor. Code still has direct filesystem I/O in orchestrator patch and repair snapshot paths.
- Expected state: external filesystem side effects are executed by runtime block capability. Orchestrator remains the deterministic business authority for contract gate, approval gate, patch evidence recording, coverage ledger and result closeout.
- Deviation: patch side effect execution, patch evidence ownership and repair target snapshot acquisition are partially coupled inside orchestrator, making it too easy to bypass block ownership, approval-before-side-effect ordering, and runtime event/evidence replay boundaries.
- Owner: `blocks/file` owns file/patch technical execution; `kernel/orchestrator` owns runtime authority, approval decision, patch evidence acceptance, coverage ledger and result gate; `workorchd` owns projection.
- Event path: LLM candidate -> orchestrator contract gate -> approval gate -> runtime block execute `patch/*` or `file/read` -> block structured result -> orchestrator evidence recorder -> coverage ledger -> run/result/projection.
- Contract gap: no stable DTO boundary for patch execution result; no explicit migration boundary for patch record vs block result; target snapshot acquisition still reads workspace files directly from orchestrator; docs disagree on patch executor owner.
- Proposed change: migrate in phases. First define DTO and `blocks/file` patch execution capability without cutting the main path. Then route orchestrator patch execution through runtime block result while preserving formal patch record and projection contract. Finally remove old direct filesystem executor paths.
- Boundary check: no template/language/tool-command special cases in core. File/path safety is neutral runtime capability logic. Business interpretation stays in profile/Soul/policy and orchestrator contracts.
- Verification: per-phase unit tests, side-effect isolation tests, approval-before-apply tests, patch evidence/projection tests, boundary search and live verification.

### Phase 6 Contract Audit

- Scope: `kernel/orchestrator` legacy patch apply/revert executor paths after `patch/apply` has been routed through `blocks/file`.
- Observed state: orchestrator can still admit direct `patches` output by calling `applyWorkspacePatch`, and can still execute `patchRevert` by `os.WriteFile` / `os.Remove`. The old `PatchRuntimeAdapter` also remains as a kernel-owned patch runtime adapter.
- Expected state: orchestrator only admits completed `blocks/file` `patch/apply` block results into the formal Patch record. Direct patch candidates may be used for governance/approval context, but must not perform filesystem side effects in orchestrator. `patch/revert` remains phase-gated until formal reverse material is available to `blocks/file`.
- Deviation: legacy fallback paths can still make orchestrator the filesystem side-effect owner, conflicting with the target ownership model.
- Owner: `blocks/file` owns patch technical execution; `kernel/orchestrator` owns approval, admission, formal Patch evidence and ledger binding.
- Event path: candidate output / approved pending patch -> runtime block `patch/apply` -> completed block result -> orchestrator formal Patch admission. Direct orchestrator write/revert paths must be rejected before side effect.
- Contract gap: missing hard rejection for non-block patch side-effect output and legacy adapter export.
- Proposed change: reject direct non-block patch application in `patchSubsystem.apply`, phase-gate `patchSubsystem.revert`, remove the old kernel `PatchRuntimeAdapter`, and keep parser/preflight helpers read-only until a later neutral relocation.
- Boundary check: no template/language/tool-command special cases; this only moves ownership of generic patch side effects to `blocks/file`.
- Verification: targeted patch tests for direct-output rejection, block-result admission, phase-gated revert, file block patch behavior, plus boundary scan for kernel-owned patch runtime adapter removal.

### Phase 5 Contract Audit

- Scope: `kernel/orchestrator/biz/runtime_repair_context_material.go` target snapshot materialization for repair context, patch suggestions, already-satisfied signals, resource-shape candidates and converted-decision readable resources.
- Observed state: repair context first consumes successful runtime-managed `file/read` snapshots, but still falls back to `os.ReadFile` in orchestrator when no such runtime result exists. The fallback marks snapshots diagnostic-only, yet still materializes target content outside `blocks/file`.
- Expected state: target snapshot content is only acquired through runtime-managed `file/read`. Orchestrator may request acquisition and consume successful block output, but must not read workspace files directly to build repair material.
- Deviation: direct fallback reads let repair prompt material bypass the runtime block owner, so patch guidance can be based on content that is not represented as replayable block evidence.
- Owner: `blocks/file` owns target file reads and content/hash metadata; `kernel/orchestrator` owns acquisition request, consumption of block output and repair contract rendering.
- Event path: repair/current-action needs target snapshot -> orchestrator requires runtime-managed `file/read` acquisition -> block emits structured `file_read` output -> orchestrator reuses that output in repairActionPackage/context -> patch or validation proceeds through normal gates.
- Contract gap: absent runtime-managed snapshot should produce an acquisition-required placeholder or no exact-content signal, not direct content/hash material.
- Proposed change: remove all `os.ReadFile` target snapshot paths from `runtime_repair_context_material.go`; keep exact suggestions/hash-drift/already-satisfied/resource-content signals only when a successful runtime-managed `file/read` output exists; add boundary verification for this file.
- Boundary check: no template/language/tool-command special cases; this only enforces the neutral file/read block ownership boundary.
- Verification: update repair context tests, run orchestrator/file block tests, `make fmt-check`, `git diff --check` and `scripts/verify-boundary-contract.sh`.

## Target Ownership Model

### `blocks/file`

Owns technical execution and safety checks:

- `file/list`
- `file/read`
- `file/edit`
- `patch/preview`
- `patch/apply`
- `patch/revert`
- workspace relative path normalization
- absolute path / `..` / symlink escape prevention
- file existence, directory, binary and size checks
- `baseHash`, unified diff context and `oldText` conflict detection
- technical result normalization

Does not own:

- Run / Step terminal state
- Result acceptance
- Goal / DoD closure
- Coverage ledger satisfaction
- Approval creation or resolution
- formal patch record truth source

### `kernel/orchestrator`

Owns deterministic authority:

- `requiredOutput` and current action rendering
- contract gate before any side effect
- approval gate before state-changing side effect
- runtime block dispatch and result admission
- formal Patch evidence record
- CoverageWorkItem accepted/rejected evidence binding
- Result / Artifact closeout
- rejected diagnostic evidence for invalid output
- NextAction recompute

Does not own:

- raw filesystem writes
- raw filesystem patch preview/apply mechanics
- target snapshot raw file reads
- path traversal and symlink policy implementation beyond consuming block result diagnostics

### `workorchd` / Projection

Owns read surface:

- patch list/detail projection
- approval relation projection
- run observability timeline
- workspace/run snapshot
- SSE/projection revision updates

Does not own:

- patch business state transitions
- patch execution
- completion or DoD closure judgment

## DTO Contract

### PatchExecutionRequest

Stable request fields:

| Field | Required | Notes |
| --- | --- | --- |
| `workspaceId` | yes | scope only |
| `runId` | yes | scope only |
| `stepId` | yes | evidence lineage |
| `operation` | yes | `preview` / `apply` / `revert` |
| `patchType` | for preview/apply | `file_write` / `unified_diff` / `text_replace` |
| `targetRef` | for preview/apply | workspace relative path |
| `content` | file_write | desired content |
| `diff` | unified_diff | single or multi hunk diff when supported |
| `oldText` | text_replace | exact current text |
| `newText` | text_replace | replacement |
| `baseHash` | optional | conflict guard |
| `occurrence` | optional | text replacement selector |
| `patchId` | for revert | formal patch record id |
| `approvalId` | optional | already approved approval id |
| `coverageTargets` | optional | copied through for evidence binding |
| `maxBytes` | optional | preview/read truncation guard |

### PatchExecutionResult

Stable result fields:

| Field | Notes |
| --- | --- |
| `status` | `completed` / `failed` |
| `stage` | `patch_preview` / `patch_apply` / `patch_revert` |
| `operation` | copied from request |
| `patchType` | copied from request |
| `targetRef` | normalized workspace relative path |
| `affectedFiles` | normalized paths |
| `beforeHash` | current target hash before apply when available |
| `afterHash` | target hash after apply when available |
| `contentHash` | preview/write content hash when applicable |
| `diffHash` | normalized diff hash when applicable |
| `addedLines` | integer |
| `deletedLines` | integer |
| `conflict` | structured conflict object |
| `approvalRequired` | bool |
| `riskLevel` | neutral risk level |
| `riskReasons` | neutral reason strings |
| `diagnostics` | structured technical diagnostics |
| `rawResultRef` | optional raw/detail ref |

The block result is not the formal Patch record. The formal Patch record is still created or updated by orchestrator after the result is admitted.

## Runtime Flow

### Preview

```text
LLM candidate patch preview
-> orchestrator validates stage/requiredOutput
-> orchestrator dispatches patch/preview to blocks/file
-> blocks/file returns PatchExecutionResult
-> orchestrator records preview evidence and approval requirement if needed
-> projection shows preview summary
```

### Apply

```text
LLM candidate patch apply
-> orchestrator validates state-changing action allowed
-> orchestrator validates approval gate before dispatch
-> orchestrator dispatches patch/apply to blocks/file
-> blocks/file performs filesystem side effect
-> blocks/file returns PatchExecutionResult
-> orchestrator records formal Patch evidence
-> orchestrator updates coverage ledger
-> orchestrator recomputes NextAction
-> projection consumes patch/result/evidence events
```

### Revert

```text
revert request
-> orchestrator validates patch record and approval gate
-> orchestrator dispatches patch/revert to blocks/file
-> blocks/file applies reverse operation
-> orchestrator records revert evidence and relation
-> projection updates patch relation
```

### Target Snapshot

```text
repair/current-action needs target snapshot
-> orchestrator requests runtime-managed file/read
-> blocks/file reads bounded content
-> result includes contentHash/truncated/isBinary/safeForPatch
-> orchestrator references snapshot evidence in repairActionPackage
```

No repair path should call raw `os.ReadFile` in orchestrator to materialize target content.

## Non-Negotiable Gates

1. Contract gate before side effect.
   - Invalid LLM output must produce rejected diagnostic evidence only.
   - Invalid output must not dispatch `patch/apply`.

2. Approval gate before side effect.
   - `approvalRequired=true` or policy-required patch must block before apply.
   - Approval creation remains orchestrator/governance responsibility.

3. Single formal patch truth source.
   - Block result is execution result.
   - Formal `Patch` record remains kernel evidence owner state.
   - Do not introduce parallel patch state in `blocks/file`.

4. Coverage ledger only accepts admitted evidence.
   - Failed/conflicted patch result is rejected diagnostic or failed technical evidence.
   - It cannot satisfy state_change coverage.

5. Projection only consumes owner state and admitted events.
   - Web/TUI must not infer patch success from raw block output alone.

## Phase Plan

### Phase 0: Design And Contract Alignment

- [x] Update `docs/30-module-architecture/blocks-runtime-adapters.md` so `patch/*` final target owner is `blocks/file`, while orchestrator remains formal authority.
- [x] Update `docs/40-profiles-capabilities/runtime-capabilities/file-diff-patch.md` to clarify block result vs formal Patch record.
- [x] Add DTO contract notes to this document or a stable runtime capability doc.
- [x] Add boundary verification target for orchestrator raw patch filesystem side effects after migration is complete.

Exit criteria:

- Docs no longer conflict on patch executor owner.
- Migration phases and rollback points are explicit.

### Phase 1: Add `blocks/file` Patch Execution Capability

- [x] Move or duplicate neutral patch mechanics into `blocks/file` behind runtime adapter methods.
- [x] Support `patch/preview`.
- [x] Support `patch/apply`.
- [x] Support `patch/revert` if current formal revert contract has enough reverse material; otherwise mark revert as phase-gated and keep formal revert blocked until evidence is available.
- [x] Return `PatchExecutionResult` shape from block output.
- [x] Add block unit tests for:
  - safe relative path
  - absolute path rejection
  - `..` rejection
  - symlink escape rejection
  - missing parent conflict
  - `baseHash` mismatch
  - `oldText` not found
  - multiple oldText without occurrence
  - binary target rejection
  - preview does not write
  - apply writes exactly once

Implementation note: `patch/revert` is explicitly phase-gated in `blocks/file` until orchestrator can provide formal reverse material from the Patch record.

Exit criteria:

- `go test ./internal/modules/blocks/file/biz -count=1` passes.
- No orchestrator caller uses the new path yet.

Verification note:

- 2026-07-06: `go test ./internal/modules/blocks/file/biz -count=1` passed.

### Phase 2: Orchestrator Dispatches Patch Through Runtime Block

- [x] Introduce a narrow `patchRuntimeExecutor` interface in orchestrator.
- [x] Route `patch/preview` through runtime block execution result.
- [x] Route `patch/apply` through runtime block execution result.
- [x] Preserve current formal Patch repository write location in orchestrator.
- [x] Preserve patch event publication order.
- [x] Convert block conflicts to current recoverable diagnostics.
- [x] Ensure duplicate runtime completed events do not duplicate side effects or Patch records.

Implementation note: this pass uses the existing runtime event executor contract as the narrow execution interface. Production `patch/*` events are now handled by `blocks/file`; orchestrator no longer starts a kernel-owned patch runtime executor. Orchestrator admits successful block `patch/apply` output into the formal Patch repository without reapplying the filesystem side effect.

Exit criteria:

- Existing patch integration tests still pass.
- New tests prove `patch/apply` side effect happens through block executor, not direct helper.
- Contract violation tests prove invalid patch output does not dispatch block side effect.

Verification note:

- 2026-07-06: added targeted tests for applied block result admission without reapply and preflight approval before side effect.
- 2026-07-06: `go test ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/blocks/file/... -count=1` passed.

### Phase 3: Approval And Risk Contract Closure

- [x] Treat block `approvalRequired/riskReasons` as input to orchestrator approval gate.
- [x] Ensure apply is blocked before side effect when approval is required and unresolved.
- [x] Ensure approval resolution resumes the same patch action with current target snapshot or conflict check.
- [x] Ensure approval context keeps `patchType`, `targetRef`, risk, affected files and baseHash.

Implementation note: dispatch-time `patch/apply` preflight now evaluates orchestrator risk policy before runtime block execution. High-risk patch input creates an approval and blocks the run before `blocks/file` can perform a side effect.
Block `patch/preview` output with `approvalRequired=true` now also enters the orchestrator approval gate, carrying `patchType`, `targetRef`, `affectedFiles`, risk fields, hashes and line stats into the approval context.
Approved pending patch application now executes the same patch request through the runtime `patch/apply` block before orchestrator admits the formal Patch record, preserving the current block-side `baseHash` / diff conflict checks.

Exit criteria:

- Tests cover high-risk preview -> approval -> approved apply.
- Tests cover rejected/unresolved approval -> no file write.

### Phase 4: Formal Evidence And Coverage Closure

- [x] Formal Patch record stores block result hashes, affected files, approval relation and diagnostics.
- [x] Coverage ledger accepts `state_change` only for admitted successful patch apply evidence.
- [x] Coverage ledger rejects conflicted/failed block result as diagnostic.
- [x] Result / Artifact closeout references formal Patch record, not raw block result.

Implementation note: formal `Patch` now carries block-admitted `affectedFiles`, line stats, risk and diagnostics together with existing hash and approval lineage. Accepted `state_change` evidence is created only after orchestrator admits a successful `patch/apply` formal Patch record, and the evidence record points back to that Patch ID as `SourceRef`. Preview-only and failed block outputs do not create formal Patch records or accepted `state_change` evidence. Result/artifact closeout continues to require accepted formal Patch IDs before generating patch diff artifacts.

Exit criteria:

- Patch evidence appears in run snapshot and observability.
- DoD state_change coverage cannot be satisfied by preview-only or failed apply.

Verification note:

- 2026-07-06: added targeted tests for formal Patch block-result metadata, accepted state_change evidence SourceRef binding, and preview/failed block outputs not creating accepted state_change evidence.
- 2026-07-06: `go test ./internal/modules/kernel/orchestrator/biz -run 'TestPatchSubsystemAdmitsAppliedBlockResultWithoutReapplying|TestPatchPreviewAndFailedBlockResultDoNotCreateAcceptedStateChangeEvidence|TestRuntimeOrchestratorApplyRuntimePatchRecordsAcceptedStateChangeEvidence' -count=1` passed.
- 2026-07-06: `go test ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/blocks/file/... -count=1` passed.

### Phase 5: Repair Target Snapshot Migration

- [x] Replace direct target snapshot reads in `runtime_repair_context_material.go` with runtime-managed `file/read` acquisition.
- [x] Snapshot evidence includes `targetRef`, `contentHash`, `truncated`, `isBinary`, `safeForPatch`, `sourceStepId`, `evidenceRef`.
- [x] RepairActionPackage only marks exact patch/write safe when snapshot is complete and not redacted/truncated.
- [x] If snapshot cannot be acquired, NextAction becomes read_required / blocker / fail-fast according to context gate, not guessed patch.

Progress note: `blocks/file` `file/read` now emits `targetRef`, `contentComplete`, `safeForPatch`, `safeForFileWrite`, `redacted`, `truncated`, and `isBinary`. Target snapshot EvidenceRecord metadata now carries `evidenceRef` and `safeForPatch`. `runtimeRepairPatchContext` now consumes prior runtime-managed `file/read` target snapshot step output; when no runtime-managed snapshot exists, it emits an acquisition-required placeholder without `content` or `contentHash`.

Progress note: Repair action package exact-patch authority now uses one safety predicate: a snapshot must be complete, hash-bound, non-binary, not redacted/truncated, and runtime-managed before it can set `mustUseCurrentTargetSnapshot` / `mustPatchDirectlyFromProvidedSnapshot`. Unsafe or missing snapshots switch required output to runtime-managed `file/read` acquisition with `stateChangingForbidden=true`, and target snapshot acquisition convergence is represented as a read-only acquisition contract rather than guessed patch content.

Verification note:

- 2026-07-06: removed production `os.ReadFile` target snapshot materialization from `runtime_repair_context_material.go`; exact patch suggestions, current-hash refresh signals, already-satisfied signals, resource-shape context and readable-resource decision context now require prior runtime-managed `file/read` output.
- 2026-07-06: `go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*Repair.*Context|Test.*TargetSnapshot|Test.*Patch.*|TestRecoveryActionAllows.*Patch|TestRuntimeRepairInputPromotesReadableResourceDecisionIntoStateChangingTargetSnapshot' -count=1` passed.
- 2026-07-06: `go test ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/file/... -count=1` passed.

Exit criteria:

- Tests prove truncated/redacted snapshot cannot be used for exact text replacement.
- Tests prove current snapshot hash is visible in retry context.

### Phase 6: Remove Old Direct Filesystem Executor Path

- [x] Delete or quarantine `applyWorkspacePatch` direct write path.
- [x] Delete or quarantine direct `previewWorkspacePatch` raw file path if superseded.
- [x] Remove production `os.ReadFile` / `os.WriteFile` / `os.MkdirAll` from orchestrator patch side-effect path.
- [x] Keep filesystem I/O in tests only, or in non-side-effect repository/readmodel code where explicitly owned.
- [x] Add static boundary check for orchestrator patch side-effect I/O.

Progress note: The old kernel `PatchRuntimeAdapter` export and direct `applyWorkspacePatch` / `applyWorkspaceFileWrite` / `applyWorkspaceTextReplace` / `applyWorkspaceUnifiedDiff` write paths have been removed. Direct patch candidates are now synchronously executed through the configured runtime `patch/apply` block and then admitted as formal Patch evidence; standalone `patchSubsystem.apply` rejects non-block side-effect output. `patch/revert` is phase-gated until formal reverse material is available to `blocks/file`. Orchestrator keeps read-only preview/preflight helpers only for governance context and parser diagnostics, not for filesystem mutation.

Exit criteria:

- Boundary scan proves no production orchestrator patch side-effect raw file I/O remains.
- Full orchestrator tests pass.

### Phase 7: Projection, Replay And Live Verification

- [x] Verify event stream still replays Patch evidence and projection state.
- [x] Verify workorchd patch/read-model projection consumes formal Patch records.
- [x] Verify Web/TUI do not infer success from block output.
- [x] Run `make verify-workspace-file-diff-patch`.
- [x] Ensure exhausted contract-violation recovery creates a governed blocker instead of terminal failed when automatic patch/state-change retry budget is exhausted.
- [ ] Run deployed live validation for a workspace requiring patch apply + validation + DoD closeout.

Exit criteria:

- Live workspace progresses through state_change -> validation -> assessment -> complete or governed blocker.
- No `run=active + currentStep=failed + no blocker + no next action`.
- CPU/IO does not regress due to repeated full snapshot reads.

Progress note:

- 2026-07-06: `make verify-workspace-file-diff-patch` passed. This covers block patch mechanics, formal Patch event replay/projection, run snapshot/observability patch evidence, and TUI/CLI patch evidence rendering from formal Patch records.
- 2026-07-06: deployed `ws-705249a8d7816330/run-083` reached formal Patch evidence and projection (`patchTotal=2`, `patchApplied=2`, `/patch/applied` events present), proving the file block side-effect owner migration path is active in live execution.
- 2026-07-06: live closeout exposed a remaining orchestrator governance gap after Patch evidence: bounded contract-violation retry exhaustion could still be rejected by the mechanical decision gate and become terminal failed instead of a governed blocker. This is not a `blocks/file` patch side-effect ownership issue, but it is required by Phase 7 live exit criteria. The decision eligibility contract now allows `runtimeRepairLoopState=contract_violation_recovery_exhausted` to create the governed blocker while keeping ordinary mechanical state-changing decisions blocked.
- 2026-07-06: deployed service later became unreachable on `127.0.0.1:8080` during verification polling, so the final deployed live DoD closeout checkbox remains open until the latest code is redeployed and the live scenario is rerun.

## Rollback Strategy

Each phase must be mergeable independently:

- Phase 1 can exist unused.
- Phase 2 must be guarded by tests before deleting old executor.
- Phase 5 target snapshot migration must retain deterministic failure behavior when block read fails.
- Phase 6 deletion happens only after Phase 2-5 tests and live verification pass.

Do not keep a runtime feature flag for compatibility unless a later release explicitly requires it. Current stage does not require compatibility; rollback should be via git revert of the phase commit, not via permanent dual-path business logic.

## Verification Matrix

| Area | Required checks |
| --- | --- |
| block patch mechanics | `go test ./internal/modules/blocks/file/biz -count=1` |
| orchestrator patch admission | targeted orchestrator patch/contract tests |
| approval ordering | unresolved approval must prevent write |
| evidence ledger | patch apply accepted, preview/failed rejected |
| projection | run snapshot, observability, patch detail |
| boundary | no production orchestrator raw patch filesystem side effect |
| live | deployed workspace completes state_change/validation/assessment chain |

## Initial Code Targets

Likely touched paths:

- `internal/modules/blocks/file/biz/file_runtime_adapter.go`
- `internal/modules/kernel/orchestrator/biz/runtime_patch_filesystem.go`
- `internal/modules/kernel/orchestrator/biz/runtime_patch_unified_diff.go`
- `internal/modules/kernel/orchestrator/biz/runtime_patches.go`
- `internal/modules/kernel/orchestrator/biz/runtime_patch_approval.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_context_material.go`
- `internal/modules/kernel/orchestrator/biz/runtime_events.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/*patch*`
- `internal/pkg/workorchruntimecontract/*`

Non-goal in early phases:

- Do not rewrite Web/TUI patch UI.
- Do not change software-dev template policy.
- Do not make Result acceptance depend on block output directly.
- Do not add orchestrator special cases for specific file names, languages or commands.

## Resolved Questions

1. `patch/revert` remains phase-gated until formal reverse material is available to `blocks/file`. Orchestrator may keep formal revert governance, but must not perform raw revert side effects.
2. `PatchExecutionRequest` remains block-local for this migration. The formal cross-module boundary is the runtime step input/output DTO and the admitted kernel Patch record.
3. Target snapshot acquisition is represented as runtime-managed `file/read` evidence. Repair context may consume that evidence but must not materialize target content through direct orchestrator file reads.
4. Static boundary checks block orchestrator-owned patch side-effect I/O and direct repair target snapshot reads. Read-only preflight helpers may remain only where they are explicitly governance/parser diagnostics and do not execute side effects.

These answers are now reflected in Phase 1-7 implementation notes and boundary verification.
