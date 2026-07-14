# Goal/DoD NextAction S1.6 TUI/REST Command Path Parity

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S1.1-S1.2 Run Command Contract](./goal-dod-nextaction-s1-1-s1-2-run-command-contract.md)
- [S1.3 Run Command Projection](./goal-dod-nextaction-s1-3-run-command-projection.md)
- [S1.5 Restart Replacement Semantics](./goal-dod-nextaction-s1-5-restart-replacement.md)

## Contract Audit

- Scope: TUI/remote REST/EventHub run command entry parity for `resume`, `replan`, and `restart`.
- Observed state: TUI submits canonical `WorkspaceRunCommand` through `internal/services/workorch/biz/backends`. Embedded backend publishes `TopicWorkorchDCommandWorkspaceRunRequested`; remote backend translates the same DTO to REST endpoints. Server endpoints then publish the same scheduler owner command and project accepted/rejected status.
- Expected state: TUI and REST are input adapters only. Both must carry the same workspace/run/decision scope and command metadata into the workorchd scheduler owner path without TUI-owned business state transitions.
- Deviation: EventHub restart had unified command coverage and REST server had projection coverage, but remote restart lacked a focused test proving `WorkspaceRunCommand{Intent:restart}` sends restart command metadata to the REST adapter path.
- Owner: port/backend adapters and workorchd scheduler. This is command plumbing, not template/Soul/policy.
- Event path: TUI command -> backend `WorkspaceRunCommand` -> embedded EventHub or remote REST -> workorchd scheduler command event -> accepted/rejected projection.
- Contract gap: remote restart parity was implicit in implementation and not independently verified.
- Proposed change: add remote backend restart test proving the same canonical command metadata is sent to `/execution-restarts`; keep existing EventHub and server tests as the owner-path verification.
- Boundary check: adapter tests assert command intent/scope/metadata only. No business template, language, provider, file, or tool-specific rule is introduced.
- Verification: remote backend test, existing EventHub/server command tests, TUI run command tests, and boundary search.

## Closure Items

1. Done: embedded backend restart publishes the unified workspace-run scheduler command.
2. Done: remote restart sends canonical command metadata to the workspace restart REST endpoint.
3. Done: server REST endpoints publish scheduler commands and project accepted/rejected status.
4. Done: TUI tests cover canonical `/run resume`, `/run replan`, and `/run restart` command submission through backend command APIs.

## Verification

Commands:

```bash
go test ./internal/services/workorch/biz/backends -run 'RemoteRunCommandRestartSendsWorkspaceRestartCommandMetadata|EventHubRunRestartPublishesUnifiedWorkspaceRunCommand' -count=1
go test ./internal/services/workorch/service/tui -run 'ModelRunRestart|ModelRunReplan|ModelRunResume|NonCanonicalRunCommands' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'WorkorchDWorkspaceRestartHTTPProjectsRestartIntent|WorkorchDWorkspaceRunCommandResumeEnqueuesRunContinuation|WorkorchDRunResumeHTTPProjectsReplanIntent' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/services/workorch/biz/backends/remote_test.go docs/70-roadmap/active/goal-dod-nextaction-s1-6-command-path-parity.md -S
git diff --check
```
