# Module And Block Contract Closure Validation Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-02

Related:

- `product-design-to-runtime-convergence-closure-plan.md`
- `../../05-product/prd.md`
- `../../05-product/product-architecture.md`
- `../../05-product/business-model-flow-contracts.md`
- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`

## Purpose

WorkOrch 的执行链路天然会跨越多个 module/block：workspace、run、orchestrator、decision、approval、result、memory、MCP、LLM、shell/file/git/web blocks、workorchd projection、TUI/Web/API。若继续按整条链路一次性修复，状态组合和分支会迅速失控，导致多轮修改互相覆盖，live 验证仍难以收敛。

本文定义新的收口方法：以 module/block 为最小闭合单位，以固定跨模块合同为边界，以 pairwise contract test 和最小主链路 e2e 作为验收。目标是把复杂全链路拆成可逐项固化、可独立验证、可组合验收的合同单元。

## Contract Audit

- Scope: module/block ownership boundary, cross-module DTO/event/contract, pairwise validation, minimal runtime chain acceptance.
- Observed state:
  - 当前 workspace 无法稳定正向推进，根因不再是单个 prompt 或单个函数缺陷，而是多个 module/block 之间的状态语义、派生字段和错误分类没有形成足够硬的交互合同。
  - 过去多轮收口中，某些修改同时触碰 orchestrator、projection、context、evidence、governance 或 block validation，导致无法明确验证是哪一段合同被修好或被破坏。
  - live 验证能暴露最终偏差，但不足以作为直接修改入口；必须先把偏差落到具体 module/block 交互边界。
- Expected state:
  - 每个 module/block 独立声明输入合同、输出合同、状态不变量、错误合同和幂等规则。
  - 跨模块只能通过固定 DTO、event、runtime contract 或 port/client contract 交互，不能读写对方内部状态。
  - 每个关键连接点都有 provider-side 与 consumer-side 的 pairwise contract test。
  - 最终再用 deterministic fake LLM/block 运行最小主链路，证明合同可以组合。
- Deviation:
  - 当前缺少按 module/block 维度组织的收口清单、验收顺序和合同测试矩阵。
  - 当前 live 偏差容易被直接推进到下游兜底，而不是先确认上游模块输出合同是否正确。
- Owner: WorkOrch core for module/block contract and runtime authority. Template/Soul/profile only own concrete strategy. Web/TUI own command submission and projection display only.
- Event path:
  - Product command -> appservice port/API -> EventHub/owner command -> kernel owner state -> orchestrator contract -> block request/result -> evidence/coverage/result/governance -> projection -> TUI/Web/API.
- Contract gap:
  - 缺少 module/block 粒度的 contract inventory。
  - 缺少固定交互字段和禁止访问边界。
  - 缺少 pairwise contract tests 与组合 e2e 的明确验收顺序。
- Boundary check:
  - 本计划只定义中立 module/block 合同，不加入模板、语言、命令、文件名、provider 或 live workspace ID 特判。

## Closure Rule

后续代码收口遵守以下规则：

- Module first: 每次只固化一个 module/block 的内部合同，或一个 pairwise 交互合同。
- Fixed contract only: 跨模块只使用 DTO、event、StagePacket、StepExecutionRequest、normalized block result、EvidenceRecord、CoverageWorkItem、Projection DTO 等固定合同。
- No private read: consumer 不能读取 provider 的内部状态、旧派生字段、raw payload 或 repository 私有结构来推断业务状态。
- No duplicated authority: 只有 runtime authority/owner state 能决定业务状态转移；module/block 不能各自推导 run completion 或 next action。
- Pairwise before e2e: 先通过 provider/consumer 双侧合同测试，再进入最小主链路 e2e。
- Live last: live 验证只确认真实环境组合行为，不能替代合同测试。

## Contract Layers

### Layer 1. Module Internal Contract

每个 module 必须独立闭合：

| Contract | Definition |
| --- | --- |
| InputContract | 允许接收的 DTO/event/model 字段、必填 scope、revision、idempotency key |
| OutputContract | 输出的状态、事件、DTO、diagnostic、raw ref、error class |
| StateInvariant | 模块内部必须持续成立的不变量 |
| ErrorContract | 可恢复、不可恢复、治理等待、环境失败、合同违例的分类 |
| IdempotencyContract | 重复命令、重试、服务重启、projection rebuild 的行为 |

### Layer 2. Pairwise Interaction Contract

关键跨模块边界必须成对验证：

| Pair | Provider obligation | Consumer obligation |
| --- | --- | --- |
| API/TUI/Web -> appservice | 提交 scoped command 或正式 API payload | 不绕过 API，不携带本地推断状态 |
| appservice -> owner/EventHub | 校验 scope、幂等键、command type | owner 只接受正式 command，不接受派生状态 patch |
| owner -> orchestrator | 提供 workspace/run/definition/evidence 权威快照 | orchestrator 不偷读端侧状态或 projection |
| orchestrator -> block | 输出 StagePacket/StepExecutionRequest/current requiredOutput | block 不判断业务 completion |
| block -> orchestrator | 返回 normalized result/error/raw ref | orchestrator 只消费合同字段，不解析 adapter 私有状态 |
| orchestrator -> evidence ledger | 写 accepted/rejected evidence with sourceRef/targetRef | ledger 不接受未过合同门禁的 state change |
| evidence ledger -> coverage ledger | 输出可绑定 evidence refs 与质量字段 | coverage 只根据 accepted evidence 更新 |
| coverage ledger -> NextAction | 输出 missing evidence/current focus | NextAction 不读取旧 repair 包作为权威 |
| governance -> continuation | 输出 scoped resolution/approval result | continuation 继承原 stage contract |
| owner/projection -> Web/TUI | 输出 bounded ViewModel/revision/health | Web/TUI 不重建业务状态机 |

### Layer 3. Minimal Chain Contract

所有 pairwise 合同通过后，使用 deterministic fake LLM/fake block 验证：

```text
definition
  -> activation
  -> coverage baseline
  -> nextAction
  -> stagePacket
  -> fake candidate
  -> contract gate
  -> evidence record
  -> coverage recompute
  -> result/governance
  -> projection
