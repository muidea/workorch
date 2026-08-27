# WorkOrch Software-Dev Profile Verification

Status: normative

Type: verification-contract

Last Verified: 2026-08-27

## Purpose

This verification layer proves that software-development specialization remains in template/Soul/profile/capability contracts
and converges through the same neutral Runtime owner model. It must not make legacy graph/replan fixtures authoritative.

## Required Assertions

### Definition And Profile

- `software-dev` template, six Soul bindings and full Soul bodies load through the correct owners;
- Goal/DoD compile into bounded CWIs, evidence roles and typed profile/execution contracts;
- the built-in terminal set is exactly `source_change`, each DoD validation, `validation_report` and
  `version_control_snapshot`; validation preparation remains a DoD prerequisite, while coverage-map/generic-deliverable remain
  derived Result material rather than independent CWIs;
- every required Profile source has an exact same-scope/id evidence declaration, explicit supported evidence kind and a
  proof-capable owner receipt operation; wildcard policy cannot substitute for an exact source;
- Profile snapshot/hash or snapshot-derived owner-fact manifest drift fails pre-seal with the Template/Profile correction action
  and creates no Run/provider/capability work;
- DoD validation preparation seals the exact prospective-satisfaction contract ref; missing/extra/drifted ref or use on a
  non-prospective policy fails pre-seal and creates no Run/provider/capability work;
- each repair-enabled DoD requirement seals one validation-failure convergence, never an alternative list: exact Shell trigger
  and diagnostic classes, File state-change owner/kinds, separate declaration/operation selectors, prospective policy, fresh
  validation requirement and bounded cycles. Unknown diagnostics, cross-owner kinds, non-state-change evidence, incomplete
  selectors, disabled fresh validation or an out-of-range bound fail pre-seal and create no Run/provider/capability work;
- `source_change` uses the broad `workspace_delivery_change` role; wildcard DoD terminal validation and its repair surface retain
  the narrower `deliverable_implementation` role;
- software-dev strings do not appear in Orchestrator reducer branches;
- current provider guidance uses current semantic-plan/candidate schemas.

### Operations And Evidence

- absent-target creation and existing-target revision-bound replacement are distinct;
- one candidate contains at most one ordinary durable operation for each capability owner/canonical target; same-file edits are
  merged even when dependency order is declared, while an owner-sealed exact required operation sequence and distinct targets
  remain executable in one dispatch;
- build/test/lint and direct entrypoint checks produce matching validation receipts;
- prospective validation preparation proves the profile-specific applicability split by publishing a provider-safe typed
  `all_of` clause set derived from the exact default recipe. The empty/source-only/module-only/ready workspace matrix must prove
  that missing root `go.mod`, test input and declared package roots become mandatory clauses without `one_of`/`any_of` fallback.
  Schema/document admission accepts one bounded 1–8-member `prospectiveTargetRefs` set only when every clause is covered and every
  target contributes; empty/partial/extra/duplicate/mismatching sets produce a typed `prospectiveTargetRefs` violation before
  File/Shell probe. File then checks the whole set against one manifest revision, including aggregate capacity, with zero writes;
  Shell judges only the hypothetical resulting manifest. None of these paths creates a file, directory, process/session, receipt
  or credit;
- the SM-2 binding must preserve the whole prospective target set, and the SM-3 operations contract must require exactly one
  ordered operation per target with an exact-singleton `TargetRef`; missing, additional, duplicate or out-of-order operations are
  rejected before File effect. The singular semantic wire field is rejected rather than treated as a compatibility alias;
- Shell dependency catalog/observation remains absent from provider C3/schema choices; File delta and Shell nested observation
  must both be exact-current at CAS and seal. Shell typed stale uses bounded refresh without consuming semantic correction.
  Persisted full-composite `would_not_satisfy` after a constraint-compatible proposal proves an owner contract contradiction and
  reaches `prospective_satisfaction_contract_violated -> semantic_contract_failed`; it never reuses the correction ledger for a
  replacement target or second provider request. Duplicate/restart neither repeats terminal delivery nor the same proposal's owner probe;
