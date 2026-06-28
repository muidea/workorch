# Runtime Convergence Regression Root Cause Analysis

Status: active

Type: root-cause-analysis

Last Updated: 2026-06-28

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-progress-quality-resource-closure-plan.md`

## Purpose

本文用于解释：为什么 WorkOrch 已经过多轮 Goal/DoD、NextAction、convergence authority、contract retry、evidence ledger、projection、resource lifecycle 等完善后，live workspace 仍然没有实现快速、稳定、可信的正向推进。

本文不是新的单点修复计划，而是对近期所有核对结果的系统性归因。后续代码收口应先确认当前改动是否直接消除本文列出的根因，避免继续围绕某个 live workspace、某次 LLM 输出、某个提示词片段或某条验证命令追加局部补丁。

## Contract Audit

- Scope: `ws-7ced70e98ea50c43/run-012` live verification、coverage/evidence ledger、convergence authority、validation follow-up、resource usage。
- Observed state:
  - workspace 处于 `running`，projection revision 对齐，说明 TUI/Web 投影链路不是当前主 blocker。
  - run-012 产生了 `main.go` 和 `config.example.yaml` 修改，且部分 `go test ./...` 验证通过，说明不是完全无代码输出。
  - coverage summary 仍为 `total=89`、`satisfied=0`，但 `evidence_ready` 从 0 增长到 5，且多个 evidence_ready item 的 evidence refs 与目标不严格对齐。
  - `cwi-088/version_control_snapshot` 被标为 `evidence_ready`，但 accepted evidence 来自 `list-workspace-root` validation 和 `config.example.yaml` provider hint patch。
  - `cwi-085/source_change` 也复用了 `list-workspace-root` validation，并混入后续 broad validation refs。
  - `graph-004` 的 tool-call output 包含 `validationEvidence.targetRefs` 与 `observedClaims`，但 step summary/Input/Output 层显示 `validationEvidence=null`，导致后续绑定和上下文依赖弱字段。
  - resource 侧仍偏高：workorch 进程约 36-40% CPU、RSS 约 1GB，run 期间 write bytes 快速增长，validation scratch 仍为 GB 级。
- Expected state:
  - `Goal/DoD -> coverage ledger -> deterministic NextAction -> narrow Stage Packet -> candidate -> contract gate -> accepted/rejected evidence -> ledger recompute` 必须形成确定性闭环。
  - accepted evidence 必须强绑定当前 coverage work item、required evidence kind、target refs、observed claims 与 source refs。
  - LLM 输出、step metadata、coverageTargets、历史 repair lineage、projection 派生字段只能作为候选或诊断，不能单独驱动 accepted evidence。
  - evidence_ready 只能表示当前 CWI 的 required evidence 已被可信证据满足，随后应进入 assessment 或下一 deterministic action。
- Deviation:
  - runtime 仍允许弱匹配证据进入 accepted evidence，导致 ledger 表面推进但语义不可信。
  - 多个派生字段仍可携带旧 focus 或 broad coverage target，导致单一 authority 模型在落地时被旁路。
  - 验证结果的强证据字段没有完整贯穿到 step/ValidationResult/evidence record/coverage ledger/LLM context。
  - resource 优化没有消除高频 request/log/projection/evidence 文件写入和 validation scratch 压力。
- Owner: WorkOrch core。
  - 主问题是中立 coverage/evidence ledger 与 convergence authority 的实现缺陷。
  - LLM 输出不稳定、provider 503、template 粒度偏大是放大因素，但不是当前根因。
- Event path:
  - LLM candidate graph -> runtime step execution -> validation result/evidence record -> coverage reconciler/evidence reuse -> coverage work item status -> convergence authority refresh -> next action/projection。
- Contract gap:
  - accepted evidence gate 没有强制 current focus 精确绑定。
  - `coverageTargets` 与 step id token 被当成足够强的绑定来源。
  - tool-call output 中的 validation proof 没有作为唯一强证明贯穿后续 ledger。
  - evidence_ready 后的 deterministic assessment/next action 边界不够窄，允许继续扩散式验证。
- Boundary check:
  - 本文只描述通用状态机、证据、scope、target refs、observed claims、resource lifecycle，不引入软件开发、Go、文件名、provider 的 core 特判。

## Current Live Findings

### Progress Exists But Is Not Trustworthy

run-012 已经产生实际文件变更：

- `main.go`: 增加本地 provider routing 逻辑。
- `config.example.yaml`: 增加 provider 选择提示。

同时也产生了通过的验证：

- `graph-003/002-verify-go-test-with-absolute-cache`: `go test ./...` 通过。
- `graph-004/001-verify-protocol-compatibility-with-isolated-go-test`: isolated go test 通过，tool-call output 带有 target refs 和 observed claims。

因此问题不是“没有推进”，而是“推进质量和证据入账不可信”。如果继续只看 `patch/applied`、`step completed` 或 `evidence_ready` 数字，会误判为改进有效。

### Evidence Ready Contains False Positives

已确认的 false positive：

| CWI | Target | Actual accepted evidence | Deviation |
| --- | --- | --- | --- |
| `cwi-088` | `version_control_snapshot` | `list-workspace-root` validation + `config.example.yaml` patch | 普通目录列表和配置提示补丁不能证明版本控制快照完成 |
| `cwi-085` | `source_change` | `list-workspace-root` validation + 多个宽泛 validation refs | 只读 listing validation 不能作为 source change validation proof |
| `cwi-001` | 本地统一入口 | `read-main-go` validation | 读取源码只能作为 context evidence，不能单独证明目标行为满足 |

这些 false positive 会污染后续 assessment。即使最终 LLM 给出“已完成”评估，runtime 也会基于不可信 ledger 做判断，形成假收敛或后续反复修复。

### Provider Error Is Not The Main Cause

run-012 中出现过 provider 503：

```text
provider_unavailable
invalid argument: llm provider http status 503: Bad Gateway
```

runtime 随后执行 provider retry，并成功继续产生后续 graph。说明 provider 503 是局部扰动，不是“近期多轮仍不收敛”的主因。

### Resource Cost Still Indicates Loop Amplification

live 观察中 workorch 进程长期保持约 36-40% CPU、RSS 接近或超过 1GB，run 期间 write bytes 快速增长。结合 request log、evidence records、coverage work item projection、validation scratch 和 LLM context capture，可以判断当前执行仍在放大循环成本。

这类资源压力不是独立性能问题。它会降低 live 验证速度、放大 IO 抖动、增加 provider request 体积，并让每次错误绑定后的后续修复成本更高。

## Why Previous Improvements Did Not Produce Fast Convergence

### 1. We Fixed Many Symptoms Around The Loop, But The Acceptance Gate Still Lets Weak Evidence In

过去多轮收口强化了：

- Goal/DoD normalization。
- NextAction 推导。
- contract violation retry。
- replan/resume/decision 触发。
- projection/ViewModel 对齐。
- context budget 和 cache 分区。
- validation resource lifecycle。

这些改动改善了“能不能继续跑”“能不能展示正确状态”“违例能不能 retry”。但当前 live 样本证明，coverage/evidence ledger 的 accepted evidence gate 仍然过宽。

只要弱证据能进入 accepted evidence，后续所有权威对象都会被污染：

```text
weak accepted evidence
  -> coverage item evidence_ready
  -> authority selects next focus/action based on false baseline
  -> LLM receives noisy current progress
  -> follow-up expands validation/repair
  -> more weak evidence gets reused
