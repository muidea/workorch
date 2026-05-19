# WorkOrch Result Evidence Strictness Closure Design

本文用于收口 `ws-3cae27602d8b7fe6` 暴露出的结果验收问题：workspace 能进入 `accepted`，但最终 DoD assessment 的证据过粗，无法严格证明每个 Goal / DoD 条目都由直接、可追踪、命中交付入口的证据支撑。

本文是 `workorch-runtime-stage-contract-neutrality-design.md` 的补充合约。后续修复必须保持 WorkOrch core 功能中立：core 只校验证据结构、证据类型、证据可追踪性和 profile 声明的机械约束；软件开发场景如何定义“入口”“测试”“构建”“用户可观察行为”，只能由 `workspaceTemplate` / profile / Soul / skill / Goal / DoD 表达。

## 问题结论

本轮核对显示：

- 运行流程可以进入终态 `completed`，且 result `State=accepted`。
- 交付目录中的测试与构建可以通过。
- 但 accepted result 中多个 DoD item 复用同一组粗粒度证据，例如 coverage matrix、race、build。
- 对流式、错误透传、本地副作用、配置、并发、最终交付入口等要求，粗粒度证据不足以证明逐项满足。

因此需要补强的是“结果证据严格性”，而不是把某个软件开发目标写死进 core runtime。

## 核心框架中立收口

### 1. Item 级直接证据

`dodAssessment.items[]`、`goalCoverage.items[]`、`acceptanceDimensions.items[]`、`deliverableCoverage.items[]` 中每个 `satisfied` 条目必须引用可追踪 evidence。

当 profile 通过 `evidenceRequirements[]` 声明 `requireDirectEvidence=true` 时，core 必须机械校验：

- 该条目至少有一个 evidenceRef 指向已知 runtime evidence。
- 若声明 `acceptedStepKinds`，至少一个 evidenceRef 的 step kind 命中 accepted 集合。
- 若只命中 `rejectedStepKinds`，该条目不能 accepted。
- 若 evidenceRef 不可追踪，不能作为 direct evidence。

Core 只能理解 step kind 和 evidence ref，不得理解测试框架、文件路径、接口协议或业务语义。

### 2. 主链路 / 入口证据

当 profile 通过 `evidenceRequirements[]` 对某个 acceptance dimension 声明 `requiresEntrypointEvidence=true` 时，core 只要求该 dimension 的 satisfied item 引用符合要求的 runtime action evidence。具体什么是“入口验证”由 profile/Soul 指导 LLM 在 evidence summary、验证命令或 coverage item 中表达，core 不解析业务内容。

### 3. 用户可观察行为证据

流式、长流程、错误行为、副作用、并发等用户可观察行为不能仅由 read-only evidence 或宽泛 build evidence 单独支撑。profile 应以 `acceptedStepKinds` / `rejectedStepKinds` 进行约束；core 只做机械校验。

### 4. Template/Profile 显式绑定

Workspace 创建时必须显式持久化：

- `TemplateID`
- `ProfileID`

Run activation 组装 `executionContract` 时优先按 `TemplateID` 读取当前 template 的 `ContextHints`。仅当旧 workspace 缺少 `TemplateID` 时，才允许用 Soul routing 进行兜底匹配。新 workspace 不再依赖 SoulBindings 反推 template contract。

## software-dev template 收口

`software-dev` template 需要增强通用工程约束，但不得绑定具体 Goal / DoD。

新增或强化：

- 默认 DoD 要求最终交付入口验证。
- 默认 DoD 要求用户可观察行为必须有直接验证或明确跳过说明。
- 默认 DoD 要求高风险维度的测试绑定真实交付入口或生产实现；测试内临时 fake/mock/helper 只能作为依赖替身或辅助，不能作为唯一 satisfied 证据。
- `runtime_entrypoint`、`streaming_or_long_running`、`error_behavior`、`persistence_or_side_effect`、`concurrency` 等 evidence requirements 设置 `requireDirectEvidence=true`。
- Soul 要求 validation 输出逐项 `DoD item -> acceptance dimension -> exact evidenceRefs`，不能用总体 build/race/coverage_map 替代所有条目。

## 收口跟踪

| ID | 内容 | 落点 | 状态 | 验收口径 |
| --- | --- | --- | --- | --- |
| RES-01 | 输出结果证据严格性设计文档 | docs | completed | 本文存在并被核心合约引用 |
| RES-02 | Workspace 持久化 `TemplateID/ProfileID` | workspace model / appservice / backend | completed | 创建 template workspace 后 workspace 记录与 backend DTO 可看到 template/profile |
| RES-03 | Activation 优先按 `TemplateID` 刷新 template contract | appservice activation | completed | `workspaceExecutionContract` 优先按 `TemplateID` 读取 template，Soul routing 仅作旧 workspace 兜底 |
| RES-04 | `evidenceRequirements.requireDirectEvidence` 机械校验 | orchestrator evidence quality gate | completed | 单测覆盖不可追踪 direct evidence 不能支撑 satisfied |
| RES-05 | software-dev template 补强最终入口、用户可观察行为和生产实现绑定约束 | builtin templates / souls | completed | 默认 DoD、profile requirement 与 Soul 已明确最终入口、用户可观察行为、高风险维度直接证据，以及 fake/mock/helper 不能作为唯一被测对象 |
| RES-06 | deployed e2e 复验复杂 Goal/DoD accepted result | scripts / deployed verification | completed | `verify-software-dev-goal-dod-e2e.sh` 已在交付型 scenario 后自动调用 `verify-workorch-runtime-profile-software-dev.sh --snapshot`，并通过 raw result payload 补齐 DoD assessment 后校验 direct evidenceRefs 可追踪且命中 profile 允许 step kind；重新部署后执行 e2e 即可复验真实服务 |
