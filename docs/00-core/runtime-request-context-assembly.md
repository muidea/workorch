# Context Assembly Flow

Status: normative


Type: runtime-flow

Last Verified: 2026-08-31

## Purpose

本文件定义 Runtime 为 candidate 请求组装上下文的链路：
**typed `RuntimeCandidateContext` + provider-facing C0–C2 stable prefix + C3–C5 current/volatile tail**。

生产主线**不**从 禁止的 execution-context map、临时 JSON bridge 或 full runtime dump 重建 authority。

## Flow

```text
persisted RuntimeFacts + RuntimeDefinitionContext
  -> RuntimeCandidateContextFromFacts
  -> seal with AuthorityTransaction / Decision
  -> RequestRuntimeCandidateCommand
       DefinitionContext
       CandidateContext
       StablePrefixHash
       CandidateAttempt + optional bounded correction observation/input
  -> LLM block renders provider-neutral sections (C0–C5)
  -> provider adapter maps cache plan (adapter-only private fields)
  -> LLM result audit must bind the same canonical request identity;
     the sealed transaction remains Orchestrator session-owned
```

## RuntimeCandidateContext

`RuntimeCandidateContext` 从 **persisted facts** 构造，显式携带：

| 字段 | 含义 |
| --- | --- |
| WorkspaceID / RunID | 作用域 |
| DefinitionRevision / EvidenceRevision | 与 authority 对齐的 revision |
| Focus | 当前 `CoverageFocus` |
| Evidence | accepted / rejected evidence ledger 视图 |
| Assessments | 已提交 assessment facts |
| Failure | 当前 `FailureFact`（若有） |
| Capabilities | owner 已确认的 capability 事实 |
| Resources | manifest、target snapshot、validation scratch、provider ceiling 等 |
| Activation | 封存的 `RuntimeActivationContext`（Soul/Skill/MCP/Memory、execution contract 等） |

校验：

- scope、revisions、focus 必填
- Activation 自身 `Validate()`
- 与 sealed authority snapshot（workspace/run/definition/evidence/focus）一致，否则拒绝

LLM block **不得**：

- 从 禁止的 map 或临时上下文重建 authority
- 在 provider document/result 中复制、重建或改写 transaction authority
- 把 provider/model/baseURL 动态绑定写回 facts

## Assembly Inputs（typed）

| 输入 | 来源 | 说明 |
| --- | --- | --- |
| Definition context | Bootstrap → aggregate | Goal/DoD identity、revision、capability contract |
| Activation context | Bootstrap → Facts | Soul/Skill/MCP/Memory 快照、EvidencePolicies、ExecutionContract |
| Current focus + remaining queue | Facts | required evidence roles、accepted/rejected kinds |
| Accepted / rejected evidence | Evidence ledger | 仅当前 Run；coverage 绑定 |
| Failure / diagnostics | Facts | stale 只进 diagnostic lineage，不污染 authority |
| Prerequisites results | manifest / target_snapshot / … | 缺失则先进入 `need_prerequisite`，不得混入 state-changing requiredOutput |
| Provider ceiling | preflight | 预算与模型上限 |
| Stage / template policy（语义） | Activation / C2 输入 | **不是** kernel 状态机状态名 |

## Authority Transaction Binding

candidate request 封存完整 authority transaction。返回的 candidate 必须绑定同一 snapshot：

- transaction 漂移 → 不得进入 seal / 执行
- attempt 1 必须复用相同 transaction 与 `StablePrefixHash`
- `BeginCandidateAttempt` 在请求前持久化 attempt 状态
- 第二次 complete candidate rejection 形成 typed candidate terminal；不存在第三次 attempt，也不创建无信息 governance

## C0–C5 Cache Partition

provider-facing 请求按 cacheability 分层（细则见 `../00-core/runtime-request-cache-note.md`）：

| 层 | 内容 | 稳定性 |
| --- | --- | --- |
| **C0** | stable runtime prefix（身份、中立 runtime 规则 / schema） | 最高 / 长寿命 |
| **C1** | workspace definition（Goal/DoD 正文等 definition 快照） | 随 definition revision |
| **C2** | capability contract / governance 声明 / profile owner-fact postcondition | 声明式稳定 |
| **C3** | current authority/focus、required output、sealed manifest 与 manifest-scoped execution context | 随 authority/decision 变 |
| **C4** | volatile execution tail（failure、recent evidence、attempt diagnostics） | 易变 |
| **C5** | large refs / trace | ref-only |

