# WorkOrch TUI Requirements

Status: active

Type: product-interface

Last Updated: 2026-05-29

## Purpose

本文件承接 WorkOrch CLI/TUI agent 的产品需求入口。

## Target Product Shape

- `workorch` 的主本地体验是 agent-first TUI，而不是被动状态面板。
- 本地模式直接复用嵌入式 WorkOrch runtime，不启动 `workorchd` 子进程。
- 本地模式不得通过 HTTP client 路径访问 WorkOrch。
- 本地模式必须通过 magicCommon EventHub 的 command/event/callback 交互驱动业务动作、进度、阻塞项和终态结果。
- 本地模式不得通过固定轮询刷新状态。
- 远程模式必须显式，通过 `--remote <addr>` 激活，并隔离在共享 backend 接口之后。
- 当前 shell 目录是默认 Workspace working directory。
- TUI 必须支持 Workspace 切换、所有 Workspace 的实时监控、inline Decision/Approval 处理，以及终态 Result/Artifact 上下文展示。

## Product Requirements

### Local-First Runtime Boundary

- 本地 CLI/TUI 不得启动 `workorchd`
- 本地 CLI/TUI 不得依赖本机 HTTP listener
- 本地 CLI/TUI 不得绕过共享 runtime contract 实现 CLI-only 业务逻辑
- TUI/agent/commands 都必须经由共享 backend 边界工作

### Result-First Default View

- 默认主视图应是 result-first，而不是 event-stream-first
- 详细 run/step/tool/provider 事件历史保留在 backend records，不作为默认 TUI debug/event-stream 视图展示
- 主视图应优先展示终态 Result、失败原因、当前阻塞项或简洁空状态

### Input And Interaction

- 输入区必须固定在底部，位于 help bar 上方
- 输入上下文必须清晰区分普通 Goal、Decision 回复、Approval 回复
- 键盘行为至少需要覆盖 submit、cancel、workspace switch、approve/reject、quit 和运行中断
- 运行中的 `Ctrl-C` 语义必须明确，且不能破坏 runtime/backend 的清理逻辑

### Workspace Model

- 启动时优先绑定当前目录对应的 Workspace
- 若当前目录未绑定，则允许回退到 last active Workspace
- 只有两者都不可用时，才允许创建或绑定新的当前目录 Workspace
- TUI 需要持续显示 active Workspace 和 all-Workspace monitor

### State Update Model

- 所有状态变化必须由 EventHub callback/subscription 驱动
- 不允许 `time.Ticker`、`time.Tick`、`tea.Tick` 或其他固定轮询刷新
- 非 active Workspace 的更新只能刷新 monitor，不应污染当前主会话视图

### Terminal Outcome

- 完成态必须展示最终 Result 摘要
- backend 事件中存在 Artifact 引用时，应展示简洁 Artifact context
- 失败态必须只显示一次清晰 failure reason

## Non-Goals

- 不保留或扩展 `--local` daemon-launching 模式
- 不把当前 TUI 做成详细调试事件流面板
- 不增加第二套 CLI-only 业务实现
- 不让本地模式依赖 HTTP server

## Product Boundary

- TUI 是产品交互面，不负责定义 backend 事件协议、runtime lifecycle 或 verifier 断言细节
- 交互合同细节转入 `interaction-contract.md`
- 刷新/显示收口规则转入 `display-refresh-consistency-design.md`
- 验收矩阵转入 `../../60-verification/tui/tui-agent-acceptance.md`

## Source Documents

- `../../60-verification/tui/tui-agent-acceptance.md`
