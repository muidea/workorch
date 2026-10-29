# MCP 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/mcp` 是 MCP tool、resource 与 prompt operation 的唯一 execution owner。它读取 MCPRegistry owner 发布的单一一致
snapshot，执行 owner-private HTTP/stdio transport/session，并把结果规范化为有界 `RuntimeOperationReceipt`。

本组件不修改 Registry，不拥有 Runtime aggregate，也不选择 focus、action、CWI credit、retry、governance resolution 或 terminal。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → MCP | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`QueryRuntimeCWIReconciliation`、`ExecuteOperation` Command/Result |
| Orchestrator | MCP → Orchestrator | exact receipt query/commit；execute 只消费 sealed operation、catalog identity 与 grant |
| MCPRegistry owner | MCP → Registry | `ReadRegistrySnapshotCommand/Result` 一次读取同 revision Server/Capability aggregate；静态单 server probe 使用 `GetServerCommand/Result` |
| Workspace | Workspace → MCP | `ObservePreparationCommand/Result` 与 `QueryPreparationCommand/Result`；独立于 Runtime 六入口，仅 Workspace producer |
| MCP server | MCP-private | bounded HTTP/stdio tool/resource/prompt invocation、session、evidence 与 effect observation |

Callable capability 与 validation surface 是不同 fact：只有 owner 明确声明、输入 binding 完整、non-durable 且 replay-safe 的
`ready + direct` surface 才可作为 validation recipe；其他 surface 仍保留 typed readiness/audit fact。

## 不变量与非职责

- SM-2 capability/admissibility/feasibility/reconciliation 查询不得调用 MCP endpoint、启动 stdio process 或创建 receipt。
- execute 前必须验证 Orchestrator producer、完整 sealed payload/scope、Registry/catalog revision、Capability allowlist/schema、effect/
  idempotency policy、execution key/fence、receipt authority 和 exact grant。
- Registry 只通过 typed EventHub bridge 读取；MCP 不注入 Registry service/repository，也不拼接两次 list 形成伪 snapshot。
- environment metadata 只允许 `env://NAME` 或受支持 workspace placeholder；secret 只在 stdio 启动前 owner-local 解析。
- HTTP/stdio response 和 stream/event 数量必须有界；endpoint、credential、stderr、raw provider error/body 不进入 aggregate。
- non-durable capability 不得报告 changed；durable capability 必须返回与 sealed policy 一致的 before/after effect observation。
- direct failure-target observation 只能来自 bounded owner response evidence，不能从 operation target、input 或自由文本推断。
- at-most-once/unknown effect 在 receipt 丢失时不得自动重发；已有 exact receipt 只能重放相同 evidence transaction。
- 全部六个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得查询 Registry、启动 session、调用 endpoint 或访问 receipt。

### 生命周期与排空合同

