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
- shared CLI backend: `internal/modules/application/workorch/biz/backend`
- HTTP client: `internal/modules/application/workorch/biz/client`
- agent implementation: `internal/modules/application/workorch/biz/agent`
- workorchd application service: `internal/modules/application/workorchd`
- EventHub contracts/helpers: `internal/pkg/events`
- runtime initiator: `internal/initiators/workorchruntime`

Observed current behavior:

- `make cli-test` requires a writable Go build cache in this sandbox; with
  `GOCACHE=/tmp/workorch-go-build`, the workorch CLI package tests and static
  architecture guard pass.
- `make cli-build` builds `./bin/workorch`.
- `make verify-cli` passes its current smoke checks.
- `--local` daemon-launch mode has been removed.
- A shared backend interface exists, with local and remote implementations.
- Default local startup initializes embedded runtime and resolves a current
  directory Workspace.
- The local EventHub backend now publishes CLI command events and receives
  command result events by correlation ID for backend operations.
- `service/cli`, `service/tui`, and `biz/agent` no longer import
  `biz/client`; HTTP is isolated behind `biz/backend/remote.go`.
- Agent execution now requires the shared backend and no longer contains direct
  `appservice.NewRuntimeSessionFromInitiator` or ticker polling paths.
- Running `workorch` with no subcommand now opens the TUI agent surface.
- TUI uses backend event subscriptions, has an agent input area, can submit
  goals, can handle decision/approval input, and monitors Workspaces.
- The startup current-directory Workspace ID is passed into TUI and agent
  commands as the default active Workspace.
- TUI package tests cover smoke rendering, goal submission, and backend event
  completion callbacks.

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
- `partial`: code exists for part of the requirement, but one or more hard
  acceptance conditions are still unmet.
- `implemented`: code has landed, but verification is not complete.
- `verified`: acceptance checks listed in this document have passed.
- `deferred`: intentionally postponed with reason recorded in this document.

Each task must record updated status, changed code locations, and verification
commands before it can move to `verified`.

## Implementation Review 2026-05-03

Review scope:

