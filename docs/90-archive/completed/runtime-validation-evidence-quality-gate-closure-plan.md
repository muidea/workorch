# Runtime Validation Evidence Quality Gate Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-evidence-proof-strength-and-assessment-closure-plan.md`
- `runtime-structured-evidence-authority-closure-plan.md`
- `runtime-progress-quality-resource-closure-plan.md`

## Purpose

本文用于收口最新 live verification 暴露的 runtime validation evidence gate 偏差：WorkOrch 已经能通过 EventHub/replan 主路径继续执行，也能生成 Stage Packet、contract retry 和 validation evidence，但 runtime 仍可能把证明强度不足或与目标不匹配的验证输出作为 accepted evidence 写入 coverage ledger，导致 workspace 看似推进、实际没有向 Goal/DoD 收敛。

本文只定义业务中立的 core 修复方案。具体软件开发、provider routing、usage/cost、fallback 等目标语义仍由 workspaceTemplate/profile/Soul/policy/DoD 提供结构化要求，core 只解释结构化合同、证据质量、断言结果和 ledger 状态。

当前阶段不考虑兼容性。若现有代码、测试或提示词与本文冲突，应按本文收口。

## Contract Audit

- Scope: `ws-7ced70e98ea50c43` / `run-013-replan-001` 的 validation/follow-up/contract-retry 链路，以及对应 core 代码路径：
  - `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
  - `internal/modules/blocks/shell/biz/shell_runtime_adapter.go`
- Observed state:
  - EventHub/replan/dispatch 主路径正常，projection source revision 与 projected revision 对齐。
  - `graph-005` shell exit 0，被绑定到 `cwi-003` accepted validation evidence，但 stdout 显示 `usage_covered_hits=0`、`interaction_covered_hits=0`，实际是负向验证结果。
  - `graph-006` shell exit 0，被绑定到 `cwi-005` accepted validation evidence，但 stdout 只证明 source hits 与少量 covered hits，不能证明目标要求的完整行为。
  - follow-up LLM 返回的 executionGraph 已包含 `validationEvidence.mode`、`observationType`、`targetRefs`、`observedClaims` 与字符串形式 `substitute`，但 runtime 报 `validation_evidence_metadata_missing`，因为当前实现只把 map 形式 `substitute` 或字符串 `substituteFor` 视为 controlled substitute 说明。
  - 单轮 LLM retry 请求可达到约 1.47MB / 78k tokens / 466s，cache read/write 为 0；弱证据被接受后会放大后续上下文与资源消耗。
- Expected state:
  - Contract gate 与对外 prompt/schema 对同一字段的解释必须一致。
  - shell/mcp validation evidence 只有在结构化合同、执行结果、声明断言和实际观测都满足时才能进入 accepted evidence。
  - 负向或不足证明强度的 validation output 必须成为 rejected diagnostic evidence 或 inconclusive evidence，不能满足 coverage。
  - Coverage ledger 的 `status`、`acceptedEvidenceRefs`、`currentGap` 必须由同一次 evidence quality verdict 重算，不能出现 `failed` 但 gap 文案表示 reused accepted evidence 的矛盾。
- Deviation:
  - Candidate contract gate 只校验 validationEvidence 字段存在性，未校验声明与观测的一致性。
  - Evidence quality gate 只识别 stdout 整体为 `0/false/null/[]/{}` 的负向标量，无法识别结构化 `key=value` 中的失败、零命中或未满足断言。
  - Coverage reconciler 只要 validation result passed 且 quality valid，就按 target token 绑定 coverage，缺少 proof strength / assertion verdict / output metric verdict 的统一输入。
  - controlled substitute 字段合同实现与输出指导不一致，导致 LLM 已返回合理字段仍被误判为合同缺失。
- Owner: WorkOrch core。问题位于中立 runtime contract、evidence quality、coverage ledger reconciliation 和 shell/mcp block structured output interpretation，不属于软件开发模板、provider、语言或具体文件名特判。
- Event path:
  - `LLM step -> validateLLMExecutionOutputContract -> executionGraph admission -> shell/mcp runtime step -> validation result -> evidence record -> coverage reconciler -> convergence authority refresh -> Stage Packet / projection`。
- Contract gap:
  - 缺少统一的 `ValidationEvidenceVerdict` 或等价结构，把字段合同、执行状态、声明断言、观测指标、证明强度和 coverage binding 合并为一个 accepted/rejected verdict。
  - 缺少中立 `requiredAssertions` / `requiredMetrics` / `successCriteria` schema，使 template/policy 无法把业务要求数据化给 core 执行。
  - 缺少 evidence quality 与 coverage ledger 的单一重算入口，导致 accepted evidence、item status、currentGap 可能短暂或持久不一致。
- Boundary check:
  - core 修复只能处理通用字段，如 `validationEvidence`、`observedClaims`、`requiredAssertions`、`metrics`、`verdict`、`evidenceQuality`、`targetRefs`、`coverageWorkItemIds`。
  - core 禁止加入 `provider`、`fallback`、`usage`、`interaction`、`go test`、文件名、workspaceId、templateId 或 live marker 特判。
  - 具体目标如何验证由 workspaceTemplate/profile/policy 将要求声明成结构化 criteria；core 只判断 criteria 是否满足。
- Verification:
  - 新增失败样本单测，证明旧的弱证据不会再绑定 coverage。
  - 新增合同字段单测，证明字符串 `validationEvidence.substitute` 或 `substituteFor` 均按合同通过。
  - 新增 reconciler 单测，证明 validation result passed 但 assertion/metric verdict failed 时不会 accepted。
  - 新增边界搜索，确认 core 没有新增业务/语言/provider 特判。

## Design Alignment

本文是对 `Goal/DoD NextAction Delivery State Machine Design` 与 `Convergence Authority Simplification Design` 的补充，不改变它们的主状态机：

```text
Goal/DoD Definition
  -> Coverage/Evidence Ledger
  -> Current Focus
  -> Deterministic NextAction
  -> Stage Packet
  -> Candidate Generation
  -> Contract Gate
  -> Evidence Quality Gate
  -> Accepted/Rejected Evidence
  -> Ledger Recompute
  -> Projection
