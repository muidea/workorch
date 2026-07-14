# WorkOrch Runtime Request Cache Note

Status: active

Type: core-note

Last Updated: 2026-07-14

## Purpose

本文件作为 core 从属 note，定义 provider-neutral 的 request cache-friendly 组织规则。  
Runtime V2 生产路径下，candidate 请求上下文来自 typed `RuntimeCandidateContext`（persisted facts + authority transaction），见 [context-assembly-flow.md](../20-runtime-flow/context-assembly-flow.md)。

## Runtime V2 Binding

- Orchestrator 在 candidate request 前计算并持久化 `StablePrefixHash`（与 authority transaction / definition / execution contract 对齐）。
- attempt 0 与 same-contract attempt 1 **必须**复用同一 stable prefix；transaction 漂移不得 seal。
- `stablePrefixHash` 意外漂移视为 request rendering bug，不是 provider 偶然行为。
- LLM block 不得从 V1 map 或临时上下文重建 authority；只消费 typed candidate command。

## Stable Rules

### Cache Before Provider Binding

- core 只定义缓存友好的分区和审计，不绑定任何 provider 私有 cache-control 字段
- provider / model / baseURL 等动态绑定不得进入稳定前缀
- provider 私有 `prompt_cache_key`、cache-control 或等价字段只能由 adapter 侧消费

### Stable Prefix Partitioning

request 应按 cacheability 从高到低组织：

- `C0` stable runtime prefix（中立 runtime 规则 / schema）
- `C1` semi-stable workspace definition（Goal / DoD）
- `C2` stable capability / governance / template stage-policy **声明**（语义输入，非 kernel 状态名）
- `C3` current focus / requiredOutput / decision bundle
- `C4` volatile execution tail（failure、recent evidence、attempt diagnostics）
- `C5` large refs / trace

稳定前缀必须保持：

- Goal / DoD 不被压缩
- capability contract 声明式稳定
- 当前失败、当前进展、latest validation、memory recall、requested action 位于易变尾部
- raw stream、full provider payload、large artifact 默认 ref-only

### Provider-Facing Stable Prefix

core 仍只输出 provider-neutral `PromptContext` 与 `ProviderCachePlan`，但 provider adapter
必须把连续的 `C0` / `C1` / `C2` section 渲染为 provider-facing 的第一段稳定 prefix。

稳定要求：

- 出站请求第一段必须只包含 `C0` stable runtime prefix、`C1` workspace definition、`C2` capability contract
- `C3` current focus bundle、`C4` volatile execution tail、`C5` large refs 必须在稳定 prefix 之后
- provider/model/baseURL、stepId 这类动态绑定只能进入动态 tail 或 request audit，不能进入稳定 prefix
- provider 私有 cache-control、prompt cache key 或等价字段只在 adapter 层设置
- adapter 应保留 section id heading，使 request-log 能对齐 `ProviderCachePlan.sectionHashes`

### Canonical Rendering

- section 顺序固定
- schema 内字段顺序稳定
- 无业务顺序语义的 refs / ids 采用 stable sort
- 有过程顺序语义的 event / timeline / validation / tool outputs 必须保留原顺序
- normalization 只能做无语义损失清理

### Request Audit Fields

每次 request 应可追溯：

- `stablePrefixHash`
- `workspaceDefinitionHash`
- `capabilityContractHash`
- `stageBundleHash` / focus-bundle hash（对应 C3 current focus / requiredOutput / decision bundle）
- `volatileTailHash`
- `definitionRevision`
- `definitionChangeImpact`
- included / normalized / compressed / omitted sections

审计字段名可保留历史 `stageBundleHash` 标识，但其语义是 **focus / requiredOutput bundle**，不是 V1 StagePacket / NextAction。

### Context Priority Tiers

provider-neutral 的请求组织还应遵守稳定上下文优先级：

