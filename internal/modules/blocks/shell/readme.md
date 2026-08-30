# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/shell` 是本地 `shell/command` 与 `validation/managed_session` 的 capability owner。

- `shell/command` 在 workspace deliverable directory 中执行受控命令，并以 workspace-durable effect 观察为准。
- `validation/managed_session` 只执行 owner catalog 已确认的 sealed recipe，在隔离的临时 workspace 副本中运行，effect 为
  `ephemeral_session`。

原始进程输出和执行细节保留在 Shell-private adapter；跨 owner 只返回有界 evidence、validation observation、effect fact 与
`RuntimeOperationReceipt`。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → Shell | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`ProbeRuntimeOwnerFactProspectiveSatisfaction`、`QueryRuntimeCWIReconciliation`、`ExecuteOperation` Command/Result |
| Orchestrator | Shell → Orchestrator | typed receipt query/commit；execute 只消费完整 `RuntimeOperationExecutionContext + SealedRuntimeOperation` |
| File owner | 经 Orchestrator fact 输入 | persisted workspace manifest/observation；Shell 不直接读取 File service/repository |
| OS process | Shell-private | command argv、workspace/scratch、timeout、process-group、bounded output 与 filesystem effect observation |

Prospective satisfaction 是 Shell-owner 的纯 factual query：它只校验一个 activation-sealed recipe 在给定 resulting manifest 上
是否满足 owner 声明的 bounded `all_of` target constraint，不选择 target、action、retry 或 Runtime 状态。

## 不变量与非职责

- `shell/command` 不得伪装成 `effect=none` validation；managed validation 不能退化为任意命令字符串。
- execute 前必须验证 producer、同步 Result、sealed payload、owner catalog、target scope、execution key/fence 和 receipt authority。
- managed validation 必须在隔离副本中运行；临时 effect 不得写回 deliverable，scratch 和进程组必须按策略释放。
- workspace-durable command 的 observed change 必须落在 sealed targets 内；未声明 effect 必须 rejected。
- prospective constraint 只支持 bounded `all_of`；不发布 clause `any_of`、surface `one_of` 或隐式可选 recipe。
- 全部七个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得启动进程、读取 workspace 或访问 receipt authority。
- Shell 只署名 capability、feasibility、validation、effect 和 receipt fact；不拥有 CWI credit、repair、retry、governance 或 terminal。
- 原始 stdout/stderr、secret 和自由文本 diagnostic 不得进入 Runtime authority；公开 diagnostic 必须有界、脱敏。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | command、managed validation、structured check、isolated workspace 与 concrete module EventHub execute tests | 已覆盖两类能力的主要正常路径 |
| invalid + scope | dangerous command、timeout/output bound、symlink/scratch、masked exit、missing target、undeclared effect、failure-target bounds、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要输入/process scope，并穷举七 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | zero-work/state-changing receipt replay、prospective duplicate pure-value tests | 已覆盖同 owner 实例的 receipt 重放和纯 query 重复调用 |
| stale + freshness | admissibility manifest prerequisite、target revision stale、prospective catalog stale tests | 已覆盖 owner/File fact freshness |
| restart / owner 重建 | 无直接组件级自动化证据 | 未覆盖新 Shell owner 从 persisted receipt 恢复且零二次进程执行 |
| lifecycle | `TestShellOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、process-group 与 scratch cleanup tests | 仅以 `AcquireRuntimeCapabilities` 代表 topic 覆盖 started fence、重复 Start、Stop 后零投递、重复 Stop 与 fresh-owner restart；不是七 topic lifecycle 矩阵；process/scratch cleanup 由独立 adapter tests 覆盖 |
| effect + receipt | declared/undeclared workspace effects、managed-session observation、accepted/rejected receipt tests | 已覆盖主要 effect 与 receipt 分类 |
| boundary | `TestShellOwnerAcquiresManagedValidationCapabilityThroughTypedEventHubContract`、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、module execute、seal/authority tests | 已覆盖七个订阅 topic 的 exact producer/strong-call/concrete payload 边界；operation-feasibility 与 semantic-proof 的真实 EventHub happy path 尚未直接覆盖 |
