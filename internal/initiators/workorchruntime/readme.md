# Repository Provider Initiator

## 功能定位

`internal/initiators/workorchruntime` 注册 ID 为 `workorch/repositories` 的单一 repository provider initiator。它只根据配置创建 file/memory repository set，并向各 owner module 提供按职责分组的 repository provider。

## 拥有边界

- 只拥有 repository provider 的基础设施装配与生命周期清理。
- 不拥有 Workspace、Run、Result、Decision、Approval 等正式业务状态。
- 不包装或导出 framework EventHub，不保存 runtime policy，不包含具体业务策略、profile 策略或运行阶段决策。

## 主要交互

- 被 `application/workorch` 入口显式 side-effect import 后注册。
- 通过 `internal/pkg/workorchdeps` 只向 kernel owner module 提供其自身 repository 分组。
- `application/workorchd` 与 `internal/services/workorch` 直接使用 magicCommon application/framework EventHub，不再依赖本 initiator。
- `kernel/orchestrator` 通过无状态 `workorchruntimepolicy.Current()` 读取 runtime policy。

## 组件交互合同

- Provider contract: `Setup` 完成后必须能通过 `workorchdeps` 解析 repository provider；每个 owner 只能获取自己的 repository 分组。
- Consumer contract: 下游 module 只能通过 `workorchdeps` 读取 repository 基础设施，不能持有 initiator 内部 repository set。
- Lifecycle contract: `Run` 不创建业务状态；`Teardown` 只清理 repository provider handles。
- Failure contract: repository/config 缺失时必须 fail-fast，不能让下游 module 运行到 nil dependency。

## 已知缺口

- C01/X01 已关闭：initiator 已收窄为单一 repository provider，ID 更新为 `workorch/repositories`；EventHub 与 runtime policy 导出已删除。
- 目录名 `workorchruntime` 仅作为现有内部 Go import path 保留，不代表继续提供聚合 runtime helper。

## 边界约束

- 依赖缺失必须在 initiator/helper 边界 fail-fast。
- 不导出跨 owner service facade。
- 不得重新增加 EventHub、CommandHub、runtime policy、background routine 或其他第二类基础设施导出。
- 不能通过隐式业务 import 触发启用；启用权只属于入口装配层。
