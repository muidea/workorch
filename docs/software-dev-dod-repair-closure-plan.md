# Software Dev DoD Repair Closure Plan

## 背景

基于 `ws-e1e310fd02e4375e` 的最新核对，`software-dev` Workspace 已能进入 Goal / DoD 自驱动开发流程，且 DoD 全量覆盖门禁已经生效。但复杂软件开发任务仍存在 repair loop 不收敛、历史 accepted Memory 污染、DoD 缺口无法稳定转化为代码和测试修复等问题。

本文档用于独立跟踪这些遗留项，避免把场景缺陷散落在运行日志或总计划中。目标是在不引入业务特例、不破坏 WorkOrch 通用编排能力的前提下，让软件开发类 Workspace 能够围绕当前有效 Goal / DoD 自动补实现、补测试、验证并产出可信交付。

## 核心原则

- WorkOrch 核心仍保持领域中立，只处理 Goal / DoD / Evidence / Repair / Memory / Result 等通用模型。
- `software-dev` 相关要求优先沉淀到 WorkspaceTemplate、Soul、默认 DoD、e2e 验收和通用质量门控中。
- 不以旧 accepted Result 或旧 Memory 直接证明当前 DoD 已满足。
- 不用 provider timeout、API Key 字样或业务文本关键词替代真实 failure stage 分类。
- DoD 满足必须依赖当前有效 DoD 的逐项证据，而不是 LLM 自选子集。

## 当前暴露问题

| ID | 问题 | 现象 | 影响 |
| --- | --- | --- | --- |
| SDR-ISSUE-01 | DoD coverage missing 被误分类 | `llm_dod_assessment` 阶段缺失 DoD 项时，因为文本包含 API Key 被归为 `auth_or_config_error` | repair 策略被误导，无法聚焦补实现和补测试 |
| SDR-ISSUE-02 | missing DoD 未结构化进入 repair | 后续 repair 只做 `file/list` 或泛化读取，未稳定生成 patch/test plan | repair loop 长时间运行但不收敛 |
| SDR-ISSUE-03 | 历史 accepted Memory 污染 | 旧规则下 accepted 的 run summary 被当前 run recall | LLM 容易提前声称“核心功能已满足” |
| SDR-ISSUE-04 | DoD 条目类型未区分 | 标题项、父级项、叶子验收项混在 `dod.items` 内 | 评估和修复目标不清晰 |
| SDR-ISSUE-05 | DoD-to-test 映射不稳定 | 缺失项没有映射到文件、测试和验证命令 | 无法形成自动开发闭环 |
| SDR-ISSUE-06 | repair 无效循环缺少收敛边界 | 多轮 DoD coverage missing 后仍继续泛化 repair | Run 长时间 `running`，用户难以判断下一步 |
| SDR-ISSUE-07 | 证据质量校验仍偏弱 | `go test`、`go build` 通过容易被泛化覆盖多个 DoD | 无法证明配置、并发、控制台反馈等条目满足 |
| SDR-ISSUE-08 | `software-dev` 实现细节缺口未被有效驱动 | 示例项目存在 `config.yaml`，但代码未读取该文件 | “配置管理” DoD 实际未满足 |

## 任务清单

| ID | 任务 | 类型 | 优先级 | 状态 | 处理方案 | 验收标准 |
| --- | --- | --- | --- | --- | --- | --- |
| SDR-01 | 修复 DoD coverage missing failure classification | 通用后端 | P0 | completed | 在 failure classification 中新增 `dod_coverage_missing`，并在 `llm_dod_assessment` 场景优先识别 DoD 缺口，避免被 API Key / auth 词误分类 | `dodAssessment does not cover all current DoD items` 不再归类为 `auth_or_config_error`；repairLoop 与失败分类一致 |
| SDR-02 | 结构化传递 missing DoD items | 通用后端 | P0 | completed | DoD 覆盖缺口通过 typed diagnostics 写入 `runtimeDiagnostics.missingDoDItems`（raw/normalized/kind/requiredEvidenceHint），并透传到 repair input 与修复升级上下文 | repair follow-up 能直接使用结构化缺口列表，无需依赖错误字符串反解析 |
| SDR-03 | repair prompt 强制 DoD-to-implementation/test plan | 通用后端 / Soul | P0 | completed | LLM 决策契约增加 `dod_coverage_missing` 约束：必须将缺口映射到 affected files、patch/apply、verification commands，禁止泛化 inspection loop | DoD 缺口 repair 的 prompt contract 已具备强约束，不再允许仅 `file/list` 循环 |
| SDR-04 | 增加无效 repair 收敛保护 | 通用后端 | P0 | completed | 结合已有 repair attempt 上限与升级机制，补充升级决策上下文（missingDoDItems、recentRepairAttempts、recommendedNextActions） | run 不会无限 `running`；repair exhausted ticket 可直接给出下一步修复方向 |
| SDR-05 | run-summary Memory 版本化与中立化 | Memory / 上下文 | P0 | completed | 新写入 run-summary memory 增加 `dod_gate_version:v2` 与中立 `runtime_result` tag；当前阶段不保留旧 summary 兼容降权分支 | recall 上下文不再注入历史兼容 trust marker；新 summary 只能作为带版本标签的运行结果证据参与上下文 |
| SDR-06 | DoD 条目类型识别 | 通用后端 | P0 | completed | completion gate 解析 DoD 为 `heading/parent/leaf`，仅 `leaf` 进入强制覆盖门禁 | 标题与父级分组不再误判为必须 satisfied 条目；叶子验收项必须覆盖 |
| SDR-07 | 强化 evidence-to-DoD 校验 | 通用后端 / resultQuality | P0 | completed | `resultQuality` 增加 DoD 叶子覆盖检查、satisfied 条目 evidence 链接性检查、弱验证证据检查 | build/test 泛化证据无法覆盖全部 DoD；弱证据与缺失证据会被标记 failed |
| SDR-08 | 强化 `software-dev` Soul 的缺口修复策略 | Template / Soul | P0 | completed | 软件开发 planning/execution/validation Soul 已固化 DoD-to-test、弱证据拒绝、补实现补测试优先策略；并由 LLM 契约补强 repair 约束 | `config.yaml` 等条目不再因文件存在被误判 satisfied，需实现与测试证据闭环 |
| SDR-09 | 补充复杂软件开发 e2e 回归 | e2e | P0 | completed | `verify-software-dev-goal-dod-e2e.sh` 覆盖自动修复、修复耗尽升级、空 workspace 初始化、累积 Goal/DoD、弱证据拒绝等场景 | 已能稳定复现并阻断 `ws-e1e310fd02e4375e` 类回归 |
| SDR-10 | Web/TUI 展示缺失 DoD 与 repair 下一步 | Web / TUI | P1 | completed | `repairLoop` 投影增加 `missingDoDItems/recentAttempts/recommendedNextSteps`，并从失败 step 与升级 ticket 上下文回填 | 用户可直接看到未完成原因与下一步建议，不再只看到泛化 running |
| SDR-11 | 费用与上下文统计关联 repair 轮次 | Observability | P1 | completed | 保持现有 usage/context/repair attempt 观测链路，并在 repairLoop 中补齐 failure/missingDoD/recentAttempt 信息用于关联分析 | Workspace/Run 可按 repair attempt 与失败上下文对齐排查成本与收敛性 |

