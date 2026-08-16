# Component Documentation Alignment

Status: active

Type: current-fact

Last Verified: 2026-08-16

## Purpose

本文记录组件定义与当前实现的对齐结果，回答两个问题：

1. 每个运行单元实际拥有的功能、状态和跨组件合同是什么；
2. 此前 README 中哪些表述已偏离代码，当前如何修正。

本文不是新的 Runtime 语义来源。Runtime 状态机仍以
[Runtime Stage Contract](../00-core/runtime-stage-contract.md)、
[Orchestrator implementation contract](../../internal/modules/kernel/orchestrator/readme.md) 与
`internal/pkg/workorchruntimecontract` 为准。

## Review Scope

- 22 个 framework 运行单元：1 个 Initiator、4 个 Module、17 个 Block；
- 1 个进程级 service shell：`internal/services/workorch`；
- 根 README、全部非归档文档，以及 active / archive 导航；
- `docs/90-archive` 只保留历史证据，不按当前实现重写正文。

核对事实来自实际注册入口、`module.go` / `plugin.go`、owner `biz`、各 owner 的
`pkg/events`、repository provider、HTTP route/DTO、Runtime contract 与现有回归测试。README
中的复杂度快照、阶段编号和单次运行记录不再作为组件定义。

## Alignment Result

| Unit | 当前功能与唯一 owner 边界 | 已清理或纠正的偏差 | Contract |
| --- | --- | --- | --- |
| Repository Provider Initiator | 只创建 memory/file repository 分组并管理基础设施生命周期；不注册 EventHub 或业务状态 | 移除 runtime policy、bootstrap export 与跨 owner facade 暗示 | [README](../../internal/initiators/workorchruntime/readme.md) |
| workorch process service | 选择 server/TUI/remote CLI 模式，装配进程生命周期与稳定 port；不属于 22 个 framework unit | 明确 local wait state、session 和 transport 都不是跨组件合同 | [README](../../internal/services/workorch/readme.md) |
| application/workorchd | HTTP/SSE、typed bootstrap、execution admission、projection 与应用用例；不拥有 Runtime drive 或 owner 状态 | 删除 application continuation queue、resolved watcher Resume、提前创建 restart successor 和 execution-context patch 主张 | [README](../../internal/modules/application/workorchd/readme.md) |
| kernel/orchestrator | 唯一 SM-2/SM-3/SM-4 reducer、candidate/sealed-dispatch、evidence、governance resolution 与 delivery ledger owner | 移除历史阶段编号、schema 版本号、active CWI 清单和自动 successor 表述 | [README](../../internal/modules/kernel/orchestrator/readme.md) |
| kernel/governance | 治理规则、风险/策略评估与不可变治理事实；不修改 Run/Decision/Approval 状态 | 移除阶段性代码拆分指标，将 resolve 后续推进归还 Orchestrator/Run owner | [README](../../internal/modules/kernel/governance/readme.md) |
| kernel/system | 启动时协调默认资源 ensure；不拥有被确保资源 | 纠正 ensure command 的定义方：合同由 Soul、Workspace、MCP Registry 等接收 owner 定义 | [README](../../internal/modules/kernel/system/readme.md) |
| blocks/workspace | Workspace、Goal、DoD、Template、canonical selection、execution intent 与 active-run release owner | restart 只做 definition/scope 准入并转交；删除 Workspace/Application 创建 successor 的旧表述 | [README](../../internal/modules/blocks/workspace/readme.md) |
| blocks/run | 唯一 SM-1 owner；持久化 Run lifecycle/lineage、opaque RuntimeStateRecord CAS 与 ReplacementIntent | schema 数字改为 current development baseline；current-baseline successor 只由 Run-private reducer 在 terminal delivery 后创建 | [README](../../internal/modules/blocks/run/readme.md) |
| blocks/result | Result/Artifact formal-outcome owner，只接收 sealed terminal material | 删除 generic entity write 暗示；accepted/rejected 裁决仍属于 Orchestrator，Result 只持久化已裁决 outcome | [README](../../internal/modules/blocks/result/readme.md) |
| blocks/decision | DecisionTicket/Resolution owner，支持查询、删除、普通 resolve 与 Runtime governance 事务 | 删除不存在的通用 create/update/supersede EventHub facade 和泛化通知 | [README](../../internal/modules/blocks/decision/readme.md) |
| blocks/approval | Approval owner，支持查询、删除及已有 pending approval 的 approve/reject | 删除不存在的公开 create/update/supersede command 与多通知族 | [README](../../internal/modules/blocks/approval/readme.md) |
| blocks/memory | MemoryItem/RecallTrace owner | 明确 repository query 只在 owner 内部，跨 owner 只使用具体 EventHub DTO | [README](../../internal/modules/blocks/memory/readme.md) |
| blocks/skill | Skill 定义、版本与查询 owner；不执行能力副作用 | 清除阶段性复杂度记录，保留 owner-local CRUD/notification 边界 | [README](../../internal/modules/blocks/skill/readme.md) |
| blocks/soul | Soul/版本/默认资源 owner；提供 activation 使用的内容快照 | 明确默认资源 ensure 由 Soul 自己定义并执行，不由 System 代写 | [README](../../internal/modules/blocks/soul/readme.md) |
| blocks/mcpregistry | MCP server/capability catalog 与选择元数据 owner；不执行 Runtime MCP operation | 区分 catalog/selection 与 capability execution，ensure 合同归接收 owner | [README](../../internal/modules/blocks/mcpregistry/readme.md) |
| blocks/eventstream | 有界 EventRecord 审计存储与查询 owner；projection/replay 只读 | 删除用事件重建 authority、修复 owner 状态或驱动 Runtime 的暗示 | [README](../../internal/modules/blocks/eventstream/readme.md) |
| blocks/llm | provider catalog/config snapshot、provider ceiling、LLM/embedding/candidate factual call owner | 修正 provider-ceiling topic；明确 LLM 不选择 attempt、retry、terminal 或 Runtime transition | [README](../../internal/modules/blocks/llm/readme.md) |
| blocks/file | file create/read/patch/text-replace 等具体 operation owner，独占目标 revision 与 create/replace admissibility | `file/write` 明确为 create-only；既存目标必须使用带目标 revision 的 replacement operation | [README](../../internal/modules/blocks/file/readme.md) |
| blocks/git | Git concrete operation owner | 将错误的 Orchestrator generic operation command 改为 Git owner 自己定义的 typed command/result | [README](../../internal/modules/blocks/git/readme.md) |
| blocks/shell | Shell concrete operation owner | 同上；副作用、准入和 factual result 都留在 capability owner | [README](../../internal/modules/blocks/shell/readme.md) |
| blocks/ssh | SSH concrete operation owner | 同上；连接/执行事实不进入 Orchestrator 通用 service facade | [README](../../internal/modules/blocks/ssh/readme.md) |
| blocks/web | Web search/fetch concrete operation owner | 同上；Web owner 返回事实，不决定 evidence acceptance 或 Runtime transition | [README](../../internal/modules/blocks/web/readme.md) |
| blocks/mcp | MCP concrete operation owner | 区分 MCP Registry metadata owner 与 MCP execution owner，并纠正 typed operation 合同归属 | [README](../../internal/modules/blocks/mcp/readme.md) |

