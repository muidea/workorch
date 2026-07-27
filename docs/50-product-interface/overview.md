# WorkOrch Product Interface Overview

Status: active

Type: product-interface

Last Updated: 2026-07-27

## Purpose

本目录用于承接 WorkOrch 的产品接口面，包括部署入口、HTTP API、TUI 和 Web Console。

生产 runtime 主链为 **Runtime V2 only**。产品接口只向 Run owner 提交 scoped activation / continuation intent，
并展示 projection；不复制 V2 reducer / authority、不指定任何 owner target state、不推断 next action，也不通过
`ExecutionContextPatch` 或 V1 graph/next-action 路径推进。

## Reading Order

1. [service-api.md](service-api.md)
2. [deployment.md](deployment.md)
3. `tui/`
   - [requirements.md](tui/requirements.md)
   - [interaction-contract.md](tui/interaction-contract.md)
   - [display-refresh-consistency-design.md](tui/display-refresh-consistency-design.md)
4. `web-console/`
   - [overview.md](web-console/overview.md)
   - [bundle-ui-optimization-record.md](web-console/bundle-ui-optimization-record.md)

## Product Surfaces

### Service API

负责：

- HTTP resource families
- JSON/read-model envelope
- governance / registry / content open path
- configuration write contract
- execution activation / Start / Resume 的 command accepted 语义（进入 V2 drive）

权威文档：

- `service-api.md`

### Deployment

负责：

- container image
- local/file backend startup
- deployment-side configuration entry

权威文档：

- `deployment.md`

### TUI

负责：

- 本地 agent-first 交互
- result-first 主视图
- workspace switching / decision / approval input
- display refresh consistency

权威文档：

- `tui/requirements.md`
- `tui/interaction-contract.md`
- `tui/display-refresh-consistency-design.md`

### Web Console

负责：

- 远程控制与观察面
- board / workspace detail / governance / runtime control / configuration / service
- 多 workspace 聚合、治理处理、配置管理

权威文档：

- `web-console/overview.md`
- `web-console/bundle-ui-optimization-record.md`

## Runtime-Facing Interface Stance

- activation / create run：封存 typed bootstrap 输入，创建 V2 Run。
- Start：向 Run owner提交 activation intent；owner-local `pending→active` notification 后进入 `DriveRuntimeV2`。
- Resume：workspace/run scope 的 continuation intent；Run owner校验 blocker 并内部恢复后通知 drive；**无** execution-context patch。
- Decision / Approval resolve：提交 resolution；**不得**要求 application 再 Resume 才能回到 drive。
- replan 语义：新 activation / replacement run，不是同 Run 自动 successor。
- UI 消费 projection；不得本地重建 RuntimeDecision。

## Cross-Layer Boundary

- core runtime contract / Runtime V2 属于 `00-core/`
- domain object 定义属于 `10-domain-model/`
- runtime flow 属于 `20-runtime-flow/`
- module architecture 属于 `30-module-architecture/`
- profile/capability 特化属于 `40-profiles-capabilities/`
- acceptance / verifier / audit 属于 `60-verification/`

产品接口层不应重新定义 runtime core 规则或业务对象边界，也不得把 V1 StagePacket/NextAction/graph dispatch 重新声明为 API 生产语义。

## Current Status

当前本层主设计已经形成稳定入口：

- `service-api.md`
  - 已承接 HTTP resource families、response family、raw/detail 打开边界和主要字段语义
- `deployment.md`
  - 已承接部署入口、配置分区与 deployment-vs-acceptance 边界
- `tui/*`
  - 已承接 result-first、workspace switch、decision/approval 交互与 display consistency
- `web-console/*`
  - 已承接 page-to-API mapping、raw-content 边界、低频交互与表单责任边界

后续若继续补写，主要只应是字段级索引、示例补细和少量产品样例增强，
而不是重新定义 runtime 或业务对象边界。
