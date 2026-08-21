# Deployed Runtime Convergence Scenario Matrix

Status: normative

Type: verification-contract

Last Verified: 2026-08-21

## Purpose

本文把 PRD 的 deployed convergence 要求映射为可重复场景。它验证已部署 file-backed 实例中的 owner integration、真实
Runtime aggregate、真实 capability owner receipt、process restart 与正式 Result；不重新定义 SM-1～SM-4 transition。

脚本/target 名称只是候选执行入口。只有入口实际产生并校验本场景规定的 manifest/assertions 时，该行才通过；同名、零
test selection、mock projection、手工改库或预制 successful payload 均不算 evidence。

## Environment Baseline

每次执行必须记录并验证：

- latest built service revision、config identity、file-backed data root 和启动时间；
- fresh、此前不存在的 Workspace/Run/Result IDs，且至少一个套件并行使用两个隔离 Workspace；
- exact admitted Definition/startup-input、Template/Profile/provider/capability revision identity；
- service process PID/instance epoch，以及每次有意 restart 前后的 owner reload evidence；
- 所有 provider/capability fact均经当前 concrete command/result和owner repository产生，不直接写aggregate、projection或Result；
- terminal/wait场景在bounded deadline内结束为预期typed disposition，超时或缺少证据即失败。

mock LLM/provider可用于CI-safe行，但 Orchestrator、Run、Workspace、Result和capability owner路径必须真实执行。要求real-provider
的行必须使用显式live gate；mock不得冒充real-provider evidence。

## Required Scenario Matrix

| ID | Required deployed flow | Mandatory assertions | Eligible current entrypoints |
| --- | --- | --- | --- |
| `DC-01` | admitted all-of Definition经过多次drive完成semantic plan→binding→candidate→sealed dispatch→receipt→evidence→assessment→Result | 每次drive revision单调；operation前已有binding/dispatch seal；至少一个真实capability owner receipt；Result与terminal delivery hash匹配 | `make verify-runtime-positive-convergence-live`; `make verify-real-runtime-closure` |
| `DC-02` | provider request crash-window：submission前中断、submission后result commit前中断、response loss | 复用same logical request/session；physical-call ledger无无界重复；complete document只消费一次attempt；unknown不按zero重放 | `make verify-runtime-restart-recovery-live`; real-provider时`make verify-real-runtime-closure` |
| `DC-03` | durable capability effect在owner commit/Runtime receipt commit前后restart | same ExecutionKey/Fence；owner effect至多一次；matching receipt/reconciliation后才credit；stale/duplicate receipt零第二credit/预算 | `make verify-runtime-restart-recovery-live`; `make verify-service-restart-recoverable-e2e` |
| `DC-04` | accepted evidence后修改一个精确相交owner resource并revalidate | 只有相交dependency set的evidence进入`stale_or_superseded`并reopen对应CWI；无关CWI保持current；新receipt/assessment后重新close | `make verify-software-dev-goal-dod-e2e`; profile-specific deployed verifier |
| `DC-05` | typed transient owner condition打开recoverable wait，process restart后由matching trigger/probe恢复 | wait record字段完整；普通Resume/HTTP wait不解除；duplicate trigger零重复call/effect；condition满足后继续；独立case覆盖deadline/guard terminal | `make verify-service-restart-recoverable-e2e`; `make verify-runtime-restart-recovery-live` |
| `DC-06` | provider和capability budget分别经历reserved→confirmed、unknown→reconciled及exhaustion | allocation先于call/effect；physical identity去重；view等于sub-ledger；restart不重置；unknown不算remaining；exhaustion只wait或typed terminal | 当前入口只有在显式导出并断言budget manifest后才合格；否则本行未通过 |
| `DC-07` | genuine Decision/Approval governance wait完成resolution | ticket具备真实external dependency；resolution scope/revision匹配；stale/duplicate确定；系统/预算错误未伪造ticket；恢复后同Run继续 | `make verify-auto-governance-ticket-approval`; `make verify-approval-decision-resolution-e2e` |
| `DC-08` | current-baseline restart跨每个ReplacementIntent/terminal-delivery crash-window | intent先于delivery；Result→Workspace→Run ack顺序；唯一successor；predecessor不可变；successor fresh policy/aggregate；duplicate request同lineage | `make verify-runtime-restart-recovery-live`; `make verify-software-dev-goal-dod-e2e` |
| `DC-09` | terminal closeout与HTTP/Web read model核对 | 每个required CWI逐项三轴；accepted refs和owner revision union精确；`indeterminate`不成功；closeout/view hash、Definition/Run lineage一致 | `make verify-runtime-positive-convergence-live`; service/read-model focused suite |
| `DC-10` | 两个Workspace并发drive、restart/resume、owner notification与SSE reconnect | request/dispatch/receipt/evidence/wait/budget/Result均不串scope；projection lag不驱动authority；各自terminal/recovery独立 | `make verify-dual-workspace-resume-replan-e2e`; `make verify-tui-web-projection-e2e` |
| `DC-11` | pre-seal candidate包含one-of/ambiguous relation，随后提交corrected all-of candidate | invalid attempt只有candidate/violation/ticket；零promotion/Run/provider/effect；duplicate同ticket；corrected promotion先pending，专用resolution + Workspace ack后才eligible；setup不强制LLM | 现有入口只有在输出candidate/ticket/admission manifest并断言零副作用后才合格；否则本行未通过 |
| `DC-12` | Runtime semantic document暴露implicit one-of/undeclared prerequisite，用户修订Definition并显式Restart | dedicated clarification dispatch/ticket绑定old Run/Definition/focus；affected focus零新operation；generic answer/Resume拒绝；new all-of revision不进入predecessor；replacement binding、terminal delivery、唯一successor跨restart确定恢复 | 现有入口只有在注入runtime gap并覆盖open/intent/resolution/ack crash-window后才合格；否则本行未通过 |

