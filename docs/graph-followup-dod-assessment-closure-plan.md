# Graph Follow-up 与 DoD Assessment 收口计划

## 背景

历史排障样本 `ws-c4b6c80ed677c443` 的最新回归暴露出一类通用执行链路问题：

- Workspace 已能在 `replan/superseded` 后继续创建新的 continuation Run。
- `file/read optional:true` 已能让缺失的可选文件不再直接卡死运行。
- 但 after-graph / repair follow-up 中，LLM 仍持续返回新的 `executionGraph`。
- 当 graph follow-up 超过扩展上限后，系统进入 repair；repair 又可能继续要求扩图，形成“读文件/跑测试/扩图/超限/repair/replan”的循环。
- 对于已经有足够执行证据的 Run，系统没有稳定强制进入 DoD assessment / Result / Artifact 收口。

该问题不是软件开发专属问题。核心框架应保持中立，只提供 graph follow-up 边界、assessment-only 模式、DoD 缺口驱动 repair/replan 等通用能力；软件开发场景的“新增 Goal 要落实到代码修改和测试”应沉淀到 `software-dev` workspaceTemplate / Soul。

## 历史排障样本

样本 Workspace：`ws-c4b6c80ed677c443`，仅作为问题分析来源，不作为 e2e fixture 或正式验收数据。

最新观察到的现象：

- 最新活跃 Run：`run-c2d3f13c9c4fb18d`
- Workspace 状态：`running`
- 无待处理 Decision / Approval
- 最新正式 Result 仍是旧 Run 的 `rejected` Result
- 新 Run 尚未产出正式 Result / Artifact
- failure signature 多次出现：
  - `graph follow-up expansion limit exceeded after 2 attempts`
  - 缺失 `sample_config.yaml` 被 repair 为 optional read 后仍继续扩图

样本中的业务缺口包括：

- 新增 Goal“支持同时代理多个 llm”没有稳定转化为实现改动。
- 当前代码仍主要是单 provider 配置结构。
- `main()` 与 `gatewayHandler()` 存在重复转发逻辑。
- `config.yaml` 已存在，但代码未稳定证明其解析和生效路径。

这些业务缺口只用于验证 `software-dev` 模板效果，不应写入 WorkOrch kernel 的通用规则。

## 目标

目标是建立一个稳定的通用收口链路：

```text
Goal/DoD 累计更新
→ 新 Run 激活
→ 读取现状和执行必要步骤
→ graph follow-up 收口已有证据
→ 达到扩展边界后进入 assessment-only
→ 输出 DoD assessment
→ 基于 DoD 缺口进入受控 repair/replan
→ 最终 Result/Artifact 或明确 rejected result
```

禁止继续出现以下循环：

```text
读取文件
→ 跑测试
→ after-graph 继续扩图
→ graph expansion limit exceeded
→ repair
→ repair 继续扩图
→ replan
→ 新 Run 重复同类路径
```

## 边界原则

- WorkOrch kernel 只实现通用机制，不写入 Go、config.yaml、多 LLM、源码、测试框架等具体场景规则。
- LLM 通用 contract 只描述 graph follow-up、assessment-only、DoD 缺口、runtime evidence 的通用输出规则。
- 软件开发策略放入 `software-dev` Soul：
  - 新增 Goal 后必须对比现状与新增目标。
  - 发现功能缺口时优先实施代码修改和补测试。
  - 重复读文件或重复跑测试不能替代实现。
  - 验证通过但新增 Goal 未覆盖时，不得输出 satisfied。
- e2e 可以使用软件开发样本验证模板效果，但 kernel 单测必须保持领域中立。

## 设计方案

### 1. Graph Follow-up 超限后进入 Assessment-only

当 Run 处于 graph/completion/no-actionable follow-up，且 follow-up 已达到扩展上限时：

- 不再允许 LLM 返回新的 `executionGraph`。
- 调度一个 assessment-only follow-up。
- assessment-only 输入只保留必要证据摘要、当前 Goal/DoD、已完成节点、失败签名和缺口上下文。
- assessment-only 输出只能是：
  - `completed + dodAssessment`
  - `failed/rejected + dodAssessment + missingDoDItems + nextActions`
  - `blocked_on_decision`
  - `blocked_on_approval`

如果 LLM 在 assessment-only 模式仍返回 `executionGraph`：

- Core 应将其视为 contract violation。
- 不继续执行该 graph。
- 记录结构化诊断，并进入受控 DoD-gap repair 或失败交付。

### 2. DoD 缺口驱动 Repair/Replan

Runtime repair 不应继续被自由 graph expansion 驱动，而应优先由 DoD assessment 结果驱动。

当 `dodAssessment.satisfied=false` 或存在 `missingDoDItems`：

- 将缺口规范化为通用 `missingDoDItems`。
- 每个缺口应包含：
  - `item`
  - `state`
  - `evidenceRefs`
  - `reason`
  - `impact`
  - `suggestedNextActions`