稳定要求：

- 出站请求第一段连续渲染 C0 / C1 / C2 作为稳定 prefix
- C3–C5 必须在稳定 prefix 之后
- Goal / DoD 不被静默裁剪
- provider/model/baseURL、step 动态 id 不得进入稳定 prefix
- raw stream、full provider payload、large artifact 默认 ref-only
- `StablePrefixHash` 由 definition + execution contract 等稳定输入计算，并随 attempt 持久化
- `stablePrefixHash` 意外漂移视为 request rendering bug
- C2 `RuntimeOwnerFactRequirement` 只声明 postcondition；operations C3 的 operation vocabulary 只由 sealed manifest
  投影。完整 owner catalog、admissibility inventory 与 freshness observation 仍留在 aggregate/request-session 本地校验，
  不能作为第二份 provider 选择合同；只有 manifest 命中的 target snapshot、stage activation 与 bounded supporting fact
  可以进入 C3。每个 manifest prerequisite/admissibility snapshot ref 必须在完整 typed context 中命中同一 canonical
  supporting fact，否则请求在 provider submission 前失败。assessment、旧 catalog 或 C4 diagnostic 不得回写 C2，
  也不得由模型宣称事实已满足。

core 只定义 provider-neutral 分区与审计；provider 私有 cache-control 仅 adapter 层设置。

### Semantic 请求的只读覆盖进度

每个 `semantic_plan` 请求必须携带 typed `RuntimeSemanticCoverageProgress`。Orchestrator 从同一 aggregate 快照
投影完整封存 CWI 清单的 `total/open/satisfied/unsatisfied`；不能把本次 eligible manifest 的数量当作 Goal 总量。
明细仅包含 coverage-order manifest 或当前 proof batch 的 CWI，按其规范顺序输出验收观察 `observedState`、当前记录的
`EvidenceRefs` 数量和 `ReopenLineage` 数量。证据引用数量不代表全部历史执行次数、fresh proof 或验收 credit；
重开后证据引用可能已被清理，不能把它当作累计工作量。原准则与具体修复结果仍分别来自 C1/C3 和下述因果反馈。

摘要绑定 workspace/run、Definition/startup、seed/manifest、step/batch、source state revision 与 evidence revision，
并带完整内容 hash。缺失、重复、错源或未知状态的 owner facts 必须拒绝，不靠删除坏行或默认 open 补足计数。
整体 JSON 上限 64 KiB，同时受 provider ceiling 限制。LLM 先校验 typed scope/hash/数量与成员完整性，再对 C4
做逐值 integrity 检查；受保护进度不得静默裁剪。hash 证明传输一致性，不授权 LLM 自报验收。

只读进度只进入 C4；单独更新它不得改变 C0～C3 或 stable prefix。重启从当前 owner 快照重新投影，同一快照的
值/hash 一致，合法 CAS 后 source revision 可以更新。它不持久化第二份状态账本、不修改旧归档、不触发状态转换，
也不改变 SM-2 的验收、SM-3 的 dispatch 或既有预算权限。

### Semantic 请求的已提交操作历史

每个 semantic 请求还必须携带 `RuntimeSemanticExecutionHistory`，由 Orchestrator 从同一 Run 的 persisted receipt
ledger 按提交顺序投影最近 16 条 owner-committed 结果，显式提供 `totalReceipts/omittedReceipts/receiptSequence`。
空历史也必须有完整 scope/source/hash，不能用缺字段冒充“尚未执行”。历史覆盖整个 sealed CWI inventory，允许看到
已经完成、因而不在当前 manifest/batch 内的 CWI 的工作；不能只把当前 proof batch 当作全部工作历史。

投影前复用 aggregate 不变量，并逐项反查 sealed dispatch、operation、execution key/fence 和 exact receipt ref/hash；
缺失、重复、错源或不一致不能在裁剪时被丢弃。`boundCoverageWorkItemIds` 仅来自原 operation 的 binding，表示历史
工作归属而不是 evidence credit；`observedState=accepted`、`changed=true` 或 durable effect 均不能替代当前验收。
尚无回执的 sealed/claimed/unknown execution 不属于该“已提交回执”计数，不能据此推断未执行或可安全重试。

