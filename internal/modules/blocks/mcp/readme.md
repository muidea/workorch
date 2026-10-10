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

## 本地生命周期资源合同

- `Start` 只在 SimpleObserver/Hub 实际完成订阅后逐 topic 记账；第 N 个订阅失败返回原 checked error，已成功资源和 owner 引用保留，可检查式清理或从未完成 topic 重试。
- `Biz.Teardown(ctx)` 返回 error，按账本逆序退订，每次成功才移除该项；失败或 context 取消保留剩余订阅与 adapter/session。全部退订实际完成且 context 有效后才调用 adapter `Close`；成功后清空引用，重复清理不再次关闭 adapter。`Module.Stop` 只有完整清理成功才清空同一 owner 引用，plugin 的 void `Teardown(ctx)` 将错误交给 framework guard。
- 生命周期调用由调用方串行化；账本不持有跨 Hub control action 的 owner 锁，也不替代输入停止与在途 handler/session 排空。最终 adapter 关闭的前提是调用方已完成真实排空，退订完成不等于 quiescence。本组件尚未提供全局停机屏障或阶段准入 guard；adapter 的既有无 context `Close` 不能被此账本变成有预算的排空协议。
- `TestMCPOwnerResourceLedgerEveryStartFailureCanTeardown`、`TestMCPOwnerResourceLedgerEveryTeardownFailureRetainsDependenciesAndRetries`、`TestMCPOwnerResourceLedgerCancellationRetainsUnreleasedResources` 及 Module resource-ledger tests 覆盖逐 N 故障、取消、retry 和同 owner 引用保留；这些测试不是全局生命周期完成证明。

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
