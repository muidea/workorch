# Shell Block

## 功能定位

`blocks/shell` 是 runtime shell command execution block。它负责在受控策略下执行本地 shell 命令，并返回 exit code、stdout/stderr 摘要、raw ref 和错误分类。

## 拥有边界

- 拥有 shell runtime adapter、timeout/resource guard 与 runtimeevent executor。
- 只执行 runtime contract 和 policy 准入的命令。
- 不拥有 Run/Result/Memory 状态，不从 shell 文本直接推导业务完成。

## 主要交互

- 接收 `kernel/orchestrator` 派发的 `shell/command` execution request。
- `blocks/llm` 可在 execution graph 中声明 shell step。
- 输出由 `kernel/orchestrator` 作为 validation evidence、diagnostic 或 blocker 处理。
- 高风险命令应通过 `kernel/approval` 或 governance flow 完成授权。

## 组件交互合同

- Input contract: request 必须包含 command、working directory/ref、timeout/policy、workspace/run/step scope。
- Output contract: 返回 exit code、stdout/stderr bounded summary、raw ref、duration 和 error class。
- Orchestrator contract: shell output 只作为 evidence input；evidence accepted/rejected 由 orchestrator 判定。
- Approval contract: destructive/background/unbounded command 必须由 policy 拒绝或 approval-gated。
- LLM contract: LLM 只能声明 shell candidate，不能把 shell 文本直接标记为 DoD satisfied。
- EventHub contract: 直接订阅 `/runtime/block` 下 shell command topic 的 `runtimeevent.ExecuteCommand`，返回 `ExecuteCommandResult`，并向 `/runtime/orchestrator` 投递具体的 accepted/progress/completed/failed data；顶层载荷禁止使用 Envelope/map/JSON 转换。

## 已知缺口

- 当前盘点未发现 Shell block 职责缺失；long-running command 和 zero-work evidence policy 仍需要持续回归。
- 当前未发现与 orchestrator 的 runtime execute 交互合同缺失。

## 边界约束

- 命令必须 bounded，不能默认后台长期运行。
- stdout/stderr 是 evidence input，不是直接业务真相。
- shell block 不能创建 accepted Result 或长期 memory。
