# WorkOrch 文档按领域模型 / 功能模块 / 数据处理流重组收口计划

Status: active

Type: plan

Last Updated: 2026-05-29

## 1. 目标

本计划用于收口 `workorch/docs` 当前平铺、角色混杂、权威边界不清的问题，
将文档体系重组为更适合指导代码架构演进的结构：

- 以业务领域模型组织核心对象和边界。
- 以数据处理流组织 activation / execution / repair / acceptance 等关键链路。
- 以功能模块组织代码落点、依赖关系和实现责任。
- 保持唯一核心 runtime 合约，不再允许并列 core 设计文档。
- 将 profile/capability、产品接口、验证材料、历史记录从 core 设计流中剥离。

## 2. 权威边界

### 2.1 唯一核心文档

`workorch-runtime-stage-contract-neutrality-design.md` 是当前 WorkOrch
runtime 演进的唯一核心设计合约。最终迁移后应落到：

`00-core/runtime-stage-contract.md`

后续任何涉及以下内容的设计、实现和评审，都必须先对齐该文档：

- runtime stage contract
- orchestrator 行为
- LLM 上下文组织
- repair / replan / provider retry
- Goal / DoD / coverage / result / evidence
- soul / skill / tool / MCP / memory 的 core runtime 边界

### 2.2 非核心文档的角色限制

除唯一核心文档外，其他文档只能承担以下角色之一：

- `core-implementation-note`
- `domain-model`
- `runtime-flow`
- `module-architecture`
- `profile-capability`
- `product-interface`
- `verification`
- `roadmap`
- `archive`

禁止继续存在第二份并列的 runtime core / LLM context / repair / replan 核心合约。

## 3. 最终目标结构

```text
docs/
  README.md

  00-core/
    runtime-stage-contract.md
    runtime-request-cache-note.md

  01-overview/
    current-state.md

  10-domain-model/
    overview.md
    workspace-goal-dod.md
    run-step-result-artifact.md
    governance-decision-approval.md
    context-capability-memory.md
    event-observability.md

  20-runtime-flow/
    overview.md
    activation-definition-flow.md
    planning-execution-flow.md
    verification-acceptance-flow.md
    repair-replan-flow.md
    context-assembly-flow.md
    evidence-result-flow.md

  30-module-architecture/
    overview.md
    architecture-overview.md
    repository-structure.md
    application-workorchd.md
    kernel-workspace-run-result.md
    kernel-orchestrator-governance.md
    kernel-context-capability.md
    blocks-runtime-adapters.md
    llm-provider-assembly.md
    storage-readmodel.md

  40-profiles-capabilities/
    overview.md
    builtin-templates-and-souls.md
    mcp/
    runtime-capabilities/
    software-dev/
    workspace/

  50-product-interface/
    deployment.md
    service-api.md
    tui/
    web-console/

  60-verification/
    service-acceptance.md
    loadtest-baseline.md
    core-runtime-verifier.md
    profile-software-dev-verifier.md
    audits/
    tui/

  70-roadmap/
    active/
    candidates/

  90-archive/
    completed/
    superseded/
    records/
```

## 4. 结构设计原则

### 4.1 业务领域模型

`10-domain-model/` 只回答以下问题：

- 系统里有哪些核心对象。
- 对象之间的关系和所有权是什么。
- 每个对象的状态、边界和不变量是什么。
- 哪些 read model / outcome / observability 是对象的投影，而不是对象本身。

不在该层讨论：

- 页面交互细节
- 某次专项排障过程
- 具体脚本命令
- 代码模块拆分历史

### 4.2 数据处理流

`20-runtime-flow/` 只回答以下问题：

- 一次 activation 如何形成首个 step input。
- execution graph、tool、patch、memory write、checkpoint 如何推进。
- verification / acceptance 如何基于 evidence 和 result 收口。
- repair / replan / provider retry 如何继承 contract 和 failure surface。
- LLM request 的 context bundle、request audit、cache-aware 结构如何组装。

不在该层重新定义对象，也不在该层承载 profile-specific 业务流程。

### 4.3 功能模块

`30-module-architecture/` 只回答以下问题：

