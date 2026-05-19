# WorkOrch Runtime Core Flow Contract

本文定义 WorkOrch runtime 的核心执行合约，用于避免后续继续以单点补丁方式修复局部症状。当前阶段目标是彻底收口 WorkOrch 自治执行能力，不保留旧兼容行为。

## 目标

- 让 LLM 输出、runtime graph、repair、provider retry、DoD gate、result 形成单一可解释状态机。
- 明确每个阶段允许的输出类型和禁止输出。
- 将 failure classification 映射到唯一恢复策略，避免 repair、provider retry、replan 互相嵌套。
- 明确终态 run 是否必须写 result，以及何时只开 decision。

## 关联设计

- `docs/workorch-runtime-lineage-and-assessment-contract.md`：定义 lineage、assessment-only、state-changing graph 准入和 correction 合同。
- `docs/workorch-runtime-readable-id-and-recovery-contract.md`：定义可读 runtime ID、provider retry 边界、contract violation 统一治理、验证脚本口径和 memory 噪音治理合同。
- `docs/workorch-runtime-provider-retry-repair-closure-design.md`：定义 provider retry / runtime repair 交织路径的 root 归一、terminal policy 与 memory trace 补充合同。
- `docs/workorch-runtime-stage-contract-neutrality-design.md`：定义面向长流程任务的中立阶段合约，明确 core runtime 不得内置软件开发或具体业务领域逻辑。

## 核心状态机

| 状态 | 进入条件 | 允许下一步 | 禁止 |
| --- | --- | --- | --- |
| `created` | run 创建完成 | `StartRun` 派发初始 LLM step | 直接写 result |
| `running` | runtime 正在执行 step / graph / repair / retry | 派发 graph node、follow-up、repair、provider retry、completion gate | 已有 rejected result 后继续派发 |
| `blocked_on_decision` | 真实需要人工信息、provider 配置修复、repair exhausted | 等待 resolution 后 resume / replan / terminate | 自动反复重试同一失败 |
| `blocked_on_approval` | 高风险操作需要审批 | 等待 approval 后继续 | 未审批执行高风险动作 |
| `completed` | DoD gate 验证通过并写 accepted result | 无 | completion 无 accepted result |
| `failed` | 不可恢复失败、rejected result、恢复策略终止 | 无 | failed 无 rejected result，除非 failure 属于纯外部 provider blocker 且 run 保持 blocked |
| `superseded` | replan 或新 run 取代旧 run | 无 | superseded 后继续派发 |

## 阶段输出合约

| 阶段 | 输入标识 | 允许输出 | 禁止输出 |
| --- | --- | --- | --- |
| planning | `runtimeStage=planning` | 可输出探索 graph；若当前 execution contract 要求 state change，则必须输出含合法 state-changing 节点的 graph、直接 state-changing output 或 blocker | state-changing contract 下只读 graph |
| execution graph node | `runtimeStage=execution` + `graphNodeId` | 具体 runtime output、合法 state-changing output、DoD assessment、decision/approval blocker | 只返回下一层 `executionGraph`；state-changing 节点 shell-only / read-only |
| graph follow-up | `graphFollowUp` / `completionFollowUp` / `continuationFollowUp` | 合法 patch graph、DoD assessment、blocker | 只读 graph |
| assessment-only | `assessmentOnly` / `providerRetryAssessmentOnly` | `dodAssessment` 或真实治理 blocker | `executionGraph`、patch、shell、memoryWrites |
| runtime repair | `runtimeRepairFollowUp` | 按 repair objective 输出具体修复、failed DoD assessment、blocker | 与 objective 冲突的 graph / shell-only |
| state-changing repair | `stateChangingOutputRequired=true` | `patchRequests` / `patches` / `nextKind=patch/apply`、包含合法 `patch/apply` 节点的 state-changing `executionGraph`、failed `dodAssessment`、blocker | read-only / inspection-only / shell-only graph、placeholder patch |
| provider retry | `providerRetryFollowUp` | 重试同一 LLM 请求，保留原阶段合约和 state-changing 约束 | 改变业务策略、创建新的 retry root、repair provider 自身 |

