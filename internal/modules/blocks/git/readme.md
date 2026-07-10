# Git Block

## 功能定位

`blocks/git` 是 runtime Git 能力 block。它负责受限的 git read/write 操作，并返回规范化 git result、error、raw ref 与摘要。

## 拥有边界

- 拥有 Git runtime adapter 与 runtimeevent executor。
- 只执行 runtime policy 与 stage contract 准入的 Git 操作。
- 不拥有 Run 状态、Result 状态或任务成功判断。

## 主要交互

- 接收 `kernel/orchestrator` 派发的 `git/read`、`git/write` execution request。
- `blocks/llm` 可在 execution graph 中声明 Git step kind。
- Git 输出由 `kernel/orchestrator` 作为 evidence candidate 或 diagnostic 处理。
- 写操作的风险准入由 runtime policy、orchestrator 和 approval 流程负责。

## 组件交互合同

- Input contract: request 必须包含 git operation、workspace repository scope、run/step scope 和参数。
- Output contract: 返回 normalized git result/error、summary、raw ref 和 changed refs。
- Orchestrator contract: git result 只作为 evidence candidate；completion 由 orchestrator gate 判定。
- Approval contract: write operation 必须由 runtime policy 准入，必要时先创建 approval。
- LLM contract: LLM 只能声明受支持的 git step，不能生成任意 Git CLI 作为绕过路径。
- EventHub contract: 直接订阅 `/runtime/block` 下 git read/write topic 的 `runtimeevent.ExecuteCommand`，返回 `ExecuteCommandResult`，并向 `/runtime/orchestrator` 投递具体的 accepted/progress/completed/failed data；顶层载荷禁止使用 Envelope/map/JSON 转换。

## 已知缺口

- 当前盘点未发现 Git block 职责缺失；受支持 write operation 清单与 prompt 能力说明需要保持同步。
- 当前未发现与 orchestrator 的 runtime execute 交互合同缺失。

## 边界约束

- Git 操作必须限定在 workspace deliverable repository 范围内。
- 不支持把任意 Git CLI 文本当作业务成功判据。
- 不执行未声明或未准入的远程/历史改写类操作。
