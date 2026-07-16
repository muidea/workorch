# Module / Block / Initiator Inventory Audit

Status: active

## 2026-07-11 组件代码与 README 再核验

Status: active

- 22 个组件根目录 `readme.md` 已统一只保留“功能说明、组件交互及合同、相关约束”三个章节；迁移过程、完成状态、已知缺口和兼容层清理历史不再放入组件 README。
- 22 个组件基线串行测试已通过，module/block 生产代码未发现跨组件 `biz`、`service` 或 owner repository 直接注入。
- `application/workorchd` 的 scheduler command 已将 `ExecutionContextPatch map[string]any` 改为无 JSON/reflect 的递归具体值，并只在 application 消费边界恢复内部 execution context。
- `blocks/workspace` 的 update/template command 已将 context hints 与 evidence policies 改为无 JSON/reflect 的递归具体值，并只在 Workspace owner 边界恢复内部模型。
- `blocks/mcpregistry` 的 capability command 已将 `Schema map[string]any` 改为无 JSON/reflect 的递归具体值，并只在 registry owner 边界恢复内部模型。
- 再核验仍发现 Decision context/payload、Approval context、Run execution context 和 Orchestrator dispatch input 的 EventHub command 继续通过 contract alias 暴露 `map[string]any`；这些项必须完成具体值迁移并通过组件回归后，本轮再核验才能重新标记 `completed`。

## 2026-07-10 组件粒度与职责重新审计

Status: completed

本节采用新的组件定义重新审计当前 22 个注册组件，并作为后续逐项收口的当前权威入口。此前 EventHub typed command/result 与兼容层清理结论继续有效，但“正式状态 owner 必然属于 module”的旧分类规则不再适用。

### 新分类基线

| Kind | 当前权威定义 |
| --- | --- |
| module | 功能闭环必须依赖多个组件合同的组合、聚合、治理或编排。module 可以管理自身协调状态，但不能直接持有其他组件的 service/repository；与 module/block 的运行期交互只能通过 EventHub。 |
| block | 围绕一个资源聚合或一个技术能力，允许管理该能力自己的正式或运行时状态。被多个调用方使用不自动升级为 module；只要功能本身仍是单一能力，就应保持 block。 |
| initiator | 只提供一种无业务状态的进程级基础能力。不得承担业务状态管理、业务策略或多种基础能力聚合；下游只能取得其公开的单一基础设施合同。 |

“多个组件交互”指 module 的功能成立依赖两个或以上独立组件合同，并对这些交互承担组合、聚合或一致性责任。单一 owner 被多个调用方读写，不视为多组件 module。

### Module / Block EventHub-only 规则

允许：

- 消费组件导入事件投递组件的 `pkg/events`，获取投递方定义的具体 command/data/result。
- 通过 magicCommon `event.Hub.Send/Post/Subscribe` 完成同步命令、异步通知和订阅。
- 依赖 `internal/pkg` 中与具体 owner 无关的共享 runtime contract、ID、配置和纯 helper。

禁止：

- module/block 直接注入或调用其他组件的 service、repository、runtime adapter、reader callback 或实现对象。
- 生产代码通过可选构造参数保留绕过 EventHub 的跨组件调用路径。
- block 直接导入其他 block 的能力常量或实现 package 来推导可用能力；能力声明必须由 orchestrator request、EventHub 查询或 owner-neutral 共享合同提供。
- application/session/bootstrap 暴露其他 owner 的 repository 集合，使调用方能够绕过 EventHub。

### 目标分类汇总

- 保留为 module：6 个。
  - `application/workorchd`
  - `kernel/run`
  - `kernel/orchestrator`
  - `kernel/governance`
  - `kernel/eventstream`
  - `kernel/system`，但应调整为明确的系统默认资源初始化协调职责。
- 调整为 stateful block：8 个。
  - `blocks/workspace`，已由 `kernel/workspace` 迁移。
  - `blocks/result`，已由 `kernel/result` 迁移。
  - `blocks/decision`，已由 `kernel/decision` 迁移。
  - `blocks/approval`，已由 `kernel/approval` 迁移。
  - `blocks/memory`，已由 `kernel/memory` 迁移。
  - `blocks/mcp-registry`，代码路径 `internal/modules/blocks/mcpregistry`，已由 `kernel/mcp` 迁移。
  - `blocks/skill`，已由 `kernel/skill` 迁移。
  - `blocks/soul`，已由 `kernel/soul` 迁移。
- 保留为技术 block：7 个。
  - `blocks/file`
  - `blocks/git`
  - `blocks/llm`
  - `blocks/mcp`
  - `blocks/shell`
  - `blocks/ssh`
  - `blocks/web`
- `workorch/repositories`：已收窄为单一 repository provider initiator；目录 import path 仍为 `internal/initiators/workorchruntime`。

### 22 个组件逐项收口清单

