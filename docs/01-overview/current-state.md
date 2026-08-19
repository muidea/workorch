# WorkOrch Current State

Status: active

Type: current-fact

Last Verified: 2026-08-19

## Purpose

本文只描述当前已落地的生产事实和真实未关项。历史 schema、workspace/run ID、逐提交收口过程和 benchmark 属于
`90-archive/`，不在当前事实中重复维护。

## Runtime Truth

**Production runtime path = Runtime only.**

```text
Application builds typed RuntimeBootstrap
  -> Workspace owner admits scoped execution intent
  -> Run owner performs SM-1 activation/continuation/replacement transition
  -> Run state fact triggers Orchestrator SM-4 drive
  -> SM-2 authority/focus convergence
  -> SM-3 semantic/candidate/sealed dispatch transaction
  -> capability-owner receipt
  -> SM-2 evidence/assessment/terminal
  -> SM-4 Result -> Workspace -> Run delivery
```

- Run owner CAS-persists lifecycle、replacement intent 和 opaque `RuntimeStateRecord`；它不解释 SM-2/3/4。
- Orchestrator 是 SM-2、SM-3、SM-4 的唯一 reducer、drive、evidence、governance-resolution 与 terminal authority。
- Capability Blocks 只提供 owner-defined prerequisite/feasibility/execute command/result 和 factual receipt。
- Result、Workspace、Run 只通过各自 private reducer 迁移自己的资源状态。
- Eventstream、projection、HTTP/SSE、TUI/Web 只审计或展示，不重建 authority、不隐式续跑。
- 恢复只从 persisted aggregate 幂等 re-drive；进程内 cache、历史文本和 projection 都不是恢复权威。

完整规约见 [Runtime 核心合同](../00-core/runtime-stage-contract.md)。

## Four State Machines

| ID | Owner | 当前职责 | 禁止越界 |
| --- | --- | --- | --- |
| SM-1 | `blocks/run` | Run lifecycle、governance wait/continue、administrative termination、replacement intent | 选择 CWI/action、解释 Runtime aggregate |
| SM-2 | `kernel/orchestrator` | authority、focus、CWI binding/credit、assessment、repair、terminal | 调 provider、执行 capability effect、写别的 Owner 状态 |
| SM-3 | `kernel/orchestrator` | semantic/candidate request session、attempt 0/1、sealed dispatch、receipt/evidence transaction | 选择下一 action、绕过 seal 执行 |
| SM-4 | `kernel/orchestrator` | bounded drive、singleflight、recovery lease/budget、delivery ledger | 从 projection 重建 authority、代写 Run/Workspace/Result |

所有外部 result 先映射为 Orchestrator private internal event；`ApplyInternalEvent` 和唯一 transition coordinator/CAS
是 aggregate mutation 边界。跨 Owner I/O 只使用 EventHub + concrete typed command/data/result。

## CWI And Candidate

- Activation 只封存 immutable Definition seed；workspace-specific target/recipe/binding 由后续 semantic planning 提出。
- LLM 只在 sealed semantic/candidate manifest 内输出完整 document，不选择 Runtime action、credit、retry 或 terminal。
- Capability owner 返回 feasibility/receipt；只有 SM-2 能封存 `CoverageEvidenceBinding`、选择 focus、retain/reopen CWI
  或派生 coverage credit。
- Candidate document 只进入一个 evaluator；complete rejection 只有 attempt 0 和可证明的同合同 attempt 1。
- `file/write` 是 create-only、`prospective_owner_target`。已有目标必须使用 owner catalog 中合法且绑定当前
  revision/baseHash 的 `file/edit` 或 `patch/apply` replacement。
- workspace-sensitive validation catalog 由 SM-2 先绑定 File manifest revision，再由 Shell owner 对声明 recipe 发布
  `ready | prerequisite_required | unavailable` surface 并过滤当前适用 confirmed set；空 confirmed set 不向 semantic
  provider 暴露 `validation/managed_session`，non-ready surface 仍保留 aggregate/audit，后续 File tree effect 会精确退休该 catalog。