每条保留 owner/kind、原 target refs、before/after hash、observed outcome/effect、changed 和有界 typed validation
check 事实。`expectedEffectScope` 必须来自 sealed execution，`requestedTargetRefs` 来自 sealed operation；二者与回执的
实际 effect/target 分开保留，不以预期补造实际事实。无 validation observation 时明确为 null；不包含原始
summary/diagnostic、output、操作参数或下一动作。
历史失败不等于当前修复来源，不能代替下述专用 exact-causal feedback；超过 16 条的旧历史明确计为省略，不能声称
已完整展开。整组 JSON 上限 64 KiB，并受 provider ceiling 约束；已选窗口超限必须拒绝，不静默删行或裁剪检查。

scope 绑定 workspace/run、Definition/startup、当前 manifest 与同一 CoverageProgress 的 source revision；完整值进入
C4，LLM 接收端检查身份、计数、序号、CWI membership 与 hash，渲染后逐值执行 `P3.semantic_execution_history`
integrity。单独反馈变化不得改变 C0～C3 或 stable prefix。同版本恢复只从 owner aggregate 重投影，session 提交造成
revision 前进不改变已有回执内容；没有新 owner、可写投影、状态机、预算或归档回填。

历史回执及下述 exact repair feedback 通过同一个纯投影携带 `diagnostic`：原 owner/class/code/signature、
expected/observed revision、原 truncated/redacted 标志和绑定完整原 feedback 的 hash；`summaryOnly=true` 必须显式保留。
可选 `validationDiagnostic` 只保留 owner 自报的 class/code、command kind、`reportedExitCode` 和 timeout；退出码默认值、
命令种类或诊断存在均不能证明进程执行过、检查通过或可修复，实际执行检查仍来自独立 validation observation。
缺少 owner diagnostic/validation diagnostic 分别显式为 null；修复专用反馈因其原 receipt 必须有 diagnostic 而不允许
省略该来源。原 message/reason、preview、detail ref、field values、输入维护授权与治理续驱均不透传；该投影不是完整
诊断材料，也不授予通过 hash 读取 owner 私有内容的权限。全部新增值只在 C4 接收/逐值保护，不改 C0～C3，
同版本重投影保留相同内容及身份。

### Semantic 请求的执行预算观察

`RuntimeSemanticExecutionBudget` 是必需的只读 C4 事实，与 CoverageProgress 使用同一 workspace/run、Definition/startup、
manifest 和 exact source state revision。Orchestrator 验证完整 aggregate 后，从既有 execution-budget view 选择
`provider_call`、`capability_operation` 两个 Run 物理维度，保留 limit、confirmed、reserved、unknown、remaining、
exhausted、policy revision 和 source-view hash；不把 allocation 明细、token usage 或当前 focus 的 repair 额度冒充全 Run 余额。

同一投影从当前及退休 semantic batches 汇总原 CWI 的 selected membership，明细只覆盖当前 manifest/batch。
每个已编译批次占一次，pending membership、同批 correction、session/recovery 不另计逻辑批次；generation retirement
不退额。`proofBatchLimit=null` 和逐项 `remaining=null` 只表示未声明附加的逐 CWI 上限，不表示无限执行；既有 request、
history、repair、recovery 和物理硬上限继续生效。此处的 proof batch 数也不等于实施迭代数或验收进度。

同一观察还必须包含 `durableDispatchLimit` 与 `visibleDurableDispatches`：从完整 validated aggregate 的原始 accepted
dispatch lineage 推导逐原 CWI durable seal 次数，再选出当前可见 CWI。封存即占额，失败/失效/无实际变更不退款；
治理续派、重复回执及重启不重复计数。未声明附加上限时 limit/remaining 显式为 null，但 used 仍如实保留。
任何隐藏 CWI 的负值、越额或外来身份也必须拒绝，不能用可见窗口遮蔽非法来源；该观察不构成实施权限或验收 credit。

这是构造请求时、该次 physical submission CAS **之前**的余额快照；本次请求仍须由 SM-3 在当前账本预留额度。
prepared/recovery 重驱重新读取当前快照，不能拿旧 revision 从新 aggregate 重建旧余额；已完成文档只恢复本地准入。
同版本同快照投影/hash 一致，submitted-unknown 保守占额。共享层只验证值和绑定，LLM 不预留、退还或重置预算。

