# Runtime Validation NextAction Convergence Improvement Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-finalization-contract-evidence-resource-closure-plan.md`
- `runtime-validation-evidence-quality-gate-closure-plan.md`
- `runtime-structured-evidence-authority-closure-plan.md`
- `runtime-live-convergence-gate-closure-plan.md`

## Purpose

本文用于固化最新 live 核对后暴露的核心偏差，并给出后续代码收口方案：当 coverage/evidence ledger 已经明确当前 focus 缺少 `validation` evidence 时，runtime 必须派发 `validate` / `produce_concrete_validation_evidence` 合同，而不能被旧 repair lineage、旧 `repairObjective`、旧 `acquire_evidence` nextAction 或诊断恢复分支抢占。

目标不是继续叠加 prompt 约束，也不是为某个 workspace、语言、文件或命令增加特判，而是把 `Goal/DoD -> coverage ledger -> deterministic NextAction -> Stage Packet -> contract gate -> evidence ledger` 的闭环重新收紧到单一权威。

当前阶段不考虑兼容性。若现有代码、测试或历史 closure 文档与本文冲突，应按本文收口，并删除或改写固化旧行为的测试。

## Contract Audit

- Scope: runtime convergence authority、coverage work item next action、validation evidence contract、contract violation recovery、repair action package requiredOutput。
- Observed state:
  - live workspace 已经产生真实状态变更文件，并且后续 gap 明确为缺少 accepted `validation` evidence。
  - projection / owner revision 已对齐，TUI/Web 展示不是主要 blocker。
  - 后续 step 输入里同时出现 `currentGap=missing accepted evidence binding for required evidence kind: validation`、`nextAllowedConvergenceStates=[need_validation]`，但 `convergenceAuthority.nextAction.kind=acquire_evidence`、`repairObjective=acquire_concrete_failure_evidence`。
  - `requiredOutput` 进入 `mustProduceConcreteFailureEvidence=true`、`stateChangingForbidden=true`、`patchForbiddenUntilConcreteTargetEvidence=true`、`deliverableWriteForbidden=true`，导致 LLM 被要求做读证据/失败证据采集，而不是执行 validation。
  - 最终失败时 tool-call output 中没有可接受的 `shell/command`、`mcp/tool_call` 或 `validation/managed_session` validation evidence。
- Expected state:
  - 当前 focus 如果已有 state-changing evidence 或已存在可验证产物，并且缺口是 `validation`，NextAction 必须是 `validate`。
  - Stage Packet、convergence contract、repair action package 和 execution contract 必须同时表达 `need_validation` 与 `mustProduceConcreteValidationEvidence=true`。
  - 如果 validation 因缺少 target snapshot 无法安全执行，runtime 也应明确进入 `acquire_target_snapshot` 或 validation-oriented controlled substitute，而不是泛化为 failure evidence acquisition。
  - 旧 failed-step input、旧 repairActionPackage、contract violation recovery、rejected assessment 和历史 budget 只能作为 diagnostic lineage，不能覆盖 current authority。
- Deviation:
  - validation gap 被 `acquire_evidence` / `acquire_concrete_failure_evidence` 抢占，实际合同与 ledger 缺口相反。
  - `need_validation`、`nextAllowedConvergenceStates`、`repairObjective`、`requiredOutput` 来自不同权威快照，dispatch 前没有被统一门禁拦截。
  - 部分测试仍把 repeated validation failure 转成 generic failure evidence acquisition 当作期望，可能继续固化当前偏差。
- Owner: WorkOrch core。问题位于中立 runtime convergence、NextAction interpreter、contract derivation、evidence gate 和 tests，不属于 workspaceTemplate、Soul、软件开发模板、provider、TUI/Web 或运行环境。
- Event path:
  - accepted state-changing candidate / existing output -> evidence ledger -> coverage ledger missing validation -> deterministic NextAction -> convergence contract -> requiredOutput -> LLM/block dispatch -> validation evidence record -> coverage recompute。