## 有效进展定义

在要求 workspace state change 的 Goal 下，以下输出才算有效进展：

- 成功应用 state-changing artifact 或返回可派发的 state-changing step。
- 新增或更新 workspace 交付内容，并能被后续验证引用。
- 输出 failed/skipped DoD assessment，明确列出缺口、影响和下一步。
- 输出真实 blocker，例如 provider 配置不可用、缺少必要需求、需要审批。

以下不算有效进展：

- 重复 file/list、file/read、shell scan。
- 只运行 build/test 但没有覆盖新增 Goal 或没有任何变更。
- LLM 反复返回下一层 `executionGraph`。
- patch 内容含 `__FROM_PREVIOUS_STEP_OUTPUT__`、`<<...>>`、省略号、TODO 代替实现。
- 在空 workspace 或缺失目标内容场景下，反复读取不存在的目标文件而不创建内容、不输出 result、不开 decision。

## Failure 分类

| failure class | 来源 | 恢复策略 |
| --- | --- | --- |
| `provider_timeout` / `rate_limited` / `context_too_large` | LLM provider runtime failure | provider retry，重试耗尽后 block_on_decision，要求修复 WorkOrch runtime provider |
| `provider_schema_drift` | provider response/protocol schema 与 runtime provider 适配不一致 | provider/config decision 或 provider adapter 修复，不进入业务 runtime repair |
| `llm_decision_invalid` | LLM 输出 JSON / WorkOrch decision schema / 阶段输出合约无效 | 普通执行阶段进入 runtime repair / contract correction；assessment-only 终态写 rejected result |
| `runtime_failed` + contract message | LLM 输出违反 WorkOrch 合约 | runtime repair；若是 state-changing contract，repair 必须继承 state-changing |
| `patch_apply_failed` / `patch_dispatch_invalid` | patch 校验或应用失败 | state-changing repair，必须基于当前文件快照生成具体 patch；若当前目标已机械满足失败 patch 意图，则禁止重复同一 patch，改为 concrete validation evidence graph 或 failed assessment/blocker |
| `dod_coverage_missing` | DoD/Goal coverage 不足 | repair 或 rejected result；不得继续 assessment-only graph |
| `workspace_path_conflict` | 文件缺失、路径冲突 | repair 可先读取/列目录，但若原目标是 state-changing，必须回到具体 patch |
| `shell_command_failed` | build/test/lint 失败 | repair 失败根因，重新 patch 或补测试 |
| `mcp_*` | MCP 能力或服务失败 | MCP repair / decision，不得误归入 provider retry |

## Recovery 优先级

`handleRuntimeStepFailure` 后只能选择一个恢复动作，优先级如下：

1. 已有 rejected result：关闭 run，禁止继续派发。
2. provider transient failure：provider retry。
3. provider retry exhausted：block_on_decision，scope 为 WorkOrch runtime provider。
4. runtime repair exhausted：写 rejected result 并 block_on_decision / replan。
5. duplicate runtime failure：block_on_decision 或 terminate。
6. runtime repair allowed：派发 repair。
7. 不可恢复：写 rejected result 并 fail run。

## Provider Retry 约束

- provider retry 只处理 LLM provider transient failure：timeout、rate limit、context too large。
- contract failure 不得进入 provider retry。
- provider retry step 失败后，不得以 retry step 自身作为新的 retry root。
- provider retry root 必须归一到原始业务 step 或原始 repair step。
- provider retry 计数按 root 聚合，`provider-retry-1-provider-retry-1` 属于非法 lineage。
- provider retry input 必须继承原始阶段合约，包括 `stateChangingOutputRequired`、assessment-only、completion follow-up 等约束。
- provider retry exhausted 只开 WorkOrch runtime provider decision，不写业务 rejected result，除非 recovery policy 明确 terminate。
- provider retry exhausted 的 decision 被 resolve 后，如果 provider 仍未恢复且同一 root 的 retry budget 仍耗尽，runtime 必须重新进入 provider decision blocker；不得因重复 ticket ID 或内部 conflict 将 DoD run 关闭为 failed。

