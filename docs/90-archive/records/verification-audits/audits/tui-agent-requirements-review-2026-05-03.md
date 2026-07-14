# WorkOrch TUI Agent Requirements Review 2026-05-03

Status: superseded-with-closure-record

Type: audit

Last Updated: 2026-06-22

## Purpose

本文件承接 `2026-05-03` 针对 CLI/TUI agent 要求的实现审查、任务状态和残余缺陷记录。

Current tracking note: this document is a historical review record. Current
TUI/MVVP and projection-only behavior is tracked by the Batch 1/B7 runtime
contract audit documents and the active TUI tests. Do not use the older
`partial` task labels below as the active code backlog without first checking
the current code and tests.

## Review Scope

- `application/workorch`
- `internal/services/workorch/biz/backends`
- `internal/services/workorch/biz/agent`
- `internal/services/workorch/service/cli`
- `internal/services/workorch/service/tui`
- `scripts/verify-cli-architecture.sh`

## Review Result Summary

已确认：

- shared `backend.Backend` 已存在
- remote backend 已隔离 HTTP
- `--local` daemon-launch 模式已移除
- TUI 不再 import `biz/client`
- 默认无子命令路径已进入 TUI agent surface
- TUI 已具备 bottom input、goal submission、inline decision/approval、EventHub callback 驱动更新

历史残余项，当时仍未完全收口：

- backend 命令覆盖仍不完整
- 若干 local/remote backend 方法仍为 stub/no-op 或覆盖不足
- local runtime lifecycle 与 dispatcher 生命周期仍需进一步清晰化
- scripted PTY E2E 对 artifact/provider-stream 断言仍不完整

## 2026-06-22 Boundary Closure Record

- Closed issue: TUI production code directly imported kernel decision/governance
  common packages for decision resolution constants and approval/decision
  semantic normalization, and imported LLM block common only to normalize
  runtime block kind display. TUI and agent code also read port event payloads
  through `internal/pkg/events` constants instead of the port contract.
- Boundary expectation: CLI/TUI/agent code should consume WorkOrch through
  `internal/pkg/workorchport` and backend interfaces, not kernel owner packages.
- Implementation:
  - Added decision resolution constants and neutral approval/decision semantic
    normalization helpers to `internal/pkg/workorchport/governance.go`.
  - Added `internal/pkg/workorchport/runtime_block.go` for client-facing
    runtime block kind constants and canonicalization.
  - Added `internal/pkg/workorchport/event_payload_keys.go` for client-facing
    backend event payload keys used by TUI and agent event handling.
  - Updated TUI command/model code to use `workorchport` for decision replan /
    reject resolution, approval semantic parsing, and runtime block kind
    display normalization.
  - Updated TUI/agent event payload parsing to use `workorchport` payload keys.
  - Updated TUI tests to assert against `workorchport` constants.
  - Added `internal/pkg/workorchport/governance_test.go` to lock the port-level
    semantic contract used by TUI.
  - Added `internal/pkg/workorchport/runtime_block_test.go` to lock
    client-facing runtime block kind canonicalization.
- Boundary result:
  - `internal/services/workorch/service/tui`, `internal/services/workorch/biz/agent`,
    and `internal/services/workorch/service/cli` no longer import
    `internal/modules/**`, `internal/pkg/events`, REST client, backend
    implementation, repository, framework, or EventHub packages.
  - The change keeps behavior business-neutral: it exposes generic governance
    command semantics at the port boundary and does not add workspace-template,
    Soul, provider, language, or live-workspace branching.
- Verification:
  - `go test ./internal/pkg/workorchport ./internal/services/workorch/service/tui ./internal/services/workorch/biz/backends ./internal/services/workorch/biz/agent ./internal/services/workorch/service/cli -count=1 -timeout=180s`
  - `rg -n "internal/modules/|internal/pkg/workorchclient|biz/backends|internal/pkg/repository|framework/application|EventHub|event\\.Hub|internal/pkg/events" internal/services/workorch/service/tui internal/services/workorch/service/cli internal/services/workorch/biz/agent internal/pkg/workorchport -g '*.go'`
  - `git diff --check` for the touched code and audit documents.

## Task Tracking Snapshot

本次审查涉及的主要任务状态保留为：

- `CLI-AGENT-01` partial
- `CLI-AGENT-02` verified
- `CLI-AGENT-03` implemented
- `CLI-AGENT-04` implemented
- `CLI-AGENT-05` verified
- `CLI-AGENT-06` partial
- `CLI-AGENT-07` partial
- `CLI-AGENT-08` implemented
- `CLI-AGENT-09` partial
- `CLI-AGENT-10` verified
- `CLI-AGENT-11` partial
- `CLI-AGENT-12` partial
- `CLI-AGENT-13` implemented
- `CLI-AGENT-14` partial
- `CLI-AGENT-15` verified

## Residual Risks

- TUI 默认不展示 provider stream detail，这是产品边界而非缺陷，但会影响调试深度
- full terminal screenshot diff coverage 仍未建立
- backend method / command coverage table 仍缺
- 多本地 TUI session 的生命周期边界仍需进一步核定

## Source Documents

- `../../50-product-interface/tui/requirements.md`