## 验收文档项

### SDR-E2E-01：DoD coverage missing 分类

构造一个 LLM 返回 `satisfied=true` 但遗漏部分 DoD 的 run。

验收：
- run 不产生 accepted Result。
- rejected Result 记录 missing DoD items。
- failureClass 为 `dod_coverage_missing` 或 `validation_gap`。
- repairLoop 不显示 `auth_or_config_error`。

### SDR-E2E-02：missing DoD 驱动 repair

构造缺少配置文件加载、并发测试或错误透传测试的 `software-dev` workspace。

验收：
- repair input 包含缺失 DoD 的结构化列表。
- 下一轮 task plan 显式包含 affected files、patch plan、test plan、verification commands。
- 产生有效 patch 或测试变更，而不是只执行 `file/list`。

### SDR-E2E-03：旧 accepted Memory 不作为强证据

准备一个历史 accepted run memory，但其 accepted result 没有经过当前 DoD 全覆盖门禁。

验收：
- 当前 run 可以 recall 该 memory。
- context 中标记为 legacy / pre-gate evidence。
- 最终 DoD assessment 仍必须基于当前 run evidenceRefs。

### SDR-E2E-04：DoD heading 与 leaf item 解析

提交包含 markdown heading、父级 checklist、叶子 checklist 的 DoD。

验收：
- heading 不要求 satisfied。
- leaf acceptance items 必须覆盖。
- resultQuality / doDEvidence 能保留展示层级。

### SDR-E2E-05：复杂软件开发闭环

以空 `software-dev` workspace 提交本地 LLM gateway 目标和细粒度 DoD，覆盖：
- `POST /v1/chat/completions` 基础转发。
- stream true 流式响应。
- usage 提取。
- 流式无 usage 的 token 估算。
- 401 / 429 异常透传。
- `usage.csv` 追加记录。
- 控制台摘要输出。
- 并发安全。
- `config.yaml` 或环境变量配置。
- 单二进制构建。

验收：
- 系统自动初始化项目、补实现、补测试。
- 所有 DoD leaf items 均有对应 evidenceRefs。
- `go test`、`go test -race`、`go vet`、`go build` 或等价命令通过。
- 产出 accepted Result、result-summary、deliverable-manifest、workspace-change-diff。
- 若无法满足，必须输出 rejected / failed Result，并列出缺失项和下一步。

## 与现有计划的关系

- 本文档是 `docs/software-requirement-development-orchestration-plan.md` 的专项后续收口文档。
- 已完成的 SRD-37 / SRD-38 解决了 DoD 子集误接受和 workspace outcome 指向旧 run 的问题。
- 本文档继续处理 DoD 全覆盖门禁生效后暴露出的 repair 收敛、证据质量、历史 memory 可信度和软件开发模板执行策略问题。

## 变更记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-08 | 初始创建。基于 `ws-e1e310fd02e4375e` 最新核对，将 DoD coverage missing、repair loop、Memory 可信度、DoD-to-test、证据质量和 e2e 回归拆分为独立收口任务。 |
| 2026-05-08 | 完成 SDR-01~SDR-11 收口：新增 `dod_coverage_missing` 分类、missing DoD 结构化透传、leaf DoD 覆盖门禁、repair 升级上下文增强、run-summary gate 版本标记、resultQuality 叶子覆盖与弱证据校验、repairLoop 缺口投影；并通过 `go test`（orchestrator/appservice）与 `scripts/verify-software-dev-goal-dod-e2e.sh`、`scripts/verify-software-requirement-development-orchestration.sh` 验证。 |
