# Focus Scoped Assessment, Managed Validation, And Context Closure Plan

Status: implemented

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-convergence-closure-plan.md`
- `runtime-validation-evidence-context-closure-plan.md`
- `runtime-contract-retry-convergence-closure-plan.md`

## Purpose

本文固化 2026-06-27 live verification 中暴露的最新偏差，并把后续代码收口拆成可逐项验收的任务。

目标不是继续放宽合同或扩大 prompt，而是让 WorkOrch 按既有设计做到：

- `NextAction` 与 `currentFocus` 是当前收敛唯一权威。
- LLM 只处理当前 narrow action。
- 局部 coverage assessment 可以被接受并入账，不被全量 Goal/DoD completion gate 误拒。
- 服务类 runtime behavior validation 由 template/Soul/policy 引导优先使用 `validation/managed_session`，不诱导后台 shell + kill。
- LLM 上下文围绕当前 focus 裁剪，避免大量历史 evidence refs 和全量 DoD 干扰当前 requiredOutput。

当前阶段不考虑兼容性。若现有实现、测试或端侧行为与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65` active run `run-006-replan-001-replan-001-replan-001-replan-001`
  - `ws-7ced70e98ea50c43` active run `run-005-replan-001-replan-001-replan-001-replan-001`
  - runtime DoD gate、coverage ledger、contract retry、validation evidence contract、LLM context rendering。
- Observed state:
  - 两个 workspace 均已进入 `waiting_decision`，projection 已追平 owner event revision。
  - `ws-4e...` 的 current focus 为 `cwi-001/evidence_ready`，NextAction 语义是 assessment；LLM 返回 `dodAssessment` 后被 DoD gate 以 “does not cover all current DoD items” 拒绝。
  - `ws-7...` 的 current focus 为 `cwi-012/open`，NextAction 语义是 validation；LLM 返回 shell graph，命令包含后台启动和 `kill $PROXY_PID`，被 runtime 以 non-destructive bounded validation 合同拒绝。
  - 两个 run 都在第三次 contract retry 后触发 `replan_lineage_budget_exhausted` 并升级 decision。
  - 停到 decision 后 event revision 不再增长，`.workorch-data` 近 1 分钟写入降为 0，说明当前主要问题不是投影持续写循环。
- Expected state:
  - 当 current focus 是单个 coverage work item 的 `assessment` action 时，runtime 应只校验该 focus 的 coverage/evidence/ref 绑定，并将 accepted assessment 入账到对应 coverage item。
  - 只有 final completion action 才能要求全量 Goal/DoD/deliverable coverage 全部 satisfied。
  - 服务类验证不应通过 shell 后台进程和手动 kill 实现；需要服务生命周期时，应由 `validation/managed_session` 或等价 runtime-managed validation 能力管理启动、timeout、端口和 cleanup。
  - LLM request 应优先渲染 current focus、相关 evidence、current requiredOutput 和当前 rejected diagnostic；全量历史 evidence 只应作为 summary/ref。
- Deviation:
  - `focus-scoped assessment` 被全量 DoD gate 拒绝，导致局部收敛无法落账，随后重复 contract retry 并进入 decision。
  - `validation/managed_session` 虽然在 requiredOutput 中可用，但 template/Soul/policy 对服务类验证的优先级和示例不够强，LLM 继续选择不合规 shell 图。
  - assessment retry request 仍包含大量历史 accepted refs 和完整 definition/context，容易淹没当前 narrow action。
- Owner:
  - Focus-scoped assessment: `core`，属于 runtime coverage ledger、DoD gate 和 NextAction completion gate 的中立状态机缺口。
  - Managed validation guidance: `workspaceTemplate/Soul/policy` 为主，core 只提供中立 `validation/managed_session` 能力和合同门禁。
  - Context focus slicing: `LLM context/provider` 与 `core context renderer`，必须保持 C0/C1/C2 稳定边界。
- Event path:
  - decision replan -> command accepted -> successor run dispatch -> convergence authority refresh -> Stage Packet -> LLM candidate -> contract gate -> rejected diagnostic evidence -> bounded retry -> budget exhausted -> decision ticket。
- Contract gap:
  - DoD gate 缺少 `focus-scoped assessment` 与 `final completion assessment` 的边界。
  - 服务类 validation 缺少 profile/template-bound managed-session 优先策略。
  - LLM context 缺少按 current focus 渲染 accepted/rejected evidence refs 的裁剪规则。
- Boundary check:
  - 本计划不允许在 core 写入 template id、Soul id、语言、命令、文件名、workspace title 或 live workspace id 特判。
  - 软件开发验证偏好只进入 template/Soul/policy；core 只消费结构化 `requiredOutput`、`validationEvidence`、`managed_session` 和 evidence kind。
