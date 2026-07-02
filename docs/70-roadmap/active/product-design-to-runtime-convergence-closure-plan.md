# Product Design To Runtime Convergence Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-01

Related:

- `../../05-product/prd.md`
- `../../05-product/product-architecture.md`
- `../../05-product/business-model-flow-contracts.md`
- `../../00-core/runtime-stage-contract.md`
- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`

## Purpose

当前 live 验证已经证明：workspace 仍无法稳定持续正向推进、达成 Goal 并满足 DoD。后续不能继续按单点现象反复补丁，而必须按产品功能、架构层级、业务模型、数据流和验收证据逐一收口。

本文定义后续收口方法：

1. 先把产品目标映射为可执行验收合同。
2. 再按唯一 runtime 主链路逐环节审计。
3. 每次只固化一个 owner 环节。
4. 每个环节必须绑定测试或 live 验收证据。
5. 只有当前环节验收通过后，才进入下一个环节。

## Contract Audit

- Scope: Goal/DoD driven workspace execution, runtime convergence, evidence ledger, governance continuation, projection consistency, TUI/Web/API command surfaces.
- Observed state:
  - 多轮 live 验证中，workspace 能进入 running、repair、waiting decision/approval 等状态，但不能稳定产生符合 Goal/DoD 的 accepted evidence 与最终 result。
  - 多次修改后仍反复暴露新偏差，说明当前代码实现仍未把执行链路收敛到单一权威路径。
  - LLM 输出不稳定是常态，但当前 runtime 对违例、retry、evidence、next action、projection 的闭环仍不足以稳定吸收这种不稳定。
- Expected state:
  - 用户只定义 Goal/DoD，runtime 根据 coverage/evidence ledger 确定唯一 NextAction。
  - LLM 只生成候选内容；runtime 负责合同校验、证据入账、状态转移和完成判定。
  - Web/TUI/API 只提交 scoped command 并消费 projection，不复制业务状态机。
- Deviation:
  - 当前仍可能在某些入口由旧派生字段、旧 repair lineage、旧 requiredOutput、旧 projection 或端侧状态影响下一轮推进。
  - 部分 live 偏差没有先形成可复现合同测试就进入代码修改，导致多轮修改之间容易互相覆盖或形成新冲突。
- Owner: WorkOrch core for runtime authority and projection; template/Soul/profile only for concrete strategy; Web/TUI only for operation and projection display.
- Event path:
  - TUI/Web/API command -> appservice/EventHub -> owner state -> orchestrator authority refresh -> StagePacket/requiredOutput -> LLM/block candidate -> contract gate -> evidence ledger -> coverage recompute -> result/governance/run state -> projection -> TUI/Web/API.
- Contract gap:
  - 缺少按产品验收目标组织的全链路 closure matrix。
  - 缺少每个 owner 环节的输入合同、输出合同、失败形态、代码路径、验证方式和 stop/go gate。
- Boundary check:
  - 本文只使用 Goal/DoD、ledger、evidence、NextAction、StagePacket、governance、projection 等中立合同。
  - 不引入模板 ID、语言、文件名、命令输出、provider 或 live workspace ID 特判。

## Closure Principle

后续收口遵守以下硬规则：

- One owner per change: 每次只修改一个 owner 环节，不跨多个环节同时补丁。
- Contract before code: 先写清输入、输出、偏差、验证，再改代码。
- Test before live claim: 没有失败样本单测、集成测试、边界搜索或 live 证据，不声明完成。
- Runtime owns transition: LLM 违例只作为 rejected diagnostic evidence，不能驱动 state-changing side effect。
- Projection is read model: Web/TUI 只显示 projection；若 projection 不足，修 projection owner，不让端侧推断。
- Profile owns strategy: 具体任务策略只能在 template/Soul/profile/policy/skill，core 只消费结构化字段。

## Product Acceptance To Runtime Contract Matrix

| Product goal | Runtime contract | Primary owner | Main code/doc area | Acceptance evidence |
| --- | --- | --- | --- | --- |
| Goal/DoD driven delivery | activation binds latest definition revision and coverage baseline | workspace/run/orchestrator | `kernel/workspace`, `kernel/run`, `kernel/orchestrator` | activation test proves Goal/DoD/revision available to first StagePacket |
| Long-running execution | resume/replan/restart/repair all enter same scoped continuation loop | appservice/EventHub/orchestrator | `workorchd appservice`, `runtime scheduler`, `orchestrator` | dual workspace continuation test and restart live check |
| Deterministic runtime authority | ledger derives unique NextAction; old derived fields are diagnostic-only | orchestrator convergence authority | `runtime_next_action`, `runtime_convergence`, StagePacket builder | unit test rejects stale requiredOutput/focus drift |
| Evidence-based acceptance | accepted Result requires current definition and accepted evidence refs | evidence/coverage/result owners | evidence recorder, coverage ledger, result gate | test proves self-claimed completion without evidence cannot complete |
| Governed human interaction | real governance gap creates scoped ticket/approval; resolution triggers continuation | governance/decision/approval | decision/approval owner, appservice command | decision resolved test produces next runtime action |
| Context and capability governance | StagePacket carries current gap, requiredOutput, evidence refs, capability refs | orchestrator/context renderer | StagePacket, LLM context assembly, capability catalog | context test protects critical fields under budget |
| Product surface consistency | projection is server-owned and bounded; UI consumes projection only | workorchd projection/port | projection builder, port DTO, Web/TUI adapters | projection revision test and UI command scope test |

## Canonical Runtime Chain

所有 live 偏差必须归入以下唯一链路的一个断点：

```text
Workspace definition
  -> Run activation
  -> Coverage/Evidence ledger
  -> Deterministic NextAction
  -> StagePacket and requiredOutput
  -> LLM/block candidate
  -> Contract gate
  -> Side-effect admission
  -> Evidence recording
  -> Coverage recompute
  -> Completion/governance decision
  -> Projection/ViewModel