- `Start` 通过 Base checked SimpleObserver API 逐成功 topic 记账；第 N 个失败保留同一 owner 与成功订阅。Start 关闭后不可重新打开代次。
- `BeginShutdown` 关闭 fresh root 准入；每个实际 EventHub handler 从准入到真实返回都持有 owner Lifecycle 票。cancel/调用者等待超时不释放票，因此子进程等待和 nested receipt commit 都留在实际执行 handler 的排空范围。
- Execute 的显式 drain 只做 sealed command 校验与 exact persisted receipt 查询/重放；未命中必须另取 fresh 票，不能借 drain 创建新 effect、governance receipt 或重新执行。其他 capability/admissibility/feasibility/reconciliation 查询在关门后拒绝 fresh root。
- `Quiesce(ctx)` 等待本 owner 真实在途归零；超时保留 adapter、订阅和同一 owner 引用，可使用独立预算重试。局部归零不封闭合法 drain，最终 Teardown 仍必须由应用共享 Hub/任务全局屏障后的调用方授权，不能把退订当作排空。
- `Biz.Teardown(ctx)` 返回 error，每项退订真实成功才 pop，失败或取消保留未释放依赖。`Module.Stop` 成功才清空 owner；public void `Teardown` 的失败交给 framework guard。生命周期配置/最终释放调用由调用方串行化，不持有跨 Hub action 的 owner 锁。
- MCP idle timer 调度前计数，关门阻止再调度；只有 `Timer.Stop()==true` 才直接释放计数，已开始 eviction 必须等待 session Close 与 callback 真实退出。adapter `CloseChecked` 仅发起一次真实关闭，预算超时保留 completion/session，重试不重复关闭。
- `OwnerQuiescenceWaitsForRealHandlerAndRetainsRetryDependencies`、`OwnerQuiescenceExplicitReceiptDrainStillWaitsForCompletion` 与 ResourceLedger/Module tests 验证本地阻塞、取消、关门、retry 和 partial Start；这些测试不证明全进程所有 owner 已适配，也不改变 SM-1～SM-4 业务合同。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | HTTP/stdio tool、resource、prompt tests；`TestMCPOwnerReadsTypedRegistrySnapshotThroughProductionEventBridge` | 已覆盖三类 operation 与生产 MCP↔MCPRegistry typed snapshot/admissibility 路径 |
| invalid + scope | transport/schema/redirect/size/secret、seal/catalog/capability/grant drift、`TestMCPRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要 owner/Registry/input/transport，并穷举六 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestMCPOwnerEmitsStructuredFailureTargetObservationThroughTypedReceipt`、owner execute duplicate/receipt tests及 stdio session reuse | 覆盖同 owner 与 fresh owner persisted receipt 重放、零二次 provider invocation 与 session reuse |
| stale + freshness | catalog drift/unavailable feasibility、execute registry drift tests | 已覆盖 Registry/catalog freshness |
| restart / owner 重建 | `TestMCPOwnerEmitsStructuredFailureTargetObservationThroughTypedReceipt` | fresh owner 查询 persisted receipt 后零二次 provider invocation |
| lifecycle | `TestMCPModuleIdentityConfigureStartStopContract`、`TestMCPOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestMCPOwnerLifecycleCoversEveryRuntimeTopic`、`TestMCPOwnerTeardownTerminatesLongLivedStdioSession`、stdio reuse/idle eviction/workspace isolation tests | 覆盖全部六 topic 的重复 Start/Stop fence，并证明 owner Teardown 终止长期 stdio session |
| effect + receipt | non-durable/durable changed/no-op、failure target/evidence persistence、resource/prompt owner receipts、`TestMCPOwnerCommitsUnknownEffectReceiptForStdioObservationAndTimeoutFailures` | 覆盖主要 effect/receipt，以及 stdio durable observation 缺失和 transport timeout 的 rejected `effect=unknown` receipt |
| boundary | capability EventHub、Registry admissibility bridge、`TestMCPRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、semantic proof、`TestMCPOwnerReconciliationPreservesDeclaredRecipeAsUnverifiableWithoutGrantingCredit` 与 seal/authority tests | 覆盖六 topic 映射；catalog 声明的 executable recipe 被保留为 target/check fact，但缺少 behavior probe 时 reconciliation 明确为 `unverifiable`，不授予 CWI credit |
| deployed MCP interoperability | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md)、[Deployed Convergence](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) | 组件级未覆盖；真实 MCP server protocol/version、process environment、credential 与远端行为仅由显式 deployed/live gate 验收，不能计入本地组件单测完成度 |

## 激活前 preparation 协议观察

`/mcp/preparation/observe` 只接收 Workspace owner 的已接纳原意图：Workspace/request/payload/snapshot/revision、
目录、精确 Registry revision、选中 server 与最小 capability 身份。MCP 自行重读完整 Registry、解析自己的启动参数，
不接受跨 owner endpoint/env/凭据，不授予 Runtime grant 或 CWI credit。
`workorch-mcp-preparation-observation/v1` 只支持 stdio：initialize、notifications/initialized、所需类别的
 tools/list、resources/list、prompts/list，然后关闭。零业务调用；必须正常退出、完整列出已选能力，分页未完不授成功。
非 stdio 返回 transport_unsupported；总 10 秒、每进程 8 秒、最多 8 server、每 server 原始 frames/stderr 512 KiB。
关闭等待 500ms 后 kill/wait，失败明确保留，超限明确不完整。原始 bytes 不清洗或摘要，受限 Workspace 证据保存。
观察包含 owner epoch、session ID、真实 PID、时间及 Registry 前后精确身份。

本进程最多保存 128 份原观察，精确重复返回 detached 原值，异 payload 拒绝；不逐出记录后重新执行。
`/mcp/preparation/query-original` 只查原 Workspace/request/payload；未找到或 owner 重建后返回 unknown，
绝不启动进程或推断未执行。两个入口均 strong call、精确 Workspace producer、Lifecycle fresh 准入与真实排空；
不放宽原六 Runtime ingress 的 Orchestrator producer、sealed operation 或 receipt 门禁。