- 模块负责什么。
- 模块依赖什么。
- 模块暴露什么。
- 模块拥有什么数据或投影责任。
- 模块参与哪些 runtime flow。
- 模块对应哪些代码路径。

### 4.4 Profile / Capability

`40-profiles-capabilities/` 承接以下内容：

- software-dev 相关流程约束
- workspace template / Soul 语义
- MCP capability 语义
- git / web-search / LSP bridge 等能力特化
- 领域术语解释、建议验证方式、典型产物结构、profile-specific coverage matrix

此层不得反向修改 core contract。

### 4.5 产品接口

`50-product-interface/` 承接以下内容：

- deployment
- HTTP API
- TUI
- Web Console

此层不再承担 runtime core 规则定义。

### 4.6 验证

`60-verification/` 必须区分：

- `core-runtime-verifier`
- `profile-software-dev-verifier`
- `service-acceptance`
- `loadtest-baseline`
- `audits`

验证文档必须与 core/profile 边界一致。

### 4.7 路线图与归档

- `70-roadmap/active/` 只保留仍需执行的计划。
- `70-roadmap/candidates/` 只保留候选项。
- `90-archive/` 负责 completed / superseded / records。

历史记录不再出现在主设计阅读路径中。

## 5. 迁移批次计划

当前迁移状态：

- 第一轮目录骨架和主要阅读入口已经建立完成
- 绝大多数 root 正文已迁入 `00-core` 到 `90-archive` 的目标目录
- 当前 root 层仅剩：
  - `README.md`
  - `docs-domain-module-flow-reorganization-plan.md`
  - `90-archive/superseded/workorch-status.md`（历史综合状态入口归档副本）
- 当前重点已经从“大批量正文拆分”转为：
  - 剩余目录正文细化
  - 旧路径互引清理
  - 审计 / tracker / overview 的最终收口

统一未收口矩阵见：

- `01-overview/current-unclosed-matrix.md`

### Batch 1：建立入口和权威边界

目标：

- 建立 `README.md`
- 固化唯一 core 文档入口
- 从状态文档中抽出当前事实入口

输出：

- `README.md`
- `00-core/runtime-stage-contract.md`
- `00-core/runtime-request-cache-note.md`
- `01-overview/current-state.md`

来源：

- `workorch-runtime-stage-contract-neutrality-design.md`
- `workorch-llm-request-cache-aware-context-design.md`
- `01-overview/current-state.md`

完成标志：

- `README.md` / `../AGENTS.md` / 主要入口说明只再引用唯一 core contract
- `01-overview/current-state.md` 能独立表达当前事实，不再混入大段历史收口记录

### Batch 2：建立领域模型层

目标：

- 抽出 Workspace、Goal、DoD、Run、Step、Result、Artifact、Decision、Approval、
  Memory、Capability、Event 等对象模型

输出：

- `10-domain-model/overview.md`
- `10-domain-model/workspace-goal-dod.md`
- `10-domain-model/run-step-result-artifact.md`
- `10-domain-model/governance-decision-approval.md`
- `10-domain-model/context-capability-memory.md`
- `10-domain-model/event-observability.md`

来源：

- `workorch-workspace-deliverable-boundary-design.md`
- `workspace-memory-recall-traceability-plan.md`
- `llm-decision-memory-improvement-plan.md`
- `next-stage-result-event-development-plan.md`
- `workorch-runtime-storage-json-control-design.md`
- `01-overview/current-state.md`

完成标志：

- 业务对象的定义、关系和不变量不再分散在状态/计划/专项 closure 文档中

### Batch 3：建立 runtime flow 层

目标：

- 建立 activation、execution、verification、acceptance、repair、replan、
  context assembly、evidence/result 主链路文档

输出：

- `20-runtime-flow/overview.md`
- `20-runtime-flow/activation-definition-flow.md`
- `20-runtime-flow/planning-execution-flow.md`
- `20-runtime-flow/verification-acceptance-flow.md`
- `20-runtime-flow/repair-replan-flow.md`
- `20-runtime-flow/context-assembly-flow.md`
- `20-runtime-flow/evidence-result-flow.md`

来源：

