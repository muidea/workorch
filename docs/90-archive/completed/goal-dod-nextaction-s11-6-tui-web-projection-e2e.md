# S11.6 TUI/Web Projection E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [Workorchd State Loop And Projection Closure Plan](./workorchd-state-loop-and-projection-closure-plan.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [S8.2 TUI Prompt Mode From Projection](./goal-dod-nextaction-s8-2-tui-prompt-mode-projection.md)
- [S8.3 TUI List And Main View Same Revision](./goal-dod-nextaction-s8-3-tui-list-main-same-revision.md)
- [S8.4 Command Pending And Accepted Display](./goal-dod-nextaction-s8-4-command-pending-accepted-display.md)
- [S8.5 SSE Reconnect And Last Event ID](./goal-dod-nextaction-s8-5-sse-reconnect-last-event-id.md)
- [S8.6 Decision And Approval View Regression](./goal-dod-nextaction-s8-6-decision-approval-view-regression.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)

## Contract Audit

- Scope: deployed or managed WorkOrch service projection, Web Console projection display, and TUI projection rendering for workspace switch, prompt/list/main/status consistency, terminal rendering, and long-output scrolling.
- Observed state: S8.x slices already moved prompt/list/main/status rendering to workorchd-owned projection and port DTOs. Existing deployed/local verification targets cover service projection scope, Web Console projection/heartbeat rendering, TUI workspace switch view snapshots, real PTY terminal rendering, and long-output scrolling. The roadmap did not yet have a first-class S11.6 document or single Make target tying those checks to the TUI/Web projection e2e acceptance item.
- Expected state: workorchd remains the ViewModel owner. Web and TUI submit commands and render the projected state only. Workspace switching must not leak stale workspace content into prompt, list, main view, status bar, run detail, or terminal viewport. Web must read server workspace projection and workspace-scoped run snapshot/observability, and must not rebuild formal run state client-side.
- Deviation: coverage existed as separate scripts and a projection closure plan, but S11.6 lacked an explicit roadmap artifact and grouped acceptance target. That made later verification prone to picking only one script and missing the cross-client MVVP contract.
- Owner: e2e. This slice groups existing service/Web/TUI projection checks. It does not change core runtime, template/Soul/policy, LLM context, or client-side state logic.
- Event path: workspace/run/domain event -> workorchd projection owner -> workspace snapshot/list/SSE/readmodel API -> Web/TUI port/client DTO -> Web/TUI rendering. TUI and Web must not derive a second formal runtime state machine from partial local data.
- Ledger impact: none. The slice validates projection visibility and client rendering only; it does not alter coverage ledger, evidence ledger, completion gate, or NextAction interpretation.
- Stage Packet impact: none.
- Contract gap: missing S11.6 delivery artifact and a single validation entry for service projection plus Web/TUI projection rendering.
- Proposed change: document S11.6, expose `verify-tui-web-projection-e2e`, and make `verify-e2e-full` depend on that grouped target instead of listing the same projection checks as unrelated entries.
- Boundary check: no production core or template behavior is changed. E2E scripts may contain fixture strings and visual markers, but assertions target projection metadata, workspace scope, server-projected state, PTY rendering, and absence of client-side formal-state rebuilding.
- Verification: Make target graph check, shell syntax checks for shell scripts, Python syntax check for the PTY smoke, boundary search over changed files, and focused S11.6 target execution when a deployed or managed service is available.

## E2E Contract

- `verify-workspace-projection-switch` validates deployed workspace projection scope, list/detail metadata, projection SSE revision/timing payload, projection rebuild metrics, and unscoped run action rejection.
- `verify-web-console-projection-visual` validates Web Console source, dist assets, and optional deployed runtime artifacts for projection status, run heartbeat, latest effective progress, stale/stalled markers, workspace-scoped run reads, stale response guards, and no client-side formal run snapshot reconstruction. In runtime mode it creates and purges its own temporary workspace so the e2e does not depend on pre-existing deployment state.
- `verify-tui-agent` validates TUI command/rendering architecture guards, including no fixed polling regression for projection refresh behavior.
- `verify-tui-workspace-switch-visual` records text snapshots before switch, after switch, after projection refresh, and after inactive projection events, proving prompt/list/main/status isolation by active workspace.
- `verify-tui-terminal-render` starts the real TUI through a PTY against a deployed service, loads a workspace projection, submits `/status`, and exits cleanly.
- `verify-tui-long-output` validates long result rendering through the real PTY, PgDown/PgUp behavior, fixed workspace panel and prompt/help rows, and disabled xterm mouse reporting.

## Acceptance

- `verify-tui-web-projection-e2e` exists as the grouped S11.6 validation target.
- `verify-e2e-full` includes S11.6 through `verify-tui-web-projection-e2e`.
- Existing individual verification targets remain callable for focused debugging.
- Web/TUI projection e2e remains a validation layer only; production ViewModel ownership stays in workorchd.

## Verification

- `make -n verify-tui-web-projection-e2e`
- `bash -n scripts/verify-workorch-projection-workspace-switch.sh scripts/verify-web-console-projection-visual.sh scripts/verify-tui-agent.sh scripts/verify-tui-workspace-switch-visual.sh scripts/verify-tui-long-output-e2e.sh`
- `python3 -m py_compile scripts/verify-tui-terminal-render-smoke.py`
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-tui-web-projection-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=1 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-6-tui-web-projection-e2e.md docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md`