- Verification:
  - 增加失败样本单测覆盖 focus-scoped assessment 被接受、全局 completion 仍需所有 coverage item ready。
  - 增加 template/Soul/policy 或 prompt rendering 测试，证明服务类 validation 优先暴露 `validation/managed_session` 示例。
  - 增加 context rendering 测试，证明 assessment action 只渲染 current focus 相关 evidence refs，历史 evidence 通过 summary/ref 出现。
  - live verification 证明局部 assessment 后进入下一 coverage item，而不是直接 `waiting_decision`。

## Design Alignment

### Goal/DoD NextAction State Machine

`goal-dod-nextaction-delivery-state-machine-design.md` 要求：

- Coverage ledger 与 evidence ledger 是收敛权威。
- Runtime 根据 ledger 生成唯一 NextAction。
- LLM 每轮只处理当前 narrow action。
- `assessment` 只能说明判定过程，不能替代缺失的 state_change 或 validation evidence。
- `complete` 必须基于所有 required evidence 满足。

因此，局部 `assessment` 的正确结果不是直接 complete run，而是：

```text
current focus evidence_ready
  -> NextAction assessment
  -> LLM returns focus-scoped dodAssessment
  -> gate validates only current focus mapping and evidence refs
  -> accepted assessment evidence recorded
  -> coverage item status becomes satisfied/assessment_accepted
  -> authority recomputes next focus or complete
```

只有当 `deriveAllSatisfiedCompleteNextAction` 判断所有 coverage work items completion-ready 时，runtime 才进入 final completion gate。

### Convergence Authority Simplification

`convergence-authority-simplification-design.md` 要求：

- `stagePacket`、`repairActionPackage.requiredOutput`、`gapBaseline`、`convergenceContract` 都从同一 authority snapshot 派生。
- 旧 repair lineage 只能作为 diagnostics。
- dispatch 前必须以 current authority focus 重新绑定 requiredOutput。

因此，DoD gate 不能在一个 focus-scoped Stage Packet 上强行使用 run-level 全量 DoD 作为当前 action 的合同。全量 DoD gate 只能属于 final completion action。

## Closure Work Items

### C1: Core Focus-Scoped Assessment Gate

Owner: core

Implementation status: done

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_dod.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- related tests in `internal/modules/kernel/orchestrator/biz/*test.go`

Required changes:

- Add a helper that detects whether the current step is a focus-scoped assessment:
  - `runtimeStage=verification`
  - `repairObjective=complete_dod_goal_coverage_assessment`
  - `assessmentOnly=true`
  - current `stagePacket.activeFocus` or `coverageWorkItemContext.currentFocus` is non-empty
  - current NextAction/requiredOutput is assessment, not final complete.
- For focus-scoped assessment:
  - validate `dodAssessment.items` or typed coverage containers against current focus only.
  - require evidence refs to map to accepted evidence of that focus.
  - do not require all run-level Goal/DoD/deliverable items in the same response.
  - record accepted assessment evidence and update the current coverage item.
- For final completion:
  - keep existing full `validateReadyForAcceptance` and all run-level DoD/Goal/deliverable gates.
  - do not allow focus-scoped assessment to mark the run complete.

Acceptance tests:

- A run with two coverage work items, current focus `cwi-001/evidence_ready`, and another open item should accept a focus-scoped assessment for `cwi-001`.
- The same run must not complete until all items are completion-ready.
- A final completion action still rejects missing run-level DoD / Goal / deliverable coverage.
- A focus-scoped assessment without evidence refs or with refs outside current focus is rejected as diagnostic evidence.

### C2: Managed Validation Preference For Service-Lifecycle Verification

Owner: workspaceTemplate/Soul/policy, with core contract tests only for generic support

Implementation status: done

Code/document scope:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- related tests

Required changes:

- Keep core rejection of background process, `nohup`, `setsid`, `kill`, `pkill`, `killall`, `fuser -k` as-is.
- Strengthen software-dev profile/Soul guidance:
  - service lifecycle validation should prefer `validation/managed_session`.
  - shell validation is for bounded foreground commands only.
  - no approval request should be generated solely to terminate a validation process.
- In core requiredOutput, expose a concise managed-session example when:
  - current action requires runtime behavior validation, and
  - `validation/managed_session` is in acceptedKinds.
- Ensure this remains data-driven and business-neutral:
  - core only sees accepted kind `validation/managed_session` and generic lifecycle fields.
  - template/Soul decides when software development tasks should prefer it.

Acceptance tests:

- RequiredOutput with `validation/managed_session` contains a minimal managed-session example.
- A shell graph with background process and `kill` remains rejected.
- A managed-session validation graph with runtime-managed timeout/cleanup is accepted by generic contract gate.
- Boundary search confirms no new template/language/live-workspace branch in `kernel/orchestrator`.