## Runtime Repair 约束

- repair scope 必须按 root step、failure class、failure signature 归一。
- repair-of-repair 必须继承更强约束，不得降级。
- repair-of-repair 若源自 graph-scoped 失败，即使当前失败发生在 repair 外壳或 LLM schema invalid 外壳上，后续 repair ID、lineage label 与 input scope 也必须保持原始 graph failure label；不得退回 `planning-001` 或其他早期 root label。
- coverage repair 的 repair-of-repair 必须继承上一轮 `missingDoDItems`、`missingGoalCoverage`、`missingDeliverableCoverage`、候选 evidence、已完成 action evidence、当前失败信息与最近 repair 尝试摘要；`repairActionPackage` 必须携带 `coverageRepairRequired`、`coverageTargetSet`、`currentExecutionProgress` 和 `requiredOutput.mustDeclareCoverageTargets`，避免 LLM 只看到输出合同错误却看不到实际进展与未关闭目标。
- LLM 输入上下文完整性、顺序、脱敏、摘要和 request audit 必须遵守 `workorch-runtime-llm-context-integrity-design.md`；P0/P1 必要上下文不得被外壳失败、历史 superseded evidence、context compaction 或 redaction 静默覆盖。
- state-changing planning read-only graph failure 首轮 repair 直接进入 state-changing repair。
- DoD run 中 `file/read` / `file_read_invalid_path` / `no such file` / `lstat` 这类缺失目标文件失败，若发生在 state-changing 闭环内，repair 必须进入 `produce_state_changing_output`，并提示创建缺失内容，而不是继续重复读取。
- state-changing planning / repair graph 中，合法 state-changing 节点不能依赖前置 `llm/workspace_execution` 分析节点；分析必须在 planning 阶段完成，state-changing graph 必须能从只读取证节点直接推进到状态变更。
- runtime repair budget 按已派发 repair attempt 计数；provider timeout 的 repair step 可以触发 provider retry / provider blocker，但不得导致同一 repair attempt 反复生成 `-2/-3/-4`。
- provider retry of runtime repair 必须继承 state-changing repair contract，包括 `repairObjective=produce_state_changing_output`、`stateChangingOutputRequired=true`、repair scope 与 repair guidance；不得把必须产出 state-changing output 的责任降级为后续 LLM execution node。
- state-changing repair 失败不能退回普通 graph planning。
- repair 输出 failed DoD assessment 时，如果不可执行 continuation，应写 rejected result 并关闭或进入 decision，不得继续循环。

## Result 与终态

| 终态 | result 要求 |
| --- | --- |
| `completed` | 必须有 accepted result，且 DoD/Goal coverage 与 evidence refs 完整 |
| `failed` | DoD gate run 必须尽力写 rejected result |
| `blocked_on_decision` | provider/config blocker 可不写 result；业务不可完成 blocker 应写 rejected result 或带 failed assessment |
| `superseded` | 不强制写 result，但若 supersede 前已判定 rejected，应保留 result；若 superseded 来源是 resolved replan，必须创建 successor run 继续执行 |
| `cancelled` | 不强制写 result |

## DoD Assessment 归一化边界

