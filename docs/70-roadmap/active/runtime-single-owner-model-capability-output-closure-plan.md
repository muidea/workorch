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
   provider 暴露不可行 recipe、required-input MCP validation surface、任意 shell command 或重复相同 semantic shape；
5. duplicate/stale/restart/recovery 不产生第二次 provider session、dispatch、durable effect、evidence credit 或
   terminal delivery；
6. revision 与 progress signature 有界推进，不出现无事实变化的 authority、candidate 或 drive 循环；
7. terminal delivery 严格按 `Result -> Workspace -> Run` 完成，投影/SSE 只展示已持久化事实。

## 最新反例后的 code closure

### Manifest-bound Shell recipe 与 terminal closeout

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

### MCP required-input validation readiness

此前 fresh-data `run-006` 已经连续完成 CWI 026、027、028、029、001 的 semantic seal、owner dispatch/receipt 与
binding-matched evidence，说明前述 replacement、单 Owner transition 和正向推进路径成立。最早未收敛点移动到 CWI 002：
semantic catalog 把 `mcp/tool_call/mcp-lsp-bridge/lsp_definition` 当作可直接执行的 validation recipe 暴露给 provider，
owner feasibility 随后才发现其 schema 必须提供 `line`、`path|file_path` 和 `col|character`，而当前 semantic behavior
probe 没有 owner-sealed input binding，最终以 `MCP semantic behavior probe cannot invent required tool parameters` 失败。

当前 workspace 已完成对应 code/automated closure：

- owner-neutral `RuntimeValidationSurface` 将 callable operation、validation readiness 与 proof applicability 分离，并持久化
  `ready | prerequisite_required | unavailable`、`direct | auxiliary | not_declared`、schema hash、
  input binding/prerequisite、check/effect/replay facts；
- MCP execution owner 在一次一致 registry snapshot 中编译 readiness：只有 non-durable、safe-to-replay、schema 合法且
  input-complete，并由 registry owner 显式声明为 direct proof 的 tool 进入 `ValidationRecipeRefs`；required-input tool 仍
  保留 callable capability，但只记录 `prerequisite_required/input_binding_required`，catalog acquisition 零 MCP endpoint side effect；
- 无 required input 的安全 tool 可以具有 owner-sealed empty-input binding；技术 readiness 与 direct proof applicability
  正交。默认 `lsp_status` 为 `ready + auxiliary`，只用于审计/诊断；`lsp_definition` 为 direct 但在缺少 sealed input 时
  不再进入 semantic recipe enum/C3。旧或伪造 proposal 仍由 MCP owner fail-closed，且不进入 operation receipt/evidence 路径；
- LLM C3 改为 provider-safe owner catalog projection，去除 non-ready surface identity、input schema/binding/prerequisite、
  auxiliary/not-declared identity、MCP effect metadata 和 Shell executable recipe；renderer 在最后边界再次按 readiness 与
  proof applicability 取交集；
- Shell confirmed recipe 同样发布 `ready + direct` surface；readiness/applicability/schema/binding 参与 catalog snapshot
  hash，并进入 Workorchd metadata-only runtime audit，projection 不参与 authority；
- contract、MCP owner matrix、provider projection、semantic schema、snapshot identity 与 audit projection 均已增加定向回归。

该收口不增加第五状态机，不让 Orchestrator 解析 MCP schema，也不自动发明 prerequisite input。若未来需要自动取得
`lsp_definition` 参数，必须先由相应 owner 增加 concrete typed acquisition command/result，再接入既有 SM-2 private
transition。当前仍需部署后以 fresh data 证明 CWI 002 能选择 owner-confirmed 的 direct proof alternative、继续推进后续
CWI 并完成 terminal delivery；因此本节仍不关闭 SO-MC-LIVE。

### Current-generation rebound queue 与 direct proof applicability

已部署构建 `69d3be7b` 的 fresh-data `run-007` 进一步完成 CWI 026～029 的正向执行，并进入 CWI 001/002。CWI 001 的
MCP behavior observation 因 `behavior_probe_unavailable` 被判为 unverifiable，SM-2 合法重开并在 current semantic manifest
下重新绑定 CWI 001；由于较早的 bound CWI 002 仍是 effect-fenced current focus，重绑后的 CWI 001 位于 remaining queue。
revision 266 的 SM-2 eligible-focus projection 把这个“同代已绑定队列成员”误判为 planning-ineligible stale row，最终以
`semantic order lineage contains non-settled ineligible coverage "cwi-001"` 失败。

同一运行还证明仅按输入 readiness 不足以约束 direct evidence：`lsp_status` 没有 required input，因此技术上 ready，
但其服务状态只属于辅助诊断，不能替代 CWI 001/002 要求的直接行为证明。

当前 workspace 已完成对应 code/automated closure：