| ID | 当前组件 | 目标类型 | 当前结论 | EventHub-only 缺口 / 收口动作 | 优先级 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| C01 | `workorch/runtime` → `workorch/repositories` | initiator | 已收窄为单一 repository provider/factory，类型名调整为 `RepositoryInitiator`。 | EventHub/CommandHub/runtime policy 导出与 `ResolveRuntimeHelper` 已删除；application/services 直接使用 framework EventHub，orchestrator 通过无状态配置函数读取 policy。 | P0 | completed |
| C02 | `application/workorchd` | module | HTTP、CLI、projection、scheduler、context registry 与 runtime session 组合多个 owner，module 定位合理。 | 生产 session/bootstrap 已移除 26 类 owner repository exports 与独立 projection Hub，只保留唯一 magicCommon CommandHub、BackgroundRoutine 与配置加载；repository builder、owner module fixture 和 repository 直写面已迁入 `biz/testkit`。 | P0 | completed |
| C03 | `kernel/workspace` → `blocks/workspace` | block | Workspace/Goal/DoD/Template 是一个 workspace-definition 聚合，不承担跨组件编排。 | 已迁移为 stateful block；组件 ID、入口、System defaults destination 与全部导入已同步，topic 和具体 command/result 合同保持稳定，调用方继续只走 EventHub。 | P1 | completed |
| C04 | `kernel/run` | module | 组合 workspace definition、Run/Step/Checkpoint 与 orchestrator 状态推进，满足多组件 module 定义。 | 生产 `Dependencies` 已删除 `WorkspaceReader` / `Orchestrator` 注入；Run biz 只暴露 EventHub assembly，repository/fake port 构造器已限制在 `_test.go`。 | P1 | completed |
| C05 | `kernel/orchestrator` | module | 组合 workspace、run、result、governance 和 7 个 runtime blocks，是明确的 runtime convergence module。 | 六类候选状态均仅由同一 convergence 状态机写入，外部只通过 Orchestrator 自有具体查询 command/result 读取；拆分会分散 contract gate、evidence、coverage 与 continuation transaction，因此保留为 module 内部协调状态。 | P1 | completed |
| C06 | `kernel/result` → `blocks/result` | block | Result/Artifact 是单一 formal-outcome 聚合，不承担多组件编排。 | 已迁移为 stateful block；Result create/update 与 Artifact create 均由 owner command handler 在持久化成功后单点发布具体通知，Orchestrator 重复发布已删除。 | P1 | completed |
| C07 | `kernel/decision` → `blocks/decision` | block | Ticket/Resolution 是单一 decision 聚合。 | X02 repository 直连保持关闭；已迁移为 stateful block 并同步组件 ID、入口与全部导入，evidence command/result、scope 与幂等合同保持稳定。 | P0 | completed |
| C08 | `kernel/approval` → `blocks/approval` | block | Approval 是单一授权状态组件。 | X02 repository 直连保持关闭；已迁移为 stateful block 并同步组件 ID、入口与全部导入，evidence command/result、scope 与幂等合同保持稳定。 | P0 | completed |
| C09 | `kernel/governance` | module | 组合 decision、approval、run、workspace、memory 等治理合同，并拥有 AutoGovernanceTrace 协调状态。 | 生产 `Dependencies` 已删除六类 owner reader 与 resolve/resume callbacks；EventHub adapters 已下沉到 biz，fake port assembly 仅保留在 `_test.go`。 | P1 | completed |
| C10 | `kernel/eventstream` | module | 核心职责是聚合多个组件事件形成统一审计流，属于跨组件聚合 module；EventRecord 仅为审计/投影状态，不是 owner 修复入口。 | 查询、purge、输入采集与 appended 输出均使用组件自有具体 command/data/result；逐 topic 订阅已替代 Envelope/map 聚合链，legacy `SubscribeWorkorch` 与 `eventRecordFromEnvelope` 已删除。 | P2 | completed |
| C11 | `kernel/memory` → `blocks/memory` | block | MemoryItem/MemoryRecallTrace 是单一 memory 聚合。 | 已迁移为 stateful block；MemoryItem create/update/delete 与 recall upsert 均由 owner command handler 单点发布具体通知，delete 显式校验 workspace scope，外围重复发布已删除。 | P1 | completed |
| C12 | `kernel/mcp` → `blocks/mcp-registry` | block | MCPServer/MCPCapability 是单一 registry 聚合。 | 已迁移到 `internal/modules/blocks/mcpregistry`，与 invocation block `blocks/mcp` 明确区分；组件 ID、入口、System defaults destination 与全部导入已同步，EventHub registry/invocation 合同保持稳定。 | P1 | completed |
| C13 | `kernel/skill` → `blocks/skill` | block | 单一 Skill registry。 | 已迁移为 stateful block 并同步组件 ID、入口与全部导入；owner command/query、bounded metadata 与 provenance 合同保持稳定。 | P1 | completed |
| C14 | `kernel/soul` → `blocks/soul` | block | 单一 Soul/stage-policy catalog。 | 已迁移为 stateful block 并同步组件 ID、入口、System defaults destination 与全部导入；结构化 constraints/provenance 合同保持稳定。 | P1 | completed |
| C15 | `kernel/system` | module | 实际代码负责 Soul、Template、MCP 默认资源初始化，属于多组件启动协调。 | 保持 `kernel/system` 命名并由应用入口显式启用；X03 repository 直连已关闭，三类 owner 只消费 System 定义的具体 EventHub command/result，lifecycle 顺序已位于 owner 订阅之后。 | P0 | completed |
| C16 | `blocks/file` | block | 单一文件系统 runtime 能力，类型合理。 | 只消费 `runtimeevent.ExecuteCommand` 并返回具体 result/data；生产代码无其他 module/block service、repository 或实现依赖。 | P2 | completed |
| C17 | `blocks/git` | block | 单一 Git runtime 能力，类型合理。 | 只通过 EventHub execute/result 合同工作；生产代码无 sibling block 实现依赖，也未暴露跨 block 调用入口。 | P2 | completed |
| C18 | `blocks/llm` | block | completion、embedding、rendering、budget 与 cache 都服务于同一 LLM 调用生命周期，保持单一技术 block 合理。 | provider/rendering/cache 已按 focused file/package 分离且无独立状态权威；X05 sibling block 静态依赖已关闭，step kind 统一来自 owner-neutral `workorchruntimecontract`。 | P0 | completed |
| C19 | `blocks/mcp` | block | 单一 MCP invocation 能力，类型合理。 | registry 查询由 block 内部 EventHub getter 完成，只导入 `blocks/mcp-registry/pkg/events` 与公开模型；未注入 registry service/repository。 | P2 | completed |
| C20 | `blocks/shell` | block | 单一 shell execution 能力，类型合理。 | 只消费 runtimeevent command/result；生产代码无其他组件实现依赖，长期运行与副作用继续受 runtime policy 约束。 | P2 | completed |
| C21 | `blocks/ssh` | block | 单一 SSH execution 能力，类型合理。 | 只消费 runtimeevent command/result；credential/session 使用结构化 ref，生产代码无其他组件实现依赖。 | P2 | completed |
| C22 | `blocks/web` | block | 单一 Web search/fetch 能力，类型合理。 | Approval 查询由 block 内部 EventHub getter 完成，只导入投递方 `pkg/events` 与公开模型；未注入 Approval service/repository。 | P2 | completed |