- `codex-like-llm-prompt-context-design.md`
- `llm-driven-workspace-execution-design.md`
- `workorch-codex-request-context-comparison.md`
- `runtime-recovery-governance-closure-design.md`
- `graph-followup-dod-assessment-closure-plan.md`
- `workorch-full-chain-assessment-closure-plan.md`
- `run-closure-quality-improvement-plan.md`
- `llm-driven-development-plan.md`

完成标志：

- activation / execution / repair / acceptance 的链路说明从 closure 文档中剥离
- flow 文档只说明链路，不再重新定义对象或 profile-specific 规则

### Batch 4：建立模块架构层

目标：

- 建立稳定的模块职责、依赖和代码落点说明

输出：

- `30-module-architecture/overview.md`
- `30-module-architecture/architecture-overview.md`
- `30-module-architecture/repository-structure.md`
- `30-module-architecture/application-workorchd.md`
- `30-module-architecture/kernel-workspace-run-result.md`
- `30-module-architecture/kernel-orchestrator-governance.md`
- `30-module-architecture/kernel-context-capability.md`
- `30-module-architecture/blocks-runtime-adapters.md`
- `30-module-architecture/llm-provider-assembly.md`
- `30-module-architecture/storage-readmodel.md`

来源：

- `structure.md`
- `workorch-architecture-design.md`
- `model-aware-llm-context-budget-design.md`
- `workorch-llm-auto-context-profile-design.md`
- `workorch-magiccommon-context-first-upgrade.md`
- `workorch-runtime-storage-json-control-design.md`

完成标志：

- 模块文档可直接指导代码重构，而不是只提供静态目录列表

### Batch 5：下沉 profile/capability 和 product/interface

目标：

- 将 software-dev / workspace / MCP / git / web-search / LSP bridge 等能力文档
  迁入 profile/capability 层
- 将 deployment / service API / TUI / Web Console 文档迁入 product/interface 层

输出：

- `40-profiles-capabilities/*`
- `50-product-interface/*`

来源：

- `builtin-templates-and-souls-design.md`
- `mcp-capability-management-design.md`
- `workspace-builtin-git-tool-capability-plan.md`
- `workspace-file-diff-patch-capability-plan.md`
- `workspace-web-search-capability-plan.md`
- `software-dev-goal-coverage-template-closure-design.md`
- `software-dev-lsp-bridge-integration-design.md`
- `software-dev-code-agent-closure-plan.md`
- `workorch-soul-md-default-resource-design.md`
- `workspace-soul-routing-plan.md`
- `workspace-setup-draft-session-plan.md`
- `deployment.md`
- `workorch-cli-tui-agent-requirements.md`
- `workorch-tui-interaction-contract.md`
- `workorch-tui-display-refresh-consistency-design.md`
- `workorch-console-bundle-ui-optimization-experience.md`
- `50-product-interface/service-api.md` 承接前的旧 API 列表和接口说明来源

完成标志：

- 领域特化约束不再污染 core 设计流
- TUI/Web/API/部署细节不再混入 runtime contract 类文档

### Batch 6：建立验证、路线图和归档层

目标：

- 将 acceptance、loadtest、audit、active roadmap、candidate roadmap、
  completed / superseded / records 全部归位

输出：

- `60-verification/*`
- `70-roadmap/*`
- `90-archive/*`

来源：

- `design-implementation-audit-2026-05-12.md`
- `runtime-contract-conformance-audit.md`
- `workspace-concurrency-loadtest-plan.md`
- `workorch-tui-agent-acceptance.md`
- `workorch-tui-display-closure-tracking.md`
- `next-mvp-acceptance.md`
- `next-stage-demand-candidates.md`
- `auto-governance-ticket-approval-plan.md`
- `model-aware-llm-context-budget-todo.md`
- `software-dev-dod-repair-closure-plan.md`
- `software-requirement-development-orchestration-plan.md`
- `workorch-unified-tui-console-migration-plan.md`
- `advanced-runtime-scheduling-plan.md`
- `mvp-backlog.md`
- `next-stage-p1-development-plan.md`
- `refactor-progress.md`
- `runtime-expansion-plan.md`
- `single-workspace-performance-optimization-record.md`
- `workorch-tui-improvements-absorption-plan.md`
- `workspace-data-layout-plan.md`