- dynamic capability catalog 区分 callable operation、validation readiness 与 proof applicability；MCP required-input tool
  仍可被合法 operation 调用，但在 owner 尚无 sealed input binding 时只记录 `prerequisite_required` surface。技术上 ready
  的 auxiliary/not-declared surface 也只保留审计；只有 `ready + direct` 进入 semantic provider 的 recipe enum/C3。LLM
  projection 不暴露 non-selectable identity、owner-private binding/schema/effect 或 executable recipe 内容。
- profile 可以声明 owner-neutral postcondition，例如 `validation_preparation` 要求 Shell 当前 catalog 存在 selectable
  managed-validation surface。Workorchd 只编译该声明，Shell 只发布 typed catalog，Orchestrator SM-2 只在 sealed proof
  其余条件均成立后检查 `ready + direct` current fact；assessment claim 或一次成功 File patch 不能替代该事实。
- owner fact 是唯一缺口时，SM-2 在 assessment commit 的同一 CAS 中重开 current CWI、退休 current binding/semantic
  generation，并由 SM-4 幂等 re-drive；次数由 focus repair budget 有界。普通 proof gap 不走该分支，耗尽后以
  `owner_fact_convergence_exhausted` fail-closed。新 focus/binding 同时绑定 `manifestHash + logicalRequestID`；历史 reopened
  binding 不得重新成为 current，导致该缺口的 accepted operation shape 跨同一 CWI successor focus 保留并由 SM-3 防重放。
- current semantic generation 已精确绑定且仍位于 active/remaining focus queue 的 CWI 保持 executable lineage；它不重新
  获得 planning eligibility。duplicate/restart projection 过滤已绑定 row 并零 CAS，stale generation/binding 或 prerequisite
  drift 继续拒绝。
- terminal closeout 只汇总当前 unsatisfied CWI 的 canonical gap，top failure 以当前 `Facts.Failure` 为先；已满足 CWI 的
  producer claimed gap 只属于 assessment audit。
- raw candidate CWI、assessment claim、文件存在、Git revision、exit 0 和非空 `PassedChecks` 都不是 credit authority。

## Restart And Development Baseline

Runtime aggregate 只支持一个 current development baseline，不维护历史 migration/reducer：

- 非当前 aggregate 的 Resume 返回 `fresh_activation_required`，零 provider/capability work；
- 针对不兼容 predecessor 的 Restart 返回 `data_reset_required`，不创建 replacement intent；清理开发数据后重新激活；
- current-baseline Restart 由 Run owner 先持久化 predecessor `ReplacementIntent`，再按
  `Result -> Workspace -> Run` 完成 administrative terminal delivery，最后由 Run-private reducer 创建并激活 successor；
- Application、Workspace、Orchestrator 均不得提前创建 successor，也不得迁移 predecessor 的 CWI credit、request、
  candidate 或 recovery budget。

## LLM Provider

- 当前推理 provider 为内置 `ai-proxy`；catalog 使用 `/v1/models`，推理使用 `/v1/responses`，embedding 使用
  `/v1/embeddings`。
- Workspace owner 保存下一次 fresh activation 的 `DefaultLLMSelection`；Run bootstrap 封存 current-run selection。
- LLM owner 是 model capability 与 `ProviderCeiling` 的唯一解释者；Orchestrator 只封存并校验 typed fact。
- `ExpectedDocument.MaxOutputTokens`、EventHub command、provider request、provider-facing token 值和 audit 必须与
  sealed ceiling 精确一致。
- active LLM configuration 在进程启动时冻结；保存配置只影响下一次进程启动，当前投影显示 `restartRequired`。
- Provider-facing context 保持 C0～C5 分区；raw prompt/body、credential、endpoint 和未脱敏内容不进入 aggregate、
  Eventstream、projection 或 Memory。
