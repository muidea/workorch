# WorkOrch Product Requirements

Status: normative

Type: product-requirements

Last Verified: 2026-08-26

## Product Positioning

WorkOrch 是 local-first 的长期任务编排与自治执行 Runtime。用户提交 Workspace 创作输入，系统派生并独立审查 DoD，
经用户精确确认和 Workspace owner 准入形成正式 Goal/DoD。用户另行显式启动后，系统基于不可变 Definition revision，
在可审计、可治理、可恢复、fail-closed 的 Runtime 中调用 LLM、provider 与 capability，持续进行
有界语义规划、执行、验证、修复与重新收敛，直到：

- accepted evidence、coverage 与 assessment 支持成功终态；
- 进入真实且明确的治理等待；
- 进入合同定义、预算有界的可恢复运行等待；
- 或因无法安全继续而 fail-closed。

核心原则是：

> Models propose. Runtime decides. Evidence proves.

本文是唯一产品需求入口，只定义产品目标、用户可观察行为和发布要求。Runtime 状态机、字段与 owner 合同以
[Runtime Core](../00-core/runtime-stage-contract.md) 为实现约束；产品架构、领域对象、接口和验证细节由各自专题展开。

## Product Principles

- **Goal-oriented**：Goal 是 Workspace owner 管理的正式定义，不是单次 prompt，也不能在 Run 内隐式修改。
- **DoD-driven**：Task、CWI、semantic plan 或 operation 执行结束本身均不等于 Goal 完成。
- **Evidence-verified**：成功必须由 current、accepted、scope-matched evidence、coverage 与 assessment 共同支持。
- **Runtime-authoritative**：LLM、provider、capability、Eventstream、projection、Web 和 TUI 均不拥有 Runtime authority。
- **Revision-bound**：Definition、owner resource、request、dispatch、operation、evidence、assessment 与 Result 必须可追溯。
- **Recoverable**：长期执行从持久化 Runtime aggregate 幂等 re-drive，不从模型文本、projection 或客户端状态恢复。
- **Fail-closed**：无法证明 transition、effect、evidence 或 recovery 安全时停止推进，不猜测、不越权、不伪造治理。

## Required User Outcomes

### 1. Goal And DoD Driven Delivery

- Goal、DoD 与 Template 是 Workspace 的正式启动输入；每次正式变更产生新的 immutable Definition revision。
- Workspace owner 必须先持久化 pre-seal `DefinitionAdmissionCandidate`，只有 pure admission 返回 exact-current
  `admitted_all_of` 才可 promotion 为正式 Definition 和 startup input。
- canonical Definition aggregation、dependency、gate 与 terminal 只支持 `all_of`。
- 显式 `one_of`、`any_of`、threshold、未声明替代分支或无法确定唯一 required relation 的候选必须返回阻塞
  clarification ticket；不得自动选支、改写、promotion 或创建 Run。
- 执行期发现的隐含 `one_of`、undeclared prerequisite 或 semantic ambiguity 必须通过专用
  runtime-definition-clarification command/result 和 ticket binding 收口；current Run 不扩图、不改 Definition。
- 每个 Run 绑定一个不可变 Definition revision、startup-input hash 与 typed `RuntimeBootstrap`；Resume 不得吸收新 revision。
- 正常 Web setup 的业务输入为 Template、Goal、Resources 和可选用户验收要求，不要求用户手工编写正式 DoD。
  directory 可选，空值解析为 `.workorch-workspaces/<workspace-id>`。
- Workspace owner 先持久化创作输入，再派生 DoD，完成六类独立审查；仅在用户精确确认当前 revision、input、permit、
  proposal 和 specification 身份后物化正式 Goal/DoD，并执行 owner admission。创作输入不等同于已封存的启动输入。
- 普通用户不需要编写 raw context、runtime、target、recipe、check、metric、artifact、evidence 或 provider JSON。
- 创建、编译与确认均不得启动 Run；执行是单独的显式意图。TUI 不提供创作 wizard，并拒绝已退休的直接定义写入。
- startup input 封存后，Runtime 可以在不修改启动输入的前提下调用 LLM 处理 bounded semantic proof、candidate、
  diagnosis 或 assessment。

### 2. Verifiable Requirement Coverage

