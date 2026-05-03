# workorch CLI/TUI Agent Requirements

## Purpose

This document tracks the required evolution of `application/workorch` from the
current management CLI prototype into the primary local-first TUI agent
experience for workorch.

The requirements in this document supersede ad-hoc CLI assumptions discovered
during the May 3, 2026 implementation review. This is the single tracking
document for the CLI/TUI agent work until the scope is split into active
delivery plans.

## Target Product Shape

- `workorch` is a CLI agent similar in interaction role to Codex CLI and Claude
  Code.
- The primary local user experience is a TUI agent session, not a passive
  status dashboard.
- The local CLI mode embeds and reuses workorch runtime capability directly.
- The local CLI mode must not start a `workorchd` child process.
- The local CLI mode must not call workorch through HTTP client paths.
- The local CLI mode must not bypass the shared runtime contract by directly
  inventing CLI-only service logic.
- The local CLI mode must use magicCommon EventHub command/event/callback
  interaction for business commands, progress, blocking items, and terminal
  results.
- The local CLI mode must not refresh state through fixed-interval polling.
  State changes must arrive through EventHub subscriptions and asynchronous
  callbacks.
- Remote mode is explicit. Only `workorch --remote <addr>` may behave as a
  remote client for an already running `workorchd`.
- In local mode, the current shell directory is the default Workspace working
  directory.
- The TUI must support Workspace switching and real-time monitoring of all
  Workspaces through EventHub-driven state updates.
- Command-line subcommands must be able to perform all workorchd business
  interactions that are exposed for operators.

## Current Implementation Snapshot

Current code locations:

- CLI entry: `application/workorch/main.go`
- Cobra commands: `internal/modules/application/workorch/service/cli`
- TUI prototype: `internal/modules/application/workorch/service/tui`
- HTTP client: `internal/modules/application/workorch/biz/client`
- local daemon launcher: `internal/modules/application/workorch/biz/local`
- direct agent prototype: `internal/modules/application/workorch/biz/agent`
- workorchd application service: `internal/modules/application/workorchd`
- EventHub contracts/helpers: `internal/pkg/events`
- runtime initiator: `internal/initiators/workorchruntime`

Observed current behavior:

- `make cli-test` passes.
- `make cli-build` builds `./bin/workorch`.
- `make verify-cli` passes its current smoke checks.
- `--local` currently starts a `workorchd` child process and then uses HTTP.
- Most management commands use `biz/client.Client` HTTP APIs.
- `agent` and the default REPL use direct `appservice.RuntimeSession` calls.
- TUI uses HTTP fetch commands and a fixed `2s` polling interval.
- TUI is a status dashboard, not the primary agent interaction surface.

## Non-Goals

- Do not keep or extend the `--local` daemon-launching mode.
- Do not make the local CLI depend on a running HTTP server.
- Do not add a second CLI-only business implementation that can diverge from
  workorchd runtime behavior.
- Do not use periodic polling as the normal local status update mechanism.
- Do not treat the current TUI dashboard as sufficient for agent interaction.

## Required Architecture Direction

The CLI must be refactored around a shared backend boundary:

```text
CLI commands / TUI / Agent session
  -> workorch CLI backend interface
  -> local EventHub backend OR explicit remote backend
```

Local backend:

```text
CLI/TUI action
  -> publish command event to magicCommon EventHub
  -> workorch runtime handles command through existing runtime/application logic
  -> runtime publishes result/progress/state events
  -> CLI receives async callback and updates UI or returns command result
```

Remote backend:

```text
CLI/TUI action
  -> remote transport to an already running workorchd
```

Remote transport can initially remain HTTP, but it must be isolated behind the
same backend interface so TUI/agent/commands do not branch their business logic.

Local-mode forbidden paths:

- Local CLI backend must not import `internal/modules/application/workorch/biz/client`.
- TUI must not import `internal/modules/application/workorch/biz/client`.
- Agent flow must not directly call `appservice.NewRuntimeSessionFromInitiator`
  or repositories for normal business interaction.
- Local CLI state refresh must not use `time.Ticker`, `time.Tick`, `tea.Tick`,
  or equivalent fixed-interval polling.
- Production CLI code must not import
  `internal/modules/application/workorch/biz/local`.
- Local mode must not spawn `workorchd` or depend on an HTTP listener.

Remote-mode policy:

