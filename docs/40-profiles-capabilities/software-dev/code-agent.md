# Software-Dev Code Agent Profile

Status: active

Type: profile-capability

Last Verified: 2026-08-20

## Purpose

`software-dev` specializes neutral Runtime contracts for code delivery. It supplies template/Soul guidance, coverage roles,
evidence requirements and allowed capability classes; it does not add a code-specific state machine to Orchestrator.

## Expected Behavior

- inspect the current Workspace and relevant contract shape before editing;
- make the smallest coherent production change tied to an active coverage work item;
- use create-only file operations only for absent targets and revision-bound replacement for existing targets;
- run bounded build/test/lint or direct entrypoint checks relevant to the DoD;
- preserve user changes and avoid unrelated rewrites;
- when meaningful output changed, use supported Git add/commit operations if a version-control snapshot is required;
- report explicit evidence refs, skipped checks, residual risks and true blockers.

## Capability Mapping

| Need | Typical capability | Constraint |
| --- | --- | --- |
| inspect files | `file/list`, `file/read` | observation only |
| create file | `file/write` or `patch/apply:file_write` | target must be absent unless owner variant explicitly carries replacement fence |
| modify file | `file/edit` or `patch/apply:text_replace|unified_diff` | current revision/base hash required by owner contract |
| validate | `validation/managed_session` | owner-sealed recipe, bounded isolated workspace copy, foreground process group |
| bounded command state change | `shell/command` | explicit durable targets; never validation/read-only evidence |
| inspect/version | `git/read`, `git/write:init|add|commit` | no unsupported remote/destructive variants |
| semantic assistance | selected LSP MCP tools | auxiliary; cannot replace build/test evidence |

Only operations present in the activation-sealed capability contract and current owner admissibility catalog are candidates.

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
