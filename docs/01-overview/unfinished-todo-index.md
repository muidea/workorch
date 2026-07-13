# WorkOrch Docs 未完成待办项索引

Status: active
Type: index
Last Updated: 2026-07-09

## Purpose

本文件是 `docs/` 目录下所有 markdown 文档中"未完成的待办项"的统一索引，覆盖 14 个子目录（含子目录）共 170+ 个文档。
目的是为后续逐项收口提供单点入口，避免再次在多个 batch/audit/tracker 文档中分散查找。

> **使用提示**
> - 优先级标识规则：本文保留原文档已标注的优先级（P0/P1/P2/P3、Active Follow-up TODO、Remaining Follow-Up、Residual 等），未标注项统一标记为「未标注」。
> - 状态标识规则：保留原文档的 `Status: active` / `Status: partial` / `Status: pending` / `Status: code-closed` / `Status: implemented` / `Status: closed` / `Status: draft` / `Status: completed` / `Status: superseded` / `Status: deferred` 等口径。
> - 排除规则：所有文档的 `[x]` 已勾选项、`Status: completed` 全清单、`Status: archived` 历史归档条目均不计入；只保留真正尚未完成的待办。

## 阅读导航

1. [A. 文档迁移与编辑性收口（最高优先）](#a-文档迁移与编辑性收口最高优先)
2. [B. Core 合约层（00-core）](#b-core-合约层00-core)
3. [C. Overview / 状态层（01-overview）](#c-overview--状态层01-overview)
4. [D. 产品层（05-product）](#d-产品层05-product)
5. [E. Domain Model 层（10-domain-model）](#e-domain-model-层10-domain-model)
6. [F. Runtime Flow 层（20-runtime-flow）](#f-runtime-flow-层20-runtime-flow)
7. [G. Module Architecture 层（30-module-architecture）](#g-module-architecture-层30-module-architecture)
8. [H. Profiles / Capabilities 层（40-profiles-capabilities）](#h-profiles--capabilities-层40-profiles-capabilities)
9. [I. Product Interface 层（50-product-interface）](#i-product-interface-层50-product-interface)
10. [J. Verification 层（60-verification）](#j-verification-层60-verification)
11. [K. Active Roadmap（70-roadmap/active）](#k-active-roadmap70-roadmapactive)
12. [L. Roadmap Candidates（70-roadmap/candidates）](#l-roadmap-candidates70-roadmapcandidates)
13. [M. Archive（90-archive）](#m-archive90-archive)

## A. 文档迁移与编辑性收口（最高优先）

来源：`docs-domain-module-flow-reorganization-plan.md` 与 `01-overview/incomplete-and-connection-gap-tracker.md`，属文档迁移顶层收口任务（不针对具体功能/产品）。

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| A1 | 目录正文细化 | `40-profiles-capabilities/` 中逐模板 Soul 细分与更多交叉引用待继续 | 当前重点 |
| A2 | 目录正文细化 | `50-product-interface/` 中 `service-api` / TUI / deployment 第二轮细化 | 当前重点 |
| A3 | 目录正文细化 | `60-verification/` 中 core/profile verifier 脚本入口、断言矩阵与映射细化 | 当前重点 |
| A4 | 目录正文细化 | `90-archive/` 中 archive/source note/旧引用最终清理 | 当前重点 |
| A5 | 历史状态入口 | 历史综合状态入口最后一批过渡引用清理与 eventual supersede | 当前重点 |
| A6 | 旧路径互引 | 旧路径互引继续清理 | 当前重点 |
| A7 | 审计收口 | 审计/tracker/overview 最终收口 | 当前重点 |
| A8 | 优先顺序 1 | `runtime-stage-contract.md` 中剩余 grouped boundary/checklist 文案评估；优先迁到 `20-runtime-flow/`，再迁到 `30-module-architecture/` | 优先顺序 1 |
| A9 | 优先顺序 2 | context/request rendering 相关文档分层 | 优先顺序 2 |
| A10 | 优先顺序 3 | TUI / MCP / workspace 这类跨 profile-module-product 的重叠主题 | 优先顺序 3 |
| A11 | 优先顺序 4 | 逐项压缩剩余历史 audit / archive 说明，完成真正归档 | 优先顺序 4 |

## B. Core 合约层（00-core）

来源：`docs/00-core/` 下 2 个文档。

| # | 文件 | 主题/章节 | 待办项 | 优先级 | 状态 |
|---|------|-----------|--------|--------|------|
| B1 | `00-core/runtime-stage-contract.md` | 极少量 grouped boundary/checklist 文案 | 继续压缩迁移到 flow/module | 编辑性收尾 | — |
| B2 | `00-core/runtime-stage-contract.md` | checklist / audit inventory + 少量 acceptance/coverage closeout 例外条目 | 后续主要是最后一轮文案压缩与是否继续降层的取舍 | 编辑性收尾 | — |

## C. Overview / 状态层（01-overview）

来源：`docs/01-overview/` 下 11 个文档；`docs/docs-domain-module-flow-reorganization-plan.md`。

### C1. `current-state.md` Active Follow-up TODO

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| C1-1 | Architecture drift | 架构文档相对代码树仍有明显滞后，后续以新 `30-module-architecture/` 持续收口 | 未标注 |
| C1-2 | Recovery governance | 恢复治理设计与实现大体一致，但 duplicate failure、预算数值、自动恢复路径等说明仍需继续细化 | 未标注 |
| C1-3 | Activation response | 补 `execution-activations` 的 latency/size regression coverage | Active Follow-up TODO |
| C1-4 | Activation response | 增加 activation critical path observability | Active Follow-up TODO |
| C1-5 | Verification isolation | 将共享 LLM 配置锁扩展到其他会改写全局 provider 配置的脚本 | Active Follow-up TODO |
| C1-6 | Verification isolation | 评估引入 per-test provider namespace / config overlay | Active Follow-up TODO |
| C1-7 | Read-model cost control | 降低脚本和 UI 对 full workspace snapshot 的默认依赖 | Active Follow-up TODO |
| C1-8 | Read-model cost control | 评估 workspace board/detail 是否统一把 activation response 视为 lightweight acknowledgement | Active Follow-up TODO |

### C2. `current-state.md` Migration Progress Snapshot

| # | 主题 | 待办项 |
|---|------|--------|
| C2-1 | 收尾中 | 少量对象边界、规则和示例补细 |
| C2-2 | 收尾中 | overview / archive / history 文档最终压缩 |
| C2-3 | 未完成但已收缩到尾项 | 少量历史 audit 语境与旧引用兼容说明 |
| C2-4 | 未完成但已收缩到尾项 | 个别文档中的示意/产物路径样例进一步规范化 |

### C3. `current-unclosed-matrix.md` Residual Editorial Follow-up

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| C3-1 | `runtime-stage-contract.md` | 极少量 grouped boundary/checklist 文案继续压缩 | Residual Editorial Follow-up；非结构迁移阻塞 |
| C3-2 | 历史追溯 | `01-overview` / `90-archive` 中的少量历史追溯旧引用继续压缩 | Residual Editorial Follow-up |
| C3-3 | 兼容别名 | 根目录兼容别名是否物理移除属于外部兼容策略决定 | Residual Editorial Follow-up |

### C4. `incomplete-and-connection-gap-tracker.md` A. 内容不完整

| # | 主题 | 待办项 |
|---|------|--------|
| C4-1 | Core contract | `00-core/runtime-stage-contract.md` 的最后少量深嵌细节 |
| C4-2 | Roadmap/Archive | `70-roadmap/*` / `90-archive/*` 的历史压缩与最终归档时机 |
| C4-3 | 文档 | 少量模块/产品文档的更细实现样例补充 |
| C4-4 | 总入口 | `README.md` 后续随目录继续调整时的小幅导航维护 |
| C4-5 | current-state | `current-state.md` 少量活跃 TODO 与收尾时机判断的持续更新 |
| C4-6 | Domain Model | flow / read-model 层继续细化 definition change impact |
| C4-7 | Domain Model | flow / product 层对结果摘要映射继续补细 |
| C4-8 | Domain Model | flow 层动作表继续细化 |
| C4-9 | Domain Model | 模块实现与字段级 API 继续补细 |
| C4-10 | Domain Model | flow / profile 层继续补细 selectedContext 继承与 context UX |
| C4-11 | Runtime Flow | profile-specific setup/template 继续补细 |
| C4-12 | Runtime Flow | 模块层更细 step kind contract 继续补充 |
| C4-13 | Runtime Flow | outcome/taskProgress 与 completion gate 的更细读模型映射 |
| C4-14 | Runtime Flow | request audit 与 cachePlan 的模块实现细节 |
| C4-15 | Runtime Flow | evidence/result 的模块与产品层具体打开方式 |
| C4-16 | Module | kernel 相关模块文档更细实现继续演进 |
| C4-17 | Module | `blocks-runtime-adapters.md` 更细模块实现与 approval block 边界继续校正 |
| C4-18 | Module | `storage-readmodel.md` 更细实现演进 |
| C4-19 | Module | `application-workorchd.md` 更细实现拆分继续演进 |
| C4-20 | Module | `llm-provider-assembly.md` 更细实现拆分继续演进；`runtime-request-cache-note.md` 剩余主要是 `llm-provider-assembly.md` 自身的实现落点继续补细 |
| C4-21 | Profiles | `builtin-templates-and-souls.md` 个别模板更细正文继续补细 |
| C4-22 | Profiles | software-dev 更细 fixture 表与跨产品示例补充 |
| C4-23 | Profiles/workspace | deliverable boundary 的 profile-facing 说明 |
| C4-24 | Profiles/workspace | memory recall / context UX 的 profile-facing 说明 |
| C4-25 | Profiles | `soul-md-default-resource.md` 更细实现样例继续补细 |
| C4-26 | Profiles | `mcp/capability-management.md` 少量更细跨产品示例补充 |
| C4-27 | Profiles | `runtime-capabilities/` 少量更细跨产品示例补充 |
| C4-28 | Product TUI | 更细异常路径样例 |
| C4-29 | Product Web | 页面字段级更细示例 |
| C4-30 | Product Web | 个别复杂表单的逐字段样例 |
| C4-31 | Product Deployment | 更细环境样例补充 |
| C4-32 | Verification overview | service / verifier / audit / tui / loadtest 之间的更细映射 |
| C4-33 | Verification overview | 从 overview 直达脚本入口和 fixture/deployed 对应关系 |
| C4-34 | Verification | `service-acceptance.md` 更细的 deployed shell 逐页样本与 SSE 断线补偿样例 |
| C4-35 | Verification | `loadtest-baseline.md` 更细历史轮次索引 |
| C4-36 | Verification | `loadtest-baseline.md` 与 records 中单工作空间优化记录的更细边界样例 |
| C4-37 | Verification | core/profile verifier 后续若继续补写，主要只剩穷举式索引增强 |
| C4-38 | Verification TUI | 更细异常路径样例与少量交叉引用压缩 |
| C4-39 | Roadmap/Archive | 个别 active roadmap plan 内偏厚背景/基线正文继续压缩 |
| C4-40 | Roadmap/Archive | 旧路径互引清理 |
| C4-41 | Roadmap/Archive | 少量最终归档时机判断 |
| C4-42 | Archive | `90-archive/records/overview.md` 当前主要剩余是其余 records 类旧文档的正式迁移 |

### C5. `incomplete-and-connection-gap-tracker.md` B. 衔接断点

| # | 主题 | 待办项 |
|---|------|--------|
| C5-1 | Core 文档 | `runtime-stage-contract.md` 剩余 checklist/audit inventory 和少量 acceptance/coverage closeout 例外条目；编辑性收尾 |
| C5-2 | 旧文档 | 旧文档普遍不是单一职责；后续不能机械搬运，需要按"对象/flow/module/profile/product/verification/archive"重写式迁移 |
| C5-3 | Context 组织 | 后续需要统一拆成 core contract、runtime flow 视角、module implementation 视角、audit/比较材料 |
| C5-4 | Memory/Recall | 后续必须继续按层切开；当前 `context-capability-memory.md` 还不能单独替代旧 Memory 文档 |
| C5-5 | Profile/Capability 与 Module/Product | 边界仍有重叠，需要继续补充跨层链接和落点说明 |
| C5-6 | Roadmap/Audit/Archive | 第二阶段迁移仍需逐项清理旧互引，并继续收口 superseded/records 类旧正文 |

### C6. `incomplete-and-connection-gap-tracker.md` D. 当前后续动作建议

| # | 方向 | 待办项 |
|---|------|--------|
| C6-1 | 方向 1 | 围绕 `runtime-stage-contract.md` 做最后一轮 grouped 文案压缩 |
| C6-2 | 方向 2 | 围绕历史 audit/archive 做最后一批旧引用清理 |

### C7. `roadmap-archive-migration-audit.md` Remaining Gaps & Follow-up

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| C7-1 | 旧路径互引 | active roadmap 文档之间仍存在旧路径互引 | Follow-up 1 |
| C7-2 | completed archive | 仍有其余历史正文未正式迁入 | Follow-up 2 |
| C7-3 | 双份正文 | 已复制后的 completed 文档后续还需判断是否继续保留双份正文 | Follow-up 3 |

### C8. `information-model-alignment-audit.md` 下一轮建议

| # | 主题 | 待办项 |
|---|------|--------|
| C8-1 | 历史引用压缩 | 视需要继续压缩 `01-overview` / `90-archive` 中仅剩的历史追溯旧引用 |
| C8-2 | records 旧引用 | 继续收缩 `90-archive/records/*` 的旧路径互引和历史说明重复 |
| C8-3 | 兼容别名 | 最后决定旧综合状态入口兼容别名的最终处置时机 |

### C9. `architecture-optimization-plan.md` 预期效果（未达成目标）

| # | 主题 | 当前 | 目标 |
|---|------|------|------|
| C9-1 | `RuntimeOrchestrator` 方法数 | 约 71 | 约 60（核心 dispatch + lifecycle） |
| C9-2 | 子系统独立测试覆盖 | 部分完成 | 每个子系统可独立构造和测试 |
| C9-3 | 新增职责归属 | 已改善但未完全闭环 | 明确必须选择子系统，状态机写入保留协调层 |

## D. 产品层（05-product）

来源：`docs/05-product/` 下 4 个文档（不含空 `restructure/` 子目录）。**该层未发现显式未完成待办项**，所有文件均为已稳定的 active 合同/PRD/产品架构/业务模型与流程合同。

## E. Domain Model 层（10-domain-model）

来源：`docs/10-domain-model/` 下 7 个文档 + `batch-2-domain-migration-audit.md`。

### E1. `batch-2-domain-migration-audit.md` 1.x 处理结论

| # | 主题 | 待办项 |
|---|------|--------|
| E1-1 | workspace-goal-dod | 目录布局和实现落点后续继续迁到模块架构层 |
| E1-2 | Memory | Memory 的排序、refresh、repair 继承等动态规则后续迁到 flow 层 |
| E1-3 | Result/Artifact/Event | 细节差异和 API/read-model 行为冲突后续在 flow/module 层继续校正 |

### E2. `batch-2-domain-migration-audit.md` 2.x 内容不完整

| # | 主题 | 待办项 |
|---|------|--------|
| E2-1 | Goal/DoD revision | `definition revision` / `previous definition hash` / `definition change lineage` 三项的对象边界尚未独立建模 |
| E2-2 | Run 过程证据 | `ToolCall` / `Patch` / `ValidationResult` / `RuntimeCheckpoint` 缺更明确的最小字段边界；与 Step/Result/Artifact 的拥有关系 |
| E2-3 | Governance 领域 | 缺对象级说明：`duplicate failure` / `repair exhausted` / `auto-governance ticket` / `successor replan lineage` |
| E2-4 | Event/Outcome/TaskProgress | 缺：`outcome` 选择规则；`taskProgress` 与 latest result/run 的约束；`event payload summary` 与 `raw payload ref` 的对象层说明 |
| E2-5 | MemoryRecallTrace | 缺：recall 排序策略；run 优先与类型配额；semantic recall 规则细化 |

### E3. `batch-2-domain-migration-audit.md` 3.x 衔接断点

| # | 主题 | 待办项 |
|---|------|--------|
| E3-1 | 旧文档 | 旧文档普遍混有实施任务/验收脚本/历史进度/API/Web/TUI 展示要求，领域层无法直接机械迁移 |
| E3-2 | 综合状态文档 | `Formal Event Boundary`、`Task Progress And Recovery Projection` 等内容仍分散在旧综合状态文档中 |
| E3-3 | Memory/Recall | 动态行为与静态对象边界仍缠绕，必须等 `20-runtime-flow/` 建立后再继续拆分 |

### E4. `event-observability.md` Topic Granularity 约束

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| E4-1 | 离散 topic SLA | 若后续产品需要把 `/run/completed`、`/step/succeeded`、`/approval/approved` 作为订阅 SLA，必须同步补 publisher、replay 顺序、projection 幂等和事件测试 | 条件性（仅当产品决定订阅 SLA 时触发） |

## F. Runtime Flow 层（20-runtime-flow）

来源：`docs/20-runtime-flow/` 下 8 个文档（含 `batch-3-flow-migration-audit.md`）。

### F1. `batch-3-flow-migration-audit.md` 2.x 内容不完整

| # | 主题 | 待办项 |
|---|------|--------|
| F1-1 | Activation flow | definition change impact、successor run activation 继承策略、activation 与 workspace setup/template 入口的差异 |
| F1-2 | Planning/Execution | patch preview/apply/revert、file/git/web capability 细化、graph node 并发约束 |
| F1-3 | Verification/Acceptance | missingDoDItems、claim-to-evidence alignment、current-run evidence gate、coverageWorkItems ledger |
| F1-4 | Context Assembly | step input refresh、provider retry/repair/replan 的 memory 继承、request audit 与 cachePlan 关系、selectedContext 继承排序与 recall policy |
| F1-5 | Evidence/Result | raw payload ref 分类、event payload summary contract、final result summary schema、artifact manifest 分类与 profile 边界 |

### F2. `batch-3-flow-migration-audit.md` 3.x 衔接断点

| # | 主题 | 待办项 |
|---|------|--------|
| F2-1 | Core 文档 | 多数 flow 规则仍深嵌在 `00-core/runtime-stage-contract.md`；flow 层只能先建立外层骨架 |
| F2-2 | closure 语义 | `70-roadmap/active/run-closure-quality-improvement-plan.md` 仍需继续拆到 flow 和 verification |
| F2-3 | software-dev | software-dev 特化仍可能污染 flow 输入材料 |

### F3. 关键 active 文档

| # | 文件 | 待办项 | 状态 |
|---|------|--------|------|
| F3-1 | `context-assembly-flow.md` | selectedContext refresh 细则与 ordering 展开；持久化/事件通知具体落点 | active |
| F3-2 | `activation-definition-flow.md` | definition change impact、successor activation inheritance 未提供落点/owner 责任分工 | active |
| F3-3 | `evidence-result-flow.md` | Raw Payload Ref / Event Payload Summary / Final Result Summary / Artifact Manifest 都用"当前方向"表达，未给出最终合同表 | active |
| F3-4 | `planning-execution-flow.md` | incrementalArtifact 输出模式内部 selector 阈值/schema/request-side 细节尚未在本文件内闭合 | active |
| F3-5 | `repair-replan-flow.md` | Decision Resolution Resume Rule、Cross-Module Recovery Invariants 缺少对原 90-archive completed 文档的逐项对齐记录 | active |
| F3-6 | `verification-acceptance-flow.md` | `missingDoDItems`、`coverageWorkItems` 未给出具体投影/审计族代码落点对应 | active |

## G. Module Architecture 层（30-module-architecture）

来源：`docs/30-module-architecture/` 下 23 个文档。

### G1. `llm-cache-improvement-design.md`（Status: draft）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G1-1 | P4 架构演进 | P4-1 Provider Adapter 抽象拆分、P4-2 Cache 预热、P4-3 跨 run cache key 均未落地（数据驱动决策） | draft |
| G1-2 | OpenAI SDK | 跟进 OpenAI SDK Chat Completions 的 `prompt_cache_key` wire 承载 | draft |
| G1-3 | 新增 surface | 新增 provider surface 时补 focused wire-level test | draft |
| G1-4 | Open Questions | 5 项待定：OpenAI SDK 类型化 prompt_cache_key 支持；HTTP protocol surface 字段；ai-proxy 透传行为；cache capability 存储方式；cache efficiency summary 展示落点 | draft |

### G2. `batch-4-module-migration-audit.md` 2.x & 3.x

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| G2-1 | kernel 分组 | run evidence 子对象的实现文档继续细化（非阻塞） | 未明确 |
| G2-2 | block adapter | `patch/*` 仍是可选后续迁移项；若迁移到 `blocks/file`，必须同时迁移 approval、patch evidence、repair contract 和 result gate 关联能力 | 可选迁移 |
| G2-3 | 内容断点 | 旧文档大量混入历史实现记录；旧综合状态文档仍分散部分模块落点事实；provider request rendering 规则仍深埋 core/flow 文档 | 持续主题 |

### G3. `framework-eventhub-structure-migration-plan.md`（无 Status 字段）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G3-1 | Phase 6 appservice split | mandatory closure criteria 已满足；剩余 appservice 编排文件为 optional cleanup（owner command orchestration + 较大 use-case 拆分） | optional maintenance |
| G3-2 | Current Baseline | `internal/modules/application/workorchd/biz/appservice` 仍是 facade 包；部分编排 facade 仍需渐进拆分 | 渐进 |

### G4. `eventhub-lane-policy-and-module-event-contract-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G4-1 | Phase 1/2 段尾 | 完成全量 `go test ./... -count=1` 和 `make verify-managed-full` 后再更新 skill | 待续 |
| G4-2 | topics_* alias | 迁移期可在 `internal/pkg/events` 保留 alias；最终 `topics_*.go` 应删除或只保留非业务通用 topic；当前已删除但范围描述作为长期归属边界保留 | 迁移期 alias 保留策略 |

### G5. `runtime-execution-event-queue-state-loop-design.md`（Status: implemented/validated）Open Questions

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G5-1 | queue 归属 | runtime execution queue 归 `blocks/pkg/runtimeevent` 还是 `kernel/runtimeexec` | Open |
| G5-2 | payload | runtime result event payload 是否直接携带完整 `StepExecutionResult.Output` | Open |
| G5-3 | patch apply | block worker 还是 orchestrator finalize（长期应保持 orchestrator finalize） | Open |
| G5-4 | 同步接口 | 是否保留同步 runtime adapter 接口（推荐保留为 worker 内部接口和单元测试接口） | Open |
| G5-5 | pending 持久化 | existing in-memory pending queue 是否持久化 | Open |

### G6. `run-resume-gap-confirmation-design.md`（Status: active）Open Questions

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G6-1 | GapBaseline owner | GapBaseline 是否正式 owner model | Open |
| G6-2 | TUI 命名 | restart 命令 TUI 命名（/run restart vs /run reset vs /run restart --new-run） | Open |
| G6-3 | assessment-only | assessment-only gap confirmation 使用正常 LLM block 还是独立 orchestrator validation stage | Open |
| G6-4 | Phase A TUI Resume Residual | 2026-06-21 Phase A TUI Resume Wording 显式 Residual Closure 段落 | Residual Closure |

### G7. `eventhub-driven-workorchd-state-loop-design.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| G7-1 | Phase 3 Scheduler Work Item Model | 待实施 | 未明确 |
| G7-2 | Phase 4 REST/TUI/Web Command Path Migration | 待实施 | 未明确 |
| G7-3 | Phase 5 Verification | 待实施 | 未明确 |
| G7-4 | Open Q1 | REST command endpoint 同步/异步语义切换节奏 | Open |
| G7-5 | Open Q2 | restart 旧 active run supersede/cancel 默认策略 | Open |
| G7-6 | Open Q3 | pending queue 持久化实体必要性 | Open |
| G7-7 | Open Q4 | auto-governance resolved notification 直接触发 continuation 还是只发布 owner notification | Open |

### G8. `goal-dod-nextaction-delivery-state-machine-design.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| G8-1 | 6 类持续现象 | run 长时间无符合 Goal 的产物；合同违例处理缺少统一收敛路径；evidence/coverage ledger 入参不稳；首次/resume/replan/restart 入口语义不统一；端侧与 workorchd projection 边界需固化；多 workspace 并发下 CPU/IO 压力 | 未明确 |
| G8-2 | Open Item 1 | CoverageLedger 与 EvidenceLedger 最终存储 owner/repository 边界 | Open |
| G8-3 | Open Item 2 | StagePacket 是独立持久化对象还是动态重建 | Open |
| G8-4 | Open Item 3 | NextAction 是否独立 event 类型 | Open |
| G8-5 | Open Item 4 | 旧 continue 语义在 REST/TUI/CLI 的清理范围 | Open |
| G8-6 | Open Item 5 | service restart 后 recoverable state projection 命名和触发语义 | Open |
| G8-7 | Open Item 6 | watchdog 预算/分类/补偿 key | Open |
| G8-8 | Open Item 7 | software-dev template/profile 策略字段与现有 souls/templates 的映射方式 | Open |

### G9. `mvvp-workspace-viewmodel-architecture.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G9-1 | partial detail / `projection.state=building` 边界条件 | 需继续观察 projection 完成后的强一致语义 | active |

### G10. `application-workorchd.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| G10-1 | Lifecycle | 已迁移到 magicCommon 的 context-first lifecycle；长期方向引用 `workorchd-state-loop-and-projection-closure-plan.md#p24` | active |
| G10-2 | Startup Recovery | watchdog 属于"运行期扩展"，长期方向是把 server-local scheduler pending 进一步持久化或 owner 化 | active |

## H. Profiles / Capabilities 层（40-profiles-capabilities）

来源：`docs/40-profiles-capabilities/` 下 13 个文档（含子目录）。核心未完成项集中在 `50-product-batch-5-audit.md`、`overview.md`、`builtin-templates-and-souls.md`。

### H1. `50-product-batch-5-audit.md` 1.x - 2.x

| # | 主题 | 待办项 |
|---|------|--------|
| H1-1 | builtin-templates | 旧模板/Soul 文档剩余"具体模板和 Soul 细节后续按模板继续细分" |
| H1-2 | deployment/status/acceptance | `deployment.md` 只保留部署和运行入口；"API 和 acceptance 后续继续拆分" |
| H1-3 | service-api | service-api.md 仍是占位入口，缺少完整资源清单、读写接口分组、detail/payload/paginated endpoint 说明 |
| H1-4 | TUI | workspace switching/monitoring、result-first display、decision/approval interaction、local vs remote mode 边界未收口 |
| H1-5 | Web Console | 缺：Web Console overview、关键页面/模型映射、Event/SSE 与 read model 的产品说明 |
| H1-6 | workspace profile | 缺：deliverable boundary profile-facing 说明、memory recall/context UX 说明 |
| H1-7 | workspace/soul-md | 仍缺与 bootstrap/verifier 的更细实现映射 |
| H1-8 | software-dev profile | 交叉链接仍不完整；缺与 verifier/module/product 层的更细交叉引用 |
| H1-9 | runtime-capabilities | step kind/adapter/read model 实现映射仍缺；approval/verifier/artifact 交叉引用仍缺 |
| H1-10 | mcp capability | 缺：capability catalog 与 application/read-model/UI 实现映射；审批策略、运行时展示和 verifier 交叉引用 |

### H2. `50-product-batch-5-audit.md` 3.x

| # | 主题 | 待办项 |
|---|------|--------|
| H2-1 | 旧综合状态文档清理 | 旧说明仍需继续清理（剩余问题主要是旧迁移说明和历史文档仍引用旧综合状态文档） |
| H2-2 | TUI 主题 | 产品要求、验收矩阵、closure 记录和 review 之间仍有剩余互引待清理 |
| H2-3 | profile/capability 与 module boundary | 仍有重叠，后续仍需在 flow/module 文档中补链路和落点 |

### H3. `overview.md` Current Gaps

| # | 主题 | 待办项 |
|---|------|--------|
| H3-1 | builtin-templates | 缺逐模板/逐 Soul 的进一步拆分 |
| H3-2 | software-dev/* | 缺更细 acceptance dimension -> fixture 对应表 |
| H3-3 | runtime-capabilities/* | 缺少量更细跨产品示例 |

### H4. `builtin-templates-and-souls.md` Open Migration Notes

| # | 模板 | 待办项 | 状态 |
|---|------|--------|------|
| H4-1 | code-review | TUI / code-review profile 后续专项验证 | 显式列出"后续" |
| H4-2 | doc-generator | 后续 doc-generator 专项 e2e | 显式列出"后续" |
| H4-3 | data-analysis | 后续 data-analysis 专项 e2e | 显式列出"后续" |
| H4-4 | generic-governed | 逐模板 Soul 细节仍在旧文档中，后续需继续按模板拆分 | Open Migration |
| H4-5 | weather-query | 同上 | Open Migration |

## I. Product Interface 层（50-product-interface）

来源：`docs/50-product-interface/` 下 11 个文档。

### I1. `service-api.md` Request Families Still To Detail

| # | 主题 | 待办项 |
|---|------|--------|
| I1-1 | 后续最值得继续拆的请求体 | `goalDoDQualityCheckRequest`、`createWorkspaceSetupDraftSessionRequest`、`appendWorkspaceSetupDraftMessageRequest`、`createSoulRequest`、`createWorkspaceTemplateRequest`、`createSkillRequest`、`createMCPServerRequest`、`updateMCPServerRequest`、`createMCPCapabilityRequest`、`createMemoryItemRequest` |
| I1-2 | Optional Further Detailing | 个别低频 workspaceSnapshotResponse / runObservabilityResponse 附属字段；个别 registry 请求体的实现枚举字段；个别 raw/SSE 错误样例 |
| I1-3 | Migration Note | 旧综合状态文档中的接口清单已落后于当前实际路由面；旧迁移说明的历史文档清理仍待收口 |

### I2. `overview.md` Current Status

| # | 主题 | 待办项 |
|---|------|--------|
| I2-1 | 字段级索引 | 后续若继续补写，主要只应是字段级索引、示例补细和少量产品样例增强 |

## J. Verification 层（60-verification）

来源：`docs/60-verification/` 下约 22 个文档（含 `audits/`、`tui/` 子目录）。

### J1. `service-acceptance.md` File Backend Reload Acceptance 当前仍缺

| # | 主题 | 待办项 |
|---|------|--------|
| J1-1 | 逐页样本 | 某些 detail page 在"真正重启后再次翻页"的逐页实例样本还没有专门 deployed script |
| J1-2 | 跨脚本样本 | raw payload ref 与 artifact uri 的跨脚本样本仍主要集中在 handler/file-backend fixture |

### J2. `verification-layer-code-alignment-checklist.md` Follow-Up

| # | 主题 | 待办项 |
|---|------|--------|
| J2-1 | 脚本报告索引 | TUI 文档场景编号比 `verify-tui-agent.sh` 的脚本化编号更完整；后续若需严格"一编号一脚本输出"，应只补脚本报告索引 |
| J2-2 | deployed shell 样本 | file backend reload 的 detail page "真正重启后再次翻页" 仍主要由 handler/file fixture 覆盖；如需 deployed shell 级样本，应在 `service-acceptance.md` 范围内补充 |

### J3. `product-interface-code-alignment-checklist.md`

| # | 主题 | 待办项 |
|---|------|--------|
| J3-1 | helpers.tsx 拆分 | `helpers.tsx` 仍是 Web 当前主要展示 helper；后续如继续拆分，应保持单一稳定模块 |

### J4. `domain-model-code-alignment-checklist.md`

| # | 主题 | 待办项 |
|---|------|--------|
| J4-1 | CoverageWorkItem 投影 | 后续修改投影结构前不得破坏现有 LLM context 投影顺序和 cache 稳定性 |

### J5. `incremental-artifact-code-review-checklist.md` Remaining Follow-Up

| # | 主题 | 待办项 |
|---|------|--------|
| J5-1 | continuation raw/detail UI | 后续如新增仍必须保持 metadata-first 主视图和显式 raw/detail 打开边界 |
| J5-2 | output mode selector | 后续如新增信号必须继续保持业务中立，不能按 template/语言/文件名或具体工具输出文本分支 |

### J6. `profile-capability-code-alignment-checklist.md` Remaining Follow-Up

| # | 主题 | 待办项 |
|---|------|--------|
| J6-1 | profile 专项 e2e | code-review / doc-generator / data-analysis 等更细粒度 profile 专项 e2e 仍属验证覆盖继续增强项 |
| J6-2 | capability 展示 | `selectedCapabilities` / `usedCapabilities` 后续可继续增强 UI 展示解释，但不应在前端重演 capability selection 规则 |

### J7. `runtime-flow-code-alignment-checklist.md`

| # | 主题 | 待办项 |
|---|------|--------|
| J7-1 | provider-facing projection | 后续修改前应保持现有 `Run.ExecutionContext.coverageWorkItems` 的稳定顺序与 provider cache 前缀规则 |

### J8. `acceptance-record-module-architecture.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| J8-1 | observed/presenter 覆盖 | observed/presenter 包已存在；read-model 大部分语义装配仍在 appservice；后续可按 snapshot/detail/observability 主语义装配是否继续分批从 appservice 根入口下沉到 readmodelprojection/readmodelports 的结构细化 | partial |

### J9. `audits/tui-agent-requirements-review-2026-05-03.md` Residual Risks

| # | 主题 | 待办项 |
|---|------|--------|
| J9-1 | screenshot diff | full terminal screenshot diff coverage 仍未建立 |
| J9-2 | backend method table | backend method / command coverage table 仍缺 |
| J9-3 | 多 session 生命周期 | 多本地 TUI session 的生命周期边界仍需进一步核定 |
| J9-4 | partial 任务 | CLI-AGENT-01/06/07/09/11/12/14 标为 partial 未完全收口 |

### J10. `audits/full-chain-assessment-closure-plan.md` Current Residual Themes

| # | 主题 | 待办项 |
|---|------|--------|
| J10-1 | completion expansion limit | completion follow-up expansion limit 与 provider retry 轻量化仍需持续守护 |
| J10-2 | 持续回归 | optional file read / patch preview repair / evidence quality gate 仍需持续回归 |
| J10-3 | 分发 | full-chain audit 的残余问题应继续分发到 runtime-flow、verification 和 profile 文档 |

### J11. `audits/runtime-contract-conformance-audit.md` 后续审计重点

| # | 主题 | 待办项 |
|---|------|--------|
| J11-1 | 继承链一致性 | follow-up / repair / replan 继承链的一致性 |
| J11-2 | 分流正确性 | context gate fail-fast 与 retry 之间的分流正确性 |
| J11-3 | 投影一致性 | workspace/run projection 对 blocker/coverage/result 的一致投影 |

### J12. `audits/design-implementation-audit-2026-06-01.md` 下次核对建议

| # | 主题 | 待办项 |
|---|------|--------|
| J12-1 | contractViolationType | 继续关注新增 runtime contract violation 是否同步写入 |
| J12-2 | context budget | 验证 LLM context budget 中 coverage summary 与 full ledger ref 的实际占比 |
| J12-3 | workspace snapshot | 验证 workspace snapshot coverage summary 与 run-scoped ledger 的一致性 |

### J13. `audits/overview.md`

| # | 主题 | 待办项 |
|---|------|--------|
| J13-1 | Current Audits 列表 | `web-console-api-parameter-alignment-review-2026-06-02.md` 已落位但 `audits/overview.md` "Current Audits" 链接列表尚未包含该文件 |

### J14. `batch-6-verification-roadmap-archive-audit.md` 2.x - 3.x

| # | 主题 | 待办项 |
|---|------|--------|
| J14-1 | verification 入口 | 还缺：服务验收命令分层、本地/file backend/deployed-instance 差异、与 deployment.md 分工说明、loadtest baseline 指标口径与脚本入口与 records 交叉引用、TUI acceptance/closure/review 之间剩余交叉引用清理 |
| J14-2 | core/profile verifier | 缺：脚本入口、具体断言项、与 e2e/unit/deployed verification 的映射 |
| J14-3 | active/candidates/archive | 缺：每个旧文档的正式迁移落位、从旧路径到新路径的引用切换、归档完成后的旧文档处理方式 |
| J14-4 | 旧综合状态文档 | 60-70-90 三层正文已建立，当前主要问题转为旧路径互引和历史说明清理 |
| J14-5 | 旧 roadmap/completed | single-workspace-performance-optimization-record.md、bundle-ui-optimization-record.md、workspace-concurrency-loadtest-record.md、多份 verification/audit/runtime 专题旧正文仍待收口；旧路径互引仍待清理 |
| J14-6 | archive 落点 | records 与剩余 completed 正文仍需后续批次逐项执行，否则旧文档仍在根目录参与主阅读路径 |

## K. Active Roadmap（70-roadmap/active）

来源：`docs/70-roadmap/active/` 下 156 个文档。本节按"具有显式未完成项"的文档进行汇总，已 `Status: completed` / `Status: code-closed` 全部收口的 goal-dod-nextaction-sX-Y 系列（94 个文档）不在此列出。

### K1. 跨多项 Plan 的 Live Verification Pending（最常见）

> 大量文档均已 code-closed 或 completed，但都需要"真实 live verification after redeploy"作为最后闭环，本索引统一归为 live verification pending 项。

涉及文档：
- `runtime-convergence-authority-final-fix-scope.md`（F8 Live Verification）
- `runtime-coverage-convergence-state-machine-closure-plan.md`（Live Verification 步骤 1-5）
- `runtime-current-action-projection-convergence-closure-plan.md`（Live verification）
- `runtime-current-live-convergence-improvement-plan.md`（Remaining live-only verification）
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`（P3 Live Verification）
- `runtime-deterministic-convergence-closure-plan.md`（Live Verification）
- `runtime-deterministic-ledger-assessment-closure-plan.md`（Live verification）
- `runtime-diagnostic-lineage-required-output-authority-closure-plan.md`（Live verification）
- `runtime-evidence-proof-strength-and-assessment-closure-plan.md`（E5-E9 live verification）
- `runtime-finalization-contract-evidence-resource-closure-plan.md`（Live verification）
- `runtime-graph-terminal-continuation-closure-plan.md`（Live Verification）
- `runtime-ledger-authority-dispatch-hardening-final-closure-plan.md`（Live verification）
- `runtime-ledger-binding-projection-authority-closure-plan.md`（Live verification）
- `runtime-live-authority-loop-full-chain-closure-plan.md`（Reopened Verification Matrix）
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`（Live verification）
- `runtime-nextaction-validation-to-statechange-closure-plan.md`（Live Verification）
- `runtime-progress-quality-resource-closure-plan.md`（Live verification）
- `runtime-required-output-authority-and-target-snapshot-closure-plan.md`（Live verification）
- `runtime-statechange-target-evidence-decision-gate-closure-plan.md`（Live Verification Plan）
- `runtime-statechange-target-snapshot-prerequisite-closure-plan.md`（Live verification）
- `runtime-statechanging-evidence-ledger-closure-plan.md`（Live verification after code closure）
- `runtime-structured-evidence-authority-closure-plan.md`（Live verification）
- `runtime-target-snapshot-authority-and-projection-closure-plan.md`（Live projection verification）
- `runtime-target-snapshot-statechange-preflight-closure-plan.md`（Live Verification）
- `runtime-terminal-ledger-nextaction-dispatch-contract-hardening-plan.md`（Live verification）
- `runtime-terminal-outcome-authority-closure-plan.md`（RTO-05 Live Verification）
- `runtime-validation-contract-gate-managed-session-closure-plan.md`（Live verification）
- `runtime-validation-evidence-context-closure-plan.md`（Live verification）
- `runtime-validation-evidence-quality-gate-closure-plan.md`（Live Verification Plan）
- `runtime-validation-required-output-authority-closure-plan.md`（Phase 6 Live 验证）

### K2. 真正 partial/pending 的子项

#### K2.1 `runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md`（Phase 1-8 全部 [ ] 未勾选）

| # | Phase | 待办项 | 优先级 |
|---|-------|--------|--------|
| K2.1-1 | Phase 1 | 将 shell/command 验证失败稳定归类为 rejected validation diagnostic | P0 |
| K2.1-2 | Phase 1 | diagnostic 字段完整：failureClass/failureSignature/validationFailureDiagnostic.class/recommendedEvidenceTargetType/coverageWorkItemIds/structuredTargetRefs/failureTargets/stderrPreview/stdoutPreview | P0 |
| K2.1-3 | Phase 1 | diagnostic 只能作为 rejected evidence 进入 ledger | P0 |
| K2.1-4 | Phase 1 | 对可行动诊断标为 auto-converge input | P0 |
| K2.1-5 | Phase 2 | validation failure diagnostic 写入后触发 current focus coverage recompute | P0 |
| K2.1-6 | Phase 2 | 当前 focus 仍缺 state_change 时派生为 need_state_change | P0 |
| K2.1-7 | Phase 2 | 禁止 stale acceptance/slice 覆盖当前 authority focus | P0 |
| K2.1-8 | Phase 3 | 禁止直接复用失败 step 的 runtimeStage/runtimeStageMode/repairObjective/stagePacket/requiredOutput/validation-only repairActionPackage | P0 |
| K2.1-9 | Phase 4 | nextAction=state_change 时 acceptedKinds 必须含 patch/apply；mustIncludeStateChangingNode=true | P0 |
| K2.1-10 | Phase 4 | nextAction=validate 时 acceptedKinds 只能含 validation 能力；mustProduceConcreteValidationEvidence=true | P0 |
| K2.1-11 | Phase 5 | dispatch 前增加硬不变量校验（contractState vs requiredOutput vs stagePacket vs repairActionPackage vs coverageFocus） | P0 |
| K2.1-12 | Phase 5 | repair dispatch 不得把 internal contract misalignment 直接转换成 terminal run failed no blocker | P0 |
| K2.1-13 | Phase 6 | validation failed diagnostic 后只允许四种结果 | P0 |
| K2.1-14 | Phase 6 | 对 recoverable diagnostic 禁止直接 run=failed no blocker | P0 |
| K2.1-15 | Phase 7 | run status/snapshot/projection 一致展示 | P0 |
| K2.1-16 | Phase 8 | 单测 + live 验证 validation command failed with actionable diagnostic | P0 |

#### K2.2 `runtime-state-transition-contract-hardening-closure-plan.md`（Done Criteria 全部 [ ]）

| # | Phase | 待办项 | 优先级 |
|---|-------|--------|--------|
| K2.2-1 | P0-1 | failed-step 唯一出口合同完成 | P0 |
| K2.2-2 | P0-2 | authority-derived snapshot hard gate 完成 | P0 |
| K2.2-3 | P0-3 | rejected diagnostic -> NextAction recompute 完成 | P0 |
| K2.2-4 | P0-4 | deterministic evidence acquisition 完成 | P0 |
| K2.2-5 | P0-5 | decision eligibility authority 完成 | P0 |
| K2.2-6 | P1 | Evidence/Projection Closure + Resource/Performance Closure | P1 |
| K2.2-7 | P2 | Live Verification Matrix（active/failed/blocked/terminal 4 类） | P2 |

#### K2.3 `runtime-current-action-projection-convergence-closure-plan.md`

| # | Phase | 待办项 | 优先级 | 状态 |
|---|-------|--------|--------|------|
| K2.3-1 | P1 | Live Chain Regression（state-change contract violation -> narrow retry -> accepted patch/governed blocker e2e fixture 仍未落地） | P1 | partial |
| K2.3-2 | P2 | Resource Guard（Projection metadata-only 路径与 scan-count regression 仍待落地） | P2 | partial |

#### K2.4 `runtime-live-convergence-gate-closure-plan.md`

| # | Phase | 待办项 | 状态 |
|---|-------|--------|------|
| K2.4-1 | LCG-06 | Validation Resource Lifecycle Cleanup 仍需 scoped cleanup/reconciliation from durable owner records | partial-closed |

#### K2.5 `runtime-managed-validation-resource-lifecycle-closure-plan.md`

| # | Phase | 待办项 | 状态 |
|---|-------|--------|------|
| K2.5-1 | M3 | Persistent owner-side resource ledger repository；Service startup orphan scan 记录 stale resources 为 diagnostics；Step/run terminal hooks outside shell-backed managed-session path | partially closed |
| K2.5-2 | M4 | template/profile policy field（如 preferManagedSessionForLongRunningValidation）；绑定到 workspace creation/activation | active |
| K2.5-3 | M5 | focus evidence-ready 时 select assessment/complete before new validation/follow-up graph | active |
| K2.5-4 | M6 | follow-up budget 绑定 current focus and NextAction；no-coverage delta 后 escalate to assessment/fail/governance by deterministic policy | active |

#### K2.6 `runtime-validation-nextaction-convergence-improvement-plan.md`

| # | Slice | 待办项 | 状态 |
|---|-------|--------|------|
| K2.6-1 | S3 | Broader typed NextAction cleanup 仍 pending | partial |
| K2.6-2 | S5 | Full focus-budget terminal blocker behavior remains out of this pass | partial |
| K2.6-3 | S6 | This pass did not modify evidence record binding/recompute | pending |
| K2.6-4 | S7 | This pass did not modify projection/ViewModel or live observer output | pending |

#### K2.7 `runtime-live-authority-loop-full-chain-closure-plan.md`（2026-06-30 Live Regression Reopen）

| # | Phase | 待办项 | 优先级 |
|---|-------|--------|--------|
| K2.7-1 | P0.6 | preserve state_change authority across prerequisites | P0 |
| K2.7-2 | P0.7 | runtime-owned target snapshot prerequisite exit | P0 |
| K2.7-3 | P0.8 | focus binding violation retry contract | P0 |
| K2.7-4 | P0.9 | ineffective retry budget + projection signal | P0 |
| K2.7-5 | P1.5 | resource guard for reopened loop | P1 |
| K2.7-6 | Coverage | TUI/Web/CLI command ingress for resume/restart/replan/decision/approval；EventHub delivery 与 owner consumption after command ingress；projection list/detail/ViewModel same-source invariants；evidence ledger metadata 完整性；Result/artifact/completion gate final authority；Provider request rendering + cache partition 稳定性；managed validation resource lifecycle；software-dev task granularity consistency；Repository/read-model indexes needed to remove hot full JSON reads | scope acknowledged but not covered |

### K3. `authority-live-verification-e2e-closure-design.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K3-1 | live workspaces | `ws-4e83810542ac4d65` / `ws-7ced70e98ea50c43` 仍 `waiting_decision`；latest service 未证明可驱动 valid replan/resume | active |
| K3-2 | P3 live verification | 来自 `convergence-authority-owner-state-continuation-closure-plan.md` 仍未编码为 reproducible e2e tests | active |
| K3-3 | P2 Persisted Request/Input Retention | partial-code-closed；per workspace/run/category persisted byte attribution 未实现 | partial-code-closed |
| K3-4 | retention policy | request/input 文件 retention policy 配置属单独 storage-policy item；不得通过 request-time full directory scans to `/metrics` 实现 | active |
| K3-5 | retry escalation | `ineffective_retry_runs=2` 显示 retry escalation 仍可能以 governance 结束 | active |

### K4. `auto-governance-ticket-approval-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| K4-1 | Auto Governance Agent | 独立运行单元和订阅链仍待补齐 | 未明确 |
| K4-2 | Ticket/Approval 风险分级 | 自动回答与 Approval 自动批准的风险分级/预算策略仍需继续固化 | 未明确 |
| K4-3 | 治理 trace | 自动治理 trace、read-model 投影和 Web/TUI 说明仍需补强 | 未明确 |
| K4-4 | e2e 覆盖 | e2e/verifier 对自动治理场景的专门覆盖仍可继续补齐 | 未明确 |

### K5. `convergence-authority-focus-drift-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| K5-1 | P0-1 ~ P0-4 / P1-1 / P1-2 / P2-1 | Required closure 未标 closed | P0-P2 |
| K5-2 | focus drift 展示 | Projection 可显示 current readiness 但尚未 surface focus drift | active |
| K5-3 | 5 项 Current risk | 1) coverage assessment 允许 LLM 标 work item 满足于 focus 之外；2) 旧 `acceptance.activeSlice` / `currentFocus` 可能残留；3) LLM input rendering 可能注入 stale global gap text；4) Repair/contract retry input 可能携带 stale fields；5) stale top-level `requiredOutput` 可能持续 | active |

### K6. `convergence-authority-owner-state-continuation-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K6-1 | P3 Live Verification | 5-step live verification（invalid `/`、valid `replan`、valid `provide_missing_info`、repeated validation contract violation、CPU/RSS + pprof）未执行 | pending |
| K6-2 | P2 acceptance | Observation: live verification pending | pending |
| K6-3 | managed path | Retry budget exhaustion 应创建 decision 仅在 current authority 无自动或 managed narrowed path；自动 narrowed path 已完成；managed path pending | pending |

### K7. `convergence-authority-single-source-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K7-1 | P5 Resource Envelope Guard | code-closed for projection rebuild burst coalescing；live verification still required（对比 active-run 20s windows for `metadata_records_scanned`、`payload_hydrates`、`workspace_projection_rebuild_total`、CPU and RSS） | pending |

### K8. `current-action-contract-authority-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K8-1 | T11 | 重新 live verify 两个 workspace 的最新 retry input | [ ] 未勾选 |

### K9. `current-contract-lineage-isolation-and-assessment-recompute-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K9-1 | C4 | contractViolationRecovery 携带 `expectedContractRole=diagnostic_lineage` 和 `expectedContractAuthoritative=false`；full Stage Packet / LLM input / context-budget diagnostic-namespace migration 未完成 | partial |
| K9-2 | C5 | Live Verification And Resource Observation e2e/live verification scripts and IO-delta observation 未做 | active |

### K10. `file-patch-side-effect-owner-migration-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K10-1 | Phase 7 | Run deployed live validation for a workspace requiring patch apply + validation + DoD closeout | [ ] 未勾选 |
| K10-2 | Phase 7 备注 | 2026-07-06 deployed service 在 verification polling 时 `127.0.0.1:8080` 不可达，最终 deployed live DoD closeout 复选框保持打开直到最新代码 redeploy | active |
| K10-3 | Phase 6 | `patch/revert` phase-gated until formal reverse material is available to `blocks/file`（明确 deferred） | deferred |

### K11. `convergence-authority-simplification-design.md`（Status: closed，但 Post-Closure Gap）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K11-1 | RequiredOutput authority drift | top-level `requiredOutput` 与 `currentActionContract.requiredOutput` 仍被部分路径视为 selectable current sources | follow-up |
| K11-2 | Target snapshot ownership drift | `acquire_target_snapshot` 仍可依赖 LLM-generated graph；deterministic read-only acquisition 应是 runtime/adapter-managed 但未实现 | follow-up |

### K12. `llm-context-budget-todo.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K12-1 | CTX-11 | 当 P0/P1 仍超当前模型预算时支持将任务拆分为多个 continuation run 或 step | active |
| K12-2 | CTX-12 | 独立 stream 详情页、筛选和跨 run 趋势视图仍待补齐（基础投影已完成） | active |
| K12-3 | 当前仍未完全收口总览 | 可插拔真实 tokenizer 未接入；跨 run 长期趋势视图和真实 tokenizer 精度仍待补齐；provider failover 权重策略与更细粒度健康评分仍待补齐；CTX-09 治理语义归一入口覆盖细化待补齐；CTX-13 provider 侧覆盖率与精度提升 | partial |

### K13. `module-block-contract-closure-validation-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| K13-1 | Module/Block Closure Inventory | 23 个 module/block 单元的 pairwise contract tests 与 minimal chain e2e 仍在 Phase A-F 阶段；需逐个冻结合同并实现"Per-Unit Work Template"（InputContract/OutputContract/StateInvariant/ErrorContract/IdempotencyContract + 双侧 pairwise contract tests + 最小主链路 e2e + live verification） | P0/P1/P2 |

### K14. `next-mvp-acceptance.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K14-1 | Current Follow-up Boundaries | Full knowledge asset、retrieval、template marketplace、enterprise collaboration、multi-tenant governance、Sub-Agent orchestration 未列入 next MVP | 边界外 |
| K14-2 | N1-QC4/QC5/QC8 | pending deployed real LLM e2e | pending |

### K15. `orchestrator-code-audit-alignment-closure.md`（Status: active）

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| K15-1 | P0 File/Patch Side-Effect Owner Migration | `file/read`、`file/list`、target snapshot acquisition 需通过 `blocks/file` 或正式 file capability 执行；`patch/preview`、`patch/apply`、`patch/revert` 落盘技术执行迁移到 block adapter；所有 contract violation 输出在副作用前被拒绝 | P0 |
| K15-2 | P1 Run Lineage Explicitization | 把 replan successor/predecessor 正式 lineage 写入 owner model；`ExecutionContext.replanContinuation` 只保留运行期恢复材料 | P1 |
| K15-3 | P1 Optional Discrete Topic Publishing | 如需要 `/run/completed`、`/step/succeeded`、`/approval/approved` 等离散 topic，需要做 publisher 收口 | P1 |
| K15-4 | 代码缺陷 | F-09、F-14、N-02 已识别待修复 | active |
| K15-5 | F-24 | 注释语言风格问题仅记录，未处理 | active |

### K16. `orchestrator-state-machine-contract-hardening-plan.md`（Status: active）

| # | Phase | 待办项 | 状态 |
|---|-------|--------|------|
| K16-1 | Phase 1 | 冻结表驱动状态机（部分代码 2026-07-09 已添加，需补 unit tests 拒绝所有 forbidden combinations） | active |
| K16-2 | Phase 2 | 单一 authority refresh output（部分实现） | active |
| K16-3 | Phase 3 | Dispatch input 变纯渲染 | active |
| K16-4 | Phase 4 | Repair/retry 变 authority consumer | active |
| K16-5 | Phase 5 | Terminal and governance gates | active |
| K16-6 | Phase 6 | Projection 只读 | active |
| K16-7 | P2 Live Verification | 选定 clean workspace 和 dirty/conflicting workspace 跑回归 | pending |

### K17. `product-design-to-runtime-convergence-closure-plan.md`（Status: active）

| # | Phase | 待办项 |
|---|-------|--------|
| K17-1 | Phase 0 | Live Baseline Freeze 流程需持续执行 |
| K17-2 | Phase 1 | Product Acceptance Mapping: G1-G7 acceptance case 清单需建立 |
| K17-3 | Phase 2 | Authority Chain Contract Hardening: 10 步逐环节固化 |
| K17-4 | Phase 3 | Business Model And Data Flow Conformance: 核对所有业务对象符合产品架构 |
| K17-5 | Phase 4 | LLM Variability Tolerance: 收口 LLM 不稳定情况 |
| K17-6 | Phase 5 | Product Surface And Projection Consistency |
| K17-7 | Phase 6 | Profile And Strategy Isolation |
| K17-8 | Phase 7 | Live E2E Acceptance |

### K18. `projection-event-io-resource-closure-plan.md`（Status: active，P0/P1/P2/P3 全部 marked closed，但仍有 live resource gate 验证待补）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K18-1 | Remaining P0 | 减少实际 detail/run snapshot rebuilds 中的 payload hydrate 量；用 compact typed summary 替换热路径的 `map[string]any` clone | active |
| K18-2 | P2 live | 仍需在 redeploy 后通过 `scripts/observe-workorch-runtime-closure.sh` 验证 | pending |
| K18-3 | 资源门禁 | `processResource.cpuPercent < 100`、`payloadHydratesDelta <= 630`、`payloadHydrateBytesDelta <= 11177537` 等需在 live 复测 | pending |

### K19. `restart-context-budget-continuation-closure-plan.md`（Status: active，前 3 项已 done）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K19-1 | Live restart verification | [ ] Re-run live restart verification after redeploy | [ ] 未勾选 |

### K20. `resolved-decision-continuation-owner-state-closure-plan.md`（Status: closed，Open Questions）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K20-1 | Q1 | 已经持久化的 invalid `provide_missing_info` resolutions 应该如何处理 | Open |
| K20-2 | Q2 | scheduler diagnostics 应保持 fixed count per workspace/run 还是 global ring + workspace index | Open |
| K20-3 | Q3 | `ContinuationAction` 是否成为所有 resolution type 的 stored resolution field | Open |

### K21. `run-closure-quality-improvement-plan.md`（Status: active，QC4/QC5/QC8/QC18-19/QC37-39/QC71/QC86/QC89/QC90）

| # | QC | 待办项 | 状态 |
|---|-----|--------|------|
| K21-1 | QC4 | DoD item-level accepted Result schema：实施完成但 pending deployed real LLM e2e | pending |
| K21-2 | QC5 | Decision/Approval resolution always enqueues/terminates：pending deployed multi-round ticket/approval e2e | pending |
| K21-3 | QC8 | Stronger LLM execution context package：pending deployed real workspace repair regression | pending |
| K21-4 | QC18 | Accepted Run terminal closeout repository read reuse：deployed quantification pending next rebuild | pending |
| K21-5 | QC19 | Accepted Run checkpoint StepID reuse：deployed quantification pending next rebuild | pending |
| K21-6 | QC37 | DoD closeout evidence ref reuse：deployed quantification pending next rebuild | pending |
| K21-7 | QC38 | Workspace activation response RunSnapshot reuse：deployed quantification pending next rebuild | pending |
| K21-8 | QC39 | LLM follow-up workspace manifest reuse：deployed quantification pending next rebuild | pending |
| K21-9 | QC71 | Normal LLM follow-up failure-history read skip：deployed quantification pending next rebuild | pending |
| K21-10 | QC86 | Workspace snapshot Workspace read parallelization：deployed quantification pending next rebuild | pending |
| K21-11 | QC89 | Execution-context empty history check reuse：is not a validated governed-stage performance win | pending |
| K21-12 | QC90 | Artifact storage run directory cache：deployed quantification pending next rebuild | pending |

### K22. `runtime-authority-derived-snapshot-hardening-closure-plan.md`（Status: active，7 个 Phase 全部 [ ] 未勾选）

| # | Phase | 待办项 |
|---|-------|--------|
| K22-1 | Phase 1 | Replan Successor Strip：新增 `stripDerivedActionContractForReplanSuccessor` |
| K22-2 | Phase 2 | Single Derived Snapshot Entry：新增 `refreshCurrentAuthorityDerivedSnapshot` 统一入口 |
| K22-3 | Phase 3 | Progress Merge Derived Keys：`managedRunProgressExecutionContextKeys()` 与 `convergenceDerivedExecutionContextKeys()` 对齐 |
| K22-4 | Phase 4 | Finalization Last Writer：重排 `finalizeRuntimeRepairFollowUpStepInput` |
| K22-5 | Phase 5 | Target Snapshot Prerequisite Contract：缺 target snapshot 时派生明确 prerequisite action |
| K22-6 | Phase 6 | Dispatch Outcome Guard：internal contract misalignment 不得直接落为 `run failed + no blocker + activeRunId=null` |
| K22-7 | Phase 7 | Regression Verification：go test / make fmt-check / git diff --check / 边界搜索 / live 验证 |

### K23. `runtime-contract-chain-incremental-closure-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K23-1 | 新发现偏差 | S1-S19 已 completed，新发现的偏差需通过审计后逐个收口 | active |
| K23-2 | 活工作区验证 | Progress Log 2026-06-30 至 2026-07-01 多个 S 切片已 closed，但仍待活工作区验证 | active |

### K24. `runtime-contract-retry-convergence-closure-plan.md`（positive live goal convergence/full real-resource still pending）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K24-1 | positive live goal convergence | 真实 live goal convergence（正向目标收敛）待补 | pending |
| K24-2 | real-provider resource | 完整 real-provider resource verification 仍需外部凭据和 bounded execution output | pending |
| K24-3 | live workspaces | `ws-4e83810542ac4d65` / `ws-7ced70e98ea50c43` 仍卡在历史 open decisions，strict live goal-convergence gate failed | active |
| K24-4 | verify-real-runtime-closure | 仅在有 external provider credentials 时跑 | pending |

### K25. `internal-dto-json-boundary-closure-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K25-1 | pprof 验证 | Live pprof no longer identifies internal snapshot JSON conversion as a CPU hotspot（需要 live 验证） | pending |

### K26. `runtime-coverage-convergence-state-machine-closure-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K26-1 | Live Verification after redeploy | 步骤 1-5：捕获 git rev、workorch PID/start time 和 /readyz；只解决 scoped open decisions；观察 active run 至 accepted result/artifact closure / structured decision/approval / bounded failure；记录 coverage item 状态与最新 step output 至 Batch 7 audit | live verification required after redeploy |

### K27. `runtime-validation-command-contract-authority-closure-plan.md`

| # | 主题 | 待办项 | 优先级 |
|---|------|--------|--------|
| K27-1 | P1-1 | recoveryAction 耗尽语义：command contract validation retry 耗尽后进入明确 blocker/terminal | P1 |
| K27-2 | P1-2 | projection/readmodel 展示一致性 | P1 |
| K27-3 | P2-1 | live-like fixture：构造 validation command 拒绝且携带 stale state-changing context | P2 |
| K27-4 | P2-2 | live 验证：触发同类 validation command contract failure 后下一步不是 patch/apply；若 LLM 继续 patch/apply 被 contract gate 拒绝，下一轮仍保持 validation-only 或进明确 blocker | P2 |

### K28. `runtime-deterministic-convergence-closure-plan.md` & `runtime-deterministic-ledger-assessment-closure-plan.md`

| # | 文档 | 主题 | 待办项 | 状态 |
|---|------|------|--------|------|
| K28-1 | runtime-deterministic-convergence | C1-C5 全链路 + Live Verification | C1-C5 完整代码落地未标 code-closed | active |
| K28-2 | runtime-deterministic-ledger-assessment | D1-D6 | D2 局部评估 accepted vs global assessment rejected 投影；D3 runtime authority recompute vs LLM required output 拆分；D1 current action label vs diagnostic lineage 拆分；D5 资源 lifecycle 扩展；D6 减少 C3/C4/C5 dynamic tail；D4 evidence-kind-driven state-change 要求 | active |
| K28-3 | runtime-deterministic-ledger-assessment | Q1-Q3 | Q1 typed EventHub result 字段；Q2 terminal gate fail-fast；Q3 validation scratch TTL/size policy | Open |

### K29. `runtime-diagnostic-lineage-required-output-authority-closure-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K29-1 | D1 | 禁止 rejected output summary 中 next-step prose 作为 current action guidance；保证不进入 C4.current-action.requirements / retryHint / requiredOutput / nextAction.summary | active |
| K29-2 | D2 | retry instruction 仅由 current requiredOutput 派生；retryHint 由 correctionContract.allowedOutputs 派生 | active |
| K29-3 | D3 | dispatch 前扫描 Stage Packet 与 C4 current-action payload 中结构化 output-shape 冲突 | active |
| K29-4 | D4 | same-rule 违例耗尽先查 requiredOutput conflict / diagnostic scrub / deterministic route 或 managed validation / template validation discovery gap | active |
| K29-5 | D5 | software-dev profile 声明结构化 validation discovery policy | active |
| K29-6 | D6 | projection 暴露 authorityDiagnostics；projection 不修复 owner state | active |

### K30. `software-requirement-development-orchestration-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 | 状态 |
|---|------|--------|--------|------|
| K30-1 | SRD-40 | Completion gate 持续收口能力（graph follow-up 边界） | P0 | implemented (not yet verified via deployed e2e) |
| K30-2 | SRD-41 | TUI 对话输入重启 Workspace 执行（executionContextPatch 透传） | P0 | implemented |
| K30-3 | SRD-42 | Replan 后自动创建 continuation Run（workorchd 绑定共享 RuntimeSession EventHub） | P0 | implemented |
| K30-4 | SRD-43 | Patch repair 上下文与软件开发补丁质量 | P0 | implemented |
| K30-5 | SRD-44 | 空 workspace 可继续候选 Decision 收口 | P0 | implemented |
| K30-6 | Q1-Q7 | Q1 accepted plan 是否新持久化模型；Q2 Goal/DoD 预检是否自动补 DoD；Q3 验证命令 Memory 按 Workspace 还是项目路径；Q4 companion template 是否自动串联；Q5 Result 质量门控失败处理；Q6 内置默认 upsert 是否覆盖用户改过的同 ID Soul/template；Q7 目标项目测试 secret 是否允许进入 shell env | Open questions with preferred direction |
| K30-7 | RGD-01 ~ RGD-14 | 复杂软件开发闭环回归收口；RGD-08/09/10/13 等仍需持续验证/补强 | P0 | follow-up tracking continues |

### K31. `unified-tui-console-migration-plan.md`（Status: active）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K31-1 | §4.1 入口收敛 | 模式参数和默认行为不能再次分叉 | active residual |
| K31-2 | §4.2 模式划分 | 各模式后续修改时不能破坏本地/远程边界 | active residual |
| K31-3 | §4.3 Shared Runtime | shared runtime 生命周期后续修改不能出现重复初始化或子进程回退 | active residual |
| K31-4 | §4.4 Workspace 策略 | 当前目录优先绑定策略不能被后续便利性改动破坏 | active residual |
| K31-5 | 迁移步骤 | Phase 1-6 实施记录目前停在 2026-05-03 收口项；新增 migration steps 没有进一步 implementation note | verified up to 2026-05-03 |

### K32. `workorchd-eventhub-watchdog-scope-closure-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 | 状态 |
|---|------|--------|--------|------|
| K32-1 | P0.1 | Scheduler Command Enqueue Reliability：covering Workspace activation/restart, run start/resume, governance continuation, replan continuation | P0 | closure scope pending test |
| K32-2 | P0.2 | Watchdog Runtime-Gap Recovery Boundary | P0 | closure scope pending |
| K32-3 | P0.3 | Runtime Recovery Safety Metadata（sideEffectPolicy/idempotencyPolicy/watchdogRecovery/idempotencyKey） | P0 | closure scope pending |
| K32-4 | P0.4 | Runtime Event Publication Reliability：Full Coverage 部分仍可继续观察 runtime event ordering for accepted/progress/completed/failed | P0 | partial |
| K32-5 | P1.5-P1.7 | EventHub Lane and Handler Audit / Projection and Client Display Boundary / CPU/IO Load Boundary | P1 | audit scope not closed |
| K32-6 | P2 | Documentation and Verification Scope：更新 5 个相关文档 + Full regression | P2 | verification scope pending |

### K33. `workorchd-scheduler-continuation-diagnostics-plan.md`（Status: active）

| # | 主题 | 待办项 | 优先级 | 状态 |
|---|------|--------|--------|------|
| K33-1 | P1 | Add metrics for scheduler command enqueue and continuation failures；Add diagnostics endpoint；Add TUI /status rendering；Add TUI tests | P1 | implementation pending |
| K33-2 | P2 | Extend observe-runtime-closure to validate resolved governance continuation main path；Extend real-runtime preflight to sample watchdog interval, run slot capacity, and continuation diagnostic metrics；Update architecture/checklist docs | P2 | implementation pending |
| K33-3 | Full regression | go test ./... -count=1 -p=1, make verify-ci-mvp-file | 未明确 | final regression pending |

### K34. `workorchd-state-loop-and-projection-closure-plan.md`（Status: closure-verified）

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| K34-1 | P2.4 Run Watchdog | "根据真实 provider 长跑结果评估是否把 replan continuation recovery 纳入 periodic watchdog" 留作 future feature 跟踪 | 未勾选 |

### K35. `runtime-convergence-regression-root-cause-analysis.md`

| # | 主题 | 待办项 | 优先级 | 状态 |
|---|------|--------|--------|------|
| K35-1 | P2 | Current-Focus-Scoped Binding：批量绑定必须由结构化 batch policy 声明；provider retry / graph follow-up 不能携带 broad coverageTargets 覆盖 current focus | P2 根因收口 | Implementation status: pending |
| K35-2 | P3 | Evidence-Ready Deterministic Transition：current focus evidence_ready 后优先 assessment；禁止 evidence_ready 后继续开放式验证扩散；assessment gate 在 evidence 质量不足时降级 | P2 根因收口 | Implementation status: pending |
| K35-3 | P4 | Resource Loop Cost Reduction：减少 heartbeat/projection/evidence 文件写入和 validation scratch 压力；live 验证时记录 CPU/RSS/read_bytes/write_bytes delta | P2 根因收口 | Implementation status: pending |
| K35-4 | Verification | true positive 样本不再出现，CPU/RSS/write_bytes 增长明显下降 | 未标注 | verification pending |

## L. Roadmap Candidates（70-roadmap/candidates）

来源：`docs/70-roadmap/candidates/` 下 2 个文档。

| # | 主题 | 待办项 |
|---|------|--------|
| L1 | C2. Distributed Scheduling And External Worker Fleets | 分布式调度和外部 Worker 集群候选尚未启动：worker ownership/状态机、失败/超时/重试/取消语义、队列和调度的可观测性等验收条目尚未在交付前定义 |
| L2 | C3. Multi-Tenant Authorization And Quota | 多租户授权与配额候选尚未进入开发：身份模型、资源所有权和可见性、Run 调度与运行时工具的配额执行点等验收待定义 |
| L3 | C4. External Runtime Marketplace | 外部运行时/工具适配器市场化候选尚未启动：适配器元数据、信任策略、enable 流程、向 LLM 规划上下文暴露运行时能力、缺失/禁用/被拒时的安全失败行为等验收待定义 |
| L4 | C7. Product Collaboration Surface | 企业协作面候选尚未启动：业务模型与所有权边界、Web 控制平面、与 Workspace/Run/Decision/Approval/Memory 模型的交互等验收待定义 |

## M. Archive（90-archive）

来源：`docs/90-archive/{completed,records,superseded}/` 下约 26 个文档。本节只列出真正的未完成项；其余文件均为已 completed 归档。

### M1. `90-archive/completed/tui-improvements-absorption-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| M1-1 | TUI-IMP-07 Conversation history persistence | 未来需要从后端 Run/Goal/Decision/Approval/Result/Memory 数据重建紧凑 Conversation，不使用本地 JSON 缓存；当前明确标为 deferred，需要单独设计与后端重建路径 | deferred |

### M2. `90-archive/completed/software-dev-code-agent-closure-plan.md`

| # | 主题 | 待办项 | 状态 |
|---|------|--------|------|
| M2-1 | Q1 TUI 中 DoD 输入方式 | TUI 中 DoD 输入先用 `/dod` 命令 + Workspace template 默认 DoD；专用编辑区推迟后续再做 | 推迟 |
| M2-2 | vendor deploy verification | 建议上线前按目标部署环境补充一次真实外部代码仓 e2e；该项依赖指定样例仓库或私有仓库授权，不影响当前代码收口结论 | 建议但非阻塞 |

### M3. `90-archive/records/delivery-history.md`

| # | 主题 | 待办项 |
|---|------|--------|
| M3-1 | Run Closure Quality Follow-up | 运行闭环质量改进方向（unified RunOutcome/WorkspaceOutcome、严格 DoD-backed acceptance、Decision/Approval/repair 恢复稳定性、执行效率、plan/evidence/final Result/Artifact 闭环质量）属于后续活跃跟踪 |

### M4. `90-archive/records/workspace-concurrency-loadtest-record.md`

| # | 主题 | 待办项 |
|---|------|--------|
| M4-1 | 待重新部署后复测 | 每次代码优化后均给出 mock c10/t50 与 c20/t100 的重新部署后复测脚本，尚未在新一轮部署后执行；c20/t100 与 c30/t150 重新部署后复测列入"待"状态 |
| M4-2 | 第 19 节 下一阶段性能专项 | 1) run 级 EventRecord 从 JSON map 多次重写推进为 append-only JSONL / per-run event index / 正式持久化后端；2) 引入 SQLite/Badger/Postgres 等正式持久化后端提升 c20 以上吞吐；3) Workspace Board / 运行过程列表增加 summary read model；4) Result / Artifact / RuntimeCheckpoint / EventRecord 完成阶段写入做批量提交或异步投影 |
| M4-3 | MCP stdio helper 进程 | 早前 long_lived 批次遗留的 MCP stdio helper 进程仍需通过重启服务或后续 MCP 管理能力清理 |

### M5. `90-archive/records/single-workspace-performance-optimization-record.md`

| # | 主题 | 待办项 |
|---|------|--------|
| M5-1 | Final Performance Results | 单 workspace 治理流目前固定 ~392ms；若显著提升需改变 LLM 传输（HTTP→in-process）、MCP 传输（stdio→in-process）或存储后端（file→memory）——这些均被视为架构性约束，尚未安排迁移：mock LLM HTTP round trips、MCP stdio 进程启动、文件后端写入延迟与不可重叠的串行依赖 |

### M6. `90-archive/records/coverage-work-item-closure-history.md`

| # | 主题 | 待办项 |
|---|------|--------|
| M6-1 | CWI-05 第二轮 | 共享 evidence 的逐项 alignment 仍由后续 evidence quality / assessment gate 执行 |

### M7. `90-archive/superseded/workspace-data-layout-plan.md`

| # | 主题 | 待办项 |
|---|------|--------|
| M7-1 | Inline preview | Artifact API 支持按 URI 下载正式交付物后，仍按需要补充 inline preview |
| M7-2 | 整体 | 已被 `docs/10-domain-model/workspace-goal-dod.md` 与 `docs/30-module-architecture/storage-readmodel.md` 替代；此前规划中的目录布局不再作为当前权威，仅作历史对照 |

## 总体统计

| 类别 | 待办条数 | 文档条数 |
|------|----------|----------|
| A. 文档迁移与编辑性收口 | 11 | 1 (顶层) |
| B. Core 合约层 | 2 | 2 |
| C. Overview / 状态层 | ~60 | 11 |
| D. 产品层 | 0 | 4 |
| E. Domain Model 层 | 12 | 1+5 |
| F. Runtime Flow 层 | 14 | 6+1 |
| G. Module Architecture 层 | ~30 | 8 |
| H. Profiles / Capabilities 层 | 16 | 4 |
| I. Product Interface 层 | 6 | 2 |
| J. Verification 层 | ~30 | 14 |
| K. Active Roadmap | ~200+ | ~35 |
| L. Roadmap Candidates | 4 | 1 |
| M. Archive | 9 | 6 |
| **合计** | **~400+** | **~90+** |

## 优先行动建议

> 来源：`01-overview/incomplete-and-connection-gap-tracker.md` 第四节"当前后续动作建议"。

**方向 1**：围绕 `00-core/runtime-stage-contract.md` 做最后一轮 grouped 文案压缩（对应 A1-C5-1、优先顺序 1）。

**方向 2**：围绕历史 audit / archive 做最后一批旧引用清理（对应 A6、C4-39-C4-42、A7）。

**最高优先未收口项**（仅列 P0 / Phase 1 / [ ] 完全未勾选）：

1. `runtime-authority-derived-snapshot-hardening-closure-plan.md`（7 个 Phase 全部 [ ]）
2. `module-block-contract-closure-validation-plan.md`（23 个 module/block 单元）
3. `runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md`（Phase 1-8 全部 [ ]）
4. `runtime-state-transition-contract-hardening-closure-plan.md`（Done Criteria 全部 [ ]）
5. `runtime-convergence-regression-root-cause-analysis.md`（P2/P3/P4 Implementation status: pending）
6. `file-patch-side-effect-owner-migration-plan.md`（Phase 7 唯一 [ ]）
7. `current-action-contract-authority-closure-plan.md`（T11 唯一 [ ]）
8. `restart-context-budget-continuation-closure-plan.md`（唯一 [ ]）

**最高优先 live verification pending 项**（一次性推动的 list）：

参见 K1 节，覆盖 ~30 个 active runtime plan 的 live verification after redeploy 项，需在 workorchd 重新部署并触发对应 workspace 后逐项执行。
