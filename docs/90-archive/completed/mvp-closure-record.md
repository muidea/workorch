# workorch MVP Closure Record

## Goal

`workorch` 当前阶段的目标，是把 `workorchd` 做成一个可本地部署、可验证、可集成的编排服务 MVP，而不是继续只扩展预置 `slice` 演示面。

当前已交付能力主要覆盖：

- `slice` 目录与执行
- `slice` 级批量 / 单条快照查询
- `slice` 级批量 / 单条状态查询
- `memory` / `file` 仓储后端
- 健康检查、就绪检查、指标、并发保护

本文件保留 MVP 功能收口记录，不再作为活跃待办清单使用。当前服务能力已从“按预置 `slice` 使用”推进到“按真实业务实例使用”。

## Closure Rule Used

每个功能项收口时，均按以下规则验收：

- 有正式业务接口，而不是只存在内部 helper 或 demo 路径
- 有单元测试或集成测试覆盖主要成功 / 空结果 / 异常分支
- 有本地服务级验证方法，可通过 `go test ./...` 或 `make verify-service*` 证明
- README 或状态文档已同步更新

## Progress

- status: `closed`
- completed: `B1 workspace 实例接口`
- completed: `B2 run 实例接口`
- completed: `B3 run 启动与恢复接口`
- completed: `B4 decision 工单查询与处理接口`
- completed: `B5 approval 工单查询与处理接口`
- completed: `B6 实例读模型补齐`
- completed: `B7 Goal 和 DoD 定义流程`
- completed: `S1 仓储后端共享契约测试`
- completed: `S2 file 后端持久化语义补强`
- completed: `S3 最小事件总线路径正式化`
- active todo: `none`

## P0 Closed Scope

### [x] B1. workspace 实例接口

Status: completed

目标：让外部可以直接创建和读取 `workspace`，而不是只能靠执行预置 `slice` 间接生成。

完成内容：

- 增加 `workspace` 业务读写模型
- 增加创建 `workspace` 接口
- 增加 `workspace` 单条查询接口
- 增加 `workspace` 列表接口
- 明确重复创建、缺失 ID、重复 ID、空结果的错误语义

建议接口：

- `POST /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces/{workspaceId}`

验收：

- 能创建并查询一个明确的 `workspace`
- `file` 后端下，跨请求仍能查询到同一 `workspace`
- 不合法参数返回稳定错误码

收口说明：

- 已提供 `POST /api/v1/workorch/workspaces`
- 已提供 `GET /api/v1/workorch/workspaces`
- 已提供 `GET /api/v1/workorch/workspaces/{workspaceId}`
- 已补 `appservice`、HTTP、进程级测试与本地验证脚本

### [x] B2. run 实例接口

Status: completed

目标：让外部围绕真实 `run` 实例使用编排能力，而不是只调用 `slice` 执行接口。

完成内容：

- 增加创建 `run` 接口
- 增加 `run` 单条查询接口
- 增加 `run` 列表接口
- 增加按 `workspaceId` 查询 `run` 的过滤能力
- 固定 `run` 读模型，包括 `state`、blocking 信息、终态原因

建议接口：

- `POST /api/v1/workorch/workspaces/{workspaceId}/runs`
- `GET /api/v1/workorch/runs`
- `GET /api/v1/workorch/runs/{runId}`

验收：

- 能基于指定 `workspace` 创建 `run`
- 能查询 `run` 当前状态和阻塞信息
- `file` 后端下，跨请求仍能读取相同 `run`

收口说明：

- 已提供 `POST /api/v1/workorch/workspaces/{workspaceId}/runs`
- 已提供 `GET /api/v1/workorch/runs`
- 已提供 `GET /api/v1/workorch/runs/{runId}`
- 已支持 `workspaceId` 过滤
- 已固定 `run` 对外读模型，包括 `state`、blocking refs、`goalId`、`doDId`、`failureReason`

### [x] B3. run 启动与恢复接口

Status: completed

目标：把当前预置 `slice` 内部完成的运行推进逻辑，转成正式服务接口。

完成内容：

- 增加启动 `run` 接口
- 增加恢复 `run` 接口
- 明确已启动、已完成、已失败、已阻塞等状态下的幂等和冲突语义
- 明确恢复接口对 decision / approval 依赖未完成时的错误语义

建议接口：

- `POST /api/v1/workorch/runs/{runId}/start`
- `POST /api/v1/workorch/runs/{runId}/resume`

验收：

- 正常 `run` 可启动到推进状态
- 被 decision / approval 阻塞的 `run` 在条件满足后可恢复
- 非法状态转换返回稳定冲突错误

收口说明：

