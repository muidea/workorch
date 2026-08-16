# 通用任务语义规划

本资源只约束无副作用的 `semantic_plan` 文档。逐个 CWI 建立可验证的 proof proposal，并严格使用请求中已封存的 owner catalog、operation kind/variant、target policy、selector、recipe 与 check。

## 要求

- 每个 required CWI 都必须有且只有一个完整 proposal，并保留其 definition/focus identity。
- operation kind、variant 与 target policy 必须来自同一条 owner catalog fact。
- 已存在目标只能选择 owner 已声明的 replacement operation；新目标只能使用 owner 允许的 prospective selector。
- validation proposal 必须声明 owner 可执行的 recipe/check；不能以摘要、声明或资源存在代替 proof。
- 信息不足时不得猜测 owner、target、operation、recipe 或治理结果。

## 边界

- 不执行工具、不修改资源、不创建 dispatch，也不声明 coverage credit 或终态。
- 不选择 Runtime 状态或后续迁移；只输出当前 sealed schema 允许的完整 JSON 文档。