- Activation / resume 构造 execution context 时必须以 definition event 的最后一次出现确定 Goal / DoD 顺序。同一 Goal / DoD ID 的重复事件只能形成一个 lineage 条目，并通过 `definitionChangeImpact.*.definitionRevisions[]` 表示修订次数；不得让重复事件把旧 ID 重新插入到 `allGoalIds` / `allDoDIds`，导致 runtime 误判 effective definition 或 LLM 误用旧验收基准。
- Core 对 Goal / DoD 覆盖只能做中立结构校验：coverage item、acceptance dimension、deliverable expectation、matrix row 与 evidenceRefs / artifactRefs 必须完整且可追踪。Core 不得内置任何具体业务对象、供应商、协议、接口、字段或测试框架名称，也不得根据自然语言中的领域词自动判定某项能力已满足。
- Completion gate 在校验前可以对 `dodAssessment` 做确定性结构归一化，包括规范化 evidence ref、补齐父级 `satisfied=true` 但子项 `state` 为空的 coverage state、以及为声明 satisfied/available/produced 但遗漏 `evidenceRefs` 的项补入当前 run 的可用证据。
- 归一化只能使用当前 run 中已完成、可追溯、非弱证据的中立 evidence，例如成功的 shell/tool/patch/validation 步骤；`file/read`、`file/list`、`mcp/resource_read`、LLM 自身输出、zero-work/inconclusive/invalid evidence 不能作为自动 fallback。
- 若没有可用 evidence，runtime 不得凭空接受 assessment，必须继续按 completion gate 失败、repair、rejected result 或 decision blocker 收口。
- 该规则只治理结构化证据引用完整性，不引入任何业务领域判断；业务语义是否充分仍由 Goal/DoD、profile contract、LLM assessment 与 evidence quality gate 共同约束。
- graph follow-up 因扩展次数达到上限而转入 assessment-only 时，assessment 输入必须携带当前 run 的 `runEvidenceSummary`、`latestClosureEvidence` 和必要的 `supersededFailureEvidence`。LLM 对 satisfied item 的 `evidenceRefs` 必须引用具体可追踪 step / patch / validation evidence，不能只引用 run ID、memory ref 或旧失败 step。
- 当 `runEvidenceSummary` 因上下文预算需要裁剪时，runtime 必须优先保留中立 action evidence，例如已完成/失败的 shell、tool、patch、validation 或其他可证明状态变化/验证结果的步骤；不得让后续 LLM 过程步骤把关键验证/变更证据全部挤出 assessment 输入。
- assessment-only coverage correction 对 graph expansion limit assessment 同样适用。若 failed `dodAssessment.items`、`goalCoverage.items` 或 `deliverableCoverage.items` 缺少可追踪 evidence，repair input 必须带入具体 `evidenceCandidates` 或最新 action evidence 候选；只有候选证据仍不足时，才切换为 concrete validation evidence repair、rejected result 或 decision blocker。
- 若 assessment-only coverage correction 自身返回 `satisfied=false` 的 failed `dodAssessment`，并明确列出仍缺失的 DoD / Goal / deliverable coverage，下一步 repair 必须清除 `assessmentOnly`，转入 concrete validation evidence repair 或治理终态；不得继续携带 assessment-only 约束，也不得漂移到无关 planning / scan scope。

## 抽象矩阵与证据门禁

当当前 Goal / DoD 或 profile contract 涉及多目标、接口/格式转换、配置、流式/长流程、错误行为、副作用或并发时，LLM assessment 应输出中立覆盖矩阵或等价的结构化 coverage items。Runtime completion gate 的职责是确认：

- 每个矩阵行或 coverage item 都有稳定标识、状态和证据引用。
- satisfied 项不能只引用 read-only、zero-work、inconclusive 或不可追踪证据。
- 一个通过命令、一个兼容 mock、一个 summary 或一个笼统 DoD item 不能自动覆盖所有矩阵行。
- result summary 中的能力声明必须能映射到对应 coverage item / matrix row / evidence refs。

矩阵的具体行内容只能由 Goal / DoD、workspace 当前上下文、profile / template / soul / skill 或 LLM assessment 动态产生；core flow 不允许预置任何业务领域行、字段或验证语义。若证据不足，runtime 应进入 assessment correction、repair、rejected result 或 decision blocker，而不是把领域语义写入 core。

## 当前实现偏差