- 已提供 `POST /api/v1/workorch/runs/{runId}/start`
- 已提供 `POST /api/v1/workorch/runs/{runId}/resume`
- 已固定 `ErrBlocked -> 409 conflict` 的服务错误语义
- 已补 `appservice`、HTTP、进程级 file 后端测试
- 已补本地服务 / 容器验证脚本，验证 `start` 成功路径与 `resume` 的 blocked 语义
- 成功 unblock 后的外部 HTTP 闭环，留待 `B4/B5` 正式接口收口

### [x] B4. decision 工单查询与处理接口

Status: completed

目标：让 decision gate 从“slice 内部动作”变成“外部可操作的业务闭环”。

完成内容：

- 增加 decision ticket 列表接口
- 增加 decision ticket 详情接口
- 增加 resolve 接口
- 增加 reject 接口
- 增加按 `runId` / `workspaceId` 过滤能力

建议接口：

- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/{ticketId}`
- `POST /api/v1/workorch/decisions/{ticketId}/resolve`

验收：

- 能查询待处理 decision ticket
- resolve 后对应 `run` 可继续推进
- reject 后对应 `run` 进入明确终态

收口说明：

- 已提供 `GET /api/v1/workorch/decisions`
- 已提供 `GET /api/v1/workorch/decisions/{ticketId}`
- 已提供 `POST /api/v1/workorch/decisions/{ticketId}/resolve`
- 已支持正式 resolution 类型 `provide_missing_info`、`reject_current_path`、`replan`
- 已支持按 `workspaceId` / `runId` 过滤 decision ticket
- 已固定对外 decision 读模型，包括 `state`、`reason`、`runId`、`workspaceId`、resolution 字段
- 已补 `appservice`、HTTP、进程级 file 后端测试
- 已补本地服务 / 容器验证脚本，验证 decision list/get/resolve/reject 与 resume 闭环

### [x] B5. approval 工单查询与处理接口

目标：让 approval gate 也具备正式的人机协同处理路径。

完成内容：

- 增加 approval 列表接口
- 增加 approval 详情接口
- 增加 approve 接口
- 增加 reject 接口
- 增加按 `runId` / `workspaceId` 过滤能力

建议接口：

- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/{approvalId}`
- `POST /api/v1/workorch/approvals/{approvalId}/approve`
- `POST /api/v1/workorch/approvals/{approvalId}/reject`

验收：

- 能查询待审批项
- approve 后对应 `run` 可继续推进
- reject 后对应 `run` 进入明确终态

收口说明：

- 已提供 `GET /api/v1/workorch/approvals`
- 已提供 `GET /api/v1/workorch/approvals/{approvalId}`
- 已提供 `POST /api/v1/workorch/approvals/{approvalId}/approve`
- 已提供 `POST /api/v1/workorch/approvals/{approvalId}/reject`
- 已支持按 `workspaceId` / `runId` 过滤 approval 项
- 已固定对外 approval 读模型，包括 `state`、`reason`、`runId`、`workspaceId` 字段
- 已补 `appservice`、HTTP、进程级 file 后端测试
- 已补本地服务 / 容器验证脚本，验证 approval list/get/approve/reject 与 resume 闭环

### [x] B6. 实例读模型补齐

目标：把“按 `sliceId` 读取”之外的正式实例查询能力补齐，降低外部集成成本。

完成内容：

- 增加按 `runId` 读取快照能力
- 增加按 `runId` 读取状态能力
- 增加按 `workspaceId` 查看关联 `run` / blocking 项能力
- 固定对外 JSON 读模型字段

验收：

- 不依赖 `sliceId` 也能完成核心查询
- 业务方能仅凭 `workspaceId` / `runId` 拉取主要编排状态

收口说明：

- 已提供 `GET /api/v1/workorch/runs/{runId}/snapshot`
- 已提供 `GET /api/v1/workorch/runs/{runId}/status`
- 已提供 `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`
- 已固定 `run snapshot`、`run status`、`workspace snapshot` 的对外 JSON 读模型字段
- 已补 `appservice`、HTTP、进程级 file 后端测试
- 已补本地服务 / 容器验证脚本，验证 `runId` / `workspaceId` 维度的实例读模型查询

### [x] B7. Goal 和 DoD 定义流程

目标：创建 `run` 前必须先在 `workspace` 下定义正式业务目标和验收标准，避免运行实例只携带外部手填的 `goalId` / `doDId` 而没有可管理的业务定义来源。

完成内容：

