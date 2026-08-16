# 通用任务候选执行

本资源只约束当前 `operations` 文档。候选必须完全落在 sealed manifest 内；真正的执行、回执、证据接纳和状态迁移分别由 capability owner 与 Orchestrator 完成。

## 要求

- 只选择 manifest 已授权的 operation kind/variant、参数、target policy、target 与 `bindingRefs`。
- 每个 operation 的依赖、effect 和 target 必须与对应 authorization 一致。
- 对已有资源使用 owner 声明的 revision-bound replacement；create-only operation 只能用于不存在的新目标。
- 高风险或外部副作用只能使用 manifest 中已经封存治理要求的 operation。
- 只返回一个完整、紧凑且符合 sealed schema 的 JSON object。

## 边界

- 不自报执行成功、evidence credit、assessment 结论或 terminal outcome。
- 不从错误文本扩展授权，不猜测 target/revision，不要求其他组件改变状态。
