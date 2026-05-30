# Storage And Read Model Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-29

## Purpose

本文件说明 WorkOrch 的持久化布局、payload refs、read-model projection
和 file backend / memory backend 的实现边界。

## Main Responsibility

该层负责：

- repository persistence contract
- file backend layout
- itemized runtime evidence storage
- append-only event storage
- raw payload ref strategy
- snapshot / observability / list projection inputs

## Stable Design Points

### Workspace Internal Storage

runtime state 应位于：

- `.workorch-data/internal/workspaces/<workspace-id>/`

相应的用户交付根目录应位于：

- `.workorch-workspaces/<workspace-id>/`

runtime tool 执行应绑定后者，repository/read-model/payload refs 应绑定前者。

### Itemized Run Storage

run 级对象应按 itemized 文件组织，而不是单个大 JSON map 重写。

典型 itemized 资源包括：

- step metadata / input / output
- tool call metadata / arguments / output
- validation metadata
- result / artifact records
- LLM call request / response / usage
- artifact continuation metadata / plan / chunk / assembled payload

### Artifact Continuation Persistence Boundary

当 runtime 启用 `incrementalArtifact` 时，本层必须把 continuation 视为
run-scoped internal state，而不是 workspace deliverable。

至少需要持久化：

- continuation metadata
  - `artifactContinuationId`
  - `runId`
  - current state / phase
  - `artifactId`
  - `targetRef`
  - `coverageTargets`
  - selector / retry / hash metadata
- accepted artifact plan
- chunk metadata
- chunk content
- assembled metadata
- assembled content

稳定边界：

- continuation 正文默认不进入 snapshot summary / list projection
- plan/chunk/assembled content 不得写入 workspace deliverable 根
- continuation content 不得默认作为 memory recall 或 event payload 正文
- 只有 compact metadata、状态摘要、hash、bytes、evidence refs 和 raw/detail ref
  可以进入 read model

典型 internal layout 应保持 run-scoped、itemized、可重放；具体路径命名可演进，
但必须满足：

- reload 后 continuation 可恢复
- chunk 顺序与 hash 可重建
- applied / failed terminal state 可区分 open continuation

### Raw Payload Strategy

raw input/output、LLM request/response、event payload 大对象应通过 ref 暴露，
而不是默认塞进主读模型。

事件应优先采用 append-only JSONL metadata + optional payload ref 的模式。

### Projection-Oriented Read Model

read model 应以：

- compact metadata
- summary
- evidence refs
- paginated detail

为主，而不是默认返回完整 raw payload。

默认至少应区分：

- summary
- detail
- raw

summary / detail 使用 compact metadata、summary、evidence refs 和分页 detail；
raw 仅通过显式 ref 打开。

对 `incrementalArtifact` 的投影还必须补充：

- workspace / run snapshot 只投影 active continuation summary
- observability / detail 可以投影 plan/chunk/apply/validation 状态、sequence 进度、
  bytes、hash 摘要和 failure reason
- raw open path 只在显式 detail/raw 打开时暴露 plan/chunk/assembled content

### Snapshot Projection Structure

snapshot projection 当前至少应稳定分为三层：

| Projection | 当前最小结构方向 |
| --- | --- |
| `WorkspaceSnapshot` | `workspace`、`outcome`、`runs`、`results`、`artifacts`、`taskProgress`、必要 governance/context 摘要 |
| `RunSnapshot` | `run`、`outcome`、`taskProgress`、`results`、`artifacts`、`closureTraces`、必要 evidence summary |
| `RunObservability` | `timeline`、`evidence`、`latestCheckpoint`、`llmUsageSummary`、`usedCapabilities`、closure 诊断 |

约束：

- snapshot 是聚合投影，不替代 run/result/goal/dod/event 本体
- 同一对象在 snapshot 与 observability 中应复用同一 closure / taskProgress 选择规则
- detail page 与 raw open path 不得因为 snapshot 已存在而被折叠掉