```

补充点是把 `Contract Gate -> Accepted/Rejected Evidence` 中间显式拆出 `Evidence Quality Gate`。该 gate 是 core 通用能力，负责将 validation candidate 的执行结果转为确定性 verdict。

## Target Model

### Validation Evidence Contract

`validationEvidence` 是候选验证节点的声明合同：

```text
ValidationEvidenceContract
  mode
  observationType
  targetRefs[]
  coverageWorkItemIds[]
  observedClaims[]
  substitute
  substituteFor
  requiredAssertions[]
  requiredMetrics[]
  successCriteria[]
```

规则：

- `mode` 必须是 core 已知的中立模式，例如 `runtime_behavior`、`controlled_substitute`、`executable_contract`、`dynamic_execution`。
- `observationType` 必须是对应的 concrete observation 类型。
- `targetRefs` 或 `coverageWorkItemIds` 必须能绑定到当前 focus 或 coverage target。
- `observedClaims` 是 LLM 声明，不等于已验收事实。它必须经 output/verdict 支撑后才能进入 accepted evidence metadata。
- `controlled_substitute` 必须有 `substituteFor` 或非空 `substitute`。`substitute` 可为字符串或对象；若需要结构化细节，template/policy 可额外提供对象字段。
- `requiredAssertions`、`requiredMetrics`、`successCriteria` 是可选增强字段；一旦声明，core 必须执行通用判定，失败则 evidence 不可 accepted。

### Validation Evidence Verdict

Runtime/block adapter 应产生或归一化出中立 verdict：

```text
ValidationEvidenceVerdict
  state                 # passed | failed | inconclusive
  quality               # valid | invalid | inconclusive | smoke | unrelated
  reason
  targetRefs[]
  observedClaims[]
  metrics{}
  assertionResults[]
  failedCriteria[]
  tags[]
```

规则：

- `state=passed` 只表示 runtime step 进程/工具执行成功，不自动等于 `quality=valid`。
- `quality=valid` 必须同时满足：
  - runtime/block 执行成功；
  - validationEvidence 合同完整；
  - required assertions/metrics/success criteria 全部满足；
  - proof strength 足以满足目标 required evidence kind；
  - 没有显式 negative structured output。
- `quality=inconclusive` 或 `invalid` 不能满足 coverage，但可以写 rejected diagnostic evidence。
- `smoke` 只能作为支持性证据，不能单独关闭 runtime behavior coverage。

### Structured Criteria

Core 只支持中立比较，不理解业务文本：

```text
RequiredMetric
  name
  op        # exists | eq | ne | gt | gte | lt | lte | nonzero | true | false
  value
  source    # stdout_key_value | output.metrics | validationEvidence.metrics | adapter
