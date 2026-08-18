# Repository Provider Initiator

Status: active

Type: initiator-contract

Last Verified: 2026-08-18

## 功能范围与边界

`internal/initiators/workorchruntime` 注册为 `workorch/repositories`，负责根据运行配置创建 memory 或 file repository，并按组件职责提供 repository 分组。它只承担 repository 基础设施的创建、暴露和生命周期清理，不管理业务状态、Runtime 策略或跨 owner 流程。

## 组件交互及合同

- 应用入口通过 side-effect import 显式启用本 initiator。
- 各状态 owner 通过 `internal/pkg/workorchdeps` 获取职责对应的 repository 分组；当前生产调用方均只使用对应 owner 分组。`workorchdeps` 的 `consumer` 参数仅用于错误上下文，不做调用方权限校验，因此该隔离是架构约束和生产调用事实，并非 API 强制隔离。
- `Setup` 必须完成配置读取和全部 repository 创建后才发布 provider；首次或重复 `Setup` 失败都必须 fail-closed，不得保留半初始化或上一次 Setup 的旧 provider。
- `RepositoryProvider` 在成功 `Setup` 前和 `Teardown` 后返回 `nil`；就绪时只返回按职责划分的 repository 接口，不暴露内部 `repositorySet`。
- `Run` 不创建业务状态；`Teardown` 只释放 provider 引用。

## 交互合同完整性

| 交互方 | 方向 | 合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorch` 入口 | 入口 → Initiator | framework 注册与 `Setup/Run/Teardown` 生命周期 | 完整；启用权只在入口 |
| Module / Block | consumer → Initiator | `internal/pkg/workorchdeps` 的按 owner repository 分组 | 完整；不返回内部 `repositorySet`，owner 隔离由架构约束和生产扫描保证 |
| EventHub | 无 | 不定义 topic、Command、Data 或 Result | 完整；Initiator 不参与业务事件 |

## 相关约束

- initiator 不得导出 EventHub、BackgroundRoutine、runtime policy、owner service 或业务 facade。
- initiator 不得执行业务状态转换、默认资源初始化或跨组件编排。
- 组件启用权只属于应用入口，不能依赖业务包的间接 import。
- file repository 构造器提供惰性存储句柄，`Setup` 不预探测路径可写性；实际 I/O 错误由执行该存储操作的 repository 返回，不扩张 Initiator 的初始化职责。

## 测试状态

- [x] memory/file 两种 backend 的 12 组 typed repository 合同均可取得完整实现。
- [x] 成功 `Setup` 才发布完整 provider；非法配置和未知 backend 均会 fail-fast，失败的重复 Setup 会先撤销旧 provider。
- [x] 重复 `Run` 无业务动作且保持 provider 不变；`Teardown` 清空 provider 引用，重复调用不产生残留状态。
- [x] 生产 AST 边界扫描确认 12 个 owner 只请求各自 repository 分组，provider 解析只经 `workorchdeps`，实现包只由入口启用。
- [x] 导出面 AST 扫描固定 Initiator 私有存储、12 组 repository 字段和 provider 接口方法集，且不允许导出或留存 EventHub、Runtime policy、owner service 或 `repositorySet`。

当前没有可执行的待完善测试项。现有 memory/file repository 构造器均不返回错误，`buildRepositories` 也没有可注入工厂，因此“任一 repository 构造失败”不是当前可直接触发的测试分支，也不列为测试缺口；只有引入可失败构造器或工厂注入后，才应补充该失败路径测试。
