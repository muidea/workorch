# WorkOrch Codex Request Context Comparison

Status: active

Type: audit

Last Updated: 2026-05-29

## Purpose

本文件保留对 Codex request/response logs 的对标核查结论，用于支撑 WorkOrch 的 context、evidence 和 repair-input 收口，但不作为第二份 runtime contract。

## Main Findings

Codex 长流程请求体现出以下模式：

- request 是 layered context，而不是 full runtime snapshot
- stable instructions、user goals、assistant progress、tool calls、tool outputs、patch calls 分层组织
- raw streaming response 单独归档，不直接塞进主 request body
- tool results 和 patch operations 保持结构化、bounded、可追溯

## WorkOrch Deviation Summary

在核对样本中暴露出的主要偏差：

- repair convergence 弱，invalid repair outputs 会放大循环
- context compression 与历史注入混在一起，关键 patch evidence 被削弱而低价值历史仍进入 prompt
- repair input 不总是以当前 failure target 为中心
- memory recall 会混入过多 process trace
- 大型 runtime JSON map 会带来 CPU、内存和读模型压力

## Absorbed Neutral Lessons

这些结论已按中立术语吸收到 runtime-flow / core 合同中：

- stage-specific context projection
- deterministic repair packet
- layered context budget
- bounded tool output summary
- patch evidence 与 target snapshot 分离
- memory recall 只保留 decision-useful signal
- raw stream / large payload 默认 ref-only

## Remaining Audit Value

本文件继续保留以下审计价值：

- 作为 context layering 和 repair packet 设计的对标证据
- 作为 “raw data by reference, bounded evidence in prompt” 原则的历史依据
- 作为未来继续核对 provider-facing projection 是否失控的参考

## Source Documents

- `../../20-runtime-flow/context-assembly-flow.md`
- `../../00-core/runtime-stage-contract.md`
