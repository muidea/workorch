# WorkOrch 设计文档 vs 实现 全面核对报告

> 核对日期：2026-05-12
> 核对范围：docs/ 下全部设计文档 vs internal/ 实际实现
> 核对方法：逐文档提取契约/规则，在代码中验证实现

---

## 一、DMI 改进计划（历史 memory 改进计划）

### DMI-16 ~ DMI-24 验证结果

**结论：9 项全部已验证落地，代码与文档描述一致。**

| DMI | 标题 | 状态 | 关键验证点 |
|-----|------|------|-----------|
| DMI-16 | state-changing hard gate 与终态 result 收口 | 已落地 | `validateLLMExecutionOutputContract` + `failRunWithRejectedCompletionResult` |
| DMI-17 | follow-up read-only graph 收口 | 已落地 | `validateLLMFollowUpGraphStateChangingContract` 拦截 read-only graph |
| DMI-18 | Planning contract repair 首轮 state-changing 收口 | 已落地 | `stateChangingRepairRequired` 覆盖 planning read-only graph failure |
| DMI-19 | Provider retry root normalization | 已落地 | `providerRetryRootStepId` 显式字段 + nested retry 防止测试 |
| DMI-20 | Recovery action 决策表代码化 | 已落地 | `recoveryActionForFailedStep` 统一决策表，两条路径共用 |
| DMI-21 | Schema invalid 语义拆分 | 已落地 | `failureClassLLMDecisionInvalid` 独立于 provider failure |
| DMI-22 | Blocker-result policy 收口 | 已落地 | `resultPolicy` 字段 + `recordRejectedResultForDecisionBlock` 条件逻辑 |
| DMI-23 | Repair lineage trace 收口 | 已落地 | 四个 recovery lineage key 在 provider retry / runtime repair input 中均写入 |
| DMI-24 | 部署核对脚本扩展 | 已落地 | `scripts/verify-workorch-runtime-core-flow.sh` 存在且可执行 |

---

## 二、Runtime 核心流程契约（当前权威入口：`../../00-core/runtime-stage-contract.md`）

### 2.1 已正确实现的核心契约（13 条）

1. **状态机转换** — `created→running→completed/failed/blocked/superseded` 全部路径正确实现
2. **终态不可逆** — `canCloseRunAs` 只允许从非终态关闭
3. **superseded 后禁止派发** — `nextDispatchSpecs` 在 `isTerminalRunState` 时返回 conflict
4. **Recovery 优先级链** — provider retry → exhausted block → repair exhausted block → duplicate block → repair → fail
5. **Provider retry 约束** — 仅 transient failure（timeout/context_too_large/rate_limited）、root normalization、budget 来自 runtime 配置（默认 10）
6. **Contract failure 不进入 provider retry** — `isLLMDecisionContractFailureStage` 的 stage 不在 provider failure 列表中
7. **Runtime repair scope 隔离** — 按 root step + failure class + failure signature 归一
8. **State-changing repair 继承** — repair-of-repair 继承 `stateChangingOutputRequired`
9. **Failure 分类全覆盖** — 14 个 class 全部实现
10. **Output contract: state-changing gate** — `stateChangingOutputRequired=true` 时必须有 patch/apply 或 concrete output
11. **Output contract: assessment-only gate** — assessment-only step 返回 graph 时被拦截
12. **Output contract: graph node 禁止返回下层 graph** — `llmExecutionGraphNodeReturnedOnlyGraph` 检测
13. **Result 与终态** — completed 必须 accepted result；failed 尽力写 rejected result

### 2.2 实现偏差（6 条，风险可控）

| # | 偏差描述 | 风险 | 建议 |
|---|---------|------|------|
| 1 | `dispatchStep` 缺少 rejected result 前置拦截 — `nextDispatchSpecs` 和 `dispatchRuntimeRepairFollowUp` 都检查了，但 `dispatchStep` 本身没有防御性检查 | 低（内部路径已覆盖） | 在 `dispatchStep` 入口增加 rejected result 短路检查 |
| 2 | `created` 状态未显式拦截直接 result 写入 — 实际不会发生（result 由 step completion 触发） | 极低 | 可选：在 result 写入 API 增加 run state 校验 |
| 3 | Planning repair 依赖 error message 字符串匹配 — `stateChangingRepairRequired` 通过匹配 "implementation planning llm step returned read-only executiongraph" 触发 | 中（脆弱） | 改用结构化标记（如 output 中增加 `contractViolationType` 字段） |
| 4 | `dod_coverage_missing` 在 assessment-only step 中仍可进入 repair — 文档要求"不得继续 assessment-only graph"，但代码未显式编码 | 低 | 在 `shouldAttemptRuntimeRepair` 中增加 assessment-only + dod_coverage_missing 的排除条件 |
| 5 | `cancelled` 终态在文档状态机表中未独立列出 — 代码正确实现了 `CancelRun` | 无（文档遗漏） | 补充文档 |
| 6 | `DispatchNextStep` 的 block 行为路径不够直观 — `blockRecoveryActionFromFailedStep` 在返回 error 前已变更 run 状态，功能正确但可读性差 | 无 | 可选：增加注释说明副作用 |