### 横向前置缺口

| ID | 缺口 | 当前证据 | 收口要求 | 状态 |
| --- | --- | --- | --- | --- |
| X01 | Initiator 聚合多种基础能力 | `RepositoryInitiator` 只保存 repository provider handles；注册 ID 已改为 `workorch/repositories`。 | EventHub wrapper、CommandHub、runtime policy 与通用 RuntimeHelper 已移出 initiator；`workorchdeps` 只解析 owner repository 分组。 | completed |
| X02 | Decision/Approval 跨 owner repository 写 EvidenceRecord | Decision/Approval 已移除 `EvidenceRecordRepository` 依赖；对应 repository provider 分组也不再暴露该 repository。 | Decision/Approval 分别定义具体 evidence command/result 和无 map/any 的递归值类型；Orchestrator 导入对应 `pkg/events`、同步订阅、幂等落库并返回具体 result。 | completed |
| X03 | System 直接访问多个 owner repositories | `kernel/system.Dependencies` 已只保留 framework EventHub；plugin 不再解析 SystemRepositories。 | System 定义三组具体 ensure command/result，Soul、Workspace、MCP 消费后只访问自己的 repository；System weight 调整为 owner 订阅之后、Orchestrator 之前。 | completed |
| X04 | Application 暴露完整 owner repository exports | 生产 `bootstrap.Exports` 与 runtime session 已不再包含或校验 owner repositories；`ExportsFromInitiator` 只解析 EventHub/CommandHub，生产 bootstrap 不再构造 repository set。 | 26 类 repository fixture、file/memory repository builder、owner module 集成装配与测试直写 exports 已集中迁入 `biz/testkit`。 | completed |
| X05 | LLM block 静态依赖其他 blocks | `runtime_adapter.go` 已删除 file/git/mcp/shell/ssh/web `pkg/common` 导入，校验与 capability declaration 统一引用 `internal/pkg/workorchruntimecontract` 的 step kind。 | sibling block 间不再通过实现包共享常量；后续新增 kind 必须进入 owner-neutral runtime contract 或由 orchestrator request 声明。 | completed |

### 逐项收口顺序

1. 先关闭 X02、X03、X04、X05，确保目录迁移前所有 module/block 生产交互已经是 EventHub-only。
2. 收窄或拆分 C01 initiator，并更新 `workorchdeps`、entry side-effect imports 和 framework lifecycle tests。
3. 迁移 C03、C06、C07、C08、C11、C12、C13、C14 为 stateful blocks；每次只迁移一个 owner，保持 topic 和具体 command/data/result 合同稳定。
4. 收口 C04、C05、C09 的测试旁路和内部状态组件边界。
5. 最后复核 C02、C10、C15 与 7 个技术 blocks 的 README、入口装配和全量测试。

### 每个组件的完成门禁

- 根目录中文 `readme.md` 已更新为最终类型、职责、状态所有权和 EventHub 交互合同。
- module 能列出至少两个由其组合、聚合或治理的独立组件合同；不能只因为拥有 repository 就保留 module 类型。
- block 能用一句话描述唯一资源聚合或技术能力，并且只管理自己的状态。
- initiator 只导出一种基础设施能力，不包含业务状态转换或多能力聚合。
- 生产代码不存在对其他 module/block `biz`、`service`、repository、adapter 或 callback 实现的直接依赖。
- 跨组件 command/data/result 由事件投递方的 `pkg/events` 定义，消费方按需导入和订阅，并通过 magicCommon EventHub 传递。
- 目标组件的直接测试、交互组件 pairwise tests、串行 `go test -p 1 ./internal/modules/... -count=1`、`make fmt-check`、`make vet` 和 `git diff --check` 全部通过。

## 2026-07-10 EventHub 兼容层后续清理

Status: completed

