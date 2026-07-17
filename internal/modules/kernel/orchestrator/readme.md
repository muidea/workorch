# 运行时编排组件

## 功能说明

`kernel/orchestrator` 是 Runtime V2 convergence authority，负责 typed facts、authority transaction、prerequisite、candidate、sealed dispatch、operation evidence、governance wait 与 terminal projection。它组合多个 owner 和技术 block 完成同一 Run 的确定性收敛闭环。

## Provider / model ceiling（2026-07-15）

- activation 持久化 `RuntimeActivationContext.ProviderTarget`
- provider ceiling prerequisite 请求与封存完整 `ProviderCeiling{Target, token/byte ceilings, revisions}`
- candidate 强制使用 ceiling target；Restart/Resume 不携带 provider/model patch
- Orchestrator 只校验 identity/revision/ceiling 完整性，不解释 ai-proxy 或模型品牌

## Runtime V2 四状态机实现约束（2026-07-16）

完整规范以 `docs/00-core/runtime-stage-contract.md` 的“Runtime V2 四状态机固化规约”为唯一权威。本模块按以下四个状态机落实现有职责，不定义第二份状态语义：

| 状态机 | 本模块职责 | 外部 owner |
| --- | --- | --- |
| SM-1 Run lifecycle | Start/Resume 准入、blocked/terminal 窄投影 | Run owner 持久化 state/aggregate/lineage；Workspace owner 持久化 activeRun |
| SM-2 authority/focus | reducer、focus/evidence/assessment/repair/governance/terminal 裁决 | application 只编译 typed bootstrap；Result 只存 outcome |
| SM-3 candidate/dispatch | attempt 0/1、transaction、seal、typed operation、evidence commit | LLM 返回 candidate；capability block 执行 operation |
| SM-4 drive/recovery/projection delivery | bounded drive、singleflight、restart load、owner projection retry | projection store/read model 只保存派生状态 |

实现必须保持：

- SM-1 不选择 action，SM-2 不执行 operation，SM-3 不选择 next action，SM-4 不重建 authority。
- 所有跨状态机推进都使用 persisted aggregate + typed EventHub command/result。
- projection failure 不得覆盖 reducer terminal；重启不得重复 prerequisite/candidate/side effect。
- 任何修改 reducer/controller/drive/operation projection 前，必须说明影响的 SM 编号并补合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试。
- 本模块不得恢复 V1 mainline、application continuation queue、通用 RuntimeAdapter registry、projection authority 或自动 successor。


## 组件交互及合同