## Cross-Component Baseline

所有组件 README 现在共同遵守以下规则：

- 每个正式状态机只有一个 owner，状态转换只能由该 owner 的 private internal event/reducer 选择；
- 外部组件只能提交意图或不可变事实，不能提交目标状态；
- 跨 owner I/O 只允许 EventHub + 接收/投递 owner 定义的具体 typed command/result/data；
- owner 不暴露 service/repository facade，调用方不持有 sibling owner 的 `biz`、service 或 repository；
- workorchd 负责入口、bootstrap、用例和投影，Run 负责 SM-1，Orchestrator 负责 SM-2/3/4，capability block
  负责副作用，Result 负责已裁决 outcome 的持久化；
- projection、Eventstream、HTTP DTO 和历史 Step/Recovery 字段都不是 authority，也不能反向修复 owner 状态；
- 当前开发基线不以 README 固化 schema 数字。非当前数据 fail closed；兼容或迁移策略必须另立明确合同。

## Documentation Cleanup Result

| 文档层 | 当前处理 |
| --- | --- |
| Core / implementation SoT | 保留稳定语义，去掉单次运行编号、阶段性关闭指标和失效 schema 数字 |
| Component README | 只写当前职责、交互合同、owner 边界和稳定约束；统一标记 `Last Verified` |
| Current overview | 只陈述当前代码事实和真实未关闭项，不保留已完成代码收口清单 |
| Active roadmap | 只保留仍需真实部署或 live evidence 的工作 |
| Verification | 将 owner/SM/invariant/restart/live closure 断言集中到稳定矩阵 |
| Archive | 历史方案、复杂度盘点和完成记录迁入 `docs/90-archive`；可追溯但不约束当前实现 |

`internal/modules/contract_inventory_test.go` 现在以本当前组件盘点校验 registered units，不再读取已完成的
active roadmap 计划。因此路线图归档不会再破坏代码测试，archive 也不会反向约束当前组件形态。

## Implementation Drift Closure Result

The review found the following executable drift and the same refresh closed it before this inventory was finalized:

| Area | Observed implementation content | Current contract | Disposition |
| --- | --- | --- | --- |
| built-in templates/Soul Markdown | provider guidance now uses semantic-plan/operations/binding/receipt/evidence language | provider-facing guidance must use typed semantic-plan/candidate/focus/evidence contracts | closed + guarded |
| executable verifier wrappers | current SM-1..SM-4/Owner tests replace graph/context/recovery fixtures; empty test selection fails | active verifier must exercise current SM-1..SM-4 path or be explicitly read-only compatibility | closed + guarded |
| container verifier/example | Workspace/Goal/DoD + `execution-activations`; removed Run writes must return `404` | only workspace `execution-activations` mutates execution | closed + verified |
| Docker builder | embedded console copied and dependency download cached | clean image build must include embedded console package/assets | closed + verified |
| read-model compatibility | still reads historical graph-like payload fields | allowed only as read-only projection; cannot feed authority | retain with explicit non-authority guard |

Evidence is recorded in the
[Runtime Contract Drift Code Closure Record](../90-archive/records/runtime-contract-drift-code-closure-record-2026-08-16.md).

## Maintenance Rule

后续变更必须同时更新代码事实源、对应组件 README 和其所属权威文档；不得只更新路线图或单个调用方文档。
新增或改变状态转换时，变更说明必须标识受影响的 SM，并验证 owner-private transition、stale/duplicate、CAS、restart、
budget/guard 与 terminal delivery。若某份文档只记录一次运行或一次重构，应直接进入 archive/record，不得长期留在
active 或组件 README。
