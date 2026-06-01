# TUI 数据稳定化 + 命令优化 实施计划

> 编制日期：2026-06-01
> 最近核对：2026-06-01
> 范围：Go TUI（Bubbletea）+ Web Console（React）两端同步改进
> 策略：后端驱动增量更新 + 上下文感知命令简化 + 命令体系重构

## 当前收口状态

本轮已完成 Go TUI、后端事件、Web Console 与 CLI 的完整收口：

- `applyWorkspaceContext` 不再在普通上下文刷新时调用 `resetWorkspaceView()`，改为按字段增量更新；完整空 snapshot 仍可清空已不存在的 Goal / DoD。
- TUI event channel 容量从 64 提升到 256，并保留关键 workspace list 事件挤出策略。
- plain text 输入改为上下文感知路由：decision / approval 优先，其次 active run 作为 operator input，idle workspace 作为新 goal。
- 新增 `/status`、`/coverage`、`/steps`、`/timeline`、`/patches`、`/artifacts` inspect 命令与 slash suggestion。
- help overlay 按 Workflow / Inspect / Governance / System 分组展示。
- patch、artifact continuation、memory recall、LLM usage 事件改为 TUI 本地增量更新，不再触发 `FetchWorkspaceContext` 全量刷新。
- backend `Envelope.Revision`、workspace revision counter、SSE `id:` / `Last-Event-ID` replay 已收口，并通过服务端 SSE 测试覆盖。
- SSE `stateSummary` 在序列化阶段富化，不污染内部事件总线。
- Web Console 改为 `useSSE` + topic router 增量消费，revision gap / reconnect 触发全量刷新兜底。
- CLI 已新增 `run coverage <id>` / `run timeline <id>` 子命令。

当前无未收口项。

---

## 一、问题诊断

### 1.1 数据显示不稳定

**Web Console：**

- 每次 SSE 事件触发 `loadWorkspaceContext`，发起 22 个并行 REST 请求全量刷新
  - 第一批 11 个：snapshot + contextBundle + goals + dods + souls + soulBindings + skills + mcpServers + mcpCapabilities + memory + configuration
  - 第二批 11 个：runSnapshot + runObservability + runContextBundle + steps + toolCalls + checkpoints + results + patches + validations + artifacts + events
- 150ms debounce 不足以抑制高频 orchestrator 事件（step dispatch、tool call、evidence 写入）
- 无版本号/ETag 机制，无法判断数据是否真的变化
- 中间态渲染：snapshot 已 set 但 run 数据仍在 loading，UI 出现不一致帧
- `requestSeq` 只防旧覆新，不防中间态

**Go TUI：**

- `applyWorkspaceContext` 先调用 `resetWorkspaceView()`（清空所有字段），再填充新数据，导致一帧空白闪烁
- 多种事件类型触发 `FetchWorkspaceContext` 全量刷新（patch applied、memory recall、LLM usage 等）
- 事件通道容量 64，高频事件可能导致 channel 满后丢弃

**SSE 后端：**

- 无 `id:` 字段输出，不支持 `Last-Event-ID` 重连
- 无 revision/sequence 机制
- Envelope 只携带 topic + IDs + raw payload，无状态摘要
- Back-pressure 静默丢弃非关键事件

### 1.2 命令体系问题

- 缺少状态检查命令（coverage 进度、repair loop 详情、step timeline）
- blocking decision 时仍需手动输入 `/decision answer` 前缀
- 命令列表扁平，无分组
- 自然语言 "继续" 有快捷路径，但 "查看状态"、"查看进度" 无对应
- CLI 有 `run status`/`run observability` 但 TUI 无等价命令

---

## 二、设计方案

### Phase 1: 后端事件增强

#### 1.1 Workspace Revision Counter

在 event hub 层为每个 workspace 维护单调递增 revision。

**修改文件：**
- `internal/pkg/events/envelope.go` — `Envelope` 增加 `Revision int64` 字段
- `internal/pkg/events/hub.go` — MemoryHub 增加 `revisions map[string]int64`，Publish 时自增并注入
- `internal/pkg/events/magic_hub.go` — MagicHub 同上

