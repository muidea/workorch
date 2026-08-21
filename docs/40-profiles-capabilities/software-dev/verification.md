# WorkOrch Software-Dev Profile Verification

Status: normative

Type: verification-contract

Last Verified: 2026-08-18

## Purpose

This verification layer proves that software-development specialization remains in template/Soul/profile/capability contracts
and converges through the same neutral Runtime owner model. It must not make legacy graph/replan fixtures authoritative.

## Required Assertions

### Definition And Profile

- `software-dev` template, six Soul bindings and full Soul bodies load through the correct owners;
- Goal/DoD compile into bounded CWIs, evidence roles and typed profile/execution contracts;
- software-dev strings do not appear in Orchestrator reducer branches;
- current provider guidance uses current semantic-plan/candidate schemas.

### Operations And Evidence

- absent-target creation and existing-target revision-bound replacement are distinct;
- build/test/lint and direct entrypoint checks produce matching validation receipts;
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
go test ./internal/modules/kernel/orchestrator/... -count=1
make verify-runtime-positive-convergence-file
make verify-boundary-contract
```

Deployed acceptance must additionally record service revision, fresh data root, Workspace/Run IDs, semantic seal, owner
catalog/feasibility, sealed operations, receipts, evidence bindings, assessment and terminal delivery.

## Compatibility Name Rule

Compatibility script filenames, scenario IDs and read-model fields may remain visible. They cannot be cited as proof that removed `executionGraph`, `NextAction`,
`ExecutionContextPatch` or application-created successor behavior is supported.
