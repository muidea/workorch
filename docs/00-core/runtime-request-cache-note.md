# WorkOrch Runtime Request And Cache Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-09-01

## Purpose

本文件是 LLM request、完整文档、context integrity 与 cache stability 的稳定入口。详细规则拆为两个同时生效的
focused contract；本页不复制字段级正文。

## Contract Set

| Topic | Contract | Scope |
| --- | --- | --- |
| request transaction | [Runtime Request Provider Contract](runtime-request-provider-contract.md) | `DocumentKind`、provider ceiling、ExpectedDocument、whole-document recovery、audit 与 context integrity |
| cache organization | [Runtime Cache Partitioning](runtime-cache-partitioning.md) | C0～C5、stable prefix、canonical rendering 与 adapter cache boundary |
| assembly flow | [Context Assembly Flow](runtime-request-context-assembly.md) | typed context 如何从 persisted facts 和 sealed authority 组装 |
| Definition partition | [Definition Context Strategy](definition-convergence/context-strategy.md) | exact C1 partition、C5 expansion 与大 Definition 预算 |

## Global Invariants

- `DocumentKind` 必须显式为 `semantic_plan | operations`；两类字段互斥，但复用同一 request session、完整文档恢复、cache plan 与 audit。
- provider-facing token 值只有 LLM owner 的 sealed `ProviderCeiling` 一个权威；submission 前任何不一致均零 physical call。
- attempt 0 与 same-contract attempt 1 复用同一 C0～C3、authority transaction、manifest 与 `stablePrefixHash`。
- operations C3 的 output contract 必须携带从 sealed manifest 纯编译的 exact `wireContractHash`、canonical
  wire layout 和 per-slot rules。attempt 1/restart 只能重算同一 hash；wire 规则漂移属于请求身份缺陷，不是
  provider correction input，且不得通过改写 C4 绕过。
- operations attempt 1 的 C4 必须携带 attempt 0 persisted directive 的 provider-safe detached correction input：manifest、
  objectives 与 concrete document-shape/schema/semantic witness；failure 与该 input 的 normalized identity 进入 correction/request-session hash，
  persisted `Disposition`/`TerminalCode` 不跨 owner。LLM 只渲染该 detached fact，不能重新选择 alternative。缺失、漂移或
  无法由相同 manifest 重验的 witness 在 physical provider call 前 fail-closed。
- `owner_candidate_constraint` correction 的 C3 authority 保留 manifest-declared `candidateConstraints`；C4 只携带 exact
  constraint ref、parameter path、closed `actual_shape`/`required_shape` 与 objective，不得携带 rejected parameter、owner
  parser text 或普通 feasibility diagnostic。attempt 1 仍复用同一 C0～C3 与 `stablePrefixHash`；required shape 只能由
  capability owner 的后继 typed conformance result 证明。
- required field missing/empty 的 schema witness 使用 `field_absent -> field_present`；required field 为 `null` 或错误 JSON type
  时，C4 必须携带 strict decoder 的 exact rejected type、同 path 与 expected schema rule，并使用
  `<actual_type> -> field_present`。两者都只要求重写完整 document，不改变 C0～C3、wire contract、stable-prefix hash 或
  candidate attempt budget，也不授权 decoder/LLM owner 修复原文。
- C0～C2 是连续稳定前缀；C3～C5 是 current/volatile tail。意外 stable-prefix 漂移属于 rendering defect。
- exact Goal/DoD partition、current authority 与 P0～P4 integrity 优先于 cache hit；缺失保护字段必须 fail-closed。
- raw prompt/body/stream、credential、endpoint 和未脱敏内容不得进入 aggregate、跨 owner diagnostic 或下一次 request。
- LLM owner result 只允许 `Stale | Feedback | DocumentOutcome` 三者恰好一个；physical response mode 只允许
  `streaming | non_streaming`，旧 `streaming` 布尔值只能由该 owner fact 派生，不能成为第二份事实。
- provider physical call的allocation/usage/unknown consumption遵守
  [Execution Budget](runtime/execution-budget.md)；cache hit、whole-document recovery与candidate attempt不能互相借用预算。

## Ownership

Orchestrator 封存 expected-document、context identity、section hash 与 request-session facts；LLM block 解释已选
provider/model ceiling并执行 provider transaction；adapter 只映射 provider 私有 cache 字段。Application、Web、TUI、
profile 和 capability owner 均不得维护第二份 token ceiling、stable prefix 或 request authority。

具体实现落点见 [LLM Block implementation contract](../../internal/modules/blocks/llm/readme.md)，验证入口见
[Core Runtime Verification](runtime/verification.md)。
