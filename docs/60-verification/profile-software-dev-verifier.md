# WorkOrch Software-Dev Profile Verification

Status: active

Type: verification-contract

Last Verified: 2026-08-16

## Purpose

This verification layer proves that software-development specialization remains in template/Soul/profile/capability contracts
and converges through the same neutral Runtime owner model. It must not make legacy graph/replan fixtures authoritative.

## Required Assertions

### Definition And Profile

- `software-dev` template, six Soul bindings and full Soul bodies load through the correct owners;
- Goal/DoD compile into bounded CWIs, evidence roles and typed profile/execution contracts;
- software-dev strings do not appear in Orchestrator reducer branches;
- active provider guidance uses current semantic-plan/candidate schemas.

### Operations And Evidence

- absent-target creation and existing-target revision-bound replacement are distinct;
- build/test/lint and direct entrypoint checks produce matching validation receipts;
- Git snapshot requirements use `git/write:add` + `git/write:commit`, not shell/read evidence;
- optional LSP MCP evidence remains auxiliary;
- assessment cites current-Run accepted evidence and partial closure leaves remaining items active.

### Ownership And Recovery

- Run owns SM-1 and restart transaction; Orchestrator owns SM-2/3/4;
- capability owners return facts only;
- same-focus repair is bounded and restart is explicit;
- duplicate/stale/crash-window behavior cannot repeat side effects or credit;
- formal Result delivery preserves Result → Workspace → Run acknowledgement order.

## Current Entry Points

| Command | Intended scope | Current trust status |
| --- | --- | --- |
| `make verify-runtime-profile-software-dev` | persisted/snapshot profile shape | compatibility verifier; must be kept aligned with current projections |
| `make verify-software-dev-code-agent` | focused code-agent unit/TUI boundary | open cleanup: contains removed package/test names |
| `make verify-software-dev-goal-dod-e2e` | deployed software-dev execution | open cleanup: mock payload/assertions still use removed graph, execution-context and auto-replan semantics |
| `make verify-software-requirement-orchestration` | aggregate profile/read-model suite | inherits the preceding legacy fixture until updated |

Therefore these wrapper names are not, by themselves, closure evidence today. The implementation gap is tracked in the
[current unclosed matrix](../01-overview/current-unclosed-matrix.md). Until it closes, use current Runtime contract tests and
file-backed convergence tests as the authoritative automated baseline.

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

## Legacy Fixture Rule

Historical script filenames, scenario IDs and read-model compatibility fields may remain visible while being migrated. They
must be labeled compatibility-only and cannot be cited as proof that removed `executionGraph`, `NextAction`,
`ExecutionContextPatch` or application-created successor behavior is supported.