```

这解释了为什么看起来每轮都修了一点，但最终没有稳定快速收敛。

### 2. `coverageTargets` Is Still Treated As Authority-Like Binding

当前 evidence record 和 step input 中存在 broad `coverageTargets`，例如：

```json
["source_change", "version_control_snapshot"]
```

这些字段本应是 LLM/step 的候选目标提示或上下文范围，不能单独证明 evidence 属于某个 CWI。但当前 `reconcileAcceptedEvidenceRecords` 与相关 binding 逻辑会根据 coverage refs、source refs、target refs 或 coverage targets 的交集复用 evidence。

当 `coverageTargets` 过宽时，一个普通 validation 或 patch 会被传播到多个 coverage item，造成 evidence_ready 膨胀。

### 3. Tool-Call Proof Does Not Become The Single Proof Source

`graph-004` 的 tool-call output 已经包含更可信的结构：

- `validationEvidence.mode`
- `validationEvidence.observationType`
- `validationEvidence.targetRefs`
- `validationEvidence.observedClaims`

但 step summary 层显示 `validationEvidence=null`。这说明强 proof 没有稳定贯穿到所有后续消费者。

结果是：

- evidence recorder 可以写出 accepted record。
- coverage reconciler / evidence reuse 仍可能依赖 step id、coverageTargets、summary、targetRef 等弱字段。
- LLM context 看到的可能是 compact summary，而不是完整 proof contract。

这会导致“验证做对了，但 ledger 绑定仍可能做错”。

### 4. Convergence Authority Was Simplified, But Derived Fields Still Leak Back

`convergence-authority-simplification-design.md` 的目标是：

```text
Coverage/Evidence Ledger -> Current Focus -> Deterministic NextAction
```

但 live 输入中仍看到：

- `acceptance.currentFocus` 与 authority focus 存在 stale lineage。
- `coverageFocus`、`budgetScope`、`repairActionPackage`、`coverageTargets` 可携带不同 focus。
- provider retry / graph follow-up 会继承旧 repair context 与 broad target set。

这意味着“单一权威”在设计上已经明确，但实现里仍有多个派生字段能影响下一轮输出或证据绑定。

### 5. Validation Follow-Up Still Allows Expansion Instead Of Closure

当某个 focus 已经生成 validation evidence 后，理想路径是：

```text
validation evidence accepted
  -> recompute current CWI
  -> if required evidence complete: assessment
  -> else next missing evidence kind