接收端验证 scope/hash、两维完整性、封存的可选限额、溢出安全的余额等式和可见 CWI 完整性；渲染后执行
`P3.semantic_execution_budget` 逐值校验，协调重算 hash 也不能掩盖裁剪或改值。整个观察不超过 32 KiB，仍受 provider
ceiling 约束，不能隐式删项。所有动态值仅进入 C4，不改变 C0～C3/stable prefix，不赋予工作、dispatch、credit 或终结权。

### Semantic repair 的执行事实反馈

`semantic_plan/proof_batch` 对本批每个 `ValidationFailureRepair` CWI 必须携带一条 typed
`RuntimeSemanticExecutionFeedback`；普通 proof 和 coverage ordering 的这个专用字段不接收历史失败。Orchestrator 从该 CWI
的持久化 reopen `TriggerRef` 定位 rejected evidence，再反证原 binding、sealed dispatch、committed execution 和
owner receipt 的 exact identity/hash；不能使用“最近一次失败”、当前 mutable failure 或签名搜索代替因果来源。
合同的 `FailedFocusID` 是原 Definition declaration 身份，不是 retired runtime focus 名称。

反馈绑定 workspace/run、Definition/startup、manifest、CWI、失败、operation 和 receipt，只在 C4 渲染。
共享的安全 validation projection 保留执行/通过/失败/跳过/不完整 check、typed assertion 与缺失/截断标志；
不携带 raw output、错误原文、operation parameters、next action 或 coverage credit。没有 typed observation 时显式
`validation: null`，不能由日志或 hash 猜测断言。单条 check inventory 受现有 failure-detail limit 约束，整个反馈集合
JSON 上限 64 KiB，同时受现有 provider ceiling 约束；超限或缺失必要来源须拒绝，不静默省略必要反馈。

LLM owner 在 provider 前校验 exact batch/scope/hash/结构；post-render integrity 再逐值检查 C4，裁剪、注入或改写均
失败。C0～C3 不因执行反馈变化而变化，原 Goal/DoD 与验收权限不变。同版本重启从原持久化事实重建相同反馈，
不新增状态/预算账户，不回填旧归档。该反馈是规划事实输入，不是新的修复或验证权限。

## What Enters Candidate Context

允许进入默认 candidate context 的：

- 当前 focus 与 required evidence / assessment 要求
- accepted / rejected evidence 摘要与 refs
- 当前 failure fact（结构化 class/reason/target，而非 raw dump）
- C4 保留 exact current failure signature 对应的 validation receipt，即使该 receipt 已落在 recent-history 窗口之外。
  显示 recipe/protocol、执行/通过/失败/跳过/未完整检查，以及有界 check/case/location/line/digest；缺失或截断必须通过
  `failureDetailsIncomplete`/`checksTruncated` 显式表达，不能猜测 assertion 的 expected/actual。comparison fingerprint 与
  occurrences 只描述同一 Run 的失败事实，不授权 retry。C3 repair projection 只保留 sealed authority refs/surface，不展开
  `CurrentFailure`、原始诊断或 rejected operation 参数；诊断 class、owner code、环境/命令与 exit/timeout facts 只在 C4。
- 可选 v2 failure detail 仅允许封闭的 `reportedAssertion` 与 opaque `comparisonHash`，不能透传原始 expected/actual 值；
  shape 是检查自报诊断，不是独立验收结论。v2 只有位置/hash 仍须标记 `failureDetailsIncomplete=true`，非法扩展直接省略并
  标记不完整；无扩展的历史请求保留原渲染字节。诊断变化不产生新 authority、permission 或 stable-prefix 变化。
- sealed manifest 命中的 target snapshot、stage activation 与 bounded supporting fact；完整 capability/admissibility/freshness
  inventory 只供 owner-local integrity/freshness 校验，不直接展开给 provider
- activation 中的 Soul / Skill / MCP / Memory **快照**（已在 bootstrap 封存）

不得默认注入：

- 完整历史 run dump / superseded lineage 正文
- 未声明 capability
- 本地绝对路径、runtime 内部存储路径、raw provider I/O 正文
- 禁止的 `executionGraph` / `llmDecision` / 完整 patch content 作为下一轮 prompt 主体