| ID | 偏差 | 风险 | 收口方案 |
| --- | --- | --- | --- |
| RFC-01 | provider retry 可以对 provider retry step 再派生 retry root | 出现 `provider-retry-1-provider-retry-1`，多消耗 LLM 并模糊失败归因 | provider retry root normalization，禁止 nested provider retry |
| RFC-02 | provider retry 与 runtime repair 可交织在同一 failure scope | timeout 与 contract failure 混合后 repair budget / retry budget 不易解释 | recovery action 增加明确互斥规则和 root lineage |
| RFC-03 | `llm_schema_invalid` 同时被视为 provider failure stage 和 contract/schema failure | schema invalid 可能走 provider retry，也可能应直接 contract correction / rejected result | 将 schema invalid 分为 provider transport schema 与 LLM contract schema |
| RFC-04 | blocked_on_decision 是否写 rejected result 规则不够统一 | provider blocker 合理无 result，业务 blocker 可能缺 rejected result | 增加 blocker scope 到 result policy 映射 |
| RFC-05 | state-changing repair 仍可能返回非 state-changing graph 或 placeholder patch | 过早禁止所有 `executionGraph` 会阻断合法多步骤 patch graph；允许所有 graph 又会回到只读循环 | 仅允许包含合法 `patch/apply` 的 state-changing graph；继续拦截 read-only / shell-only graph |
| RFC-05A | 失败 patch 的目标状态已满足但仍重复提交同一 patch | context mismatch 后当前文件可能已经包含目标变更，继续强制 patch 会形成重复失败循环 | 增加 `currentTargetAlreadySatisfiesFailedPatch` 中立信号，repairActionPackage 禁止 `repeat_same_failed_patch`，允许受限 concrete validation evidence graph |
| RFC-06 | provider retry / runtime repair 交织后 root 可指向 repair-of-provider-retry 外壳 | `provider-retry-001-of-repair-001` 的 root 指向 `repair-001-of-provider-retry-001`，budget 与 result attribution 被外壳污染 | 新增 recovery root normalizer，provider retry / runtime repair / result attribution 均使用结构化业务 root |
| RFC-07 | provider/config terminal failure 可写业务 rejected result 并 failed | `auth_unavailable` 被当作业务 DoD rejected result，缺少 WorkOrch provider config decision | provider/config blocker 统一进入 `blocked_on_decision`，`resultPolicy=none` |

## 分阶段收口计划

| ID | 优先级 | 内容 | 验收 |
| --- | --- | --- | --- |
| DMI-19 | P0 | Provider retry root normalization 与 nested retry 禁止 | 已落地：同一 root 下最多 runtime 配置的 provider retry 次数（默认 10）；不会产生 `provider-retry-1-provider-retry-1` |
| DMI-20 | P0 | recovery action 决策表代码化 | 已落地：failure follow-up 与 resume dispatch 复用同一恢复动作表；contract failure 不进入 provider retry；provider retry 超预算不再派发 max+1 或 runtime repair |
| DMI-21 | P0 | schema invalid 语义拆分 | 已落地：`llm_schema_invalid` / `llm_decision_invalid` 归为 LLM decision contract failure，不再作为 provider failure stage；provider schema drift 使用显式 provider stage |
| DMI-22 | P1 | blocker-result policy | 已落地：decision context 显式 `resultPolicy`；provider/config blocker 不写业务 result；repair/duplicate/business terminal blocker 写 rejected result |
| DMI-23 | P1 | repair lineage trace | 已落地：provider retry / runtime repair input 显式记录 `recoveryAction`、`recoveryRootStepId`、`recoveryLineageType`、`recoveryBudgetScope` |
| DMI-24 | P1 | 部署核对脚本扩展 | 已落地：`scripts/verify-workorch-runtime-core-flow.sh` 自动检测 nested retry、recovery lineage 缺失、state-changing 降级、terminal/blocker result policy |

## 核对清单

每次部署后按以下顺序核对：