```

当前 live 过程更接近：

```text
validation evidence accepted or partially accepted
  -> LLM follow-up
  -> generate another validation for next broad target
  -> evidence_ready count grows
  -> no satisfied item yet
```

这会让系统看起来“持续推进”，但并没有快速关闭当前 slice。特别是在 DoD 很多、目标很长时，这种扩散会显著变慢。

### 6. Resource Closure Reduced Some Leaks, But Did Not Eliminate High-Frequency IO

前序 resource 收口主要解决了：

- 部分 validation scratch release。
- 部分 managed validation resource metadata。
- projection payload on-demand。
- request compaction。

但当前仍有大量写入来源：

- LLM request/response capture。
- events/payloads/checkpoints。
- evidence records。
- coverage work item projection files。
- validation scratch/cache/log。
- read model/projection refresh。

这些写入在错误收敛循环中被持续放大。只做资源清理不能解决根因；必须先减少错误 evidence 绑定和不必要 follow-up。

## Root Cause Summary

根因不是单一 bug，而是三层缺口叠加：

1. **Evidence binding is too permissive.**
   - accepted evidence 可以由 broad coverage target、step id token 或弱 targetRef 触发。
   - current focus 精确绑定不是硬门禁。

2. **Authority simplification is incomplete in implementation.**
   - 设计上已有单一 authority。
   - 实现上仍有旧 focus、旧 repair package、coverageTargets、acceptance currentFocus 等派生字段影响后续 dispatch 和 binding。

3. **The loop optimizes continuation before correctness.**
   - provider retry、contract retry、validation follow-up、repair follow-up 都能让 run 继续。
   - 但继续之前没有强制证明 ledger 变更可信，导致“会跑”优先于“收敛正确”。

## Required Closure Principles

后续改动必须按以下原则收口，否则容易继续打圈。

### Principle 1: Accepted Evidence Requires Exact Binding

accepted evidence 必须满足至少一类强绑定：

- explicit `coverageItemRefs` 精确包含当前 CWI id。
- explicit `sourceRequirementIds` 精确匹配当前 CWI source requirement。
- explicit `targetRefs` 与当前 CWI 的 declared target refs 精确或结构化匹配。
- template/profile policy 声明该 evidence kind 可跨 item 复用，并且该复用是结构化、可审计的。

`coverageTargets`、step id、summary、targetText fuzzy match、LLM 自述不能单独作为 accepted evidence 绑定依据。

### Principle 2: Weak Evidence Goes To Supporting Or Diagnostic Ledger

不满足强绑定但可能有参考价值的 evidence 应进入：

- `supportingEvidenceByKind`
- `rejectedEvidenceByKind`
- diagnostic evidence

它们可以进入 LLM context，但不能把 CWI 推到 `evidence_ready`。

### Principle 3: Tool-Call Validation Proof Must Be Preserved End-To-End

validation proof 必须从 adapter 输出贯穿到：

- tool-call output
- step output/summary
- ValidationResult
- EvidenceRecord
- CoverageWorkItem binding
- Stage Packet / C4 evidence context

缺少 `targetRefs` 或 `observedClaims` 的 validation 只能作为 supporting evidence，除非 profile policy 显式允许该验证类型作为全局 smoke evidence。

### Principle 4: Current Focus Is The Default Binding Scope

runtime step 的 accepted evidence 默认只能绑定到当前 authority focus。要跨 focus 绑定，必须有显式 `coverageItemRefs` 或结构化 batch policy。

这可以防止一个 broad validation 自动满足多个无关 CWI。

### Principle 5: Evidence Ready Must Trigger Deterministic Closure

当 current focus required evidence complete 后，runtime 应确定性进入 assessment 或下一个 missing evidence kind。不能默认再请求 LLM 做开放式 follow-up。

LLM follow-up 只在以下情况允许：

- 当前 NextAction 仍需要 LLM 生成候选内容。
- ledger 无法确定下一步。
- validation proof 不完整，需要 LLM 生成更窄的 validation graph。

### Principle 6: Resource Work Follows Correctness

资源优化优先级应服务于收敛正确性：

1. 先阻断 false positive evidence binding，减少无效循环。
2. 再减少 follow-up LLM 调用。
3. 再优化 projection/request/evidence 文件写入。
4. 最后处理历史 scratch/orphan 清理。

## Proposed Closure Scope

### P0: Evidence Binding Hard Gate

Implementation status: completed for the first live-verification closure pass.

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`