- 新增 `EventHubLaneScope` / `EventHubLaneKeyFor`，具体 command 的 lane 计算不再为了复用 lane 策略临时构造 `Envelope`。
- `kernel/governance` 的 decision/approval/run/workspace/memory owner adapter 已直接使用 framework 注入的 magicCommon `event.Hub`，删除包装后再调用 `RawHub()` 的反向兼容链。
- `kernel/run` 的 workspace reader 与 orchestrator command adapter 已直接使用 raw EventHub，删除 `MagicHub.RawHub()` 类型断言。
- `kernel/orchestrator` 的 state/result/governance repository adapters 及 runtime adapter 已直接使用 framework `CommandHub`；后续已继续删除独立 projection Hub 与 legacy Hub 构造参数。
- application bootstrap/runtime session 已从当时的 projection `EventHub` + framework `CommandHub` 过渡为唯一 magicCommon CommandHub；repository initiator 不再转发任何 Hub。
- `application/workorchd` 的 `runtimeports.EventHubActionPort` 与 managed runtime factory 已切换为显式 `CommandHub`，该目录不再包含 `RawHub()` 兼容代码。
- setup draft LLM command、activation embedding/run command 与 CLI governance continuation 已改为显式使用 session `CommandHub`；对应分散的 `rawMagicHubProvider` 已删除。
- server production startup、HTTP runtime policy update、projection/scheduler watcher、测试 runtime 与 server fixture 已统一只传入 magicCommon CommandHub，不再反查 `RawHub()` 或保留 projection Hub 参数。
- orchestrator biz dispatch-step 已改为显式 `CommandHub`，至此 `kernel/governance`、`kernel/run`、`kernel/orchestrator` 的生产代码均不再使用 `RawHub()` 反向拆包。
- `application/workorchd` 七组 command/read port、bootstrap、runtime session、testkit、server、`internal/services/workorch` backend/runtime 的单 Hub 兼容层已全部删除；全仓 Go 代码中 `RawHub()`、`rawMagicHubProvider`、`RawMagicHubProvider` 扫描为零。
- `internal/services/workorch` 到 `application/workorchd` 的 CLI facade 已从 `/cli/command` Envelope + `/cli/result` map/any 兼容协议切换为 owner 定义的 `CLICommand/CLIResult`，直接通过 magicCommon CommandHub 同步收发；旧 map 编解码与 `events.Response` 已删除。
- 测试兼容残留也已清理：orchestrator graph/repair 测试不再把 `DispatchStepCommand` 复制为 command-topic Envelope 后通过 `Published()` 断言，而是直接记录并断言具体 command；server decision/approval 测试辅助直接向 owner CommandHub 发送具体 command 并校验具体 result。
- projection/SSE/事件回放已改为 typed EventRecord/WorkspaceProjection data 加本地 HTTP DTO，不再需要 `MagicHub` 载体；后续不得恢复兼容 wrapper 或拆包路径。

## 2026-07-11 单 CommandHub 装配与构造兼容清理

Status: completed

- `application/workorchd` 的 bootstrap infrastructure exports、runtime session、managed runtime factory、runtimeports、module、HTTP Server、CLI dispatcher、testkit 与 `internal/services/workorch` backend/runtime 已统一只接收 magicCommon `event.Hub`；独立 projection `events.Hub` 字段、构造参数和 session export 已删除。
- `RequiredEventHub`、`EventHubFromContext`、`ContextWithEventHubs` 与双 Hub session helper 已删除；请求及后台任务统一使用 `ContextWithCommandHub`。
- Server 测试夹具已删除被忽略的 legacy `events.Hub` 参数；未注入 Hub 的独立构造会创建自己的 magicCommon CommandHub，并直接从该 Hub 建立 RuntimeSession。Server 全量测试通过。
- `internal/services/workorch/biz/backends` 的测试 dispatcher helper 不再返回空转的 legacy Hub；embedded backend 只暴露并使用具体 magicCommon CommandHub。
- `kernel/orchestrator/biz` 的三个构造函数已删除从未读取的 legacy `events.Hub` 参数；module 与 focused tests 不再创建只用于兼容传参的 `MemoryHub`。Orchestrator 全量测试通过。
- `kernel/run/biz` 的 test-only service assembly 已改为直接接收 magicCommon Hub，Run created/state-changed 测试直接观察 `RunStateChangedData`；legacy notifier、Envelope payload 与 `Published()` 断言已删除。`kernel/eventstream/biz` 测试 helper 也删除了被忽略的 legacy Hub 参数。
- HTTP/SSE 与 remote backend 测试已改用各包本地 `streamEvent` / `backendEventData` DTO；ManagedRuntime、workspace projection 与 Observed 测试直接构造 `EventRecordData` fixture，不再借用组件 Envelope。
- `internal/pkg/events` 的 `Envelope`、legacy `Hub`/`AsyncHub`、`MemoryHub`、`MagicHub`、`PublishByContract`、`EventHubLaneKey(Envelope)` 及其专属测试已删除；所有测试 fixture 直接创建和终止 magicCommon Hub。
- 当前全仓非 vendor Go 代码中，`events.Envelope`、`events.Hub`、`MagicHub`、`MemoryHub` 与 `PublishByContract` 扫描均为零；跨组件 command/data/result 继续由投递方 `pkg/events` 定义并通过唯一 magicCommon EventHub 传递。

## 2026-07-10 收口进展

