# S8.2 TUI Prompt Mode From Projection

Status: completed

Type: roadmap-slice

Owner: TUI

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: TUI input prompt mode and contextual slash command suggestions for the active workspace.
- Observed state: TUI renders the input prompt suffix from `activeWorkspaceOverview().State` and uses local `activeBlockingDecision()` / `activeBlockingApproval()` caches to decide contextual command suggestions. These caches can lag behind workspace switches or local decision/approval resolution events.
- Expected state: workorchd projection should own prompt mode. TUI should only render the prompt and contextual commands from projection-derived prompt mode, then fetch blocker details only when a projected decision or approval mode requires an actionable ID.
- Deviation: prompt rendering and command suggestions can still be influenced by local blocker caches, so the input prompt can diverge from the active workspace projection after workspace switching or resolution events.
- Owner: TUI consumes projection; workorchd appservice owns the prompt mode field in `WorkspaceViewModel`.
- Event path: EventHub source event -> workspaceviewcore projection -> `WorkspaceViewModel.promptMode` -> port/client/backend snapshot -> TUI active workspace runtime state -> prompt/commands render.
- Ledger impact: none.
- Stage Packet impact: none.
- Contract gap: no explicit projected prompt mode is transported to TUI, and contextual command suggestions still use local blocker cache as a mode source.
- Proposed change: add neutral `promptMode` metadata to `WorkspaceViewModel`, populate it from workspace projection state, cache it in the TUI runtime state, and render prompt suffix/contextual command suggestions only from that projection-derived mode.
- Boundary check: prompt modes are generic UI modes: `workspace`, `running`, `decision`, `approval`. The change does not inspect template ID, Soul ID, provider, command output, language, goal text or business content.
- Verification: focused TUI tests for prompt/suggestions after projection switch, workspaceviewcore ViewModel tests, backend decode tests, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- Add `promptMode` to ViewModel metadata DTOs.
- Populate `promptMode` from workspace projection state.
- Store the latest projected prompt mode in TUI workspace runtime state.
- Render prompt suffix and contextual slash commands from projected prompt mode.
- Preserve decision/approval submission behavior by resolving blocker IDs from projection before submitting.

Out of scope:

- Workspace list/main view same-revision rendering.
- Command pending/accepted visual state.
- SSE reconnect behavior.
- Web UI changes.

## Acceptance

- TUI input prompt suffix uses projection-derived prompt mode, not local blocker cache.
- Contextual decision/approval slash suggestions use projection-derived prompt mode.
- Switching workspace cannot carry the previous workspace local blocker cache into the prompt mode.
- Decision/approval detail caches remain available for labels and submission payloads but are not prompt mode owners.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `go test ./internal/services/workorch/service/tui -run 'Prompt|DecisionSlashCompletion|ApprovalSlashCompletion|WorkspaceSwitch|LiveStateEventClearsStaleDecisionPrompt|LiveRunStateEventClearsStaleDecisionPrompt' -count=1`
- Passed: `go test ./internal/services/workorch/service/tui -count=1`
- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'TestStoreReplaceAllListSnapshotAndDelete' -count=1`
- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`
- Passed: `go test ./internal/services/workorch/biz/backends -run 'TestDecodeSnapshotInfoPreservesWorkspaceViewModel' -count=1`
- Passed: `go test ./internal/services/workorch/biz/backends -count=1`
- Passed: `go test ./internal/pkg/workorchclient ./internal/pkg/workorchport -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched Go files.
