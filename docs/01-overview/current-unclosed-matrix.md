# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-09

## Purpose

汇总 Runtime 代码收口与 **2026-07-16 文档清理** 之后仍未关闭的事项。
已完成的过程计划、假活跃产品日记、dated live 记录与 verification artifacts **不再占用本矩阵**。

## Closed / Archived

Runtime production path is Runtime-only. Dated closure records do not appear in this matrix as actionable work
and cannot override a current persisted aggregate counterexample.

2026-07-30 audit also removed OPS-1: it referred to a Runtime maintenance command that has no code, typed
contract, or CLI entry point. It is superseded by the Runtime-only production boundary and remains archive history,
not an operational task.

2026-08-04 EC audit closed the 22-component EventHub contract queue: 22/22 owner-local concrete contracts and the
full test/race/console/managed/fresh dual-workspace matrix passed. The [completed record](../90-archive/completed/component-event-contract-closure-plan.md)
is regression evidence, not active work.

## Residual Mandatory Work

### A. Runtime 单 owner semantic contract 与 CWI reconciliation（RSO，active）

presentation slice 与 semantic focus 分离、完整 preflight、resume/restart reconciliation、activation recommendation，
以及 Goal/DoD + selected template/profile + owner facts → semantic proposal → owner feasibility → SM-2 seal 已完成代码基线。
普通 UI/API 的 raw `contextHints/evidencePolicies` JSON 输入已删除，合同声明只来自 template/profile；run snapshot、
RuntimeView 与 Web acceptance 已展示 run-scoped planning phase、逐 CWI owner fact、完整 original diagnostic 与 gap lineage，
并以 workspace/run/state revision 门禁拒绝 stale 混合。managed verifier、空 workspace prospective target、
resume/restart/drift、superseded predecessor 隔离、crash/concurrency 与 CPU guard 已通过 fresh deployed 验证。最新合同
审计确认 capability execute 与 LLM provider EventHub contract 曾由 Orchestrator 代接收 owner 定义；当前 File、Shell、Git、
MCP、SSH、Web execute、LLM provider ceiling/preflight/request、八个 owner 的 Runtime capability acquisition、六个 owner 的 operation admissibility acquisition、operation feasibility 与 semantic-proof feasibility、四个 owner 的 CWI reconciliation query、File target snapshot/selection/manifest/repair-surface query，
以及 Git capability-state acquisition、Decision governance-request 合同已直接回迁各自 `blocks/*/pkg/events` 并由 boundary verifier 固化；Run/Result/Workspace lifecycle 合同审计也已确认由各自 owner 定义并消费。
owner observation/failure/diagnostic、EventHub 接收 owner 归属与 Orchestrator private mutation surface 已形成回归基线；
controller 只公开唯一 internal-event mutation dispatcher 和 scoped read，所有 lane caller 只传唯一接收 owner
`Contracts()`。后续代码已关闭 predecessor-owned restart、release-before-successor、sealed expected document、same-step
identity fencing、raw suffix 删除和 whole-document bounded recovery。`run-029` 暴露的 actionable typed semantic
correction directive、proof-batch 最小 wire、exact-active-batch reducer invariant 与 terminal-consistent canonical Definition
outcome/activity 已完成代码收口。最新增量已通过 fmt/vet/full/boundary/affected-package race/managed；`run-054` 已证明
MCP typed metadata 可部署加载并推进至 semantic seal、dispatch、receipt/evidence commit。后续 `run-055`/`run-056`
已证明 recipe/effect 反向不匹配会在 dispatch 前以零 receipt/evidence fail-closed；这只是 RSO-20 的防线，不是
provider effect authority。`run-061`/`run-062` 证明 LLM 仍会在 `lsp_status` 与 `lsp_initialize` 间交换
`requiredEffectScopes`，因此新增的 RSO-23 已完成本地代码收口：删除 provider effect/idempotency echo，由 owner
feasibility fact 经 Orchestrator 私有事件和 SM-2 编译 canonical binding。RSO-19～RSO-21、RSO-23 已完成代码收口；
RSO-22 fresh binding-matched positive convergence 尚未完成；whole-document recovery fault injection 仍独立保留。

`SMO-R2-C12` 仍是 raw-credit 防线：candidate raw CWI、assessment claim、`PassedChecks`、exit code、文件/Git
存在不得直接关闭 coverage。R3 必须在保留该防线的前提下自动完成 semantic contract：

- `/run restart` 先在 predecessor Run 持久化 replacement intent，经 SM-4 `Result -> Workspace -> Run` 完成释放后，
  Run owner 才幂等创建 new Run；new Run 以最小 Definition seed 进入 semantic planning；
