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
  `GOCACHE=/tmp/workorch-go-build`, the workorch CLI packages pass.
- `make cli-build` builds `./bin/workorch`.
- `make verify-cli` passes its current smoke checks.
- `--local` daemon-launch mode has been removed.
- A shared backend interface exists, with local and remote implementations.
- Default local startup initializes embedded runtime and resolves a current
  directory Workspace.
- Local backend methods still call appservice/repositories directly instead of
  publishing CLI command events.
- Some CLI command files still import `biz/client`.
- Agent has a backend path but still keeps direct appservice legacy paths and a
  ticker-based `pollRun()` fallback.
- TUI uses backend types and has no fixed polling loop, but it is still a
  status dashboard rather than the primary agent interaction surface.

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
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/... --count 1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui --count 1
bash scripts/verify-tui-agent.sh --static-only
```

Observed result:

- `./scripts/verify-cli-architecture.sh` passes, but it still treats some
  forbidden legacy paths as warnings or transition allowances.
- `make verify-cli` passes the existing smoke suite.
- `make cli-test` fails in the current sandbox because the default Go build
  cache points at a read-only path.
- Re-running the same package tests with `GOCACHE=/tmp/workorch-go-build`
  passes only before considering the currently untracked TUI test files.
- Untracked TUI acceptance artifacts exist:
  `internal/modules/application/workorch/service/tui/model_test.go`,
  `internal/modules/application/workorch/biz/backend/fake_test.go`, and
  `scripts/verify-tui-agent.sh`.
- With those untracked files present, `go test
  ./internal/modules/application/workorch/service/tui --count 1` fails to
  compile because `model_test.go` reuses `:=` with no new variables and tries to
  type-assert a concrete `AppModel`.
- `bash scripts/verify-tui-agent.sh --static-only` fails because the TUI model
  tests fail.

High-level compliance:

- A shared `backend.Backend` interface now exists.
- A remote backend exists and isolates HTTP in `biz/backend/remote.go`.
- `--local` daemon-launcher mode and `biz/local` have been removed.
- TUI no longer imports `biz/client` and no longer uses fixed tick polling.
- Default local startup now creates or finds a Workspace for the current
  working directory.
- The implementation is not yet fully compliant with the design because local
  commands still execute direct appservice/repository calls rather than
  EventHub command/result contracts.
- Agent still contains a legacy direct appservice path and still falls back to
  `pollRun()` with a `time.NewTicker(500ms)` even when a backend is present.
- TUI remains a status/workspace dashboard, not the primary agent interaction
  surface defined in the requirements.
- Many local and remote backend methods are still repository-level shortcuts,
  no-op stubs, or incomplete coverage.
- There is no runtime-side EventHub command dispatcher for CLI command topics.
- No command event envelope/result envelope implementation was found.
- TUI/model and backend/eventhub focused tests are still missing.
- A TUI model test file has been drafted but is not yet passing and is not yet
  suitable as acceptance evidence.

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
- TUI uses the backend interface.
- Agent has a backend path, but still keeps legacy direct appservice paths.
- Some Cobra command packages still import `biz/client`; migration is not
  complete.
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

Status: partial

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

- `backend.NewEventHub` exists and subscribes to existing workorch event
  envelopes.
- Local operations still execute by direct `appservice.*WithSession` calls or
  direct repository access, not by publishing CLI command events and waiting for
  command result events.
- There is no command `correlationId` result path.
- `eventhub.go` handles progress/state subscription only; it is not a full
  command backend.

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

Review 2026-05-03:

- No CLI command envelope/result envelope contract implementation was found.
- The current EventHub path adapts existing business events only.
- This remains a prerequisite for full local-mode compliance.

### CLI-AGENT-05. Refactor Agent Flow To Backend/EventHub

Status: partial

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
- Agent still contains `Run()` and `CreateWorkspace()` legacy paths that call
  `appservice.NewRuntimeSessionFromInitiator`.
- `RunWithBackend` still calls `pollRun()`, and `pollRun()` uses
  `time.NewTicker(500 * time.Millisecond)` even when a backend is present.
- Existing tests are still appservice/session oriented, not fake-backend event
  tests.

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

Review 2026-05-03:

- TUI now depends on `backend.Backend`, not HTTP client.
- Current TUI is still a Workspace/status dashboard with list/detail views.
- It does not provide an agent input area, event stream, inline
  decision/approval handling, run submission, or Result/Artifact display.
- `docs/workorch-tui-interaction-contract.md` defines the intended interaction,
  but implementation has not reached it.

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
- `pollInterval`, `PollTick`, and `tea.Tick` were removed.
- TUI still performs an initial `FetchAll` pull via backend and does not yet
  subscribe to backend events.
- A TUI test file exists as an untracked draft, but it currently fails to
  compile and does not cover callback updates, stale events, nil workspace
  rendering, or wide-character truncation.

### CLI-AGENT-08. Default Current Directory Workspace Binding

Status: implemented

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

Review 2026-05-03:

- `application/workorch/main.go` resolves `os.Getwd()` and finds or creates a
  default Workspace with that working directory.
- `ExecuteWithBackend` stores `defaultWorkspaceID`.
- Verification tests for this behavior were not found, so this is not yet
  `verified`.

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
- A command coverage table was not found.
- CLI command packages still contain direct `biz/client` imports in some files.

### CLI-AGENT-10. Remote Mode Isolation

Status: partial

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
- `root.go` still imports `biz/client`, keeps `apiClient`, and sets it when
  `--remote` is present.
- Some CLI command files still import `biz/client`, so remote transport is not
  fully hidden behind the shared backend.
- Architecture guard currently allows `service/cli` HTTP imports as a
  transition exception.

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
- The guard still treats some known gaps as `WARN` or transition allowances,
  so it does not enforce all hard acceptance rules.
- `make verify-cli` passes.
- `make cli-test` fails in this sandbox due default Go build cache path being
  read-only; `GOCACHE=/tmp/workorch-go-build go test
  ./internal/modules/application/workorch/... --count 1` passes.
- An untracked `scripts/verify-tui-agent.sh` exists, but
  `bash scripts/verify-tui-agent.sh --static-only` currently fails because the
  untracked TUI model tests do not compile.

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
- Subscription cleanup and shutdown idempotency tests were not found.

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

Review 2026-05-03:

- No runtime-side CLI command dispatcher was found.
- No command event topics or command result events are wired.
- Current local backend bypasses this requirement by calling appservice and
  repositories directly.

### CLI-AGENT-14. Define Detailed TUI Interaction Contract

Status: partial

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

Review 2026-05-03:

- `docs/workorch-tui-interaction-contract.md` exists and defines first-screen
  layout, input behavior, event stream display, inline blocking handling,
  keyboard actions, interrupt semantics, and rendering notes.
- A TUI acceptance script and model tests exist as untracked files, but the TUI
  test package currently fails to compile, so the contract is not yet accepted
  or verified.

### CLI-AGENT-15. Static Architecture Guards

Status: partial

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
- The script still prints warnings for some unmet requirements and allows
  `service/cli` direct HTTP client imports during transition.
- It permits agent direct session initialization as a legacy path, so it does
  not enforce the final hard rules yet.

## Current TUI Defect List

These defects are tracked here until `CLI-AGENT-06` and `CLI-AGENT-07` replace
the current TUI model.

Resolved since initial review:

- TUI no longer imports `biz/client` directly.
- TUI no longer has `pollInterval`, `PollTick`, or `tea.Tick` based refresh.

Remaining defects after 2026-05-03 implementation review:

- TUI is still dashboard/status oriented, not agent-session oriented.
- TUI does not expose an input area for goals or follow-up messages.
- TUI cannot start runs.
- TUI cannot resolve decisions or approvals inline.
- TUI does not subscribe to backend/EventHub events for live state updates.
- TUI does not show runtime event stream, provider stream output, Result, or
  Artifact context.
- TUI has no package tests for callback updates, stale event handling,
  Workspace switching, blocking item handling, terminal result display, or
  interrupt behavior.
- The current untracked TUI model test draft does not compile and cannot be
  used as acceptance evidence yet.
- `modeRunsList` and `runs` remain effectively unused.
- Key constants in `keys.go` are not consistently used by `model.go`.
- Byte/display-width safe truncation and ANSI-aware alignment still need
  verification.

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