- Contract gap:
  - `validation` 缺口没有作为 high-priority deterministic action 贯穿到所有派生字段。
  - `acquire_evidence` 同时表示 target snapshot acquisition、failure diagnosis、read-only exploration 和 validation 前置条件，语义过宽。
  - contract violation recovery 允许旧诊断线路重新选择 generic evidence acquisition，绕过 current ledger。
- Proposed change:
  - 收拢 NextAction 到 `validate`、`state_change`、`acquire_target_snapshot`、`request_governance`、`assessment` 等明确动作。
  - 禁止 validation-only gap 生成 generic `mustProduceConcreteFailureEvidence`。
  - dispatch 前强校验 current focus、NextAction、contract state、repairObjective、requiredOutput 和 Stage Packet 是否同源。
- Boundary check:
  - 只依据结构化 evidence kind、coverage status、target snapshot completeness、governance state 和 capability contract。
  - core 禁止出现模板 ID、Soul ID、语言、命令、文件名、provider、workspace title 或 live workspace ID 特判。
- Verification:
  - 单测覆盖 validation gap 被错误派发为 acquire_evidence 的失败样本。
  - 单测覆盖 target snapshot 缺失时只能进入 `acquire_target_snapshot`，不能进入 generic failure evidence acquisition。
  - live 验证要求缺 validation 的 workspace 下一步必须产生 concrete validation evidence、accepted/rejected validation record 或明确 governed blocker。

## Design Alignment

### Alignment With Goal/DoD NextAction Design

基础设计要求 deterministic NextAction 的顺序是：

1. 缺 `state_change` evidence -> `state_change`。
2. 已有 `state_change` 但缺 `validation` evidence -> `validate`。
3. evidence 足够但缺 assessment -> `assessment`。
4. 所有 required evidence 满足 -> `complete`。

当前 live 偏差违反第 2 条：ledger 已经把缺口定位为 `validation`，但实际 dispatch 合同要求 LLM 采集 failure evidence。该偏差会让 workspace 在“已有产物但未验证”的阶段不断读取、分析或生成诊断，无法形成可关闭 DoD 的 validation evidence。

### Alignment With Convergence Authority Simplification

简化设计要求 `gapBaseline`、`convergenceContract`、`stagePacket`、`repairActionPackage.requiredOutput` 必须由同一次 authority snapshot 派生。当前偏差说明仍有旧 lineage 进入了派生链路：

- `currentGap` 和 `nextAllowedConvergenceStates` 指向 `need_validation`。
- `convergenceAuthority.nextAction.kind` 或旧 `repairObjective` 指向 `acquire_evidence`。
- `requiredOutput` 继承了 generic failure evidence acquisition 限制。

后续收口必须让 dispatch gate 在 provider/block 调用前拒绝这种不一致，而不是把互相矛盾的合同发送给 LLM。

## Root Cause

### R1: Generic Evidence Acquisition Semantics Too Broad

`acquire_evidence` 当前覆盖了至少四类不同需求：

- 缺少当前 target snapshot，需要 read-only snapshot acquisition。
- validation 失败后需要更多失败诊断。
- state_change 之前缺少可行动目标证据。
- contract violation recovery 需要 rejected output 的修正诊断。

这些语义在 requiredOutput 上都容易落到 `mustProduceConcreteFailureEvidence`。当 current gap 是 validation-only 时，这个泛化动作会抢占正确的 validation action。

### R2: Validation Gap Priority Is Not Enforced End To End

`missingValidationNextAction` 可以产生 `NextActionTypeValidate`，`concreteValidationEvidenceRequiredOutput` 也能表达 validation 合同，但 `runtime_convergence.go` 中 coverage-work-item 分支、contract violation recompute 和 `populateConvergenceContractFromNextAction` 仍可把旧 `repair/acquire_evidence` 转成 `need_evidence`。

这说明 validation priority 只在局部成立，没有成为 dispatch 前的全链路 invariant。

### R3: Diagnostic Lineage Can Still Become Current Authority

