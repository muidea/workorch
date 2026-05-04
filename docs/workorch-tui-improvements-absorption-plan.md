# workorch TUI improvement absorption plan

## Purpose

This document tracks the review and absorption of
`agent-tui/docs/WORKORCH_IMPROVEMENTS.md` into the current `workorch` TUI.

The absorption must preserve the confirmed workorch boundaries:

- TUI remains result-first and does not expose a detailed debug event stream.
- TUI state is derived from backend/EventHub data; no local second source of
  truth for runs, decisions, approvals, or workspace state.
- Local TUI/CLI interaction remains EventHub driven, with no local HTTP client
  path and no polling.
- Bubble Tea mouse reporting must remain disabled so terminal-native text
  selection and copy continue to work.

## Status legend

- `pending`: accepted but not implemented.
- `in_progress`: being implemented.
- `verified`: implemented and verified.
- `absorbed_by_existing`: current implementation already satisfies the useful
  part of the proposal.
- `deferred`: useful but intentionally delayed because it needs a separate
  design or validation pass.
- `rejected`: not aligned with current product boundaries.

## Absorption tasks

| ID | Source proposal | Decision | Status | Acceptance |
| --- | --- | --- | --- | --- |
| TUI-IMP-01 | Welcome page / empty-state guidance | Absorb directly | verified | Empty TUI state explains how to use `/workspace create`, `/help`, and `/dod` without requiring an external CLI command. |
| TUI-IMP-02 | Theme system | Absorb directly | verified | Styles are derived from a small theme palette; `WORKORCH_THEME=light|dark|auto` can override automatic selection. |
| TUI-IMP-03 | Double Ctrl+C exit protection | Absorb directly | verified | During a run, `Ctrl+C` interrupts; outside a run, first `Ctrl+C` shows a quit hint and second `Ctrl+C` within a short window exits. `/exit` remains immediate. |
| TUI-IMP-04 | Tab completion dropdown | Absorb directly | verified | Slash command candidates are visible above the input while typing a slash command; `Tab` still completes to the first match. |
| TUI-IMP-05 | Markdown rendering upgrade | Absorb with default glamour renderer | verified | Final Result rendering distinguishes headings, bullets, inline code, and fenced code blocks without leaking raw Markdown delimiters; glamour is the default renderer for Result/Artifact text; `WORKORCH_TUI_MARKDOWN=plain` explicitly falls back to the lightweight renderer; no mouse or event-stream behavior changes. |
| TUI-IMP-06 | Viewport component instead of manual scrollOffset | Absorb keyboard-only viewport | verified | Main display area uses Bubble viewport for `PgUp`/`PgDown` keyboard scrolling; right workspace panel and input/help bars stay fixed; Bubble Tea mouse reporting and viewport mouse wheel handling remain disabled so terminal-native selection/copy is preserved. |
| TUI-IMP-07 | Conversation history persistence | Defer backend-derived reconstruction | deferred | Future implementation must rebuild a compact Conversation from backend Run/Goal/Decision/Approval/Result/Memory data, not local JSON cache. |
| TUI-IMP-08 | Step execution timeline | Reject as detailed timeline; absorb only compact progress already present | absorbed_by_existing | TUI shows compact progress state only. Detailed step/event history remains backend-owned. |
| TUI-IMP-09 | Tool-call visualization panel | Reject for default TUI | rejected | Tool and execution evidence remains available through backend records and artifacts; default TUI keeps result-first display. |

## Implementation notes

- This round focuses on `TUI-IMP-01` through `TUI-IMP-06`.
- `TUI-IMP-06` is implemented as a keyboard-only viewport after the separate
  UX pass confirmed mouse reporting should stay disabled. `TUI-IMP-07` remains
  deferred because it needs backend-derived conversation reconstruction.
- `TUI-IMP-08` and `TUI-IMP-09` are closed by product boundary decision rather
  than code changes.
- Cleanup after implementation removed stale styling code and hard-coded alert
  border color usage. Alert panels now reuse the theme-derived `AlertBorder`.
- Glamour rendering is now the default for Result/Artifact text.
  `WORKORCH_TUI_MARKDOWN=plain` explicitly falls back to the lightweight
  renderer while validating rollout behavior.
- The default workorch glamour style is tightened for TUI usage: document-level
  leading/trailing blank space is minimized, heading marker prefixes such as
  `##` are suppressed, tables are kept compact, and preserved newlines keep
  multi-line results readable.
- Result-level artifact references are not rendered twice: when the Result
  body already contains an artifact index, the separate bottom `Artifacts`
  section is hidden.
- The workspace monitor renders each workspace as a single compact line. When
  workspace titles collide, the short workspace ID is kept visible before the
  state so duplicate names remain distinguishable in the narrow right panel.
- Main result display uses Bubble viewport for keyboard scrolling only. No
  Bubble Tea mouse reporting, local HTTP client, or fixed-interval polling was
  introduced; viewport mouse wheel handling is explicitly disabled.

## Verification commands

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui -count=1
./scripts/verify-cli-architecture.sh
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build make verify-software-dev-code-agent
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build go test ./... -count=1
WORKORCH_LLM_VERIFY_BASE_URL=http://127.0.0.1:8080 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh
```

## Verification record

2026-05-04:

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui -count=1
./scripts/verify-cli-architecture.sh
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build make verify-software-dev-code-agent
GOCACHE=/tmp/workorch-go-build go test ./... -count=1
WORKORCH_LLM_VERIFY_BASE_URL=http://127.0.0.1:8080 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh
```

Result: all passed. The architecture guard still confirms no local TUI HTTP
client, no fixed-interval polling, and mouse selection/copy preservation. The
deployed service e2e also passed against `http://127.0.0.1:8080`.

2026-05-04:

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui -count=1
WORKORCH_TUI_MARKDOWN=plain GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui -count=1
./scripts/verify-cli-architecture.sh
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
```

Result: passed. Glamour rendering is the default path for `Result`/`Artifact`
text, and `WORKORCH_TUI_MARKDOWN=plain` keeps the lightweight fallback path
available for comparison. Keyboard-only viewport scrolling is covered by TUI
unit tests, and the architecture guard rejects Bubble Tea mouse reporting,
viewport mouse wheel enablement, local HTTP client usage, and fixed polling.
The existing TUI acceptance target also passed.

2026-05-04:

```bash
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
```

Result: passed with `TUI-E2E-12`, which verifies keyboard viewport scrolling,
fixed workspace monitor rendering, and disabled viewport mouse wheel reporting.

Long-output manual confirmation fixture:

```bash
WORKORCH_TUI_LONG_OUTPUT_BASE_URL=http://127.0.0.1:8080 GOCACHE=/tmp/workorch-go-build make verify-tui-long-output
```

The script creates a deployed-service workspace with a deterministic long final
Result containing `LONG-IMPLEMENTATION` markers, then prints the workspace ID
for manual TUI `PgDown` / `PgUp` confirmation.