- `application/workorch`
- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/biz/agent`
- `internal/modules/application/workorch/service/cli`
- `internal/modules/application/workorch/service/tui`
- `scripts/verify-cli-architecture.sh`

Verification run:

```bash
./scripts/verify-cli-architecture.sh
make verify-cli
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/... --count 1
GOCACHE=/tmp/workorch-go-build go test ./internal/pkg/events ./internal/modules/application/workorch/... --count 1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui --count 1
GOCACHE=/tmp/workorch-go-build bash scripts/verify-tui-agent.sh --static-only
```

Observed result:

- `./scripts/verify-cli-architecture.sh` passes and now treats service/cli HTTP
  imports, agent direct runtime sessions, agent ticker polling, and legacy REPL
  entrypoints as hard failures.
- `make verify-cli` passes the existing smoke suite.
- `make cli-test` fails in the current sandbox because the default Go build
  cache points at a read-only path; the same target passes with
  `GOCACHE=/tmp/workorch-go-build`.
- `GOCACHE=/tmp/workorch-go-build go test
  ./internal/modules/application/workorch/... --count 1` passes.
- `GOCACHE=/tmp/workorch-go-build go test
  ./internal/modules/application/workorch/service/tui --count 1` passes.
- `GOCACHE=/tmp/workorch-go-build make verify-tui-agent` passes.
- `GOCACHE=/tmp/workorch-go-build go test ./internal/pkg/events
  ./internal/modules/application/workorch/... --count 1` passes.
- `GOCACHE=/tmp/workorch-go-build bash scripts/verify-tui-agent.sh
  --static-only` passes. The script name is misleading: even in
  `--static-only` mode it still runs TUI, backend, and agent Go tests.
- `scripts/verify-tui-agent.sh` scripted E2E mode was not accepted as evidence
  for the required TUI agent experience because the current TUI is not yet
  agent-first.

High-level compliance:

- A shared `backend.Backend` interface now exists.
- A remote backend exists and isolates HTTP in `biz/backend/remote.go`.
- `--local` daemon-launcher mode and `biz/local` have been removed.
- TUI no longer imports `biz/client` and no longer uses fixed tick polling.
- Default local startup now creates or finds a Workspace for the current
  working directory.
- Local backend operations now execute through EventHub CLI command/result
  contracts; dispatcher internals still call existing appservice/repository
  capabilities as the shared runtime implementation.
- Agent no longer contains legacy direct appservice paths or ticker polling.
- The default no-subcommand path is the TUI agent session.
- TUI is now agent-oriented, but Result/Artifact display and richer provider
  stream rendering remain incomplete.
- Many local and remote backend methods are still repository-level shortcuts,
  no-op stubs, or incomplete coverage.
- A CLI command dispatcher exists for the embedded EventHub backend.
- CLI command/result envelope helpers exist in `internal/pkg/events`.
- TUI model tests and EventHub backend command/result tests exist and pass.

### CLI-AGENT-01. Define Shared CLI Backend Boundary

Status: partial

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

Review 2026-05-03:

- `internal/modules/application/workorch/biz/backend/backend.go` defines the
  shared interface and local/remote implementations exist.
- TUI, agent, and Cobra commands use the backend interface and no longer import
  `biz/client` directly.
- `agent [goal]` uses `agent.NewWithBackend()` and the startup default
  Workspace instead of legacy runtime session helpers.
- Backend coverage is incomplete: Result/Artifact and several management
  operations are absent or stubbed.

### CLI-AGENT-02. Remove `--local` Daemon Launcher Mode

Status: verified

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

Review 2026-05-03:

- `--local` flag is no longer registered in `root.go`.
- `internal/modules/application/workorch/biz/local` is absent.
- `./scripts/verify-cli-architecture.sh` passes the `biz/local` guard.
- `make verify-cli` passes.

### CLI-AGENT-03. Implement Local EventHub Backend

Status: implemented

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

Review 2026-05-03:

- `backend.NewEventHub` subscribes to workorch event envelopes and registers a
  CLI command dispatcher.
- Local backend operation methods publish `/cli/command` events and wait for
  `/cli/result` events by `correlationId`.
- Dispatcher internals call existing appservice/repository capabilities rather
  than inventing CLI-only business logic.
- `eventhub_test.go` verifies Workspace create goes through CLI command/result
  events.

### CLI-AGENT-04. Define CLI Command Event Contracts

Status: implemented

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

Review 2026-05-03:

- `internal/pkg/events/cli_events.go` defines CLI command/result/progress
  topics, command constants, command payloads, result payloads, and encode/
  decode helpers.
- Command result correlation is exercised by the EventHub backend test.
- Additional version-compatibility tests should still be added before marking
  this task verified.

### CLI-AGENT-05. Refactor Agent Flow To Backend/EventHub

Status: verified

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

Review 2026-05-03:

- `NewWithBackend` and `RunWithBackend` were added.
- Legacy `Run()`, `CreateWorkspace()`, direct appservice session creation, and
  ticker-based polling have been removed from `biz/agent`.
- `agent [goal]` and TUI flows use the shared backend.
- Agent terminal state, decisions, approvals, progress, and results are driven
  by backend event callbacks.
- Agent tests now use a fake backend and event delivery.

### CLI-AGENT-06. Rebuild TUI As Primary Agent Interface

Status: partial

Primary locations:

- `internal/modules/application/workorch/service/tui`

Related locations:

- `internal/modules/application/workorch/service/cli/repl.go`
- `internal/modules/application/workorch/service/cli/status.go`

Requirements:

- Replace the dashboard-first TUI with an agent-first TUI.
- Provide an input area for user goals/messages.
- Show compact agent progress, current blocking decisions/approvals, final
  results, and Artifact context in the default view.
- Keep detailed run, step, tool, decision, approval, and provider stream event
  history in backend records; do not render a default TUI debug/event-stream
  view.
- Render the input area near the bottom of the terminal above the help bar.
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

Review 2026-05-03:

- `workorch` with no subcommand now starts the TUI agent session.
- `status` and `repl` also route to the same TUI agent surface.
- TUI now depends on `backend.Backend`, not HTTP client.
- TUI now provides a bottom input area, compact progress summary, simplified
  Workspace monitor, run submission, inline decision/approval handling, and
  event-callback updates.
- The default view no longer renders raw `run:`, `step:`, `goal:`, `answer:`,
  or `Ready.` event-history lines.
- Result summary and concise Artifact context display are implemented for
  result event payloads.
- `docs/workorch-tui-display-closure-tracking.md` records this display closure
  scope and verification.

### CLI-AGENT-07. Remove TUI HTTP Fetch/Polling Model

Status: partial

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

Review 2026-05-03:

- TUI no longer imports `biz/client`.
- `pollInterval`, `PollTick`, `tea.Tick`, and fixed refresh loops are absent.
- TUI subscribes to backend events and processes callback messages.
- TUI tests cover initial model, quit, workspace list update, error display,
  non-empty rendering, goal submission, run completion callback, stale event
  ignore, Workspace switching callback, bottom input, decision input context,
  result-first rendering, error deduplication, and Artifact display.
- ANSI/CJK display-width handling is now covered by display-width truncation in
  the TUI model; screenshot-level diff coverage is still deferred to future E2E
  improvements.

### CLI-AGENT-08. Default Current Directory Workspace Binding

Status: implemented

Primary locations:

- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/service/cli/root.go`
- `internal/modules/application/workorch/service/tui`

