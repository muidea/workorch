# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-09-04

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

Prospective satisfaction 是 Shell-owner 的纯 factual query：它只校验一个 activation-sealed recipe declaration 经 owner adapter
按 exact File manifest 解析后的 recipe 在给定 resulting manifest 上
是否满足 owner 声明的 bounded `all_of` target constraint，不选择 target、action、retry 或 Runtime 状态。

## 不变量与非职责

- `shell/command` 不得伪装成 `effect=none` validation；managed validation 不能退化为任意命令字符串。
- execute 前必须验证 producer、同步 Result、sealed payload、owner catalog、target scope、execution key/fence 和 receipt authority。
- managed validation 必须在隔离副本中运行；临时 effect 不得写回 deliverable，scratch 和进程组必须按策略释放。
- activation-declared recipe 可以是固定 transport 或 `shell/project-native-validation/v1` owner-adapter ref；模板不得携带
  语言 command/args/env。Shell 只依据同一 request 绑定的 File-owner manifest 解析具体 recipe，且该解析只产生 owner fact，
  不触发 Runtime 状态。Shell 只有在 Orchestrator request 携带 exact Definition-owned
  `RuntimeValidationProofRequirement` 时，才可对 matching recipe 发布逐 CWI applicability binding；recipe ID、进程 exit 0、
  zero-work marker 或 surface-wide check union 都不能自行成为 CWI proof。
- applicability binding 必须把每个 Definition check 映射到 Shell-owner 的 exact observation protocol 与 native check
  identity，并随 sealed execution authority 一同哈希封存。receipt/reconciliation 只承认该 protocol adapter 产生的 typed
  check attestation；raw stdout/stderr（包括 `WORKORCH_CHECK_PASSED=*`）、recipe ID 或任意进程自报 marker 只能是诊断，
  不能填充 passed check authority。另一个 CWI/native check 即使来自同一 recipe 也不能获得当前 CWI credit。
- recipe 的静态/输入 `Readiness` 与 requirement readiness 分开。一个 ready recipe 可以同时发布 ready CWI 的 exact binding
  和 missing CWI 的 exact preparation；preparation 封存 requirement、catalog revision、native mapping 与完整 prospective
  all-of target constraint，但不授予 proof/dispatch。File 完成 Profile 已声明的 acquisition 后，Shell 必须 fresh acquire
  workspace manifest 并重新发布 binding；旧 preparation、别的 CWI 或别的 protocol mapping 不得复用。
- workspace-durable command 的 observed change 必须落在 sealed targets 内；未声明 effect 必须 rejected。
- prospective constraint 只支持 bounded `all_of`；不发布 clause `any_of`、surface `one_of` 或隐式可选 recipe。
- 全部七个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得启动进程、读取 workspace 或访问 receipt authority。
- Shell 只署名 capability、feasibility、validation、effect 和 receipt fact；不拥有 CWI credit、repair、retry、governance 或 terminal。
- managed Go validation 的 bounded tool output 若明确报告依赖元数据问题，Shell 必须发布
  `validation_dependency_mismatch + dependency_contract`，即使 sealed recipe 只携带 generic build hint；incidental
  `*.go:line` 不得成为 implementation repair candidate。Shell 只发布事实；当前默认 Profile 没有该类自动 repair route，
  由 SM-2 有界终止。
- managed Go validation 只在 bounded tool grammar 命中时发布领域中立的可路由 owner code：`module_manifest_parse_failed` 对应
  manifest-scoped `go.mod|go.work|go.sum:line`，`source_compile_failed` 对应 manifest-scoped `*.go:line[:column]`/package conflict，
  `test_failed` 对应 typed test observation。无法精确分类的非零退出保留 `shell_exit_nonzero`，不得借 generic code 进入
  software-dev repair route。`go test -json` 的编译诊断只从有界 `output|build-output` 事件的 `Output` 字段解码，再与
  sealed File manifest scope 求交；JSON envelope、任意外部路径和未封存 target 都不能形成 repair candidate。
- structured validation observation 的 diagnostic hash 必须使用共享 canonical contract，绑定 schema/class、owner code、
  evidence target type、stage/operation、diagnostic targets 与 sealed validation scope；不得使用只覆盖 class/path 的 owner-private
  digest。diagnostic operation kind 必须是实际 sealed Shell operation kind；Shell 不选择匹配 route、repair owner、confirmation
  protocol 或终态。
- 原始 stdout/stderr、secret 和自由文本 diagnostic 不得进入 Runtime authority；公开 diagnostic 必须有界、脱敏。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | command、managed validation、structured check、isolated workspace 与 concrete module EventHub execute tests | 已覆盖两类能力的主要正常路径 |
| invalid + scope | dangerous command、timeout/output bound、symlink/scratch、masked exit、missing target、undeclared effect、failure-target bounds、`TestShellValidationDiagnosticClassifiesGoDependencyMetadataBeforeGenericBuildFailure`、`TestShellValidationDiagnosticPublishesExactGoOwnerCodes`、`TestShellManagedGoModuleDiagnosticReportsOnlyManifestScopedModuleTarget`、`TestShellManagedGoJSONDiagnosticReportsActualManifestScopedFailureTarget`、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要输入/process scope、精确 Go owner code、dependency-contract、Go JSON build output 与 implementation target 分离，并穷举七 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | zero-work receipt replay、`TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays`、prospective duplicate pure-value tests | 已覆盖同 owner 与 fresh owner 的 exact receipt 重放和纯 query 重复调用 |
| stale + freshness | admissibility manifest prerequisite、target revision stale、prospective catalog stale tests | 已覆盖 owner/File fact freshness |
| restart / owner 重建 | `TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays` | fresh owner 从 persisted receipt 重放，外部改写后的 target 保持不变，证明零二次进程执行 |
| lifecycle | `TestShellOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestShellOwnerLifecycleCoversEveryRuntimeTopic`、process-group 与 scratch cleanup tests | 覆盖全部七 topic 的重复 Start 单次投递、Stop 后零投递，以及独立 process/scratch cleanup |
| effect + receipt | declared/undeclared workspace effects、managed-session observation、accepted/rejected receipt、diagnostic hash field-drift tests | 已覆盖主要 effect、receipt 分类及 route 字段哈希完整性 |
| boundary | `TestShellOwnerAcquiresManagedValidationCapabilityThroughTypedEventHubContract`、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、`TestShellOwnerFeasibilityAndSemanticProofHappyPathsUseTypedEventHub`、module execute、seal/authority tests | 覆盖七 topic 的 exact producer/strong-call/concrete payload，以及 owner adapter 的 typed EventHub admissibility、operation-feasibility 与 state-change semantic feasible 路径 |
