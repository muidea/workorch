# Web 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/web` 是 `web/search` 与 `web/fetch` 的 capability owner。它负责 provider snapshot、URL/DNS/redirect policy、
有界 transport、内容规范化、owner-private evidence 持久化和 factual receipt。

Search 只向配置的 HTTP provider 发出有界 POST；Fetch 由 Web owner 直接执行 DNS-pinned GET，并在每次 redirect 前重新执行
URL、DNS、domain、redirect-limit 与 grant 检查。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → Web | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`ExecuteOperation` Command/Result |
| Orchestrator | Web → Orchestrator | exact receipt query/commit；需要放宽网络策略时消费 sealed governance grant |
| Approval owner | 无直接 I/O | Web 不查询 Approval；grant 只由 Orchestrator 封存进 execution envelope |
| Search provider / Fetch target | Web-private | bounded POST 或 DNS-pinned GET、safe redirect、content normalization 与 evidence file |

跨 owner 的唯一执行结果是 `RuntimeOperationReceipt`。完整规范化内容只写入 workspace internal directory 下的
Web-owner evidence file；EventHub 只携带 summary、hash、opaque source/raw refs 和 truncation/quality tags。

## 不变量与非职责

- execute 前必须验证 Orchestrator producer、同步 Result、sealed payload/scope、catalog snapshot、execution key/fence 和 receipt authority。
- provider revision 比较、provider 选择与 transport 必须使用同一 immutable snapshot；revision drift 在网络访问前返回 typed stale。
- 只允许 HTTP/HTTPS；userinfo、metadata、link-local、unspecified、multicast、非法 port/zone 和 deny-domain 必须预先拒绝。
- plain HTTP、loopback、private network 或超出默认 result policy 的访问必须取得 exact grant。
- search/fetch 都是 `effect=none + at_most_once`；无 receipt 不能被解释为可安全重放外部请求。
- transport/provider 私有错误、endpoint、target URL、header 和 credential 不得进入公开 feedback。
- 全部五个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得加载 provider、访问网络或读取/提交 receipt。
- Web 只提供 context/evidence candidate，不拥有 action、CWI credit、retry、governance resolution 或 terminal。

## 生命周期合同

- `Start` 逐成功订阅记账；部分失败保留 adapter/topic，未清理前拒绝再次注册。Configure 不覆盖未释放 owner。
- `BeginShutdown` 非等待式停止新 catalog/probe 和新外部访问，不取消已接纳执行，不提前退订 receipt 所需通路。
- Execute 的 drain 票仅用于原 sealed/producer/scope 校验与 exact persisted receipt 查询重放；未命中必须另取 fresh 票，关闭后零新网络请求、零新 receipt 提交。`effect=none` 不构成停机访问豁免。
- 票持续到真实 handler 返回（含 nested receipt commit）；Quiesce 超时保留依赖，可用新预算重试。Stop 封最终准入并再次排空后逆序 checked 退订，成功才移除资源账本、清 adapter/Module 引用。
- 未知 panic 不吞掉，不遗留清理 claim；plugin guard 保留原 cause。调用方串行 Configure/Start/Stop；本地 Quiesce 不替代全进程共享 task/Hub 屏障。
- 回归由 `biz/owner_lifecycle_test.go`、`module_lifecycle_test.go` 和既有 exact receipt 测试固定。SM-1～SM-4 的权威、预算、网络授权与 at-most-once 合同不改变。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | `TestRuntimeAdapterSearchReturnsRecoverableBoundedEvidence`、`TestWebOwnerFailsClosedBeforeNetworkAndCommitsRecoverableEvidence`、`TestWebOwnerFetchRequiresGrantThenCommitsBoundedReceipt` | 已覆盖两类 operation 的 adapter 与 owner receipt 正常路径 |
| invalid + scope | `TestRuntimeAdapterURLPolicyCoversSSRFBoundaries`、`TestRuntimeAdapterFetchChecksRedirectTargetBeforeAccess`、`TestRuntimeAdapterFetchPinsOneDNSResolutionToTransport`、`TestWebRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖网络/scope/authorization，并穷举五 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestWebOwnerFailsClosedBeforeNetworkAndCommitsRecoverableEvidence`、`TestWebOwnerFetchRequiresGrantThenCommitsBoundedReceipt` | 覆盖 search/fetch 在同 owner 与 fresh owner 上的 exact receipt replay，均为零二次外部请求 |
| stale + freshness | `TestWebAdmissibilityObservationReturnsTypedConfigurationStale`、`TestWebOwnerProbeDistinguishesConfigurationDriftFromUnavailableConfiguration`、`TestWebOwnerRejectsStaleSealedProviderRevisionBeforeNetwork` | 已覆盖 provider snapshot freshness |
| restart / owner 重建 | `TestWebOwnerFailsClosedBeforeNetworkAndCommitsRecoverableEvidence`、`TestWebOwnerFetchRequiresGrantThenCommitsBoundedReceipt` | fresh owner 对 search/fetch 均先返回 persisted receipt，不重复 provider/target 请求 |
| lifecycle | `TestWebModuleIdentityConfigureStartStopContract`、`TestWebOwnerLifecycleCoversEveryRuntimeTopic` | 覆盖全部五 topic 的重复 Start 单次投递与 Stop 后零投递 |
| effect + receipt | `TestWebOwnerFailsClosedBeforeNetworkAndCommitsRecoverableEvidence`、`TestWebOwnerFetchRequiresGrantThenCommitsBoundedReceipt`、`TestWebOwnerCommitsSafeRejectedReceiptForProviderFailure` | 已覆盖 `effect=none` 的 accepted/rejected receipt 和安全 failure feedback |
| boundary | `TestWebOwnerAcquiresCapabilitiesThroughTypedEventHubContract`、`TestWebRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、`TestWebOwnerAdmissibilityUsesTypedEventHubAndDoesNotAdvertiseCoverageProof` | 覆盖五 topic 边界与真实 typed EventHub admissibility；Web search/fetch 按合同只提供 context/evidence，不发布 validation/state-change coverage proof，因此不存在 semantic-proof happy path |
| deployed external network | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md)、[Deployed Convergence](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) | 组件级未覆盖；真实 search provider、public DNS/redirect、credential 与 egress policy 必须由显式 deployed/live gate 验收；当前 acceptance inventory 无 Web-search-specific producer，因此本项仍是部署缺口，不能计入本地组件单测完成度 |
