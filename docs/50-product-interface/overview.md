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

## Current Gaps

当前本层仍未完全收口的重点包括：

- `service-api.md`
  - 极少量 response/schema 收尾字段
- `deployment.md`
  - 与 service-acceptance 的职责边界、file backend / deployed instance 差异摘要
- `tui/*`
  - workspace monitoring / result-first / decision-approval / local-vs-remote 继续细化
- `web-console/*`
  - 页面字段级映射与低频交互补充

完整目录级缺口总表见：

- `../01-overview/current-unclosed-matrix.md`
