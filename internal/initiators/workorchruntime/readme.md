# Repository Provider Initiator

Status: active

Type: initiator-contract

Last Verified: 2026-08-17

## 功能范围与边界

`internal/initiators/workorchruntime` 注册为 `workorch/repositories`，负责根据运行配置创建 memory 或 file repository，并按组件职责提供 repository 分组。它只承担 repository 基础设施的创建、暴露和生命周期清理，不管理业务状态、Runtime 策略或跨 owner 流程。

## 组件交互及合同

- 应用入口通过 side-effect import 显式启用本 initiator。
- 各状态 owner 通过 `internal/pkg/workorchdeps` 获取自己的 repository 分组；Workspace、Run、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance、Eventstream 和 Orchestrator 只能取得各自分组。
- `Setup` 必须完成配置读取和全部 repository 创建；配置非法、backend 不支持或 repository 创建失败时必须 fail-fast。
- `RepositoryProvider` 只返回按职责划分的 repository 接口，不暴露内部 `repositorySet`。
- `Run` 不创建业务状态；`Teardown` 只释放 provider 引用。

## 交互合同完整性

| 交互方 | 方向 | 合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorch` 入口 | 入口 → Initiator | framework 注册与 `Setup/Run/Teardown` 生命周期 | 完整；启用权只在入口 |
| Module / Block | consumer → Initiator | `internal/pkg/workorchdeps` 的按 owner repository 分组 | 完整；不返回内部 `repositorySet` |
| EventHub | 无 | 不定义 topic、Command、Data 或 Result | 完整；Initiator 不参与业务事件 |

## 相关约束

- initiator 不得导出 EventHub、BackgroundRoutine、runtime policy、owner service 或业务 facade。
- initiator 不得执行业务状态转换、默认资源初始化或跨组件编排。
- 组件启用权只属于应用入口，不能依赖业务包的间接 import。

## 待完善测试点

- [ ] 覆盖 memory/file 两种 backend 的分组可见性，验证每个 owner 只能取得自己的 repository 集合。
- [ ] 覆盖未知 backend、缺失配置和任一 repository 构造失败，确认 `Setup` fail-fast 且不发布半初始化 provider。
- [ ] 覆盖重复 `Run`、重复 `Teardown` 和 `Setup` 失败后的清理，确认生命周期幂等且无悬挂引用。
- [ ] 增加生产边界扫描，确认 Initiator 未导出 EventHub、Runtime policy、owner service 或 repository set。