- 后续 repair/replan 输入只围绕这些缺口展开。
- 如果缺口不能自动处理，再创建 Decision；否则继续自动执行。

### 3. Completion Follow-up Contract

增强通用 LLM contract：

- graph follow-up 是为了收口当前 execution graph 的证据，不是无限追加计划。
- 已有验证证据时，优先输出 DoD assessment。
- 达到 graph expansion limit 后必须 assessment-only。
- assessment-only 模式禁止返回 executionGraph。
- 需要后续动作时，通过 `dodAssessment.items`、`missingDoDItems`、`nextActions` 表达，由 runtime 决定是否开启新 repair/replan。

### 4. Runtime 可观测性

Run Snapshot / Observability 需要展示：

- graph follow-up 扩展次数。
- 是否进入 assessment-only。
- assessment-only 是否拒绝了 executionGraph。
- DoD 缺口列表。
- 缺口驱动 repair/replan 的来源 assessment step。
- 最终 Result / Artifact 是否来自最新 active Run。

### 5. Software-dev Soul 增强

`software-dev` Soul 需要补充：

- 新增 Goal 后先进行 gap analysis，列出当前代码已满足项和未满足项。
- 如果新增 Goal 要求能力变化，不能只跑已有测试；必须实施相关代码和测试变更。
- 如果验证通过但未覆盖新增 Goal，应返回 failed/rejected DoD assessment，并列出缺口。
- 对重复读文件、重复验证命令进行收敛，避免用检查替代实现。

## 任务清单

| ID | 任务 | 类型 | 优先级 | 状态 | 收口标准 |
| --- | --- | --- | --- | --- | --- |
| GFA-01 | 梳理现有 graph follow-up 扩展计数与限制逻辑 | Orchestrator | P0 | implemented | 明确当前 expansion limit、repair、provider retry 的入口和状态字段 |
| GFA-02 | 增加 graph follow-up 超限后的 assessment-only 调度 | Orchestrator | P0 | implemented | 超限后不再直接进入自由 repair graph，先强制 DoD assessment |
| GFA-03 | assessment-only 禁止 executionGraph | Orchestrator/LLM | P0 | implemented | LLM 返回 executionGraph 时不执行，记录 contract violation |
| GFA-04 | DoD assessment 缺口规范化为 missingDoDItems | Orchestrator | P0 | implemented | failed/skipped/missing 项能形成结构化缺口输入 |
| GFA-05 | repair/replan 优先使用 missingDoDItems | Orchestrator | P0 | implemented | repair 输入包含缺口来源、原因、影响和建议动作，不再只依赖失败 step |
| GFA-06 | 增强通用 LLM contract | Blocks/LLM | P0 | implemented | prompt 明确 graph follow-up、assessment-only、DoD 缺口输出规则 |
| GFA-07 | 增强 runtimeCapabilities/输入标记 | Blocks/LLM | P1 | implemented | provider request 明确 `assessmentOnly=true` 时的输出约束 |
| GFA-08 | Run Snapshot / Observability 展示收口状态 | Read Model | P1 | implemented | 可看到 assessment-only、graph limit、missingDoDItems、repair source |
| GFA-09 | 增强 `software-dev` Soul | Template/Soul | P0 | implemented | 新增 Goal gap analysis、实现优先、验证不能替代实现等约束 |
| GFA-10 | 添加领域中立 orchestrator 单测 | Test | P0 | implemented | 覆盖 graph follow-up 超限后 assessment-only、拒绝 executionGraph、缺口驱动 repair |
| GFA-11 | 添加 LLM contract 单测 | Test | P0 | implemented | 确认 core prompt 包含通用约束且不含软件开发专属语义 |
| GFA-12 | 添加 software-dev 模板默认值单测 | Test | P0 | implemented | 确认 Soul 包含新增 Goal gap analysis 和实现优先约束 |
| GFA-13 | 添加 deployed e2e 回归 | E2E | P0 | implemented | 覆盖追加 Goal 后从检查/验证进入 assessment-only，再基于缺口 repair/replan，最终交付或 rejected；部署态 e2e 已通过 |
| GFA-14 | 动态同类 workspace 回归 | Manual/E2E | P0 | implemented | 不依赖历史 workspace ID；脚本动态创建隔离 workspace 并用唯一 marker 验证最新 Run 不再反复扩图，最终有 accepted/rejected Result 且无无界循环 |
| GFA-15 | execution activation 返回已创建 Run 的可观测状态 | Appservice/API | P0 | implemented | Run 已创建并推进到 failed/blocked/superseded 等可观测状态时，activation 返回 Run/快照，不将已发生运行隐藏成 409 |

## 单测建议

### Orchestrator

- `TestGraphFollowUpExpansionLimitSchedulesAssessmentOnly`
- `TestAssessmentOnlyRejectsExecutionGraph`
- `TestDoDAssessmentMissingItemsDriveRepairInput`
- `TestGraphFollowUpLimitDoesNotCreateUnboundedRepairGraph`