- Definition 必须将适用 Goal/DoD 声明确定性映射为具有稳定 identity 的 Coverage Work Item（CWI）。
- `required_coverage` item 至少映射一个 CWI；aggregation、constraint 和 context 不得伪装成 executable CWI。
- coverage、evidence 和 assessment 是三个独立维度，不得合并为一个通用状态或模型评分。
- 产品 read model 至少应能表达：
  - coverage：`uncovered | partially_covered | covered`；
  - assessment：`unassessed | satisfied | unsatisfied | indeterminate`；
  - evidence：`candidate | accepted | rejected | stale_or_superseded`。
- evidence 存在不等于 coverage 完整；coverage 完整也不等于 assessment satisfied。
- 当前 all-of 产品基线不提供 `indeterminate` 成功例外：任一 required CWI 为 `indeterminate` 时 terminal gate 必须阻塞成功。
  Goal/DoD、Template/Profile、assisted draft、LLM 或客户端均不能声明 `indeterminate_as_success` 一类降级策略。
- closeout 必须能逐 CWI/requirement 解释 accepted evidence、coverage、assessment verdict 和未判定原因。
- 无法由当前 capability、环境或声明合同验证的 requirement 必须保留明确 gap，而不是假定满足。

### 3. Bounded Semantic Planning And Work Convergence

- LLM 只提出 bounded semantic-plan document、candidate、diagnosis 或 assessment，不产生 authoritative transition。
- 当前正式执行结构是 Definition-derived CWI、focus、remaining queue、`CoverageEvidenceBinding` 与 reconciliation contract；
  不建立独立 authoritative work graph、Task scheduler、`NextAction` 或 checkpoint authority。
- semantic plan 只能逐 CWI 提议 proof shape、owner、operation/effect、target policy、selector、recipe/check 和适用
  metric/artifact；capability owner 只返回当前 feasibility/admissibility fact。
- 只有 Orchestrator SM-2 private internal event 可以校验并封存 binding、reconciliation contract、coverage credit 与 terminal。
- operation candidate 和 dispatch 必须发生在 required CWI binding 已封存之后。
- 产品语义中的 repair/replanning 仅表示同一 Run 内有界的 same-focus repair，或 owner-fact 变化导致当前 CWI
  reopen 后重新生成 semantic proof；它不建立通用 graph replan lifecycle，也不自动创建 successor。
- re-evaluation 不得修改 Goal、DoD、Template、directory 或 Definition revision，并受独立 guard/budget 约束。
- 所有 CWI settled 仍须通过 Run-level completion gate；不能仅凭工作项完成或模型输出 `done` 判定成功。

### 4. Purpose-Bound Semantic Provider Invocation

- 每次 LLM/provider semantic invocation 必须在 physical call 前由 SM-3 已持久化的 request session 驱动，并绑定：
  - workspace/run/Definition/authority scope；
  - logical request 和 physical-call identity；
  - canonical convergence state 与 required-output contract；
  - `DocumentKind=semantic_plan | operations`；
  - exact allowed facts、capability projection、output schema 与 ExpectedDocument；
  - retry、provider ceiling、reasoning effort、budget 与 stable-prefix identity。
- 该组合实现 purpose-bound semantic activation，但不是新的 owner、第五状态机或独立 Runtime authority，也不替代
  `RuntimeBootstrap.Activation`。
- 模型不能选择、扩展或升级自己的 semantic role。
- planning 不能直接声明 Run 成功；assessment 不能 dispatch operation；operation proposal 不能直接产生 capability effect。
- provider output 必须先通过完整文档、strict decode、schema、semantic contract 和 owner feasibility 校验。
- best-effort parsing、prompt interpretation、UI patch 或 raw JSON bridge 不得把非法输出转换为 authority。
- repeated complete contract failure、request recovery exhaustion 和 provider submission-unknown 必须按 typed guard 处理。

### 5. Capability Feasibility And Sealed Dispatch

- capability 通过 owner projection 和 typed catalog 进入 Runtime；模型不得假设 catalog 外能力存在。
- availability、readiness、proof applicability、target/selector 和 operation feasibility 必须在 dispatch 前确认。
- accepted semantic candidate 必须先转换为 concrete typed operation，再持久化 `SealedDispatch`。
- dispatch 至少绑定 run、Definition、authority transaction、candidate、operation、capability、parameters、binding、
  target/selector、owner revision/fence、budget 和 dispatch identity。
