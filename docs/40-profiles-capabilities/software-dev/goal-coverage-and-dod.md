# Software-Dev Goal Coverage And DoD

Status: normative

Type: profile-capability

Last Verified: 2026-09-10

## Scope

This document defines the built-in software-development evidence specialization. Its stable template ID and product
presentation remain language- and framework-neutral. Shell's current Go-native validation adapter
(`shell/go-test-json/workspace-behavior/v5` for the `shell/project-native-validation/v4` adapter, with
sealed `shell/go-test-json/v1` and behavior v2/v3/v4 replay retained) is one typed capability-owner implementation, not part of the template identity or Runtime-core
language. The neutral CWI/focus/evidence machinery remains owned by Orchestrator and
[the core Runtime contract](../../00-core/runtime-stage-contract.md).

The delivery under test may use any implementation language. A concrete native proof must be supplied by the matching typed
capability-owner adapter (or by an exact MCP proof contract); it must not be inferred from template presentation, silently
reuse an unrelated recipe, trust stdout self-reporting, or use an Application-inferred fallback.

Implementation language and supported product languages/backends are independent dimensions. A Go implementation of a
multi-language MCP/LSP bridge is a Go development target; all languages, tools, protocol behavior and metrics admitted in its
Goal/DoD remain required. Deferring development-template adapters for other implementation languages does not defer those
product capabilities. A single-language milestone or mock-backend calibration is not acceptance of the full multi-language
product. Local best-effort dependency checks must report missing/skipped work as unverified; a strict release environment must
actually execute every required integration check.

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
| each `dodItem/*` | `validation` | capability-owner typed validation receipt for the sealed proof identity; generic shell/file success and optional MCP navigation evidence are not proof |
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
target-clause constraint derived from the owner-resolved recipe applicability rule; every clause is mandatory and each clause's
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
owner-resolved recipe identity. A Shell `would_not_satisfy` result is
persisted with the File delta as one exact-current composite and never becomes a binding; because the proposal already satisfied
the published constraint, it is an owner contract contradiction and directly reaches
`prospective_satisfaction_contract_violated -> semantic_contract_failed`, with no replacement-target correction. Shell typed
stale only uses bounded catalog refresh. None of these branches creates validation credit, governance, a ticket or an acquisition
effect.

`coverage_map` and a generic `user_deliverable` are Result/trace material derived from the proved CWI set, not unconditional
terminal members. A concrete Goal may still declare an additional artifact through its admitted Definition/Profile contract, but
the Runtime must not manufacture a generic CWI without an exact proof declaration.

## Admission Rules

Native-check structural readiness, observed execution of the delivery implementation, and semantic satisfaction of the CWI are
separate claims. Helper/subtest assertions may establish structural readiness; workspace implementation coverage supplies only
a necessary execution fact. Neither is a general-purpose Goal/DoD grader. Functional/API/CLI/protocol claims require assertions
against the actual delivery entrypoint, not source/README strings, a parallel implementation inside the test, or direct calls to
external dependencies that bypass the delivery. A matching owner adapter must observe the delivery path; unsupported subprocess
or language instrumentation is unverifiable, not a successful proof fallback.

Production implementation, verification inputs and acceptance criteria remain separate responsibilities. A failing assertion's
source location is diagnostic, not automatic permission to edit that assertion. The v2 Shell adapter supplies independently
observed implementation candidates to the existing File confirmation lane. Changing verification inputs/criteria requires a
separately declared contract and positive/negative calibration preserving the original acceptance intent; the default
implementation repair route does not authorize such a change. Built-in reference tests calibrate their fixed fixture only and
must not be presented as a production registry of trusted graders for arbitrary requirements.

Failure feedback must distinguish traceability from repair information. With the optional v2 diagnostic extension, a check
location and digest alone remain incomplete; a bounded reported assertion may convey a closed expected/observed shape but is
not an independently verified value or acceptance evidence. The Shell content contract may declare optional report syntax for
new inputs. Neither that syntax nor missing reports authorize rewriting existing criteria. A new scratch path, receipt, or
comment-only implementation change is not validation progress; only fresh exact-CWI proof can close the failure window.

`TestStdioEnvelopeReferenceGraderCalibration` independently exercises the same fixed entrypoint grader against known correct,
incorrect and comment-only implementations. `TestDriveRuntimeAssertionFeedbackRepairsBehaviorAcrossRestart` then exercises
real File/Shell/Run owners with a scripted provider through failure, typed feedback, implementation repair, fresh validation and
terminal delivery, including CAS/restart cuts. `verify-software-dev-goal-dod-e2e` runs these contract calibrations, not a real-model
completion benchmark. The full deployed Goal and release DCE still require their own exact evidence and registered producers;
these tests must not be registered as substitutes.