- SM-2 semantic lineage 仅在 row 精确绑定 current manifest、仍位于 active/remaining focus queue、binding ref 完整匹配，且
  所有 prerequisite 仍 settled+satisfied 时保留该 bound row；stale manifest、binding drift、completed row 与 reopened
  prerequisite 继续 fail-closed；
- SM-2 duplicate/restart re-drive 在上述 lineage 校验后只把仍 planning-eligible 的 row 交给纯 eligibility projector；
  已绑定 queue row 保持原顺序并零 CAS，不会被重新规划或创建第二份 binding；
- MCP Registry owner 显式维护 `direct | auxiliary | not_declared` proof applicability；Runtime catalog 只有
  `ready + direct` 才产生 provider-selectable `ValidationRecipeRefs`，该字段参与 canonical snapshot identity；
- 默认 `lsp_status` 已声明为 auxiliary；直接 LSP 行为工具保持 direct，但 required-input surface 在 owner 封存输入前仍为
  `prerequisite_required`。LLM 最终 renderer 再次按 readiness 与 applicability 过滤，Workorchd 只投影该审计事实。

本次影响 SM-2/SM-3 的 current-generation lineage/admissibility intersection；SM-1 与 SM-4 transition 均不改变，SM-4
重复/restart drive 仍只进入同一 SM-2 幂等投影。不新增状态机、Owner 或跨组件 target-state command。部署后仍必须证明：
系统取得 owner-confirmed Shell direct recipe，或通过对应 capability owner 的 concrete typed acquisition 封存 direct
MCP 输入；若两者都不存在，应有界 fail-closed 且不得给辅助 probe 记 credit。只有继续推进后续 CWI 并完成完整 terminal
delivery 才能关闭 SO-MC-LIVE。

### Validation-preparation current owner postcondition

对同一 fresh-data `run-007` 的进一步 persisted 核对显示，CWI 029 曾通过 File owner 的 revision-bound patch 把
`mcp-config.json` 中的 `go test ./...` 改为 `go test -race ./...`，并取得有效 state-change receipt/evidence；随后 assessment
宣称该变更已经建立 executable validation，SM-2 因而把 CWI 029 标为 satisfied。可是 assessment 时的 current Shell
admissibility catalog 没有任何 recipe/ref/surface，因此这个 CWI 实际只证明“配置发生变化”，没有证明“当前存在可选择的
验证入口”。CWI 001～025 又依赖 CWI 029，这个假 credit 会提前放开后续队列，并可能被更晚的失败掩盖。

当前 workspace 已完成对应 code/automated closure：

- runtime contract 增加 owner-neutral `RuntimeOwnerFactRequirement`；software-dev `validation_preparation` 声明
  `blocks/shell + selectable_validation_surface + validation/managed_session`，且 operation 必须属于该 owner 并位于 profile
  accepted kinds。该声明进入 execution-contract stable hash、bootstrap、immutable DefinitionFocus 和 generation focus；
- Workorchd 只解析、规范化并传播声明，不读取 Shell 状态；Shell 仍只通过原有 typed admissibility command/result 发布
  `RuntimeOperationAdmissibility`。没有新增跨组件 setter、service/repository 访问或第五状态机；
- SM-2 assessment 先独立校验 binding-matched evidence/effect/quality，再从 current owner catalog 检查 operation kind、
  `ready + direct` surface 与 `ValidationRecipeRefs` 的精确交集。producer 的 satisfied disposition、文件 patch、旧 recipe
  或 auxiliary/non-ready surface 均不能覆盖该结果；
- 只有 owner fact 是唯一缺口时，assessment commit 才在同一 CAS 中提交 audit/dispatch、重开 current bound CWI、清除
  current binding、更新 reconciliation，并以 typed lineage 退休 semantic generation。普通弱证据/缺证据继续走既有
  repair；SM-4 只从新 aggregate 幂等 re-drive；
- 同 CWI owner-fact replan 由 `MaxRepairAttemptsPerFocus` 有界；耗尽返回
  `owner_fact_convergence_exhausted`。定向测试覆盖 ready/direct、auxiliary、required-input、unpublished ref、sole-gap guard、
  CWI/semantic generation reopen、restart 零重复 CAS 与正常 completion。

本次主要影响 SM-2 assessment/CWI/reconciliation transition；SM-3 的 candidate/dispatch seal、SM-4 drive 和 Shell owner
state machine 均未增加新 transition。部署后必须证明 CWI 029 不再仅凭 preparation patch 关闭：新 semantic generation
应在 refreshed Shell catalog 中选择 owner-confirmed direct validation surface；若始终不存在，则必须在有界次数内
fail-closed，不能继续释放依赖 CWI。该 deployed 证明及后续 terminal delivery 尚未取得，因此 SO-MC-LIVE 保持 open。

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
