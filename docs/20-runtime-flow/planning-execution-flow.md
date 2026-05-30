# Planning And Execution Flow

Status: active

Type: runtime-flow

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 从 planning 到 execution 的主运行链路。

## Flow

```text
llm/workspace_execution
-> normalized LLM output
-> next step / executionGraph / governance gate / failed assessment
-> orchestrator dispatch
-> block runtime adapters execute
-> Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint evidence
-> next graph batch or follow-up LLM step
```

## Main Stages

### 1. Planning

planning 的职责是将 Goal / DoD 拆成可执行单元。

允许输出：

- plan
- implementation units
- executionGraph
- risk / blocker
- stage transition proposal

planning 不应直接冒充：

- 已完成的 accepted Result
- 已验证的 DoD satisfied

### 2. Execution

execution 的职责是：

- 产生真实状态变更，或
- 明确说明当前不能继续自动变更

execution graph、shell、patch、MCP、file/generic runtime action 都属于 execution
阶段的实际动作。

补充稳定链路要求：

- Workspace activation 的首个正式运行步骤应进入 `llm/workspace_execution`
- LLM 只产生结构化 execution decision；它不能直接写文件、执行 shell 或调用 MCP
- 只有进入 Step output 的 normalized LLM output 才具备业务意义
- 决策、审批、patch、tool call、validation、memory write 都必须继续经由 orchestrator 映射为正式运行链路

### 3. Graph Dispatch

当 LLM 输出 executionGraph 后：

- orchestrator 持有 graph 的调度权
- ready nodes 可并发执行
- block adapters 只执行单节点动作，不拥有 Run 生命周期
- graph leaf 结束后，应进入 follow-up、verification 或 acceptance，而不是无界扩图

graph / follow-up 补充要求：

- executionGraph 是 advisory plan，直到被验证并写入 Step output 后才成为正式调度输入
- graph leaf 完成后如仍要求 DoD assessment，应进入 follow-up LLM step，而不是让执行 block 直接判定业务成功
- graph exhausted 不能无限返回新的空转 continuation；应进入 verification / acceptance / repair / replan 分流

#### Graph Parallel Dispatch

graph 调度的稳定并发规则至少应保持：

- ready nodes 可以并发执行
- 同一依赖链上的节点必须保持拓扑顺序
- graph leaf 全部完成后，才能进入 follow-up / verification / acceptance 分流
- block adapter 只执行单节点动作，不持有 graph 全局生命周期

约束：

- 并发执行不等于并行关闭业务收口；closeout 仍回到 verification / acceptance
- graph 扩展上限、空转 continuation 和 wrapper failure 不得因为并发分发而被隐藏

### 4. Patch And State-Changing Execution

state-changing execution 在 planning/execution 主链中至少应区分：

| 动作 | 当前职责 |
| --- | --- |
| `patch/preview` | 只预演 diff / 风险 / approvalRequired / conflict，不落盘 |
| `patch/apply` | 正式落盘，并生成 patch record / evidence / event |
| `patch/revert` | 对已有 patch 或变更做受控回退，不等于任意重置工作区 |

稳定规则：

- `patch/preview` 不替代 `patch/apply`
- `patch/apply` 进入副作用前必须通过 target relevance、coverage target、bounded verification 等合同校验
- `patch/revert` 只属于受治理的恢复动作，不等于自由 `git reset` 或任意 destructive rollback

### 5. File / Git / Web Capability Participation

planning/execution 主链中，file / git / web capability 的稳定角色至少包括：

| 能力族 | 主链职责 |
| --- | --- |
| `file/*` / `patch/*` | 文件读取、变更预演、正式落盘 |
| `git/*` | 仓库状态观察、差异、最小提交闭环 |
| `web/search` / `web/fetch` | 受控来源发现与来源读取 |

约束：

- 这些能力都属于 execution 证据面，不直接表达 accepted business outcome
- file/git/web 的业务解释由 profile/capability 层细化，不下沉到本 flow
- 大正文、raw payload、大 diff、网页正文都优先通过摘要 + ref，而不是塞入默认主链输出

### 6. Incremental Artifact Output Flow

当 `need_state_change` 的目标预计无法稳定放入单次 `executionGraph` JSON 时，
planning/execution 主链必须允许切换到 `incrementalArtifact`。

该模式的职责是：

