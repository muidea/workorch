# WorkOrch Repository Structure

Status: active

Type: module-architecture

Last Updated: 2026-07-27

## Purpose

本文件说明当前仓库的稳定目录组织与模块形状，对齐 Runtime V2 生产主链与 `internal/` 树。

## Repository Shape

单 `go.mod` 代码库。

```text
application/workorch/          # thin main
internal/
  initiators/workorchruntime/  # repository provider 装配
  modules/
    application/workorchd/     # appservice / server / bootstrap / projection
    kernel/
      orchestrator/            # Runtime V2 convergence authority
      governance/              # auto-governance coordination
      system/                  # defaults ensure
    blocks/
      workspace/ run/ result/ decision/ approval/ memory/
      soul/ skill/ mcpregistry/ eventstream/
      llm/ file/ git/ shell/ mcp/ web/ ssh/
      pkg/                     # blocks 共享辅助（若有）
  pkg/
    workorchruntimecontract/   # Runtime V2 中立 contract
    workorchrepo/ workorchport/ workorchownercontract/
    workorchconfig/ workorchdefaults/ workorchmanagedconfig/
    workorchruntimepolicy/ workorchclient/ workorchdeps/
    repository/                # memory / file backends
    eventcontract/ events/ eventvalue/ runtimeevent/
    common/ dialoglanguage/ doditems/ runtimepath/
  services/workorch/           # CLI / TUI / agent session backends
web/workorch-console/          # Web Console
docs/                          # 设计与模块文档
```

### Top-Level Intent

| 路径 | 意图 |
| --- | --- |
| `application/workorch` | 进程入口 |
| `internal/initiators` | framework / repository initiator |
| `internal/modules/application` | appservice、server、bootstrap、observed/presenter |
| `internal/modules/kernel` | Orchestrator/Governance/System 多 owner 协调 Modules |
| `internal/modules/blocks` | stateful owners + capability execution |
| `internal/pkg` | 全局共享 contract 与基础设施 |
| `internal/services` | CLI/TUI 等人机入口服务 |
| `web/workorch-console` | 前端 |

## Framework Unit Shape

Module/Block 最小形状：

```text
module.go
plugin.go
biz/
service/          # 仅按需的 HTTP/CLI/session/protocol adapter
pkg/common/
pkg/models/       # 按需
pkg/events/       # EventHub 具体 command/data/result
pkg/contracts/    # 按需
readme.md
```

含义：

- `plugin.go`：向 magicCommon/pluginmodule 注册
- `module.go`：生命周期入口
- `biz/`：业务实现
- `service/`：仅用于 HTTP/CLI/session/protocol adapter；owner unit 不导出 direct
  service facade，生产跨组件只能使用 typed EventHub command/result。
- `pkg/events`：跨组件唯一交互合同
- `pkg/common` / `pkg/models`：常量与本地模型

## Kernel / Orchestrator Layout

`kernel/` 只包含多 owner 协调 Module：`orchestrator`、`governance`、`system`。
`blocks/run` 与 `blocks/eventstream` 是单资源 Block owner；其目录、module ID、
EventHub source/destination 与 import path 均使用 `blocks/*`。

```text
internal/modules/kernel/orchestrator/
  biz/runtimev2/          # controller, reducer, prerequisite, dispatch, executor, governance
  pkg/events/             # Drive / Operation / Candidate / Governance 合同
  pkg/contracts/
  runtime_v2_*.go         # drive, state repository, operation executor, result projection
  readme.md
```

```text
internal/modules/blocks/run/
  biz/ pkg/events/ pkg/models/ pkg/contracts/
  # RuntimeBootstrap + RuntimeStateRecord 随 Run 持久化与 CAS
```

## Blocks Layout

```text
internal/modules/blocks/
  # Stateful owners
  workspace/ run/ result/ decision/ approval/ memory/
  soul/ skill/ mcpregistry/ eventstream/
  # Capability execution
  llm/          # 含 biz/runtimev2 candidate 边界
  file/ git/ shell/ mcp/ web/ ssh/
```

Capability block 不再依赖“注册到 Orchestrator RuntimeAdapter registry”的生产路径；Orchestrator 通过 typed operation/candidate EventHub 命令调用。

## Shared Package Highlights

| 包 | 职责 |
| --- | --- |
| `workorchruntimecontract` | `RuntimeBootstrap`、`RuntimeStateRecord`、`RuntimeFacts`、candidate、dispatch、governance、evidence、profile/execution contract |
| `workorchrepo` / `repository` | repository 接口与 memory/file 实现 |
| `workorchport` / `workorchownercontract` | 跨模块 port / owner 合同辅助 |
| `workorchconfig` / `workorchdefaults` / `workorchmanagedconfig` | 配置与默认 Soul/Template 等 |
| `eventcontract` / `runtimeevent` | 事件合同与运行时事件定义 |
| `workorchclient` | 客户端访问 |

## Application Shape

- `main.go` 尽量薄：解析参数 + lifecycle
- bootstrap：application-owned infrastructure only（无 owner repository export）
- appservice：activation、Start/Resume 转发、read-model、retirement migration
- observed / presenter：与 HTTP 渲染分离

## Current Mapping Direction

| 关注点 | 落点 |
| --- | --- |
| thin main | `application/workorch/main.go` |
| application wiring | `internal/modules/application/workorchd/biz/bootstrap` |
| appservice composition | `internal/modules/application/workorchd/biz/appservice` |
| Runtime V2 drive | `internal/modules/kernel/orchestrator/biz/runtimev2` + `runtime_v2_drive.go` |
| Runtime V2 contract | `internal/pkg/workorchruntimecontract` |
| Run aggregate CAS | `internal/modules/blocks/run` |
| TUI / CLI | `internal/services/workorch/service/{tui,cli}` |
| workspace projection | `internal/modules/application/workorchd` observed / appservice |

## Explicit Non-Structure（V1 已移除主链）

- 无生产路径依赖 `DispatchNextStep` / RuntimeAdapter registry 目录
- 无 application continuation queue 状态目录作为主链
- 无 `ExecutionContext` map 作为 activation/resume payload
- retirement / migration 代码可仍存在于 appservice，但不是 Runtime V2 生产调度

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `application-workorchd.md`
- `blocks-runtime-adapters.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `internal/modules/blocks/run/readme.md`
- `internal/modules/application/workorchd/readme.md`
