# Convergence Authority Focus Drift Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-26

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-dispatch-closure-checklist.md`
- `../../60-verification/convergence-authority-dispatch-live-verification.md`

## Purpose

This document records the latest live deviation after convergence authority
dispatch closure and turns it into a focused implementation closure plan.

The previous dispatch closure proved that workorchd can refresh
`convergenceAuthority`, `gapBaseline`, and `stagePacket` before dispatch. The
new live evidence shows that this is not sufficient: older derived fields such
as `acceptance`, `coverageFocus`, and `repairActionPackage.requiredOutput` can
still remain in the LLM/runtime step input and conflict with the current
authority snapshot.

The closure target is:

```text
Coverage/Evidence Ledger
  -> one convergenceAuthority snapshot
  -> gapBaseline / convergenceContract / Stage Packet / requiredOutput
  -> LLM visible C3/C4 context
  -> contract gate
```

Every field visible to LLM or runtime gates must either be derived from the
current authority snapshot or explicitly marked as diagnostic lineage. No stale
focus field may remain as an action authority.

Current stage does not require backward compatibility.

## Contract Audit

- Scope: live workspaces `ws-4e83810542ac4d65` and
  `ws-7ced70e98ea50c43`, active LLM/runtime dispatch input, Stage Packet,
  `acceptance`, `coverageFocus`, `repairActionPackage.requiredOutput`, prompt
  C4 signals, projection metadata, and resource-envelope evidence.
- Observed state:
  - Service is ready.
  - Workspace-level ViewModel projection is ready and aligned for both
    workspaces. TUI/Web prompt mode correctly shows decision after the latest
    failures.
  - EventHub publish failure delta is zero and projection schedule drop delta is
    zero.
  - Both workspaces made in-flight progress during live verification, but both
    eventually entered `waiting_decision` / `blocked_on_decision`.
  - `ws-4e83810542ac4d65` failed with
    `dod_assessment_missing_evidence_refs`.
  - `ws-7ced70e98ea50c43` failed with
    `concrete_validation_contract_missing`.
  - Live verification artifact:
    `/tmp/workorch-live-audit-20260626105833`.
- Expected state:
  - Before every LLM/runtime dispatch, `convergenceAuthority`, `gapBaseline`,
    `convergenceContract`, `stagePacket`, `requiredOutput`, `acceptance`, and
    `coverageFocus` must describe the same current focus and NextAction.
  - LLM C3/C4 must expose exactly one current focus and one current
    requiredOutput.
  - Old failed-step input, old repair packages, old replan continuation, old
    acceptance slices, and old coverage focus may appear only as diagnostic
    lineage, never as current action authority.
- Deviation:
  - `ws-4e83810542ac4d65` current step input had:
    - `convergenceAuthority.currentFocus = cwi-001`
    - `gapBaseline.currentFocus = cwi-001`
    - `stagePacket.activeFocus = cwi-001`
    - but `acceptance.currentFocus = focus-cwi-039`
    - and `coverageFocus.id = focus-cwi-039`
  - `ws-7ced70e98ea50c43` current step input had:
    - `convergenceAuthority.currentFocus = cwi-020`
    - `gapBaseline.currentFocus = cwi-020`
    - `stagePacket.activeFocus = cwi-020`
    - but `acceptance.currentFocus = focus-cwi-085`
    - `coverageFocus.id = focus-cwi-085`
    - and `stagePacket.requiredOutput.coverageFocusId = focus-cwi-085`
  - Stage Packet can therefore carry a new `activeFocus` while its
    `requiredOutput` still binds an old focus.
  - Prompt C4 still injects `acceptance-focus` as a separate LLM-visible signal,
    so the LLM sees competing current-focus instructions.
  - Process resource usage remains high: live verification reported about
    197% CPU, about 1GB RSS, about 49MB write delta, about 119k metadata records
    scanned, and about 80MB payload hydrate bytes in 60 seconds.
- Owner: WorkOrch core.
  - Primary owner: orchestrator dispatch input and Stage Packet builders.
  - Secondary owner: LLM prompt/context renderer and read model projection.
  - Resource-envelope owner: event record read model and observability/detail
    snapshot hydration.
  - This is not a workspaceTemplate-specific rule, not a provider-only issue,
    not a TUI-owned state machine issue, and not caused solely by unstable LLM
    output.
- Event path:
  - TUI/Web/REST command -> workorchd scoped command/EventHub -> owner state ->
    runtime scheduler -> convergence authority refresh -> dispatch input
    rebuild -> Stage Packet and C3/C4 prompt rendering -> LLM/block output ->
    contract gate -> accepted/rejected evidence -> coverage ledger recompute ->
    projection/ViewModel.
- Contract gap:
  - Provider-before-dispatch integrity gate does not validate every
    LLM-visible focus-bearing field.
  - `runExecutionInput` still copies stale `acceptance` and `coverageFocus`
    from `run.ExecutionContext` into current dispatch input.
  - `stagePacketRequiredOutput` can read stale `repairActionPackage` before the
    current authority-derived required output is rebuilt.
  - Prompt C4 `acceptance-focus` can reintroduce stale focus after Stage Packet
    has selected the current focus.
  - Projection can display current readiness, but it does not yet surface this
    authority-vs-derived-field focus drift as a first-class diagnostic.
- Boundary check:
  - All closures must stay business-neutral.
  - Core may compare structured focus IDs, coverage work item IDs, NextAction
    kind, requiredOutput focus binding, Stage Packet fields, and diagnostics.
  - Core must not branch on template ID, Soul ID, programming language, command
    name, file name, provider brand, workspace title, or live workspace ID.
- Verification:
  - Add failing sample tests that construct stale `acceptance` /
    `coverageFocus` / `requiredOutput.coverageFocusId` with a fresh authority
    focus and prove dispatch rebuild aligns or rejects them before provider
    call.
  - Add prompt rendering tests that prove C4 cannot expose stale
    `acceptance-focus` when Stage Packet authority exists.
  - Add projection diagnostics tests for authority focus drift.
  - Re-run targeted orchestrator, LLM prompt/context, projection tests, then
    live verification against at least two workspaces.

## Root Cause

The implementation completed only a partial authority closure.

`convergenceAuthority`, `gapBaseline`, and part of `stagePacket` are refreshed
before dispatch, but older derived fields are still copied into the same input
and still rendered to the LLM. This leaves multiple action authorities in one
request:

- current authority focus from `convergenceAuthority`
- current projected focus from `gapBaseline`
- old acceptance slice focus from `acceptance.currentFocus`
- old runtime focus from `coverageFocus`
- old focus binding from `repairActionPackage.requiredOutput.coverageFocusId`
- old focus signal from prompt C4 `acceptance-focus`

When these disagree, LLM output can be structurally valid for one focus but fail
runtime contract validation for another focus. The repeated LLM contract
violations are therefore a symptom amplified by conflicting runtime input, not
the sole cause.

## Code Cause Map

### Dispatch Guard

Current path:

```text
dispatchStepWithWorkspace
  -> ensureGapBaselineBeforeDispatch
  -> rebuildDispatchInputFromConvergenceAuthority
  -> validateConvergenceDispatchInput
  -> enrichLLMStepInputContext