### LLM

- `TestDecisionContractIncludesAssessmentOnlyGraphLimitRules`
- `TestAssessmentOnlyProviderRequestCarriesOutputConstraint`
- `TestCorePromptDoesNotContainSoftwareDevSpecificRules`

### Bootstrap Defaults

- `TestSoftwareDevSoulRequiresGoalGapAnalysisBeforeValidation`
- `TestSoftwareDevSoulDisallowsVerificationOnlyForNewImplementationGoal`

## E2E 建议

新增或扩展软件开发 e2e 场景：

1. 创建 `software-dev` workspace。
2. 初始项目具备已通过的基础能力。
3. 追加新 Goal / DoD。
4. Mock LLM 或真实 LLM 先返回检查/验证 graph。
5. graph follow-up 多次返回新 graph。
6. Runtime 达到扩展上限后进入 assessment-only。
7. assessment 输出缺口。
8. Runtime 基于缺口进入 repair/replan。
9. 最终验证：
   - 要么最新 Run 输出 accepted Result / Artifact。
   - 要么最新 Run 输出 rejected Result，且缺口、证据、下一步动作完整。
   - 不允许继续停留在 running + repeated graph expansion。

## 验收标准

- 动态创建的同类 e2e workspace 不再无限扩图或连续 replan。
- graph follow-up 超限后必然进入 assessment-only。
- assessment-only 模式下 executionGraph 不会被执行。
- DoD 未满足时，最终 Result 必须明确 rejected，并包含缺口、证据和 nextActions。
- DoD 满足时，最终 Result / Artifact 必须来自最新 active Run。
- Core 代码不包含软件开发、Go、config.yaml、多 LLM provider 等具体业务规则。
- `software-dev` Soul 承担软件开发场景策略。
- 正式 e2e 不使用固定历史 workspace ID；脚本必须创建独立 workspace，并通过唯一 marker 过滤旧 Run/旧数据。

## 验证命令

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/bootstrap -count=1
go test ./... -count=1
git diff --check
```

部署后回归：

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 ./scripts/verify-software-dev-goal-dod-e2e.sh
WORKORCH_BASE_URL=http://127.0.0.1:8080 WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh
```

## 跟踪记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-10 | 初始创建。基于历史排障样本暴露的问题，形成 graph follow-up 超限后 assessment-only、DoD 缺口驱动 repair/replan、software-dev Soul 策略增强的收口计划；该样本不作为 e2e fixture。 |
| 2026-05-10 | 第一批实现：graph follow-up 超限后改为调度 assessment-only follow-up，不再直接进入自由 repair graph；assessment-only 返回 executionGraph 时作为 contract violation 失败且不执行；通用 LLM contract 和 `software-dev` Soul 增加对应约束；补充 orchestrator、LLM、bootstrap 默认值单测。 |
| 2026-05-10 | 第二批实现：DoD assessment 中 failed/skipped/missing 条目规范化为 `missingDoDItems` 和 `runtimeDiagnostics`，让后续 repair/replan 可直接基于缺口来源、原因、影响、证据和建议动作继续推进。 |
| 2026-05-10 | 第三批实现：Run Snapshot / Observability 增加 `closureTraces` 投影，展示 graph follow-up、assessment-only、graph expansion limit、ignored executionGraph 和 missingDoDItems，方便 Web/TUI 和排障链路直接追踪收口状态。 |
| 2026-05-10 | 第四批实现：`verify-software-dev-goal-dod-e2e.sh` 增加 graph follow-up expansion limit → assessment-only → `closureTraces` 场景，并通过唯一 marker 隔离部署环境中的旧 Run；同时修复 activation API 在 Run 已创建并进入可观测状态后返回 409 的问题。本地 `go test ./... -count=1` 与 `git diff --check` 通过；部署态 e2e 需包含本批代码后复验。 |
| 2026-05-10 | 第五批收口：修复自动治理 `replan` 后的 continuation 调度缺口。新增 `/governance/auto/resolved` 监听、`/execution-activations` 同步返回 `superseded` 时的 continuation 调度，以及启动时补偿扫描；e2e Scenario-2 支持自动/显式 replan 两种配置并验证 continuation accepted Result。同步修复 web/fetch final URL 测试的外部 DNS 依赖。本地当前工作树已通过 `go test ./... -count=1`、`git diff --check`，并在临时 file backend 服务 `127.0.0.1:18088` 通过 `verify-software-dev-goal-dod-e2e.sh` 全场景。 |
| 2026-05-10 | 部署态复验完成：重新部署后执行 `WORKORCH_BASE_URL=http://127.0.0.1:8080 ./scripts/verify-software-dev-goal-dod-e2e.sh` 通过，覆盖 Scenario-1 到 Scenario-6；脚本使用动态创建的隔离 workspace 和唯一 marker，历史排障样本仅作人工诊断参考，不纳入 e2e 验收数据；`GFA-13/GFA-14` 收口。 |