- 增加 Goal / DoD 正式模型仓储，并支持 `memory` / `file` 后端
- 增加 `workspace` kernel module 的 `DefineGoal` / `DefineDoD` 服务
- 增加 Goal / DoD HTTP 创建、列表和详情接口
- `POST /api/v1/workorch/workspaces/{workspaceId}/runs` 要求引用已定义的 `goalId` 和 `doDId`
- Web 管理界面在 `Workspaces` 页面以 Workspace 为主体查看和维护 Goal / DoD，创建 Workspace 后继续通过向导定义 Goal / DoD，并在创建 Run 时按 Workspace 加载可选 Goal / DoD

验收：

- 能在一个 `workspace` 下创建并查询 Goal
- 能在一个 `workspace` 下创建并查询 ready 状态 DoD
- 创建 `run` 时使用已定义 Goal / DoD，并在 `file` 后端重启后仍可查询
- `make verify-service` 和 `make verify-service-file` 覆盖该流程

## P1 Closed Stability Scope

### [x] S1. 仓储后端共享契约测试

目标：保证 `memory` / `file` 两套后端在相同业务动作下表现一致。

完成内容：

- 抽象共享仓储契约测试
- 覆盖 workspace / run / step / ticket / approval / resolution 的基本读写
- 覆盖重复写入、缺失读取、终态更新、跨请求读取

验收：

- 相同契约测试同时覆盖 `memory` / `file`
- 任一后端回归会直接在测试中暴露

收口说明：

- 已补共享仓储 bundle contract，对 `workspace / run / step / ticket / resolution / approval` 复用同一套业务流断言
- 已同时覆盖 `memory` / `file` 两套后端
- 已覆盖重复写入、缺失读取、终态更新、跨请求读取
- 任一后端在仓储契约回归时可直接通过 `go test ./internal/pkg/repository -count=1` 暴露

### [x] S2. file 后端持久化语义补强

目标：把 `file` 后端从“可用的第一版”补成稳定的本地服务持久化实现。

完成内容：

- 补齐重复执行同一业务实例时的幂等验证
- 补齐重复读取、重启后读取、终态读取的一致性验证
- 补齐异常输入下的文件持久化边界验证

验收：

- `file` 后端下重复启动服务后，实例状态仍可稳定查询
- 重复执行和重复读取的行为有稳定预期

收口说明：

- 已补 `appservice` 层 file 后端终态实例的重复读取一致性测试
- 已补 `appservice` 层 completed / failed run 在 reload 后再次 `start` / `resume` 的稳定幂等 / 冲突语义测试
- 已补 `workorchd` 进程级 file 后端重启测试，覆盖 completed / failed / resolved / rejected 实例在重启后的 `get` / `snapshot` / `status` / `workspace snapshot` 查询
- 已补 `internal/pkg/repository/file` 损坏 JSON 持久化文件边界测试，确认返回显式错误且不静默覆盖原文件
- 已通过定向包测试、全量 `go test ./...` 与 `make verify-service-file`

### [x] S3. 最小事件总线路径正式化

目标：把当前事件采集和观察能力从演示式接线，推进到正式 runtime 边界。

完成内容：

- 明确哪些事件是正式业务事件
- 明确 `run` / `decision` / `approval` / `workspace` 的事件发布点
- 明确 `workorchd` 内部消费与快照组装边界
- 补齐事件相关测试

验收：

- 快照与状态读取不再过度依赖 demo 语义
- 后续扩展真实 adapter 或通知通道时，不需要重写核心事件边界

收口说明：

- 已明确当前正式业务事件集合：`workspace created/state changed`、`run created/state changed`、`step dispatched/state changed`、`ticket created/resolved`、`approval created/resolved`
- 已把 `run created` 正式接入 `EventHub`、event contract、collector 和 observed snapshot 合同
- 已补 run/workspace 事件 payload 自包含字段，`run` 事件现在携带 `goalId`、`doDId`、`failureReason`，`workspace state changed` 事件现在携带 `title`
- 已明确 `workorchd` 内部边界：`modules` 负责发布正式业务事件，`observed/collector` 负责订阅与归一，`observed/workorch_contract` 负责事件到 snapshot item 的合同映射，只有缺失匹配事件时才回退到状态兜底
- 已补 `events`、`run` 模块、`observed` 的事件相关测试，并通过相关包回归、`go test ./...` 与 `make verify-service-file`

## Closed Scope Notes

MVP 范围内不再保留活跃待办项。本文件只保留早期服务 MVP
收口记录，不再承担后续功能范围跟踪职责。

后续已经完成的 runtime expansion、LLM-driven Workspace execution、真实
provider 验证入口分别由以下文档跟踪：

- `runtime-expansion-plan.md`
- `llm-driven-development-plan.md`
- `../../50-product-interface/deployment.md`

仍未进入当前收口范围的长期能力包括分布式调度、完整外部 runtime
marketplace、多租户治理、知识资产/检索增强体系，以及 MCP resources、
prompts、streaming、session reuse 等高级协议能力。