```

Risk:

- The guard validates selected NextAction/contract/stage fields, but does not
  hard-check every focus-bearing derived field.

Likely files:

- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

### Step Input Assembly

Current risk:

- `runExecutionInput` copies `acceptance` and `coverageFocus` from
  `run.ExecutionContext` after authority/gap baseline are already present.
- `slimExecutionContextForStepInput` can preserve nested execution context
  signals that still carry stale acceptance/focus data.

Likely file:

- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`

### Stage Packet Assembly

Current risk:

- `stagePacketActiveFocus` prefers authority.
- `stagePacketRequiredOutput` prefers `repairActionPackage.requiredOutput`.
- If `repairActionPackage` is stale, Stage Packet can contain a new active focus
  and an old required output.

Likely file:

- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

### Required Output Focus Binding

Current risk:

- Focus-binding helpers correctly enforce a concrete `coverageFocusId`, but the
  source focus can be stale when authority has moved forward.
- These helpers should bind to the current authority focus or be treated as
  diagnostic lineage.

Likely files:

- `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

### Prompt Rendering

Current risk:

- C3 renders Stage Packet.
- C4 also renders `acceptance-focus`.
- If C4 acceptance focus is stale, LLM receives conflicting current-focus
  instructions.

Likely file:

- `internal/modules/blocks/llm/biz/prompt_messages.go`

### Projection Diagnostics

Current risk:

- ViewModel is ready and prompt mode is correct, but projection does not expose
  authority focus drift between `convergenceAuthority`, `gapBaseline`,
  `coverageFocus`, `acceptance`, and `requiredOutput`.

Likely files:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/acceptance_projection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`

