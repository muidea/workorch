# Runtime Positive-Convergence Live Closure

Status: active

Type: deployed-verification-plan

Last Verified: 2026-08-16

## Purpose

本计划只保留当前仍未关闭的 deployed/live 验收。single-owner、model capability ceiling、complete-document
request、Definition declaration/generation、File target policy、equivalent-authority fence 以及 Orchestrator/Workorchd
内部内聚均已完成 code/automated closure；实现历史与旧 schema/run 反例见
[归档记录](../../90-archive/records/runtime-single-owner-model-capability-output-code-closure-record-2026-08-16.md)。

本计划不能覆盖 [Runtime 核心合同](../../00-core/runtime-stage-contract.md) 或 Orchestrator 实现合同。

## 唯一未关项

### SO-MC-LIVE — fresh-data 持续正向收敛

必须在已部署目标构建、清理后的 development data 和真实 provider 上取得一条完整 persisted 证据链：

```text
fresh activation
  -> semantic-plan request
  -> capability-owner feasibility batch
  -> SM-2 binding/reconciliation seal
  -> operations request
  -> SM-3 CandidateAttempt + SealedDispatch
  -> capability-owner receipt
  -> binding-matched evidence
  -> later eligible CWI
  -> sealed terminal
  -> Result -> Workspace -> Run delivery acknowledgements
```

验收必须同时证明：

1. provider selection 与 output ceiling 只来自 LLM owner 封存的 `ProviderCeiling`，ExpectedDocument、EventHub
   command、provider request 与 audit token 值完全一致；
2. semantic planning 只使用 current Definition declaration 和 owner-published catalog，capability owner 只返回事实，
   只有 Orchestrator SM-2 能封存 binding、选择 focus 或重开 CWI；
3. 当已存在目标被重新打开时，semantic/operations candidate 使用 owner catalog 中合法的 revision-bound
   `file/edit` 或 `patch/apply` replacement，不再选择 create-only `file/write`；
4. replacement 成功后继续推进至少一个后续 eligible CWI，不能把“正确 fail-closed”当成持续收敛；
   validation-preparation 遇到无当前适用 recipe 时必须选择 profile 允许且 owner-confirmed 的 File preparation，不能先向
   provider 暴露不可行 recipe、任意 shell command 或重复相同 semantic shape；
5. duplicate/stale/restart/recovery 不产生第二次 provider session、dispatch、durable effect、evidence credit 或
   terminal delivery；
6. revision 与 progress signature 有界推进，不出现无事实变化的 authority、candidate 或 drive 循环；
7. terminal delivery 严格按 `Result -> Workspace -> Run` 完成，投影/SSE 只展示已持久化事实。

## 最新反例后的 code closure

已部署构建 `58b20ec8` 的最新核对已经证明 existing-target replacement、owner receipt/evidence、CWI 026～028 推进和三段 terminal
delivery 顺序成立；最早未收敛点是后续 validation-preparation CWI：静态 catalog 宣告了对当前 workspace 无实际测试
输入的 `go test` recipe，semantic feasibility 随后才返回 `validation_recipe_unavailable`。同一失败结果还暴露了
terminal closeout 把已满足 CWI 的 producer claimed gap 排在当前失败之前。

当前 workspace 已完成对应 code/automated closure：

- SM-2 先取得 File manifest/observation，再以强类型 immutable binding 请求 Shell admissibility；
- Shell 只发布对该 manifest 当前适用的 recipe，并把 exact File tree revision 写入 freshness dependency；
- semantic-plan manifest 在 recipe set 为空时移除 `validation/managed_session`，`validation_preparation` 的 fallback 只保留
  File-owned `file/write` / `patch/apply`，不允许任意 `shell/command`；
- SM-4 Result closeout 只保留当前 unsatisfied CWI 的 canonical gap，并以当前 `Facts.Failure` 为 top failure；
- contract、owner preflight、SM-2 prerequisite ordering/idempotency、resource-change stale intersection 与 terminal
  projection 均有定向回归。

这些修改仍需部署后使用 fresh data 重新证明后续 CWI 和完整 live terminal；本节不关闭 SO-MC-LIVE。

## 执行前提

- 明确记录被测部署构建 identity；不得用本地工作区代码替代 deployed binary。
- 使用新建 Workspace、Goal、DoD、Template 与 deliverable directory；不迁移旧 Runtime aggregate。
- 真实 provider 凭据和 endpoint 只存在于部署配置，不进入证据文档。
- 先运行 [Runtime owner convergence matrix](../../60-verification/runtime-owner-convergence-matrix.md) 的自动化门禁。

## 失败处理

live 失败时只记录最早的 persisted invariant/counterexample，包括 workspace/run、state revision、authority、
request、candidate、dispatch、owner receipt 和 delivery identity。不得根据错误文本增加跨 owner target-state command、
兼容 schema、额外 retry 状态机或字段级旁路；任何代码修复必须重新对照 SM-1～SM-4 与对应 Owner README。

## Exit

只有完整 persisted 正向链、后续 CWI 推进和 terminal delivery 同时成立，才可关闭本计划。managed mock、局部制品、
revision 增长、单次合法 dispatch 或正确 fail-closed 均不足以关闭 live closure。