| 层级 | 当前方向 |
| --- | --- |
| `P0 Contract` | runtime contract、schema、workspace/run identity、当前 Goal / DoD、当前 requiredOutput / 动作要求 |
| `P1 Current Failure` | 当前 failure class/signature、repairObjective、requiredOutput、diagnostics |
| `P2 Coverage Scope` | coverage target、coverage work item summary/current focus、evidence candidates |
| `P3 Current Evidence` | latest action/validation/artifact evidence 与 progress digest |
| `P4 Current Snapshots` | target snapshots / manifest / repair target context |
| `P5+` | recent attempts、historical context、raw archives |

稳定约束：

- `P0`~`P4` 属于当前决策必要上下文
- 若 budget / redaction / compaction 使 `P0`~`P4` 关键字段缺失，request audit 必须记录 integrity warning
- provider-facing request 可以压缩低优先级层，但不得静默牺牲高优先级层

## Module Landing Matrix

本 note 只保留 provider-neutral 规则；具体实现责任点应下沉到模块与 flow 文档。

| 规则族 | core note 负责 | 主要模块落点 | 主要 flow / verification 落点 |
| --- | --- | --- | --- |
| stable prefix partitioning | 定义 `C0`~`C5` 的中立分层 | `../30-module-architecture/llm-provider-assembly.md` | `../20-runtime-flow/context-assembly-flow.md` |
| canonical rendering | 定义稳定排序、字段顺序、无语义损失 normalization | `../30-module-architecture/llm-provider-assembly.md` | `../60-verification/core-runtime-verifier.md` |
| request audit fields | 定义 hash / included / omitted / redacted 摘要字段 | `../30-module-architecture/llm-provider-assembly.md` | `../60-verification/core-runtime-verifier.md` |
| integrity priority | 定义 P0-P4 保护字段优先级 | `../30-module-architecture/llm-provider-assembly.md` | `../20-runtime-flow/context-assembly-flow.md` / `../60-verification/core-runtime-verifier.md` |
| provider-specific cache mapping | 不在 core 中展开 provider 私有字段 | `../30-module-architecture/llm-provider-assembly.md` | provider-facing request verifier / request-log audit |

## Provider-Neutral Vs Provider-Specific Boundary

以下内容继续留在 core-note：

- cache-friendly section 分层原则
- 哪些信息属于 stable prefix、哪些属于 volatile tail
- request audit 至少要保留哪些 provider-neutral 元数据

以下内容必须下沉，不再在 core-note 里扩展：

- provider 私有 cache-control 字段名
- provider/model/baseURL 绑定逻辑
- request-log 文件布局、raw request/response/ref 命名
- revision cache、model capability table 与 provider config loader 的实现细节

## Verification Landing

当前该 note 的验证应分三层理解：

- core verifier
  - 检查 stable/volatile 分层、protected fields、request audit completeness
- context assembly flow
  - 检查 `RuntimeCandidateContext`、C0–C5 / current focus bundle、selected context、omitted/compressed 规则是否符合中立合同；不得从 V1 graph/NextAction map 重建
- provider assembly / request-log audit
  - 检查 provider-specific cache mapping、request-log ref、raw request rendering 是否符合模块实现约束

因此“provider-specific cache mapping”与“request-log verifier”不再视为 core-note 自身缺失定义，
而应视为模块层和 verifier 层的实现/验证落点。

request audit 在 verifier 中的最低核查点至少包括：

- `included` / `omitted` / `redacted sections`
- `protectedFields`
- `integrityWarnings`
- `source refs`
- 是否发生 Goal / DoD 压缩或 P0-P4 缺失

### Integrity Priority

缓存友好组织不能覆盖 context integrity：

- P0-P4 必要字段缺失时，不得因为稳定前缀复用而继续提交 provider
- Goal / DoD、current failure、target snapshot、coverage target 等保护字段优先于 cache hit

## Cross References

- context turn 组织与 prompt packing 见 `../20-runtime-flow/context-assembly-flow.md`
- provider-specific cache mapping 与 request-log verifier 见 `../30-module-architecture/llm-provider-assembly.md`
- core verifier 入口见 `../60-verification/core-runtime-verifier.md`

## Source Documents

- `../20-runtime-flow/context-assembly-flow.md`
- `runtime-stage-contract.md`
