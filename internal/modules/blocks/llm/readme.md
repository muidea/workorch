# LLM 能力组件

## 功能说明

`blocks/llm` 提供 LLM provider 配置、typed `PromptContext` 渲染、稳定 cache prefix 规划、provider ceiling 校验、流式/非流式调用、embedding，以及 Runtime V2 candidate 的严格解码和合同校验能力。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `RequestRuntimeCandidateCommand/Result` 同步请求 Runtime V2 candidate；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- provider completion 与 embedding 的直接调用由 `blocks/llm/pkg/events` 定义 `CompleteProviderCommand`、`GenerateEmbeddingCommand` 和 `LLMCommandResult`。
- Runtime V2 provider ceiling 和 LLM capability prerequisite 使用 Orchestrator 定义的具体 EventHub command/result；ceiling 由当前 provider context profile 与 inherited ceiling 的较小值确定。
- Runtime V2 candidate request 只携带 sealed `RuntimeDecision`、`AuthorityTransaction`、Run owner 已持久化的 typed 定义上下文和 candidate attempt；本 block只能返回与当前 action 对应的 `StateChangeCandidate`、`ValidationCandidate`、`AssessmentCandidate` 或 `GovernanceCandidate`，不能返回 convergence、focus、authority 或 repair objective。
- provider request 不包含自由 `Input map[string]any`、Runtime V1 execution decision、repair request 或 capability map；C0/C1/C2 直接由 typed `PromptContext` 渲染为稳定前缀，C3 及以后只进入动态消息。
- C1 definition context 包含 typed definition change impact；累积定义 identity、revision lineage 与 strategy requirements 属于稳定定义事实，本 block 不从历史 execution-context map 或 governance 文本重建。
- C2 capability contract 同时包含 capability kinds 和 typed `RuntimeExecutionContract`；state-change policy、capability usage policy、profile acceptance/deliverable/evidence contract 或 template stage policy 变化必须改变 stable prefix hash，不能只作为 C3 动态 tail 文本。
- provider、schema、contract 和 internal failure 使用具体 `RuntimeCandidateFailure` 返回；本 block 不在进程内决定是否重试，attempt 预算与持久化状态由 Orchestrator controller 管理。
- 输出只能是当前 action 对应的 typed candidate 或具体 provider/schema/contract failure；Orchestrator 负责 retry disposition、contract gate 和状态推进。

## 相关约束

- provider-facing cache prefix 必须保持 C0、C1、C2、C3-C5 分层稳定。
- context budget 不得丢失 Goal/DoD、requiredOutput、latest failure、critical evidence 和 target snapshot refs。
- 本 block 不拥有 runtime 状态，不得直接写 Run、Result、Memory 或 EvidenceRecord。
- capability kind 只能来自 owner-neutral runtime contract 或 Orchestrator request，不得导入 sibling block 实现常量。
- provider、schema 和 contract failure 不得绕过 Orchestrator 修改 owner 状态；LLM block 不保留 Runtime V1 `RuntimeAdapter`、execution decision parser 或本地 repair 主链。
- 同一 authority transaction 最多一个初始 candidate request 和一个 same-contract retry；第二次仍无效时不得生成第三次请求或切换 action。
- attempt 1 必须保持 transaction、requiredOutput hash 和 stable prefix 不变；typed failure 不得通过普通 EventHub error 伪装成新的 recovery authority。
- definition context revision 必须与 authority transaction 一致；LLM block 不查询或拼装另一份 Goal/DoD 定义，也不得接受缺失正文或 capability contract 的请求。
- profile contract 只作为 typed C2 输入描述验收维度、交付期望和证据要求；本 block 不把 required deliverable 自行降维为普通 coverage focus，也不裁决 accepted step kind/operation 是否满足交付合同。
- assessment candidate 只能声明当前 focus 的 satisfied/failed、summary 与 evidence/artifact refs；不能夹带 execution graph、operation、下一 convergence state 或 Result owner 写入指令。
- template stage policy 只以 typed stage/entry/required-output/governance/policy-ref 结构进入 C2；本 block 不根据 template ID、Soul ID、业务术语或自然语言重建阶段策略。
