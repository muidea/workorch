---
name: workorch-core-template-boundary
description: "在 WorkOrch 开发、评审或排查中涉及 kernel/orchestrator/appservice、workspaceTemplate、Soul、DoD、evidence policy、prompt/context、ticket/approval、runtime block、memory 或 e2e 时使用；用于强制核对 WorkOrch 基础能力与 workspaceTemplate 业务策略边界，避免把具体场景、语言、工具或模板规则写入 core 通用流程。"
metadata:
  version: "1.0.2"
  author: "rangh"
  created_at: "2026-05-09T17:01:44+08:00"
---

# WorkOrch Core / Template Boundary

本技能用于在 WorkOrch 代码开发、代码评审、缺陷排查和需求收口时，先确认“基础能力”和“模板业务策略”的职责边界，再决定修改落点。

## 适用场景

- 修改 `internal/modules/kernel/**`、`internal/modules/blocks/**`、`internal/modules/application/workorchd/biz/appservice/**`、`internal/pkg/repository/**`、`application/workorch` 或 CLI/TUI/Web 入口时。
- 涉及 `workspaceTemplate`、Soul、Goal、DoD、Run、Ticket、Approval、Memory、Artifact、EvidenceQuality、runtime block、LLM prompt/context、e2e 验收时。
- 排查“某个模板或业务场景执行异常”时，特别是软件开发、天气查询、代码审查、文档生成等模板相关问题。
- 发现通用代码中出现具体业务名、模板名、编程语言、命令、外部服务、城市、服务器地址、测试输出 marker 等内容时。

## 核心原则

- WorkOrch core 只提供中立的编排、状态机、事件、治理、证据、上下文传递、运行时调度和质量门禁能力。
- `workspaceTemplate`、Soul、默认 DoD、template-bound policy、Skill、MCP 配置和 e2e 数据承载具体业务策略。
- 通用代码可以消费结构化字段，但不能根据模板名、Soul 名称、业务文本、命令输出文本或具体场景字符串推断行为。
- 如果需要支持某类场景规则，优先把规则做成数据配置，再由通用能力读取数据执行。
- 不为了快速修复在 core 中加入临时兼容、场景特判或 “contains xxx” 逻辑。
- LLM cache 是 core runtime 合同的一部分。任何 prompt/context/request rendering 修改都必须保持 provider-facing 稳定前缀稳定，不能为了当前阶段、repair、provider retry 或某个模板的执行效果污染 C0。
- Runtime 必须维持贯穿 planning、execution、follow-up、repair、validation、acceptance、resume 的强一致 Goal/DoD coverage contract；任何阶段不得丢失当前定义、未关闭缺口、已接受证据、被拒绝证据和下一步收敛约束。
- Memory 是当前决策的参考资产，不是事件日志搬运。无法作为后续决策参考、缺少 provenance、已过期、已被 superseded、纯诊断流水或低价值的 memory 不应进入 LLM 可见上下文。

## 职责边界

WorkOrch core 可以包含：

- 通用状态：Goal、DoD、Run、Step、Ticket、Approval、Memory、Artifact、EvidenceQuality。
- 通用机制：事件发布订阅、异步回调、上下文预算、traceability、质量门禁、repository clone、read model 投影。
- 通用 block 能力：shell/file/web/llm/mcp/approval 等能力的参数执行、结构化输出和错误处理。
- 通用策略解释器：例如读取 `evidencePolicies` 并按统一 schema 标记 `evidenceQuality`。
- 通用 LLM request 分区：C0 稳定语言/Schema 合同、C1 Goal/DoD 定义、C2 声明式能力合同、C3 阶段合同、C4 当前进展/失败/动作、C5 大对象引用。

WorkOrch core 不应包含：

- 具体模板 ID 或 Soul ID 的业务判断，例如 `software-dev`、`weather-query`、`code-review`。
- 具体语言、框架或命令策略，例如只针对 Go、Python、Node、`go test`、`npm test` 的判断。
- 具体业务实体，例如城市、服务器地址、SSH 账号、比赛、某个应用配置。
- 具体输出 marker 的硬编码，例如某测试框架的“未执行测试”文本。
- 只为某个 e2e 场景通过而加入的流程分支。

Template / Soul / policy 可以包含：

