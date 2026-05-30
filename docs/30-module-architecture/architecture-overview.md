# WorkOrch Architecture Overview

Status: active

Type: module-architecture

Last Updated: 2026-05-29

## Purpose

本文件提供 WorkOrch 当前稳定的模块架构总览，不再承担完整 runtime contract、
全量 API 清单或历史实现记录。

## Stable Layers

WorkOrch 当前可按稳定责任划分为：

- application layer
- kernel layer
- block/runtime adapter layer
- shared internal package layer
- web console layer

### Layer Roles

- application layer：进程入口、bootstrap、HTTP/TUI surface、appservice 级 use-case 组合
- kernel layer：Workspace / Goal / DoD / Run / Step / Decision / Approval / Result / Artifact / EventStream / Orchestrator 等正式业务状态与编排
- block/runtime adapter layer：LLM、MCP、shell、file、git、web、ssh、approval 等外部动作与 provider 调用
- shared internal package layer：common types、event helpers、repository interfaces/implementations、config 等共享基础设施
- web console layer：基于 read model 的产品展示与操作入口

## Top-Level Structure

核心代码落点：

- `application/workorch`
- `internal/initiators/workorchruntime`
- `internal/modules/application`
- `internal/modules/kernel`
- `internal/modules/blocks`
- `internal/pkg`
- `web/workorch-console`

## Key Modules

### Application

- `application/workorch`: 主可执行入口，负责本地 TUI 和 `--server` 模式启动
- `internal/initiators/workorchruntime`: 提供 repository、event hub、runtime policy 和 cross-module wiring
- `internal/modules/application/workorchd`: 应用层插件模块，承接 appservice、server、observed/presenter

### Kernel

当前稳定 kernel 关注点包括：

- workspace / goal / dod
- run / step / tool call / patch / validation / checkpoint
- soul / skill / mcp / memory
- decision / approval / result / artifact
- eventstream
- orchestrator

### Blocks

当前稳定 block 集合包括：

- `blocks/llm`
- `blocks/mcp`
- `blocks/shell`
- `blocks/file`
- `blocks/git`
- `blocks/web`
- `blocks/ssh`

## Architecture Boundary

### Application

应用层负责：

- 进程入口
- 运行时装配
- HTTP/TUI 对外面向
- appservice use-case 组合

### Kernel

kernel 负责：

- 正式领域对象
- Run/Step/Decision/Approval/Result 等业务状态
- orchestrator 编排
- eventstream 持久化事件

### Blocks

blocks 负责：

- 外部动作执行
- provider 调用
- shell / MCP / file / git / web / ssh 等 runtime adapter 行为

### Shared Internal Packages

`internal/pkg` 负责：

- common types
- shared event helpers
- repository interfaces/implementations

## Key Rule

稳定规则是：

- business state transitions 留在 kernel / orchestrator
- side effects 留在 blocks
- request/service composition 留在 application layer
- shared primitives 留在 `internal/pkg`

补充：

- product/UI 层不得绕过 read model 直接定义业务状态
- template / Soul / capability 特化不得下沉到 kernel 基础 contract
- runtime adapter 不负责 accepted Result 或最终 DoD 裁决

## Current Stable Decisions

- 仓库保持单 `go.mod` 形态，不拆成多个 Go modules
- `application/workorch` 保持 thin main，显式选择加载 initiators / modules
- 大型 runtime 文件按责任拆分，但仍保持同一应用/模块边界
- shared event helpers、repository 和 config 统一沉到 `internal/pkg`

## Source Documents

- `repository-structure.md`
- `../00-core/runtime-stage-contract.md`