“Eligible current entrypoints”不表示截至某次提交已经完整覆盖该行。任何入口缺少一项mandatory assertion时，必须增强该入口
或增加聚合gate；在此之前该行保持failed，并在`docs/`外创建blocking ticket，不能修改本矩阵降低标准。

## Evidence Manifest

每个场景必须生成结构化manifest并由verifier主动读取校验，至少包含：

```text
DeployedConvergenceEvidenceManifest
  ScenarioID
  ServiceRevision
  ConfigIdentity
  DataRootIdentity
  StartedAt
  WorkspaceIDs[]
  RunIDs[]
  DefinitionRevisions[]
  RuntimeStateRevisions[]
  RequestSessionRefs[]
  DispatchRefs[]
  OwnerReceiptRefs[]
  EvidenceRefs[]
  OwnerRevisionSetHashes[]
  BudgetViewHashes[]
  WaitRefs[]
  GovernanceRefs[]
  DefinitionCandidateRefs[]
  DefinitionClarificationRefs[]
  ReplacementIntentRefs[]
  ResultRefs[]
  TerminalDeliveryRefs[]
  RestartEpochs[]
  Assertions[]
  CompletedAt
```

允许字段为空仅限场景不适用，并由scenario schema明确；不能用空数组表示“已验证”。refs必须可在保留artifact模式下解析为
同一data root中的owner/read evidence。manifest可以保存safe summary/hash/ref，但不能复制credential、raw prompt/provider body或
未脱敏文件内容。

## Crash-Window Method

crash场景必须在被测transaction的持久化边界注入确定性stop/failpoint或使用受控owner response loss，而不是根据sleep概率
猜测。每个窗口独立使用fresh Workspace/Run：

1. external submission/effect前、allocation/seal CAS后；
2. owner submission/effect commit后、Runtime result/receipt CAS前；
3. Runtime CAS后、caller收到response前；
4. terminal delivery各owner acknowledgement前后；
5. recoverable wait probe claim后、owner result join前。

恢复后必须读取owner repository和opaque aggregate重新drive；保留旧process memory、重放projection/eventstream或人工Resume
都使该case无效。

## Release Decision

- CI-safe发布基线至少覆盖可用mock条件下的`DC-01`、`DC-03`、`DC-05`、`DC-06`、`DC-07`、`DC-08`、`DC-09`、`DC-10`。
- Definition发布基线必须另覆盖`DC-11`与`DC-12`；只证明一个合法all-of happy path不能替代显式/隐式歧义的fail-closed路径。
- real-provider发布声明必须另覆盖`DC-02`及`DC-01`的real-provider variant。
- profile发布声明必须覆盖`DC-04`的对应owner mutation/revalidation variant。
- 任一required行缺失entrypoint、manifest、assertion或bounded terminal都使相应发布gate失败；不能以unit test或其它场景成功替代。

核心transition断言见 [Core Runtime Verification](../00-core/runtime/verification.md)，HTTP/部署层规则见
[Service Acceptance](service-acceptance.md)，profile-specific证明见对应profile verification。
