# Batch 6 Verification Roadmap Archive Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 6` 建立 verification / roadmap / archive 层时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 验收、部署、状态文档职责重叠仍然明显

当前以下文档都在承担部分验收角色：

- `../deployment.md`
- 旧综合状态文档（现归档副本见 `../90-archive/superseded/workorch-status.md`）
- `../workspace-concurrency-loadtest-plan.md`
- `../workorch-tui-agent-acceptance.md`

处理结论：

- `60-verification/` 只建立正式入口
- 具体内容仍需后续继续拆分迁入
- TUI 验收、display closure 和 `2026-05-03` review 已开始分拆到
  `60-verification/tui/` 与 `60-verification/audits/`

### 1.2 路线图文档与完成记录文档仍混杂

当前 active / candidate / completed 的边界在旧文档中不稳定：

- `../next-stage-demand-candidates.md` 与 `../next-stage-result-event-development-plan.md` 互相引用
- 旧综合状态文档当时同时列 active next stage、completed P1、runtime expansion、advanced scheduling

处理结论：

- 新结构已经分层
- 旧文档内容仍需后续逐项迁出

### 1.3 审计文档与计划文档仍存在角色交叉

例如：

- `../runtime-contract-conformance-audit.md` 同时包含核查、偏差、收口跟踪
- `../workorch-full-chain-assessment-closure-plan.md` 同时是审计和计划

处理结论：

- 本批先将其归入 verification/audits 入口
- 后续仍需判断哪些部分属于 audit，哪些部分属于 roadmap
- `../runtime-contract-conformance-audit.md`、
  `../workorch-full-chain-assessment-closure-plan.md`、
  `../workorch-codex-request-context-comparison.md` 已开始迁入
  `60-verification/audits/`

## 2. 内容不完整，需要补充

### 2.1 verification 入口仍不完整

还缺：

- 服务验收命令分层
- 本地 / file backend / deployed-instance 差异
- 与 `../deployment.md` 的分工说明
- loadtest baseline 的指标口径、脚本入口与 records 交叉引用
- TUI acceptance / closure / review 之间的剩余交叉引用清理

### 2.2 `core-runtime-verifier.md` 和 `profile-software-dev-verifier.md` 目前只有边界说明

还缺：

- 脚本入口
- 具体断言项
- 与 e2e / unit / deployed verification 的映射

### 2.3 `active/`, `candidates/`, `archive/` 仍未全部完成正文迁移

还缺：

- 每个旧文档的正式迁移落位
- 从旧路径到新路径的引用切换
- 归档完成后的旧文档处理方式

## 3. 无法完整衔接的内容断点

### 3.1 旧综合状态文档已降级为过渡索引，但旧链路仍未清理完

`60-70-90` 三层的正文已经建立，当前主要问题转为旧路径互引和历史说明清理。

### 3.2 旧 roadmap / completed 文档仍未全部收口

当前虽然已经迁出第一批 active / candidate / completed 正文，并开始迁入 records 正文，但仍有剩余旧文档未逐项收口：

- `../single-workspace-performance-optimization-record.md`
- `../workorch-console-bundle-ui-optimization-experience.md`
- `../workspace-concurrency-loadtest-plan.md`
- 多份 verification / audit / runtime 专题旧正文

并且旧路径互引仍待清理。

### 3.3 archive 目前仍是“目标落点”，不是“已完成归档”

当前 `90-archive/` 已接收多份 completed / superseded 正文，
但 records 与剩余 completed 正文仍需后续批次逐项执行，否则旧文档仍在根目录参与主阅读路径。
