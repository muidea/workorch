# Repository Guidelines

## Project Structure & Module Organization

WorkOrch is a Go service with an embedded Web console. Main entry points live in `application/workorch`. Core runtime code is under `internal/modules/kernel` (Runtime V2 authority in `kernel/orchestrator`), reusable packages under `internal/pkg` (especially `workorchruntimecontract`), application adapters under `internal/modules/application/workorchd`, capability/owner blocks under `internal/modules/blocks`, and initiator wiring under `internal/initiators`. Web console source is in `web/workorch-console`; production assets are generated during build. Runtime docs are under `docs/`, with `docs/00-core/runtime-stage-contract.md` as the design contract (neutral stage semantics + Runtime V2 mapping) and module `readme.md` files (especially `kernel/orchestrator/readme.md`) as implementation SoT. Fact entry: `docs/01-overview/current-state.md`. Residual open work only: `docs/70-roadmap/active/overview.md` and `docs/01-overview/current-unclosed-matrix.md` — archived plans under `docs/90-archive/` must not constrain new implementation.

## Build, Test, and Development Commands

- `make deps`: download Go modules and install console dependencies.
- `make build`: build console assets and the `./bin/workorch` binary.
- `make run`: run the service with `config`.
- `make run-file`: run with file-backed storage from `config/file`.
- `make fmt-check`: verify Go formatting.
- `make test`: run `go test ./...`.
- `make vet`: run `go vet ./...`.
- `make verify-boundary-contract`: check runtime/Web business neutrality.
- `make verify-ci-mvp-file`: run CI-safe deployed-instance checks with mock providers.

## Coding Style & Naming Conventions

Use `gofmt` for Go and keep package names short, lowercase, and role-oriented. Prefer existing module boundaries and helper APIs. Web code uses TypeScript, React, Vite, and Ant Design; place page-level code under `web/workorch-console/src`. Do not commit generated runtime data from `.workorch-data`, `.workorch-workspaces`, `bin`, or `node_modules`.

## Testing Guidelines

Add focused Go tests beside changed code using `*_test.go`. For runtime, LLM request assembly, event projection, TUI, and presenter changes, include contract regression tests. Run targeted `go test ./path -count=1` first, then broader `make test` or relevant `make verify-*` targets.

## Commit & Pull Request Guidelines

History uses concise imperative summaries such as `fix runtime incremental artifact convergence` and `docs: reorganize workorch documentation structure`. Keep commits scoped. PRs should describe behavior changes, verification commands, config or migration impact, and screenshots for Web console UI changes.

## Agent-Specific Runtime Constraints

Before changing runtime behavior, read `docs/00-core/runtime-stage-contract.md` and `internal/modules/kernel/orchestrator/readme.md`. For LLM request rendering, context budget, cache planning, provider adapters, memory recall, or evidence packaging, also read `docs/00-core/runtime-request-cache-note.md`.

Production path is **Runtime V2 only** (`DriveRuntimeV2` + reducer + sealed dispatch). Do not reintroduce V1 StagePacket, NextAction, DispatchNextStep, ExecutionContextPatch, auto replan successor, or map-based authority rebuild.

Core runtime must remain functionally neutral. Do not hard-code software development, language stacks, test frameworks, MCP-specific behavior, or business-domain logic into orchestrator core. Put feature behavior in profile, capability, template, or adapter layers. Cross-owner I/O must use EventHub + concrete typed command/result.

Keep the provider-facing LLM cache prefix stable: long-lived system/schema/runtime contracts belong in `C0`, Goal/DoD definitions in `C1`, declarative capability/governance contracts in `C2`, and volatile runtime details in `C3` through `C5`. Treat unexpected `stablePrefixHash` drift as a request rendering bug.