```

禁止用下游兜底掩盖上游断点。例如：

- StagePacket 缺字段时，修 StagePacket 构造，不让 LLM 猜。
- Evidence 未绑定 coverage 时，修 evidence recorder/normalizer，不放宽 completion gate。
- Projection 不一致时，修 projection owner，不让 TUI/Web 本地推断。
- Replan/resume 不继续时，修 scoped continuation event/owner state，不靠 watchdog 随机补偿。

## Closure Phases

### Phase 0. Live Baseline Freeze

目标：每次改代码前冻结事实，避免继续凭印象修改。

必须记录：

- workspaceId、runId、stepId、ticketId/approvalId。
- 当前 Goal/DoD definition revision。
- current focus、NextAction、requiredOutput、StagePacket hash 或等价诊断。
- accepted/rejected evidence refs。
- 最后 command/event、projection revision、TUI/Web 观察状态。
- 当前真实 blocker：无目标产出、合同违例、evidence 缺失、governance 等待、projection 滞后、provider/environment 失败。

Go/No-Go:

- 无法定位到 canonical runtime chain 的具体断点时，不进入代码修改。

### Phase 1. Product Acceptance Mapping

目标：把 PRD 验收项全部转成测试或 live 验证项。

收口项：

- 为 G1-G7 建立 acceptance case 清单。
- 每个 case 绑定至少一个 unit/integration/e2e/live 验证方式。
- 每个 case 标注 owner 和失败时的回退分析入口。

验收：

- 所有 PRD 验收项都能落到代码 owner 或正式文档 owner。
- 不存在“只在文档里声明、没有验证入口”的关键合同。

### Phase 2. Authority Chain Contract Hardening

目标：按主链路逐环节固化合同，每次只改一个环节。

顺序：

1. Definition revision binding.
2. Coverage/evidence ledger initialization.
3. Deterministic NextAction derivation.
4. StagePacket/requiredOutput rendering.
5. Candidate contract gate.
6. Side-effect admission gate.
7. Evidence recording and binding.
8. Coverage recompute and completion gate.
9. Governance continuation.
10. Projection/ViewModel.

每个环节必须输出：

- input contract。
- output contract。
- allowed failure state。
- forbidden shortcut。
- affected code paths。
- regression test。
- live verification note。

Go/No-Go:

- 当前环节测试未通过，不进入下一环节。
- 若同一 live blocker 连续两次未改善，停止补丁并回到 Phase 0。

### Phase 3. Business Model And Data Flow Conformance

目标：核对业务对象与数据流是否完全符合产品架构。

核对对象：

- Workspace / Goal / DoD / DefinitionRevision。
- Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint。
- CoverageWorkItem / EvidenceRecord。
- Result / Artifact / Outcome / TaskProgress。
- Decision / Approval / Resolution。
- Soul / Skill / MCP / Memory。
- EventRecord / Projection / RunSnapshot。

验收：

- 每个状态变化都有 owner。
- 每个 state-changing command 都携带 workspace scope。
- 每个 accepted outcome 都能追溯到 current definition 和 accepted evidence。
- 大 payload 只通过 bounded summary + raw ref 进入 projection。

### Phase 4. LLM Variability Tolerance

目标：确保 LLM 不稳定不会破坏 runtime 收敛。

收口项：

- parse/schema violation -> rejected diagnostic evidence。
- contract violation -> bounded retry when current NextAction still needs candidate content。
- repeated same signature -> governance or terminal according to policy。
- self-claimed completion without evidence -> rejected assessment。
- stale requiredOutput/focus lineage -> internal contract misalignment, not LLM repair。

验收：

- 违例输出不能执行、不能成为 accepted evidence/result/artifact/memory。
- retry 必须继承当前 StagePacket 和 current focus。
- 当 ledger 已能确定下一步时，不再请求 LLM 重述状态转移。

### Phase 5. Product Surface And Projection Consistency

目标：保证 Web/TUI/API 显示和操作符合 projection-first 架构。

收口项：

- Web/TUI 主视图只使用 projection/port DTO。
- 操作入口只提交 scoped command 或正式 API。
- detail/raw payload 按需打开。
- projection health/revision 可见。
- 多 workspace 下无 scope 操作必须 conflict 或拒绝。

验收：

- TUI prompt、workspace list、main view、Web detail 对同一 projection revision 一致。
- projection 不修 owner state，只展示 derived diagnostics。

### Phase 6. Profile And Strategy Isolation

目标：确认具体任务策略没有进入 core。

收口项：

- 软件开发任务粒度、验证偏好、MCP/LSP/Skill 选择在 template/Soul/profile/policy。
- core 只读取结构化 policy/capability/evidence kind。
- e2e fixture 可以具体，但断言必须验证通用合同。

验收：

- kernel/orchestrator/appservice/repository 不包含模板、语言、命令、文件名、provider、live workspace ID 特判。
- 非软件开发 workspace 不受 software-dev profile 规则影响。

### Phase 7. Live E2E Acceptance

目标：用 live 验证证明 WorkOrch 能稳定达成最终产品目标。

最小验收场景：

- 新 workspace 从 Goal/DoD 启动，产生 todo/plan，并开始 state-changing 输出。
- LLM 合同违例后进入 bounded retry，未执行违例副作用。
- accepted state change 后进入 validation。
- validation accepted 后 coverage 更新。
- coverage satisfied 后生成 accepted Result。
- decision/approval resolved 后能 scoped continuation。
- restart/replan/resume 不丢 current definition 和 evidence lineage。
- Web/TUI projection 与 owner state 一致。

最终通过标准：

- 不再出现 `run=running + currentStep=failed + no blocker + no next action`。
- 不再出现 `waiting decision/approval resolved but no continuation`。
- 不再出现 accepted result 缺 current Goal/DoD 或 accepted evidence refs。
- 不再出现 projection 与 owner state 长期不一致。

## Per-Closure Unit Template

后续每个代码收口项必须使用此模板：

```markdown
## Closure Unit: <name>

- Product requirement:
- Canonical chain link:
- Owner:
- Input contract:
- Output contract:
- Observed deviation:
- Forbidden shortcut:
- Code paths:
- Test plan:
- Live verification:
- Boundary check:
- Go/No-Go:
```

## Execution Order For Next Work

建议从最小主链路开始，不同时修多个环节：

1. 选取一个当前 live workspace，完成 Phase 0 baseline freeze。
2. 将 blocker 归入 canonical runtime chain 的一个断点。
3. 只为该断点创建一个 Closure Unit。
4. 先补失败样本测试或诊断验证。
5. 修改对应 owner 代码。
6. 跑受影响测试和边界搜索。
7. 重新 live 验证同一个 workspace 或等价复现样本。
8. 通过后再进入下一断点。

## Required Verification Commands

根据实际修改范围选择：

```bash
go test ./internal/modules/kernel/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
go test ./internal/pkg/... -count=1
go test ./... -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|workspace title|provider" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

若修改 LLM context/cache，还必须补充 request/cache 稳定性测试。若修改 Web/TUI，还必须补充 projection revision 和 scoped command 验证。
