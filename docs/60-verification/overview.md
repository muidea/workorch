# WorkOrch Verification Overview

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本目录用于集中承接服务验收、core/profile verifier、audit、TUI acceptance 和 loadtest baseline。

## Reading Order

1. [service-acceptance.md](service-acceptance.md)
2. [core-runtime-verifier.md](core-runtime-verifier.md)
3. [profile-software-dev-verifier.md](profile-software-dev-verifier.md)
4. [loadtest-baseline.md](loadtest-baseline.md)
5. [audits/overview.md](audits/overview.md)
6. `tui/`
   - [tui-agent-acceptance.md](tui/tui-agent-acceptance.md)
   - [display-closure-tracking.md](tui/display-closure-tracking.md)

## Verification Layers

- Service acceptance
  - 验证服务可启动、最小业务链可运行、file backend / deployed instance 基线成立
- Core runtime verifier
  - 验证中立 contract、context gate、repair/replan/governance 边界、request audit
- Profile verifier
  - 验证 software-dev 的 coverage、evidence、entrypoint integrity 和 profile-specific 规则
- Audits
  - 承接 design-vs-implementation、runtime contract、full-chain、Codex comparison 等核查材料
- TUI verification
  - 承接 acceptance、display closure、review absorption
- Loadtest baseline
  - 承接并发容量与历史轮次基线

## Cross-Layer Boundary

- 产品入口与资源清单属于 `50-product-interface/`
- 运行链路属于 `20-runtime-flow/`
- profile-specific 规则来源属于 `40-profiles-capabilities/`
- archive / completed records 属于 `90-archive/`

## Current Gaps

当前本层仍未完全收口的重点包括：

- `service-acceptance.md`
  - active 设计层已补 paginated detail / file-backend reload / deployed 对照矩阵；后续若继续补写，主要是更细 deployed shell 样本
- `core-runtime-verifier.md`
  - active 设计层已收口；后续若继续补写，仅是穷举 rule-to-test 索引
- `profile-software-dev-verifier.md`
  - active 设计层已收口；后续若继续补写，仅是穷举 profile-rule-to-assertion 索引
- `tui/*`
  - 缺 acceptance / closure / review 的剩余交叉引用清理
- `loadtest-baseline.md`
  - active 设计层已补入口、指标口径、failure-class contract 与 records 分层；后续若继续补写，主要是更多历史轮次索引

完整目录级缺口总表见：

- `../01-overview/current-unclosed-matrix.md`