- C03 已关闭：Workspace/Goal/DoD/WorkspaceTemplate owner 已从 `internal/modules/kernel/workspace` 迁移到 `internal/modules/blocks/workspace`，组件 ID 与 command destination 改为 `blocks/workspace`；topic、具体 command/result、definition revision 与 stale `ActiveRunID` 清理 invariant 保持不变。
- C06 已关闭：Result/Artifact owner 已从 `internal/modules/kernel/result` 迁移到 `internal/modules/blocks/result`，组件 ID 与 command destination 改为 `blocks/result`；topic、具体 command/result、accepted/rejected outcome、artifact logical ref 与 workspace/run scope 合同保持不变。
- C07 已关闭：DecisionTicket/Resolution owner 已从 `internal/modules/kernel/decision` 迁移到 `internal/modules/blocks/decision`；Decision 定义的 evidence command/result 继续由 Orchestrator 订阅，未恢复 EvidenceRecord repository、callback 或 adapter 旁路。
- C08 已关闭：Approval owner 已从 `internal/modules/kernel/approval` 迁移到 `internal/modules/blocks/approval`；Approval 定义的 evidence command/result 继续由 Orchestrator 订阅，未恢复 EvidenceRecord repository、callback 或 adapter 旁路。
- C11 已关闭：MemoryItem/MemoryRecallTrace owner 已从 `internal/modules/kernel/memory` 迁移到 `internal/modules/blocks/memory`；召回分类、provenance、workspace/run scope 与 EventHub command/result 合同保持不变。
- C13 已关闭：Skill registry owner 已从 `internal/modules/kernel/skill` 迁移到 `internal/modules/blocks/skill`；结构化 Skill command/query、bounded metadata、provenance 与引用合同保持不变。
- C14 已关闭：Soul/stage-policy catalog owner 已从 `internal/modules/kernel/soul` 迁移到 `internal/modules/blocks/soul`；System defaults、结构化 constraints/provenance 与 EventHub command/result 合同保持不变。
- C12 已关闭：MCPServer/MCPCapability registry owner 已迁移到 `internal/modules/blocks/mcpregistry`，组件 ID 为 `blocks/mcp-registry`；与 `blocks/mcp` invocation 能力继续只通过 EventHub registry 合同交互。
- C04 已关闭：Run 生产装配不再接受 WorkspaceReader/Orchestrator 注入；workspace read 与 orchestrator command adapters 已下沉为 EventHub-only biz assembly，repository/fake 构造只保留在测试文件。
- C05 已关闭：ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem、ArtifactContinuation 均只由同一 runtime convergence 状态机写入，没有独立生命周期或多写入方；保留为 Orchestrator 内部协调状态，外部读取只走其具体 EventHub 查询合同。
- C09 已关闭：Governance 生产装配不再接受 Decision/Approval/Run/Workspace/Memory readers 或 resolve/resume callbacks；跨组件读写只通过 biz 内部 EventHub adapters，fake ports 仅在测试文件可用。
- C15 已关闭：System 保持多 owner 默认资源启动协调 module，入口显式启用、lifecycle 顺序与三组具体 ensure command/result 均已复核。
- C16-C22 已完成职责复核：七个技术 block 均保持单一技术能力；生产代码不存在 sibling/kernel `biz`、`service` 或 repository 直连。LLM 内部 provider/rendering/budget/cache 继续作为同一调用生命周期下的 focused implementation，不新增 wrapper-only block。
- 当前 22 项已全部 completed；C10 的 projection notification typed 迁移与 legacy 输入删除已完成。
- C10 阶段进展：Eventstream 查询 result 已改为具体递归 `EventRecordData` / `EventValue`，不使用 JSON/reflect 桥；持久化后输出已改为具体 `EventRecordAppendedData`，Server SSE/replay、运行状态缓存、治理续跑 watcher、observed collector、workspace projection manager 与 embedded WorkOrch backend 均直接订阅该合同。
- magicCommon observer 的注册标识实际取 match ID；Eventstream typed notification 因此统一发布到 `/eventstream/consumers/#`，每个消费实例使用唯一 consumer match ID，避免多个 `/#` 订阅互相覆盖。
- C10 输入端已完成：Eventstream 逐 owner/topic 订阅投递组件定义的具体 data，内部投影为 EventRecord，不再消费 Envelope/map owner 通知。
- C10 owner 输入迁移已完成首个切片：`blocks/decision` 的 created/updated/resolved 通知改为 `TicketChangedData` / `TicketResolvedData`，resolution 的开放字段使用递归 `DecisionEvidenceValue`，Governance 与 Eventstream 均直接订阅 Decision 合同；Decision 生产路径已无 Envelope notification 发布或消费。
- C10 第二个 owner 输入切片已完成：`blocks/approval` 的 created/resolved 通知改为 `ApprovalChangedData`，context 使用递归 `ApprovalEvidenceValue`；Governance 与 Eventstream 均直接订阅 Approval 合同，Approval 生产路径已无 Envelope notification 发布或消费。
- C10 第三个 owner 输入切片已完成：`kernel/governance` 的 evaluated/resolved/escalated/blocked 通知改为具体 `GovernanceData`；Eventstream 直接订阅 Governance 合同，Governance 输出生产路径已无 Envelope notification。后续 Run 输入也已完成 typed 迁移。
- C10 第四个输入切片已完成：`kernel/run` 的 created 与状态流转通知统一为 `RunStateChangedData`，生产 Run service 与 EventDrivenDispatch Orchestrator 均直接通过 `/run/consumers/#` 投递；Governance 最后一个 Run Envelope 输入已删除，Eventstream 直接保留实际投递 sourceModule。
- C10 第五个输入切片已完成：Run 的 Step created/dispatched/succeeded/failed/skipped/state-changed 使用 `StepChangedData`，LLM progress/usage 使用 `LLMStreamProgressData` / `LLMUsageRecordedData`；动态摘要与 usage 扩展字段由 Run 自有递归 `NotificationPayload` 表达，Eventstream 只在内部审计投影边界恢复 map。生产 EventDrivenDispatch 路径不再为这些 topic 发布 Envelope。
- C10 第六个输入切片已完成：`blocks/workspace` 的 Workspace created/state-changed/deleted、Goal defined、DoD defined 通知分别使用 `WorkspaceChangedData`、`GoalDefinedData`、`DoDDefinedData`，直接通过 `/workspace/consumers/#` 投递；Eventstream 逐 topic 导入 Workspace owner 合同，生产 Workspace block 已删除 MagicHub/Envelope 通知包装。
- C10 第七个输入切片已完成：Result created/accepted/rejected/superseded 与 Artifact created 分别使用 `ResultChangedData` / `ArtifactCreatedData`；最终由 Result owner command handler 向 `/result/consumers/#` 单点投递，Orchestrator 重复发布已删除。
- C10 第八个输入切片已完成：Memory created/updated/deleted 与 recall recorded 分别使用 `MemoryChangedData` / `MemoryRecallRecordedData`；Memory owner command handler 统一发布，application/Orchestrator 外围重复发布已删除。
- C10 第九个输入切片已完成：Orchestrator Patch previewed/applied/failed/reverted 使用 `PatchChangedData`，ArtifactContinuation created/chunk-recorded/chunk-rejected/assembled/applied/failed 使用 `ArtifactContinuationChangedData`；ContractViolation 后续切片也已完成，无生产来源的 stage/graph/repair/replan topic 已删除。
- C10 第十个输入切片已完成：ContractViolation recorded/retry-requested/escalated 使用 `ContractViolationData`，开放 violation/context 由 Orchestrator 自有递归 `StructuredPayload` 表达，Eventstream 只在内部恢复审计 map。盘点确认 stage_changed/graph_dispatched/repair_started/replan_triggered 没有生产发布点，相关 topic、contract、版本矩阵与测试引用已作为失效代码删除，不新增伪 typed 发布链。
- Eventstream 现统一为 workspace 内持久化事件分配连续 revision；legacy MagicHub Envelope 输入、`SubscribeWorkorch`、`eventRecordFromEnvelope` 与公开 `WorkorchTopics` 聚合 API 已删除，C10 完成。
- Orchestrator notification 兼容层已进一步清零：`eventSubsystem` 不再持有 legacy Hub 或 Envelope fallback，旧 `Published()` 测试均改为 typed observer；Approval created 由 Approval owner 单点发布，MemoryRecall recorded 由 Memory owner upsert handler 单点发布，Orchestrator 生产与测试代码中的 Envelope 发布扫描为零。
- 后续装配兼容清理已完成：activation recall 删除 projection Hub 参数和外围 Envelope；Governance 删除 MagicHub bridge/legacy Hub 字段；Orchestrator module、plugin 与 testkit 删除 projection EventHub dependency，只保留 framework CommandHub。上述路径均通过 owner 定义的具体 command/data/result 交互。
- application projection 后续兼容清理已完成两个切片：`workspaceviewcore.Manager` 直接处理 Eventstream 的 `EventRecordData`，不再执行 `EventRecordData -> Envelope -> handleEvent`；`runtimecore.ManagedRuntime` 只从 typed EventRecord collector 读取已发布事件，旧 projection Hub `Published() []Envelope` 回读与合并去重代码已删除。Workspace projection changed 由 workorchd 定义 `WorkspaceProjectionChangedData` 并通过 CommandHub 投递，Server 直接订阅该具体类型。
- HTTP/SSE 展示与回放也已改用 Server 自有 `streamEvent` DTO；`events.Envelope` 不再存在于 workorchd Server、Workspace projection、Managed runtime、Observed collector/read-model/snapshot 或组件间 EventHub command/data/result 路径。
- Server 的 EventRecord 底层订阅回调已改为具体 `EventRecordData`；run-status cache invalidation 与 governance continuation watcher 直接读取该类型，只有 event-stream 输出分支才进入 stream boundary adapter。EventRecord/WorkspaceProjection 到 Envelope 的函数已明确命名为 stream adapter，并直接从 typed data 构造 HTTP/SSE DTO，不再通过 EventRecord model 中转；失效的 scheduler Envelope accepted-response 校验函数以及 testkit `Published() []Envelope` 透传已删除。
- Observed/read-model 兼容层已关闭：Collector 只保存并深拷贝 Eventstream owner 定义的 `EventRecordData`，ManagedRuntime、read-model readers 与 snapshot projection 继续传递该具体类型并按 topic 调用 payload decoder；`internal/pkg/events` 中已无生产引用的 `WorkorchEvent`、Envelope decoder/normalize 实现及其专属测试已删除。
- Observed snapshot 输出兼容层也已关闭：`SnapshotItem` 直接持有具体 `SnapshotPayload` 并通过 `PayloadMap()` 映射 presenter DTO，`FromEnvelope`、`SnapshotItem.Envelope()`、`EnvelopePayload()` 和只为该桥服务的 snapshot version topic matrix 已删除；snapshot 版本常量归属 Observed focused package。
- Server/CLI backend 展示兼容层已继续关闭：Server SSE 使用本地 `streamEvent` JSON DTO，Eventstream 与 WorkspaceProjection typed data 只在 HTTP 边界映射，不再借用组件 Envelope；embedded backend 只持有 magicCommon CommandHub，直接订阅 `WorkspaceProjectionChangedData` 与 `EventRecordAppendedData`，EventRecord 直接映射 backend Event。旧 projection `events.Hub` 字段、`eventRecordEnvelope`、`convertEnvelope` 以及 `NewEventHub(eventHub, commandHub)` 双 Hub 构造已删除，remote SSE 使用同一 backend 本地 DTO 解码。
- Testkit 最后一处 Envelope wrapper 已关闭：repository fixtures 与 owner module lifecycle 改由 test-only magicCommon CommandHub 持有；后续 projection Hub 装配参数、context API 与 legacy Hub 实现也已全部删除。
- Gap 1/X04 已关闭：appservice runtime session 只校验 application-owned EventHub/CommandHub，26 个 repository fixtures 已迁入 testkit；repository initiator 不再向 application 导出聚合能力。
- Gap 2 已关闭：Goal/DoD 定义变化会清理失效 `ActiveRunID`；activation 仅恢复唯一 active/blocking 非终态 run，多个候选返回 conflict。
- Gap 3 的 appservice/server 测试装配缺口已关闭：测试通过显式 owner lifecycle fixture 启动 7 个 blocks 与 kernel owner modules；`go test ./internal/modules/application/workorchd/biz/appservice -count=1` 和 `go test ./internal/modules/application/workorchd/service/server -count=1` 均通过。
- workorchd scheduler 已关闭 Envelope/map command 遗留：投递方定义并直接发送 `WorkspaceRunRequestedCommand`，消费方返回 `WorkspaceRunRequestedResult`。
- runtime block 链路的代码缺口已关闭：`internal/pkg/runtimeevent` 定义 `ExecuteCommand`/`ExecuteCommandResult` 与 accepted/progress/completed/failed 具体 data；orchestrator 和 7 个 blocks 直接使用 magicCommon `event.Hub` 的 `Send/Post/Subscribe`，不再通过 Envelope 顶层 payload、通用 map 或 JSON 转换交互。
- `internal/initiators/...` 与 `internal/services/workorch/...` 全量测试已通过；services backend 测试已改为显式启动 owner lifecycle，并直接模拟 typed scheduler result。
- 5 个 appservice 子包测试夹具遗留已关闭：`activationports`、`contentports`、`contextregistryports`、`qualityports`、`runports` 已改为具体 owner command/result observer 或显式 owner lifecycle，不再通过 Envelope response 模拟 owner command。
- `GOCACHE=/tmp/workorch-go-build go test -p 1 ./internal/modules/... ./application/workorch ./internal/pkg/... ./internal/services/workorch/... -count=1`、`make fmt-check`、`GOCACHE=/tmp/workorch-go-build make vet` 与 `git diff --check` 已全量通过。默认并行执行曾因大量测试 runtime 同时创建 EventHub/BackgroundRoutine 而在 10 分钟超时；该现象属于测试环境资源饥饿，不是组件合同失败，最终门禁以串行全量结果为准。

