# WorkOrch Verification Overview

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本目录用于集中承接服务验收、core/profile verifier、audit、TUI acceptance 和 loadtest baseline。

## Reading Order

1. [service-acceptance.md](service-acceptance.md)
2. [core-runtime-verifier.md](core-runtime-verifier.md)
3. [incremental-artifact-code-review-checklist.md](incremental-artifact-code-review-checklist.md)
4. [domain-model-code-alignment-checklist.md](domain-model-code-alignment-checklist.md)
5. [runtime-flow-code-alignment-checklist.md](runtime-flow-code-alignment-checklist.md)
6. [module-architecture-code-alignment-checklist.md](module-architecture-code-alignment-checklist.md)
7. [profile-capability-code-alignment-checklist.md](profile-capability-code-alignment-checklist.md)
8. [product-interface-code-alignment-checklist.md](product-interface-code-alignment-checklist.md)
9. [verification-layer-code-alignment-checklist.md](verification-layer-code-alignment-checklist.md)
10. [profile-software-dev-verifier.md](profile-software-dev-verifier.md)
11. [workorch-runtime-closure-modification-spec.md](workorch-runtime-closure-modification-spec.md)
12. [loadtest-baseline.md](loadtest-baseline.md)
13. [audits/overview.md](audits/overview.md)
14. `tui/`
   - [tui-agent-acceptance.md](tui/tui-agent-acceptance.md)
   - [display-closure-tracking.md](tui/display-closure-tracking.md)

## Verification Layers

- Service acceptance
  - 验证服务可启动、最小业务链可运行、file backend / deployed instance 基线成立
- Core runtime verifier
  - 验证中立 contract、context gate、repair/replan/governance 边界、request audit
- Profile verifier
  - 验证 software-dev 的 coverage、evidence、entrypoint integrity 和 profile-specific 规则
- Runtime closure modification spec
  - 约束线上 workspace / TUI / projection / repair / CPU-IO 问题收口时的修改落点、审核顺序和反打圈检查
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

## Current Status

当前本层已经形成稳定入口：

- `service-acceptance.md`
  - 承接服务启动、最小业务链、file backend persistence 与 deployed verification 入口
- `core-runtime-verifier.md`
  - 承接 core contract、context gate、repair/replan、request audit 的中立验证
- `profile-software-dev-verifier.md`
  - 承接 software-dev profile 的 coverage/evidence/deliverable 闭环验证
- `workorch-runtime-closure-modification-spec.md`
  - 承接运行期异常收口、core/template 边界和修改审核规范
- `tui/*`
  - 承接 TUI acceptance、display closure 与 review boundary
- `loadtest-baseline.md`
  - 承接当前 active 并发基线、指标口径和 records layering

后续若继续补写，主要只应是脚本索引、字段级映射和更多样例补细，
而不是重新定义 runtime / product / profile 设计边界。