**设计要点：**
- revision 由 hub 统一分配，publisher 不感知
- WorkspaceID 为空的事件 revision = 0（全局事件不参与追踪）
- revision 持久化随 workspace state 一起写入（file backend 已有 workspace metadata）

#### 1.2 SSE `id:` 字段 + 重连支持

**修改文件：**
- `internal/modules/application/workorchd/service/server/http.go`（line 1723 区域）

**变更：**
```
输出格式从：
  event: workorch\ndata: {json}\n\n
改为：
  id: {envelope.ID}\nevent: workorch\ndata: {json}\n\n
```

新增 serverState 级 ring buffer（256 条），通过常驻 EventHub 订阅记录事件；SSE handler 解析 `Last-Event-ID` header 或 `lastEventId` query 实现短时重连回放。

**新文件：**
- `internal/modules/application/workorchd/service/server/sse_replay.go` — ring buffer 实现

**实现结果：**
- `Envelope` 增加 lower camel JSON tag 与 `revision` 字段，SSE 输出与 Web 消费字段一致。
- `MemoryHub` / `MagicHub` 按 workspace 分配单调递增 revision；全局事件 revision = 0。
- `/api/v1/workorch/events/stream` 输出 `id:` 行，并在重连时按 `Last-Event-ID` / `lastEventId` 回放 ring buffer 中后续事件。

#### 1.3 State Summary 事件富化

在 SSE 序列化时（非 publish 时）为关键状态变更事件附加 `stateSummary`。

**修改文件：**
- `internal/modules/application/workorchd/service/server/http.go` — marshal 前调用 enrichment

**新文件：**
- `internal/modules/application/workorchd/service/server/sse_enrichment.go`

**富化范围：**

| Topic | 附加内容 |
|-------|----------|
| `/workspace/state/changed` | workspaceState, activeRunId, revision |
| `/run/state/changed` | runState, progressSummary, blockingTicketId, blockingApprovalId |
| `/result/accepted` | outcome summary |

**payload 结构示例：**
```json
{
  "stateSummary": {
    "workspaceState": "running",
    "activeRunId": "run-003",
    "runState": "running",
    "progressSummary": "Working: mcp/tool_call",
    "blockingTicketId": "",
    "blockingApprovalId": "",
    "revision": 42
  }
}
```

**设计要点：**
- 富化发生在 SSE handler 序列化时，不污染内部事件总线
- 只有 SSE 客户端承担富化开销
- 后向兼容：`stateSummary` 是可选字段，旧客户端忽略即可

**实现结果：**
- 新增 `sse_enrichment.go`，对 workspace/run/result 关键 topic 注入 `stateSummary`。
- `stateSummary` 放入 SSE 输出 payload，内部 EventHub envelope 保持原始事件。

---

### Phase 2: 前端增量消费

#### 2.1 Web Console: Topic 路由增量更新

**修改文件：**
- `web/workorch-console/src/hooks/useSSE.ts` — 传递完整 envelope（含 revision）给回调
- `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`（line 162-218）— 替换全量刷新

**路由策略：**

| Topic | 动作 | Debounce |
|-------|------|----------|
| `/step/dispatched`, `/step/state/changed` | 更新 progress 文本，不 fetch | 0ms |
| `/run/state/changed` | 从 stateSummary 更新 run state；终态时 fetch snapshot | 0ms |
| `/workspace/state/changed` | 从 stateSummary 更新 workspace state | 0ms |
| `/run/llm_usage/recorded` | 更新 LLM usage 计数器 | 500ms |
| `/patch/applied`, `/patch/reverted` | 仅 fetch patches 列表 | 200ms |
| `/result/accepted` | fetch run snapshot + workspace snapshot | 0ms |
| `/ticket/created`, `/approval/created` | 更新 blocking state | 0ms |
| `/memory/recall/recorded` | 追加 memory recall（不 fetch） | 500ms |
| 其他 | fetch workspace snapshot | 500ms |