- capability owner 只验证和执行 concrete operation，receipt 只描述发生了什么，不返回 coverage、next action 或 terminal。
- durable effect 使用稳定 ExecutionKey/Fence/owner receipt；duplicate、stale、crash-window 和 outcome-unknown 必须有确定语义。
- 没有 persisted receipt 时不得盲目重复可能已产生副作用的 operation。

### 6. Evidence, Assessment And Result

- raw observation、provider output 或 capability receipt 不自动成为 accepted evidence。
- evidence candidate 必须回链 source operation、sealed dispatch、binding、owner fact 和适用 revision/fence。
- Orchestrator 独占 evidence quality judgement、acceptance、CWI credit、assessment acceptance 与 terminal adjudication。
- rejected、stale、superseded、cross-run、cross-focus 或 revision-mismatched evidence 只能进入 diagnostic/audit lineage。
- accepted evidence 仍须通过 binding、quality、effect、target、recipe/check/metric/artifact 和 freshness gate。
- assessment 是 semantic judgment candidate，不是 authority transition；satisfied verdict 必须引用 current-run accepted evidence。
- partial assessment 只能关闭显式 satisfied CWI，未满足 CWI 保持 active；repairable gap 回到同 focus 有界收敛。
- successful closeout 至少可表达 Definition revision、Run/lineage、Goal/DoD、逐项 coverage、accepted evidence refs、
  assessment、deliverable/artifact refs、适用 owner revision set、known limitations 和 execution lineage。
- Result owner只保存 Orchestrator 已裁决并密封的 closeout，不重新判断 DoD，也不调用模型重算终态。
- governance wait、recoverable runtime wait、projection lag 或 Result persistence retry不得伪装为 successful Result。

### 7. Revision And Freshness

- Definition revision、Runtime aggregate `StateRevision`、evidence revision、dispatch/operation fence 和 owner resource
  revision 是不同概念，不得混用。
- 产品需要可判断 evidence freshness 的 execution-world view；其正式形态必须是有界的 owner revision/snapshot/fence set
  或其确定性 hash，而不是一个拥有跨 owner 写 authority 的全局 world counter。
- evidence 只绑定其实际依赖的 owner revisions；无关资源变化不得使全部 evidence 失效。
- owner resource mutation 必须退休依赖精确相交的 observation/binding，必要时 reopen CWI 或要求 revalidation。
- 旧 revision 上的验证不能无条件证明新 revision，但仍适用且有明确有效性规则的 evidence 可以复用。

### 8. Waiting, Governance And Failure

- Runtime 必须区分：
  - governance wait：继续需要真实 Decision 或 Approval resolution；
  - recoverable runtime wait：无需人类业务选择，且合同能够证明未来可安全恢复；
  - terminal failure：无法在当前 Run lineage 内安全、合法或有预算地继续。
- provider transient failure 优先使用同一 sealed request session 的有界 whole-document recovery；它不创建 candidate attempt。
- prerequisite acquisition、catalog refresh、operation outcome recovery 和 delivery retry使用各自 typed、persisted、bounded ledger。
- capability/provider 暂时不可用只有在存在明确 reason、恢复条件、retry/budget policy 和重新 drive 触发源时，才可进入
  recoverable runtime wait；否则 fail-closed。
- governance wait 只用于真实外部选择、权限或风险授权。系统错误、合同失败、预算耗尽和 invariant violation 不得伪造
  Decision/Approval。
- resolution 必须匹配 workspace/run/request/ticket/revision binding；stale/duplicate resolution 确定性处理。
- waiting 不得无限绕过预算、deadline、no-progress guard 或 fail-closed 条件。

### 9. Resume, Restart And Execution Lineage

- `resume` 只请求继续当前符合资格的非终态 Run；不改变 Definition、不接受 `ExecutionContextPatch`、不创建新 lineage。
- terminal Run 不得通过 Resume 重新打开。
- `restart` 按请求时 Workspace 当前 eligible startup input 建立新的 Run lineage。
- restart 必须先在 predecessor 持久化 Run-owned `ReplacementIntent`，完成 Orchestrator → Result → Workspace → Run
  终态确认后，Run-private reducer才可创建并发布唯一 successor。
- predecessor Result、evidence、aggregate 和 execution history保持不可变；successor 不继承 predecessor authoritative aggregate。
- Application、Workspace、Orchestrator、Web、TUI 和 projection 均不选择或预创建 predecessor/successor。
- duplicate、stale、concurrent restart 和任一 crash-window 必须收敛到同一确定 lineage 或 typed failure。

