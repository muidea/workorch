# Repository Provider Initiator

## 功能说明

`internal/initiators/workorchruntime` 注册为 `workorch/repositories`，负责根据运行配置创建 memory 或 file repository，并按组件职责提供 repository 分组。它只承担 repository 基础设施的创建、暴露和生命周期清理，不管理业务状态或业务策略。

## 组件交互及合同

- 应用入口通过 side-effect import 显式启用本 initiator。
- 各状态 owner 通过 `internal/pkg/workorchdeps` 获取自己的 repository 分组；Workspace、Run、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance、Eventstream 和 Orchestrator 不能取得其他 owner 的 repository。
- `Setup` 必须完成配置读取和全部 repository 创建；配置非法、backend 不支持或 repository 创建失败时必须 fail-fast。
- `RepositoryProvider` 只返回按职责划分的 repository 接口，不暴露内部 `repositorySet`。
- `Run` 不创建业务状态；`Teardown` 只释放 provider 引用。

## 相关约束

- initiator 不得导出 EventHub、BackgroundRoutine、runtime policy、owner service 或业务 facade。
- initiator 不得执行业务状态转换、默认资源初始化或跨组件编排。
- 组件启用权只属于应用入口，不能依赖业务包的间接 import。