- Skill 只以 activation-time sealed snapshot 进入 volatile C3：WorkOrch Registry 与 SkillHub Catalog 使用不同 typed
  provenance，最多 5 项，instruction 单项 8 KiB/总计 24 KiB，metadata 总计 16 KiB。LLM/Orchestrator 不直读
  Registry/Catalog，也不在请求或 drive 阶段重新选择、裁剪或提升 Skill 到 C0～C2。

## Component Snapshot

仓库保持 **1 Initiator + 4 Modules + 17 Blocks = 22 个 framework 运行单元**，另有一个非 framework unit 的
`internal/services/workorch` process service。完整职责和代码差异见
[Component Documentation Alignment](component-documentation-alignment.md)。

- Initiator：`workorchruntime` repository provider/factory。
- Modules：`workorchd`、`orchestrator`、`governance`、`system`。
- Stateful/resource Blocks：`workspace`、`run`、`result`、`decision`、`approval`、`memory`、`skill`、`soul`、
  `mcpregistry`、`eventstream`。
- Capability Blocks：`llm`、`file`、`git`、`shell`、`ssh`、`web`、`mcp`。

## Interfaces And Persistence

- 默认二进制模式启动 TUI；`--server` 启动 HTTP/Web 服务。
- Workorchd 构造 bootstrap、编排应用用例、维护 derived projection 和 SSE；它不拥有 Runtime drive 或正式 owner state。
- 正式资源只由对应 Owner repository 写入；projection store 是可重建读模型，EventRecord 是审计流。
- Workspace scope 绑定在每次 EventHub context；组件实例、全局 cache 和后台任务不得保存可被静默切换的
  “current workspace”。
- Memory item/trace command 同样强制该 scope；RecallTrace 采用 create-once exact replay。Activation 的候选、排除、
  quota、score/hit 与 byte/token budget 作为 sealed context provenance 持久化和投影，但不参与 SM-2 credit。
- Skill Block 只拥有 WorkOrch Registry；SkillHub 是 Application-private external catalog。Registry create 由 owner 签发
  canonical content hash provenance，get/list 使用 concrete bounded EventHub result；选择后的两类 Skill 都以 detached
  typed provenance 与 byte budget 封入 Run activation，后续源数据变化不具备 current-run authority。
- 普通 Workspace setup 的必填业务输入只有 Template、Goal、DoD；workspace directory 输入可选，空值解析为
  `.workorch-workspaces/<workspace-id>`，且不要求用户提交 Runtime JSON。

## Explicitly Not Current

以下内容不是受支持的生产 authority；文档和组件合同不再把它们描述为当前行为：

- `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`；
- application continuation queue、projection/watchdog authority、通用 RuntimeAdapter registry；
- 自动 replan successor、checkpoint replay、map/JSON/reflect authority bridge；
- 外部 target-state setter 或跨 Owner service/repository 调用。

Active default prompt、verifier/example 和 container packaging 已对齐当前合同。只读 compatibility projection 仍可保留
历史显示字段，但静态门禁禁止它们进入 owner aggregate、reducer、provider request 或执行选择。

## Open Work

当前保留三个未关队列：

1. [Workspace Goal/DoD global semantic convergence](../70-roadmap/active/workspace-goal-dod-global-semantic-convergence-closure-plan.md)：
   大量 Definition 的 semantic dependency、非序号/ID 排序、immutable C1 partition、bounded C5/focus 和全局 CWI
   持续收敛代码收口。
2. [Runtime positive-convergence live closure](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)：
   fresh-data real-provider persisted chain、existing-target revision-bound replacement、后续 CWI 推进和 terminal delivery。
3. [Workspace reasoning effort deployed verification](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)：
   Web/TUI、Resume/Restart、active configuration 和双 Workspace 隔离。

完整未关矩阵见 [current-unclosed-matrix.md](current-unclosed-matrix.md)。