The separate `missing-behavior-to-new-validation-input` route admits only the exact Shell
`validation_output_inconclusive / validation_output / validation_workspace_behavior_unobserved` tuple with a matching typed
`InputAddition`. It declares File `file/write`, `prospective_owner_target`, `RequireInputAddition=true` and the
`validation_input_supplement` role. This authorizes one new input, never replacement of existing criteria: the required/original
check must match the rejected binding, and the proposal must name only the owner-specified new path. File remains create-only;
its receipt gives no DoD credit. Fresh Shell v3 binding requires the original and supplemental checks together, including real
workspace implementation observation. Missing/mismatched input contracts and an ineffective existing supplement terminate
boundedly; they do not open repeated input additions. Input compilation errors do not authorize editing original assertions.
The addition also carries the owner's bounded `contentContractRef + contentRequirements`; filename/check identity alone is
insufficient. The legacy project-native v1 adapter permits only imports and the exact supplemental test at file scope, with supporting
constants, variables, types and closures local to that test. Existing invalid inputs become per-CWI owner unavailability, not
another preparable path or hypothetical `would_satisfy`. SM-2 closes before another provider call and preserves the exact cause
through Result delivery; neither Soul guidance nor file creation is validation proof.
The current adapter publishes this addition only for a single native attestation; broader input/toolchain contracts need an
explicit extension. Template identity and Runtime core remain language-neutral; this does not claim general multilingual or
arbitrary-Goal trusted-grader support.

The independent `input-symbol-case-to-attested-patch` route matches only Shell managed validation's
`validation_build_failed / validation_input / validation_input_symbol_case_mismatch` tuple. It declares
`RequireInputRepair=true`, File `patch/apply`, existing-owner scope and `validation_input_maintenance`, without increasing the
shared two-attempt repair bound. The v4 Shell owner may attest only unambiguous same-file data-identifier case corrections in
the exact bound supplement, with bounded compiler locations and byte-exact pre/postimage hashes. File independently confirms
the existing input and previews the same delta; SM-3 seals one operation and its replay proof. All other bytes, original
checks, assertions and expected values remain unchanged. Ambiguous/unsupported compilation failures, assertion failures and
missing/mismatched contracts do not gain input-edit authority. Fresh original + supplemental validation is still required for
credit. This is criterion-preserving input maintenance, not a general test-repair permission or a substitute for product acceptance.

- every required Profile acceptance/deliverable source maps to one admitted exact CWI; the activation compiler emits its canonical
  exact evidence declaration, while the Profile supplies nonempty supported evidence kinds and at least one accepted operation
  capable of returning the matching typed owner receipt;
- wildcard DoD/Goal evidence policy is a declarative template applied only after an exact CWI is admitted: it may constrain
  quality, target/selector, owner-fact acquisition, repair and operation/evidence bounds, but cannot supply a current owner fact,
  recipe/native check, dispatch or credit authority, or replace the
  compiler-owned exact CWI declaration; proof-incomplete Profile input opens `select_or_fix_template_profile` before promotion and creates no Run;
- every operation must match the current binding, required variant, target authorization and owner catalog revision;
- existing files require revision-bound replacement; `file/write` is create-only;
- validation recipes must be activation-sealed and owner-confirmed;
- the stable `software-dev` identity does not select a language or framework; a concrete native or MCP proof closure is an
  explicit typed adapter contract, not an alternative inferred by Application or Orchestrator;
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

### Versioned entrypoint observation and unchanged-input reuse

New template activations select `shell/project-native-validation/v4`. Its Go behavior v5 contract isolates each CWI in
`workorch_validation/v4/<lowercase-native-identity>/input_test.go`; a conjunctive supplement stays in that same package.
It allows named same-package assertion helpers, types, constants and data-only variables. Runner overrides (`init` or
`TestMain`), extra test definitions, methods and effectful global initializers remain inadmissible. The bounded native walker
still requires reachable real `testing.T` assertions. This is a new observation/input contract, not permission to revise criteria:
existing bytes are re-inspected without rewriting any workspace input or historical archive. Invalid syntax, unresolved
assertions and unsupported input shapes remain explicit owner unavailability, not replacement-file authority.

Each ready binding requires an independent `InputReview` of the complete current check package and present root module/workspace
definitions, within the unchanged 16-file/64-KiB envelope. Other CWI inputs remain protected, but do not accumulate in this
subject or its Go package execution. The owner-authored `ScopeContractRef + ScopeRequirements` is hashed with the subject and
fully projected in C3, so the reviewer sees the actual execution boundary, not only the recipe name. Nested/intermediate
modules, nested Go packages, cgo and cross-harness imports are unsupported. Missing/unreviewable external helpers or fixtures
cannot be inferred. An original full-suite/build criterion still needs its own actual execution/assertions; selecting one CWI
package is not proof of `go test ./...` and does not waive historical failures. New preparation carries the owner's exact content
contract, not just a filename. Consistent review permits the existing SM-2 feasibility/seal path; conflict fails, and complete
inconclusive review opens the existing Decision ticket for bounded information and re-review. Review and creation grant zero credit.