Requirements:

- In local mode, resolve the shell current working directory at CLI startup.
- Prefer an existing Workspace bound to that working directory.
- If the current directory is unbound, reuse the last active Workspace recorded
  by the CLI config.
- Create a current-directory Workspace only when no reusable Workspace is
  available.
- Use the selected Workspace as the initial active Workspace for TUI and agent
  runs.
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
- Starting `workorch` twice in the same directory does not create duplicate
  Workspaces.
- Current directory binding wins over the last active Workspace fallback.

Review 2026-05-03:

- `application/workorch/main.go` resolves `os.Getwd()` and now selects a
  default Workspace by current directory binding first, CLI last-active
  fallback second, and create-only-when-needed third.
- `ExecuteWithBackend` stores `defaultWorkspaceID`.
- The default no-subcommand path now passes `defaultWorkspaceID` into the TUI
  model.
- `agent [goal]` uses `defaultWorkspaceID` when present.
- TUI selects the startup Workspace as the active Workspace when the Workspace
  list arrives, and TUI Workspace switching records the last active Workspace
  in CLI config.
- `application/workorch/main_test.go` covers current directory binding
  priority, last active fallback, and create-only-when-needed behavior.

### CLI-AGENT-09. Complete Command-Line Coverage For workorchd Interactions

Status: partial

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

Review 2026-05-03:

- The backend interface covers more domains than the old CLI client path.
- Several local backend methods are no-op or incomplete, including MCP
  create/update, Memory create/list, Template create/update/delete, and Config
  get.
- Several remote backend methods are stubs returning empty data or the input
  object.
- Cobra command packages no longer contain direct `biz/client` imports.
- A command coverage table was not found.
- `events stream` no longer uses HTTP, but event history listing is not yet
  exposed by the shared backend.

### CLI-AGENT-10. Remote Mode Isolation

Status: verified

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

Review 2026-05-03:

- `backend.NewRemote` exists and isolates HTTP client usage in
  `biz/backend/remote.go`.
- `root.go` no longer imports `biz/client` or keeps a direct `apiClient`.
- `service/cli` has no direct HTTP client imports.
- Architecture guard treats any service/cli HTTP client import as a hard
  failure.
- `mustClient()` and stale `--local` guidance have been removed.

### CLI-AGENT-11. Verification And Regression Baseline

Status: partial

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

Review 2026-05-03:

- `scripts/verify-cli-architecture.sh` exists and passes.
- The guard now fails on service/cli HTTP client imports, TUI polling, agent
  runtime-session creation, agent polling, `biz/local`, and legacy REPL
  entrypoints.