- 模板 ID、阶段 Soul、默认 Goal、默认 DoD、业务提示词和执行约束。
- template-bound policy，例如 `workspaceTemplate.evidencePolicies`。
- 特定模板的验收标准、e2e fixtures、示例仓库、场景化测试数据。

## LLM Context / Cache 约束

涉及 `prompt_messages.go`、`http_provider.go`、`sdk_provider.go`、`provider_context_budget.go`、`provider_cache_plan.go`、`runtime_adapter.go`、LLM usage/read model、memory recall、context budget 或 request log 的修改时，必须额外遵守本节。

- C0 只能表达长期稳定的 system prompt、语言规则、输出 JSON schema、字段边界和中立 runtime 合同。
- C0 禁止包含 workspaceId、runId、stepId、Goal/DoD 正文、capability catalog、当前阶段、当前失败、当前进展、memory recall、provider retry、recovery mode、`assessmentOnly`、`stateChangingOutputRequired`、`repairObjective`、`repairActionPackage` 或 requested action。
- `stateChangingOutputRequired`、`repairObjective`、`stateChangingRecoveryMode`、provider timeout retry、assessment-only、requiredOutput / forbiddenActions 等运行期动态约束必须进入 C3 stage contract 或 C4 current-action。
- C1 只能承载当前 Goal/DoD 定义及 definition revision / definitionChangeImpact；Goal/DoD 变化应改变 `workspaceDefinitionHash`，不应改变 `stablePrefixHash`。
- C2 只能承载声明式 runtime capability、governance、Soul/policy 能力合同；当前阶段临时允许或禁止 executable output 不得改写 C2 能力目录。
- C3/C4/C5 承载运行期易变信息，包括 workspace/run identity、stage contract、repair action package、current failure/progress、process evidence timeline、memory recall、tool results、snapshots、trace refs 和 current requested action。
- 同一 run 内仅因 repair/follow-up/assessment/provider retry/state-changing 模式变化导致 `stablePrefixHash` 变化，必须视为 request rendering 缺陷。
- Provider 私有 cache-control 只能由 adapter 基于中立 `cachePlan.adapterHint` 转换；core runtime 不直接写入 provider 私有缓存字段，也不把 provider 私有缓存状态作为业务恢复状态。

## 强一致收敛合同约束

涉及 orchestrator、runtime repair、graph follow-up、completion gate、DoD gate、decision ticket、resume/replan、provider retry 或 LLM step input 的修改时，必须额外遵守本节。

- 每个 LLM step input 必须能重建当前有效 Goal/DoD、definition revision、coverage baseline、unresolved coverage、current progress、failure surface、process evidence timeline、accepted evidence、rejected evidence、requiredOutput / allowedOutputs 和 current requested action。
- Goal/DoD 通过 Web/TUI/API 补充或变更后，必须形成新的 definition revision，并触发后续 run/step 基于最新定义重新评估；不得把新增 Goal 追加成低显著性的历史文本，也不得继续按旧基线完成。
- 新 definition 是 cumulative latest。除非存在显式冲突，新的 Goal/DoD 只能补充或细化，不得弱化旧约束、删除仍有效证据或降低验收目标。
- Repair / follow-up / provider retry / resume 必须继承原失败 step 的阶段语义和收敛合同；decision resolution 只能补充恢复上下文，不能覆盖原 step 的 `runtimeStage`、`runtimeStageMode`、`repairObjective`、`stateChangingOutputRequired` 或 required output。
- `repairActionPackage` 是 primary actionable contract。governanceHistory、memory、旧 executionGraph、旧 rejected patch、旧 decision question 只能作为证据或 lineage，不得与当前 repairActionPackage 竞争。
- 当 runtime 已具备可自动继续的 evidence、capability 和 requiredOutput 时，LLM 返回 `blocked_on_decision` 应先经过中立的 decision ticket gate；不应在可机械修复、可继续验证或可 runtime repair 的场景直接阻塞。
- 上下文门禁失败属于 provider 前准入失败，不能被包装成普通 state-changing repair 交给 LLM 自修；应按 read_required / fail_fast 等结构化 gate action 处理。

## Context / Memory 质量约束

涉及 memory 写入、memory recall、selectedContext、processEvidenceTimeline、currentProgressDigest、request log、context budget 裁剪或摘要时，必须额外遵守本节。