1. 最新 active run 是否存在。
2. planning / execution / repair / retry step 是否满足阶段输出合约。
3. state-changing repair 是否全程保持 `stateChangingOutputRequired=true`。
4. provider retry 是否只有同一 root 下的 `provider-retry-1`、`provider-retry-2`。
5. 是否出现 retry-of-retry、repair-of-provider-retry、assessment-only graph。
6. terminal run 是否有符合状态的 result。
7. Memory recall 是否选择高价值 input/output，而不是历史噪音。
8. 验证脚本自身 fixture 是否仍覆盖 handled/unhandled violation 和 graph scope 失败，避免部署核对口径漂移。

## 当前落地记录

- DMI-19 已落地：
  - provider retry input 增加显式 `providerRetryRootStepId`。
  - 非 provider retry step 上的通用 `failedStepId` 不再作为 provider retry root，避免 runtime repair 的业务 failed step 污染 retry lineage。
  - provider retry step 再失败时继续使用同一个 root 计算 attempt 和 step ID，不再派生 `provider-retry-1-provider-retry-1`。
  - 已补回归测试覆盖 repair step root normalization 和 retry failure 后的同 root attempt 聚合。
- DMI-20 已落地：
  - `dispatchRuntimeRepairFollowUp` 使用 `recoveryActionForFailedStep` 统一选择 provider retry、provider decision、repair decision、duplicate decision、runtime repair 或 fail run。
  - `nextDispatchSpecsFromStep` 复用同一 action 表，resume/补派路径不再绕过 provider retry budget。
  - `DispatchNextStep` 在恢复动作应进入 decision 时会实际阻塞 run，而不是只返回错误后保持 running。
  - provider retry 超预算后返回 blocked，不再创建 `provider-retry-3` / max+1，也不会退入 runtime repair。
  - contract/runtime failure 在 resume 路径派发 runtime repair，不会误归入 provider retry。
- DMI-21 已落地：
  - 新增独立 failure class `llm_decision_invalid`，用于 `llm_schema_invalid` / `llm_decision_invalid` 这类 LLM 输出合约错误。
  - `llm_schema_invalid` / `llm_decision_invalid` 不再属于 `isLLMProviderFailureStage`，因此不会触发 provider retry / provider exhausted 规则。
  - state-changing repair 的 `llm_schema_invalid` 后续 repair input 必须包含 schema invalid recovery contract：只允许基于 `repairActionPackage` 和当前 target snapshot 重建单个合法 JSON decision；不得复用 rejected raw JSON、markdown 包裹、注释、重复 JSON object、尾随多余括号或局部数组。
  - provider schema drift 保留为 provider failure，但必须使用显式 `llm_provider_schema_drift` / `llm_provider_response_schema_invalid` / `llm_provider_protocol_invalid` stage。
  - assessment-only / providerRetryAssessmentOnly 的 LLM decision contract terminal failure 继续写 rejected result 并关闭，不退回普通 repair。
- DMI-22 已落地：
  - decision context 增加 `resultPolicy`，当前支持 `none` 和 `rejected`。
  - provider retry exhausted 使用 `resultPolicy=none`，只阻塞 WorkOrch runtime provider config decision，不写业务 rejected result。
  - runtime repair exhausted 和 duplicate runtime failure 使用 `resultPolicy=rejected`，阻塞 decision 前写 rejected DoD result。
  - 普通 LLM decision block 只有显式 `resultPolicy=rejected` 时写 rejected result，避免可继续补信息的 blocker 被误判为终态失败。
- DMI-23 已落地：
  - provider retry input 写入 `recoveryAction=provider_retry`、`recoveryRootStepId`、`recoveryLineageType=provider_retry`。
  - runtime repair input 写入 `recoveryAction=runtime_repair`、`recoveryRootStepId`、`recoveryLineageType=runtime_repair`。
  - `recoveryBudgetScope` 结构化记录 lineage type、root step、failure class 和 failure signature，后续核对不再依赖 step ID 字符串推断。
