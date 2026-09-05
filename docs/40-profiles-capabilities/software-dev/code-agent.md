# Software-Dev Code Agent Profile

Status: normative

Type: profile-capability

Last Verified: 2026-09-04

## Purpose

`software-dev` specializes neutral Runtime contracts for code delivery. The stable ID currently selects a Go-native validation
adapter and should be presented as **Software development (Go-native validation)**. It supplies template/Soul guidance,
coverage roles, evidence requirements and allowed capability classes; it does not add a code-specific state machine to
Orchestrator. Other framework-native or MCP-only proof closures require a separately declared typed Profile.

## Expected Behavior

- inspect the current Workspace and relevant contract shape before editing;
- make the smallest coherent production change tied to an active coverage work item;
- use create-only file operations only for absent targets and revision-bound replacement for existing targets;
- consolidate all durable changes to the same owner/canonical target into one operation; dependency order does not authorize a
  second same-target patch because both replay proofs were sealed before execution; follow an exact owner-required operation
  sequence when the manifest explicitly seals one;
- run bounded build/test/lint or direct entrypoint checks relevant to the DoD;
- preserve user changes and avoid unrelated rewrites;
- when meaningful output changed, use supported Git add/commit operations if a version-control snapshot is required;
- report explicit evidence refs, skipped checks, residual risks and true blockers.

## Capability Mapping

| Need | Typical capability | Constraint |
| --- | --- | --- |
| inspect files | `file/list`, `file/read` | observation only |
| create file | `file/write` | prospective target must be absent and owner-admitted |
| modify existing file | `patch/apply:unified_diff` | one minimal multi-hunk diff per owner/canonical target; current revision/base hash required |
| validate | `validation/managed_session` | current built-in Profile: Shell `go test -json` exact native check; owner-sealed recipe, bounded isolated workspace copy, foreground process group |
| bounded command state change | `shell/command` | explicit durable targets; never validation/read-only evidence |
| inspect/version | `git/read`, `git/write:init|add|commit` | no unsupported remote/destructive variants |
| semantic assistance | selected LSP MCP tools | auxiliary; cannot replace build/test evidence |

Only operations present in the activation-sealed capability contract and current owner admissibility catalog are candidates.

For the built-in DoD validation-readiness closure, a prospective File create is eligible only when the sealed
`ProspectiveSatisfactionContractRef` lets Shell prove from the File-authored hypothetical resulting manifest that an exact
managed-validation recipe would become selectable. The default `go test -v ./...` recipe requires a root `go.mod` and at least
one non-excluded `*_test.go` in the isolated workspace manifest. When `go.mod` already exists, adding a real `*_test.go` target
can satisfy the remaining all-of applicability contract; a test file alone in a workspace without `go.mod`, or a generic
`scripts/validate.sh`, cannot make the recipe selectable. If module and test prerequisites are both missing, the bounded
collective File preparation must cover both exact mandatory clauses before Shell reobservation. This is software-dev
Profile/Shell policy, not an Orchestrator filename or language rule.

## Runtime Mapping

The Definition seed is transformed into CWI/focus/binding through bounded semantic planning and owner feasibility. Orchestrator
then uses normal `need_prerequisite`、`need_state_change`、`need_validation`、`need_assessment`、`need_governance` or terminal
convergence. “Planning”, “execution” and “validation” in Souls are semantic roles only.

Repair stays on the current focus when current definition and evidence permit another bounded candidate. Definition changes or
a requested clean baseline use explicit Workspace restart; the code profile never creates a successor Run.

## Evidence Quality

- source/config/test changes need matching owner state-change receipts;
- validation must exercise the relevant production entrypoint, not only a test-local imitation;
- successful command output without meaningful assertions can be inconclusive;
- LSP diagnostics and file reads are supporting observations, not sufficient high-risk acceptance evidence;
- a satisfied assessment must cite current-Run accepted evidence for every closed item.

## Non-Goals

- hard-coding Go/npm/test framework rules in Runtime core;
- returning removed execution graph or next-action authority schemas;
- using shell to bypass file/Git capability contracts;
- creating a Decision merely because an automated retry/repair budget is exhausted.

## Verification

- current typed/core baseline: `go test ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/file/... -count=1`;
- profile projection compatibility: `make verify-runtime-profile-software-dev`;
- `make verify-software-dev-goal-dod-e2e` and `make verify-software-dev-code-agent` exercise current typed bootstrap,
  File-owner admissibility and file-backed patch/restart behavior; their names do not restore a graph dispatcher.