完成标志：

- 主阅读路径只剩 core / domain / flow / module / profile / product / verification
- 历史计划和 superseded 文档全部退出主入口

## 5.1 当前剩余收口主题

当前仍需继续处理的重点是：

- `40-profiles-capabilities/` 中的逐模板 Soul 细分与更多交叉引用
- `50-product-interface/` 中 service-api / TUI / deployment 的第二轮细化
- `60-verification/` 中 core/profile verifier 的脚本入口、断言矩阵与映射
- `90-archive/` 中 archive/source note/旧引用的最终清理
- 历史综合状态入口的最后一批过渡引用清理与 eventual supersede

## 6. 现有文档迁移映射表

以下映射表用于逐项收口。若单份旧文档需要拆分到多个目标文件，则优先抽取稳定内容，再归档旧文档。

| 现文件 | 目标位置 | 处理策略 | 说明 |
| --- | --- | --- | --- |
| `advanced-runtime-scheduling-plan.md` | `90-archive/completed/advanced-runtime-scheduling-plan.md` | 归档 | 已完成阶段计划 |
| `auto-governance-ticket-approval-plan.md` | `70-roadmap/active/auto-governance-ticket-approval-plan.md` | 迁移 | 活跃计划 |
| `builtin-templates-and-souls-design.md` | `40-profiles-capabilities/builtin-templates-and-souls.md` | 重写迁移 | profile/capability 主入口 |
| `codex-like-llm-prompt-context-design.md` | `20-runtime-flow/context-assembly-flow.md` | 内容迁移 | 稳定的上下文组织规则进入 flow |
| `deployment.md` | `50-product-interface/deployment.md` | 重命名迁移 | 产品接口层 |
| `design-implementation-audit-2026-05-12.md` | `60-verification/audits/design-implementation-audit-2026-05-12.md` | 迁移 | 审计 |
| `graph-followup-dod-assessment-closure-plan.md` | `20-runtime-flow/verification-acceptance-flow.md` + `graph-followup-dod-assessment-closure-plan.md` | 拆分归档 | 稳定规则进入 flow |
| `llm-decision-memory-improvement-plan.md` | `10-domain-model/context-capability-memory.md` + `20-runtime-flow/context-assembly-flow.md` + `llm-decision-memory-improvement-plan.md` | 拆分 | 规则沉淀后仅保留活跃待办 |
| `llm-driven-development-plan.md` | `20-runtime-flow/*` + `90-archive/completed/llm-driven-development-plan.md` | 拆分归档 | 主要是历史实现记录 |
| `llm-driven-workspace-execution-design.md` | `20-runtime-flow/planning-execution-flow.md` | 内容迁移 | execution 主链设计 |
| `mcp-capability-management-design.md` | `40-profiles-capabilities/mcp/capability-management.md` | 迁移 | capability 层 |
| `model-aware-llm-context-budget-design.md` | `30-module-architecture/llm-provider-assembly.md` | 合并迁移 | LLM provider/预算实现说明 |
| `model-aware-llm-context-budget-todo.md` | `70-roadmap/active/llm-context-budget-todo.md` | 迁移 | 活跃 TODO |
| `mvp-backlog.md` | `90-archive/completed/mvp-closure-record.md` | 归档 | MVP 收口记录 |
| `next-mvp-acceptance.md` | `70-roadmap/active/next-mvp-acceptance.md` | 迁移 | 未来验收目标 |
| `next-stage-demand-candidates.md` | `70-roadmap/candidates/next-stage-demand-candidates.md` | 迁移 | 候选项 |
| `next-stage-p1-development-plan.md` | `90-archive/completed/p1-development-record.md` | 归档 | 已完成 |
| `next-stage-result-event-development-plan.md` | `10-domain-model/run-step-result-artifact.md` + `10-domain-model/event-observability.md` + `90-archive/completed/result-event-development-plan.md` | 拆分归档 | 稳定模型沉淀 |
| `refactor-progress.md` | `90-archive/superseded/refactor-archive.md` | 归档 | archive marker |
| `run-closure-quality-improvement-plan.md` | `20-runtime-flow/verification-acceptance-flow.md` + `70-roadmap/active/run-closure-quality-improvement-plan.md` | 拆分 | 稳定规则沉淀 |
| `runtime-contract-conformance-audit.md` | `60-verification/audits/runtime-contract-conformance-audit.md` | 迁移 | runtime contract audit |
| `runtime-expansion-plan.md` | `90-archive/completed/runtime-expansion-plan.md` | 归档 | 已完成 |
| `runtime-recovery-governance-closure-design.md` | `20-runtime-flow/repair-replan-flow.md` + `90-archive/completed/runtime-recovery-governance-closure-design.md` | 拆分归档 | 稳定规则进入 flow |
| `single-workspace-performance-optimization-record.md` | `90-archive/records/single-workspace-performance-optimization-record.md` | 归档 | 性能记录 |
| `software-dev-code-agent-closure-plan.md` | `40-profiles-capabilities/software-dev/code-agent.md` + `90-archive/completed/software-dev-code-agent-closure-plan.md` | 拆分归档 | stable profile rule + 历史 closure |
| `software-dev-dod-repair-closure-plan.md` | `70-roadmap/active/software-dev-dod-repair-closure-plan.md` | 迁移 | 活跃专项 |
| `software-dev-goal-coverage-template-closure-design.md` | `40-profiles-capabilities/software-dev/goal-coverage-and-dod.md` | 迁移 | software-dev 主设计 |
| `software-dev-lsp-bridge-integration-design.md` | `40-profiles-capabilities/software-dev/lsp-bridge-integration.md` | 迁移 | software-dev 能力扩展 |
| `software-requirement-development-orchestration-plan.md` | `70-roadmap/active/software-requirement-development-orchestration-plan.md` | 迁移 | 活跃主计划 |
| `structure.md` | `30-module-architecture/repository-structure.md` | 重命名迁移 | 模块/目录结构 |
| `workorch-architecture-design.md` | `30-module-architecture/architecture-overview.md` | 重写迁移 | 只保留稳定模块架构 |
| `workorch-cli-tui-agent-requirements.md` | `50-product-interface/tui/requirements.md` | 迁移 | TUI 主需求文档 |
| `workorch-codex-request-context-comparison.md` | `20-runtime-flow/context-assembly-flow.md` + `60-verification/audits/codex-request-context-comparison.md` | 拆分 | 中立规则沉淀 + 对标审计保留 |
| `workorch-console-bundle-ui-optimization-experience.md` | `50-product-interface/web-console/bundle-ui-optimization-record.md` | 迁移 | Web Console 记录 |
| `workorch-full-chain-assessment-closure-plan.md` | `20-runtime-flow/overview.md` + `60-verification/audits/full-chain-assessment-closure-plan.md` | 拆分 | flow 总体问题进入 runtime-flow，评审留在 audit |
| `workorch-llm-auto-context-profile-design.md` | `30-module-architecture/llm-provider-assembly.md` | 合并迁移 | provider/profile 组装说明 |
| `workorch-llm-request-cache-aware-context-design.md` | `00-core/runtime-request-cache-note.md` | 重命名迁移 | core 从属 request note |
| `workorch-magiccommon-context-first-upgrade.md` | `30-module-architecture/application-workorchd.md` + `90-archive/completed/magiccommon-context-first-upgrade.md` | 拆分归档 | 稳定接线规则保留 |
| `workorch-runtime-stage-contract-neutrality-design.md` | `00-core/runtime-stage-contract.md` | 重构迁移 | 唯一核心 contract |
| `workorch-runtime-storage-json-control-design.md` | `10-domain-model/event-observability.md` + `30-module-architecture/storage-readmodel.md` | 拆分 | 对象/存储分离 |
| `workorch-soul-md-default-resource-design.md` | `40-profiles-capabilities/workspace/soul-md-default-resource.md` | 迁移 | workspace profile 资源约定 |
| `90-archive/superseded/workorch-status.md` | `01-overview/current-state.md` + `50-product-interface/service-api.md` + `60-verification/service-acceptance.md` + `90-archive/records/delivery-history.md` | 拆分 | 当前事实、接口、验收、历史分离 |
| `workorch-tui-agent-acceptance.md` | `60-verification/tui/tui-agent-acceptance.md` | 迁移 | TUI 验收 |
| `workorch-tui-display-closure-tracking.md` | `60-verification/tui/display-closure-tracking.md` | 迁移 | TUI closure 跟踪 |
| `workorch-tui-display-refresh-consistency-design.md` | `50-product-interface/tui/display-refresh-consistency-design.md` | 迁移 | TUI 产品交互设计 |
| `workorch-tui-improvements-absorption-plan.md` | `90-archive/completed/tui-improvements-absorption-plan.md` | 归档 | 已完成 |
| `workorch-tui-interaction-contract.md` | `50-product-interface/tui/interaction-contract.md` | 迁移 | TUI contract |
| `workorch-unified-tui-console-migration-plan.md` | `70-roadmap/active/unified-tui-console-migration-plan.md` | 迁移 | 活跃迁移计划 |
| `workorch-workspace-deliverable-boundary-design.md` | `10-domain-model/workspace-goal-dod.md` + `30-module-architecture/storage-readmodel.md` | 拆分 | 领域边界与实现落点分离 |
| `workspace-builtin-git-tool-capability-plan.md` | `40-profiles-capabilities/runtime-capabilities/builtin-git-tool.md` | 迁移 | capability 文档 |
| `workspace-concurrency-loadtest-plan.md` | `60-verification/loadtest-baseline.md` + `90-archive/records/workspace-concurrency-loadtest-record.md` | 拆分 | 当前压测基线与历史复盘分离 |
| `workspace-data-layout-plan.md` | `90-archive/superseded/workspace-data-layout-plan.md` | 归档 | 已被新边界设计取代 |
| `workspace-file-diff-patch-capability-plan.md` | `40-profiles-capabilities/runtime-capabilities/file-diff-patch.md` | 迁移 | capability 文档 |
| `workspace-memory-recall-traceability-plan.md` | `10-domain-model/context-capability-memory.md` + `20-runtime-flow/context-assembly-flow.md` | 拆分 | 对象/flow 分离 |
| `workspace-setup-draft-session-plan.md` | `40-profiles-capabilities/workspace/setup-draft-session.md` | 迁移 | workspace profile 能力设计 |
| `workspace-soul-routing-plan.md` | `40-profiles-capabilities/workspace/soul-routing.md` | 迁移 | workspace profile 路由 |
| `workspace-web-search-capability-plan.md` | `40-profiles-capabilities/runtime-capabilities/web-search.md` | 迁移 | capability 文档 |

