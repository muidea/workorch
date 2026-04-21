# WorkOrch 整理进度跟踪

> 最近更新：2026-04-21

## 当前目标

围绕 `go-multi-module-dev` 规则，持续收敛 `workorch` 的模块边界、应用层职责和代码落点，同时保持 phase1 demo 与测试基线稳定。

## 已完成

### 1. 模块骨架补齐

- 已为以下模块补齐 `module.go`
  - `internal/modules/kernel/workspace`
  - `internal/modules/kernel/run`
  - `internal/modules/kernel/decision`
  - `internal/modules/kernel/orchestrator`
  - `internal/modules/blocks/approval`
  - `internal/modules/blocks/legacy`
- 已为以上模块补齐模块级 `pkg/common/ModuleID`
- 已为缺失模块补齐 `service/` 契约层文件

### 2. 模块名与入口收敛

- `application/workorchd/main.go` 已改为优先依赖模块根包，而不是直接拼接 `biz` 层
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go` 中的 `SourceModule` 已切换为模块常量
- `application/workorchd/demo/scenarios.go` 已切换为优先使用模块根包导出的 command 类型别名，不再直接依赖 `internal/pkg/contracts`

### 3. 应用层拆分

应用层目前已收敛为四层：

- `application/workorchd/main.go`
  - 只负责进程入口和输出循环
- `application/workorchd/bootstrap`
  - 负责 env、backend、repository 与 runtime 装配
- `application/workorchd/demo`
  - 负责场景注册、场景执行和快照组装
- `application/workorchd/presenter`
  - 负责输出渲染

### 4. demo 进一步拆分

`application/workorchd/demo` 已按职责拆成：

- `demo.go`
  - 场景注册与总调度
- `scenarios.go`
  - phase1 场景流转
- `snapshot.go`
  - 快照读取与组装

### 5. 测试补充

- 保留原有 `application/workorchd` 冒烟测试
- 新增 `application/workorchd/bootstrap/runtime_test.go`
  - 覆盖 `memory` backend 组装
  - 覆盖 `file` backend 最小持久化行为
- 新增 `application/workorchd/demo/demo_test.go`
  - 覆盖 `memory` backend 下 4 条 phase1 场景返回结果
  - 覆盖 `file` backend 下场景执行后的持久化文件落地
- 新增 `application/workorchd/demo/scenarios_test.go`
  - 覆盖 blocked run 上重复 start 的冲突路径
  - 覆盖 completed run 上重复 start 的幂等路径
  - 覆盖未阻塞 run 上 resume 的冲突路径
  - 覆盖 decision 未 resolve 时 resume 的 blocked 路径
  - 覆盖 failed run 上重复 resume 的终态幂等路径
  - 覆盖 run 与 workspace 绑定不一致时的冲突路径
  - 覆盖 ticket / approval 绑定不一致时的冲突路径
  - 覆盖 approval 已 reject 后再次 approve 的冲突路径
  - 覆盖 approval 已 approve 后再次 reject 的冲突路径

### 6. bootstrap 能力面收敛

- `application/workorchd/bootstrap.Runtime` 已开始从公开字段集合收敛为显式方法面
- `demo/scenarios.go` 与 `demo/snapshot.go` 已切换为通过 `Runtime` 方法访问 workspace/run/step/ticket/approval 与服务动作
- 应用层对底层 repository / service 字段的直接依赖已显著减少

### 7. bootstrap 职责继续拆分

- `application/workorchd/bootstrap/runtime.go` 已收敛为 `Runtime` 类型与方法面
- `application/workorchd/bootstrap/backend.go`
  - 负责 env 常量、repository backend 选择与 repository 装配
- `application/workorchd/bootstrap/assembly.go`
  - 负责 `Runtime` 组装、runtime adapter、event hub 与各模块 service 接线
- `bootstrap` 包内的 “后端选择 / 装配接线 / 运行时方法” 已不再堆在同一文件中

### 8. 模块内 command 契约收敛

- `internal/modules/kernel/workspace/service/service.go` 已收回 `CreateWorkspaceCommand` 与 `WorkspaceService`
- `internal/modules/kernel/run/service/service.go` 已收回 run 模块全部 command DTO 与 `RunService`
- `internal/modules/kernel/decision/service/service.go` 已收回 decision 模块 command DTO 与 `DecisionService`
- 对应 `module.go`、`biz/*.go` 和单测已切换为优先依赖模块本地 `service` 契约，不再直接依赖 `internal/pkg/contracts`

### 9. orchestrator 契约收尾

- `internal/modules/kernel/orchestrator/service/service.go` 已收回 `RuntimeOrchestrator` 接口定义
- `internal/modules/kernel/orchestrator/module.go` 已补齐模块级 `Service` 导出
- 当前 `application/` 与 `internal/modules/` 范围内，已不存在对 `internal/pkg/contracts` 的直接依赖

### 10. contracts 遗留包清理完成

- `internal/pkg/contracts` 已整体删除
- `README.md`、`docs/phase1-status.md`、`docs/structure.md` 已同步去除过渡期描述
- 共享内部能力当前仅保留 `internal/pkg/{common,events,repository}`

### 11. 应用层 command 感知继续压缩

- `application/workorchd/bootstrap.Runtime` 已进一步收敛为应用级参数面
- `application/workorchd/demo/scenarios.go` 已不再直接构造任何模块 command DTO
- `application/workorchd/demo/scenarios_test.go` 与 `application/workorchd/bootstrap/runtime_test.go` 已切换为通过 `Runtime` 应用级方法验证行为
- 当前 `application/workorchd` 范围内，模块 command DTO 的构造已集中在 `bootstrap` 包内部

## 当前结构判断

目前仓库已经达到以下状态：

- 模块目录结构基本符合 `go-multi-module-dev` 最小骨架
- 应用入口已经从“大文件拼装”演进到“入口 / bootstrap / demo / presenter”分层
- 应用层已不再直接依赖共享 contracts，也不再直接构造模块 command DTO
- `workspace/run/decision/orchestrator` 四个核心模块已具备自有契约层，旧 `contracts` 共享包已完成移除
- demo 逻辑现在已具备包级测试，不再只依赖应用级 smoke test
- bootstrap 作为应用装配层已经开始暴露稳定方法面，而不是直接暴露 repo/service 字段
- bootstrap 已从单文件装配继续收敛为 `backend`、`assembly`、`runtime` 三个职责文件

## 当前未完成

以下工作仍建议继续推进：

1. 继续为 `demo` 中剩余异常分支补测试，例如更多 ticket/approval 绑定异常与终态行为校验

## 本轮基线

当前整理后的验证基线：

```bash
go test ./...
```

最近一次已通过。