- a validation entry is recipe lineage, not an implementation repair target; unstructured output yields no target candidate,
  while managed Go diagnostics may report only actual `*.go:line[:column]` locations inside the fresh File manifest scope;
- a matching typed validation rejection without an exact owner-grounded target opens one
  `validation_failure_repair` semantic generation for the same CWI. The manifest preserves terminal `validation`, exposes only
  current File state-change authority, and binds the failure/convergence/catalog identities. File feasibility must confirm the
  proposed bounded prospective target set before SM-2 seals it; partial members grant no DoD credit and cannot be combined across
  dispatches. The complete repair receipt set retires the repair binding and requires fresh validation. Duplicate/restart is
  idempotent, stale catalog/failure lineage cannot seal, and the Profile/repair
  budgets terminate rather than loop;
- Git snapshot requirements use `git/write:add` + `git/write:commit`, not shell/read evidence;
- optional LSP MCP evidence remains auxiliary;
- assessment cites current-Run accepted evidence and partial closure leaves remaining items active.
- filesystem/repository/artifact mutation只使dependency精确相交的owner revision set evidence stale；无关owner变化不触发
  全局失效，旧proof只有在明确freshness规则成立时才能复用；
- coverage、assessment、evidence三轴投影保持独立，`indeterminate`不关闭required CWI。

### Ownership And Recovery

- Run owns SM-1 and restart transaction; Orchestrator owns SM-2/3/4;
- capability owners return facts only;
- same-focus repair is bounded and restart is explicit;
- duplicate/stale/crash-window behavior cannot repeat side effects or credit;
- a complete candidate rejection opens attempt 1 only when the persisted manifest-bound correction directive contains a legal
  semantic witness. Operation ID or summary-only changes are not witnesses; an empty finite frontier terminates at attempt 0 as
  `candidate_authority_exhausted`, while a rejected witness-bound attempt 1 terminates as `candidate_retry_exhausted`. Restart
  revalidates the same manifest/witness before any provider or owner call;
- formal Result delivery preserves Result → Workspace → Run acknowledgement order.
- provider/capability transient unavailability只有在typed condition/trigger/deadline/budget完整时进入recoverable wait；否则
  fail-closed，且wait恢复不重复file/git/shell effect或预算消费；
- Result closeout包含Definition/Run lineage、逐CWI coverage、accepted evidence、assessment、artifact、applicable owner
  revision set和known limitations。

## Current Entry Points

| Command | Normative scope |
| --- | --- |
| `make verify-runtime-profile-software-dev` | embedded defaults, activation compilation, replacement operation and file-backed convergence |
| `make verify-software-dev-code-agent` | current Template/Soul/runtime document contracts, Definition seed boundary, File owner admissibility and receipt-bound restart |
| `make verify-software-dev-goal-dod-e2e` | Goal/DoD coverage compilation, private semantic-plan/evidence transitions and two-focus file-backed restart |
| `make verify-software-requirement-orchestration` | the two focused verifiers plus Workorchd read-model/HTTP activation non-authority guards |

These wrappers invoke current package/test names and fail on any selected-test failure. Their assertions are normative only for the
listed scope; broader Runtime guarantees still require the core and boundary suites below.

## Current Authoritative Baseline

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/pkg/workorchdefaults -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/git/... -count=1
go test ./internal/modules/blocks/mcp/... -count=1
go test ./internal/modules/blocks/shell/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
make verify-runtime-positive-convergence-file
make verify-boundary-contract
```

Deployed acceptance must additionally record service revision, fresh data root, Workspace/Run IDs, semantic seal, owner
catalog/feasibility, sealed operations, receipts, evidence bindings, assessment and terminal delivery.

## Compatibility Name Rule

Compatibility script filenames, scenario IDs and read-model fields may remain visible. They cannot be cited as proof that removed `executionGraph`, `NextAction`,
`ExecutionContextPatch` or application-created successor behavior is supported.