### 2.3 实现中存在但文档未覆盖的行为（10 条）

| # | 行为 | 位置 | 建议 |
|---|------|------|------|
| 1 | `validating` 中间状态（running→completed 之间） | `validateAndCompleteRunWithWorkspace` | 补入状态机表 |
| 2 | Assessment-only contract correction step（二次机会） | `assessmentOnlyContractViolation` 处理 | 补入 output contract 节 |
| 3 | Graph follow-up expansion limit（`maxGraphFollowUpExpansionAttempts=2`） | runtime_orchestrator.go:78 | 补入 graph 执行节 |
| 4 | Continuable decision 自动转 repair | decision block 处理逻辑 | 补入 decision 处理节 |
| 5 | Duplicate failure blocker 自动 resume | `duplicateRuntimeFailureBlockerCanAutoResume` | 补入恢复路径节 |
| 6 | Repair exhaustion blocker success evidence 自动 resume | `runtimeRepairExhaustionBlockerHasSuccessEvidence` | 补入恢复路径节 |
| 7 | Running 状态下的 decision recovery | `runningDecisionRecoveryContext` | 补入 resume 节 |
| 8 | Patch dispatch gate | `validatePatchStepInputBeforeRuntime` | 补入 step dispatch 节 |
| 9 | Patch approval / revert approval block | `patchApprovalBlockSpec` / `patchRevertApprovalBlockSpec` | 补入 approval 节 |
| 10 | `blocked_on_approval` rejection → fail run | approval rejected 处理 | 补入状态机表 |

---

## 三、架构设计文档（当前权威入口：`../../30-module-architecture/overview.md`）

### 3.1 一致项

- Kernel / Blocks / Application 三层分层正确
- 核心数据模型全部存在（Workspace, Run, Step, Goal, DoD, Result, Artifact, Ticket, Resolution, Soul, Skill, Server, Capability, MemoryItem, EventRecord）
- 事件发布/订阅通信方式一致（MemoryHub）
- 服务接口直接调用方式一致（service 包暴露函数指针）
- Application 层 appservice 12 个核心文件全部存在

### 3.2 文档滞后的偏差

| 类别 | 文档描述 | 实际实现 | 建议 |
|------|----------|----------|------|
| Governance 模块 | 未列为独立 kernel 模块 | 权重最高(45)的 kernel 模块，负责自动治理评估/解析 | 补充为独立模块描述 |
| Result 模块 | 列为独立 kernel 模块 | 仅有 pkg/models，无独立注册，逻辑嵌入 orchestrator | 更正模块描述或补充实现 |
| File block | 未提及 | 已实现 file/list, file/read, file/edit 三种 step kind | 补充 blocks 列表 |
| Web block | 未提及 | 已实现 web/search, web/fetch 两种 step kind | 补充 blocks 列表 |
| Step Kind | 7 种 | 14 种（新增 file/list, file/read, file/edit, web/search, web/fetch, patch/preview, patch/revert） | 更新 step kind 表 |
| 事件 Topic | 18 个 | 29 个（新增 patch/memory/governance/streaming 相关） | 更新事件表 |
| App Service | 12 个 | 30+ 个 | 更新 service 列表 |
| 客户端应用 | 仅描述 workorchd | 还有完整 CLI/TUI/Agent 入口（workorch 子模块） | 补充客户端架构 |
| Soul 权重 | 20 | 25 | 更新 |
| Skill 权重 | 20 | 27 | 更新 |
| Memory 权重 | 20 | 26 | 更新 |
| EventStream 权重 | 20 | 22 | 更新 |

---

## 四、恢复治理设计（当前 active 规则入口：`../../20-runtime-flow/repair-replan-flow.md`；历史归档：`../../90-archive/completed/runtime-recovery-governance-closure-design.md`）

### 4.1 设计与实现一致的规则（22 条）

1. Provider retry 预算为有限次数 — `runtime.maxProviderRetryAttempts`，默认 `10`
2. Runtime repair 预算为有限次数 — `runtime.maxRuntimeRepairAttempts`，默认 `10`
3. Provider retry 仅对可重试 LLM 故障生效（timeout/context_too_large/rate_limited）
4. Provider retry 耗尽后创建 Decision block（`recoveryPolicy=system_config_required`、`dependency=llm_provider`）
5. Runtime repair 耗尽后创建 Decision block（`recoveryPolicy=runtime_repair_exhausted_requires_strategy`）
6. Repair exhausted ticket 可被 auto-governance 以 `replan` 自动解决
7. Auto-governance 在 `allowedRecoveryActions` 不含 `replan` 时升级
8. Auto-governance 在有 manual-required marker 时升级
9. Auto-governance 在 provider/config 依赖时升级
10. Duplicate failure signature 触发 Decision block
11. Duplicate failure block 仅对特定 failure class 生效
12. No-actionable continuation 最多派发一次 follow-up
13. Follow-up 仍无 actionable continuation 时进入 repair/exhaustion 路径
14. Repair exhausted block 记录 rejected result
15. Duplicate failure block 记录 rejected result
16. Fail run 时记录 rejected DoD assessment
17. Failure classification 覆盖设计文档列出的所有类别
18. Decision block 使用结构化 context keys
19. 核心代码保持领域中立
20. DoD coverage missing 独立分类（SDR-01）
21. Missing DoD items 结构化透传到 repair input（SDR-02）
22. Repair 收敛保护含 missingDoDItems/recentRepairAttempts/recommendedNextActions（SDR-04）

