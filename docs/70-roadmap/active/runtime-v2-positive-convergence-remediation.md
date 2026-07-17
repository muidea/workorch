# Runtime V2 Positive Convergence Remediation

Status: active

Type: roadmap

Last Updated: 2026-07-17

## Purpose

The previous positive-convergence completion record was reopened after code review
found gaps between the four-state-machine contract and persisted production
authority. This tracker is a remediation index; the core contract remains
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) and the
orchestrator readme remains the implementation SoT.

## Completed In This Remediation Slice

- `RuntimeStateRecord` and `RuntimeBootstrap` now carry a coordinated schema
  version. Legacy schema `0` is normalized in memory on load and the next Run
  CAS writes schema `1`; future schemas fail fast.
- Feasibility is a typed, CAS-persisted `RuntimeFacts.Feasibility` input. A
  missing executable operation does not call the provider or consume a candidate
  attempt and is returned to the reducer.
- Item-level coverage closure is persisted in `RuntimeFacts.Coverage`; partial
  assessments only close named satisfied items and keep unresolved work current.
- Acceptance projection reads the persisted Runtime V2 facts rather than an
  empty/legacy diagnostics projection.
- File default configuration no longer embeds the live-verifier provider or its
  test secret.

## Remaining Blocking Work

| ID | State-machine scope | Required closure |
| --- | --- | --- |
| RVC-R1 | SM-2/SM-3 | Complete typed validation-policy compilation and seal configured repair limits into every new bootstrap. |
| RVC-R2 | SM-2/SM-4 | Make Result closeout and continuation consume the coverage ledger exclusively; remove residual compatibility reads of assessment booleans. |
| RVC-R3 | SM-2/SM-4 | Add legacy active-Run restart, partial-slice CAS/restart and feasibility no-provider regression suites. |
| RVC-R4 | SM-4 | Replace the test aggregation script with a real file-backed service E2E, then capture two real workspace cross-focus proofs without altering default provider configuration. |

The archived completion record is historical evidence only and must not be used
as an implementation authority or as proof that these items are closed.