Date: 2026-07-09

Related:

- `module-block-contract-closure-validation-plan.md`
- `../../00-core/runtime-stage-contract.md`
- `../../30-module-architecture/llm-provider-assembly.md`

## Purpose

This audit freezes the current registered `module`, runtime `block`, and
`initiator` inventory and checks whether each component still matches its
functional definition. The check is structural and contract-oriented: lifecycle,
owner state, cross-module boundary, and runtime neutrality are more important
than file size.

## Historical Functional Definition Baseline

本节保留 2026-07-09 首轮盘点采用的历史定义，仅用于解释既有结论。当前分类与逐项收口必须以上方“2026-07-10 组件粒度与职责重新审计”为准。

| Kind | Functional definition |
| --- | --- |
| initiator | Process-level infrastructure provider. It may provide repository providers, EventHub wrappers, runtime policy, config loading, route registry, client factory, or background routine. It must not own workspace/run/result business state or business strategy. |
| kernel module | Formal owner of one business or governance state family. It owns repository access for that state, subscribes to owner command/query topics, publishes formal events, and must be the authority for its state transitions. |
| application module | Product/API/application use-case surface. It may assemble routes, projections, command facades, context registry, and configuration surfaces. It must not become the owner of kernel state or repair owner state through projection. |
| runtime block | Runtime capability adapter for technical execution or external systems. It produces normalized execution results/evidence candidates, but must not own formal business state, decide run completion, or mutate coverage/result state directly. |
| process service | Process mode selection and user surfaces under `internal/services/<entry>`. It is not a plugin module and must depend on stable `workorchport` / `workorchclient` contracts rather than owner repositories. |

