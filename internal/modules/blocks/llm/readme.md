# LLM Block

## 功能定位

`blocks/llm` 是 runtime LLM provider 与 context rendering block。它负责 provider 配置加载、请求渲染、cache prefix 分区、context budget、流式/非流式 provider 调用、schema/parse diagnostic 与规范化 LLM candidate 输出。

## 拥有边界

- 拥有 LLM provider adapter、prompt/context assembly 的 block 侧实现和 runtimeevent executor。
- 不拥有 runtime state transition；LLM 输出必须由 `kernel/orchestrator` contract gate 判定。
- 不内置具体业务领域策略。

## 主要交互

- 接收 `kernel/orchestrator` 派发的 `llm/workspace_execution` 请求。
- 读取 `blocks/workspace`、`kernel/run`、`blocks/memory`、`blocks/mcp-registry`、`blocks/skill`、`blocks/soul` 等经 orchestrator 组织后的上下文字段。
- 生成 executionGraph、assessment、artifact plan/chunk 等候选输出供 `kernel/orchestrator` 校验。
- 向 `kernel/eventstream` 和 runtime progress 链路提供 provider usage、stream、cache usage 等观测数据。

## 组件交互合同

- Input contract: request 必须包含 stage packet、requiredOutput、Goal/DoD、critical evidence refs、runtime capabilities 和 context budget。
- Output contract: 返回 parsed candidate、provider diagnostic、schema violation 或 structured blocker；不得直接提交 owner state。
- Cache contract: C0/C1/C2/C3-C5 分层必须稳定，volatile runtime details 不得污染 stable prefix。
- Orchestrator contract: orchestrator 是唯一 contract gate；LLM block 不决定 evidence accepted 或 Result accepted。
- Event contract: provider usage、stream、cache usage 和 failure diagnostic 必须以 bounded payload/raw ref 发布。
- EventHub contract: 直接订阅 `/runtime/block` 下 workspace execution topic 的 `runtimeevent.ExecuteCommand`，返回 `ExecuteCommandResult`，并向 `/runtime/orchestrator` 投递具体的 accepted/progress/completed/failed data；顶层载荷禁止使用 Envelope/map/JSON 转换。

## 已知缺口

- 当前盘点未发现 LLM block 职责缺失；但 cache prefix 稳定性、critical field protection、large output mode 是高风险合同，修改时必须补专项测试。
- 当前未发现与 orchestrator 的 runtime execute 交互合同缺失。
- X05 已关闭：LLM block 不再导入 file/git/mcp/shell/ssh/web sibling block 的 `pkg/common`；所有 step kind 统一引用 `internal/pkg/workorchruntimecontract` 的 owner-neutral 常量。
- C18 粒度复核已完成：provider adapter、embedding、request rendering、context budget 与 cache planning 都服务于同一个 LLM 调用生命周期，当前已按 focused file/package 分离；它们没有独立 framework 生命周期或状态权威，不再拆分新的 block。

## 边界约束

- provider-facing cache prefix 必须保持 C0/C1/C2/C3-C5 分层稳定。
- context budget 不能丢失当前 Goal/DoD、requiredOutput、latest failure、critical evidence 或 target snapshot refs。
- provider retry 和 schema retry 不得直接修改 owner state。
- capability kind 只能来自 owner-neutral runtime contract 或 orchestrator request，禁止重新依赖其他 block 的实现包或 `pkg/common`。