### C3: Focus-Scoped LLM Context Rendering

Owner: core LLM context renderer

Implementation status: done

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_context_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- related tests

Required changes:

- For `assessment` NextAction:
  - render current focus and its target refs in C3.
  - render only focus-related accepted/rejected evidence refs in C4/C5.
  - move all other accepted evidence refs to summary counts and trace refs.
  - preserve latest definition revision and current Goal/DoD identity.
- Keep enough information for full completion only when NextAction is `complete`.
- Do not remove protected fields:
  - current Goal/DoD revision
  - current focus
  - requiredOutput
  - latest rejected diagnostic
  - current target snapshot integrity
  - process evidence refs needed by the current focus.

Acceptance tests:

- Assessment retry request includes current focus evidence refs and omits unrelated historical accepted refs from LLM-visible main body.
- C0 stable prefix hash does not change due to assessment retry diagnostics.
- Current focus and requiredOutput remain in C3/C4 after context budget compaction.

### C4: Projection And Live Verification

Owner: workorchd projection and verification

Implementation status: projection fields verified by automated tests; live verification still requires a deployed service run.

Code/document scope:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/service/server/**`
- `docs/60-verification/**` if a live report is added.

Required changes:

- Ensure projection can show:
  - current focus id
  - NextAction type
  - whether blocker is contract violation, decision, approval, or completion gate.
  - authority diagnostics when focus-scoped assessment is rejected.
- After C1-C3, run live verification:
  - replan a decision-blocked workspace.
  - confirm accepted focus-scoped assessment advances to next coverage item.
  - confirm service validation either uses managed session or gets rejected with actionable diagnostic.
  - confirm no repeated full-history context inflation for assessment retry.

Acceptance tests:

- ViewModel exposes the same prompt mode/list/main state after focus-scoped assessment.
- Live result distinguishes `workspaceProgress` from final `goalConvergence`.

## Implementation Order

1. C1 Core focus-scoped assessment gate.
2. C2 Managed validation preference and examples.
3. C3 Focus-scoped LLM context rendering.
4. C4 Projection/live verification.

This order is intentional:

- C1 fixes the direct core blocker for `ws-4e...`.
- C2 fixes the main recoverable LLM-output drift for service validation without weakening core safety.
- C3 reduces the probability that LLM ignores the narrow contract.
- C4 verifies the end-to-end behavior after state machine changes.

## Non-Goals

- Do not weaken full completion gates.
- Do not accept missing evidence.
- Do not allow shell process termination as validation cleanup.
- Do not add software-dev, LSP, Go, proxy, file-name, command-name, provider, or workspace-id branches to core.

## Implementation Notes

- C1 is implemented by `validatedCompletionDoDAssessmentForStep`: focus-scoped assessment uses the current structured focus and accepted evidence refs; final completion still uses the full Goal/DoD/deliverable gates.
- C2 keeps shell background/termination rejection unchanged. Core exposes a generic `validation/managed_session` example only when that accepted kind is present; software-dev defaults express the service-lifecycle preference in template/Soul data.
- C3 scopes compacted assessment context to the current focus. Unrelated coverage evidence refs are omitted from the main body and represented as counts.
- C4 did not require new projection fields in this pass because read model and workspace view already expose `currentFocus`, `NextAction`, contract violation, convergence status, and authority diagnostics.
- Do not let projection repair owner state.
- Do not treat memory or compacted history as completion authority.

## Verification Commands

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/service/server -count=1
```

Context/cache tests when C3 is touched:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Context|Cache|StagePacket|DoD|Coverage|NextAction|Validation' -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|LSP|lsp|localproxy|main.go|mcp-config.json|kill \\$|pkill|killall|fuser -k" internal/modules/kernel internal/pkg
rg -n "assessmentOnly|complete_dod_goal_coverage_assessment|coverageWorkItemContext|stagePacket|validation/managed_session|validationEvidence" internal/modules/kernel/orchestrator/biz -g "*.go"
```

Full verification before closure:

```bash
go test ./... -count=1
git diff --check
```

## Done Criteria

- Focus-scoped assessment accepts only the current focus and updates coverage ledger without requiring all Goal/DoD/deliverable items in the same response.
- Final completion still requires all coverage work items and full Goal/DoD/deliverable gates.
- Service lifecycle validation is guided toward `validation/managed_session`; unsafe shell validation remains rejected.
- Assessment request context is narrow, traceable, and preserves current focus/requiredOutput while avoiding unrelated full history.
- Live verification shows replan can resume progress from decision into the next coverage item or a precise governed blocker.
- Core remains business-neutral.