- `/run resume` 复核 current non-terminal Run 已 sealed 的 Definition/CWI/binding，不重新 semantic planning；
- validation/behavior CWI 保持单 CWI focus 与独立 binding/proof；slice 只表达展示和排序；
- template/profile 只声明可复用 capability/selector/recipe family 与质量门禁，不写用户 workspace 的业务答案；
- 普通 TUI/Web create/edit 删除 raw contract JSON；semantic planning 失败返回完整逐 CWI diagnostic，零 operation/ticket/effect；
- capability owner 只给 observation/receipt，只有 SM-2 能 retain/reopen coverage（File/Shell/Git/MCP 已增加不足证据回归）；
- 每个跨 owner topic/Command/Result 由接收方 owner 的 `pkg/events` 定义；Orchestrator 只消费合同并将校验后的 fact 转换为
  private internal event；
- `workorchruntimecontract` 只保留 owner-neutral value，跨 owner feedback 使用有限 stable outcome 并完整保留 producer
  original diagnostic；
- Orchestrator 只保留 `ApplyInternalEvent` mutation dispatcher 与唯一 aggregate CAS boundary，删除薄 wrapper 和第二 mutation API；
- 非 current schema 只产生 `fresh_activation_required`，不参与执行。
- semantic-plan 与 operations authority 只接受单次完整 JSON；output limit/EOF 进入同一 sealed expected-document 的有界
  whole-document recovery，不拼接 raw suffix，也不消耗 semantic correction attempt；
- complete semantic rejection 先作为 factual diagnostic 持久化，SM-3 private reducer 只能封存一个 typed correction
  directive 或 final rejection；LLM C4 只渲染 provider-safe rejection + directive，不读取 raw `OriginalMessage`；
- RuntimeView/outcome/activity 只映射 selected authoritative Run/Result；definition-changed 是 read-only attention，不能覆盖
  terminal outcome 或形成 projection-owned lifecycle。

唯一实施队列见
[runtime-cwi-reconciliation-single-owner-closure-plan.md](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)。

### B. Workspace reasoning effort 单 Owner 收口（WRE，code-closed，待 deployed）

当前代码已形成唯一 selection 链：

- LLM owner 解析并校验模型公开的 reasoning capability；
- Workspace owner 持久化下一次运行的 `DefaultLLMSelection`；
- Run owner在 fresh activation/restart 封存 selection，Resume 保持当前 Run 值；
- Orchestrator 只携带 sealed fact并将其纳入 provider ceiling，不选择或降级 effort；
- LLM owner只发送目录允许的 nested `reasoning.effort`；空值省略、`none` 保持显式；
- Web Start/Restart 与 TUI `/model` 通过同一 owner command更新 workspace 偏好；
- target-only API、全局 provider reasoning source、旧 wire DTO 和兼容路径已删除。

剩余门槛只是在重新部署实例上验证 Web Start/Restart、TUI `/model reasoning`、Resume 保持 sealed selection以及双
workspace 隔离；未执行 live 前不宣称完整关闭。

该项不新增状态机，也不修改 CWI、candidate、dispatch、evidence 或 terminal authority。唯一实施队列见
[workspace-reasoning-effort-single-owner-closure-plan.md](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)。

### D. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选执行一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime 收口 | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### E. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

## Judgment

- **代码主链**：Runtime only；禁止的 authority 不再是问题来源。R3 不增加第二主链或第五状态机，而是
  SM-2 的 owner-local reconciliation 前置 input。complete candidate document 必须先持久化，
  single evaluator 只返回完整 typed diagnostic；是否 correction/final 由 SM-3 internal event 决定。owner
  feasibility 是正交 typed fact。C12 是必须保持的 SM-2 evidence/coverage 回归基线：只有 sealed binding + owner receipt
  才能关闭 CWI，LLM/raw check 不得越权；已收口的 semantic correction 与 projection 不能绕过或改写该基线。
- **正向收敛基础**：CAS、sealed authorization、三种 TargetPolicy、request session、whole-document recovery、receipt、
  binding-derived CWI credit、single-owner lifecycle/SM-4 producer trust 与 terminal delivery 均为 R3 回归基线；当前 active
  typed semantic correction directive、proof-batch wire、terminal-consistent canonical projection 与 RSO-23 provider effect
  echo removal 已 code-closed；仍需 deployed semantic positive matrix 和 recovery fault injection。latest persisted
  aggregate/live 反例优先于过去的本地绿灯。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；CWI 主线实施顺序由 R3 active plan 定义，workspace
  reasoning effort 实施顺序由 WRE active plan 定义，archive 不得推导新实现约束。
- **Active roadmap**：RSO 当前包含 typed semantic correction、canonical projection、MCP effective capability/effect
  feasibility、provider effect/idempotency echo removal、fresh LSP positive convergence 与 whole-document recovery fault
  injection；restart ordering 和
  complete-document recovery 是必须保持的代码回归基线。WRE 是独立必需收口项，用于 workspace reasoning
  capability/selection/seal/request/UI 合同。EC 的 22 组件 EventHub 合同与全链验收已经关闭并归档。OPT-1 为非阻塞
  可选验证。ARCH-1、RUNTIME-SMO、SMO-R2 C12、
  旧 VPC、OPS-1、RCA-1、RPF 和 RPA-1 均归 archive，不能覆盖当前 Runtime 设计。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
