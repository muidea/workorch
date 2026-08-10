# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-08-10

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime 生产主链 | **Runtime-only**（`WorkspaceExecutionIntent → owner-local Run fact → DriveRuntime`）；application direct lifecycle、Run source guard、owner outcome forwarding 与 SM-4 producer trust 均为已关闭回归基线。 |
| Runtime receipt/binding 基线 | **已完成基线**；presentation slice 与 semantic focus 已分离，C12 保证 credit 只能从 sealed binding、operation 与 receipt 派生。 |
| Runtime 单 owner lifecycle、semantic contract 与 CWI reconciliation | **active（RSO-1～RSO-25）**；RSO-1～RSO-9、RSO-11～RSO-14、RSO-16、RSO-18～RSO-25 已 code-closed；`run-055`/`run-056` 已证明 MCP owner-fact mismatch 在 dispatch 前零副作用拒绝，`run-061`/`run-062` 暴露的 provider `requiredEffectScopes` echo 已删除，`run-063` 暴露的 provider `requiredCheckRefs` echo 已删除；`run-064`/`run-065` 暴露的 owner-feasibility timing、MCP catalog/probe、focus batch 和 failure-class 缺口已完成本地收口。最新 `run-066` 在 MCP 行为探针事实不足时以 `candidate_infeasible/mcp_semantic_proof_unavailable` 零 dispatch fail-closed，属于负向证明而非正向收敛。RSO-22 仍需 fresh positive 与 zero-dispatch deployed convergence；whole-document recovery 仍需独立 deployed fault injection。见 [Runtime plan](runtime-cwi-reconciliation-single-owner-closure-plan.md)。 |
| Workspace reasoning effort | **code-closed，待 deployed**；canonical selection、owner CAS、Run seal、Responses `reasoning.effort`、Web 与 TUI 合同已落地，剩余 live matrix，见 [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md)。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework structure | **已完成并归档**；1/4/17、Base Biz、owner/lane metadata、owner-local Topic/DTO、MessageContract root alias 零债务、sender/receiver、Post subscriber Result 零依赖与递归 concrete/resource 图已固化；当前 260 个 Topic，全仓 race、console、managed deployed 与 fresh 双 workspace/C7 已通过，见 [归档记录](../../90-archive/completed/component-event-contract-closure-plan.md)。 |
| 禁止的 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Mandatory Work

当前 Runtime CWI 主线的代码增量已覆盖 typed semantic correction、proof-batch 最小 wire、exact-active-batch reducer
invariant 与 canonical combined Definition projection，且 RSO-9 增量 race/managed 已通过。`run-054` 已证明新部署
可以加载 MCP typed metadata，并推进至 semantic seal、dispatch、receipt/evidence commit；`run-055`/`run-056` 又证明
不匹配 recipe/effect 会在 dispatch 前以零副作用拒绝，但这只是 fail-closed 防线。`run-061`/`run-062` 证明 provider
仍提交 `requiredEffectScopes` 并在 recipe 间交换 effect；RSO-23 已删除 provider effect/idempotency echo，由 owner
feasibility fact 经 Orchestrator 私有事件和 SM-2 编译 binding。`run-063` 又证明 provider 可提交 owner 无法执行的
`requiredCheckRefs`，RSO-24 已完成代码收口；`run-064`/`run-065` 暴露的 owner feasibility timing、MCP behavior probe、focus batch 与 failure classification 已完成 RSO-25 本地代码收口。`run-066` 已补充缺失 MCP proof 的零 dispatch 负向证据。下一步执行 RSO-22 fresh positive/zero-dispatch convergence，并
通过单独 fault injection 证明 whole-document recovery。此前 restart release-before-create、
complete-document、22 组件 EventHub 合同回迁、owner-local terminal delivery、typed diagnostic、single mutation surface、
managed verifier 与 reasoning selection 仍是回归基线，但不能替代最新 persisted aggregate/provider/projection 反例。

语义 catalog freshness 只允许由带 expected/observed revision fence 的 `stale` owner fact 触发；当前没有 typed
semantic prerequisite acquisition 合同时，`prerequisite_required` observation 保留原始诊断和既有 catalog，直接
fail-closed，不创建第二条 refresh/状态机路径。

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| RSO | typed semantic correction + canonical projection + single-owner CWI convergence | SM-3-sealed correction directive; provider-safe C4; terminal-first outcome/activity; preserve restart/complete-document baselines; feasibility/seal/dispatch/evidence and recovery live | [active plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| WRE | Workspace reasoning effort deployed proof | 代码收口完成；验证 Web Start/Restart、TUI `/model reasoning`、Resume sealed selection与双 workspace隔离 | [active plan](workspace-reasoning-effort-single-owner-closure-plan.md) |
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
8. C12、slice/focus 分离、普通用户 raw contract JSON 删除、owner contract 回迁、restart release-before-create、
   complete-document recovery 与 managed matrix 仍是有效代码基线。最新 live 打开的缺口曾是 typed semantic correction
   directive 和 canonical terminal projection；whole-document recovery 仍缺 deployed fault injection。不得以 raw message、
   prompt overlay、ticket、budget/decoder 放宽、字符串清理、合同 alias/forwarder 或历史绿灯绕过 RSO。
