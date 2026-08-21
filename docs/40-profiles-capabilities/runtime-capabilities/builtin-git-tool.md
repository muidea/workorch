# Built-In Git Capability

Status: normative

Type: profile-capability

Last Verified: 2026-08-16

## Ownership

`blocks/git` owns Git operation admissibility, feasibility, workspace-scoped execution and factual receipts. Orchestrator may
request only an operation already allowed by the activation and owner catalog; it owns evidence judgement, not Git behavior.

## Supported Operations

| Kind | Variant | Required / relevant parameters | Effect / replay class |
| --- | --- | --- | --- |
| `git/read` | `status` | optional timeout | none / safe to replay |
| `git/read` | `diff` | optional pathspecs, ref/baseRef, timeout | none / safe to replay |
| `git/read` | `log` | optional ref, limit, pathspecs, timeout | none / safe to replay |
| `git/read` | `show` | optional ref, pathspecs, timeout | none / safe to replay |
| `git/read` | `branch` | optional timeout | none / safe to replay |
| `git/write` | `init` | optional timeout | workspace durable / owner-managed |
| `git/write` | `add` | non-empty workspace-relative `pathspecs` | workspace durable / safe to replay |
| `git/write` | `commit` | non-empty `message`, optional `expectedHead` | workspace durable / at most once |

`push`、`pull`、`fetch`、`merge`、`checkout`、`reset`、`rebase`、tag mutation 和 remote management 不在当前 Runtime
surface。不得用 `shell/command` 绕过缺失的 Git variant 或 evidence requirement。

## Scope And Fences

- working directory is the Workspace deliverable root selected by owner context;
- pathspecs must stay within that root;
- `expectedHead` is an optimistic fence for commit when provided;
- dispatch/operation identity and owner receipt make duplicate delivery distinguishable from a new request;
- command output is bounded and normalized before crossing the owner boundary.

## Evidence Semantics

- status/diff/log/show/branch are observation facts; they do not prove state change.
- init/add/commit receipts describe the actual repository transition.
- a version-control snapshot requirement normally needs a matching accepted `git/write:commit` receipt; shell `git log` or a
  text summary cannot substitute.
- only Orchestrator can bind the receipt to current CWI/coverage and accept it.

## Failure Semantics

Invalid variant/parameters, path escape, repository precondition, expected-HEAD conflict, command timeout and Git exit failure
return typed factual failure. Git owner does not choose retry, repair, governance or terminal state.

## Verification

- `go test ./internal/modules/blocks/git/... -count=1`
- `make verify-runtime-positive-convergence-file`