**Revision gap 检测：** 收到的 revision 与 lastRevision 差 > 1 → 触发全量刷新。

**Reconnect：** `onReconnect` 回调触发全量刷新（利用 Last-Event-ID 回放减少丢失窗口）。

**实现结果：**
- `useSSE` 透传 `id`、`revision`、`stateSummary`，并用 `lastEventId` query 支持手动重连回放。
- `WorkspaceDetailPage` 内置 topic router：workspace/run state 本地合并，patch topic 只刷新 patch 页，终态/定义/治理阻塞类事件触发必要上下文刷新，revision gap 与 reconnect 走全量刷新。
- 未新增独立 `useWorkspaceEventRouter.ts`，原因是当前页面状态分散在多个本地 state 与分页加载函数中，内联路由可直接复用既有加载/合并函数，避免引入额外跨组件状态同步层。

#### 2.2 Go TUI: 消除 resetWorkspaceView 闪烁

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/workspace_state.go`（line 204-311）

**变更：**
- 删除 `m.resetWorkspaceView()` 调用
- 改为逐字段更新：只在新数据非空时覆盖，空值保留旧值
- 扩展已有的 `restoreDefinitionsWhenPartialContext` 模式到所有字段

**示例逻辑：**
```go
// Before (causes flash):
m.resetWorkspaceView()
m.decisions = msg.Decisions

// After (no flash):
if msg.Decisions != nil {
    m.decisions = msg.Decisions
}
```

**实现结果：**
- `applyWorkspaceContext` 删除普通刷新路径中的 `m.resetWorkspaceView()`。
- `Decisions` / `Approvals` 仅在响应中非 nil 时覆盖。
- 无 snapshot 的 partial context 保留已有 Goal / DoD / runtime display；完整空 snapshot 仍清空服务端已确认不存在的定义。

#### 2.3 Go TUI: 减少全量刷新触发

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/model.go`（line 1428-1601, `applyBackendEvent`）

**改为增量处理的事件：**

| 事件 | 增量处理方式 |
|------|-------------|
| `EventPatchPreviewed/Applied/Failed/Reverted` | 从 payload 构建 PatchInfo 追加到 `m.patches` |
| `EventArtifactContinuationChanged` | 从 payload 更新 `m.artifactContinuations` |
| `EventMemoryRecallRecorded` | 从 payload 追加到 `m.memoryRecalls` |
| `EventLLMUsageRecorded` | 从 payload 更新 `m.llmUsage` |

**保留 `FetchWorkspaceContext` 的场景：**
- `EventResultAccepted` / `EventResultRejected`
- Run 终态（completed/failed/cancelled）
- `EventGoalDefined` / `EventDoDDefined`
- Workspace 终态

**实现结果：**
- patch 事件通过 `patchInfoFromEvent` / `upsertPatch` 增量更新。
- artifact continuation 事件通过 `artifactContinuationInfoFromEvent` / `upsertArtifactContinuation` 增量更新。
- memory recall 事件通过 `memoryRecallInfoFromEvent` / `upsertMemoryRecall` 增量更新。
- LLM usage 事件通过 `applyLLMUsageEvent` 累加 token 统计并更新 context budget。
- 上述事件只追加 runtime event 和刷新本地视图，不再触发 `FetchWorkspaceContext`。

#### 2.4 Event Channel 容量

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/model.go`（line 138）

**变更：** `make(chan backend.Event, 64)` → `make(chan backend.Event, 256)`

#### 2.5 Revision 追踪

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/model.go` — `AppModel` 增加 `lastRevision int64`
- `web/workorch-console/src/hooks/useSSE.ts` — `lastRevisionRef` 追踪

**实现结果：**
- TUI 在收到 workspace revision gap 时触发 `FetchWorkspaceContext`。
- Web Console 在收到 revision gap 时触发 workspace list + context 全量刷新。

---

### Phase 3: 命令体系

