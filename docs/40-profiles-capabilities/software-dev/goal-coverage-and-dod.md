# Software-Dev Goal Coverage And DoD

Status: normative

Type: profile-capability

Last Verified: 2026-08-25

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

## Default Proof-Closed CWI Set

The built-in `software-dev` Profile declares only terminal work that has an exact executable proof path:

| Source | Required evidence | Proof owner path |
| --- | --- | --- |
| `deliverable/source_change` | `state_change` | File/Patch owner receipt for the grounded workspace target |
| each `dodItem/*` | `validation` | managed-validation or MCP tool typed validation receipt for the sealed proof identity; generic shell/file success is not proof |
| `deliverable/validation_report` | `validation` | Shell managed-validation receipt; generic shell success is not proof |
| `deliverable/version_control_snapshot` | `state_change` | matching Git write/add/commit owner receipts |

`source_change` must settle before DoD validation; DoD validation must settle before `validation_report`; source, DoD validation
and report feed the final version-control snapshot gate. Validation readiness is a prerequisite of each DoD proof: if Shell does
not publish the required selectable managed-validation surface, the sealed Profile allows at most one File-owned preparation
acquisition followed by Shell reobservation. The acquisition generation requires only its exact File `state_change` receipt while
the immutable DoD CWI continues to require terminal `validation`. A ready Shell reobservation retires the acquisition generation
and reopens the same CWI for a fresh validation binding; it never grants DoD credit and is not an independent terminal CWI.

That one-shot closure uses the Profile-sealed
`shell/selectable-validation-surface/prospective-workspace-manifest/v1` satisfaction contract. Before any File effect, File
validates the exact target proposed by SM-3 at the intersection of the Definition declaration selector/candidate refs and the
current File operation selector/constraints, and returns an immutable
current-tree-revision-bound hypothetical manifest delta. Shell evaluates it against its exact non-selectable
dependency catalog/observation plus the activation-sealed recipe identity. For the default Go recipe, a resulting `*_test.go`
can make `go test ./...` applicable; a resulting `scripts/validate.sh` alone cannot. A Shell `would_not_satisfy` result is first
persisted with the File delta as one exact-current composite and never becomes a binding by itself. At semantic attempt 0, if the
same declaration selector/candidate refs and File constraints still admit a different target, the existing SM-3 correction ledger
reopens the same batch at `Requesting`; the next proposal must use that different in-bounds target and obtain fresh File/Shell
facts. A fixed/singleton or exhausted candidate scope at attempt 0, an invalid attempt-1 document, or an attempt-1
`would_not_satisfy` reaches the common `semantic_contract_failed` terminal; the second owner rejection retains typed original
reason `prospective_owner_fact_unsatisfied`, while a valid attempt-1 `would_satisfy` may proceed to the SM-2 binding seal. Shell typed stale only uses
bounded catalog refresh and does not consume correction. None of these branches creates validation credit, governance, a ticket
or an acquisition effect.

`coverage_map` and a generic `user_deliverable` are Result/trace material derived from the proved CWI set, not unconditional
terminal members. A concrete Goal may still declare an additional artifact through its admitted Definition/Profile contract, but
the Runtime must not manufacture a generic CWI without an exact proof declaration.

## Admission Rules

- every required Profile acceptance/deliverable source has an exact same-scope/id evidence declaration, nonempty supported
  evidence kinds and at least one accepted operation capable of returning the matching typed owner receipt;
- wildcard DoD/Goal evidence policy cannot replace an exact Profile-owned proof declaration; proof-incomplete Profile input opens
  `select_or_fix_template_profile` before promotion and creates no Run;
- every operation must match the current binding, required variant, target authorization and owner catalog revision;
- existing files require revision-bound replacement; `file/write` is create-only;
- validation recipes must be activation-sealed and owner-confirmed;
- `prospective_owner_target` validation preparation must carry the exact Profile-derived
  `ProspectiveSatisfactionContractRef`; the Shell dependency catalog is freshness authority only and never LLM-selectable;
- the Profile declaration does not carry an exact prospective target; SM-3 proposes one at the intersection of its declaration
  selector/candidate refs and the current operation selector/owner constraints, File/Shell validate it, and only the SM-2 binding
  authorizes the later operation candidate/dispatch;
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
