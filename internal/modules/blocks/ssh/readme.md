# SSH 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/ssh` 是 `ssh/command` 的 capability owner。它根据 owner-private target/session/credential snapshot 建立受控 SSH
调用，在命令前后执行远端 observation，并将实际 remote effect 规范化为 `RuntimeOperationReceipt`。

EventHub operation 只携带 opaque `targetRef/sessionRef/credentialRef` 和 sealed command；host、username、password、private
key/path、known-hosts 内容及环境 secret 不跨 owner。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → SSH | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`ExecuteOperation` Command/Result |
| Orchestrator | SSH → Orchestrator | exact receipt query/commit；远端访问前要求 sealed approval grant |
| SSH target | SSH-private | strict host identity、本地 ssh child、owner-resolved credential、before/after observation |

`ssh/command` 的 owner policy 固定为 `remote_durable + at_most_once`。配置 revision 只包含 frozen policy 和
secret/host identity hash，不包含明文。

## 不变量与非职责

- target、session、credential 必须来自同一 owner catalog binding；operation 不接受 raw endpoint、username 或 secret。
- host-key verification 必须保持 strict；禁用用户 SSH config，不允许 option-shaped endpoint 或配置路径逃逸。
- 远端命令必须先取得 exact approval；grant 必须绑定 command hash、target/session/credential、execution key 与 next fence。
- 命令成功或失败都必须以后置 observation 判断 changed；失败且 changed 仍报告 `remote_durable`，观察失败报告 unknown。
- timeout/cancel 必须终止本地 ssh child；普通 SSH 连接不能证明远端后台进程终止，因此本组件不承诺该语义。
- receipt 已提交后的重复调用只能重放 exact receipt；未知 side-effect outcome 不得自动重发 at-most-once command。
- 全部五个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得解析 secret、建立连接或访问 receipt authority。
- SSH 不拥有 Runtime action、retry、governance resolution、CWI credit 或 terminal 状态。

### 生命周期与排空合同

- `Start` 通过 Base checked SimpleObserver API 逐成功 topic 记账；第 N 个失败保留同一 owner 与成功订阅。Start 关闭后不可重新打开代次。
- `BeginShutdown` 关闭 fresh root 准入；每个实际 EventHub handler 从准入到真实返回都持有 owner Lifecycle 票。cancel/调用者等待超时不释放票，因此子进程等待和 nested receipt commit 都留在实际执行 handler 的排空范围。
- Execute 的显式 drain 只做 sealed command 校验与 exact persisted receipt 查询/重放；未命中必须另取 fresh 票，不能借 drain 创建新 effect、governance receipt 或重新执行。其他 capability/admissibility/feasibility/reconciliation 查询在关门后拒绝 fresh root。
- `Quiesce(ctx)` 等待本 owner 真实在途归零；超时保留 adapter、订阅和同一 owner 引用，可使用独立预算重试。局部归零不封闭合法 drain，最终 Teardown 仍必须由应用共享 Hub/任务全局屏障后的调用方授权，不能把退订当作排空。
- `Biz.Teardown(ctx)` 返回 error，每项退订真实成功才 pop，失败或取消保留未释放依赖。`Module.Stop` 成功才清空 owner；public void `Teardown` 的失败交给 framework guard。生命周期配置/最终释放调用由调用方串行化，不持有跨 Hub action 的 owner 锁。
- SSH adapter `CloseChecked` 使用一次真实 credential-memory 清理 completion；预算超时保留同一事实并可重试。不承诺远端后台进程停止。
- `OwnerQuiescenceWaitsForRealHandlerAndRetainsRetryDependencies`、`OwnerQuiescenceExplicitReceiptDrainStillWaitsForCompletion` 与 ResourceLedger/Module tests 验证本地阻塞、取消、关门、retry 和 partial Start；这些测试不证明全进程所有 owner 已适配，也不改变 SM-1～SM-4 业务合同。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | `TestRuntimeAdapterExecutesPasswordSSHWithStrictHostIdentityAndObservation`、`TestRuntimeAdapterExecutesPrivateKeyWithoutExposingConfiguredPath`、`TestRuntimeAdapterBoundsAndHashesFullSSHCommandOutput` | 通过 PATH 注入 fake ssh/setsid 覆盖 argv、credential staging、observation 与 output bound |
| invalid + scope | config path/permission、owner ref、seal/catalog/grant drift、`TestSSHRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要 secret/scope/authorization，并穷举五 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestSSHOwnerRequiresExactApprovalBeforeNetworkAndCommitsObservedChange`、`TestSSHOwnerCommitsUnknownEffectReceiptWhenRemoteObservationFails` | 覆盖同 owner 与 fresh owner 的 exact receipt replay，以及 observation failure receipt 的零二次远端调用 |
| stale + freshness | admissibility/feasibility/semantic-proof catalog stale tests | 已覆盖 owner policy revision drift |
| restart / owner 重建 | `TestSSHOwnerRequiresExactApprovalBeforeNetworkAndCommitsObservedChange` | fresh owner 查询 persisted receipt 后零二次远端调用 |
| lifecycle | `TestSSHModuleIdentityConfigureStartStopContract`、`TestSSHOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestSSHOwnerLifecycleCoversEveryRuntimeTopic`、`TestSSHOwnerRequiresExactApprovalBeforeNetworkAndCommitsObservedChange` | 覆盖全部五 topic 的重复 Start/Stop fence，并断言 Teardown 清空 target/session/credential memory |
| timeout / cancellation | `TestRuntimeAdapterTimeoutAndCancellationTerminateLocalSSHAndRemoveCredentialMaterial` | 覆盖本地 ssh child 的 deadline/cancel 终止、typed timeout/unknown-effect fact 与临时 password material 清理；按合同不承诺远端后台进程终止 |
| effect + receipt | `TestSSHOwnerCommitsFailedCommandWithObservedEffectClassification`、`TestSSHOwnerCommitsUnknownEffectReceiptWhenRemoteObservationFails`、成功 changed/no-op receipt tests | 覆盖成功、命令失败 changed/no-change，以及 before/after observation failure 的 rejected `effect=unknown` receipt |
| boundary | `TestSSHOwnerAcquiresCapabilitiesThroughTypedEventHubContract`、`TestSSHRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、typed operation/seal 及 admissibility/feasibility/semantic EventHub tests | 已覆盖 capability 正常 Send，以及五个订阅 topic 的 exact producer/strong-call/concrete payload 映射 |
| deployed SSH interoperability | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md) | 组件级未覆盖；真实 SSH handshake/auth/host-key/network 与远程环境必须由显式 deployed/live gate 验收；当前 acceptance inventory 无 SSH-specific producer，因此本项仍是部署缺口，不能计入本地组件单测完成度 |
