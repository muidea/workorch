# WorkOrch Runtime Failed Surface And Root Cause Target Design

## 1. Background

Recent execution checks for `ws-5823c555519e601c` showed that runtime repair can still converge to the wrong artifact surface:

1. A repeated failed surface was identified and `strategyReassessment.required=true` was injected.
2. The immediate rejected output was correctly blocked when it tried to patch the same failed surface without a root-cause target decision.
3. The next repair step lost part of that reassessment context and could downgrade into ordinary patch repair.
4. Later outputs still patched verification or acceptance surfaces instead of proving why those surfaces were the real correction target.

This document defines the neutral runtime contract needed to separate where failure appears from where repair should be applied.

## 2. Scope And Neutrality

WorkOrch core remains functionally neutral. Runtime may reason about:

- failure evidence,
- failed surfaces,
- target snapshots,
- declared repair targets,
- repeated failure lineage,
- structured correction evidence,
- patch admission.

Runtime must not reason about:

- software development,
- test frameworks,
- production code,
- HTTP routes,
- protocol names,
- language-specific symbols,
- concrete Goal or DoD business semantics.

Any profile-specific explanation of what an artifact means belongs to workspaceTemplate, Soul, skill, or the user's Goal and DoD. Core only validates whether the LLM declared enough neutral evidence to justify the selected repair target.

## 3. Contract Model

Runtime repair uses three distinct layers.

| Layer | Meaning | Patchable By Default |
| --- | --- | --- |
| `failureEvidence` | Raw or compact diagnostics, errors, logs, line refs, target-like tokens, rejected output summaries | No |
| `failedSurfaceSet` | Resource or scope where the failure appeared | No |
| `repairTargetSet` | Resource or scope allowed as an actual repair target, derived from current failure targets, snapshots, manifest, declared related resources, or explicit reassessment | Yes |

The key rule is:

> A failed surface is evidence, not automatically the correct repair target.

When the same failed surface repeats, runtime must require the LLM to declare a `rootCauseTarget`. If the LLM still chooses to patch the failed surface, it must provide a structured `surfaceContractCorrection` proving that the surface itself is the correction target rather than merely the place where the failure was detected.

## 4. Runtime Input Requirements

When `runtimeStage=repair`, `stateChangingOutputRequired=true`, and repeated failure indicates `strategyReassessment.required=true`, the repair input must carry:

- `strategyReassessment`
- `repairActionPackage.strategyReassessment`
- `failedSurfaceSet` or the equivalent `failedSurface`
- `rootCauseCandidateContext` when available
- previous rejected output summary
- current target snapshots or manifest refs when available
- repair guidance prohibiting repeated failed-surface patching without target reassessment

If a repair output is rejected for violating the strategy reassessment contract, the next repair step must inherit the same `strategyReassessment` and candidate context. A runtime contract failure on the wrapper must not erase the original repeated-surface repair contract.

## 5. Patch Admission Rules

For state-changing repair under `strategyReassessment.required=true`:

1. A patch output must declare:
   - `failedSurface`
   - `rootCauseTarget`
   - `repairTargetDecision`
   - `forbiddenRepair`
   - `verificationPlan`
2. If repeated failed-surface repair requires a target shift, patch target must normally differ from the failed surface.
3. If patch target equals the failed surface, the patch is allowed only when it includes complete `surfaceContractCorrection`.
4. `surfaceContractCorrection` must be neutral and include:
   - `whySurfaceIsWrong`
   - `contractConflict`
   - `currentResourceEvidence`
   - `boundedVerificationPlan`
5. Runtime contract diagnostic field names such as `strategyReassessment.required`, `failedSurface`, `rootCauseTarget`, and `surfaceContractCorrection` must never be promoted into `repairTargetSet`.

`surfaceContractCorrection` is intentionally not named after any domain artifact. It means: the LLM claims the failed surface's own contract is invalid or stale, and provides bounded evidence for changing that same surface.

## 6. Expected Behavior

When failure repeatedly appears on the same surface:

- Runtime asks for a root-cause target decision.
- LLM can patch a distinct target and declare the failed surface as related failure evidence.
- LLM can patch the failed surface only with a complete `surfaceContractCorrection`.
- LLM can return failed assessment or blocker if no valid repair target can be determined.
- Runtime rejects repeated failed-surface patches that only adjust the observed failure artifact without proving that the artifact itself is wrong.

## 7. Implementation Tracking

| ID | Item | Layer | Status |
| --- | --- | --- | --- |
| FSRC-01 | Add this failed-surface/root-cause contract document | Docs | Completed |
| FSRC-02 | Reference this contract from the stage neutrality core contract | Docs | Completed |
| FSRC-03 | Inherit `strategyReassessment` and root-cause candidate context across repair-of-repair | Core runtime | Completed |
| FSRC-04 | Add neutral `surfaceContractCorrection` admission for same-surface patching | Core runtime | Completed |
| FSRC-05 | Add tests for context inheritance and same-surface admission/rejection | Tests | Completed |
| FSRC-06 | Run focused orchestrator tests | Verification | Completed |

## 8. Progress Log

### 2026-05-19

Completed:

1. Added this focused contract and linked it from `workorch-runtime-stage-contract-neutrality-design.md`.
2. Runtime repair follow-up now preserves inherited `strategyReassessment`, `targetRepairAttempts`, `failureTargetScope`, and `rootCauseCandidateContext` when the previous repair failed at the runtime contract wrapper.
3. Same-surface patching under repeated failed-surface reassessment now requires complete neutral `surfaceContractCorrection`.
4. Added unit coverage for repair-of-repair inheritance, incomplete same-surface correction rejection, and complete same-surface correction admission.
5. Removed touched runtime heuristics that inferred candidate targets from domain-shaped file names; root-cause candidates now use neutral adjacent or referenced resource relations.
6. Verified with `go test ./internal/modules/kernel/orchestrator/biz -count=1`.