#### 3.1 上下文感知直接输入

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/model.go`（line 544-585, `submitInput`）

**Plain text input 路由规则：**

| 优先级 | 条件 | 行为 |
|--------|------|------|
| 1 | blocking decision | 作为 decision answer（已有） |
| 2 | blocking approval | 作为 approval intent（已有） |
| 3 | running + active run | 作为 operator input（ContinueRun）**新增** |
| 4 | idle + workspace | 作为 new goal（SubmitGoal）**新增** |

当前第 3 条只在 `isRunControlIntent` 匹配时触发，改为所有 plain text 都作为 operator input。

**实现结果：**
- active run / running 状态下 plain text 进入 `ContinueRun(..., operatorInput)`。
- idle workspace 下 plain text 进入 `SubmitGoal`。
- `/goal` 仍可显式把“继续处理”等 run-control 文本作为 goal 提交。

#### 3.2 新增 Inspect 命令

**状态：已收口**

**新文件：**
- `internal/modules/application/workorch/service/tui/inspect_commands.go`

**命令列表：**

| 命令 | 功能 | 数据来源 |
|------|------|----------|
| `/status` | workspace + run 状态摘要 | `m.snapshot` + `m.progress` |
| `/coverage` | DoD coverage 进度 | `m.dodItems` + snapshot taskProgress |
| `/steps` | 最近 N 个 step 摘要 | snapshot steps（需 fetch） |
| `/timeline` | 事件时间线 | `m.runtimeEvents` |
| `/patches` | patch 列表 | `m.patches` |
| `/artifacts` | artifact 列表 | `m.artifacts` + `m.artifactContinuations` |

**修改文件：**
- `internal/modules/application/workorch/service/tui/model.go`（`handleSlashCommand`）— 增加 case 分支
- `internal/modules/application/workorch/service/tui/input.go`（`baseSlashCommandSuggestions`）— 增加建议项

**实现结果：**
- 新增 `inspect_commands.go`，包含 status / coverage / steps / timeline / patches / artifacts 渲染。
- `handleSlashCommand` 与 slash suggestion 已接入上述命令。
- `/steps` 当前基于 `TaskDecompositionInfo` 的节点摘要展示，避免新增后端 fetch 合同。

#### 3.3 命令分组 Help

**状态：已收口**

**修改文件：**
- `internal/modules/application/workorch/service/tui/view.go`（help 渲染区域）

**分组结构：**
```
工作流:
  /goal <text>         提交新目标
  /run continue        继续当前运行
  /run restart         重启工作空间
  /dod <items>         设置验收标准

检查:
  /status              工作空间与运行状态
  /coverage            DoD 覆盖进度
  /steps               Step 时间线
  /timeline            事件时间线
  /patches             Patch 证据
  /artifacts           交付物列表
  /memory              记忆召回

治理:
  /decision answer     回答当前决策
  /decision replan     请求重规划
  /approval approve    批准请求
  /approval reject     拒绝请求

系统:
  /workspace           选择工作空间
  /config              系统配置
  /clear               清除显示
  /help                显示帮助
  /exit                退出
