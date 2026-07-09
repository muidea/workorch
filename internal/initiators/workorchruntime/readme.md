# workorchruntime Initiator

## 功能定位

`workorch/runtime` 是 WorkOrch 进程级运行时基础设施 initiator。它负责在 magicCommon plugin 生命周期中初始化 repository provider、WorkOrch EventHub owner wrapper 和 runtime policy，并把这些基础能力暴露给后续加载的 modules。

## 拥有边界

- 拥有 repository provider、EventHub helper、runtime policy 的装配与生命周期清理。
- 不拥有 Workspace、Run、Result、Decision、Approval 等正式业务状态。
- 不包含具体业务策略、profile 策略或运行阶段决策。

## 主要交互

- 被 `application/workorch` 入口显式 side-effect import 后注册。
- 为 `application/workorchd`、`kernel/*`、`blocks/*` 提供统一 EventHub 与 repository 依赖。
- 通过 `internal/pkg/workorchdeps` 向 application bootstrap 提供窄接口解析能力。
- 与 `internal/pkg/workorchruntimepolicy` 协作加载 runtime policy。

## 组件交互合同

- Provider contract: `Setup` 完成后必须能通过 `workorchdeps` 解析 repository provider、EventHub 与 runtime policy。
- Consumer contract: 下游 module 只能通过 `workorchdeps` 或构造参数读取基础设施，不能持有 initiator 内部结构。
- Lifecycle contract: `Run` 不创建业务状态；`Teardown` 必须清理 repository provider、EventHub wrapper 和 runtime policy。
- Failure contract: repository、config、EventHub helper 缺失时必须 fail-fast，不能让下游 module 运行到 nil dependency。

## 已知缺口

- `application/workorchd` 的 borrowed exports 当前依赖 `ExportsFromInitiator`，其中 `CoverageWorkItems` 导出链路需要补齐并增加边界测试。

## 边界约束

- 依赖缺失必须在 initiator/helper 边界 fail-fast。
- 不导出跨 owner service facade。
- 不能通过隐式业务 import 触发启用；启用权只属于入口装配层。