## Historical Registered Inventory Snapshot

| Kind | ID / component | Code path | Current fit |
| --- | --- | --- | --- |
| initiator | `workorch/runtime` | `internal/initiators/workorchruntime` | Fits initiator role: repository provider, EventHub owner wrapper, runtime policy. |
| application module | `application/workorchd` | `internal/modules/application/workorchd` | Fits application role; borrowed exports are validated at the runtime-session boundary. |
| kernel module | `kernel/workspace` | `internal/modules/kernel/workspace` | Fits owner role for Workspace / Goal / DoD; definition changes clear stale active-run bindings. |
| kernel module | `kernel/run` | `internal/modules/kernel/run` | Fits owner role for Run / Step lifecycle and runtime checkpoints. |
| kernel module | `kernel/orchestrator` | `internal/modules/kernel/orchestrator` | Fits orchestration authority role for convergence, evidence, coverage, repair/replan, and block dispatch. |
| kernel module | `kernel/result` | `internal/modules/kernel/result` | Fits Result / Artifact owner role. |
| kernel module | `kernel/decision` | `internal/modules/kernel/decision` | Fits DecisionTicket / Resolution owner role. |
| kernel module | `kernel/approval` | `internal/modules/kernel/approval` | Fits Approval owner role. |
| kernel module | `kernel/governance` | `internal/modules/kernel/governance` | Fits governance trace / recommendation role. |
| kernel module | `kernel/eventstream` | `internal/modules/kernel/eventstream` | Fits audit event storage/query role; not an owner-state repair authority. |
| kernel module | `kernel/memory` | `internal/modules/kernel/memory` | Fits memory and recall trace owner role. |
| kernel module | `kernel/mcp` | `internal/modules/kernel/mcp` | Fits MCP server/capability registry role. |
| kernel module | `kernel/skill` | `internal/modules/kernel/skill` | Fits skill registry role. |
| kernel module | `kernel/soul` | `internal/modules/kernel/soul` | Fits soul catalog/stage policy metadata role. |
| kernel module | `kernel/system` | `internal/modules/kernel/system` | Fits infrastructure/status role; intentionally narrow. |
| runtime block | `blocks/file` | `internal/modules/blocks/file` | Fits block role; registers file list/read/edit/patch execution kinds. |
| runtime block | `blocks/git` | `internal/modules/blocks/git` | Fits block role; registers git read/write execution kinds. |
| runtime block | `blocks/llm` | `internal/modules/blocks/llm` | Fits LLM provider/context block role; cache-prefix and critical-field contracts remain high-risk and need dedicated tests when changed. |
| runtime block | `blocks/mcp` | `internal/modules/blocks/mcp` | Fits MCP invocation block role. |
| runtime block | `blocks/shell` | `internal/modules/blocks/shell` | Fits shell execution block role. |
| runtime block | `blocks/ssh` | `internal/modules/blocks/ssh` | Fits SSH execution block role. |
| runtime block | `blocks/web` | `internal/modules/blocks/web` | Fits web/search/fetch block role. |
| process/product surface | workorch service / port / client / web | `internal/services/workorch`, `internal/pkg/workorchport`, `internal/pkg/workorchclient`, `web` | Fits process/product surface role; should stay outside `internal/modules`. |

