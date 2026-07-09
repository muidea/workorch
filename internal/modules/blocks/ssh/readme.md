# SSH Block

## 功能定位

`blocks/ssh` 是 runtime remote command/session block。它负责通过结构化 target/session/credential refs 执行远程命令，并返回规范化 SSH result。

## 拥有边界

- 拥有 SSH runtime adapter、timeout/resource guard 与 runtimeevent executor。
- 只处理远程执行技术能力。
- 不拥有远程环境业务状态、Run 状态或 Result 判断。

## 主要交互

- 接收 `kernel/orchestrator` 派发的 `ssh/command` execution request。
- `blocks/llm` 可在 execution graph 中声明 SSH step。
- SSH 输出由 `kernel/orchestrator` 分类为 evidence candidate、diagnostic 或 blocker。
- 远程高风险副作用应通过 runtime policy、governance 与 approval 流程准入。

## 组件交互合同

- Input contract: request 必须包含 target/session/credential refs、command、timeout/policy、workspace/run/step scope。
- Output contract: 返回 normalized remote exit status、stdout/stderr summary、raw ref、target ref 和 error class。
- Orchestrator contract: SSH result 只作为 evidence candidate 或 blocker；coverage/result 由 orchestrator gate 判定。
- Approval contract: remote side effect 需要明确准入，缺 credential/permission 应返回 structured blocker。
- LLM contract: LLM 只能声明 SSH candidate，不能把远程输出直接变成 accepted Result。

## 已知缺口

- 当前盘点未发现 SSH block 职责缺失；credential/permission blocker 与 remote side-effect policy 需要持续 pairwise 验证。

## 边界约束

- target、session、credential 必须使用结构化引用。
- remote side effect 必须有明确 stage contract admission。
- 远程输出必须经过 evidence binding 才能用于 coverage。
