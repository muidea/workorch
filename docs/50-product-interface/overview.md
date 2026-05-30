# WorkOrch Product Interface Overview

Status: active

Type: product-interface

Last Updated: 2026-05-30

## Purpose

本目录用于承接 WorkOrch 的产品接口面，包括部署入口、HTTP API、TUI 和 Web Console。

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

## Cross-Layer Boundary

- core runtime contract 属于 `00-core/`
- domain object 定义属于 `10-domain-model/`
- runtime flow 属于 `20-runtime-flow/`
- module architecture 属于 `30-module-architecture/`
- profile/capability 特化属于 `40-profiles-capabilities/`
- acceptance / verifier / audit 属于 `60-verification/`

产品接口层不应重新定义 runtime core 规则或业务对象边界。

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