```

该链路只验证 runtime 合同组合，不依赖真实 provider、真实 shell 环境或具体业务模板。

### Layer 4. Live Acceptance

live 验证只在上述三层通过后执行，用于确认真实 provider、真实文件系统、真实 Web/TUI/API 组合没有破坏合同。

## Registered Component Inventory

本节按当前代码实际注册单元列出完整 inventory。后续收口不得只按主链路概括项推进；每个正式 `module.go` / `plugin.go` / `initiator` 都必须能映射到下表。

| Kind | ID / Component | Code path | Closure unit |
| --- | --- | --- | --- |
| initiator | `workorch/repositories` | `internal/initiators/workorchruntime` | Repository Initiator |
| application module | `application/workorchd` | `internal/modules/application/workorchd` | Workorchd Application |
| stateful block | `blocks/workspace` | `internal/modules/blocks/workspace` | Workspace Block |
| kernel module | `kernel/run` | `internal/modules/kernel/run` | Run |
| kernel module | `kernel/orchestrator` | `internal/modules/kernel/orchestrator` | Orchestrator |
| stateful block | `blocks/result` | `internal/modules/blocks/result` | Result Block |
| stateful block | `blocks/decision` | `internal/modules/blocks/decision` | Decision Block |
| stateful block | `blocks/approval` | `internal/modules/blocks/approval` | Approval Block |
| kernel module | `kernel/governance` | `internal/modules/kernel/governance` | Governance |
| kernel module | `kernel/eventstream` | `internal/modules/kernel/eventstream` | Eventstream |
| stateful block | `blocks/memory` | `internal/modules/blocks/memory` | Memory Block |
| stateful block | `blocks/mcp-registry` | `internal/modules/blocks/mcpregistry` | MCP Registry Block |
| stateful block | `blocks/skill` | `internal/modules/blocks/skill` | Skill Registry Block |
| stateful block | `blocks/soul` | `internal/modules/blocks/soul` | Soul Registry Block |
| kernel module | `kernel/system` | `internal/modules/kernel/system` | System |
| runtime block | `file` | `internal/modules/blocks/file` | File Block |
| runtime block | `git` | `internal/modules/blocks/git` | Git Block |
| runtime block | `llm` | `internal/modules/blocks/llm` | LLM Block |
| runtime block | `mcp` | `internal/modules/blocks/mcp` | MCP Block |
| runtime block | `shell` | `internal/modules/blocks/shell` | Shell Block |
| runtime block | `ssh` | `internal/modules/blocks/ssh` | SSH Block |
| runtime block | `web` | `internal/modules/blocks/web` | Web Block |
| service/client surface | process service / port / client / web assets | `internal/services/workorch`, `internal/pkg/workorchport`, `internal/pkg/workorchclient`, `web` | Workorch Service And Product Surfaces |

Inventory rule:

- 新增任何 `module.go`、`plugin.go`、runtime block、initiator 或正式 product surface 时，必须同步补充本表和下方 Closure Inventory。
- 若一个组件只参与 wiring 或只读查询，也必须有 `InputContract`、`OutputContract`、`StateInvariant` 和至少一个 smoke/contract test。
- 分组章节可以用于主链路理解，但不能替代逐组件验收。

## Module And Block Closure Inventory

### 1. Repository Initiator

- Owner: `internal/initiators/workorchruntime`
- Owns: 单一 repository provider/factory 基础设施能力。
- InputContract:
  - 只能从 application entry 注册和启动。
  - Setup 只负责构造并注册 owner repository provider handles。
- OutputContract:
  - 向 owner module/block 提供按职责分组的 repository provider。
  - lifecycle Run/Teardown 可幂等执行。
- StateInvariant:
  - initiator 不拥有 workspace/run/result 业务状态。
  - 不导出 EventHub、CommandHub、runtime policy 或通用 service facade。
  - module/block/initiator 注册只能通过 `application/workorch` 按需引入。
- Pairwise tests:
  - initiator -> workorchd/modules: dependencies available before module Run。
  - shutdown -> modules: teardown releases background routines and event subscriptions。

### 2. Workorchd Application Module

- Owner: `internal/modules/application/workorchd`
- Owns: HTTP API use cases、projection/read model、SSE/event refresh、operation entry、context registry appservice、configuration appservice。
- InputContract:
  - writes are formal API/commands with scope。
  - reads are projection/detail/raw ref access。
  - configuration and context registry writes must not mutate runtime owner state directly。
- OutputContract:
  - bounded projection DTO with revision/health。
  - command accepted/rejected result。
  - scoped raw/detail response。
- StateInvariant:
  - appservice does not own business state。
  - projection does not repair owner state。
  - Web/TUI/API operation surfaces must not duplicate runtime state machine。
- Pairwise tests:
  - owner -> projection: revision and state alignment。
  - Web/TUI -> appservice: unsupported writes rejected。
  - config/context registry -> runtime: structured refs only, no business state patch。

### 3. Workspace Stateful Block

- Owner: `internal/modules/blocks/workspace`
- Owns: Workspace、Goal、DoD、definition revision、workspace binding。
- InputContract:
  - create/update command 必须携带 workspace scope 或创建上下文。
  - Goal/DoD 更新必须产生新的 effective definition revision。
- OutputContract:
  - stable workspace definition snapshot。
  - latest effective Goal/DoD/revision。
- StateInvariant:
  - 只管理 workspace-definition 聚合自身状态，不承担跨组件编排。
  - active definition 不得被历史摘要替代。
  - cumulative latest 不得静默弱化旧约束。
- Pairwise tests:
  - workspace -> run activation: first run receives latest definition revision。
  - workspace -> projection: projection shows same effective Goal/DoD revision。

### 4. Run Module

- Owner: `internal/modules/kernel/run`
- Owns: Run、Step、ToolCall、Patch、ValidationResult、RuntimeCheckpoint。
- InputContract:
  - run command 必须 scoped by workspaceId/runId。
  - step finalization 必须接收 contract gate outcome。
- OutputContract:
  - run/step lifecycle transition。
  - process evidence records with source refs。
- StateInvariant:
  - 不允许 `running + currentStep failed + no blocker + no next action`。
  - step 不能先 completed 后再被同一候选输出改成 failed/contract violation。
- Pairwise tests:
  - orchestrator -> run: contract failure finalizes as rejected diagnostic/failed or governed path, not completed。
  - run -> projection: current run/step state is consistent。

### 5. Orchestrator Module

- Owner: `internal/modules/kernel/orchestrator`
- Owns: convergence authority、coverage/evidence interpretation、NextAction、StagePacket、contract gate、repair/replan/continuation。
- InputContract:
  - reads owner snapshots only。
  - dispatch must use one aligned authority snapshot。
- OutputContract:
  - deterministic NextAction。
  - StagePacket/current requiredOutput。
  - accepted/rejected evidence classification。
- StateInvariant:
  - old repairActionPackage、old requiredOutput、old projection、old LLM output are diagnostic-only。
  - LLM candidate cannot own state transition。
- Pairwise tests:
  - coverage -> NextAction: missing evidence maps to exactly one action。
  - StagePacket -> LLM context: required fields preserved under context budget。
  - contract gate -> evidence ledger: rejected output has no side effect。

### 6. Result Stateful Block

- Owner: `internal/modules/blocks/result`
- Owns: Result、Artifact、accepted outcome。
- InputContract:
  - result acceptance requires current definition refs and accepted evidence refs。
- OutputContract:
  - accepted/rejected/superseded result state。
  - logical artifact refs。
- StateInvariant:
  - 只管理 Result/Artifact formal-outcome 聚合自身状态，不承担 completion gate 编排。
  - LLM self-claimed completion without evidence cannot create accepted Result。
  - artifact must not expose local absolute paths in product surface。
- Pairwise tests:
  - completion gate -> result: all satisfied coverage creates accepted result。
  - result -> projection: outcome cites evidence refs。

### 7. Decision Stateful Block

- Owner: `internal/modules/blocks/decision`
- Owns: DecisionTicket、Resolution。
- InputContract:
  - ticket and resolution commands must be scoped by workspaceId/runId/stepId when related to runtime execution。
  - resolution payload may supplement continuation context only。
- OutputContract:
  - open/resolved/rejected decision state。
  - resolution record with lineage and scoped continuation intent。
- StateInvariant:
  - 只管理 Ticket/Resolution 聚合自身状态，不承担 continuation 编排。
  - decision is created only for real governance gaps。
  - resolution cannot overwrite original runtime stage contract。
- Pairwise tests:
  - orchestrator/governance -> decision: ticket creation is scoped and idempotent。
  - decision -> continuation: resolved ticket triggers scoped continuation。

### 8. Approval Stateful Block

- Owner: `internal/modules/blocks/approval`
- Owns: Approval、approval decision result。
- InputContract:
  - approval command must be scoped by workspaceId/runId/stepId/action when runtime-related。
  - approve/reject is an authorization decision, not business evidence by itself。
- OutputContract:
  - pending/approved/rejected approval state。
  - approval result event for continuation or terminal handling。
- StateInvariant:
  - 只管理 Approval 聚合自身状态，不承担授权后执行或 completion 编排。
  - approval cannot create accepted Result。
  - approval ID conflicts across workspaces require scoped lookup。
- Pairwise tests:
  - orchestrator/governance -> approval: pending approval blocks run。
  - approval -> continuation: approved action resumes with original stage contract。

### 9. Governance Module

- Owner: `internal/modules/kernel/governance`
- Owns: AutoGovernance trace、policy gate、governance suggestion and adapters。
- InputContract:
  - consumes structured failure/retry/policy signals。
  - cannot parse business text, template ID, language or command output as policy authority。
- OutputContract:
  - decision/approval/terminal/governed wait recommendation。
  - audit trace with source refs。
- StateInvariant:
  - only real governance gap can block a run。
  - recoverable runtime gap should return runtime action, not human decision。
- Pairwise tests:
  - repeated failure -> governance: bounded and idempotent escalation。
  - governance -> decision/approval: scoped object creation only。

### 10. Eventstream Module

- Owner: `internal/modules/kernel/eventstream`
- Owns: EventRecord storage/query/purge、audit event access、replay input surface。
- InputContract:
  - event query and purge must carry workspace scope when mutating or reading scoped data。
  - event payload is audit data, not owner state mutation。
- OutputContract:
  - ordered bounded EventRecord list。
  - purge result count and audit diagnostic。
- StateInvariant:
  - eventstream is not the only truth source and must not repair owner state。
  - default projection must not inline large payloads。
- Pairwise tests:
  - owner -> eventstream: emitted events preserve source module, subject and scope。
  - eventstream -> projection/audit: query is bounded and scoped。

### 11. Memory Stateful Block

- Owner: `internal/modules/blocks/memory`
- Owns: Memory、recall trace、selected context。
- InputContract:
  - only high-value reusable information with provenance can be stored。
- OutputContract:
  - selected/candidate/excluded recall trace。
- StateInvariant:
  - 只管理 MemoryItem/MemoryRecallTrace 聚合自身状态，不承担 runtime context 编排。
  - rejected diagnostic retry流水不能成为高价值长期 memory。
  - memory cannot override current Goal/DoD or StagePacket。
- Pairwise tests:
  - memory -> context renderer: selected context is traceable and bounded。
  - rejected diagnostic -> memory: not persisted as accepted memory。

### 12. MCP Registry Stateful Block

- Owner: `internal/modules/blocks/mcpregistry`
- Owns: MCPServer、MCPCapability registry、capability selection metadata。
- InputContract:
  - server/capability commands are structured and scoped when workspace-bound。
  - capability metadata must not encode runtime state decisions。
- OutputContract:
  - declared capability catalog and refs for runtime context。
- StateInvariant:
  - 只管理 MCPServer/MCPCapability registry 自身状态，不执行 MCP invocation。
  - core consumes capability kind/schema, not concrete business meaning。
- Pairwise tests:
  - mcp registry -> activation/context: selected capability refs are traceable。
  - mcp registry -> mcp block: capability invocation contract remains structured。

### 13. Skill Registry Stateful Block

- Owner: `internal/modules/blocks/skill`
- Owns: Skill registry and selected skill metadata。
- InputContract:
  - skill definitions are guidance/configuration, not runtime state transitions。
  - workspace/run binding must be traceable。
- OutputContract:
  - selected skill refs and bounded instruction metadata。
- StateInvariant:
  - 只管理 Skill registry 自身状态，不承担 runtime context 或执行编排。
  - skill guidance cannot override current Goal/DoD, StagePacket or execution contract。
- Pairwise tests:
  - skill -> activation/context: selected skills appear as refs with provenance。
  - skill -> orchestrator: core consumes structured refs, not template-specific branch。

### 14. Soul Registry Stateful Block

- Owner: `internal/modules/blocks/soul`
- Owns: Soul catalog、stage policy refs、capability and risk constraints。
- InputContract:
  - Soul commands operate on catalog/binding metadata。
  - stage policy must be structured。
- OutputContract:
  - selected stage policy refs and constraints。
- StateInvariant:
  - 只管理 Soul/stage-policy catalog 自身状态，不承担 runtime 调度或策略编排。
  - core does not branch on Soul ID or Soul display text。
- Pairwise tests:
  - soul -> activation/context: stage policy is bound to run context。
  - soul -> orchestrator: constraints are consumed through neutral fields。

### 15. System Module

- Owner: `internal/modules/kernel/system`
- Owns: system-level module identity, health or infrastructure contract surface。
- InputContract:
  - only accepts system/infrastructure commands if explicitly defined。
  - cannot mutate workspace/run/result business state。
- OutputContract:
  - system status or infrastructure diagnostic data。
- StateInvariant:
  - system module remains infrastructure-only。
  - no business completion, governance or evidence decision lives here。
- Pairwise tests:
  - system -> appservice/health: health/status is read-only and bounded。
  - system lifecycle: setup/run/teardown remains idempotent。

### 16. File Block

- Owner: `internal/modules/blocks/file`
- Owns: bounded file read/write/list style side effects and normalized file result。
- InputContract:
  - receives block request with workspace-safe path/ref and operation kind。
  - write requires admitted state-changing contract。
- OutputContract:
  - normalized result/error/raw ref/summary。
- StateInvariant:
  - file block cannot decide coverage/result completion。
  - path exposure to product surfaces must use logical refs。
- Pairwise tests:
  - orchestrator -> file: invalid path or unauthorized write rejected before side effect。
  - file -> orchestrator: output carries target refs and bounded summary。

### 17. Git Block

- Owner: `internal/modules/blocks/git`
- Owns: git side-effect adapter and normalized git result。
- InputContract:
  - receives explicit git operation request under runtime policy。
  - repository path/ref must be scoped to workspace context。
- OutputContract:
  - normalized git result/error/raw ref。
- StateInvariant:
  - git block cannot infer task success from command text。
  - git block does not own run state。
- Pairwise tests:
  - orchestrator -> git: operation requires admitted contract。
  - git -> orchestrator: failure classification uses structured result。

### 18. LLM Block And Context Renderer

- Owner: `internal/modules/blocks/llm` plus orchestrator context assembly。
- Owns: provider request rendering、cache partition、schema output handling。
- InputContract:
  - C0 stable contract excludes dynamic runtime fields。
  - C1-C5 carry definition, capability, stage, current action, evidence refs and large object refs。
- OutputContract:
  - parsed candidate or parse/schema violation diagnostic。
- StateInvariant:
  - provider retry and contract retry do not mutate stable prefix。
  - context budget cannot drop current Goal/DoD, requiredOutput, latest failure, accepted/rejected evidence or target snapshot refs。
- Pairwise tests:
  - StagePacket -> request: critical fields present。
  - provider output -> contract gate: invalid output becomes rejected diagnostic only。

### 19. MCP Block

- Owner: `internal/modules/blocks/mcp`
- Owns: MCP tool/resource/prompt invocation adapter and normalized MCP result。
- InputContract:
  - invocation must reference declared MCP server/capability。
  - params are structured and bounded。
- OutputContract:
  - normalized MCP result/error/raw ref。
- StateInvariant:
  - MCP block cannot mark DoD satisfied directly。
  - undeclared capability cannot be invoked as a runtime shortcut。
- Pairwise tests:
  - mcp registry -> mcp block: declared capability is required。
  - mcp block -> orchestrator: result uses structured evidence refs。

### 20. Shell Block

- Owner: `internal/modules/blocks/shell`
- Owns: shell command execution adapter, timeout/resource guard, normalized shell result。
- InputContract:
  - command request must be admitted by runtime contract and policy。
  - long-running validation must use bounded foreground harness according to policy。
- OutputContract:
  - normalized exit code/stdout/stderr summary/raw ref/error class。
- StateInvariant:
  - shell output text is evidence input, not direct business truth。
  - shell block cannot create accepted result or memory。
- Pairwise tests:
  - orchestrator -> shell: disallowed/background/destructive form rejected or approval-gated。
  - shell -> validation/evidence: accepted evidence requires structured binding。

### 21. SSH Block

- Owner: `internal/modules/blocks/ssh`
- Owns: remote command/session adapter and normalized SSH result。
- InputContract:
  - target/session/credential refs must be structured and policy-admitted。
  - remote side effects require explicit contract admission。
- OutputContract:
  - normalized SSH result/error/raw ref。
- StateInvariant:
  - SSH block cannot encode environment-specific recovery in core。
  - remote output requires evidence binding before coverage use。
- Pairwise tests:
  - orchestrator -> ssh: missing credential/permission becomes structured blocker。
  - ssh -> orchestrator: remote failure is classified through structured result。

### 22. Web Block

- Owner: `internal/modules/blocks/web`
- Owns: web/search/fetch adapter and normalized web result。
- InputContract:
  - request must be bounded, scoped and policy-admitted。
  - network/provider failure must be classified separately from contract violation。
- OutputContract:
  - normalized web result/error/raw ref/source refs。
- StateInvariant:
  - web content is external evidence candidate, not accepted result by itself。
  - large content must use summary/raw ref。
- Pairwise tests:
  - orchestrator -> web: bounded fetch/search request。
  - web -> evidence: source refs and truncation flags are preserved。

### 23. Workorch Service / CLI / TUI / Client / Web Assets

- Owner: `internal/services/workorch`, `internal/pkg/workorchport`, `internal/pkg/workorchclient`, `web`
- Owns: user operation surface, port/client DTO, remote/embedded backend, Web assets。
- InputContract:
  - consumes projection/port DTO。
  - submits scoped command/API requests。
- OutputContract:
  - command result display。
  - projection display。
- StateInvariant:
  - no local business state machine。
  - no unscoped run/ticket/approval mutation when multiple objects may conflict。
- Pairwise tests:
  - TUI/Web prompt mode equals projection mode。
  - client path/query matches server handler contract。

## Closure Phases

### Phase A. Contract Inventory Freeze

目标：为每个 module/block 建立唯一合同文件或测试 fixture。

Tasks:

1. 列出每个 registered module/block/initiator/surface 的 owner、input、output、invariants、error classes。
2. 标记所有跨模块 DTO/event/contract 的代码落点。
3. 标记仍使用 raw map、旧派生字段、旧 repair lineage 或 repository 私有结构跨边界传递的路径。

Acceptance:

- 每个 registered module/block/initiator/surface 都能被归入上面的 inventory。
- 每个跨模块调用都有 provider 和 consumer。

### Phase B. Internal Module Contract Tests

目标：先保证每个 module/block 自己不产生非法状态。

Tasks:

1. 为上方完整 inventory 中的每个 closure unit 增加或补齐内部合同测试。
2. 测试只验证该 owner 的输入输出，不引入下游兜底。
3. 对错误分类建立 stable test table。

Acceptance:

- 每个 owner 至少有一个 failure sample。
- 旧非法状态在 owner 内被提前拒绝、分类或转入明确等待/失败。

### Phase C. Pairwise Contract Tests

目标：固化跨模块连接点。

Tasks:

1. 对上文 pairwise table 中 P0 连接点建立 provider/consumer 双侧测试。
2. consumer test 必须证明只依赖合同字段，不读取 provider 内部字段。
3. provider test 必须证明输出字段完整且 scope/revision/idempotency 可追溯。

Acceptance:

- 修改 provider 字段时，consumer contract test 能及时失败。
- 旧派生字段不能再成为 consumer 的权威输入。

### Phase D. Minimal Chain E2E

目标：证明 module/block 合同可以组合为完整 runtime 主链路。

Tasks:

1. 使用 deterministic fake LLM/fake blocks。
2. 跑通 definition -> result/projection 的最小闭环。
3. 增加违例分支：fake LLM invalid output -> rejected diagnostic -> bounded retry -> valid output。

Acceptance:

- e2e 不依赖真实 provider 或具体模板策略。
- 不产生 accepted evidence/result from invalid output。
- projection 与 owner state revision 对齐。

### Phase E. Live Verification

目标：确认真实环境中合同没有被 provider、shell、file system、TUI/Web/API 打破。

Tasks:

1. 选定一个 workspace 做 baseline freeze。
2. 记录 binary version、service start time、workspaceId、runId、definition revision。
3. 观察每一轮 NextAction、StagePacket、evidence、coverage、projection。
4. 若失败，只归入一个 module/block 或 pairwise 边界，再回到 Phase B/C。

Acceptance:

- workspace 能持续产生与 Goal/DoD 相关的 accepted evidence。
- run 最终 completed with accepted result，或进入明确 governed/terminal state。
- 不出现无 blocker、无 next action、无 continuation 的悬挂状态。

## Priority Order

P0 必须先完成：

1. Workspace -> Run activation definition revision contract。
2. Orchestrator authority snapshot -> StagePacket/requiredOutput contract。
3. LLM/block output -> contract gate -> rejected diagnostic no-side-effect contract。
4. Evidence recorder -> coverage ledger binding contract。
5. Coverage ledger -> NextAction deterministic contract。
6. Governance resolution -> scoped continuation contract。
7. Owner state -> projection revision contract。

P1 后续完成：

1. Memory recall trace and exclusion contract。
2. Raw payload ref/detail boundary。
3. Capability/profile policy structured consumption。
4. TUI/Web command surface parity。
5. Restart/replan/resume idempotency expansion。

P2 回归与性能：

1. Snapshot/projection resource budget。
2. Large payload on-demand detail。
3. Multi-workspace conflict and scope tests。
4. Context cache stability and budget stress。

## Per-Unit Work Template

后续每次代码收口必须以此模板开头：

```markdown
## Module/Block Closure Unit: <name>

