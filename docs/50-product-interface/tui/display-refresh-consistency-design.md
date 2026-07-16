# WorkOrch TUI Display Refresh Consistency Design

Status: active

Type: product-interface

Last Updated: 2026-06-09

## Purpose

本文件承接 TUI 刷新一致性和显示层设计。

## Stable Display Rules

### Projection Contract

TUI 必须使用与 backend/Web 一致的状态源语义：

- workspace list 来自 workorchd `WorkspaceOverview` projection
- 当前 workspace detail 来自 workorchd `WorkspaceDetail` projection
- 当前 workspace state 来自 projection 中的 workspace state
- 当前 active run 来自 projection 中的 active run 字段
- blocker、run status、result/outcome 都必须以 active workspace + active run 为准
- superseded / failed / completed 的旧 run 只能作为历史摘要，不得驱动当前状态栏、prompt mode 或 active blocker
- TUI 不自行组合 run/status/goal/dod/decision/approval/result 状态

### Main Viewport

默认主视图应保留：

- active workspace 与 template/soul binding 摘要
- Goal / conversation / 当前 DoD
- compact task decomposition、memory recall、LLM usage/context、runtime/governance sections
- Result、artifacts、sources、patches
- last error

主视图必须支持键盘 `PgUp/PgDn` 滚动；workspace panel 在宽终端下保持固定。

### Runtime Status Bar

状态栏位于输入区上方，至少应包含：

- progress label / progress text
- active workspace short id
- active run short id
- 当前 blocker id（若存在）
- 用户手动滚离尾部时的 new update marker

### Input Area

- 输入区固定在底部
- 默认 prompt 保持中立：`ws-xxx >`
- 运行状态、progress、blocker、available actions 不应堆到 prompt suffix，而应进入状态栏和主视图
- 只有 decision / approval blocker 会改变 prompt context

### Refresh Rules

- 本地 TUI 刷新必须继续通过 shared backend + EventHub/SSE projection change callbacks
- 不允许恢复 HTTP polling、`time.Tick`、`tea.Tick` 或 fixed-interval refresh
- non-active workspace 事件只能刷新 monitor，不得污染 active session 的主结果区
- workspace switch 后必须读取目标 workspace detail projection，并丢弃旧 workspace
  scope 的异步响应
- command 返回只表示请求已提交，最终显示状态以服务端 projection revision 为准

### Terminal Usability

- 保持 native mouse copy 可用，mouse reporting 默认关闭
- ANSI/CJK 文本截断与布局必须稳定
- 右侧 workspace panel、底部输入与 help 区在常见终端尺寸下应始终可读

## Source Documents

- `../../90-archive/superseded/mvvp-workspace-viewmodel-architecture.md`（历史）
- `../../90-archive/records/tui-display-closure-tracking.md`（历史闭环记录）
