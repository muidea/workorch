# Runtime Cache Partitioning Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-27

## Provider-Neutral Cache Model

Core 只定义缓存友好的 section、hash 与顺序，不绑定 provider 私有 cache-control 字段。provider/model/baseURL、
physical call identity 和 adapter options 不能进入稳定前缀；adapter 可以从 sealed `ProviderCachePlan` 映射私有字段，
但不能改变 section 内容、顺序或 hash。

## C0-C5 Partition

| Section | Content | Stability |
| --- | --- | --- |
| C0 | 中立 Runtime rules、schema 与 owner-neutral contract | long-lived stable |
| C1 | immutable `DefinitionPartitionEnvelope` 与 current partition exact Goal/DoD members | Definition-revision stable |
| C2 | declarative capability/governance/validation/acceptance-slice/template-profile policy | activation stable |
| C3 | current focus、required output、decision bundle、sealed manifest/current owner catalog | authority-current |
| C4 | provider-safe current failure、attempt diagnostic、recent evidence identity、detached correction input | volatile |
| C5 | bounded large refs、trace 与 cross-partition expansion | ref-oriented volatile |

Provider-facing 第一段必须是连续的 C0→C1→C2。C3→C4→C5 只能出现在其后。canonical Goal/DoD 在 owner/aggregate
完整保留，C1 exact member 正文不得摘要；raw stream、full provider payload 与 large artifact 默认 ref-only。

## Operations Request Rules

- C3 operation kind、variant、parameter、effect、coverage 与 target policy 只从 persisted
  `RuntimeCandidateContractManifest` 渲染，不能从 profile、owner inventory 或错误文本拼第二份合同；
- active rejected shape、failure lineage、current owner readiness 与 admissible operation set 属于 C3/C4，不得回写 C2；
- C4 correction 只包含 provider-safe rejection 与 Orchestrator persisted `CorrectionDirective` 的 detached
  `correctionInput` 投影。`correctable_once` 必须投影 manifest、objectives、concrete document-shape/schema/semantic
  witness 和 exact alternative；correction/request-session identity 只绑定 failure 与该 detached input，不携带 persisted
  `Disposition`/`TerminalCode`。C4 不得包含 owner raw diagnostic、Run/repair/recovery action、governance continuation、raw
  evidence summary 或未封存 allowed value；
- external capability receipt 只把 owner-normalized bounded summary/hash放入C4 evidence tail；raw payload只能作为
  opaque C5 ref，不能展开provider body、transport endpoint或diagnostic；
- operation kind、variant 与 target policy 必须来自同一 owner fact；具体 owner/kind/target 只存在于 C3 manifest。

attempt 0 与 same-contract attempt 1 必须复用相同 C0～C3 section hash、manifest、authority transaction 与
`StablePrefixHash`。attempt 1 只能改变 C4，并在 provider call 前从 persisted attempt 0 重新验证相同 manifest/witness、
再投影唯一 detached input；
任何要求改变 C0～C3 的修正都不是 same-contract correction。

## Semantic-Plan Request Rules

semantic-plan 复用同一 section 顺序：

- C0 保存中立 semantic schema/rules；
- C1 保存 current immutable partition 的完整 exact members；
- C2 保存 immutable execution contract、DefinitionFocus declaration 与 profile-declared owner-fact requirement；
- C3 保存 Definition seed、sealed semantic manifest/schema、current owner admissibility snapshot/revision、activation已封存的
  planning Soul guidance以及bounded target/selector/constraint/recipe vocabulary；Soul只提供context，不能新增或裁剪manifest authority；
- C4 保存 attempt 0/1 与 provider-safe rejection；C5 保存 acceptance-slice/cross-partition refs。

上一 generation 的 operation、target、binding 和 owner readiness 只能进入 volatile lineage，不能回写 C2。
`RuntimeOwnerFactRequirement` 在 C2 只描述 owner、fact kind 和 operation kind；current readiness、recipe identity、
input binding 与 proof applicability 只能来自 C3 current catalog。catalog revision 变化必须产生新 manifest 和 logical
request，不能 continuation 旧 complete document。

semantic owner 的 `stale|prerequisite_required` original diagnostic 只进入 aggregate audit，不进入 C4。只有带 exact
revision fence 的 stale 可触发 SM-2 有界重获；当前无 owner-defined acquisition contract 的 prerequisite-required
必须保留事实并 fail-closed，不能伪装为 refresh。generation history 与完整 owner diagnostic 不进入 C0～C5。

`validation_failure_repair` 的 failed-focus identity、stable failure signature、closed trigger diagnostic class、Profile-sealed
convergence 与 exact repair vocabulary 属于 C3。它不得进入 C2，也不得携带 raw diagnostic、receipt、Runtime action、retry
或 terminal disposition。专用 provider instruction 只在 current proof batch 含 repair obligation 时出现；相同 Definition
下普通 generation 与 repair generation 的 C0～C2 bytes/hash、`StablePrefixHash` 必须一致。

## Canonical Rendering

- section 顺序与 section ID heading 固定；
- schema 字段顺序稳定；无业务顺序的 refs/IDs stable sort；
- event/timeline/validation/tool output 等具有过程语义的集合保留原顺序；
- normalization 只能做无语义损失清理；
- renderer、schema、decoder、evaluator 与 request audit 必须对同一 vocabulary/shape/hash 达成一致。

`ProviderCachePlan.sectionHashes` 必须能与实际 provider-facing section 逐项对齐。`StablePrefixHash` 意外漂移、
动态 binding 渗入 C0～C2、protected content 压缩或 section hash 与 rendered bytes 不一致均是 request rendering defect，
不能归因于 provider cache miss。

## Adapter Boundary

Core 保留 section 分层、stable/volatile boundary、canonical rendering 和 provider-neutral audit。provider 私有
cache-control/key、request-log 文件布局、raw ref、model capability/config loader 与实际 wire mapping 只属于
[LLM Block implementation contract](../../internal/modules/blocks/llm/readme.md)。adapter 不得以 provider 特性重排 C0～C5、
改变 stable prefix 或覆盖 context-integrity gate。