### 4.2 设计文档不够具体的规则（3 条）

| # | 问题 | 建议 |
|---|------|------|
| 1 | 文档只说"有限预算"，未写明 `runtime.maxProviderRetryAttempts` / `runtime.maxRuntimeRepairAttempts` 的配置来源与默认值 | 在 Recovery Model 节补充预算配置来源和默认值 |
| 2 | 文档未提及 duplicate failure signature 机制 | 补充为独立小节 |
| 3 | 文档未说明 auto-governance run-level budget 的具体实现 | 补充 budget 计算规则 |

### 4.3 实现中存在但设计文档未覆盖的规则（11 条）

| # | 实现规则 | 位置 | 建议 |
|---|---------|------|------|
| 1 | `maxGraphFollowUpExpansionAttempts = 2` | runtime_orchestrator.go:78 | 补入恢复治理或 graph 执行文档 |
| 2 | `terminalAssessmentOnlyDecisionContractFailure` — assessment-only 的 contract failure 直接终止 | runtime_repair.go:766-773 | 补入终态处理节 |
| 3 | `stateChangingRepairRequired` — repair 步骤是否需要 state-changing output | runtime_repair.go:906-942 | SDR-03 实现细节，补充检测逻辑描述 |
| 4 | `duplicateRuntimeFailureBlockerCanAutoResume` — duplicate failure block 后续 repair 成功时自动恢复 | runtime_repair.go:514-535 | 补入自动恢复路径节 |
| 5 | `runtimeRepairExhaustionBlockerHasSuccessEvidence` — repair exhaustion block 有成功证据时自动恢复 | runtime_repair.go:537-562 | 补入自动恢复路径节 |
| 6 | Provider retry 不适用于非 `initialStepKind` 的步骤 | runtime_repair.go:466 | 补入 provider retry 约束节 |
| 7 | `shouldCloseCompletionGateFailureWithoutRepair` — 特定条件下跳过 repair 直接 fail | runtime_repair.go:87-105 | 补入终态处理节 |
| 8 | Repair scope 按 `failureSignature` 隔离预算 | runtime_repair.go:489-512 | 补入 repair 预算节 |
| 9 | `patchRepairContext` — repair 步骤自动附带 workspace 文件快照 | runtime_repair.go:943-970 | 可选补充 |
| 10 | `workspaceFileManifest` — workspace_path_conflict 的 repair 附带文件清单 | runtime_repair.go:1079-1098 | 可选补充 |
| 11 | `rejectedRunResultSummary` 前置检查 — repair dispatch 前检查已有 rejected result 则直接 fail | runtime_repair.go:113-119 | 补入恢复优先级链 |

---

## 五、综合结论与行动建议

### 一致性评分

| 维度 | 一致性 | 主要问题 |
|------|--------|----------|
| DMI 改进计划 | **100%** | 无 |
| Runtime 核心流程契约 | **~85%** | 6 条轻微偏差 + 10 条未文档化行为 |
| 架构设计 | **~70%** | 文档严重滞后，缺少 governance/file/web 模块、CLI/TUI 入口 |
| 恢复治理设计 | **~90%** | 缺少具体数值和 3 个机制的文档描述 |

### 建议行动项（按优先级）

#### P0 — 高优先级

1. **更新 `../../30-module-architecture/architecture-overview.md`**
   - 补充 governance 模块描述（权重 45，自动治理引擎）
   - 补充 file/web blocks
   - 更新 step kind 表（7→14）
   - 更新事件 topic 表（18→29）
   - 补充 CLI/TUI/Agent 客户端架构
   - 更正 result 模块状态（非独立注册模块）

2. **更新 `../../90-archive/completed/runtime-recovery-governance-closure-design.md`**
   - 补充 duplicate failure signature block 机制
   - 写明预算数值（provider retry=2, runtime repair=3）
   - 补充 repair scope 按 failureSignature 隔离计数策略
   - 补充两个自动 resume 路径

#### P1 — 中优先级

3. **更新 `../../00-core/runtime-stage-contract.md`**
   - 补充未文档化的中立实现行为
   - 补充运行状态、终态、恢复和上下文合约到唯一核心设计文档

4. **修复代码偏差**
   - `stateChangingRepairRequired` 改用结构化标记替代字符串匹配
   - `dispatchStep` 入口增加 rejected result 短路检查

#### P2 — 低优先级

5. **文档维护**
   - 更新模块权重数值
   - 更新 app service 列表
   - 补充 patch approval/revert approval 触发条件描述