### 10. Context, Cache And Memory Governance

- Soul、Skill、MCP、Memory、Template/Profile 和 capability catalog 只能通过 owner projection 与 typed contract 进入
  bootstrap/request。
- Memory 只能提供 typed contextual fact 或 candidate context，不得覆盖 Definition、Run、binding、evidence 或 terminal authority。
- provider-facing cache prefix保持 C0/C1/C2 稳定；C3-C5承载 current/volatile Runtime 事实。
- exact Goal/DoD partition、current authority、failure、coverage、evidence 和 target snapshot优先于 cache hit。
- unexpected `stablePrefixHash` drift 是 request rendering defect；cache optimization 不得改变 semantic scope。
- raw provider body、credential、endpoint、未脱敏内容和隐式全局上下文不得进入 aggregate或后续 authority context。

### 11. Execution Budget

- 长期执行必须由显式、持久化、可恢复的 execution-budget view 约束，而不只是单次 token limit。
- 各事实仍由对应 owner/Runtime sub-ledger独占；产品级 budget view只能确定性汇总，不能成为第二份消费 authority。
- 至少治理 provider calls/tokens、允许时的 monetary cost、physical-call recovery、candidate correction、semantic proof
  generation、capability operations、operation recovery、same-focus repair/reopen 和 reasoning effort。
- budget allocation必须在相关 call/effect 前确定；duplicate delivery不得重复计算同一 logical consumption。
- crash recovery必须能够区分 confirmed、unknown 和 remaining consumption。
- 无法证明仍有预算安全继续时必须进入合同定义的 bounded wait 或 fail-closed；不得自动创建治理 ticket绕过限制。
- restart 是否获得新 budget由 Workspace/Run owner policy明确决定，不从 predecessor隐式继承。

## Runtime Authority Boundaries

| Authority | Sole owner | Must not |
| --- | --- | --- |
| Definition/startup input | Workspace owner | 由 LLM/UI/Run 隐式修改 |
| SM-1 lifecycle/replacement | Run owner | 选择 focus/action或解释 SM-2/3/4 aggregate |
| SM-2/3/4 semantic authority | Orchestrator | 直接写其他 owner repository |
| Decision / Approval lifecycle | corresponding owner | 修复系统内部失败或直接改 Run |
| Capability effect/receipt | capability owner | 选择下一动作、coverage或terminal |
| Formal Result | Result owner | 重新评估 DoD |
| Projection / Eventstream | workorchd/eventstream | 回建或修改 authority |
| LLM/provider | no Runtime authority | 直接产生 transition/effect/credit |
| Web/TUI | no business authority | 维护影子状态机 |

每个状态机必须只有一个 owner，状态选择和转换只能由该 owner 的 private internal event/reducer
执行。需要跨多个 owner 分段收集事实的可恢复流程，必须明确一个业务 owner 持久化唯一
coordinator aggregate；Application/projection 不得保存第二份 saga、phase、retry journal 或根据
部分结果代替 owner 推进。Workspace delete/retain/purge 的这个 owner 固定为 Workspace：
Application 只提交一个 delete intent，其余 owner 只执行自身的 typed fence/query/purge 并返回事实。

跨 owner I/O 只使用 EventHub 与接收 owner 定义的 concrete typed command/result；禁止跨 owner service/repository、
`map[string]any` authority bridge 和通用 callback/registry。

## Product Interfaces

- 默认二进制模式是 TUI；`--server` 启动 HTTP/Web。
- execution intent 写入口是 `POST /api/v1/workorch/workspaces/:workspaceId/execution-activations`，intent 仅为
  `resume | restart`。
- Resume API 不接受 Definition、execution context、目标 Run state 或 successor identity。
- Definition admission/promotion、runtime-definition clarification、Decision/Approval resolution 和其他资源管理使用各自
  typed API。
- Web/TUI 只提交 intent、owner command 和 governance resolution，并以服务端 projection 展示 progress、wait、Result 和 audit。
- projection stale、lag、failure 或 rebuild不得触发业务 transition；客户端不能通过本地状态补写 Runtime。

## Persistence And Recovery