- 用小 envelope 承载 plan/chunk
- 让 runtime 持有 continuation cursor
- 在 assemble/apply 后回到普通 verification / acceptance 闭环

它不负责：

- 改写 `ConvergenceContract.State`
- 直接表达 accepted Result
- 绕过 evidence / validation / acceptance gate

稳定主链：

```text
planning/execution
-> output mode selector
-> artifact_plan
-> artifact_chunk*
-> assemble
-> apply
-> validation / follow-up
-> verification / acceptance
```

约束：

- `incrementalArtifact` 只是 `need_state_change` 的输出承载模式，不是新的 runtime stage
- plan/chunk 阶段不得直接 satisfied DoD / Goal / deliverable coverage
- assemble/apply 前不得把 chunk 正文当成正式业务证据
- 只有 apply/validation 产生的 evidence refs 才能进入 acceptance 候选集

#### Mode Selection Boundary

本 flow 层只承接 mode 切换后的运行链，不重写 selector 细节。

最小稳定语义：

- selector 只能在 `executionGraph` 与 `incrementalArtifact` 之间选择输出模式
- 切换到 `incrementalArtifact` 后，当前 LLM step 只允许返回该 mode 对应的单一结构
- 若 selector 回落到 `executionGraph`，当前 step input 中的 artifact continuation contract 必须被清理

selector 阈值、schema 和 request-side 细节见：

- `../00-core/runtime-stage-contract.md`
- `../30-module-architecture/llm-provider-assembly.md`

#### Artifact Context Read Phase

分片过程中若缺少 concrete target context，允许进入受控 `artifact_context` 读阶段。

稳定规则：

- 只允许 bounded read / list / command / fetch
- 该阶段只能补上下文，不能推进 chunk cursor
- `artifact_context` 输出必须是独立的只读 `executionGraph`
- 不得与 `artifactPlan`、`artifactChunk`、`dodAssessment` 混在同一响应

#### Assemble / Apply / Validation Routing

artifact chunk 全部通过合同校验后，主链必须进入：

1. assemble
2. apply
3. validation / follow-up

稳定要求：

- assemble 只生成 runtime internal payload，不直接生成 accepted evidence
- apply 才是正式 state-changing 落点
- validation 负责把 apply evidence 绑定到 `coverageTargets` / current work item scope
- 若 apply 失败、hash 不一致、sequence 缺失或 chunk 合同失效，必须进入 repair /
  retry / governance 分流，而不是静默丢弃 continuation

#### Continuation Resume Boundary

服务重启、provider retry、schema repair、repair-of-repair 或 follow-up 后，
若当前 run 存在未完成 continuation，planning/execution 主链必须先恢复 continuation 再继续。

恢复时至少应保留：

- `artifactContinuationId`
- 当前 phase / state
- expected sequence
- targetRef / artifactId
- accepted plan / chunk hashes / tail hash
- latest apply / validation evidence refs

恢复后：

- 继续 chunking，或
- 进入 assemble/apply/validation，或
- 转入 repair / governance

但不得把未完成 continuation 误投影为普通 graph follow-up。

## Key Flow Guarantees

- LLM 输出是结构化建议，只有进入 Step output 后才产生业务意义
- orchestrator 拥有状态迁移和证据映射
- block modules 只拥有外部动作执行
- patch preview / apply / revert 必须保持预演、落盘、回退三种职责分离
- file / git / web capability 参与 execution 主链，但不直接拥有 closeout 语义
- graph 并发只属于调度优化，不改变 verification / acceptance 的收口顺序
- execution 不得以无限只读 graph 代替真实 state-changing output
- `incrementalArtifact` 只能作为 state-changing 的输出承载，不得绕过 apply/validation/acceptance
- 缺失 LLM 配置或 provider 不可用必须清晰失败，不能静默 fallback 成已完成运行
- Soul / Skill / MCP / Memory 都应作为一等 execution context 输入，而不是拼接型 prompt helper

## Evidence Produced

planning / execution 主链会沉淀：

- Step
- ToolCall
- Patch
- ValidationResult
- RuntimeCheckpoint
- Memory write request

这些都是后续 verification / acceptance 的输入，而不是最终业务结果本体。

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `../30-module-architecture/llm-provider-assembly.md`
- `../30-module-architecture/storage-readmodel.md`
- `verification-acceptance-flow.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