- 通过 EventHub 读取 `blocks/workspace` 和 `kernel/run` 的权威快照，并通过 Run owner command 推进 Run/Step/Checkpoint。
- Runtime V2 通过 Orchestrator 自有 `ExecuteRuntimeOperationCommand/Result` 调用 File、Git、Shell、MCP、Web 和 SSH block；operation command/result 使用具体类型，同步返回的 typed result 是提交 execution evidence 的唯一执行回执。
- 通过 EventHub 调用 Result、Decision、Approval 和 Memory owner 的具体 command；不得直接持有这些 owner 的 service 或 repository。
- 与 `kernel/governance` 协作处理治理策略、decision/approval blocker 和恢复，但 Orchestrator 保持 runtime contract gate 权威。
- Decision 与 Approval 使用各自 `pkg/events` 定义 evidence command；Orchestrator 幂等写入自己的 EvidenceRecord 并返回具体 result。
- Orchestrator 自有 Runtime V2 drive、prerequisite、candidate、operation、governance resolution、retirement resource release与只读查询合同定义在 `kernel/orchestrator/pkg/events`。
- Runtime V2 prerequisite command/result 由 Orchestrator 在 `kernel/orchestrator/pkg/events` 定义；当前 target snapshot acquisition 通过 EventHub 交给 File block，并将具体 `TargetSnapshot` 提交回 typed runtime facts。
- Runtime V2 candidate command/result 同样由 Orchestrator 定义；请求封存完整 authority transaction，LLM block 返回的 candidate 必须绑定同一 snapshot，transaction 漂移不得进入执行。
- candidate request 携带从 persisted facts 构造的 typed `RuntimeCandidateContext`，明确当前 focus、accepted/rejected evidence、failure、capabilities、manifest、target snapshot、validation scratch 与 provider ceiling；LLM block 不从 V1 map 或临时上下文重建 authority。
- Runtime V2 dispatch 在执行前持久化 `SealedDispatch`、完整 transaction、candidate hash 和 typed step input；execution evidence 只有在 snapshot、dispatch 与 operation 一致时才能提交并触发下一次 reducer。
- Runtime V2 aggregate 通过 Run owner 的 EventHub update 合同持久化；Orchestrator 使用 expected revision 执行 CAS，不直接访问 Run repository，也不把 aggregate 编码进 `map[string]any`。
- Runtime V2 初始化时封存 typed Goal/DoD definition context；后续 candidate request 与重启恢复只能读取 Run owner 中持久化的定义快照，不能由调用方临时重建。
- `InitializeRuntimeV2FromRun` 只读取 Run owner 已持久化的 typed bootstrap，生成 initial facts 并执行 preflight/reducer；Orchestrator 不解析 V1 execution context map 来重建定义或 focus。
- Runtime V2 controller 统一负责 initialize、prerequisite commit、candidate seal 和 evidence commit；服务重启只加载既有 aggregate，不得因重复 Start/Resume 再次执行 reducer 或增加 attempt。
- prerequisite mainline 按 typed requirement 的 owner module 调用 EventHub，并在每个结果提交后原子执行 reducer；多个 capability owner 的结果必须合并，不能互相覆盖。
- sealed candidate 通过 `ExecuteRuntimeOperationCommand/Result` 按 operation kind 投递到 File、Git、Shell、MCP、Web 或 SSH owner；跨组件 command/result 只包含具体 operation、transaction、status、failure 和 evidence 类型。
- 多 operation dispatch 按 dependency graph 确定性执行；每个 operation 独立提交 evidence，全部成功或任一失败后才结束 dispatch 并再次 reducer。
- RuntimeFacts 持久化当前 focus 与有序 remaining focus queue；当前 focus 的 required evidence 满足后，evidence commit 原子切换到下一 focus，再调用同一 reducer。
- 显式声明 `RequireAssessment` 的 focus 在执行证据齐备后进入 typed `need_assessment` authority；LLM 只能返回绑定当前 focus/coverage 的 `AssessmentCandidate`，Orchestrator 校验其 evidence refs 来自当前 Run 的 accepted evidence 后写入 assessment ledger。
- 最后一个 assessment 关闭后，Orchestrator 通过 Result owner 定义的具体 EventHub create command 写入 accepted/rejected DoD Result，再投影 Run/Workspace terminal；Result block 只持久化正式 outcome，不参与 acceptance 裁决。
- CoverageFocus 可携带 evidence role、accepted/rejected step kinds 与 accepted operations；reducer 只在 owner 已确认的 capability kinds 中做中立交集/排除，并把结果封入 `RequiredOutputContract`。若声明 accepted operations，candidate 至少包含一个匹配的具体 operation 参数，不能用同 step kind 的其他动作替代。
- candidate attempt 0/1 的 pending、failed、accepted 状态随 aggregate 持久化；attempt 1 必须复用相同 transaction 和 stable prefix，第二次失败后只能进入 governance 或 terminal。
- governance candidate 作为 sealed dispatch 持久化；治理结果通过 `CommitRuntimeGovernanceResolutionCommand/Result` typed owner command 提交，同一 resolution ID 与 authority snapshot 最多消费一次。
- Decision owner 可通过 `CommitRuntimeGovernanceTicketResolutionCommand` 只提交 ticket resolution；Orchestrator 从 persisted binding 补齐 authority snapshot、dispatch 与 dependency 后进入同一 resolution commit。
- governed dispatch 通过 `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner 创建确定性 ticket；ticket、snapshot、dispatch 与 dependency 的绑定随 Runtime V2 aggregate 持久化。
- `DriveRuntimeV2Command/Result` 是 Runtime V2 唯一同步推进合同；Orchestrator 在有界循环内依次处理 prerequisite、candidate、sealed dispatch、typed operation、evidence/reducer，并只返回 `governed_wait` 或 typed terminal。重复调用和进程重启必须从 Run owner aggregate 幂等恢复。
- 进程内后台 drive 按 workspace/run scope 使用 singleflight gate；重复 Start/Resume/governance continuation 只复用当前 drive，调度失败或 drive 结束必须释放 gate，不得用并发 CAS conflict 生成伪 `runtime_v2_drive_failed` terminal。
- reducer/aggregate 已进入 governed wait 或 terminal 后，后续 owner projection 失败只能记录 projection error 并等待幂等重试，不得把已确定的 complete/fail authority 改写成 `runtime_v2_drive_failed`。
- Start command 先原子投影 pending Run 为 active，再由 Orchestrator 后台执行唯一 Runtime V2 drive；Resume command 只携带 workspace/run scope，只允许 active 或已解决 blocker Run，并进入同一 drive。application 不再拥有 Start/Resume continuation queue，Orchestrator service 不再暴露 execution-context-patch resume facade。
- Orchestrator biz 只保留 Start 状态准入 `AcceptRunStart`、Resume 状态准入 `CanResumeRun` 与 V2 drive 所需 blocker/terminal 投影；不存在可直接调用的 V1 `StartRun`、`DispatchNextStep`、`ResumeRun`、patch resume 或自动 replan lifecycle。
- V1 runtime progress/terminal-result checkpoint、restart replay、decision resume subsystem 和 approval patch resume 已删除；Runtime V2 重启只加载 persisted aggregate，治理继续只消费 typed resolution。
- Orchestrator plugin 不再创建 RuntimeAdapter registry、EventHub runtime adapter 或 Step kind registration matrix；V2 operation 只通过各组件 typed EventHub command/result 执行。
- Orchestrator 不再暴露 V1 `NextActionForRun` 或接收 `UpdateRuntimePolicyCommand`；Runtime V2 action 只来自 typed reducer，动态策略从 canonical configuration source 按需读取。
- 已提供 Runtime V2 blocker→active 与 typed terminal 的窄 Run/Workspace 投影能力；正式入口切换后不得借此创建 V1 Step 或经过 V1 Result/DoD completion gate。
- Run/Workspace 窄投影必须可重试收敛：active 重放要修复缺失的 workspace active-run 投影；terminal 投影先释放 workspace 再写 Run 终态，Run 已终态时仍要重试 workspace release，避免单边成功后永久悬挂。
- Eventstream 订阅仍在生产发布的 Orchestrator、Run 和 Result typed notification 形成审计记录；Runtime V2 只消费同步 typed operation result 推进状态。
- workspace/run purge 时，Orchestrator 只删除自身 ToolCall/Patch/ValidationResult/Evidence/Coverage/ArtifactContinuation/MemoryTrace；Step 与 RuntimeCheckpoint 必须通过带 WorkspaceID/RunID 的 Run owner purge command 删除，Orchestrator 不持有其 repository。
- Runtime V1 retirement resource release 使用 Orchestrator 自有 typed command，关闭 pending/executing Step、ToolCall、ArtifactContinuation，并释放 managed validation resource；重复调用只返回本次实际变化数量。

## 相关约束

- 核心 runtime 必须保持领域中立，不得写死软件开发、语言栈、测试框架、MCP 或业务领域逻辑。
- 每次 dispatch 必须基于一致的 workspace/run/definition/evidence 快照。
- stale evidence 只能进入 diagnostic lineage，duplicate event 必须幂等；运行重启后从持久化 aggregate 恢复，不允许依赖进程内去重 map。
- execution evidence 必须携带来自 sealed operation 的 coverage work item IDs；前序 focus evidence 不得满足后续 focus，最后一个 focus 满足后才允许 Complete。
- profile-specific accepted/rejected kinds 只能作为 typed focus requirement 输入；reducer 不识别 profile ID、deliverable ID、软件开发术语或具体业务语义。
- `runtimeContractVersion=v2` 仅为审计字段；正式状态以 typed `RuntimeStateRecord` 为准，不得作为 V1/V2 路由开关。
- authority attempt 只有在 state、action、focus、definition/evidence revision 和 requiredOutput hash 全部相同时才增加；prerequisite 或其他事实推进必须重置 attempt。
- block adapter 的既有 map 输入只能存在于消费 block 内部的技术适配边界，不得重新进入 EventHub command/result、RuntimeFacts、candidate、dispatch 或 evidence。
- accepted candidate attempt 必须对应 active 或 completed sealed dispatch；不存在 dispatch 的 accepted attempt、第三次请求或 accepted 后重试都必须被 invariant validation 拒绝。
- drive loop 的每个非等待、非终态 transition 都必须增加 persisted state revision；无持久化进展或超过 transition guard 必须 fail-fast，禁止进程内空转。
- Start/Resume 生产入口只进入 Runtime V2；缺少 typed bootstrap 必须失败并投影 typed terminal，禁止以 `RuntimeBootstrap == nil` 为条件保留兼容路由。
- `ResumeRunCommand` 不得重新增加 loaded Run、`ExecutionContextPatch` 或其他 V1 continuation payload；上下文变化只能通过创建新的 Runtime V2 activation/bootstrap 表达。
- Orchestrator service facade 不暴露 `DispatchNextStep`；V2 operation 只能由 persisted drive/reducer 主链调度，外部 module 不得直接推进 legacy Step。
- Orchestrator service facade 不暴露 Start/Resume；这两个生产 EventHub handler 直接进入 Runtime V2 activation/drive，禁止外部组件通过 service lifecycle 重新进入 V1 主链。
- Runtime V2 不自动创建 V1 replan successor；重新定义目标或执行基准时必须通过新的 typed activation/bootstrap 创建独立 V2 Run。
- 不记录或恢复 `runtime_step_result_available`、watchdog recovery policy 等 V1 checkpoint；历史 checkpoint 仅作为 retirement 后只读审计数据。
- Run/Workspace 状态通知不再隐式写入 V1 runtime checkpoint；状态事件与 Runtime V2 aggregate 各自保持单一职责。
- HTTP runtime 配置更新不得向 Orchestrator 推送进程内 repair policy 副本，避免 canonical config 与 V1 policy cache 形成双权威。
- Orchestrator 不注册 ValidateRun/CompleteRun command；validation 只能提交 sealed operation evidence，completed terminal 只能来自 reducer decision 与 typed owner projection。
- V1 `EnableEventDrivenDispatch`、`DispatchStepRequested`、通用 runtime executor 与 runtime-gap recovery command 已删除；组件执行不得形成第二条 dispatch owner。
- V1 `StagePacket`、`NextAction`、RuntimeStep Accepted/Progress/Completed/Failed 通用事件结构已删除；不得以历史合同重新包装 Runtime V2 facts、candidate 或 operation evidence。
- retirement resource command 必须拒绝任何带 Runtime V2 bootstrap/state 或 V2 contract 的 Run；历史 evidence、terminal Step、completed ToolCall 和已应用 ArtifactContinuation 不得被修改。
- governance continue 必须写入 accepted governance evidence、递增 evidence revision 并回到同一 reducer；governance fail 直接生成 `governance_rejected` terminal，stale resolution 只能进入 diagnostic lineage。
- resolution 必须携带并匹配已绑定 ticket；不存在绑定、同一 dispatch 绑定不同 ticket、或 dependency 漂移都必须返回 conflict。
- `runtime_v2_governance` resolution committed 后由 Orchestrator 自行投影 terminal 或调度 drive continuation；application resolved watcher 不得再次发送 Resume。
- 相同 governance resolution 的 duplicate commit 必须重试幂等 continuation/projection；singleflight gate 负责与仍在执行的 drive 合并，避免“aggregate 已提交但 Run 仍 blocked”的单边状态。
- definition revision、Goal/DoD identity、正文或 capability contract 任一漂移都必须返回 conflict；definition context revision 必须与 facts 和 authority transaction 一致。
- LLM dispatch 前必须先完成 manifest、target snapshot、failure target、capability、validation scratch 和 provider ceiling preflight；缺失资源只能进入 deterministic `AcquirePrerequisite`，不得混入 LLM state-changing requiredOutput。
- block 输出只是 candidate；evidence quality、coverage binding、next action 和 accepted Result 只能由 Orchestrator 判定。
- assessment-only focus 不允许 executable operation；没有 current-run accepted evidence 的 satisfied assessment 必须被拒绝，不能用 summary、历史 memory 或自造 ref 关闭 coverage。
- ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 和 ArtifactContinuation 只属于 Orchestrator 的单一收敛状态机，不对外暴露写入 repository。
- 跨组件交互只能使用 EventHub 与具体 command/data/result；禁止跨 owner service/repository、Envelope、JSON/reflect 类型桥和通用投递包装。
- provider-facing LLM cache prefix 必须遵守 C0、C1、C2、C3-C5 稳定分层。