rejected assessment、old failed step input、old repair action package、contract violation recovery 和 focus budget 本应只作为 C4 diagnostic / retry hint。但 live 输入表明它们仍可影响 current `repairObjective` 和 requiredOutput。

该问题会导致 LLM 收到“当前缺 validation，但禁止 validation 相关 state/action”的矛盾合同。

### R4: Tests May Preserve The Wrong Closure

已有测试中存在 repeated validation failure 被期望转为 `repairObjective=acquire_concrete_failure_evidence`、`convergenceContract=need_evidence` 的样例。该类测试需要重新审计：

- 如果真实缺口是 missing target snapshot，应改成 `acquire_target_snapshot`。
- 如果真实缺口是 validation evidence，应改成 `produce_concrete_validation_evidence`。
- 如果确实缺少外部权限/配置，应进入 governance 或 fail-fast。

不应再把 validation loop 的通用兜底定义为 generic failure evidence acquisition。

## Target Invariants

### I1: Validation Gap Dominates Generic Evidence Acquisition

当 current focus 满足以下条件时：

- `missingEvidenceKinds` 包含 `validation`；
- `missingEvidenceKinds` 不包含未满足的 `state_change`，或 state-changing evidence 已被 accepted / workspace output 可验证；
- 没有 pending approval / decision / external dependency；

则 current action 必须是：

```text
NextAction.type = validate
ConvergenceContract.State = need_validation
repairObjective = produce_concrete_validation_evidence
requiredOutput.mustProduceConcreteValidationEvidence = true
```

并且不得出现：

```text
requiredOutput.mustProduceConcreteFailureEvidence = true
repairObjective = acquire_concrete_failure_evidence
stateChangingForbidden used as a reason to prevent validation
deliverableWriteForbidden used as a generic substitute for validation contract
```

### I2: Target Snapshot Acquisition Is A Separate Action

如果 validation 或 state_change 无法继续的唯一原因是缺少当前 target snapshot，则 action 必须是 `acquire_target_snapshot`，requiredOutput 必须只允许 read-only snapshot acquisition。

该 action 不能被命名或记录为 generic `acquire_evidence`，避免后续误解为 failure evidence acquisition。

### I3: Diagnostic Evidence Cannot Override Current NextAction

contract violation recovery、rejected diagnostics、old repairActionPackage、old budget 和 historical step input 只能影响：

- C4 retry hint；
- rejected evidence timeline；
- budget signature；
- authority diagnostics。

它们不能覆盖 current focus、current NextAction、contract state、repairObjective 或 requiredOutput。

### I4: Dispatch Input Must Be Self-Consistent

provider/block dispatch 前必须检查：

- `convergenceAuthority.currentFocus`、`gapBaseline.currentFocus`、`coverageWorkItemContext.currentFocus` 一致；
- `convergenceAuthority.nextAction.type` 与 `convergenceContract.State` 一致；
- `repairActionPackage.requiredOutput` 与 contract state 一致；
- `stagePacket.requiredOutput` 与 top-level requiredOutput 一致；
- validation gap 不携带 generic failure evidence acquisition requiredOutput。

不一致时只能：

1. 刷新 authority 并重建派生字段；
2. 若仍不一致，按 runtime internal contract misalignment fail-fast；
3. 不能继续派发给 LLM。

## Closure Scope

### P0: NextAction And Contract Authority

涉及代码：

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`

收口要求：

- `missingValidationNextAction` 产生的 `validate` 必须在后续 contract derivation 中保持到 dispatch。
- `populateCoverageWorkItemConvergenceContract` 必须先处理 validation-only gap，再考虑 generic evidence acquisition。
- `populateConvergenceContractFromNextAction` 对 `repair/acquire_evidence` 必须区分来源；如果 current coverage focus 是 validation gap，则不能生成 `need_evidence`。
- `convergenceConcreteValidationCannotAdvanceWithoutTarget` 只能路由到 `acquire_target_snapshot` 或 validation-oriented controlled substitute，不能路由到 generic failure evidence acquisition。

验收：

- 单测证明 validation-only gap 不会生成 `need_evidence`。
- 单测证明 stale `nextAction=acquire_evidence` 在 current authority `need_validation` 时被忽略或 fail-fast。

### P0: RequiredOutput Single Source

涉及代码：

- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`