- Owner:
- Boundary:
- InputContract:
- OutputContract:
- StateInvariant:
- ErrorContract:
- IdempotencyContract:
- Provider obligations:
- Consumer obligations:
- Observed deviation:
- Forbidden shortcut:
- Code paths:
- Tests:
- Live verification:
- Boundary neutrality check:
- Go/No-Go:
```

## Required Verification

根据修改范围选择：

```bash
go test ./internal/modules/kernel/... -count=1
go test ./internal/modules/blocks/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
go test ./internal/pkg/... -count=1
go test ./... -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|workspace title|provider" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "kernel/.*/(biz|service|repository)|repository\\." internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient -g "*.go"
```

若修改 LLM context/cache，还必须补充 stable prefix 和 critical field protection 测试。若修改 Web/TUI/API，还必须补充 projection revision、scoped command 和 unsupported write rejection 测试。

## Stop Conditions

出现以下情况必须停止当前代码修改，回到合同审计：

- 一个 patch 同时修改三个以上 owner 环节。
- 无法说明失败归属到哪个 module/block 或 pairwise boundary。
- live 验证失败后准备直接改下游兜底。
- consumer 需要读取 provider 私有字段才能继续。
- 需要通过模板、语言、命令、文件名或 provider 特判才能让测试通过。
- 同一 blocker 连续两次修改后仍未改善。
