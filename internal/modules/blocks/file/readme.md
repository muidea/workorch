# File Block

## 功能定位

`blocks/file` 是 runtime 文件能力 block。它负责 bounded file list/read/edit 以及 patch apply/preview/revert 等文件系统侧效果，并返回规范化 runtime execution result。

## 拥有边界

- 拥有文件类 runtime adapter 与 runtimeevent executor。
- 只执行被 runtime contract 准入的文件操作。
- 不拥有 Workspace/Run/Result 状态，不决定 coverage 或 DoD 是否满足。

## 主要交互

- 接收 `kernel/orchestrator` 通过 runtimeevent 发来的 file execution request。
- 与 `blocks/llm` 通过 execution graph 中的 step kind 间接协作。
- 执行结果被 `kernel/orchestrator` 转换为 evidence、coverage 或 diagnostic。
- 与 `blocks/approval` 间接关联：高风险写操作应由 orchestrator/policy 先完成准入或审批。

## 组件交互合同

- Input contract: request 必须包含 step kind、workspace/run/step scope、operation payload 和 path/ref。
- Output contract: 返回 normalized result/error、target refs、summary、raw ref 和 side-effect metadata。
- Orchestrator contract: orchestrator 负责 state-changing admission、evidence quality 和 coverage binding。
- Approval contract: 写入或 patch 操作的风险审批必须在进入 block 前完成或由 block 返回 structured blocker。
- LLM contract: LLM 只能声明 file step candidate，不能绕过 orchestrator 直接调用文件写入。
- EventHub contract: 直接订阅 `/runtime/block` 下对应 file step kind 的 `runtimeevent.ExecuteCommand`，返回 `ExecuteCommandResult`，并向 `/runtime/orchestrator` 投递 `AcceptedData`、`ProgressData`、`CompletedData` 或 `FailedData`；顶层载荷禁止使用 Envelope/map/JSON 转换。

## 已知缺口

- 当前盘点未发现 File block 职责缺失；路径 scope、raw ref 和 large content 截断仍需持续回归。
- 当前未发现与 orchestrator 的 runtime execute 交互合同缺失。

## 边界约束

- 路径必须受 workspace/internal/deliverable scope 约束。
- 大内容应使用 summary/raw ref，不默认塞入投影。
- 文件写入成功不等于业务完成。