Expected changes:

- `coverageTargets` 不再单独触发 accepted evidence 复用。
- accepted evidence 绑定必须通过 exact CWI/sourceRequirement/targetRef proof。
- weak matched evidence 写入 supporting/diagnostic，不改变 `missingEvidenceKinds`。
- 新增失败样本测试：`version_control_snapshot` 不能被 `list-workspace-root` validation 或 unrelated config patch 满足。

Implemented behavior:

- `reconcileAcceptedEvidenceRecords` now applies accepted evidence through proof-strength options instead of bypassing the ledger gate.
- `coverageTargets` and `coverageTargetTokens` are no longer accepted-reuse binding sources; explicit `coverageItemRefs`, `sourceRequirementIds`, `coverageTargetIds`, or target refs are required.
- supporting-only reuse no longer increments the accepted `bound` count or writes accepted reuse edges.
- validation proof alignment no longer treats `targetText` as a sufficient target ref candidate.

### P1: Validation Proof Propagation

Implementation status: completed for the first live-verification closure pass.

Scope:

- runtime adapter output normalization。
- `recordValidationResult` / `runtime_step_evidence` / evidence record creation。
- step summary / compact context rendering。

Expected changes:

- tool-call output 中的 `validationEvidence` 写入 step output 或 validation metadata。
- `targetRefs` 和 `observedClaims` 成为 accepted validation 的必要 proof。
- 缺 proof 的 validation 不能进入 accepted ledger。

