# LLM Block

## 功能说明

`blocks/llm` 提供 LLM provider 配置、请求渲染、context budget、cache prefix 规划、流式/非流式调用、embedding、schema/parse diagnostic 和 candidate 规范化能力。

## 组件交互及合同

- `kernel/orchestrator` 使用 `internal/pkg/runtimeevent.ExecuteCommand` 派发 workspace execution，并接收 `ExecuteCommandResult` 与 runtime progress 通知。
- provider completion 与 embedding 的直接调用由 `blocks/llm/pkg/events` 定义 `CompleteProviderCommand`、`GenerateEmbeddingCommand` 和 `LLMCommandResult`。
- 输入必须包含当前 stage contract、requiredOutput、Goal/DoD、关键 evidence refs、runtime capability 和 context budget。
- 输出只能是 candidate、provider diagnostic、schema violation 或 structured blocker；Orchestrator 负责 contract gate 和状态推进。
- provider usage、stream、cache usage 和 failure diagnostic 必须以有界字段或 raw ref 进入通知链。

## 相关约束

- provider-facing cache prefix 必须保持 C0、C1、C2、C3-C5 分层稳定。
- context budget 不得丢失 Goal/DoD、requiredOutput、latest failure、critical evidence 和 target snapshot refs。
- 本 block 不拥有 runtime 状态，不得直接写 Run、Result、Memory 或 EvidenceRecord。
- capability kind 只能来自 owner-neutral runtime contract 或 Orchestrator request，不得导入 sibling block 实现常量。
- provider retry、schema retry 和 parse diagnostic 不得绕过 Orchestrator 修改 owner 状态。
