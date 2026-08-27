# WorkOrch Runtime Request And Cache Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-08-27

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
- operations attempt 1 的 C4 必须携带 attempt 0 persisted directive 的 provider-safe detached correction input：manifest、
  objectives 与 concrete document-shape/schema/semantic witness；failure 与该 input 的 normalized identity 进入 correction/request-session hash，
  persisted `Disposition`/`TerminalCode` 不跨 owner。LLM 只渲染该 detached fact，不能重新选择 alternative。缺失、漂移或
  无法由相同 manifest 重验的 witness 在 physical provider call 前 fail-closed。
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
