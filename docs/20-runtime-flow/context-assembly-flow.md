# Context Assembly Flow

Status: active

Type: runtime-flow

Last Updated: 2026-07-29

## Purpose

本文件定义 Runtime V2 为 candidate 请求组装上下文的链路：  
**typed `RuntimeCandidateContext` + provider-facing C0–C5 stable prefix**。

生产主线**不**从 V1 execution-context map、临时 JSON bridge 或 full runtime dump 重建 authority。

## Flow

```text
persisted RuntimeFacts + RuntimeDefinitionContext
  -> RuntimeCandidateContextFromFacts
  -> seal with AuthorityTransaction / Decision
  -> RequestRuntimeCandidateCommand
       DefinitionContext
       CandidateContext
       StablePrefixHash
       CandidateAttempt + optional PreviousFailure
  -> LLM block renders provider-neutral sections (C0–C5)
  -> provider adapter maps cache plan (adapter-only private fields)
  -> candidate response must echo same transaction
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

- 从 V1 map 或临时上下文重建 authority
- 在 candidate 响应中改写 transaction
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

## C0–C5 Stable Prefix

provider-facing 请求按 cacheability 分层（细则见 `../00-core/runtime-request-cache-note.md`）：

| 层 | 内容 | 稳定性 |
| --- | --- | --- |
| **C0** | stable runtime prefix（身份、中立 runtime 规则 / schema） | 最高 / 长寿命 |
| **C1** | workspace definition（Goal/DoD 正文等 definition 快照） | 随 definition revision |
| **C2** | capability contract / governance 声明 | 声明式稳定 |
| **C3** | current stage / focus bundle（required output、focus、action） | 随 decision 变 |
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

core 只定义 provider-neutral 分区与审计；provider 私有 cache-control 仅 adapter 层设置。

## What Enters Candidate Context

允许进入默认 candidate context 的：

- 当前 focus 与 required evidence / assessment 要求
- accepted / rejected evidence 摘要与 refs
- 当前 failure fact（结构化 class/reason/target，而非 raw dump）
- 已获取的 manifest / target snapshot / capability / ceiling 事实
- activation 中的 Soul / Skill / MCP / Memory **快照**（已在 bootstrap 封存）

不得默认注入：

- 完整历史 run dump / superseded lineage 正文
- 未声明 capability
- 本地绝对路径、runtime 内部存储路径、raw provider I/O 正文
- V1 `executionGraph` / `llmDecision` / 完整 patch content 作为下一轮 prompt 主体

## Memory / Recall

- activation 快照中的有界 memory 进入 candidate 上下文
- recall **不得**替代 current-run accepted evidence 关闭 assessment
- Memory owner 经 EventHub 读写；Orchestrator 不直接持有 Memory repository

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
- previous failure（attempt 1）
- included context 的 scope（focus id、capability classes 等）

详细 section hash 字段见 core cache note；本 flow 只要求 **hash 与 transaction 可审计**。

## Forbidden

- 用 `ExecutionContext` / patch / `map[string]any` 重建 authority 或 definition
- 把 profile ID / 软件开发术语写进 kernel reducer 选择逻辑
- 在缺失 prerequisite 时要求 LLM 直接 state-changing
- 第三次 candidate attempt 或 accepted 后同 snapshot 无 dispatch 重试
- block adapter 内部 map 重新进入 EventHub command/result、RuntimeFacts、candidate、dispatch 或 evidence

## Key Flow Guarantees

- candidate context 只来自 persisted facts + sealed definition，不来自 V1 map rebuild
- authority transaction 在请求与响应之间不可漂移
- C0–C5 分层保证稳定 prefix 与易变尾部分离
- 缺失资源走 prerequisite，不走伪造 LLM state-change
- attempt 1 复用 stable prefix；第二次失败后离开 candidate 重试预算

## Source Documents

- [overview.md](overview.md)
- [planning-execution-flow.md](planning-execution-flow.md)
- [activation-definition-flow.md](activation-definition-flow.md)
- `../00-core/runtime-request-cache-note.md`
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_candidate.go`、`runtime_activation.go`）