Implemented behavior:

- runtime step coverage scope now propagates `validationEvidence.targetRefs`, `observedClaims`, `mode`, `observationType`, `proofKind`, and `coverageAlignment` into validation evidence records.
- coverage validation reconciliation now derives proof from the runtime step validation evidence instead of treating ValidationResult summary as an observed claim.
- passed runtime-step validation without structured target refs and observed claims is retained as supporting evidence only.

### P2: Current-Focus-Scoped Binding

Implementation status: pending.

Scope:

- convergence authority refresh。
- dispatch input rebuild。
- coverage reconciler。

Expected changes:

- 默认只绑定当前 authority focus。
- 批量绑定必须由结构化 batch policy 声明。
- provider retry / graph follow-up 不能携带 broad `coverageTargets` 覆盖 current focus。

### P3: Evidence-Ready Deterministic Transition

Implementation status: pending.

Scope:

- NextAction interpreter。
- graph follow-up dispatch。
- assessment gate。

Expected changes:

- current focus evidence_ready 后，优先 assessment。
- assessment 不需要 LLM 时，runtime 直接执行 deterministic assessment。
- 禁止在 evidence_ready 后继续开放式验证扩散，除非 policy 明确 batch validation。

### P4: Resource Loop Cost Reduction

Implementation status: pending.

Scope:

- LLM request capture。
- evidence/projection file writes。
- validation scratch lifecycle。

Expected changes:

- 对未改变语义的 heartbeat/projection 不重复写大 payload。
- coverage item projection 批量/增量写入。
- validation scratch 默认 step-scoped cleanup，并提供 orphan reconciliation。
- live 验证时记录 CPU/RSS/read_bytes/write_bytes delta。

## Verification Requirements

每项收口必须提供至少一类证据：

- Unit test:
  - unrelated validation 不可满足 `version_control_snapshot`。
  - broad `coverageTargets` 不可单独绑定 accepted evidence。
  - missing `observedClaims` 或 `targetRefs` 的 validation 只能成为 supporting evidence。
  - current focus evidence_ready 后 next action 是 assessment。
- Integration test:
  - provider retry 后不继承 stale broad coverageTargets。
  - graph follow-up 不绕过 current focus binding。
- Live verification:
  - evidence_ready 增长必须能逐条追溯到 exact proof。
  - false positive 样本不再出现。
  - CPU/RSS/write_bytes 增长明显下降。

## Non-Goals

- 不通过放宽 completion gate 加快表面完成。
- 不通过在 core 中加入软件开发、Go、文件名、provider 或 live workspace 特判修复样本。
- 不依赖 LLM 自觉声明“这是强证据”。
- 不用 prompt 约束替代 runtime accepted evidence gate。
- 不把 projection 展示修正当成 owner state 修复。

## Current Conclusion

经过多轮完善后仍未实现快速正向推进，是因为系统已经具备“继续运行”的能力，但尚未严格保证“每次继续都基于可信 ledger”。当前 runtime 可以 retry、follow-up、验证、投影和继续切换 focus，但 accepted evidence gate 仍允许弱证据污染 coverage ledger。

因此下一阶段的收口重点不是继续增加 prompt、扩大上下文、提高 retry 次数或补更多 TUI/projection 细节，而是先收紧 evidence binding 与 current focus authority：

```text
no exact proof -> no accepted evidence
no accepted evidence -> no evidence_ready
no evidence_ready -> no assessment/complete
```

只有这一层闭合后，后续资源优化、prompt 优化、template 粒度优化和 live convergence acceleration 才会产生稳定收益。
