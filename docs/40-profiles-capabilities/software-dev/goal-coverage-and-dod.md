# Software-Dev Goal Coverage And DoD

Status: normative

Type: profile-capability

Last Verified: 2026-09-03

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
| `deliverable/source_change` | `state_change` | File/Patch owner receipt for the grounded `workspace_delivery_change` target role |
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
validates the bounded 1–8 target set proposed by one SM-3 semantic document at the intersection of the Definition declaration
selector/candidate refs and the current File operation selector/constraints. The built-in Profile pre-seals
`ProspectiveTargetAllOfConstraintRefs=[workspace-relative-path,new-target-requires-owner-selector]` for source-change, DoD and
Goal target-grounding declarations. Both refs are mandatory, File must echo/accept the exact set before semantic planning, and
the proof-batch proposal must return the exact same `prospectiveConstraintRefs`; this declaration has no exact path and no
alternative branch. Shell separately publishes a provider-safe typed `all_of`
target-clause constraint derived from the activation-sealed recipe applicability rule; every clause is mandatory and each clause's
predicates are conjunctive. The acquisition/manifest seals it into C3; no clause or target has a `one_of`/`any_of` escape. For the
default `go test -v ./...` recipe, current readiness requires both root `go.mod` and a non-excluded test input. If either is
missing, Shell publishes a corresponding mandatory clause: root module uses exact path `go.mod`, test input uses `_test.go` plus
the excluded-segment predicates, and a missing declared package root adds its exact path-prefix clause. At most eight clauses may
be published. The proposal must cover every clause and every target must contribute, so an empty or source-only workspace can
propose `go.mod` and a test input in the same bounded all-of transaction without authorizing unrelated files. A mismatching,
partial, duplicate or extra-target proposal is rejected by JSON Schema/document admission before File or Shell is called, and
only an ordinary complete-document correction may replace it. File validates every path, absence/parent relation and aggregate
manifest capacity against one complete current-tree revision, then returns an immutable side-effect-free hypothetical manifest
delta. Shell evaluates the resulting manifest against its exact non-selectable dependency catalog/observation plus the
activation-sealed recipe identity. A Shell `would_not_satisfy` result is
persisted with the File delta as one exact-current composite and never becomes a binding; because the proposal already satisfied
the published constraint, it is an owner contract contradiction and directly reaches
`prospective_satisfaction_contract_violated -> semantic_contract_failed`, with no replacement-target correction. Shell typed
stale only uses bounded catalog refresh. None of these branches creates validation credit, governance, a ticket or an acquisition
effect.

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
  `ProspectiveSatisfactionContractRef`; Shell must publish one matching provider-safe typed `all_of` constraint, while the
  dependency catalog and recipe command remain freshness authority only and never LLM-selectable;
- every built-in target-grounded source/DoD/Goal requirement carries the exact mandatory File creation constraint refs
  `workspace-relative-path + new-target-requires-owner-selector`; the Profile hash, File TargetSelection, semantic obligation,
  C3/schema, SM-2 binding and reconciliation must preserve that full set. A subset/superset is not another admissible option;
- the Profile declaration does not carry an exact prospective target set; one SM-3 proposal supplies 1–8
  `ProspectiveTargetRefs` at the intersection of its declaration selector/candidate refs and the current operation
  selector/owner constraints. File/Shell validate the whole all-of set, and only the SM-2 binding authorizes the later
  operation candidate/dispatch. SM-3 then requires one ordered `file/write` operation with an exact singleton `TargetRef` for
  every bound target; partial or additional operations are rejected before effect;
- evidence from a predecessor or another Workspace cannot close the current Run;
- partial assessment closes only explicitly satisfied work items and leaves the rest active;
- no current-Run accepted evidence means a satisfied assessment is invalid.

## Repair And Restart

The built-in wildcard `dodItem/*` requirement seals one
`ValidationFailureRepairConvergence` with an exact-trigger lookup table. The routes are repair specializations, not alternative
terminal requirements; every DoD CWI still closes through its original `all_of` proof contract:

```text
Shell validation/managed_session rejected with the owner-produced build-failure diagnostic
  -> one File-owned state_change repair generation
  -> a fresh validation generation for the same CWI
```

The declaration is sealed only in the immutable `RuntimeDefinitionFocus`. Mutable generation `CoverageFocus` values never
copy or override it; SM-2 consumes the manifest obligation compiled from that definition snapshot.

Each route binds one exact `(trigger owner, operation kind, diagnostic class, evidence target type, owner diagnostic code)` tuple
to one repair owner and surface. The built-in executable route binds Shell's deterministic build-failure tuple to File
`file/write | patch/apply`, the independent Definition selector `file/workspace_deliverable` and File operation selector
`workspace-relative-new-file-target`. Duplicate trigger tuples or cross-owner operations fail pre-seal. The operation vocabulary is
a bounded owner capability set; it is not a Goal/DoD `one_of` terminal relation, and an accepted repair receipt cannot itself
satisfy the DoD validation requirement.

The built-in managed-validation wire contains only the sealed `recipeId`; it does not carry a caller-selected failure class.
Therefore `implementation_gap_detected` and `validation_test_failed` are not declared as default routes while Shell cannot produce
those exact tuples from the production operation contract. A project Profile may add a route only after its producing owner and
repair owner publish the complete typed diagnostic and surface-confirmation contracts.

The routes contain no exact repair path. Their semantic proposal supplies one bounded prospective target set within both
independent selectors; File confirms every member, and only SM-2 seals the resulting `CoverageEvidenceBinding`. The complete repair
receipt atomically retires that binding and reopens the same CWI for fresh validation.

Dependency/toolchain/environment diagnostics deliberately have no built-in software-dev route. An unmatched exact tuple seals
`validation_failure_repair_route_unavailable` in the same owner-private CAS as the rejected receipt; no semantic provider request,
generic File fallback, command synthesis, governance ticket, or SM-4 retry is allowed. Project-specific dependency remediation
requires a separately declared typed owner capability and Profile extension; it is not a central default-template behavior.

Missing, ambiguous or invalid Profile convergence is a pre-seal Template/Profile defect: admission opens a blocking Definition
clarification ticket and creates no Run until the declaration is corrected. By contrast, a sealed convergence whose current owner
catalog has no legal repair surface fails typed and bounded at Runtime; it does not open a generic Decision/Approval ticket, invent
a target, repeat validation indefinitely or weaken the startup input. When Goal/DoD or the clean baseline itself must change, the
caller uses explicit Workspace `restart`; Run owner performs the replacement transaction.

## Result Requirements

The final software-development Result should include change summary, verification commands/results, evidence/artifact refs,
per-DoD assessment, failures/skips, residual risk and—when required—a version-control snapshot. Result owner persists this
already-judged closeout and does not recompute coverage.

## Verification

- `make verify-runtime-profile-software-dev` checks persisted/profile projection compatibility;
- `make verify-software-dev-goal-dod-e2e` and the SRD wrapper execute current bootstrap, CWI binding, focus advancement,
  evidence-credit and file-backed restart tests. Their historical target names do not add a second Runtime contract.