Within the same managed session, Shell instruments Go test execution and Go-built child executables, restores its own
`GOCOVERDIR` at the test-execution boundary, and converts bounded fresh binary coverage with the Go toolchain. Only non-test,
non-harness implementation paths from the sealed workspace manifest contribute execution facts. Building without running,
prebuilt/unobserved binaries, foreign/harness coverage, malformed/truncated artifacts and skipped checks cannot substitute for
proof. Source/module/test input changes inside the disposable execution copy invalidate observation. Backend processes in
other languages may execute but do not themselves establish execution of the Go delivery. Actual protocol and semantic
assertions remain mandatory; this instrumentation is neither a universal grader nor an adversarial OS sandbox.

Adapter selection is activation-sealed: new Start/Restart can use the current template, while Resume/recovery retains the old
adapter and binding. Automatic default loading requires the exact reviewed previous Template/Soul fingerprints; customized
or unknown content stays untouched. No protocol is upgraded by checking its name, current Soul text or current registry alone. Original and
supplemental checks remain all-of. File retains sole durable-write ownership; Shell performs only ephemeral execution;
Orchestrator retains all SM-2/3/4 authority and Run retains SM-1. A missing-input addition uses the existing typed route and
fresh-validation transaction; unchanged-input reuse needs no File mutation, extra acquisition budget or fifth state machine.

## Repair And Restart

The built-in wildcard `dodItem/*` declarative policy contributes validation/acquisition/repair guards only after Application
binds them to an admitted exact CWI; it does not itself seal owner facts, executable proof or Runtime authority.
For each admitted DoD CWI, the activation compiler emits a distinct exact requirement; the Profile separately seals one
`ValidationFailureRepairConvergence` with an exact-trigger lookup table. The routes are repair specializations, not alternative
terminal requirements; every DoD CWI still closes through its original `all_of` proof contract:

The built-in Profile explicitly declares the selectable Shell owner-fact and bounded File-acquisition/Shell-reobserve closure
for managed `acceptanceDimension/*`, `goalCoverage/*`, `dodItem/*`, and `deliverable/validation_report`. Application only
instantiates those declarations for admitted exact CWIs and binds the generated requirement hash; it does not manufacture this
cross-owner policy. A Shell `ProofRequirementPreparation` identifies the exact missing native test/input and all-of target set but
grants no credit. File must commit the complete prospective set, Shell must reobserve it, and only the resulting exact binding may
be sealed for execution.

```text
Shell validation/managed_session rejected with the owner-produced build-failure diagnostic
  -> one File-owned state_change repair generation
  -> a fresh validation generation for the same CWI
```

The declaration is sealed only in the immutable `RuntimeDefinitionFocus`. Mutable generation `CoverageFocus` values never
copy or override it; SM-2 consumes the manifest obligation compiled from that definition snapshot.

Each route binds one exact `(trigger owner, operation kind, diagnostic class, evidence target type, owner diagnostic code)` tuple
to one repair owner and surface. The built-in executable routes bind Shell's domain-neutral
`module_manifest_parse_failed`, `source_compile_failed` and `test_failed` facts to File `file/edit`, the independent Definition selector
`file/workspace_deliverable` and File operation selector `workspace-relative-existing-file-target`. Their target policy is
`existing_owner_scope`: Shell must publish manifest-scoped structured candidates and File must confirm them before SM-2 creates
repair authority. Duplicate trigger tuples or cross-owner operations fail pre-seal. The operation vocabulary is a bounded owner
capability set; it is not a Goal/DoD `one_of` terminal relation, and an accepted repair receipt cannot itself satisfy the DoD
validation requirement.

The software-dev template seals `shell/project-native-validation/v4`, not a language command, argument vector or environment.
Shell alone resolves that declaration against the exact File-owner manifest and publishes the concrete executable recipe; the
current Shell adapter can recognize Go project facts and keeps all Go grammar private to the owner. The managed-validation wire
then contains only the sealed `recipeId`; it does not carry a caller-selected command or failure class. Unrecognized failures
retain `shell_exit_nonzero` and cannot match a default route. A project Profile may add a route only after its producing owner and
repair owner publish the complete typed diagnostic and surface-confirmation contracts.

The built-in routes contain no prospective target selection. Shell's structured candidates remain non-authoritative until File
confirms the exact current regular-file set; SM-2 then seals a `confirmed_validation_repair` directive whose allowed operation set
is exactly `file/edit`. Missing structured candidates or rejected File confirmation terminate as `repair_surface_unavailable`.
The complete repair receipt reopens the same CWI for fresh validation.

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
