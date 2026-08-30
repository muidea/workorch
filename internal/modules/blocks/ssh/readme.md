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

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | `TestRuntimeAdapterExecutesPasswordSSHWithStrictHostIdentityAndObservation`、`TestRuntimeAdapterExecutesPrivateKeyWithoutExposingConfiguredPath`、`TestRuntimeAdapterBoundsAndHashesFullSSHCommandOutput` | 通过 PATH 注入 fake ssh/setsid 覆盖 argv、credential staging、observation 与 output bound；不证明真实 SSH handshake/auth/host-key/network 互操作 |
| invalid + scope | config path/permission、owner ref、seal/catalog/grant drift、`TestSSHRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要 secret/scope/authorization，并穷举五 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestSSHOwnerRequiresExactApprovalBeforeNetworkAndCommitsObservedChange` | 已覆盖同 owner 实例的 exact receipt replay 与零二次远端调用 |
| stale + freshness | admissibility/feasibility/semantic-proof catalog stale tests | 已覆盖 owner policy revision drift |
| restart / owner 重建 | 无直接组件级自动化证据 | 未覆盖新 SSH owner 查询 persisted receipt 后零二次远端调用 |
| lifecycle | `TestSSHModuleIdentityConfigureStartStopContract`、`TestSSHOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery` | 仅以 `AcquireRuntimeCapabilities` 代表 topic 覆盖 started fence、重复 Start、Stop 后零投递、重复 Stop 与 fresh-owner restart；不是五 topic lifecycle 矩阵，credential memory 清空和临时 secret 清理也无直接断言 |
| timeout / cancellation | 无直接组件级自动化证据 | 未覆盖本地 ssh child 的 deadline/cancel 终止、timeout fact 与临时 credential material 清理；更不承诺远端后台进程终止 |
| effect + receipt | `TestSSHOwnerCommitsFailedCommandWithObservedEffectClassification`、成功 changed/no-op receipt tests | 已覆盖成功及命令失败后的 changed/no-change receipt；before/after observation 自身失败的 owner receipt 尚未覆盖 |
| boundary | `TestSSHOwnerAcquiresCapabilitiesThroughTypedEventHubContract`、`TestSSHRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、typed operation/seal 及 admissibility/feasibility/semantic EventHub tests | 已覆盖 capability 正常 Send，以及五个订阅 topic 的 exact producer/strong-call/concrete payload 映射 |