Coverage / ledger 相关的读模型也必须分层：

- formal definition
  - Goal / DoD / deliverable expectation 的正式定义
- run-scoped ledger
  - `coverageWorkItems`、accepted/rejected evidence refs、revision lineage
- projection
  - `coverageWorkItemContext.summary` / `currentFocus` / `ledgerSummary`

约束：

- projection 只表达当前视图，不反向覆盖 run-scoped ledger
- run-scoped ledger 不得写回 formal Goal / DoD
- full ledger 默认通过 raw/payload ref 或 detail 打开，而不是强塞进 summary

### Run-Scoped Coverage Ledger Persistence

当 runtime 维护 `coverageWorkItems` / `coverageWorkItemContext` 时，本层至少应承接：

- run-scoped 持久化
- `definitionRevision` / source refs / accepted / rejected evidence refs
- `historical` / `superseded` item 保留
- `currentFocus` / omitted counts / compact ledger summary 的投影输入

稳定持久化约束：

- 同一 `definitionRevision + normalizedSourceKey` 只能保留唯一 active item 记录
- `acceptedEvidenceByKind` / `rejectedEvidenceByKind` 必须按 evidence kind 分开持久化，
  不得让单一 evidence ref 批量关闭不同 kind
- revision 变化、source 移除或 successor rebuild 后，旧 item 必须进入
  `historical` / `superseded` 视图，不能继续混入当前 focus
- run-scoped ledger 与 projection 必须保留 `omittedCount`、summary、full-ledger ref
  的一致投影语义，避免 Web/TUI/API/LLM-facing 看到不同 ledger 轮廓

本层不负责：

- 领域语义装配
- profile-specific claim 判断
- accepted / rejected 的最终业务裁决

### Artifact Continuation Projection Direction

当 run 存在 active `ArtifactContinuation` 时，read-model 至少应支持以下投影分层：

| 投影层 | 最小内容 |
| --- | --- |
| workspace summary | 当前是否存在 active continuation、targetRef、state、sequence progress、failure summary |
| run snapshot | continuation metadata、output mode、apply/validation 进度、evidence refs |
| run observability | chunk timeline、hash/bytes、plan accepted/rejected、assemble/apply diagnostics |
| raw/detail | plan 正文、chunk content、assembled content、full metadata |

约束：

- summary/snapshot 不得内嵌 chunk content 或 assembled body
- observability 默认展示 compact metadata 与 refs，不直接暴露正文
- raw/detail 打开必须继续走受控 ref / endpoint
- continuation projection 不能替代正式 Result / Artifact / Validation projection

## File Backend Rules

- overwrite 写入必须使用原子写策略
- append-only event 写入必须有 per-run append lock
- segment rotation 必须原子完成
- reader 不应观察到半写入 JSON

## API And Read Model Direction

- workspace snapshot 不应默认内嵌 raw `eventRecords`
- run snapshot 默认返回 summary/detail projection
- event list 必须有默认 `limit`
- raw payload 通过 ref 或 detail endpoint 显式打开
- workspace 级只投影 active run 的 coverage summary，不替代 run-scoped ledger
- run snapshot / observability 可投影 `coverageWorkItems` / `coverageWorkItemContext`
- currentFocus / omitted counts / full-ledger ref 应保持同一投影语义，避免 Web/TUI 与 LLM-facing projection 不一致

### Detail Endpoint Support Mapping

storage/read-model 层对 detail endpoint 的支撑关系至少应保持：

| Detail Endpoint Family | 主要仓储/投影输入 |
| --- | --- |
| `steps` / `tool-calls` / `runtime-checkpoints` | itemized run evidence records |
| `results` / `artifacts` / `patches` / `validation-results` | result / artifact / patch / validation repositories |
| `artifact-continuations` | continuation metadata / plan / chunk / assembled repositories |
| `decisions` / `approvals` | governance repositories + resolution/approval state |
| `events` | append-only event storage + page query |
| `runtime/payloads` / `artifacts/content` | payload ref / artifact URI resolution + file backend content open |