收口要求：

- `requiredOutput` 只能由 current contract state 生成。
- `need_validation` 必须调用 `concreteValidationEvidenceRequiredOutput` 或等价 managed validation requiredOutput。
- `need_evidence` 只能用于 read-only evidence acquisition，且必须有明确 action subtype，例如 `target_snapshot`、`diagnostic_probe`、`external_dependency_probe`。
- dispatch gate 拒绝 `need_validation` + `mustProduceConcreteFailureEvidence` 的组合。

验收：

- 单测覆盖 `repairActionPackage.requiredOutput`、top-level `requiredOutput`、`stagePacket.requiredOutput` 三者对齐。
- 单测覆盖 managed validation / shell / MCP validation acceptedKinds 与 capability catalog 一致。

### P0: Contract Violation Retry Recompute

涉及代码：

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`

收口要求：

- 违例重试先重算 current ledger 和 current NextAction，再根据 current action 构造 correction contract。
- 如果 current action 仍是 validation，则 retry 必须维持 `produce_concrete_validation_evidence`。
- repeated same-rule violation 只能消耗 focus-scoped retry budget，预算耗尽后进入 governed blocker 或 terminal failure，不能退回 generic acquire evidence 无限绕圈。

验收：

- 单测覆盖 rejected assessment 不能把 validation gap 转成 acquire_evidence。
- 单测覆盖 repeated validation contract violation 仍保持 validation contract，直到预算耗尽进入明确 blocker/fail。

### P1: Evidence Binding And Coverage Recompute

涉及代码：

- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`

收口要求：

- validation evidence record 必须稳定携带 `coverageWorkItemIds`、`structuredTargetRefs`、`observedClaims`、`sourceRequirementRefs`。
- managed validation、shell validation、MCP validation 都必须走同一 metadata normalization。
- accepted validation evidence 入账后必须触发 coverage recompute，并把 current focus 推进到 assessment 或 complete。

验收：

- 单测覆盖 accepted validation evidence 绑定当前 CWI 后，missing validation 被关闭。
- 单测覆盖未绑定 CWI 的 validation evidence 不能被复用关闭 coverage。

### P1: Projection And Live Observability

