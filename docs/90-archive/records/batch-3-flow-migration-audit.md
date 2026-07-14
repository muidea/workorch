# Batch 3 Flow Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 3` 建立 runtime flow 层时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 Flow 文档和对象文档在输入来源上仍有重叠

当前很多源文档同时描述：

- 业务对象
- runtime flow
- 实现任务
- e2e 验收

处理结论：

- `20-runtime-flow/` 只保留链路规则
- 对象定义继续留在 `10-domain-model/`
- 脚本和验收要求后续迁到 `60-verification/`

### 1.2 Context 组织规则在多个文档重复出现

以下文档都涉及 context 组织：

- `context-assembly-flow.md`
- `../00-core/runtime-request-cache-note.md`
- `../60-verification/audits/codex-request-context-comparison.md`
- 旧核心合同文档

处理结论：

- `00-core/` 保留唯一 contract
- `context-assembly-flow.md` 只保留链路视角
- provider-facing 结构和实现细节后续放模块层
- `../60-verification/audits/codex-request-context-comparison.md` 的对标结论已拆到
  `context-assembly-flow.md` 与 `../60-verification/audits/codex-request-context-comparison.md`
- `../00-core/runtime-request-cache-note.md` 的稳定 cache-aware 规则已拆到
  `../00-core/runtime-request-cache-note.md`

### 1.3 Repair / Replan / Governance 仍然存在“对象”和“流程”交织

历史恢复治理设计文档（现归档于 `../90-archive/completed/runtime-recovery-governance-closure-design.md`）既讲 ticket 结构，
也讲 auto-governance 和恢复动作表。

处理结论：

- 对象定义已在 domain 层抽出
- 恢复链和动作转换留在 flow 层
- 上述历史恢复治理设计文档的稳定恢复治理规则已迁入
  `repair-replan-flow.md`，历史正文进入 completed archive

## 2. 内容不完整，需要补充

### 2.1 Activation flow 仍缺 definition revision / successor rebuild 细节

当前 `activation-definition-flow.md` 只建立了主链骨架，仍缺：

- definition change impact
- successor run activation 继承策略
- activation 与 workspace setup/template 入口的差异

### 2.2 Planning/execution flow 仍缺 block kind 级细化

当前 `planning-execution-flow.md` 已继续吸收 `planning-execution-flow.md`
中的稳定链路规则，但还没有展开：

- patch preview / patch apply / patch revert
- file / git / web capability
- graph node 并发执行的更细约束

### 2.3 Verification/acceptance flow 仍缺 coverageWorkItems 细化

当前已继续吸收 `../70-roadmap/active/run-closure-quality-improvement-plan.md` 中的 closure 规则，
但只抽出了：

- evidence-backed dodAssessment
- accepted/rejected Result

仍缺：

- missingDoDItems
- claim-to-evidence alignment
- current-run evidence gate
- coverageWorkItems ledger

### 2.4 Context assembly flow 仍缺 selectedContext refresh 细节

当前已迁入 recall trace 的主流程，但还没有把以下规则完整迁入：

- step input refresh
- provider retry / repair / replan 的 memory 继承
- request audit 与 cachePlan 的关系
- selectedContext 继承排序与 recall policy 细节

### 2.5 Evidence/result flow 仍缺 payload ref 和 summary schema 细化

当前只建立了整体链路，仍缺：

- raw payload ref 分类
- event payload summary contract
- final result summary schema
- artifact manifest 分类与 profile 边界

## 3. 无法完整衔接的内容断点

### 3.1 很多 flow 规则仍深嵌在核心 contract 中

当前 `../00-core/runtime-stage-contract.md` 仍保留大量：

- context bundle
- acceptance gate
- repair contract
- provider retry
- coverage ledger

这意味着 `20-runtime-flow/` 目前只能先建立外层骨架，不能完全替代 core 文档中
的细化规则。

### 3.2 `../70-roadmap/active/run-closure-quality-improvement-plan.md` 仍是 closure 语义的重要来源

例如：

- outcome projection
- strict DoD acceptance
- runtime recovery stability
- process completeness

这些内容目前已部分迁入 `verification-acceptance-flow.md` 和 active roadmap，
但仍需继续拆到 flow 和 verification。

### 3.3 software-dev 特化仍会污染 flow 输入材料

`verification-acceptance-flow.md` 和
`../60-verification/audits/full-chain-assessment-closure-plan.md` 虽强调 kernel 中立，
但大量示例和验收路径仍以 software-dev 为主。

处理结论：

- 允许当前作为问题样本输入
- 但 flow 文档本身不继续吸收 software-dev 专属规则
