# Software-Dev Goal Coverage And DoD

Status: normative

Type: profile-capability

Last Verified: 2026-08-16

## Scope

This document defines software-development evidence specialization. The neutral CWI/focus/evidence machinery remains owned by
Orchestrator and [the core Runtime contract](../../00-core/runtime-stage-contract.md).

## Coverage Dimensions

A software-development definition can require one or more of:

- functional behavior;
- external/API/CLI contract;
- runtime or delivery entrypoint;
- persistence/side-effect behavior;
- source/config/test/documentation change;
- build/test/lint/static validation;
- error, streaming, configuration, concurrency or recovery behavior;
- quantitative performance/resource criteria;
- version-control snapshot;
- final artifact and traceable result summary.

The semantic planner may split these claims into bounded CWIs and order them, but cannot add requirements outside Goal/DoD.
Capability owners must prove that each proposed evidence path is executable before SM-2 seals the binding.

## Evidence Roles

| Role | Examples | Must not be substituted by |
| --- | --- | --- |
| primary state change | file edit/create, relevant config/test/source update | report-only file, read operation, unrelated marker |
| direct validation | build/test/lint, real CLI/API/service entry check | model assertion, unevaluated command text |
| supplemental evidence | diff, diagnostics, coverage report, artifact index | primary change when one is still missing |
| version-control snapshot | matching accepted `git/write:commit` receipt | shell git log/status, summary text |
| assessment | evidence-bound per-item disposition | historical evidence or invented ref |

## Admission Rules

- every operation must match the current binding, required variant, target authorization and owner catalog revision;
- existing files require revision-bound replacement; `file/write` is create-only;
- validation recipes must be activation-sealed and owner-confirmed;
- evidence from a predecessor or another Workspace cannot close the current Run;
- partial assessment closes only explicitly satisfied work items and leaves the rest active;
- no current-Run accepted evidence means a satisfied assessment is invalid.

## Repair And Restart

An actionable validation/assessment gap returns to the same focus with structured missing evidence and target facts. Bounded
repair may choose a different legal variant or target only within the sealed definition/binding contract. When the definition
or clean baseline must change, the caller uses explicit Workspace `restart`; Run owner performs the replacement transaction.

## Result Requirements

The final software-development Result should include change summary, verification commands/results, evidence/artifact refs,
per-DoD assessment, failures/skips, residual risk and—when required—a version-control snapshot. Result owner persists this
already-judged closeout and does not recompute coverage.

## Verification

- `make verify-runtime-profile-software-dev` checks persisted/profile projection compatibility;
- `make verify-software-dev-goal-dod-e2e` and the SRD wrapper execute current bootstrap, CWI binding, focus advancement,
  evidence-credit and file-backed restart tests. Their historical target names do not add a second Runtime contract.