涉及代码：

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`

收口要求：

- Projection 展示 owner structured NextAction，不二次推导业务状态。
- ViewModel 暴露 authority diagnostics：例如 validation gap 被 stale requiredOutput 抢占。
- TUI/Web prompt、主视图、workspace list 必须展示同一 revision 的 current action。

验收：

- projection 单测覆盖 owner nextAction=validate 时 ViewModel 不显示 acquire_evidence。
- live 验证脚本输出 current focus、missing evidence、nextAction、contract state、requiredOutput kind。

### P2: Test And Legacy Cleanup

涉及代码：

- `internal/modules/kernel/orchestrator/biz/*_test.go`
- `docs/70-roadmap/active/**`

收口要求：

- 审计并改写期望 repeated validation failure -> acquire failure evidence 的测试。
- 删除或降级“generic acquire_evidence 是 validation loop 默认兜底”的文档表述。
- 新增防回归测试，证明 core 不引入模板、语言、命令、provider 特判。

验收：

- `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过。
- `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1` 通过。
- core boundary search 无新增业务特判。

## Implementation Order

### S1: Freeze Failing Fixture

建立最小失败样本：

- coverage item missing only `validation`；
- accepted state-change evidence 或可验证 workspace output 已存在；
- old lineage 带 `repairObjective=acquire_concrete_failure_evidence` 或 stale `nextAction=acquire_evidence`；
- 期望重建后 output 为 `need_validation`。

Done:

- 该测试在当前实现下失败。
- 测试不包含具体语言、命令、文件名或 workspace ID。

### S2: Reorder Contract Derivation

调整 `populateCoverageWorkItemConvergenceContract`：

- governance blocker 仍最高优先级；
- state_change 缺口优先进入 state_change 或 acquire target snapshot；
- validation-only 缺口优先进入 validation；
- generic evidence acquisition 仅允许在明确缺 target snapshot / diagnostic probe subtype 时出现。

Done:

- validation-only gap 不再被 `convergenceConcreteValidationCannotAdvanceWithoutTarget` 泛化抢占。

### S3: Split Evidence Acquisition Subtypes

把当前 generic `acquire_evidence` 拆为结构化 subtype：

- `acquire_target_snapshot`
- `acquire_diagnostic_evidence`
- `acquire_external_dependency_evidence`

`NextAction`、contract state、requiredOutput 和 projection 都必须携带 subtype。

Done:

- validation gap 不再使用无 subtype 的 acquire_evidence。
- projection 可以显示具体原因，而不是泛化成 repair。

### S4: Add Dispatch Consistency Gate

在 dispatch 前新增/收紧一致性校验：

- validation gap + failure evidence requiredOutput -> fail-fast / rebuild；
- NextAction validate + contract need_evidence -> fail-fast / rebuild；
- Stage Packet requiredOutput 与 repairActionPackage requiredOutput 不一致 -> fail-fast / rebuild。

Done:

- 不一致合同不会发送给 provider/block。

### S5: Rewrite Retry Recovery

contract violation recovery 必须：

- 先从 ledger 刷新 current authority；
- 再生成同 action correction contract；
- retry hint 只能进入 C4；
- 不允许 rejected output 反向选择 NextAction。

Done:

- rejected assessment 后仍派发 validation contract。
- repeated violation 按 focus budget 收敛到 blocker/fail。

### S6: Evidence Binding Closure

统一 validation evidence metadata：

- shell / MCP / managed validation 都写同一结构化字段；
- coverage recompute 只接受结构化绑定；
- accepted validation 后 current focus 推进。

Done:

- live 中不再出现“validation 已执行但 ledger 不接受”的断裂。

### S7: Projection And Live Verification

补充 live 观测字段：

- workspaceId/runId/currentStep；
- current focus CWI；
- missing evidence kinds；
- deterministic NextAction；
- convergence contract state；
- requiredOutput mode；
- accepted/rejected evidence delta。

Done:

- live 验证能直接判断是否是正向推进，而不依赖人工阅读长日志。

## Verification Matrix

| Scenario | Expected result |
| --- | --- |
| Missing validation only | `NextAction=validate`, `need_validation`, concrete validation requiredOutput |
| Missing validation with stale acquire_evidence lineage | stale lineage diagnostic-only; dispatch stays validation |
| Missing target snapshot before state change | `acquire_target_snapshot`, read-only snapshot requiredOutput |
| Validation contract violation retry | same validation contract + rejected diagnostic + bounded retry |
| Repeated validation violation budget exhausted | governed blocker or terminal failure, no running/no-action state |
| Accepted validation evidence with CWI binding | coverage missing validation closes, next action assessment/complete |
| Accepted validation evidence without CWI binding | evidence kept, coverage not closed, next action remains validation |
| Projection view | ViewModel nextAction equals owner structured nextAction |

## Boundary Search

Before and after code closure:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|LSP|mcp-config|handler.go|main.go|provider|DeepSeek|OpenAI|Anthropic" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "acquire_evidence|acquire_concrete_failure_evidence|mustProduceConcreteFailureEvidence|mustProduceConcreteValidationEvidence|need_validation|need_evidence" internal/modules/kernel/orchestrator/biz -g "*.go"
rg -n "repairActionPackage|convergenceAuthority|gapBaseline|stagePacket|requiredOutput|coverageWorkItemContext" internal/modules/kernel/orchestrator/biz -g "*.go"
```

业务词搜索若命中 tests/fixtures 可以保留，但 runtime core 分支不能新增业务特判。

## Live Verification Plan

1. 启动最新 workorchd，记录 commit、pid、start time。
2. 对当前失败 workspace 执行 `/run resume` 或 decision replan。
3. 等待一个 dispatch 周期，抓取 projection 与 run detail。
4. 核对：
   - current focus 是否与 missing evidence 对齐；
   - nextAction 是否是 validate / state_change / governance / assessment 中的确定动作；
   - requiredOutput 是否与 nextAction 一致；
   - 是否产生 accepted/rejected evidence delta；
   - validation gap 是否实际触发 shell/MCP/managed validation 或明确 blocker。
5. 若仍失败，必须能归类为：
   - LLM candidate 合同违例且已 bounded retry；
   - validation 执行失败且 rejected evidence 入账；
   - 外部依赖/权限缺失进入 governance；
   - runtime internal contract misalignment fail-fast。

不得再出现：

- run 仍 running，但 current step failed 且无 blocker / no next action；
- current gap 是 validation，但 requiredOutput 是 generic failure evidence；
- assessment-only step 代替 validation；
- projection ready 但 TUI/Web 展示另一个 current action。

## Done Criteria

- validation-only coverage gap 在所有入口中都稳定派发 validation contract。
- generic `acquire_evidence` 不再作为 validation loop 默认兜底。
- dispatch 前能拒绝或重建互相矛盾的 authority/contract/requiredOutput。
- contract violation retry 不改变 current NextAction，只追加 rejected diagnostic 与 bounded retry hint。
- validation evidence accepted 后能结构化绑定 CWI 并推动 coverage recompute。
- ViewModel 展示的 nextAction 与 owner authority 一致。
- 新增回归测试覆盖 live 偏差，不含业务/语言/命令特判。

## Closure Progress

| Item | Status | Evidence |
| --- | --- | --- |
| S1 Freeze failing fixture | code-closed | Added `TestBuildConvergenceContractValidationFocusPreemptsStaleEvidenceAcquisition`, covering current focus missing only validation while stale authority/requiredOutput asks for generic evidence acquisition. |
| S2 Reorder contract derivation | code-closed | `populateCoverageWorkItemConvergenceContract` now handles validation/artifact gaps before `convergenceConcreteValidationCannotAdvanceWithoutTarget`; current validation focus no longer falls through to generic evidence acquisition. |
| S3 Split evidence acquisition subtypes | partial | Target snapshot acquisition already uses `acquire_target_snapshot`; generic evidence acquisition requiredOutput now carries `evidenceAcquisitionSubtype=diagnostic_probe`, and target snapshot acquisition carries `evidenceAcquisitionSubtype=target_snapshot`. Broader typed NextAction cleanup remains pending. |
| S4 Dispatch consistency gate | code-closed | `validateConvergenceDispatchRequiredOutputSemantics` rejects `need_validation` / `validate` paired with `mustProduceConcreteFailureEvidence` or equivalent concrete failure evidence requiredOutput. |
| S5 Retry recovery | partial | Contract derivation now prevents stale `repair/acquire_evidence` authority from overriding current validation focus. Forced-narrow `state_changing_output` now synchronizes `repairActionPackage` with the top-level state-changing contract and clears stale evidence-acquisition fields. Full focus-budget terminal blocker behavior remains out of this pass. |
| S6 Evidence binding closure | pending | This pass did not modify evidence record binding/recompute. |
| S7 Projection and live verification | pending | This pass did not modify projection/ViewModel or live observer output. |

## Live Verification

### 2026-06-29 Latest Deployed Service

Scope:

- `ws-705249a8d7816330/run-002`
- `ws-db040d24a190f6e5/run-002`
- workorchd `http://127.0.0.1:8080`

Service health:

- `/readyz` returned `ready`.
- Projection was aligned: `sourceEventRevision=1241`, `projectedSourceRevision=1241`.
- Two workspaces were projected.

Observed runtime states:

| Workspace | Run state | Current focus | Missing evidence | Authority next action | Contract | Outcome |
| --- | --- | --- | --- | --- | --- | --- |
| `ws-705249a8d7816330` | `blocked_on_decision` | `cwi-085` / `source_change` | `validation`, `state_change` | `request_decision` | `need_governance` | Repeated contract violation exhausted retry budget. |
| `ws-db040d24a190f6e5` | `blocked_on_decision` | `cwi-036` / `source_change` | `validation`, `state_change` | `request_decision` | `need_governance` | Repeated contract violation exhausted retry budget. |

Key findings:

- The current live runs do not exercise the primary fixed path from this plan:
  validation-only focus (`missingEvidenceKinds=["validation"]` without
  `state_change`) preempting stale `acquire_evidence`.
- No live snapshot showed `need_validation` / `validate` paired with
  `mustProduceConcreteFailureEvidence`; the specific dispatch contradiction
  closed by S1-S4 did not reappear in the observed state.
- Both current workspaces are blocked earlier on source-change delivery:
  the current focus still lacks `state_change` and `validation`.
- `ws-705249a8d7816330` failed after a state-changing attempt because
  `patch/apply` content contained redacted runtime markers.
- `ws-db040d24a190f6e5` failed on schema/contract issues including
  `json: unknown field "coverageFocusId"` and later concrete evidence
  acquisition contract violations.
- Both snapshots reported `stale_validation_resource_lifecycle` diagnostics
  for failed scratch resources.

Resource observations:

- workorch process `./bin/workorch` was observed around `17-20%` CPU and
  roughly `200-248MB` RSS during this window.
- Historical validation scratch proxy/localproxy processes from older
  workspaces remained alive under `/tmp/workorch-validation-scratch`.

Conclusion:

- S1-S4 are code-closed and did not regress in this live sample.
- The plan is not fully live-closed: S5 remains partial, and S6/S7 are still
  pending.
- A valid final live closure still requires a workspace that reaches a
  validation-only focus and then proves the next dispatch uses
  `NextAction=validate`, `ConvergenceContract.State=need_validation`, and
  `mustProduceConcreteValidationEvidence=true`.

### 2026-06-29 Follow-up Live Audit

Scope:

- `ws-705249a8d7816330/run-002`
- `ws-db040d24a190f6e5/run-002`

Observed state:

- Both workspaces remained `blocked_on_decision`.
- Both current focuses still lacked `state_change` and `validation` evidence,
  so they still did not exercise the validation-only path.
- The latest retry request had top-level authority aligned to state-changing
  repair:
  - `convergenceContract.state=need_state_change`
  - `repairObjective=produce_state_changing_output`
  - `requiredOutput.mustIncludeStateChangingNode=true`
  - `requiredOutput.acceptedKinds` included `patch/apply`
- The same request still carried stale repair-package lineage:
  - `repairActionPackage.repairObjective=acquire_concrete_failure_evidence`
  - `repairActionPackage.evidenceAcquisitionRequired=true`
- The graph validator then treated the step as concrete failure evidence
  acquisition and rejected candidate graphs with:
  `concrete failure evidence acquisition must return bounded file/read,
  file/list, git/read, shell/command, or mcp/tool_call executionGraph without
  patch/apply or nested llm nodes`.

Deviation:

- S5 was not fully closed. Forced-narrow state-changing retry rebuilt the
  top-level required output but did not clear stale `repairActionPackage`
  evidence-acquisition authority.
- This produced a split-brain contract: LLM-visible requiredOutput requested a
  state-changing graph, while runtime graph validation enforced failure
  evidence acquisition.

Owner:

- WorkOrch core contract recovery and graph validation.
- This is not a template, provider, workspace-specific, or TUI/Web issue.

Required closure:

- Forced-narrow `state_changing_output` normalization must set
  `repairActionPackage.repairObjective=produce_state_changing_output`.
- It must remove stale repair-package evidence-acquisition fields such as
  `evidenceAcquisitionRequired`, `concreteFailureEvidenceRequired`,
  `evidenceAcquisitionReason`, `outputDecisionContract`, and `exitCriteria`.
- `stepRequiresConcreteFailureEvidenceAcquisition` must treat current
  state-changing authority as dominant over stale repair-package lineage.
- Regression tests must prove a state-changing forced-narrow retry with stale
  package fields is not validated as concrete failure evidence acquisition.