- `make verify-cli` passes.
- `make cli-test` fails in this sandbox due default Go build cache path being
  read-only; `GOCACHE=/tmp/workorch-go-build make cli-test` passes.
- `GOCACHE=/tmp/workorch-go-build go test
  ./internal/modules/application/workorch/... --count 1` passes.
- `GOCACHE=/tmp/workorch-go-build bash scripts/verify-tui-agent.sh
  --static-only` passes. Scripted PTY E2E coverage still needs to be expanded
  for Artifact/provider-stream assertions.

### CLI-AGENT-12. Implement Local Runtime Lifecycle

Status: partial

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

Review 2026-05-03:

- `application/workorch/main.go` initializes embedded runtime in default local
  mode and skips it for explicit `--remote`.
- It still directly creates an appservice runtime session with
  `appservice.NewRuntimeSessionFromInitiator`.
- It uses `backend.NewEventHub(session, exports.Infrastructure.EventHub)` for
  local command/result and subscription paths.
- Subscription cleanup and shutdown idempotency tests were not found.

### CLI-AGENT-13. Implement EventHub Command Dispatcher

Status: implemented

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

Review 2026-05-03:

- The embedded EventHub backend registers a dispatcher for `/cli/command`.
- Dispatcher publishes `/cli/result` with matching `correlationId` and
  structured success/error fields.
- Dispatcher is currently owned by the backend construction path rather than a
  separate runtime initiator, so lifecycle placement should still be revisited
  before marking verified.

### CLI-AGENT-14. Define Detailed TUI Interaction Contract

Status: partial

Primary locations:

- `docs/workorch-cli-tui-agent-requirements.md`
- `internal/modules/application/workorch/service/tui`

Requirements:

- Define the first-screen layout for the agent session.
- Define the main input area behavior for goals, follow-up messages, and
  decision answers.
- Define the compact progress summary for agent progress, runtime steps,
  decisions, approvals, and terminal states.
- Define that detailed run/step/tool/provider event history remains backend
  recorded and is not shown as a default TUI event stream.
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

Review 2026-05-03:

- `docs/workorch-tui-interaction-contract.md` exists and defines first-screen
  layout, bottom input behavior, compact progress summary, inline blocking
  handling, keyboard actions, interrupt semantics, and rendering notes.
- `docs/workorch-tui-agent-acceptance.md` exists and lists acceptance layers
  and E2E scenarios.
- `scripts/verify-tui-agent.sh --static-only` and current TUI smoke tests pass
  with a writable `GOCACHE`.
- The interaction contract has been refreshed to the result-first default TUI
  display and is implemented for the current closure scope.

### CLI-AGENT-15. Static Architecture Guards

Status: verified

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

Review 2026-05-03:

- `scripts/verify-cli-architecture.sh` exists and passes.
- It hard-fails forbidden service/cli HTTP imports, `biz/local`, TUI polling,
  agent direct runtime sessions, agent fixed-interval polling, and legacy REPL
  entrypoints.
- It is included in `make cli-test`.

## Current TUI Defect List

These defects are tracked here until `CLI-AGENT-06` and `CLI-AGENT-07` replace
the current TUI model.

Resolved since initial review:

- TUI no longer imports `biz/client` directly.
- TUI no longer has `pollInterval`, `PollTick`, or `tea.Tick` based refresh.
- `workorch` with no subcommand now opens the TUI agent surface.
- TUI now exposes a bottom input area, can submit goals, can resolve decisions
  and approvals inline, and subscribes to backend/EventHub events.
- TUI default display is result-first and no longer shows raw event stream
  noise in the main view.
- TUI package tests cover goal submission, backend run-completion callback,
  stale events, Workspace switching, blocking item input context, error
  deduplication, and Artifact context.

Remaining defects after 2026-05-03 implementation review:

- Detailed provider stream rendering is intentionally not part of the default
  TUI; provider details remain backend records.
- Full terminal screenshot diff coverage is still deferred.

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
