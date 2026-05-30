# WorkOrch Repository Structure

Status: active

Type: module-architecture

Last Updated: 2026-05-29

## Purpose

本文件说明当前仓库的稳定目录组织和模块形状。

## Repository Shape

当前仓库仍是单 `go.mod` 代码库。

主要结构：

- `application/workorch`
- `internal/initiators/workorchruntime`
- `internal/modules/application`
- `internal/modules/kernel`
- `internal/modules/blocks`
- `internal/pkg`
- `web/workorch-console`

### Top-Level Intent

- `application/workorch`: 进程入口
- `internal/initiators`: runtime / framework initiator
- `internal/modules/application`: appservice、server、observed/presenter 等应用层模块
- `internal/modules/kernel`: 正式业务模型与 orchestrator
- `internal/modules/blocks`: runtime adapters
- `internal/pkg`: 全局共享基础设施
- `web/workorch-console`: Web Console 前端

## Module Shape

模块最小形状建议为：

- `module.go`
- `plugin.go`
- `biz/`
- `service/`
- `pkg/common`
- `pkg/models`（按需）

含义：

- `plugin.go`: 向 magicCommon/pluginmodule 注册模块
- `module.go`: 暴露模块生命周期入口
- `biz/`: 业务实现
- `service/`: 面向外部的模块服务面
- `pkg/common`: 模块级常量/枚举
- `pkg/models`: 模块本地实体/DTO/view-model

## Application Shape

`application/{app}` 层建议保持：

- `main.go` 尽量薄
- 应用层装配显式
- 业务 orchestration 在 appservice
- bootstrap 提供分组后的 capability exports

补充约束：

- `main.go` 只负责解析启动参数和触发 lifecycle
- appservice 负责 use-case 组合，不承担 provider/block 细节
- bootstrap 负责 grouped exports、runtime shell 和基础依赖装配
- observed / presenter / read model 语义应从纯渲染逻辑中分离

## Refactor Direction

当前整理方向是：

- keep main thin
- keep appservice for workflow composition
- keep bootstrap for grouped assembly exports
- split large runtime files by responsibility

## Current Mapping Direction

当前已稳定的映射方向包括：

- thin main -> `application/workorch/main.go`
- application runtime wiring -> `internal/modules/application/workorchd/biz/bootstrap`
- appservice workflow composition -> `internal/modules/application/workorchd/biz/appservice`
- TUI surface -> `internal/modules/application/workorch/service/tui`
- orchestrator split-by-responsibility -> `internal/modules/kernel/orchestrator/biz`
- observed snapshot assembly -> `internal/modules/application/workorchd/pkg/observed`

## Source Documents

- `repository-structure.md`
