# WorkOrch Runtime Lineage and Assessment Contract

## 背景

WorkOrch runtime 的恢复链路需要同时支持 provider retry、runtime repair、graph follow-up、assessment-only correction 和最终 DoD result。近期执行暴露出两个核心问题：

- step ID 过度拼接祖先 ID，导致 `provider-retry-1-provider-retry-1` 这类字符串形态，混淆真实 lineage。
- assessment-only correction 继承 provider retry / recovery 上下文，并在输入中残留可执行 graph，导致 correction 继续输出 `executionGraph`。

## 设计原则

1. ID 与 lineage 分离
   - step ID 只做短稳定标识，不能承载完整历史链路。
   - 完整因果关系通过结构化字段表达：`recoveryRootStepId`、`providerRetryRootStepId`、`recoveryBudgetScope`、`assessmentOnlyContractViolationStepId`。

2. State-changing graph 准入
   - `stateChangingOutputRequired=true` 可返回顶层 `patchRequests` / `patches` / `nextKind=patch/apply`。
   - 多步骤依赖可返回 `executionGraph`，但 graph 必须包含合法 `patch/apply`。
   - read-only / inspection-only / shell-only graph 继续视为 contract failure。

3. Assessment-only 绝对只读
   - assessment-only 和 assessment-only correction 不允许携带或返回 executable fields。
   - correction 输入不得继承 provider retry / runtime repair / recovery markers。
   - correction 输入必须递归清理嵌套 `executionGraph`、`patchRequests`、`shellCommands`、`mcpToolCalls`、`nextStep` 等字段。

4. Provider retry root 归一
   - retry budget 只按结构化 root 统计。
   - step ID 不参与 root 推断，脚本只把 ID 作为异常形态检测。

5. 验证脚本契约
   - 核对 recovery lineage 字段是否存在。
   - 核对 provider retry ID 是否出现嵌套形态。
   - 核对 state-changing graph 是否包含合法 `patch/apply`。
   - 核对 terminal / blocker result policy。

## 收口项

- LAC-01：已落地。follow-up / correction / retry / repair step ID 使用短稳定 token，不拼接完整祖先 step ID。
- LAC-02：已落地。state-changing repair guidance 与 runtime contract 对齐，允许含 `patch/apply` 的 graph。
- LAC-03：已落地。assessment-only correction 输入去除 provider retry、runtime repair、recovery 和 state-changing markers。
- LAC-04：已落地。assessment-only sanitizer 递归清理所有 executable fields，包括嵌套 `llmDecision.executionGraph`。
- LAC-05：已落地。补充测试覆盖 ID、correction 输入、递归清理和 state-changing graph 准入。

## 验收点

- step ID 只允许包含 run 前缀、短 token 和本地语义后缀；不得重新嵌入完整 ancestor step ID。
- repeated graph node 在不同 graph root 下必须生成 token-scoped step ID，不能复用 base ID，也不能拼接 root 的完整字符串。
- assessment-only correction 输入不得含 provider retry、runtime repair、recovery、state-changing markers。
- assessment-only correction 输入不得含顶层或嵌套 executable fields。
- state-changing repair 可输出 `executionGraph`，但 graph 必须至少包含一个合法 `patch/apply` 节点。
- provider retry step ID 和 budget root 必须来自结构化 root，不得通过 ID 字符串继续派生 retry root。
