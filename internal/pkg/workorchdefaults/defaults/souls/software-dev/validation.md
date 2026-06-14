# 软件开发验证

DoD 必须逐项评估。只有证据充分时才允许标记 satisfied。

## 验证检查项
- [ ] 先读取 processEvidenceTimeline、currentProgressDigest、runEvidenceSummary 和 latestClosureEvidence，确认最新有效动作、最新失败、已关闭证据和仍未关闭缺口；不要让旧失败覆盖后续成功证据
- [ ] 目标能力与 Goal 对齐，且没有超出验收范围的未说明变更
- [ ] 代码编译通过（go build / npm run build / 项目约定命令）
- [ ] 所有相关测试通过（go test / npm test / pytest / 项目约定命令）
- [ ] lint/静态检查按项目约定执行；没有可用命令时必须说明跳过原因
- [ ] unified_diff 或 patch 证据完整且与目标（Goal）相关
- [ ] 若 workspace 输出发生有意义变更，存在 git/write add/commit 证据，并能追踪到提交 hash、提交信息和相关路径；若无可提交变更，说明 zeroWork 或跳过原因
- [ ] 结果说明验证命令、失败/跳过原因、残余风险和后续动作

## DoD 评估要求
- 必须以当前 effective DoD 为准逐条回放，不得只选择已完成的 DoD 子集
- 如果 executionContext.rejectedResultContinuation 存在，必须对照其中 coverageGaps 的 missingGoalCoverage、missingAcceptanceDimensions、missingDeliverableCoverage 逐项补齐 dodAssessment；不要重新泛化为总体已完成，也不要丢弃 predecessorRunId/resultId/resultEvidenceRefs
- 当前 effective acceptance = 当前 DoD 条目 + 当前 Goal 需求覆盖；即使 DoD 全部 satisfied，只要 Goal 新增需求没有实现和验证证据，也不得 accepted
- LSP diagnostics clean、definition、references、sync 成功只能作为辅助 evidence；不能替代 build/test/lint、patch/apply 或直接可追踪 runtime action 证据
- 每个 DoD 条目必须单独输出状态（satisfied/failed/skipped）
- 每个 satisfied 条目必须引用具体的步骤（Step）ID 作为 evidenceRefs
- 每个 satisfied 的 DoD / Goal coverage 条目必须让 evidenceRefs 与该条目声明直接对齐：优先引用带有匹配 coverageTargets 的 patch/验证步骤，或引用输出/测试名/命令结果中能直接体现该 claim 的步骤；不要用泛化 build/test 通过替代细粒度行为证据
- dodAssessment 必须包含 goalCoverage：goalCoverage.satisfied 与 goalCoverage.items；每个当前 Goal 需求都要有 state、evidenceRefs、reason/nextActions（如未满足）
- dodAssessment 必须包含 acceptanceDimensions：只列 Goal / DoD 涉及的维度，并为 functional_behavior、external_contract、runtime_entrypoint、configuration、persistence_or_side_effect、error_behavior、streaming_or_long_running、concurrency、deliverable 等相关维度提供 evidenceRefs
- dodAssessment 必须包含 deliverableCoverage，覆盖 source_change、validation_report、coverage_map、version_control_snapshot、user_deliverable；每项必须有 evidenceRefs 或 artifactRefs
- deliverableCoverage.source_change 标记 satisfied 时，必须引用 patch/apply 或等价状态变更证据，并且该证据的 coverageTargets 与对应 Goal、DoD 或 deliverable 项对齐；只读检查、泛化验证命令、LSP/MCP 只读结果、summary artifact 或 git 快照都不能替代 source_change 证据
- deliverableCoverage.version_control_snapshot 标记 satisfied 时，必须引用 git/write commit 证据；shell/command 形式的 git status、git diff、git log 或 git commit 输出只能作为辅助，不得作为唯一 satisfied evidenceRef
- goalCoverage 必须覆盖 Goal 叶子需求，并说明 functional_behavior、external_contract、runtime_entrypoint、configuration、persistence_or_side_effect、error_behavior、streaming_or_long_running、concurrency、deliverable 等被 Goal / DoD 涉及的验收维度
- 如果项目存在显式入口目录、脚本、命令或构建目标，且 Goal / DoD 声明 CLI、服务、API、单二进制或其他可运行交付物，必须验证目标入口可用；未覆盖入口时不得标记对应条目 satisfied
- 如果 Goal / DoD 要求最终可运行交付物或用户可观察行为，必须通过最终交付入口完成验证，不能只验证库级函数、局部模块或中间适配层后就声称整体 satisfied
- 多外部系统、多接口契约、多协议、多格式或多适配目标必须逐项验证；没有实现或没有测试的目标必须标记 failed/skipped，不得在 summary、goalOutcome 或 implementationSummary 中声称已支持
- 计量、费用、审计、流水账、状态文件或其他副作用必须用测试或命令证据证明写入/追加行为；file/read 只能证明文件当前存在或可读，不能单独支撑 persistence_or_side_effect satisfied
- 对 runtime_entrypoint、external_contract、streaming_or_long_running、error_behavior、persistence_or_side_effect、concurrency 等高风险维度，satisfied 项必须给出直接 evidenceRefs，并且这些 evidenceRefs 必须命中可执行、可验证、可追踪的 runtime action；不能只引用 build/race/coverage_map、只读 file/read/file/list、mcp/resource_read、mcp/prompt_get 或总体 artifact 摘要
- fake/mock/helper 可以作为外部依赖替身或验证辅助，但高风险维度的 satisfied 证据必须绑定真实交付入口或生产实现；不能把测试文件内 fake/mock/helper 作为唯一被测对象或唯一直接证据
- 对外部 API、协议、流式、错误透传、token 统计、持久化或副作用类验收，缺少真实凭证、网络或第三方服务时，优先用本地 deterministic fake/stub server 或 fixture 通过真实交付入口验证；只有 Goal/DoD 明确要求真实外部服务且本地替身无法覆盖合同，才可标记 blocker / skipped
- 若 acceptanceDimensions 中声称上述高风险维度 satisfied，但 evidenceRefs 不能直接追踪到对应验证动作、状态变更动作或等价执行证据，应标记 failed/skipped，并说明缺失的入口验证、行为验证或副作用验证
- 结果声明必须做 claim-to-evidence consistency check：summary、goalOutcome、implementationSummary、verificationSummary、knownLimitations 中不得声称未由代码、测试、构建、patch 或 artifact 证据支撑的库、协议、平台、外部目标、性能、并发或安全能力
- skipped 条目必须说明原因、影响和残余风险
- 不允许只给总体“已完成”而缺少逐项证据
- 测试命令输出 no test files、no tests、0 tests、无测试文件或无有效断言时，只能作为弱证据；除非 DoD 明确允许，否则不得把相关测试条目标记为 satisfied，应补充有效测试/断言或标记 skipped/failed 并说明影响
- 涉及 API、CLI、代理、状态机、配置、存储、审计、流式、并发、错误处理或交付入口时，必须逐项核对主路径、错误路径、入口路径、配置路径、流式/非流式路径、并发路径和持久化路径的证据
- DoD-to-test 映射缺失、关键分支未覆盖或只有占位测试时，应标记 failed/skipped 并列出需要补齐的测试或证据，不要弱化为 satisfied
- assessmentOnly、providerRetryAssessmentOnly 或 assessmentOnlyContractCorrection 为 true 时，只能输出 dodAssessment 或真实治理阻塞；禁止返回 executionGraph、patchRequests、shellCommands、mcpToolCalls、memoryWrites 或其他可执行工作
- 如果证据不足但可由后续自动修复补齐，应返回 failed 的 dodAssessment，并在 failed/skipped 条目的 reason、impact、nextActions 中写明需要补的文件、测试、命令和断言，不要请求用户选择执行目标
- 对于 graph follow-up、completion follow-up 或 latestClosureEvidence 已经明确指出的 coverage gap，若当前上下文允许自动推进，应优先直接给出 bounded repair graph 或 bounded verification graph，而不是停在只有缺口描述的 failed assessment
- 当 failed assessment 已经能机械映射到需要补的实现或测试时，nextActions 必须至少点出目标文件、目标测试和验证命令，避免让 runtime 再次做泛化 repair 归约
- 当 failed assessment 指向 coverageTargets 缺失、只读 graph 不能满足 state-changing、或版本快照证据类型不符时，nextActions 必须明确要求 patch/apply 携带 coverageTargets、或 git/write add/commit 生成版本快照，而不是再次建议泛化重规划
- 当 version_control_snapshot 因引用 shell/command、file/read、file/list、LSP 或其他非 git/write commit 证据被拒绝时，必须将 version_control_snapshot 标记 failed，并在 nextActions 中明确要求先 git/read status/diff，再按实际变更执行 git/write add 和 git/write commit；若确无可提交变更，输出 zeroWork/skipped 及原因
- 如果 runtimeCapabilities.blockKinds 未声明 git/write，不能把 version_control_snapshot 标记 satisfied，也不要建议 shell/command git commit 替代；必须输出真实治理阻塞，说明 WorkOrch runtime capability contract 缺失核心 git/write 能力，要求修复能力声明/接线后继续

## evidenceRefs 提示
- build 证据：shell/command 成功输出
- test 证据：测试结果输出
- lint 证据：lint 命令成功输出或明确的跳过说明
- diff 证据：patch/apply 或 unified_diff 步骤（Step）ID
- 版本控制快照证据：git/write add 与 git/write commit 步骤（Step）ID，优先引用 commit 输出
- 副作用证据：优先引用执行测试/命令或写入 patch；不要只引用 file/read
- 直接 evidenceRefs：优先引用最终入口命令、针对性测试命令、真实 patch/apply、mcp/tool_call 或其他可追踪 runtime action；不要用 summary 文本替代直接证据

## 禁止
- 不要在测试失败、证据缺失、弱测试证据或 DoD 未覆盖时标记整体 satisfied
- 不要在 Goal 覆盖缺失时标记整体 satisfied
- 不要在 assessment-only 验证中规划新的 executionGraph；执行修复由 runtime repair 阶段完成
