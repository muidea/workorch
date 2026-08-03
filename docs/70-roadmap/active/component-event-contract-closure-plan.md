# 22 组件 EventHub Topic / Data / Result 归一收口计划

Status: active

Type: closure-plan

Last Updated: 2026-08-03

## 1. 范围与唯一目标

本计划是 22 个 framework 运行单元的 EventHub 合同收口唯一实施队列。它不定义 Runtime 状态迁移，
不替代 [`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)，也不修改 SM-1～SM-4 的 owner。
Runtime semantic/CWI 正向收敛继续由
[`runtime-cwi-reconciliation-single-owner-closure-plan.md`](runtime-cwi-reconciliation-single-owner-closure-plan.md)
跟踪；本计划只关闭运行单元之间的通信合同与使用边界。

目标是让每项生产合同同时满足：

1. 维护资源或状态的接收 owner 在自身 `pkg/events` 定义唯一 topic 和 concrete `Command`、`Data`、`Result`；
2. 调用方显式导入接收 owner 合同，不复制、不 alias、不通过共享 registry、JSON、reflect 或兼容桥间接取得合同；
3. 需要结果、失败反馈、强一致记账或状态迁移的命令只使用 `Send`；真正的异步事实通知只使用 `Post`；
4. 每个生产 command 都有真实 sender 和 receiver，每个 notification 都有真实 publisher 和 subscriber；
5. handler、sender 和 Result decoder 严格使用 `MessageContracts()` 声明的具体类型；
6. EventHub 合同类型图只传有界、可复制、排序稳定的数据，不包含 map、无约束 `any`、channel、I/O 或 owner 可变资源；
7. 所有 Post handler 在 `result == nil` 时安全，所有 Send caller 检查 nil、error、`Get()` error 和精确类型；
8. 合同删除、迁移和重命名不保留兼容 alias、forwarder、dual subscribe 或 fallback。

当前阶段不考虑兼容处理。失效合同直接删除，不能为了历史调用形状建立第二通信路径。

## 2. 规范基线

本计划采用 `go-multi-module-dev` 的以下硬规则：

- Initiator 不拥有业务 topic，也不内嵌 `basebiz.Base`；
- 使用 EventHub 的 Module/Block 必须在 Biz 内嵌 `basebiz.Base`；
- `module.go` 只做 framework 生命周期接线，`service/` 不持有 Hub、不订阅、不发送；
- `pkg/events` 是本 owner topic/Command/Data/Result 的唯一生产定义位置；
- `pkg/models` 只保存稳定 DTO/读模型，不能借 Result 泄漏 owner 可变状态；
- 同步 command handler 返回 concrete Result，Post notification handler 不依赖 Result；
- 共享包只保存 owner-neutral pure value/helper，不建立全局 topic 或类型索引。

WorkOrch 的额外约束：

- 22 单元类型基线固定为 1 Initiator、4 Modules、17 Blocks；本计划不新增运行单元；
- Runtime-sensitive 合同继续声明 layer、SM、effect、source、scope、identity、feedback、lane metadata；
- 合同搬迁不得改变 Runtime reducer、coverage credit、sealed dispatch、receipt 或 terminal delivery 语义；
- owner 内部状态转换仍只能由 owner 私有事件/reducer 触发，EventHub command 只提交 typed intent/fact。

## 3. 2026-08-03 审计事实

### 3.1 已符合的基线

- 审计基线共有 307 个 topic；EC-B1/B2/B3 已删除 24 个无生产闭环 topic，projection/activation/release 收口又删除 3 个 write topic，本轮继续删除 14 个仅有测试 publisher 或仅有 subscriber 的通知，当前生产 inventory 为 266 个，仍分布于
  20 个 owner `pkg/events`；`workorchruntime` 与 `kernel/system` 不定义业务 topic；
- `Contracts()` 与 `MessageContracts()` 数量一致，topic 全局唯一；
- owner、receiver、command class、lane policy 和 Runtime metadata 现有登记门禁通过；
- 使用 Hub 的 21 个 Module/Block 均通过 Biz 内嵌 Base；Initiator 不使用 Base；
- test-only 递归源码图已拒绝 map、any、channel、function、I/O、database/sql、net/http、Repository、Registry 与 Recorder 等资源句柄；MessageContract 的 Command/Data/Result root 同时拒绝同包 type alias；
- command sender/receiver 与 Post site 已形成 test-only 源码图/零增长门禁；Post 生产点只允许 concrete Notify 合同；
- notification subscriber 完全忽略同步 Result，test-only AST 拒绝 callback 调用 `Result.Set`；module 内未消费的 `Send` Result 由零增长门禁拒绝；
- 二次 owner-local 定义权审计已清除共享 `workorchownercontract` 中的 2 个 Topic 与 Approval/MCP Registry 业务 DTO；
  workorchd、Workspace、Orchestrator 的 Event 合同不再直接导入其他 owner 的 `pkg/events`/`pkg/models`；新增 AST
  门禁拒绝共享业务 Topic、转引 Topic、跨 owner Event package import 与共享业务 model alias；
- 现有 eventcontract、boundary 和 modules 测试全部通过。

上述结论由登记、递归类型图、生产使用闭环、Post site、owner mapping 与行为测试共同证明；完整 race、console、managed deployed 验证仍由 EC-F2 跟踪。

### 3.2 组件结论

| 组件 | Topic | 当前结论 | 必须关闭的缺口 |
| --- | ---: | --- | --- |
| `initiators/workorchruntime` | 0 | 符合 | 保持无业务 topic、无 Base |
| `application/workorchd` | 56 | 符合 | CLI Workspace/WorkspaceSnapshot/Template 已使用 owner-local concrete DTO；Template 不再复用 Workspace `SoulBinding`，跨 owner 转换只在纯无状态 mapping 边界进行 |
| `kernel/governance` | 5 | 符合 | 保持 typed Data/Result 与 Post nil 回归 |
| `kernel/orchestrator` | 13 | 符合 | 5 个 evidence query 均使用独立 concrete Command；ToolCall/Patch/ValidationResult 使用 Orchestrator owner-local immutable Result DTO；Biz 显式映射 persistence model，不改变 SM-2/SM-3 aggregate 与 receipt |
| `kernel/system` | 0 | 符合 | 保持只发送各接收 owner 的同步 ensure command |
| `blocks/approval` | 9 | 符合 | Topic 与 Approval DTO 均由 owner 直接定义；approved/rejected 使用不同 concrete Command，reject scope 不再依赖 alias 类型身份 |
| `blocks/decision` | 13 | 符合 | 无 publisher 的 updated/superseded 通知已删除；created/resolved notification 使用精确 Data；共享 helper 只绑定 lane |
| `blocks/eventstream` | 4 | 符合 | 失效订阅已删除；Purge DTO 已由 owner `pkg/events` 直接定义 |
| `blocks/file` | 15 | 符合 | 保持 owner-local typed capability/query/execute 合同 |
| `blocks/git` | 7 | 符合 | 保持 owner-local typed capability/query/execute 合同 |
| `blocks/llm` | 10 | 符合 | provider request EventHub DTO 的 `TraceRefs` 已 concrete 化；Get/Refresh catalog 分别精确解码 Result |
| `blocks/mcp` | 6 | 符合 | 保持 owner-local typed capability/query/execute 合同 |
| `blocks/mcpregistry` | 9 | 符合 | Topic 与 Server model 均由 owner 直接定义；Server Command/Result 的 `Env` 已 concrete 化，owner 与调用方显式映射 |
| `blocks/memory` | 14 | 符合 | Recall Trace Command/Result 的 `TypeQuota`、score、hit 已 concrete 化并深 clone |
| `blocks/result` | 19 | 符合 | 13 个 Command 已回迁，Create/Update 返回 owner clone 与 revision，不再共享可变指针 |
| `blocks/run` | 31 | 符合 | step detail/summary query 使用不同 concrete Command；replacement termination intent、同步 typed Result 与 CAS 确认语义保持不变 |
| `blocks/shell` | 6 | 符合 | 保持 owner-local typed capability/query/execute 合同 |
| `blocks/skill` | 3 | 符合 | EventHub DTO 已由 owner `pkg/events` 直接定义并映射私有 service input |
| `blocks/soul` | 7 | 符合 | EventHub DTO 已由 owner `pkg/events` 直接定义并 clone 映射私有 input |
| `blocks/ssh` | 5 | 符合 | 保持 owner-local typed capability/query/execute 合同 |
| `blocks/web` | 5 | 符合 | 保持 EventHub contract 与 provider config 隔离 |
| `blocks/workspace` | 29 | 符合 | fresh-activation release 使用 Workspace owner-local bounded disposition，不再导入 Run model；`SoulBindings` 使用稳定 entry slice，Biz 显式映射 Run fact |

当前代码结论为 22/22 组件符合。全部组件继续纳入全量门禁；EC-F2 只跟踪 race、console、managed deployed 与双 workspace 验证，不再承载已关闭代码项。

## 4. 缺陷清单与唯一处理

### EC-1 — EventHub 类型定义包不唯一

跨包 EventHub type alias 与 MessageContract root 同包 alias 均已清零。除 Memory、Result、Workspace 的 27 个外部
alias 回迁外，Approval、MCP Registry、Run、Workspace、Orchestrator 的 10 个 Command root alias 已改为独立 concrete
type；test-only AST 同时扫描外部 alias 与 generic root declaration，防止债务回归。owner-neutral supporting value 可以复用，
但不得作为 Command/Data/Result root alias。

处理规则：

1. 每个 MessageContract 使用的 Command/Data/Result 必须在接收 owner `pkg/events` 直接定义；
2. 同 owner Biz 用例需要相同字段时，由 EventHub handler 显式映射到 private use-case input；
3. 可复用字段只抽成 owner-neutral pure value，不复用整个 Command/Result；
4. 删除只为 EventHub alias 服务的 `pkg/contracts` 类型；仍有非 EventHub 用途的合同保留但不得被 MessageContract 引用；
5. 测试通过生产 AST 穷举 `MessageContracts()` 的 generic type argument，并在 test-only 类型核对中验证实际
   command/data/result `PkgPath` 等于 `DefinitionPackage`；禁止为此建立生产 reflect registry。

### EC-2 — Topic 生产/消费未闭环

以下是审计期间发现并已删除/私有化的原始债务，不是当前 inventory：

完全零使用：

- `TopicCLIProgress`
- `TopicOrchestratorRuntimeTerminalDeliveryRequested`

只有 subscriber、没有 publisher：

- `TopicApprovalApproved`
- `TopicApprovalRejected`
- `TopicDecisionTicketCreated`
- `TopicDecisionTicketResolved`
- `TopicDecisionTicketSuperseded`
- `TopicRunResumed`

只有 publisher、没有 subscriber：

- `TopicOrchestratorRuntimeGovernanceWaitRecorded`
- `TopicOrchestratorRuntimeContinuationEligible`

只有 handler、没有生产 sender 的 command：

- Approval：Create、Ensure、Update；
- Decision：CreateTicket、EnsureTicket、UpdateTicket、CreateResolution；
- Run：Ensure、LinkSuccessor、Continue；
- Skill：Update；
- Workspace：Ensure、Purge；
- Orchestrator：RetryRuntimeDrive。

最终源码图门禁又发现并删除了 Approval created/superseded、Decision updated/superseded 以及 10 个
Orchestrator Patch/ArtifactContinuation 展示通知；这些 topic 过去只有 subscriber，测试通过手工构造事件充当假 publisher。
当前保留 command 均有 production sender/receiver；保留 Post site 均绑定无 receiver 的 concrete Notify 合同。

处理规则：

- 被 canonical topic/owner-private path 替代的合同直接删除；
- 确有生产业务入口的 command 补唯一 typed sender、精确 Result decoder 和完整测试；
- 只服务 owner 内部的动作改为 private function/internal event，不暴露为跨组件 EventHub topic；
- 通知若只用于 audit，eventstream 必须显式订阅并验证 concrete Data；否则删除；
- 禁止用“未来可能调用”保留内部无 sender 合同。

### EC-3 — Write/状态命令错误使用 Post

经 Post 投递的 write/state/execute command 已清零。Workspace owner-self `ApplyRunProjectionFact`、Run owner-self `Activate`
public command 均已删除；fresh-activation release 改由 Run 同步 Result 返回完整 fact，Workspace 在原 Send 返回后调用私有 reducer；
Run → Orchestrator termination 则先与 successor 原子持久化 owner-local delivery intent，退出 Workspace → Run 同步 lane 后再由
`BackgroundRoutine` 使用 `Send` 投递和校验精确 SM-4 acknowledgement。pending/attempt/error/ack 均由 Run repository CAS 持久化，
启动恢复按 `WorkspaceID + RunID` 重投，不使用进程内 authority map。

处理规则：

- owner 内部状态转换改为 owner private internal event/reducer 调用；
- 跨 owner 强一致 intent 使用 Send，检查具体 ack/result；
- 若调用链必须异步，先由当前 owner 同步持久化 typed intent/outbox，再由 BackgroundRoutine 幂等发送；
- Post 只保留已经完成状态写入后的 immutable notification；
- 禁止简单把 `PostEvent` 替换为 `SendEvent` 而不核对 lane、重入、死锁、失败和重启语义。

该项影响 Runtime 时必须标注：Run lifecycle 为 SM-1；Orchestrator termination/delivery 为 SM-4；
Workspace projection/release 为 Workspace owner 私有状态。不得让其中一个 owner 直接触发另一个 owner reducer。

### EC-4 — 合同类型图包含 map/any

已替换并由递归源码图固化的字段：

- workorchd CLI 引用的 `workorchport.WorkspaceInfo`、`WorkspaceSnapshotInfo`、`TemplateInfo` 中的 `SoulBindings`、`Sections`；`WorkspaceRuntimeViewInfo`、`RunInfo`、`RunStatusInfo`、`ServiceConfig` 不含 map，`workorchport.Event.Payload` 未进入 MessageContract 图，不属于本项；
- Workspace Command/Result DTO 的 `SoulBindings`；
- MCP Registry Server Command/Result 的 `Env`；
- Memory Recall Trace Command/Result 的 `TypeQuota`；
- LLM provider request 的 `TraceRefs`；
- Orchestrator artifact continuation Result 的 `ReceivedChunkHashes`、`RetryCounters`；
- Decision notification helper 的 `data any`。

统一替换为排序稳定的 concrete entry slice，例如 `[]SoulBinding`、`[]EnvironmentVariable`、
`[]TraceReference`、`[]QuotaEntry`、`[]SectionState`、`[]ChunkHashEntry`、`[]RetryCounterEntry`。
构造与持久化边界必须 clone，Validate 必须拒绝重复 key；需要 map 查找时只能在 owner Biz 内部临时构建。

`workorcheventpayload.Payload` 是 concrete value contract，可以继续用于有界动态字段；但 EventHub message 不得调用
`Map()` 后把 `map[string]any` 作为 Data/Result 发送。

### EC-5 — Result 解码与错误处理不严格

1. workorchd LLM catalog caller 必须按 topic 分开解码：Get 只接受 `GetProviderModelCatalogResult`，Refresh 只接受
   `RefreshProviderModelCatalogResult`，删除裸 model fallback；
2. workspace purge 删除 Decision Resolution 时只允许忽略明确 `ErrNotFound`，其它错误必须返回；
3. 所有 Send caller 统一验证 result 非 nil、`Error()`、`Get()` error 和精确类型；
4. 禁止一个 decoder 根据多个可能 Result 猜测成功；
5. Result type mismatch 必须返回包含 topic、expected、actual 的 typed contract defect，不得继续下一状态动作。

## 5. 逐阶段实施顺序

### Phase A — 固化全量审计门禁

组件：`internal/pkg/eventcontract`、`internal/pkg/events`、boundary verifier、全部 owner `pkg/events`。

- 从 owner `Contracts()`/`MessageContracts()` 和生产 AST 自动生成审计事实，不建立生产全局 registry；
- 增加实际类型 `PkgPath`、递归 map/any/resource handle、Send/Post class、sender/receiver/publisher/subscriber 门禁；
- 明确测试、fixture、archive 不计入生产使用闭环；
- 对动态 topic helper 建立有限、可穷举的 concrete topic 集，禁止任意 string topic。

完成证据：新增门禁能在当前代码上精确报告 EC-1～EC-5，且无误报符合组件。

### Phase B — 删除失效合同和兼容形状

组件：workorchd、approval、decision、eventstream、run、skill、workspace、orchestrator。

- 删除零使用、无 publisher、无 subscriber 且无正式用途的 topic/message/contract/handler；
- 删除 eventstream 对失效通知的订阅；
- 对 14 个无 sender command 逐项作“补 sender”或“删除/私有化”裁决；
- 同步测试、readme 和 topic inventory，不保留 alias/forwarder。

完成证据：所有保留 command 和 notification 均形成生产闭环。

### Phase C — EventHub 类型回迁 `pkg/events`

组件：12 个存在外部 alias 的 owner。

- 按 owner 逐个迁移，不建立共享 message package；
- 同一提交切换 sender、handler、Result decoder、tests；
- 删除外部 type alias；
- 每完成一个 owner 运行其包测试和全量 eventcontract 门禁。

完成证据：MessageContract 实际类型定义包 100% 等于 owner `pkg/events`。

### Phase D — Post/Send 与 owner internal event 归一

组件：Run、Workspace、Orchestrator；关联 workorchd scheduler/recovery。

- 为剩余 termination Post 画出 source owner、receiver owner、lane、持久化点、失败返回和重启路径；
- owner 内部迁移使用 private internal event；跨 owner 交互使用 Send 或 durable outbox；
- 增加 duplicate、stale、re-entry、handler error、response loss、restart、双 workspace 隔离测试；
- 确认没有 EventHub handler 持锁 Send 到可能回调同 lane 的 owner。

完成证据：生产 Post 只投递 CommandClassNotify/DataTopic；所有 write/execute command 有完整 Result 闭环。

### Phase E — Payload/Result 具体化

组件：workorchd、Workspace、MCP Registry、Memory、LLM、Orchestrator、Decision。

- 用 concrete entry slices 替换 map/any；
- 定义排序、去重、clone、Validate 和 round-trip 规则；
- Result 返回 owner DTO clone，不暴露 owner 内部可变对象；
- 更新 TUI/Web/read model adapter，但不让 projection 变成正式状态 owner。

完成证据：递归 EventHub message type graph 不含 map、any、channel、function、I/O/owner resource handle。

### Phase F — Result 严格解码与全链验收

组件：workorchd、所有 Send sender、全部 owner handler。

- 删除 LLM catalog fallback 和所有被忽略 Send error；
- 为每个 Post subscriber 增加 `result=nil` 直接调用测试；
- 为每个 Result decoder 增加 nil/error/Get error/wrong type 测试；
- 运行全仓 test/race、boundary、console、managed deployed verification；
- fresh 双 workspace 验证 topic scope、lane、通知和状态转换互不串扰。

## 6. 文件级实施追踪

| ID | 状态 | 组件/路径 | 收口内容 | 验收 |
| --- | --- | --- | --- | --- |
| EC-A1 | code_closed | `internal/pkg/eventcontract`、`internal/pkg/events` | test-only AST 已固化外部及 MessageContract-root alias 零债务、递归 concrete/resource 图、command sender/receiver、Post site/Notify class、notification Result 零依赖与 ignored Send 零债务；不建立生产 registry | `go test ./internal/pkg/events -count=1` |
| EC-B1 | code_closed | workorchd、orchestrator | 已删除 `CLIProgress`、terminal-delivery-requested 及两个无消费者 lifecycle notification；Run 同步 owner command 与 terminal delivery 实际调用不变 | workorchd/orchestrator targeted tests |
| EC-B2 | code_closed | approval、decision、run、eventstream、orchestrator | 除此前删除项外，继续删除 Approval created/superseded、Decision updated/superseded 与 10 个 Orchestrator Patch/ArtifactContinuation 无 publisher 通知；测试不再手工伪造 publisher | notification closure gate + affected owner tests |
| EC-B3 | code_closed | 7 个 command owner | 14 个无 sender command 均已删除或回归 owner 私有用例；无测试专用 sender/兼容 topic | affected owner tests + sender/receiver scan |
| EC-C1 | code_closed | 全部 owner | 所有外部 EventHub alias 与 10 个同包 MessageContract Command root alias 已清零；sender、handler 与私有 use-case 映射使用 topic-specific concrete type | AST zero-debt gate + owner tests |
| EC-D1 | code_closed | Workspace | `RunStateChanged` fact 直接转为 Workspace private projection reducer input；owner-self public write topic 与 Post 已删除 | duplicate/projection reducer tests |
| EC-D2 | code_closed | Run、Orchestrator、Workspace | Run-private activation 与 Workspace private release 已闭环；termination intent 与 successor 原子持久化，后台 Send 严格解码 ack，CAS 记录 attempt/error/ack，支持 response-loss 幂等、启动重投、CAS conflict 与双 workspace 隔离 | SM-1/SM-4 + owner/repository tests |
| EC-E1 | code_closed | workorchd、Workspace | Workspace 与 workorchd CLI Workspace/WorkspaceSnapshot/Template 均使用 owner-local concrete DTO；纯 mapping 保留 stable order、duplicate rejection、detached clone、round-trip 与 nil/empty 语义 | recursive graph + mapping tests |
| EC-E2 | code_closed | MCP Registry、Memory、LLM | MCP Env、Memory RecallTrace TypeQuota/score/hit、LLM TraceRefs 均已 concrete 化；owner 与 caller 显式双向映射并拒绝重复 key | clone/order/duplicate tests |
| EC-E3 | code_closed | Orchestrator、Decision | Decision notification any 已清零；ArtifactContinuation Result 的 chunk hash/retry counter 已改为稳定 entries，未改变 Runtime reducer/authority | Result/notification tests |
| EC-F1 | code_closed | workorchd | Get/Refresh catalog 已按 topic 精确解码并拒绝裸 model/交叉 Result；purge resolution 仅忽略明确 NotFound | wrong-type/error tests |
| EC-G1 | code_closed | Approval、MCP Registry、workorchd、Workspace、Orchestrator | 清除共享 Topic/业务 DTO 与跨 owner Event DTO/model import；只在 Biz/adapter 显式双向映射，新增 owner-local 定义权 AST 门禁 | owner mapping tests + `go test ./internal/pkg/events -count=1` |
| EC-F2 | verification_pending | 全部 22 单元 | targeted/full test、full race、fmt、vet、boundary、console 与文档刷新已通过；剩余 managed deployed 和 fresh 双 workspace live | §7 剩余命令 |

每完成一项必须更新状态和可复现证据。不得用删除门禁、排除组件、测试专用 sender 或 archive 文档引用把项目标为完成。

## 7. 验证矩阵

按以下顺序执行：

```bash
go test ./internal/pkg/eventcontract ./internal/pkg/events -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/modules/blocks/... -count=1
go test ./internal/modules/kernel/... -count=1
make fmt-check
make vet
make verify-boundary-contract
go test ./... -count=1
go test -race ./... -count=1
make console-build
make verify-ci-mvp-file
```

涉及 Runtime-sensitive Post/Send 调整时，还必须补：

- SM-1 lifecycle transition/invariant/idempotency/restart/stale/duplicate tests；
- SM-4 scheduling/delivery/response-loss/recovery tests；
- 双 workspace 同 ID/不同 ID 并发隔离；
- idle/pending/error 路径 CPU guard；
- fresh deployed resume/restart/terminal delivery 验证。

## 8. 完成定义

只有同时满足以下条件，本计划才可归档：

1. 307 个审计基线 topic 已被重新裁决；删除后当前 inventory 与代码一致；
2. 每个保留 command 有真实生产 sender、receiver、精确 Command 和精确 Result；
3. 每个保留 notification 有真实 publisher、subscriber 和 concrete Data；
4. 所有 MessageContract 的实际类型定义包等于接收 owner `pkg/events`，外部 alias 与 Command/Data/Result root alias 均为零；
5. write/execute/state command 经 Post 投递为零；
6. Post handler 完全不读取或写入 Result，notification subscriber AST 门禁通过；
7. Send 的 nil/error/Get/type 检查覆盖率 100%，无被忽略错误或多类型 fallback；
8. 递归 EventHub message type graph 中 map、any、可变资源句柄为零；
9. 1/4/17 组件分类、Base Biz、module/service/initiator 边界继续通过；
10. Runtime 四状态机 owner、internal-event reducer、sealed authority 和 evidence gate 未被改变；
11. 全仓 test/race、boundary、console、managed deployed 和双 workspace 验证通过；
12. active overview、未关矩阵、组件 readme 与 typed contract 不再宣称未完成项已经闭环。
