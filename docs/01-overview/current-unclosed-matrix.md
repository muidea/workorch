# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-08

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

### A. Runtime 单 owner semantic contract 与 CWI reconciliation（R3，active）

presentation slice 与 semantic focus 分离、完整 preflight、resume/restart reconciliation、activation recommendation，
以及 Goal/DoD + selected template/profile + owner facts → semantic proposal → owner feasibility → SM-2 seal 已完成代码基线。
普通 UI/API 的 raw `contextHints/evidencePolicies` JSON 输入已删除，合同声明只来自 template/profile；run snapshot、
RuntimeView 与 Web acceptance 已展示 run-scoped planning phase、逐 CWI owner fact、完整 original diagnostic 与 gap lineage，
并以 workspace/run/state revision 门禁拒绝 stale 混合。managed verifier、空 workspace prospective target、
resume/restart/drift、superseded predecessor 隔离、crash/concurrency 与 CPU guard 已通过 fresh deployed 验证。最新合同
审计确认 capability execute 与 LLM provider EventHub contract 曾由 Orchestrator 代接收 owner 定义；当前 File、Shell、Git、
MCP、SSH、Web execute、LLM provider ceiling/preflight/request、八个 owner 的 Runtime capability acquisition、六个 owner 的 operation admissibility acquisition、operation feasibility 与 semantic-proof feasibility、四个 owner 的 CWI reconciliation query、File target snapshot/selection/manifest/repair-surface query，
以及 Git capability-state acquisition、Decision governance-request 合同已直接回迁各自 `blocks/*/pkg/events` 并由 boundary verifier 固化；Run/Result/Workspace lifecycle 合同审计也已确认由各自 owner 定义并消费。
owner observation/failure/diagnostic、EventHub 接收 owner 归属与 Orchestrator private mutation surface 已完成 R3-F1～F5
代码及全链验收；controller 只公开唯一 internal-event mutation dispatcher 和两个 scoped read，所有 lane caller 只传唯一
接收 owner `Contracts()`，生产合同聚合与 owner/topic 错配由 AST 门禁拒绝。当前强制遗留仅为 fresh LSP Bridge 逐 CWI
binding-matched semantic matrix。

`SMO-R2-C12` 仍是 raw-credit 防线：candidate raw CWI、assessment claim、`PassedChecks`、exit code、文件/Git
存在不得直接关闭 coverage。R3 必须在保留该防线的前提下自动完成 semantic contract：

- `/run restart` 以最小 Definition seed 创建 new Run；SM-3 请求受约束 semantic-plan document，owner 返回
  feasibility，只有 SM-2 internal event 能 seal binding/reconciliation contract；
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
  feasibility 是正交 typed fact。C12 重新打开的不是 recovery/owner/projection，而是 SM-2 evidence/coverage
  authority：只有 sealed binding + owner receipt 才能关闭 CWI，LLM/raw check 不得越权。
- **正向收敛基础**：CAS、sealed authorization、三种 TargetPolicy、request session、receipt、
  binding-derived CWI credit、single-owner lifecycle/outcome/SM-4 producer trust 与 terminal delivery均为 R3 回归基线；
  当前 active 缺口是 minimal bootstrap 后的自动 semantic-plan/owner-feasibility/SM-2 seal 事务、普通产品入口收拢与
  deployed semantic matrix。latest persisted aggregate/live 反例优先于过去的本地绿灯。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；CWI 主线实施顺序由 R3 active plan 定义，workspace
  reasoning effort 实施顺序由 WRE active plan 定义，archive 不得推导新实现约束。
- **Active roadmap**：R3 当前仅剩 E3 live；WRE 是独立必需收口项，用于 workspace reasoning capability/selection/seal/request/UI 合同。EC 的 22 组件 EventHub 合同与全链验收已经关闭并归档。OPT-1 为非阻塞可选验证。ARCH-1、RUNTIME-SMO、SMO-R2 C12、
  旧 VPC、OPS-1、RCA-1、RPF 和 RPA-1 均归 archive，不能覆盖当前 Runtime 设计。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
