# S8.5 SSE Reconnect And Last Event ID

Status: completed

Type: roadmap-slice

Owner: Web/client

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [S8.3 TUI List And Main View Same Revision](./goal-dod-nextaction-s8-3-tui-list-main-same-revision.md)
- [S8.4 Command Pending And Accepted Display](./goal-dod-nextaction-s8-4-command-pending-accepted-display.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: Web console SSE subscription hook and API event-stream URL construction.
- Observed state: workorchd `/api/v1/workorch/events/stream` already accepts `Last-Event-ID` and `lastEventId`, replays persisted and in-memory envelopes, and emits `id:` for each envelope. Web `useSSE` stores the last event id and passes it as query parameter when it reconnects. However, after `maxRetries` is reached, the hook calls `onExhausted` and stops reconnecting, which can leave Web pages permanently stale after a transient network or incomplete chunked stream failure.
- Expected state: Web must keep using owner-projected SSE state with `lastEventId` replay and must never create a second local state machine. A broken stream should trigger refresh and reconnect with the last observed event id; after the normal retry budget is exhausted, it should notify the page once and continue slow reconnect attempts rather than permanently stopping.
- Deviation: retry exhaustion can become a permanent terminal state for the client-side event stream.
- Owner: Web/client consumes workorchd projection stream. workorchd remains the ViewModel and replay owner.
- Event path: workorchd projection event -> SSE envelope with id -> Web hook records id -> reconnect URL includes `lastEventId` -> workorchd replay -> page refresh from projection.
- Ledger impact: none.
- Stage Packet impact: none.
- Contract gap: Web retry policy treats stream exhaustion as terminal instead of degraded-but-recovering.
- Proposed change: make `useSSE` continue reconnecting with capped slow retry after exhaustion, notify `onExhausted` once per outage, reset exhaustion after a successful open/event, and keep forwarding `lastEventId` through the existing query parameter.
- Boundary check: the change only handles generic event-stream transport state. It does not inspect template ID, Soul ID, provider, command output, programming language, goal text, or business content.
- Verification: Web TypeScript/Vite build, existing server SSE replay tests, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- Web hook retry policy.
- Last event id preservation across reconnect attempts for the same workspace/run scope.
- One-time exhausted notification per outage while slow retry continues.

Out of scope:

- Changing workorchd projection or replay semantics.
- Adding local Web business state.
- TUI event subscription behavior.
- Decision/approval view regression beyond stream refresh reliability.

## Acceptance

- `useSSE` reconnect URL continues to include the latest known `lastEventId`.
- After normal retry budget exhaustion, Web reports degraded stream state but keeps retrying.
- A successful open or event resets retry/exhaustion state.
- Existing page `onReconnect`/`onExhausted` refresh callbacks continue to work.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `npm run build` in `web/workorch-console`
- Passed: `go test ./internal/modules/application/workorchd/service/server -run 'EventStreamReplay|EventStreamReplays|SSE' -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched Web source files. The only generic `server` matches were in the existing server test package/path and local test variables.