### Resource Envelope

Current risk:

- Detail snapshot and observability paths still hydrate and encode large JSON
  payloads during active execution.
- Live verification snapshot files were about 3MB and 3.7MB, and 60-second
  observation hydrated about 80MB of payloads.

Likely files:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/service/server`
- `internal/pkg/repository`

## Closure Order

## Closure Progress

Last updated: 2026-06-26

- P0-1 Current Authority Focus Binding: closed.
  Dispatch input now normalizes `coverageFocus`, current `acceptance`, nested
  `coverageWorkItemContext`, and retained repair package focus from the current
  `convergenceAuthority` / `gapBaseline` focus before Stage Packet rendering.
- P0-2 Required Output From Authority: closed.
  `repairActionPackage.requiredOutput`, top-level `requiredOutput`, and Stage
  Packet `requiredOutput` are rebound to the current authority focus for
  state-changing / validation actions. Assessment/governance actions drop stale
  executable required output before provider dispatch.
- P0-3 Dispatch Integrity Gate Expansion: closed.
  Provider dispatch now fails before the LLM call when authority, gap baseline,
  Stage Packet, coverage focus, acceptance focus, or required output focus
  bindings disagree after rebuild.
- P0-4 Prompt C4 Focus Hygiene: closed.
  C4 no longer renders `acceptance-focus` when C3 Stage Packet already carries
  the current `activeFocus`.
- P1-1 Projection Authority Drift Diagnostics: closed.
  Run and workspace snapshots expose read-only `authorityDiagnostics` for
  persisted focus drift without repairing owner state in projection.
- P1-2 Resource Envelope Thresholds: closed by existing verification support.
  `observe-workorch-runtime-closure.sh` and
  `verify-convergence-authority-dispatch-live` already support configurable
  metadata scan, payload hydrate, process CPU, RSS, read, and write thresholds
  with warning/strict behavior.
- P2-1 Detail Snapshot And Observability Payload Budget: closed.
  Normal run projection now uses compact metadata-only event queries; explicit
  observability keeps full payload hydration. Workspace projection already uses
  compact workspace event queries and separate bounded LLM usage payload
  queries.

### P0-1: Current Authority Focus Binding

Owner: WorkOrch core orchestrator.

Required closure:

- Define one helper that extracts the current authority focus from
  `convergenceAuthority` / `gapBaseline`.
- Rebuild or overwrite `coverageFocus` from that helper before dispatch.
- Rebuild or overwrite `acceptance.currentFocus` from that helper when
  `acceptance` is retained as LLM-visible current signal.
- If old acceptance slice metadata is still useful, move it under diagnostic
  lineage or mark it as historical.

Acceptance:

- A dispatch input cannot contain
  `convergenceAuthority.currentFocus.id != stagePacket.activeFocus.id`.
- A dispatch input cannot contain current `coverageFocus` or
  `acceptance.currentFocus` with a different coverage work item from authority.
- Existing old focus metadata may remain only under diagnostic lineage.

### P0-2: Required Output From Authority

Owner: WorkOrch core orchestrator.

Required closure:

- Rebuild `repairActionPackage.requiredOutput` from current NextAction and
  current authority focus before Stage Packet is attached.
- If current NextAction is `assessment`, Stage Packet must not retain stale
  validation/state-change required output.
- If current NextAction is `validate`, required output must bind the current
  authority focus, not a previous repair focus.

Acceptance:

- Stage Packet cannot contain `activeFocus=cwi-X` with
  `requiredOutput.coverageWorkItemIds=[cwi-Y]`.
- Stage Packet cannot contain `activeFocus=cwi-X` with
  `requiredOutput.coverageFocusId=focus-cwi-Y`.

### P0-3: Dispatch Integrity Gate Expansion

Owner: WorkOrch core orchestrator.

Required closure:

- Expand `validateConvergenceDispatchInput` to check focus alignment across:
  - `convergenceAuthority.currentFocus`
  - `gapBaseline.currentFocus`
  - `stagePacket.activeFocus`
  - `coverageFocus`
  - `acceptance.currentFocus`
  - `repairActionPackage.requiredOutput.coverageFocusId`
  - `stagePacket.requiredOutput.coverageFocusId`
  - `requiredOutput.coverageWorkItemIds`
- If misaligned, refresh/rebuild once. If still misaligned, fail fast as
  runtime internal contract misalignment before provider call.

Acceptance:

- The two live drift shapes above are represented as failing sample tests.
- Provider dispatch is not attempted with conflicting focus fields.

### P0-4: Prompt C4 Focus Hygiene

Owner: LLM context renderer.

Required closure:

- When Stage Packet / convergence authority exists, C4 must not render stale
  `acceptance-focus` as a current focus signal.
- If acceptance slice information remains useful, render it as historical or
  diagnostic context with explicit non-authoritative labeling.
- C3 Stage Packet must be the only current action/focus contract.

Acceptance:

- Prompt rendering test proves C4 does not expose stale acceptance currentFocus
  when Stage Packet activeFocus exists.
- LLM-visible context contains exactly one current focus contract.

### P1-1: Projection Authority Drift Diagnostics

Owner: workorchd projection/ViewModel.

Required closure:

- Projection should expose `authorityDiagnostics` when persisted run fields
  disagree:
  - authority focus vs gap baseline focus
  - authority focus vs coverage focus
  - authority focus vs acceptance focus
  - authority focus vs requiredOutput focus binding
- Projection must not fix owner state. It only surfaces diagnostics.

Acceptance:

- Snapshot/detail tests show focus drift diagnostics are present when stale
  fields are found.
- TUI/Web consume the diagnostics through ViewModel without local inference.

### P1-2: Resource Envelope Thresholds

Owner: verification harness and read model IO.

Required closure:

- Add optional thresholds for:
  - metadata records scanned delta
  - payload hydrate delta
  - payload hydrate bytes delta
  - process CPU percent
  - RSS bytes
  - write bytes delta
- Make the thresholds configurable so live verification can fail when resource
  envelope regresses.

Acceptance:

- Live verification can run in warning mode and strict mode.
- Strict mode fails on the observed class of unbounded payload hydration.

### P2-1: Detail Snapshot And Observability Payload Budget

Owner: workorchd read model and HTTP server.

Required closure:

- Keep normal workspace list/detail projection compact.
- Hydrate large step input/output only on explicit observability/debug request.
- For observability, return bounded windows and refs by default; require explicit
  parameters for full payload hydration.
- Avoid repeatedly JSON-decoding large persisted payloads when only metadata is
  required.

Acceptance:

- Snapshot payload size and hydrate byte delta are bounded during two-workspace
  execution.
- CPU profile no longer dominated by JSON decode/GC during normal projection
  refresh.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "convergenceAuthority|gapBaseline|stagePacket|coverageFocus|acceptance|repairActionPackage|requiredOutput|coverageFocusId" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/modules/blocks/llm -g '*.go'
```

Live verification:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_OBSERVE_WORKSPACE_IDS=ws-a,ws-b \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
make verify-convergence-authority-dispatch-live
```

Strict resource verification should be enabled after P1-2 adds thresholds.

## Done Criteria

- One current authority focus drives `gapBaseline`, `convergenceContract`,
  `stagePacket`, `requiredOutput`, `coverageFocus`, and any current
  `acceptance` signal.
- Old focus-bearing fields are either removed from current input or explicitly
  marked diagnostic-only.
- Prompt C3/C4 exposes exactly one current focus/action contract.
- Provider dispatch fails before LLM call when focus-bearing fields conflict.
- Projection can display focus drift diagnostics without repairing owner state.
- Live verification passes display/progress checks and no longer shows the two
  recorded focus drift shapes.
- Resource-envelope strict mode can detect excessive payload hydration and CPU
  pressure.
- Core remains business-neutral and contains no template/language/provider/live
  workspace specific branch.