```

```text
RequiredAssertion
  id
  description
  source
  expected
```

模板或 policy 可以把“必须覆盖某类行为”“必须产生某类归档”“必须验证 fallback 成功”等转为中立 criteria。Core 不写这些具体词，只执行 criteria。

## Closure Work Packages

### P0: Contract Field Consistency

Owner: core contract gate

Files:

- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract_test.go`
- existing graph/follow-up contract tests as needed

Changes:

- 将 `controlled_substitute` 的 substitute 判定统一为：
  - `substituteFor` 非空字符串，或
  - `substitute` 非空字符串，或
  - `substitute` 非空对象。
- 把 validationEvidence 字段校验拆成可返回具体缺失字段的 helper，避免所有缺陷都报同一个 metadata missing。
- 更新 prompt/schema 示例时保持字段与 runtime 判定一致。

Acceptance:

- 字符串 `validationEvidence.substitute` 不再触发 `validation_evidence_metadata_missing`。
- 缺少 mode、observationType、targetRefs、observedClaims、controlled_substitute substitute 信息时分别有稳定 rule/reason。
- 现有合同违例仍只作为 rejected diagnostic evidence，不执行 side effect。

### P1: Evidence Quality Verdict

Owner: core evidence quality / shell block adapter

Files:

- `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality.go`
- `internal/modules/blocks/shell/biz/shell_runtime_adapter.go`
- `internal/modules/blocks/shell/biz/evidence_policy.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- corresponding tests

Changes:

- 新增或归一化 `ValidationEvidenceVerdict` 结构，至少覆盖 `quality`、`reason`、`metrics`、`assertionResults`、`failedCriteria`、`tags`。
- shell adapter 从 output 中提取通用 structured metrics：
  - 优先读取 `output.metrics` / `validationEvidence.metrics`；
  - 可将 stdout 中明确的 `key=value` 行解析为中立 metrics；
  - 不按 key 名业务含义判断，只按 criteria 判断。
- `evidenceQualityFromRuntimeStep` 不再只做整体 scalar negative 检测；应调用 verdict evaluator。
- validation result 的 `EvidenceQuality` 必须来自 verdict evaluator。没有 verdict 且没有 criteria 时，仍可按现有质量规则处理，但 proof strength gate 不能被绕过。

Acceptance:

- shell exit 0 但 criteria failed 时，validation result 是 `inconclusive` 或 `invalid`，不能 accepted。
- shell exit 0 且 stdout 包含 `some_required_metric=0`，只有当 criteria 声明该 metric 必须 nonzero/gte 时才失败；core 不因为名字本身失败。
- 无 criteria 的普通 smoke/diagnostic validation 不被误判为可关闭 runtime behavior coverage，除非 proof kind 与 coverage requirement 明确允许。

### P2: Coverage Binding Requires Quality Verdict

Owner: coverage ledger / convergence authority

Files:

- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_coverage.go`
- coverage work item repository/projection tests as needed

Changes:

- `reconcileValidationAcceptable` 必须检查 structured verdict，而不是只检查 validation state passed + quality valid 默认值。
- `coverageEvidenceAcceptanceOptions` 增加 verdict/proof strength/criteria result，coverage apply 时统一判断。
- `currentGap`、`status`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs` 在同一次 reconcile 中重算，禁止出现 status failed 但 gap 表示 reused accepted evidence 的矛盾。
- accepted evidence 更新后立即触发 authority refresh 或标记需要刷新，确保 NextAction 不基于旧 gap 推进。

Acceptance:

- 旧样本 `exit 0 + negative metrics` 不会把 coverage item 变成 `evidence_ready` 或 `satisfied`。
- `status` 与 `currentGap` 一致：没有 accepted validation 时 gap 必须仍表示 missing validation；accepted validation 被拒绝时 rejected refs 可见。
- projection 只展示 owner ledger 结果，不二次修正 evidence 状态。

### P3: Template/Policy Criteria Binding

Owner: workspaceTemplate/profile/Soul/policy

Files:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- template-bound evidence policy fixtures/tests

Changes:

- 对需要强验证的 workspaceTemplate，提供结构化 `evidencePolicies` 或 equivalent criteria profile。
- 软件开发模板可声明：
  - validation 必须绑定当前 coverage focus；
  - runtime behavior 不能只靠 source inspection；
  - 当目标需要行为验证时，criteria 应由模板/policy 或 LLM graph 明确声明。
- 不把具体目标词写入 core。模板只把业务目标转成 data-driven criteria。

Acceptance:

- workspace 创建时 template policy 被固化到 execution context。
- LLM input 中能看到当前 focus 的 required criteria 或 policy refs。
- core tests 中使用中立 fixture；template tests 可覆盖软件开发专用策略。

### P4: Resource Guard For Failed Validation Loops

Owner: runtime resource lifecycle

Files:

- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/blocks/shell/biz/shell_runtime_adapter.go`
- resource lifecycle tests

