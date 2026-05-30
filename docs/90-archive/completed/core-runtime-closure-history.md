# Core Runtime Closure History

Status: active

Type: archive

Last Updated: 2026-05-30

## Purpose

本文件承接 core runtime 合同相关的阶段性收口记录、实现清单与历史落地轨迹。

它不再作为当前 active core contract 的权威入口。

当前稳定定义入口：

- `../../00-core/runtime-stage-contract.md`
- `../../20-runtime-flow/context-assembly-flow.md`
- `../../20-runtime-flow/repair-replan-flow.md`
- `../../20-runtime-flow/verification-acceptance-flow.md`
- `../../30-module-architecture/llm-provider-assembly.md`
- `../../60-verification/core-runtime-verifier.md`

## Incremental Artifact Closure History

### Historical Acceptance Matrix

曾用于 `incrementalArtifact` 设计落地的中立验收场景包括：

| 场景 | 期望 |
| --- | --- |
| 小 patch | 仍走 `executionGraph` |
| 大 raw preview schema invalid | 切换到 `incrementalArtifact` |
| artifact plan 包含 executionGraph | 被拒绝 |
| chunk sequence 跳号 | 被拒绝并重试 expected sequence |
| chunk targetRef 改变 | 被拒绝 |
| chunk hash mismatch | 拒绝；同 sequence 多次不一致进入 chunk repair |
| 服务重启于 chunking | 从最后 confirmed sequence 后继续 |
| final chunk 收齐 | 进入 assemble |
| assemble 完成 | 进入 apply 并产生 state-changing evidenceRef |
| apply 失败 | 进入普通 state-changing repair，不回到 chunking |
| validation 失败 | 针对已落地 targetRef repair，不重新生成全量 artifact |
| chunk 事件 | event 不包含大段正文 |
| retention 禁止正文 | 仍保留 hash / bytes / evidenceRef |
| coverageTargets 缺失 | plan 或 apply 被拒绝 |
| LLM 请求切换模式但阈值不满足 | runtime 继续使用 `executionGraph` |

### Historical IA Checklist

`incrementalArtifact` 的阶段性落地清单当时包括：

- `IA-01` ~ `IA-40`
- 配置、模型、存储、selector、LLM schema、runtime、恢复、evidence、event、
  LLM context、Web/TUI、测试、e2e、文档与审计
- 当时所有项状态均已收口为 `Done`

该清单现已转为历史实现轨迹，不再作为当前 core contract 的活跃待办。

### IA-37 Closure Note

`IA-37` 的历史收口说明保留以下事实：

- `verify-incremental-artifact-e2e` 已覆盖 activation、mock LLM、artifact plan/chunk/apply、
  completion follow-up、event 与 snapshot metadata-only 校验
- 输出模式信号、artifact continuation 恢复、completion follow-up 回切、
  raw payload 隔离与 metadata-only 投影均已完成首轮收口
- 后续新增相关逻辑应继续通过正式 verifier / e2e 覆盖 response schema、阶段切换、
  raw payload 隔离与完成 follow-up 回切

## Abstract Matrix / Prompt-Memory Closure History

### Historical Closure Trackers

以下跟踪表已完成历史收口，当前只保留追溯价值：

- `SNC-01` ~ `SNC-05`
  - 抽象矩阵收口跟踪
- `STG-10-01` ~ `STG-10-05`
  - Prompt / Memory 收口跟踪

当前稳定规则已迁入：

- `../../00-core/runtime-stage-contract.md`
- `../../20-runtime-flow/context-assembly-flow.md`
- `../../30-module-architecture/llm-provider-assembly.md`

## Runtime Closure Timeline

以下日期记录保留为历史落地轨迹：

- 2026-05-13
  - STG-01 / STG-02 / STG-03 / STG-06 / STG-08 第一批收口
- 2026-05-14
  - repair failure scope、target snapshot、patch repair context、replan continuation 收口
- 2026-05-15
  - execution contract 下发与 template contract 刷新收口
- 2026-05-17
  - 运行态上下文放大收口、DoD/Goal coverage 缺口提示中立化收口
- 2026-05-19
  - graph expansion limit assessment 与 evidence 优先级收口
- 2026-05-20
  - context gate / provider request 准入收口，Codex 启发的中立过程证据收口
- 2026-05-22 ~ 2026-05-27
  - `CVC-01` 多批收口、capability policy repair 收口、`CWI-01` 设计固化

更细的逐批细节以当期变更记录和对应 verifier / archive 文档为准，不再在 core contract 中保留长篇落地日志。

## Archive Boundary

本文件只承接：

- 已完成 core runtime 设计收口的历史实现轨迹
- 历史验收矩阵与阶段性清单
- STG / CVC / CWI 的 dated closure 说明

本文件不承接：

- 当前 active core contract
- 当前 active flow / module / verification 定义
- 当前 active roadmap 待办
