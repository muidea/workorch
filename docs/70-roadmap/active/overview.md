# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-08-03

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime 生产主链 | **Runtime-only**（`WorkspaceExecutionIntent → owner-local Run fact → DriveRuntime`）；application direct lifecycle、Run source guard、owner outcome forwarding 与 SM-4 producer trust 均为已关闭回归基线。 |
| Runtime receipt/binding 基线 | **已完成基线**；presentation slice 与 semantic focus 已分离，C12 保证 credit 只能从 sealed binding、operation 与 receipt 派生。 |
| Runtime 单 owner lifecycle、semantic contract 与 CWI reconciliation | **active（仅 R3-E3）**；Runtime-sensitive owner 合同回迁、单 mutation surface、全仓 race 与 fresh managed deployed matrix 已闭环；剩余 fresh LSP Bridge 逐 CWI 验收，见 [Runtime plan](runtime-cwi-reconciliation-single-owner-closure-plan.md)。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework structure | **分类/lifecycle 与 EventHub 代码收口已完成，验证尾项 active**；1/4/17、Base Biz、owner/lane metadata、typed DTO、MessageContract root alias 零债务、sender/receiver、Post subscriber Result 零依赖与递归 concrete/resource 图已固化；307-topic 基线累计淘汰 41 个，当前 266 个，剩余 race/console/managed deployed/fresh dual-workspace 验证见 [component EventHub plan](component-event-contract-closure-plan.md)。 |
| 禁止的 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Mandatory Work

当前必须关闭的 Runtime 项仅剩 **fresh LSP Bridge deployed semantic matrix**；同时存在独立的 **22 组件 EventHub
合同归一**，它不新增或修改 Runtime 状态机。CWI semantic contract
自动编译、deployed reconciliation、空 workspace prospective target、resume/restart/drift/crash/concurrency 与 CPU guard 已形成
基线。capability execute、LLM provider、Runtime capability acquisition、operation admissibility/feasibility、semantic-proof
feasibility、CWI reconciliation、File target/manifest/repair-surface、Git capability-state 与 Decision governance-request 合同
均已回迁接收 owner；Run/Result/Workspace lifecycle、feedback/diagnostic 与 controller mutation surface 也已闭环。
R3-F1～F5 已完成 Runtime-sensitive scope 的代码与全链验收；生产 lane caller 显式使用唯一接收 owner `Contracts()`，禁止跨 owner inventory 聚合，
fresh managed artifact 为 `/tmp/workorch-r3-f5-managed-20260803c`。剩余仅为 fresh LSP live。
LSP live 必须继续由受约束 semantic-plan document + owner feasibility 形成 proposal，并仅由 SM-2 seal；合同迁移必须由
接收 owner 定义 concrete EventHub contract，原始诊断不得被 stable category/摘要覆盖。

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| R3 | single-owner semantic contract + CWI reconciliation + contract consolidation | minimal bootstrap；SM-3 typed document；SM-2 seal；`resume=current_run`、`restart=workspace_full`；接收 owner EventHub 合同；private internal event；canonical feedback/original diagnostic；LSP Bridge 逐 CWI验收 | [active plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| EC | 22-component EventHub topic/data/result closure | 22/22 代码完成；307-topic 基线累计删除 41 个、当前 266 个；仅剩 race、console、managed deployed 与 fresh dual-workspace 验证 | [active plan](component-event-contract-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 可补一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime 收口 | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime 的正式约束只来自 `00-core`、Orchestrator readme 与 typed contract；所有 completed record 都仅作
   回归基线，不得从历史记录另行推导状态迁移、lifecycle 或 live 修复队列。
2. current-schema completed Run 必须有 CWI→binding→sealed operation→owner receipt→evidence 链；并且 binding
   必须声明该 CWI 的独立语义 proof。机械执行绿灯、TODO test、泛化 command success、candidate raw CWI、assessment
   claim、`PassedChecks` 或 terminal 都不能替代该链。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：禁止的 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 旧 `RUNTIME-SMO-R2` 与 `OPS-1` 均已归 archive；其中 OPS-1 只是假定存在 production maintenance command
   的历史记录，不能作为维护窗口任务。
8. C12、slice/focus 分离、activation recommendation、semantic contract 自动编译、普通用户 raw contract JSON 删除、
   R3-E2 deployed matrix 与 Runtime-sensitive R3-F 合同回迁已收口；current R3 只剩 fresh LSP Bridge 逐 CWI semantic
   matrix。R3-F 的历史绿灯不代表 307 个 EventHub 合同已经完成全量使用审计，EC 计划是该范围的唯一实施队列。
   不得以 ticket、手工 overlay、budget/decoder 放宽、合同 alias/forwarder 或历史绿灯绕过 R3-E3/EC。
