# WorkOrch TUI Agent Acceptance

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件作为 TUI agent 验收入口文档。

## Acceptance Scope

验收目标是默认本地 `workorch` TUI 模式。

In scope:

- `workorch` 无子命令启动 TUI agent session
- 本地模式复用嵌入式 runtime
- 本地模式通过 EventHub command/event/callback 交互
- TUI 状态变化由 EventHub callback 驱动，而不是 polling
- 当前 shell 目录是默认 Workspace working directory
- TUI 可以发起 run、观察进度、处理 Decision/Approval、切换 Workspace，并展示终态 Result/Artifact context
- 默认 TUI 是 result-first，不展示详细 runtime event stream

Out of scope:

- Web Console 行为
- `workorchd` HTTP 服务验收
- 并发/容量压测
- 远程 TUI transport 的深度实现细节

## Hard Acceptance Rules

- 本地 TUI 不得 import `internal/pkg/workorchclient`
- 本地 TUI 不得使用固定轮询刷新状态
- 本地 TUI 不得启动或依赖 `workorchd` 子进程
- 本地 TUI 不得依赖本机 HTTP listener
- agent 交互不得绕过共享 backend contract
- 默认 TUI 不得渲染 `run:`、`step:`、`goal:`、`answer:` 这类 raw 内部事件行
- 输入区必须位于底部且在 help bar 上方

## Verification Layers

### Layer 1. Static Architecture Guards

验证：

- 无 `biz/client` 直连
- 无 `time.Tick` / `tea.Tick` / 固定 polling
- 无 `biz/local`
- agent 无直接 runtime session 捷径

### Layer 2. TUI Model Tests

必须覆盖：

- 初始模型状态
- 当前目录 Workspace 绑定/复用
- Goal 提交
- Workspace 切换
- compact progress 更新
- Decision / Approval inline 处理
- terminal completed / failed result
- stale event ignore
- async error display

### Layer 3. Backend / EventHub Integration Tests

必须覆盖：

- Workspace ensure/create
- Run activation
- runtime progress 回调
- Decision resolve
- Approval approve/reject
- Run resume
- concurrent command correlation
- subscription cleanup

### Layer 4. Scripted TUI E2E

必须证明：

- 默认启动进入 agent TUI
- 提交 Goal 会发起 run
- callbacks 驱动进度/阻塞/终态变化
- 可 inline 处理 Decision/Approval
- completed / failed 终态可见

## Core Scenarios

- `TUI-E2E-01` 默认启动进入 agent TUI
- `TUI-E2E-02` 当前目录 Workspace 绑定
- `TUI-E2E-03` 提交 Goal 启动 Run
- `TUI-E2E-04` 通过 callback 更新运行进度
- `TUI-E2E-05` Decision Ticket inline resolve
- `TUI-E2E-06` Approval inline approve/reject
- `TUI-E2E-07` Workspace switch
- `TUI-E2E-08` terminal Result / Artifact display
- `TUI-E2E-09` failure display
- `TUI-E2E-10` interrupt and cleanup
- `TUI-E2E-11` remote mode isolation
- `TUI-E2E-12` keyboard viewport scroll and native mouse copy
- `TUI-E2E-13` workspace list live refresh

## Scenario Intent Mapping

为避免把 TUI 验收误读成“功能点罗列”，当前关键场景还应映射到产品边界：

| 场景 | 主要验证边界 |
| --- | --- |
| `TUI-E2E-05` Decision Ticket inline resolve | blocker prompt 只服务当前 open ticket；提交后回到中立 prompt |
| `TUI-E2E-06` Approval inline approve/reject | approve/reject 是显式动作，不依赖自由文本推断 |
| `TUI-E2E-07` Workspace switch | active workspace 切换后，主结果区与 blocker 上下文必须跟随切换 |
| `TUI-E2E-08` terminal Result / Artifact display | 默认为 result-first review，不进入 event-stream-first |
| `TUI-E2E-11` remote mode isolation | local/remote transport 差异不改变业务对象语义 |
| `TUI-E2E-13` workspace list live refresh | non-active workspace 更新只刷新 monitor，不污染 active 主视图 |

## Review / History Boundary

TUI acceptance 当前还应明确验证以下边界：

- review boundary
  - 默认主视图只验证 accepted result、artifact context、blocker/failure、compact progress
- history boundary
  - backend records 中的详细 step/tool/provider/event history 不作为默认 TUI 主阅读面
- long-output boundary
  - 长结果通过主结果区滚动阅读，而不是切换成详细 debug stream

这些边界主要由：

- `TUI-E2E-08`
- `TUI-E2E-09`
- `TUI-E2E-12`
- `display-closure-tracking.md`

共同证明。

## Verification Entry

```bash
make cli-test
make verify-cli
make verify-tui-agent
go test ./internal/services/workorch/... --count 1
```

## Source Documents

- `../../50-product-interface/tui/requirements.md`
- `display-closure-tracking.md`