- LLM 可见上下文必须按事件发生顺序、定义修订顺序、工具调用顺序和验证顺序保序；只有无语义顺序的集合才允许按 stable id 排序。
- 裁剪上下文时，禁止裁掉当前 Goal/DoD、requiredOutput、current failure、latest actionable diagnostics、target snapshot hash、accepted/rejected evidence、processEvidenceTimeline 中的关键 sourceRef。
- 脱敏和摘要不能改变语义。被 redacted/truncated 的内容不得被标记为可用于 complete file_write / exact patch 的安全快照。
- Memory 写入必须包含可追溯 sourceRef、类型、用途、confidence/importance 或等价质量字段；不能把 provider retry 流水、泛化失败摘要、无行动价值的日志片段写成可召回 memory。
- Memory recall 只能注入当前 retrieval 选中的高价值参考；excluded、stale、superseded、diagnostic-only、低置信或低重要度 memory 只能保留审计摘要，不能进入 LLM 正文。
- 如果 LLM 持续扩图、重复检查或只修测试/表象，优先核对上下文是否缺少当前真实进展、已存在内容、失败诊断、目标资源快照、未关闭 coverage 和已拒绝方案，而不是在 core 中加入场景特判。

## 工作流程

1. 先定位需求属于基础能力还是模板策略。
   - 如果多个模板都应复用，优先放在 core 通用能力。
   - 如果只对某类 workspaceTemplate 生效，优先放在 template/Soul/policy/e2e 数据。
   - 如果不确定，默认不要写入 kernel/orchestrator，先抽象成结构化数据和通用解释器。

