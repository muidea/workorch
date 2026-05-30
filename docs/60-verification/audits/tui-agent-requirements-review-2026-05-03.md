# WorkOrch TUI Agent Requirements Review 2026-05-03

Status: active

Type: audit

Last Updated: 2026-05-29

## Purpose

本文件承接 `2026-05-03` 针对 CLI/TUI agent 要求的实现审查、任务状态和残余缺陷记录。

## Review Scope

- `application/workorch`
- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/biz/agent`
- `internal/modules/application/workorch/service/cli`
- `internal/modules/application/workorch/service/tui`
- `scripts/verify-cli-architecture.sh`

## Review Result Summary

已确认：

- shared `backend.Backend` 已存在
- remote backend 已隔离 HTTP
- `--local` daemon-launch 模式已移除
- TUI 不再 import `biz/client`
- 默认无子命令路径已进入 TUI agent surface
- TUI 已具备 bottom input、goal submission、inline decision/approval、EventHub callback 驱动更新

仍未完全收口：

- backend 命令覆盖仍不完整
- 若干 local/remote backend 方法仍为 stub/no-op 或覆盖不足
- local runtime lifecycle 与 dispatcher 生命周期仍需进一步清晰化
- scripted PTY E2E 对 artifact/provider-stream 断言仍不完整

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