Changes:

- validation scratch 必须绑定 workspace/run/step scope 和 retention policy。
- 对 managed validation / controlled substitute 产生的进程、端口、临时目录，记录 cleanup handle。
- step terminal 后执行幂等 cleanup；cleanup 失败进入 diagnostic，不阻塞 ledger 重算。
- repeated weak-evidence loop 达到预算时停止继续放大 LLM 请求，进入 deterministic fail/governance，而不是继续发大上下文 retry。

Acceptance:

- 失败 validation 不遗留长驻 proxy/localproxy 进程。
- scratch 目录增长受 retention 控制。
- 同一 focus 重复 weak evidence 超预算后不再触发无限 LLM follow-up。

## Implementation Order

1. P0 field consistency：先修复合同实现与提示不一致，避免正确输出被错误 retry。
2. P1 evidence quality verdict：建立中立 verdict evaluator，阻止弱证据进入 accepted。
3. P2 coverage binding：把 verdict 接入 coverage reconciler，并修复 status/currentGap 一致性。
4. P3 template/policy criteria：让模板把强验证要求数据化进入 runtime。
5. P4 resource guard：限制失败验证循环的资源放大。

每一步必须有独立测试，不能等全部改完后只做 live verification。

## Required Tests

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'ValidationEvidence|EvidenceQuality|CoverageReconciler|ContractViolation|Convergence' -count=1
go test ./internal/modules/blocks/shell/biz -run 'ValidationEvidence|EvidencePolicy|RuntimeAdapter' -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
```

Full tests before live verification:

```bash
go test ./... -count=1
git diff --check
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|provider|fallback|usage|interaction|workspaceId|ws-" internal/modules/kernel internal/pkg -g '*.go'
rg -n "validationEvidence|requiredAssertions|requiredMetrics|successCriteria|EvidenceQuality|currentGap|acceptedEvidenceRefs" internal/modules/kernel internal/modules/blocks -g '*.go'
```

The first search may find existing legitimate strings in tests or docs; any new core production-code match must be justified or moved to template/policy.

## Live Verification Plan

After code closure:

1. Start new workorch binary and record git commit, pid, start time and provider config.
2. Trigger or resume one workspace with at least one validation-focused coverage item.
3. Confirm command accepted -> authority refresh -> dispatch path has no projection backlog.
4. Force or observe one invalid validation evidence candidate:
   - missing validationEvidence field should become rejected diagnostic + bounded retry;
   - criteria failed should become invalid/inconclusive validation evidence, not accepted coverage.
5. Observe one valid validation evidence candidate:
   - evidence record has verdict/quality/criteria results;
   - coverage item accepted refs and currentGap update consistently;
   - NextAction moves to assessment/state_change/complete according to ledger.
6. Observe CPU/RSS and scratch directory growth across at least two failed validation cycles.

## Done Criteria

- Contract field interpretation matches prompt/schema and tests.
- Validation evidence acceptance depends on deterministic verdict, not shell exit 0 alone.
- Structured negative or failed criteria outputs cannot satisfy coverage.
- Coverage item status、currentGap、accepted/rejected refs come from one consistent recompute.
- Template-specific validation expectations are data-driven and do not appear as core production-code branches.
- Repeated weak evidence loops stop by budget/governance and do not keep amplifying context/resource usage.

## Non-Goals

- 不在 core 中 hardcode 当前 live workspace 的 `usage_covered_hits`、`provider_covered_hits`、`fallback`、`interaction` 等字段名。
- 不通过放宽合同让 LLM 违例直接执行。
- 不把 shell stdout 自然语言总结当作 accepted evidence。
- 不让 projection 修复 owner ledger 状态。
- 不用更多 prompt 文本替代结构化 criteria 和 verdict。