## Memory / Recall

- activation 快照中的有界 memory 进入 candidate 上下文
- activation同时封存bounded typed selection evidence：candidate/selected/excluded identity、逐项score/reason、type quota、
  retrieved hit与byte/token budget；selected identity必须与实际封存进activation的memory identity精确一致；
- recall **不得**替代 current-run accepted evidence 关闭 assessment
- Memory owner 经 EventHub 读写；Orchestrator 不直接持有 Memory repository
- selection evidence只证明context选择过程；SM-4不得从`MemoryRecallTrace`反向重建authority，duplicate owner记录必须是
  exact create-once replay。

## Prerequisite Before Context-Changing LLM

若 preflight 发现缺失资源，drive 先进入 `need_prerequisite`，**不**把“缺快照”伪装成
state-changing candidate 的 requiredOutput。

典型 prerequisite kinds：

- Manifest
- TargetSnapshot
- FailureTarget
- ValidationScratch
- ProviderCeiling
- Capability（按 owner module 合并，不得互相覆盖）

## Request Audit（最小）

每次 candidate request 应可追溯：

- `StablePrefixHash`
- definition / evidence revision
- authority snapshot id
- candidate attempt index
- bounded correction observation/input identity（attempt 1）
- included context 的 scope（focus id、capability classes 等）

详细 section hash 字段见 core cache note；本 flow 要求各 section hash 与完整 typed
`AuthorityTransaction` **分别可审计**，不引入或暗示一份 synthetic transaction hash。

## Forbidden

- 用 `ExecutionContext` / patch / `map[string]any` 重建 authority 或 definition
- 把 profile ID / 软件开发术语写进 kernel reducer 选择逻辑
- 在缺失 prerequisite 时要求 LLM 直接 state-changing
- 第三次 candidate attempt 或 accepted 后同 snapshot 无 dispatch 重试
- block adapter 内部 map 重新进入 EventHub command/result、RuntimeFacts、candidate、dispatch 或 evidence

## Key Flow Guarantees

- candidate context 只来自 persisted facts + sealed definition，不来自 禁止的 map rebuild
- authority transaction 由 request session 封存；result 只能以 canonical request identity 回绑，不能携带另一份 transaction authority
- C0–C5 分层保证稳定 prefix 与易变尾部分离
- 缺失资源走 prerequisite，不走伪造 LLM state-change
- attempt 1 复用 stable prefix；第二次失败后离开 candidate 重试预算

## Source Documents

- [Runtime Flow Overview](runtime/flows/overview.md)
- [Planning And Execution](runtime/flows/planning-execution.md)
- [Activation And Definition](definition-convergence/activation-flow.md)
- `../00-core/runtime-request-cache-note.md`
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_candidate.go`、`runtime_activation.go`）

## Validation preparation observations

`runtime/validation-generation-context/v1` is an optional, capability-owner selected project observation on a
`RuntimeValidationSurface`. Its opaque task-specific contract, owner catalog revision, immutable completeness outcome and exact UTF-8 files
are preserved in the catalog snapshot identity. Core only checks scope, identity, content hashes and capacity; it does not
select languages, project paths or assertions. The envelope is at most 64 files / 256 KiB. `unavailable` and `over_capacity`
carry no partial file collection. `complete` describes the owner's declared selection, not all project resources or proof
that implementation is correct; an empty selection never proves a requirement.

The observation is shared by all matching preparations of one surface, rather than repeated for every native check. C3
projects it only for the selected proof batch or current prospective preparation, together with exact owner, catalog,
recipe and requirement identities. Per-check source obligation text/declaration comes from the original requirement;
published preparation extensions must match that requirement. Task-specific content instructions remain owner-authored.
Both semantic and operations integrity gates compare the projected context to the original typed facts before submission.

These are generation inputs, not mutation grants, accepted evidence, input consistency verdicts or a second task graph.
They do not turn mutable implementation into immutable review input, widen the sealed manifest, authorize replacing an
existing validation file or enable a new work/repair policy. Independent review, owner feasibility, current execution
dependencies and final all-of evidence remain mandatory. Context changes stay in C3; C0-C2 do not absorb project observations.