## Closed Gaps

### Gap 1. Borrowed runtime exports were under-validated

- Area: `application/workorchd` runtime session.
- Resolution:
  - `ExportsFromInitiator` now copies `orchestratorRepos.CoverageWorkItems` into `RepositoryExports`.
  - Runtime session construction validates all 26 repository exports and EventHub, returning a named missing-dependency error instead of a downstream generic `invalid argument`.
  - Pairwise tests cover initiator exports -> appservice session assembly.

### Gap 2. Workspace definition-change contract was not fully closed

- Area: `blocks/workspace`（原 `kernel/workspace`）。
- Resolution:
  - Goal/DoD definition changes clear stale `ActiveRunID` when reopening a terminal or superseded workspace.
  - Workspace-state and definition notification counts are aligned with the asynchronous publish contract and covered by regression tests.
  - Activation only restores a unique active/blocking non-terminal run; multiple candidates return conflict.

### Gap 3. Application module tests exposed broad invalid-argument failures

- Area: `application/workorchd/biz/appservice` and `application/workorchd/service/server`.
- Resolution:
  - Appservice and server tests now use an explicit owner lifecycle fixture that starts the required kernel modules and runtime blocks.
  - Owner command tests send concrete owner-defined command types and assert concrete result types; no Envelope response fixture is used for these command paths.
  - `appservice/...`, `service/server`, and the serial `internal/modules/...` suite all pass.

## Boundary Checks

| Check | Current result |
| --- | --- |
| Entry side-effect imports live in `application/workorch/main.go` | Pass. The entry explicitly imports the initiator, application module, kernel modules, and runtime blocks. |
| Runtime blocks own no formal workspace/run/result state | Pass by structure. Blocks register `runtimeevent.Executor` handlers and hold adapters/executors, not owner repositories. |
| Kernel modules expose owner command/query topics | Pass by structure. Kernel modules subscribe to module-specific EventHub topics in `Start`. |
| Application/service code stays out of `internal/modules/kernel/*/biz` direct owner implementation | Pass for production interaction paths. Application/service code uses owner EventHub ports or stable contracts; repository access remains test-fixture setup only. |
| Cross-component command/data/result ownership | Pass for the audited closure paths. Owner or sender packages define concrete message types; consumers import those definitions and use magicCommon EventHub directly. Runtime block and scheduler paths do not use Envelope/map/JSON conversion bridges. |
| Core runtime neutrality | Pass for this change set. Event transport changes add no software-development, provider-specific, or business-domain strategy to orchestrator core. |

## Verification Run

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
GOCACHE=/tmp/workorch-go-build go test -p 1 ./internal/modules/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/initiators/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/services/workorch/... -count=1
```

Result: all pass on 2026-07-10.

The default parallel modules suite previously exceeded 10 minutes because many test runtimes created EventHub and BackgroundRoutine instances concurrently. The corresponding tests pass individually and the serial full suite passes, so closure uses the serial full-suite result as the deterministic gate.

## Final Conclusion

- 2026-07-10 之前识别的 EventHub typed command/result、Envelope/map/JSON 兼容层和三项功能缺口已经关闭；这些完成项继续有效。
- 按新的 module/block/initiator 定义，C01-C22 与 X01-X05 已全部完成，本文总状态为 `completed`。
- 当前目标分类为 6 个 module、15 个 block，以及已收窄完成的 `workorch/repositories` initiator。
- Decision/Approval evidence repository 直连、System 多 owner repository 直连、application repository exports、LLM block 间静态依赖、目录迁移与 EventHub 兼容层均已关闭。
- 当前可以认定审计涉及的 22 个组件已按职责定义完成收口；后续变更仍需持续遵守本文完成门禁。