- authoritative Runtime aggregate 按 Run scope 持久化，并使用 CAS 或等价 concurrency control。
- recovery 从 aggregate 幂等 re-drive；process-local cache、LLM conversation、Step/checkpoint、Eventstream 和 projection不是恢复源。
- concurrent/duplicate drive不得重复 authoritative transition、provider logical request或durable effect。
- event delivery必须容忍 duplicate、stale和适用的out-of-order；projection可以重建，authority不能由projection反向重建。
- 每个非等待、非终态 transition必须形成持久化进展；无进展、guard exhaustion或invalid aggregate必须fail-closed。
- terminal delivery按 Orchestrator → Result → Workspace → Run acknowledgement 顺序幂等恢复。

## Non-Goals And Forbidden Surfaces

WorkOrch 当前不要求，也不得通过兼容包装恢复：

- LLM 直接拥有 Run/Runtime state machine；
- capability 自行决定后续 workflow；
- 用户维护 raw runtime/evidence/provider JSON；
- authoritative execution graph、graph follow-up、`NextAction` 或通用 Task scheduler；
- `StagePacket`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher；
- application queue、checkpoint/event replay或projection回建 authority；
- 自动 replan successor或同一 Run 内隐式替换 Definition；
- 将模型 `done`、operation success、artifact存在或全部CWI执行结束直接视为Goal success；
- 通过Decision/Approval、无限retry或best-effort parsing绕过合同、预算和fail-closed门禁。

## Acceptance Criteria

发布前至少满足：

1. **Definition**：pre-seal candidate、all-of admission、one-of blocking clarification、corrected resolution acknowledgement、
   Resume/Restart revision isolation均通过验证。
2. **Ownership**：SM-1～SM-4、Workspace/Result/Decision/Approval/capability和typed EventHub边界无跨 owner写泄漏。
3. **Semantic invocation**：每次provider semantic call均绑定sealed request purpose/context/output/budget；非法输出不能进入authority。
4. **Convergence**：CWI mapping、semantic proof、owner feasibility、binding seal、candidate、dispatch、receipt、evidence、assessment、
   terminal形成唯一主链，且不存在graph/NextAction影子路径。
5. **Evidence**：raw observation不自动accepted；requirement-level coverage、freshness、partial/indeterminate assessment和closeout可解释。
6. **Recovery**：stale/duplicate/CAS、provider和capability crash-window、outcome unknown、waiting、delivery和fresh-process reload确定收敛。
7. **Budget/guard**：provider、candidate、operation、repair、semantic generation与reasoning effort预算有持久化、去重和耗尽路径。
8. **Restart**：ReplacementIntent先于successor；predecessor历史不可变；并发/重复restart只产生一个确定lineage。
9. **Interfaces**：TUI/Web/API只提交typed intent/command并消费projection；接口行为、scope和错误语义一致。
10. **Deployed convergence**：file-backed场景覆盖多 drive、真实owner receipt、真实capability operation、governance、revalidation、
    terminal Result以及关键crash-window，并逐行满足
    [Deployed Runtime Convergence Scenario Matrix](../50-product-interface/deployed-convergence-scenarios.md)。
11. **Documentation**：`docs/`只保留当前唯一标准和可重复verification/acceptance合同；实施进度、ticket和一次性证据在文档树外管理。

验证入口见 [Verification By Subject](../README.md#verification-by-subject)。

## Product Invariants

- **INV-AUTH**：Model proposes；只有对应 Runtime owner执行authority transition。
- **INV-DEFINITION**：Run绑定immutable admitted-all-of Definition；Resume不改变它。
- **INV-COVERAGE**：Evidence presence不等于coverage，coverage不等于satisfaction。
- **INV-EVIDENCE**：Observation未经Orchestrator acceptance不是正式evidence；evidence必须scope/revision/binding-matched。
- **INV-RECOVERY**：authority只从persisted aggregate恢复。
- **INV-GOVERNANCE**：治理必须代表真实外部选择或授权，不能修复内部contract/budget failure。
- **INV-BUDGET**：消费与剩余预算是持久化、可恢复、不可重复计算的Runtime fact。
- **INV-RESTART**：Restart创建新lineage，不重开或改写predecessor。
- **INV-PROJECTION**：Projection/Eventstream不拥有业务authority。
- **INV-TERMINAL**：CWI、semantic plan或operation完成不自动意味着Goal/DoD满足。
- **INV-FAIL-CLOSED**：无法证明安全继续时停止推进，不进行推测性执行。