2. 修改前执行边界搜索。
   - 使用 `rg` 检查目标通用目录是否已有或即将加入场景字符串。
   - 对涉及证据、prompt、DoD、ticket、approval 的修改，检查是否有按文本内容推断业务语义。
   - 对涉及 LLM request/context/cache 的修改，检查动态运行期字段是否进入 C0，检查 provider 私有 cache 字段是否进入 core。
   - 对涉及 repair/follow-up/resume/definition change 的修改，检查是否仍能继承最新 Goal/DoD、coverage、requiredOutput、accepted/rejected evidence 和 current requested action。

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "stateChangingOutputRequired|repairObjective|providerTimeoutStateChangingRetry|assessmentOnly|runtimeRepairFollowUp|currentProgressDigest|memoryRecall|Cache-Control|cache_control" internal/modules/blocks/llm internal/modules/kernel
rg -n "definitionChangeImpact|coverageTargetSet|repairActionPackage|processEvidenceTimeline|requiredOutput|blocked_on_decision|contextGate" internal/modules/kernel internal/modules/blocks/llm
```

3. 选择正确落点。
   - 通用字段和状态：放在 `internal/pkg/common`、kernel model、repository、read model。
   - 通用执行与门禁：放在 orchestrator 或 block 通用解释器。
   - 模板策略：放在 `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`、`souls.json`、Skill/MCP 配置或 e2e fixture。
   - workspace 创建期绑定：模板数据需要随 workspace 固化时，创建 workspace 时从 template 拷贝到 workspace，运行期只读取 workspace 绑定数据。

4. 修改中保持数据化。
   - 使用结构化字段，例如 `evidenceQuality`、`zeroWork`、`evidenceTags`、`policyId`。
   - 避免在通用代码里解析自然语言业务结论。
   - 避免通过 Soul ID、template ID、workspace title 推断功能分支。

5. 修改后做反向核对。
   - 搜索通用目录中的业务词、模板词、语言词和命令词。
   - 确认 template 专项内容只出现在 `templates.json`、`souls.json`、template/e2e 测试或专用 fixture。
   - 确认新增单测至少覆盖：通用解释器、template 数据绑定、非目标模板不被影响。
   - 如果修改了 LLM request rendering，新增或更新 cache 相关回归测试，至少证明动态阶段字段不会改变 C0 `stablePrefixHash`。
   - 如果修改了 Goal/DoD、repair、follow-up、resume、memory 或 context budget，新增或更新回归测试证明最新定义、未关闭 coverage、requiredOutput 和关键 evidence 未丢失且顺序正确。

## 排查流程

1. 先复盘异常来自哪个层级：template 数据、workspace 绑定数据、run executionContext、block 输出、orchestrator 门禁、read model 展示。
2. 如果通用层误判业务语义，优先删除通用层判断，并改为结构化 metadata 或 template-bound policy。
3. 如果 template 数据未生效，检查创建 workspace 时是否已绑定到 workspace；不要在激活 run 时按模板名临时补策略。
4. 如果 e2e 依赖某个具体场景，测试名和 fixture 可以具体，但断言应证明通用能力消费的是结构化字段或模板数据。

## 验证清单

- `go test ./... -count=1` 通过，或至少先跑受影响包再跑全量。
- 通用目录搜索没有新增不该出现的具体业务/模板/语言/命令特判。
- 新增模板策略能从 template 创建期绑定到 workspace，并能在 run executionContext 中追溯。
- Web/TUI/CLI 只展示业务模型字段，不复制后端的业务推断逻辑。
- 文档同步说明策略归属：core 通用能力、template 数据、Soul 约束或 e2e fixture。
- 涉及 LLM context/cache 时，必须补充或更新 `docs/workorch-llm-request-cache-aware-context-design.md` / `docs/workorch-runtime-stage-contract-neutrality-design.md` 中的对应约束。
- 涉及 Goal/DoD 变更、repair/follow-up/resume、memory/context 裁剪时，必须证明 LLM request 中仍包含完整收敛合同和可追溯 evidence。

## LLM Cache 验证清单

- `go test ./internal/modules/blocks/llm/biz -count=1` 通过。
- 若修改 request log / usage / read model，补跑相关 appservice 或脚本验证。
- 对历史或新 workspace request log 使用 `./scripts/verify-llm-cache-aware-request-log.sh <workspaceId>`；历史旧日志可作为问题样本，新日志必须满足同一 run 内 `stablePrefixHash` 不随动态阶段变化。
- 单测应覆盖至少一种动态输入变化，例如 `assessmentOnly`、`stateChangingOutputRequired`、provider retry、repair objective 或 Goal/DoD definition revision；其中动态阶段变化不得改变 `stablePrefixHash`，Goal/DoD revision 变化只能改变 `workspaceDefinitionHash`。
- Review diff 时明确确认 C0/C1/C2/C3/C4/C5 归属，没有把当前 failure/progress/memory/repair guidance 拼进稳定前缀。

## Formatter

- `SKILL.md` / Markdown / YAML: 保持标题、列表和代码块格式稳定；归档前运行 `skill-hub validate workorch-core-template-boundary --links`。
- `scripts/`: 当前技能未包含脚本；新增脚本时，必须在本段补充项目可运行的具体 formatter 命令。
- Go 代码示例或辅助脚本使用 `gofmt -w <files>`；Shell 脚本至少保持 Bash 语法一致并运行可用的语法检查。
- 不要声明当前项目无法执行的 formatter；如果对应文件类型没有 formatter，明确写出人工格式要求。

## 输出要求

- 开发或排查完成时，明确说明修改落点为什么属于 core 或 template。
- 列出发现并清理的边界偏差。
- 给出执行过的边界搜索命令和测试命令。
- 如果新增或调整 template/Soul/policy，必须说明该策略如何绑定到 workspace、如何进入 run context、如何被通用能力消费。

## 禁止事项

- 禁止在 `kernel/orchestrator`、通用 `appservice` 或 repository 中写入具体模板、业务、语言、命令输出 marker 的特判。
- 禁止用“先兼容一下”的方式绕过 template/Soul/policy 归属。
- 禁止让 TUI/Web/CLI 形成另一套业务决策逻辑。
- 禁止把 e2e fixture 中的业务文本复制到 core 作为判断依据。
- 禁止把运行期动态字段、repair guidance、provider retry guidance、当前失败/进展、memory recall 或 requested action 拼入 C0 stable prompt。
- 禁止为了缓存命中删除 Goal/DoD、requiredOutput、failure evidence、target snapshot、process evidence timeline 等 LLM 必需上下文；缓存优化必须通过分区稳定性和有序裁剪实现。
- 禁止把旧 Goal/DoD、旧 graph、旧 memory 或旧 decision resolution 当作当前权威合同，覆盖最新 definition revision。
- 禁止把可自动继续的 validation/repair gap 直接转成人工 ticket，除非结构化 blocker 表明缺少 runtime 无法获得的外部信息、权限、凭证、审批或配置。
