# WorkOrch Runtime Flow Overview

Status: active

Type: runtime-flow

Last Updated: 2026-05-29

## Purpose

本目录用于定义 WorkOrch 的核心运行时链路，而不是对象定义或模块结构。

本层只回答：

- 一次 activation 如何形成 Run 和首个 Step 输入
- planning / execution 如何派发 graph、tool、patch、memory write
- verification / acceptance 如何基于 evidence 和 result 收口
- repair / replan / provider retry 如何继承 contract 和 failure surface
- LLM request 的 context bundle、request audit、cache-aware 结构如何组装

## Reading Order

1. [activation-definition-flow.md](activation-definition-flow.md)
2. [planning-execution-flow.md](planning-execution-flow.md)
3. [verification-acceptance-flow.md](verification-acceptance-flow.md)
4. [repair-replan-flow.md](repair-replan-flow.md)
5. [context-assembly-flow.md](context-assembly-flow.md)
6. [evidence-result-flow.md](evidence-result-flow.md)

## Cross-Layer Boundary

- 对象、边界、不变量属于 `10-domain-model/`
- 核心 stage contract、context integrity、ConvergenceContract 属于 `00-core/`
- 代码模块、落点和实现文件属于 `30-module-architecture/`
- software-dev / workspace template / MCP capability 特化属于 `40-profiles-capabilities/`

## Full-Chain Closure Themes

当前 runtime-flow 层还持续承接以下跨链路收口主题：

- executionGraph exhausted 后如何进入 validation / repair / replan，而不是直接无意义失败
- completion gate 如何区分 invalid continuation、evidence gap、state-change gap 和 governance blocker
- context assembly 如何避免把 superseded history、raw stream 和低价值过程噪声注入当前 repair prompt
- verification / acceptance 如何在 evidence quality、coverage 和 result 之间完成闭环
