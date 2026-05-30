# WorkOrch TUI Interaction Contract

Status: active

Type: product-interface

Last Updated: 2026-05-30

## Purpose

本文件承接 TUI 交互 contract。

## Stable Interaction Rules

### First-Screen Layout

默认布局至少包括：

- top bar：title + active workspace title/path
- progress summary：idle / running / waiting / completed / failed
- main result area：final result 与 artifact context
- right panel：bounded all-workspace monitor
- bottom input area：slash command / decision / approval input
- bottom help bar：keyboard shortcut reference

### Input Contract

- 正常状态 prompt 使用中立 `ws-xxx >`
- 默认输入采用 slash commands，例如 `/goal`、`/run continue`、`/run restart`、`/dod`、`/memory`、`/result`
- 普通状态下裸文本不应被隐式当作 goal
- decision blocker 时 prompt 进入 `decision` 上下文，plain text 作为 decision answer
- approval blocker 时 prompt 进入 `approval` 上下文，plain text 作为 approve/reject intent，歧义时要求显式 slash command

### Keyboard Contract

- `Enter`：提交
- `Alt+Enter`：插入换行
- `Ctrl+P / Ctrl+N`：本地输入历史
- `Tab`：slash command completion
- `Ctrl+C`：中断 active run
- `Esc`：清空当前输入

### Default Slash Commands

- `/help`
- `/clear`
- `/workspace`
- `/workspace create`
- `/goal`
- `/dod`
- `/result`
- `/memory`
- `/run continue`
- `/run restart`
- `/decision show` / `/decision list`
- `/decision answer`
- `/approval approve` / `/approval reject`
- `/exit`

### Workspace Contract

- startup active workspace 优先当前目录绑定，其次 last active，最后才创建/绑定新 workspace
- workspace switch 必须影响后续 goal/run 命令
- TUI 不提供常规 workspaceTemplate rebind flow；template 选择是 workspace 创建期能力

### Workspace Monitor Contract

- 右侧 all-workspace monitor 只显示 bounded summary，不展开为第二主视图
- 非 active workspace 至少显示：
  - workspace title / short id
  - active run short state
  - blocker badge
  - 最新 completed/failed 摘要
- monitor 选择某个 workspace 时，必须显式切换 active workspace，不能只在局部面板中悬挂“临时焦点”
- monitor 刷新只消费 summary/read-model，不直接渲染 raw event payload

### Low-Frequency Interaction Contract

低频交互当前应保持以下边界：

- workspace create
  - 只负责创建期 title/path/template 输入，不承接 template rebind
- decision / approval detail
  - 默认展示当前 open blocker；历史项通过显式 `/decision list`、`/approval ...` 打开
- result / artifact deep view
  - 默认展示 compact summary；完整 artifact/path/raw payload 只在显式命令中展开
- memory / context inspect
  - 默认查看当前 active workspace / run 的 compact context，不把 recall trace 全量堆回主屏
- long output review
  - 仍停留在主结果区滚动阅读，不切回 event-stream-first 调试模式

### Historical Interaction Examples

以下示例用于固化当前 TUI 的历史交互边界，避免再次滑回 event-stream-first
或 CLI-only 语义。

#### Example 1. Workspace Switch

```text
ws-a1b2 > /workspace
  -> 选择 ws-c3d4
ws-c3d4 >
```

约束：

- 切换后 prompt、主结果区、blocker 上下文都必须切到新 active workspace
- 原 workspace 的后续事件只允许刷新右侧 monitor，不得回写主结果区

#### Example 2. Decision Inline Resolve

```text
decision: ticket-001 > /decision answer
补充信息：目标城市=杭州，输出格式=markdown
```

约束：

- decision prompt 只在当前 blocking ticket 存在时出现
- 提交后 prompt 必须回到中立 workspace prompt，而不是保留 `decision:` 前缀
- 用户的自由文本回答应进入 resolution payload，不应原样堆成长期历史日志

#### Example 3. Approval Confirm

```text
approval: approval-009 > /approval approve
```

约束：

- approve/reject 是对当前 pending approval 的显式动作，不是普通聊天输入推断
- 审批完成后主视图应继续展示当前 run 的 compact progress 或 terminal result

#### Example 4. Result / Artifact Review

```text
ws-a1b2 > /result
```

约束：

- 默认先展示 accepted result summary、artifact index、failure/risk 摘要
- 若正文很长，继续通过主结果区滚动阅读，而不是切到独立 debug stream 面板
- artifact/raw payload 只在显式查看时展开

### Review Boundary

TUI 中的 “review” 当前应只表达对正式业务结果和当前阻塞面的阅读与确认，不扩展为新的调试产品面。

- review 可以包括：
  - accepted result summary
  - artifact index / compact context
  - current blocker / latest failure reason
  - compact task progress / recovery suggestion
- review 不应包括：
  - 默认全量 step/tool/provider/event stream 回放
  - 第二套独立的 raw JSON 诊断面板
  - 在 TUI 内重建 Web Console 式治理工作台
- 若需要更深诊断，应继续通过 backend records、Web Console 或专门 verifier，而不是把默认 TUI 变成综合 debug 工具

### Local Vs Remote Interaction Boundary

- local 模式下，slash command 和 blocker resolution 直接走 shared backend / EventHub
- remote 模式下，slash command 和 blocker resolution 走共享 backend 抽象后的 remote adapter
- 两种模式都必须呈现同一组 workspace/run/decision/approval/result 语义
- 远程模式可以存在更高的读写延迟，但不能因此回退到不同的 prompt contract 或 blocker contract
- 本地与远程模式的差异应体现在 transport / latency / availability 提示，而不是体现在业务字段或对象模型

### Blocking Contract

- Decision / Approval 以当前 blocking state 呈现，而不是历史事件流
- resolution callback 后应清空当前 blocking prompt 并回到正常 progress
- 用户答案不应作为永久可见历史噪音堆积在主视图

## Source Documents

- `requirements.md`
