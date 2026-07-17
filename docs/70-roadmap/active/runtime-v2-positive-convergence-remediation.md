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
- Typed validation policies now compile at the application boundary and seal
  configured repair/validation-only limits into every new bootstrap.
- Candidate context now includes exact active rejected operation shapes and
  admissible operation kinds derived from persisted dispatch/evidence lineage.
- Result closeout packages current-run evidence/coverage/assessment ledgers even
  when terminal failure occurs before assessment; continuation prefers closeout.
- Runtime V2 read models now separate candidate contract retry from focus repair
  and clear superseded ineffective-retry state after focus advance.
- Legacy active aggregate restart, partial-slice restart/CAS, and feasibility
  no-provider paths have direct regression coverage.
- Acceptance focus count is bounded and the software-dev profile batches direct
  validation coverage in groups of up to 16 items.
- The ai-proxy HTTP adapter now omits the Authorization header consistently for
  catalog, chat, stream, and embedding when the optional client API key is empty.

## Remaining Blocking Work

| ID | State-machine scope | Required closure |
| --- | --- | --- |
| RVC-R4 | SM-4 | Replace the test aggregation script with a real file-backed service E2E, then capture two real workspace cross-focus proofs without altering default provider configuration. |

The archived completion record is historical evidence only and must not be used
as an implementation authority or as proof that these items are closed.