- `--remote <addr>` is the only explicit remote activation path.
- Remote mode may initially use HTTP only behind the shared backend interface.
- No TUI, agent, or Cobra command may import the HTTP client directly.
- If remote mode later needs EventHub-compatible transport, it must be added as
  a backend implementation without changing TUI, agent, or command behavior.

## Task Tracking

Task statuses:

- `pending`: accepted requirement, implementation not started.
- `in_progress`: active implementation is underway.
- `blocked`: implementation cannot proceed without a resolved dependency or
  design decision.
- `implemented`: code has landed, but verification is not complete.
- `verified`: acceptance checks listed in this document have passed.
- `deferred`: intentionally postponed with reason recorded in this document.

Each task must record updated status, changed code locations, and verification
commands before it can move to `verified`.

### CLI-AGENT-01. Define Shared CLI Backend Boundary

Status: pending

Primary location:

- `internal/modules/application/workorch/biz/backend`

Related locations:

- `internal/modules/application/workorch/service/cli`
- `internal/modules/application/workorch/service/tui`
- `internal/modules/application/workorch/biz/agent`

Requirements:

- Define one backend interface consumed by Cobra commands, TUI, and agent
  session logic.
- Cover workspace, goal, DoD, run activation, run resume, decision resolution,
  approval resolution, snapshots/status, event stream, configuration, Skill,
  Soul, MCP, Memory, Result, and Artifact interactions as needed by CLI.
- Keep backend data types stable and not tied directly to HTTP response
  envelopes.
- Support context cancellation and command timeout.
- Support subscribing to state/progress events.
- Provide separate local and remote implementations without exposing transport
  details to callers.

Acceptance:

- `service/cli`, `service/tui`, and `biz/agent` can compile against the backend
  interface without importing `biz/client` directly.
- Backend interface tests cover command result and event subscription behavior
  with a fake backend.
- Static checks prove migrated local packages do not reintroduce direct HTTP
  client imports.

### CLI-AGENT-02. Remove `--local` Daemon Launcher Mode

Status: pending

Primary locations:

- `internal/modules/application/workorch/service/cli/root.go`
- `internal/modules/application/workorch/biz/local`
- `Makefile`
- `scripts/verify-cli.sh`

Requirements:

- Remove the `--local` flag from CLI help and behavior.
- Stop starting `workorchd` as a child process.
- Remove or archive `biz/local/launcher.go` after no code depends on it.
- Update CLI verification scripts so they do not expect `--local`.

Acceptance:

- `workorch --help` no longer lists `--local`.
- No production CLI code imports `internal/modules/application/workorch/biz/local`.
- `make cli-test` and `make verify-cli` pass after script updates.

### CLI-AGENT-03. Implement Local EventHub Backend

Status: pending

Primary locations:

- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/biz/eventhub`
- `internal/pkg/events`
- `internal/initiators/workorchruntime`

Related locations:

- `internal/modules/application/workorchd/biz/appservice`
- `internal/modules/kernel/*`

Requirements:

- Use the application EventHub provided by the embedded magicCommon runtime.
- Publish formal command events for CLI-originated business actions.
- Receive command result events by `correlationId`.
- Subscribe to workspace/run/step/decision/approval/result/artifact/memory
  state events.
- Provide callback delivery to TUI and agent sessions without fixed-interval
  polling.
- Preserve runtime ordering and lane constraints for run, decision, and
  approval flows.
- Surface command failures as structured backend errors.

Acceptance:

- Local workspace create, run activation, decision resolve, approval resolve,
  and run resume can complete without HTTP.
- A local TUI subscriber receives run state changes from EventHub callbacks.
- No local backend operation uses `biz/client.Client`.

### CLI-AGENT-04. Define CLI Command Event Contracts

Status: pending

Primary locations:

- `internal/pkg/events`
- `internal/modules/application/workorch/biz/eventhub`

Requirements:

- Define command envelope fields:
  `commandId`, `correlationId`, `actor`, `issuedAt`, `workspaceId`, `runId`,
  `subjectType`, `subjectId`, and typed payload.
- Define result envelope fields:
  `correlationId`, `commandId`, `ok`, `error`, `resource`, `snapshot`, and
  `occurredAt`.
- Define command topics for workspace create/switch, goal define, DoD define,
  execution activate, run resume, decision resolve, approval approve/reject,
  and management operations required by CLI.
- Define progress topics for runtime step/tool/provider stream updates needed
  by agent and TUI.
- Avoid ad-hoc string payload parsing when typed payloads are available.

Acceptance:

- Event contract tests validate encode/decode, required fields, and version
  compatibility.
- Command result correlation works with concurrent commands.
- Progress events can be filtered by workspace, run, and correlation ID.

### CLI-AGENT-05. Refactor Agent Flow To Backend/EventHub

Status: pending

Primary locations:

- `internal/modules/application/workorch/biz/agent`
- `internal/modules/application/workorch/service/cli/agent_cmd.go`
- `internal/modules/application/workorch/service/cli/agent_repl.go`

Requirements:

- Replace direct `appservice.NewRuntimeSessionFromInitiator()` usage with the
  shared backend.
- Replace repository polling in `pollRun()` with EventHub-driven subscription
  callbacks.
- Stream progress, tool calls, decisions, approvals, and terminal results to
  the UI as events arrive.
- Keep decision and approval handling in the same backend path as normal CLI
  commands.

Acceptance:

- Agent can complete a local run without HTTP and without ticker-based polling.
- Agent receives decision/approval prompts from event callbacks.
- Existing agent tests are rewritten around fake backend/event delivery.

### CLI-AGENT-06. Rebuild TUI As Primary Agent Interface

Status: pending

Primary locations:

- `internal/modules/application/workorch/service/tui`

Related locations:

- `internal/modules/application/workorch/service/cli/repl.go`
- `internal/modules/application/workorch/service/cli/status.go`

Requirements:

- Replace the dashboard-first TUI with an agent-first TUI.
- Provide an input area for user goals/messages.
- Show streaming agent progress, runtime steps, tool calls, decisions,
  approvals, and final results.
- Keep an always-available workspace/status view for all Workspaces.
- Support switching the active Workspace.
- Support inline decision resolution and approval/rejection.
- Support current directory as the default local Workspace working directory.
- Use backend event subscriptions instead of polling.
- Define keyboard behavior for submit, cancel, workspace switch, next blocking
  item, approve/reject, open result/artifact, and quit.
- Define interrupt behavior while a Run is active.

Acceptance:

- Starting `workorch` with no subcommand opens the agent TUI.
- The TUI can start a local run in the current directory Workspace.
- The TUI updates run state from callbacks, not `TickMsg`.
- The TUI can resolve a blocking decision and approval without leaving the
  session.
- The TUI can display terminal Result and Artifact references after a Run
  completes.

### CLI-AGENT-07. Remove TUI HTTP Fetch/Polling Model

Status: pending

Primary locations:

- `internal/modules/application/workorch/service/tui/model.go`
- `internal/modules/application/workorch/service/tui/commands.go`
- `internal/modules/application/workorch/service/tui/messages.go`

Requirements:

- Remove `pollInterval`, `watchMode`, `PollTick`, and periodic `FetchAll`.
- Replace HTTP fetch commands with backend commands and subscription messages.
- Add stale-message guards for workspace/run scoped updates.
- Handle all async errors in model state.
- Fix nil workspace rendering in detail view.
- Replace byte-based truncation with rune/display-width safe truncation.

Acceptance:

- TUI package has no import of `biz/client`.
- TUI package has no fixed interval state refresh.
- TUI package tests cover callback update, stale event ignore, nil workspace
  rendering, and wide-character truncation.

### CLI-AGENT-08. Default Current Directory Workspace Binding

Status: pending

Primary locations:

- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/service/cli/root.go`
- `internal/modules/application/workorch/service/tui`

Requirements:

- In local mode, resolve the shell current working directory at CLI startup.
- Find or create a default Workspace bound to that working directory.
- Use that Workspace as the initial active Workspace for TUI and agent runs.
- Allow switching to another Workspace and switching back to the current
  directory Workspace.
- Keep remote mode behavior explicit and avoid implicit local writes when
  `--remote` is used.

Acceptance:

- Running `workorch` in a project directory shows or creates the corresponding
  Workspace.
- A run started without explicit workspace uses the current directory
  Workspace.
- Switching Workspace updates subsequent agent commands.

### CLI-AGENT-09. Complete Command-Line Coverage For workorchd Interactions

Status: pending

Primary locations:

- `internal/modules/application/workorch/service/cli`
- `internal/modules/application/workorch/biz/backend`

Related locations:

- `internal/modules/application/workorchd/service/server/http.go`

Requirements:

- Cover all operator-facing workorch interactions currently available through
  workorchd APIs.
- Maintain a command coverage table in this document or a generated companion
  file, mapping:
  backend method -> local EventHub command -> remote transport -> Cobra command
  -> tests.
- Add missing command coverage for:
  goals, DoDs, run snapshot, task progress, context bundle, task
  decompositions, artifact content, configuration LLM update, Soul
  create/update/delete, Skill get/create and workspace scope, MCP server
  create/update, MCP capability list/get/create/update, Memory create/list,
  workspace template get/create/update/delete, batch decision resolution, and
  batch approval resolution.
- Ensure local commands use EventHub backend and remote commands use the same
  backend interface.

Acceptance:

- A generated command coverage table maps every operator-facing backend method
  to a Cobra command.
- CLI tests cover representative read/write commands in fake backend mode.
- The coverage table identifies unsupported operations explicitly instead of
  silently omitting them.

### CLI-AGENT-10. Remote Mode Isolation

Status: pending

Primary locations:

- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/biz/client`
- `internal/modules/application/workorch/service/cli/root.go`

Requirements:

- Keep `--remote <addr>` as the only remote-client activation path.
- Hide HTTP details behind the shared backend.
- Ensure remote mode does not initialize local EventHub command processing for
  business writes.
- Ensure local mode does not instantiate the HTTP client.

Acceptance:

- In local mode, tests prove no HTTP client is used.
- In remote mode, tests prove commands are routed to the remote backend.
- CLI help clearly describes default local embedded mode and explicit remote
  mode.

### CLI-AGENT-11. Verification And Regression Baseline

Status: pending

Primary locations:

- `Makefile`
- `scripts/verify-cli.sh`
- `internal/modules/application/workorch/**`

Requirements:

- Update `make cli-test` to cover backend, TUI model, agent event flow, and
  command routing.
- Update `make verify-cli` to validate default local embedded mode.
- Add a no-HTTP local verification check.
- Add a no-polling TUI static check.
- Keep existing workorchd verification targets unchanged unless a shared
  contract requires updates.

Acceptance:

```bash
make cli-test
make verify-cli
go test ./internal/modules/application/workorch/... --count 1
```

Additional expected checks:

- no local CLI import of `internal/modules/application/workorch/biz/client`
  outside remote backend implementation
- no local TUI `time.Tick`, `tea.Tick`, or fixed poll interval for state
  refresh
- no production import of `internal/modules/application/workorch/biz/local`

### CLI-AGENT-12. Implement Local Runtime Lifecycle

Status: pending

Primary locations:

- `application/workorch/main.go`
- `internal/modules/application/workorch`
- `internal/initiators/workorchruntime`
- `internal/modules/application/workorch/biz/backend`

Requirements:

- Define how `workorch` initializes the embedded local runtime at CLI startup.
- Reuse the application EventHub and runtime exports created by magicCommon
  lifecycle.
- Keep local config directory behavior separate from current working directory
  Workspace binding.
- Ensure Ctrl-C and context cancellation stop active command waits and
  unsubscribe callbacks.
- Ensure shutdown releases runtime subscriptions and application resources.
- Define behavior for multiple simultaneous local CLI/TUI sessions.

Acceptance:

- Starting `workorch` with no `--remote` initializes local runtime once.
- Exiting TUI or pressing Ctrl-C unsubscribes EventHub callbacks and shuts down
  cleanly.
- Tests cover startup failure, cancellation, and shutdown idempotency.

### CLI-AGENT-13. Implement EventHub Command Dispatcher

Status: pending

Primary locations:

- `internal/modules/application/workorch/biz/eventhub`
- `internal/modules/application/workorchd/biz/appservice`
- `internal/initiators/workorchruntime`
- `internal/pkg/events`

Requirements:

- Provide the runtime-side consumer for CLI command topics.
- Dispatch command events to existing workorch appservice/runtime capabilities
  instead of reimplementing business logic.
- Publish command result events with matching `correlationId`.
- Publish command failure events with structured error code and message.
- Preserve workspace/run ordering guarantees required by kernel state
  transitions.
- Ensure dispatcher startup and teardown are owned by embedded local runtime
  lifecycle.

Acceptance:

- EventHub command dispatcher tests cover workspace create, execution activate,
  decision resolve, approval approve/reject, and run resume.
- Concurrent commands with different correlation IDs receive the correct result.
- Dispatcher failures do not leave command waiters blocked until process exit.

### CLI-AGENT-14. Define Detailed TUI Interaction Contract

Status: pending

Primary locations:

- `docs/workorch-cli-tui-agent-requirements.md`
- `internal/modules/application/workorch/service/tui`

Requirements:

- Define the first-screen layout for the agent session.
- Define the main input area behavior for goals, follow-up messages, and
  decision answers.
- Define the event stream display for agent progress, runtime steps, tool
  calls, provider stream output, decisions, approvals, and final results.
- Define the all-Workspace monitor panel and active Workspace switcher.
- Define final Result and Artifact display behavior.
- Define keyboard actions for submit, cancel, workspace switch, next blocking
  item, approve/reject, open result, and quit.
- Define interrupt semantics for Ctrl-C while a Run is active.
- Define what remains visible after a Run completes or fails.

Acceptance:

- TUI implementation tasks can point to this task as their interaction
  contract.
- TUI tests cover submit, workspace switch, blocking decision, blocking
  approval, progress update, terminal result, and interrupt flows.

### CLI-AGENT-15. Static Architecture Guards

Status: pending

Primary locations:

- `scripts`
- `Makefile`
- `internal/modules/application/workorch/**`

Requirements:

- Add static checks that fail if forbidden local-mode dependencies return.
- Check that TUI does not import `biz/client`.
- Check that agent does not directly call `appservice.NewRuntimeSessionFromInitiator`
  for normal business interaction.
- Check that production CLI code does not import `biz/local`.
- Check that local TUI state refresh does not use fixed polling constructs.
- Include the guard in `make cli-test` or a dedicated CLI verification target.

Acceptance:

- A deliberate forbidden import or polling call fails the guard script.
- Guard script output points to the offending file and line.

## Current TUI Defect List

These defects are tracked here until `CLI-AGENT-06` and `CLI-AGENT-07` replace
the current TUI model.

- TUI imports `biz/client` directly and is coupled to HTTP.
- TUI refreshes state through `pollInterval = 2s` and `PollTick`.
- TUI is dashboard/status oriented, not agent-session oriented.
- TUI does not bind the current directory to the default Workspace.
- TUI cannot start runs or resolve decisions/approvals from implemented model
  actions even though message structs exist.
- `WorkspaceDetailView` can panic when `findWorkspace` returns nil.
- `WorkspaceSnapshotMsg` does not check `WorkspaceID` against `currentWS`, so
  stale async responses can render the wrong Workspace.
- List message errors for decisions, approvals, and runs are mostly ignored.
- `modeRunsList`, `RunSnapshotMsg`, and `RunObservabilityMsg` are effectively
  unused.
- Key constants in `keys.go` are not used by `model.go`.
- Byte-based truncation can corrupt UTF-8 and mismeasure wide characters.
- ANSI styled strings are padded with `fmt %-*s`, which can break alignment.

## Open Design Questions

- Which EventHub lane should CLI command events use for workspace-scoped
  operations versus run-scoped operations?
- Should command result events be persisted into the formal Event Stream, or
  treated as transient control-plane events?
- What is the canonical Workspace identity for a working directory: absolute
  path hash, repository root hash, or explicit user-selected name?
- How should CLI handle multiple simultaneous local TUI sessions in the same
  working directory?

## Suggested Delivery Order

1. `CLI-AGENT-01 Define Shared CLI Backend Boundary`
2. `CLI-AGENT-12 Implement Local Runtime Lifecycle`
3. `CLI-AGENT-04 Define CLI Command Event Contracts`
4. `CLI-AGENT-13 Implement EventHub Command Dispatcher`
5. `CLI-AGENT-03 Implement Local EventHub Backend`
6. `CLI-AGENT-02 Remove --local Daemon Launcher Mode`
7. `CLI-AGENT-10 Remote Mode Isolation`
8. `CLI-AGENT-05 Refactor Agent Flow To Backend/EventHub`
9. `CLI-AGENT-08 Default Current Directory Workspace Binding`
10. `CLI-AGENT-14 Define Detailed TUI Interaction Contract`
11. `CLI-AGENT-07 Remove TUI HTTP Fetch/Polling Model`
12. `CLI-AGENT-06 Rebuild TUI As Primary Agent Interface`
13. `CLI-AGENT-09 Complete Command-Line Coverage`
14. `CLI-AGENT-15 Static Architecture Guards`
15. `CLI-AGENT-11 Verification And Regression Baseline`