约束：

- snapshot 不替代 detail page
- detail page 不替代 raw open path
- page/cursor/limit 应优先由 repository query contract 驱动，而不是由前端推断全量装载

### Repository And Read-Open Contract

本层至少需要承接以下 repository/read-open contract：

- list/get/create/update 的 itemized persistence contract
- page query contract
  - `limit`
  - `cursorAfterId`
  - `hasMore` / `nextCursorAfterId`
- raw open contract
  - `payloadRef` -> raw payload
  - `artifact URI` -> artifact content
- replay / restart contract
  - file backend 重启后 snapshot / observability / event replay 可重建

path-safe readable naming 方向：

- 面向用户或产品层的 ref / uri / targetRef 应优先使用稳定、可读、workspace-relative 命名
- 本地绝对路径、临时内部路径和未受控文件名不应直接暴露到 API / UI

### Path-Safe Readable Naming

本层对 path-safe readable naming 的稳定职责至少包括：

- Workspace / Run / Step / Artifact / payload 相关 ref 使用稳定、可读、相对 workspace 的逻辑命名
- 用户或产品层可见的 URI / ref / targetRef 必须避免：
  - 本地绝对路径
  - `..`
  - 未受控的临时文件名
  - 仅内部实现可理解的随机目录结构

典型方向：

| 场景 | 当前命名方向 |
| --- | --- |
| workspace deliverable target | workspace-relative `targetRef` |
| artifact 正文 | 受控 artifact URI |
| raw payload 打开 | 受控 `payloadRef` |
| detail page item identity | run-scoped readable id / cursor id |

约束：

- readable naming 只服务可追踪与产品可读性，不改变正式对象 identity
- 内部磁盘目录可继续保持 itemized persistence 结构，但对外必须经由受控 naming / ref 映射

### Raw Payload Ref Shape

run-scoped raw payload ref 应继续使用同一可读层级规则，而不是暴露本地绝对路径。

典型方向：

```text
workorch://workspaces/<workspace-id>/runs/<run-id>/llm-calls/<run-id>/repair-001/response.json
workorch://workspaces/<workspace-id>/runs/<run-id>/steps/<run-id>/graph-002/007-run-go-test/output.json
```

约束：

- raw ref 是稳定逻辑定位，不是本地文件系统路径
- 打开前必须继续校验：
  - `workspace`
  - `run`
  - collection
  - item path
  - 文件名白名单
- Step / ToolCall / Patch / Result / Artifact 等 run-scoped ID 可以复用可读 run 前缀，
  但不应退化成随机 hash / base64 主路径

### Segment Overflow Fallback

当单段 payload、事件段、artifact blob 或列表页数据过大时，本层需要有稳定 fallback。

当前至少应支持：

- append-only event / payload segment 轮转
- 大对象降级为 ref-only / detail-only
- page query 通过 `limit` / `cursorAfterId` 分段读取
- snapshot / observability 只保留 compact summary，把 full body 留给 raw/detail 打开

约束：

- segment overflow 只影响读取方式，不应改变对象本体身份
- fallback 后仍必须保留可追溯 ref / cursor / count / omitted reason
- 不能因为 overflow 就把 raw body 静默塞回 summary projection

## Consistency Repair Boundary

当 run-scoped ledger、read-model projection、result artifact 或 UI 展示不一致时：

- 以 run-scoped ledger 为准
- projection 负责重建 summary / detail / currentFocus
- result artifact 或 UI 不得反向覆盖 ledger
- console / TUI / Web 应使用 pagination 和 lazy loading，而不是默认全量加载

## Boundary

该层不负责：

- business outcome 语义裁决
- provider request contract
- TUI/Web 产品展示策略

## Source Documents

- `../10-domain-model/workspace-goal-dod.md`
- `../10-domain-model/event-observability.md`
- `../50-product-interface/service-api.md`
- `../60-verification/service-acceptance.md`