- DMI-24 已落地：
  - 新增 `scripts/verify-workorch-runtime-core-flow.sh <workspace-id> <run-id>`。
  - 自动检查 provider retry step ID / root 是否出现 nested retry。
  - 自动检查 recovery step 是否具备 `recoveryAction`、`recoveryRootStepId`、`recoveryLineageType`、`recoveryBudgetScope`。
  - 自动检查 `stateChangingOutputRequired=true` 的 step 是否错误返回非 patch `executionGraph`。
  - 自动检查 completed/failed/blocked_on_decision 的 result policy 是否符合 contract。
  - 自动区分 unhandled violation 与已由 runtime repair / assessment correction / rejected result / decision ticket 覆盖的 handled violation。
  - Go fixture 回归已覆盖未治理 invalid patch graph、已治理 invalid patch graph、旧 `/block/` graph node ID、缺失 `graphSeq`。
- DMI-25 已落地：
  - DoD run 中缺失 workspace 文件读取失败会触发 state-changing runtime repair，repair input 写入 `repairObjective=produce_state_changing_output` 与 `stateChangingOutputRequired=true`。
  - repair guidance 明确要求空 workspace / 目标内容缺失时使用可审计的 state-changing 操作创建目标内容，不再重复读取。
  - 验证脚本新增 running DoD run 反复缺失文件读取且无 patch/result/ticket 的失败检查。
  - provider retry / runtime repair 这类 recovery step 可携带失败 graph node 上下文，但不按 graph dispatch step 强制要求 `/graph-<seq>/` ID；普通 graph dispatch step 仍必须满足 graph scope 与 `graphSeq` / `graphNodeSeq`。
- DMI-26 已落地：
  - provider retry exhausted blocker 重复创建时会复用未解决 ticket；如果旧 ticket 已 resolved，则创建带序号的新 provider blocker ticket。
  - 新 ticket 保留 `previousTicketId` 与 `repeatBlocker`，用于追踪 resolution 后 provider 仍不可用的重复阻塞。
  - resume 过程中的重复 provider blocker 不再外溢为 `ErrConflict` 并关闭 run 为 failed，仍保持 `blocked_on_decision` 与 `resultPolicy=none`。
- DMI-27 已落地：
  - resolved replan 不再只把当前 run 标记为 `superseded` 后结束；orchestrator 会创建 `<run-id>-replan-<seq>` successor run 并立即启动。
  - successor run 继承 workspace、Goal、DoD、priority 与 executionContext，并写入 `replanContinuation.predecessorRunId/ticketId/resolutionId` 以保持追溯。
  - HTTP replan continuation watcher 保留为外层补偿，但 replan 续跑能力已收口到 runtime 核心状态机。
- DMI-28 已落地：
  - state-changing planning graph 若把所有合法 state-changing 节点放在 `llm/workspace_execution` 依赖之后，runtime 立即按 contract failure 进入 repair，不再允许“graph 里有 state-changing 节点但永远先卡在分析 LLM”。
  - state-changing repair graph 复用同一约束，避免 repair 再次输出“先分析、后 patch”的阻塞 graph。
  - runtime repair attempt 计数改为按已派发 repair step 计入预算，provider timeout 不再让同一 attempt 反复派发；provider 不可用仍由 provider retry/provider blocker 单独治理。
- DMI-29 已落地：
  - provider retry of state-changing repair 显式继承 `repairObjective`、`stateChangingOutputRequired`、repair scope 与 repair guidance；即使旧输入缺失 marker，也会从 gated graph / read-only planning contract failure 中恢复 state-changing contract。
  - repair exhausted decision 创建前先写 rejected result，避免 ticket created 事件触发自动治理 replan 时抢先 supersede，导致 `resultPolicy=rejected` 但无 result。
  - 验证脚本新增 provider retry of repair 丢失 state-changing contract、以及 superseded/failed/blocked run 存在 rejected resultPolicy ticket 但缺 rejected result 的失败检查。