## 7. 收口规则

每次迁移一个文档，必须同时完成：

1. 明确该文档的新角色。
2. 判断是直接迁移、拆分迁移，还是归档。
3. 更新其被引用位置。
4. 清理旧路径上的“并列权威”表述。
5. 在 `README.md` 更新新的入口路径。

禁止出现以下情况：

- 已迁移后的旧文档仍被 README / AGENTS / 核心设计入口当作权威文档引用。
- 新旧文档同时声明自己是 runtime core contract。
- profile/capability 文档反向定义 core contract。
- status / progress / audit / record 再次混入当前权威设计入口。

## 8. 收口完成标准

文档体系重组完成时，应满足：

- 任何 runtime core 问题都只能回到一份核心 contract 裁决。
- 任何业务对象问题都能在 `10-domain-model/` 找到权威定义。
- 任何 runtime 主链路问题都能在 `20-runtime-flow/` 找到权威说明。
- 任何模块责任或重构问题都能在 `30-module-architecture/` 找到落点。
- 任何 software-dev / workspace / MCP / git / web-search 特化规则都只能在
  `40-profiles-capabilities/` 解释。
- 任何 TUI/Web/API/部署问题都只能在 `50-product-interface/` 查找。
- 任何验收、audit、loadtest、verifier 问题都只能在 `60-verification/` 查找。
- 已完成计划、superseded 文档、历史记录全部退出主阅读路径。