```

**实现结果：**
- `renderHelpOverlay` 已按 Workflow / Inspect / Governance / System 分组。
- 底部短 help 纳入 `/status` / `/coverage` 等常用 inspect 命令。

#### 3.4 CLI 新增子命令

**状态：已收口**

**新文件：**
- `internal/modules/application/workorch/service/cli/run_inspect.go`

**命令：**
- `run coverage <id>` — 读取 workspace snapshot，展示 DoD 覆盖状态
- `run timeline <id>` — 读取 workspace snapshot 中的 event records，展示 run 时间线

**命名对照：**

| TUI 命令 | CLI 命令 |
|----------|----------|
| `/status` | `run status <id>` |
| `/coverage` | `run coverage <id>` |
| `/steps` | `run observability <id>` |
| `/timeline` | `run timeline <id>` |
| `/patches` | `run patches <id>` |

---

## 三、实施顺序

| 序号 | 内容 | 依赖 | 复杂度 | 效果 |
|------|------|------|--------|------|
| 1 | Go TUI 闪烁修复（2.2） | 无 | 低 | 已完成 |
| 2 | Channel 容量（2.4） | 无 | 极低 | 已完成 |
| 3 | 上下文感知输入（3.1） | 无 | 低 | 已完成 |
| 4 | Inspect 命令 + Help 分组（3.2+3.3） | 无 | 中 | 已完成 |
| 5 | Go TUI 减少全量刷新（2.3） | 无 | 中 | 已完成 |
| 6 | Revision counter（1.1） | 无 | 中 | 已完成 |
| 7 | SSE id + replay（1.2） | 6 | 中 | 已完成 |
| 8 | State summary 富化（1.3） | 6 | 中 | 已完成 |
| 9 | Web Console topic 路由（2.1） | 8 | 高 | 已完成 |
| 10 | Revision 追踪（2.5） | 6 | 低 | 已完成 |
| 11 | CLI 子命令（3.4） | 无 | 低 | 已完成 |

两批内容均已完成代码收口。

---

## 四、后向兼容性

| 变更 | 兼容性保证 |
|------|-----------|
| `Envelope.Revision` 字段 | 默认 0，旧客户端忽略 |
| SSE `id:` 行 | 旧客户端不解析 `id:` 行，无影响 |
| `stateSummary` payload key | 可选字段，旧客户端不读取 |
| Topic 路由 | 保留 fallback 全量刷新路径 |
| 上下文感知输入 | `/` 前缀命令行为不变 |

---

## 五、验证方式

| 验证项 | 方法 |
|--------|------|
| Go TUI 闪烁修复 | `go test ./internal/modules/application/workorch/service/tui -count=1` 覆盖 partial context 不清空显示、完整空 snapshot 清空定义 |
| 上下文感知输入 | `go test ./internal/modules/application/workorch/service/tui -count=1` 覆盖 active run plain text → operator input、idle workspace plain text → goal |
| Inspect 命令 | `go test ./internal/modules/application/workorch/service/tui -count=1` 覆盖 `/status`、`/coverage`、`/timeline`、`/patches`、`/artifacts` |
| Revision counter | `go test ./internal/pkg/events -count=1` 覆盖 MemoryHub / MagicHub workspace revision |
| SSE id / stateSummary / replay | `go test ./internal/modules/application/workorchd/service/server -run 'TestEventStream|TestEventRecordQuery' -count=1` |
| Web Console 增量 | `npm --prefix web/workorch-console run build` 验证类型与打包 |
| CLI 子命令 | `go test ./internal/modules/application/workorch/service/cli -count=1` |
| 整体 | 本轮未执行 `make test-race`；已执行受影响包定向测试与 Web build |

---

## 六、关键文件索引

| 文件 | 角色 |
|------|------|
| `internal/pkg/events/envelope.go` | Envelope 结构定义 |
| `internal/pkg/events/hub.go` | MemoryHub（测试用） |
| `internal/pkg/events/magic_hub.go` | MagicHub（生产用） |
| `internal/pkg/events/topics.go` | 37 个事件 topic 常量 |
| `internal/modules/application/workorchd/service/server/http.go:1723` | SSE endpoint handler |
| `internal/modules/application/workorch/service/tui/model.go` | TUI 主模型 + Update + applyBackendEvent |
| `internal/modules/application/workorch/service/tui/workspace_state.go` | applyWorkspaceContext + resetWorkspaceView |
| `internal/modules/application/workorch/service/tui/input.go` | slash 命令建议 + 自然语言识别 |
| `internal/modules/application/workorch/service/tui/view.go` | 渲染 + help |
| `internal/modules/application/workorch/service/tui/commands.go` | FetchWorkspaceContext + 命令 Cmd |
| `web/workorch-console/src/hooks/useSSE.ts` | SSE hook |
| `web/workorch-console/src/pages/WorkspaceDetailPage.tsx:162-218` | SSE 事件处理 + loadWorkspaceContext |
| `web/workorch-console/src/api.ts` | REST API 客户端 |
