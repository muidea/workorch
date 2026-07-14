# WorkOrch Incomplete And Connection Gap Tracker

Status: active

Type: overview

Last Updated: 2026-05-30

## Purpose

本文件用于统一汇总文档重组过程中已识别出的两类问题：

- 内容不完整，需要补充
- 无法完整衔接，需要继续拆分、迁移或补链

本文件不记录“冲突和不一致”项。该类问题的集中入口已补入：

- `information-model-alignment-audit.md`

本文件的目标是为后续统一收口提供一个跨目录、跨批次的集中入口，避免问题继续散落在各个 `Batch` 审计文档中。

目录级剩余正文缺口总表见：

- `current-unclosed-matrix.md`

## Source Audits

本文件汇总自以下批次审计：

- `batch-1-migration-audit.md`
- `../10-domain-model/batch-2-domain-migration-audit.md`
- `../20-runtime-flow/batch-3-flow-migration-audit.md`
- `../30-module-architecture/batch-4-module-migration-audit.md`
- `../40-profiles-capabilities/50-product-batch-5-audit.md`
- `../60-verification/batch-6-verification-roadmap-archive-audit.md`

## A. 内容不完整，需要补充

当前阶段补充说明：

- `10-domain-model/`、`20-runtime-flow/`、`30-module-architecture/`、
  `40-profiles-capabilities/` 的 active 主体迁移已基本收口
- 当前剩余问题已明显收缩到：
  - `../00-core/runtime-stage-contract.md` 的最后少量深嵌细节
  - `70-roadmap/*` / `90-archive/*` 的历史压缩与最终归档时机
  - 少量模块/产品文档的更细实现样例补充

当前目录级判断：

- `10-domain-model/`、`20-runtime-flow/`、`30-module-architecture/`、
  `40-profiles-capabilities/` 的 active 主体已经基本退出总表
- `current-unclosed-matrix.md` 当前主要只剩 core contract 与 archive/history 收尾项

当前判断：

- 若按“是否还需要逐一做正文迁移”衡量，主体迁移工作已经接近完成
- 后续工作更偏：
  - core contract 最后一轮剥离
  - archive/history 压缩
  - 个别文档的示例/实现细节补强

### A1. 总入口与总览层

- `../README.md` 已补入正式阅读导航，但仍可继续细化：
  - 当前主要剩余是后续随目录继续调整时的小幅导航维护
- `current-state.md` 已补稳定仓库事实摘要、主模块集合摘要和迁移进度摘要；
  当前主要剩余是少量活跃 TODO 与收尾时机判断的持续更新

补充：

- `current-state.md` 已继续吸收 activation response、deployed verification、稳定目录结构和 2026-05-12 design-vs-implementation audit 的主结论

### A2. Domain Model 层

- `../10-domain-model/workspace-goal-dod.md` 已补 definition revision、
  previous definition hash 与 lineage boundary；
  当前主要剩余是 flow / read-model 层继续细化 definition change impact
- `../10-domain-model/run-step-result-artifact.md` 已补 ToolCall / Patch /
  ValidationResult / RuntimeCheckpoint 的对象边界与 Ownership Mapping；
  当前主要剩余是 flow / product 层对结果摘要映射继续补细
- `../10-domain-model/governance-decision-approval.md` 已补治理触发面、
  Resolution/recovery mapping、auto-governance 与 successor replan lineage；
  当前主要剩余是 flow 层动作表继续细化
- `../10-domain-model/event-observability.md` 已补 outcome/taskProgress 选择规则、
  payload summary/ref 对象层说明与 product/read-model mapping；
  当前主要剩余是模块实现与字段级 API 继续补细
- `../10-domain-model/context-capability-memory.md` 已补 recall 排序策略、
  run 优先与类型配额、semantic recall 规则；
  当前主要剩余是 flow / profile 层继续补细 selectedContext 继承与 context UX

### A3. Runtime Flow 层

- `../20-runtime-flow/activation-definition-flow.md` 已补 definition change impact、
  successor activation inheritance 与 activation vs workspace setup boundary；
  当前主要剩余是 profile-specific setup/template 继续补细
- `../20-runtime-flow/planning-execution-flow.md` 已补 patch preview/apply/revert、
  file/git/web capability participation 与 graph parallel dispatch；
  当前主要剩余是模块层更细 step kind contract 继续补充
- `../20-runtime-flow/verification-acceptance-flow.md` 已补 missingDoDItems、
  claim-to-evidence alignment、current-run evidence gate 与 coverageWorkItems
  ledger role；当前主要剩余是 outcome/taskProgress 与 completion gate 的更细读模型映射
- `../20-runtime-flow/context-assembly-flow.md` 已补 step input refresh、
  provider retry / repair / replan 的 memory 继承，以及 selectedContext
  排序/recall policy；当前主要剩余是 request audit 与 cachePlan 的模块实现细节
- `../20-runtime-flow/evidence-result-flow.md` 已补 raw payload ref taxonomy、
  event payload summary contract、final result summary schema 与 artifact
  manifest/profile boundary；当前主要剩余是模块与产品层的具体打开方式继续补细

### A4. Module Architecture 层

- `../30-module-architecture/kernel-workspace-run-result.md` /
  `../30-module-architecture/kernel-orchestrator-governance.md` /
  `../30-module-architecture/kernel-context-capability.md` 已补
  `eventstream` / `result` / `run evidence` 与 `decision` / `approval`
  的责任细化；当前主要剩余是更细实现继续演进
- `../30-module-architecture/blocks-runtime-adapters.md` 已补 step kind ->
  block kind 注册矩阵、capability boundary 与 normalization 差异；
  当前主要剩余是更细模块实现与 approval block 边界继续校正
- `../30-module-architecture/storage-readmodel.md` 已补 snapshot projection、
  paginated detail endpoint、payload ref / repository contract、path-safe readable naming
  与 segment overflow fallback；当前主要剩余是更细实现演进
- `../30-module-architecture/application-workorchd.md` 已补 request-scoped
  runtime session、bootstrap exports 分组与 observed/presenter/read-model
  边界；当前主要剩余是更细实现拆分继续演进
- `../30-module-architecture/llm-provider-assembly.md` 已补 provider-specific
  cache mapping、request-log verifier landing 与 model capability / revision cache
  落点；当前主要剩余是更细实现拆分继续演进

补充：

- `../30-module-architecture/application-workorchd.md` 的稳定内容已迁入 `../30-module-architecture/application-workorchd.md`
- `../30-module-architecture/llm-provider-assembly.md` 与 `../30-module-architecture/llm-provider-assembly.md` 的稳定内容已迁入 `../30-module-architecture/llm-provider-assembly.md`
- `../00-core/runtime-request-cache-note.md` 已补 provider-neutral vs provider-specific
  boundary、module landing matrix 与 verification landing；当前剩余主要是
  `llm-provider-assembly.md` 自身的实现落点继续补细

### A5. Profiles / Capabilities 层

- `../40-profiles-capabilities/builtin-templates-and-souls.md` 已补逐模板
  setup-draft / activation 字段矩阵与 formal split / cross-reference；
  当前主要剩余是个别模板更细正文继续补细
- `../40-profiles-capabilities/software-dev/goal-coverage-and-dod.md`、
  `code-agent.md`、`lsp-bridge-integration.md` 已补 verifier / artifact / TUI /
  MCP capability 映射与私有配置边界；当前主要剩余是更细 fixture 表与跨产品示例补充
- `workspace/` 当前仍缺：
  - deliverable boundary 的 profile-facing 说明
  - memory recall / context UX 的 profile-facing 说明
- `../40-profiles-capabilities/workspace/soul-md-default-resource.md` 已补
  bootstrap 默认资源加载映射与默认 Soul 测试/校验映射；
  当前主要剩余是更细实现样例继续补细
- `../40-profiles-capabilities/mcp/capability-management.md` 已迁入稳定正文，
  并已补 activation/read-model/UI/approval/verifier 映射；
  当前主要剩余是少量更细跨产品示例补充
- `runtime-capabilities/` 已迁入 step kind / adapter / read-model 映射与
  approval / verifier / artifact 交叉引用；当前主要剩余是少量更细跨产品示例补充

### A6. Product Interface 层

- `50-product-interface/tui/` 已迁入 workspace switching / monitoring、
  result-first display、decision / approval interaction、local/remote boundary、
  历史交互示例与 review boundary；当前主要剩余是更细异常路径样例
- `50-product-interface/web-console/` 已迁入 overview、关键页面/模型映射和
  Event/SSE、低频交互与 create/update form 行为边界，但仍缺：
  - 页面字段级更细示例
  - 个别复杂表单的逐字段样例
- `../50-product-interface/deployment.md` 已迁入部署/运行入口，并已与
  `../60-verification/service-acceptance.md` 固化职责边界；当前主要剩余是更细环境样例补充

### A7. Verification / Roadmap / Archive 层

- `../60-verification/overview.md` 已建立总入口，但仍需继续细化：
  - service / verifier / audit / tui / loadtest 之间的更细映射
  - 从 overview 直达脚本入口和 fixture/deployed 对应关系
- `../60-verification/service-acceptance.md` 已迁出启动与最小验收命令，并已补
  本地 / file backend / deployed-instance 分工说明；当前主要剩余是更细的
  deployed shell 逐页样本与 SSE 断线补偿样例
- `../60-verification/loadtest-baseline.md` 已迁入稳定并发验收基线，但仍缺：
  - 更细历史轮次索引
  - 与 records 中单工作空间优化记录的更细边界样例
- `../60-verification/core-runtime-verifier.md` 和 `../60-verification/profile-software-dev-verifier.md`
  已迁入脚本入口、断言范围、fixture/deployed 映射和 representative rule/test
  对应；后续若继续补写，主要只剩穷举式索引增强
- `60-verification/tui/` 已迁入 acceptance、display closure 与 review boundary；
  当前主要剩余是更细异常路径样例与少量交叉引用压缩
- `70-roadmap/active/`、`candidates/`、`90-archive/` 已继续接收正文并建立过渡索引，但仍缺：
  - 个别 active roadmap plan 内仍偏厚的背景/基线正文继续压缩
  - 旧路径互引清理
  - 少量最终归档时机判断
- `../90-archive/records/overview.md` 已补 records layering 与 active 入口分流；
  当前主要剩余是其余 records 类旧文档的正式迁移

补充进展：

- `../70-roadmap/active/overview.md` 已统一成 active roadmap 入口，并补充 layering / stable entrypoint 指引
- `../70-roadmap/candidates/overview.md` 已统一成 candidate roadmap 入口
- `../70-roadmap/active/llm-context-budget-todo.md` 已压缩为 stable definition entry +
  follow-up 形态，不再重复维护 core/flow/module 正文
- `../70-roadmap/active/unified-tui-console-migration-plan.md` 已压缩为 stable definition entry +
  migration follow-up 形态，不再重复维护 TUI 产品/验收主体正文
- `../70-roadmap/active/auto-governance-ticket-approval-plan.md` 已压缩为 stable definition entry +
  governance follow-up 形态，不再重复维护 decision/approval/gov kernel 主体正文
- `../70-roadmap/active/software-requirement-development-orchestration-plan.md` 已压缩为
  stable definition entry + software-dev follow-up 形态，不再重复维护 profile/flow/verifier 主体正文
- `../70-roadmap/active/next-mvp-acceptance.md` 已压缩为 stable definition entry +
  next-mvp acceptance follow-up 形态，不再重复维护 snapshot/state/API 主体正文
- `../70-roadmap/active/run-closure-quality-improvement-plan.md` 已压缩为 stable definition entry +
  closure-quality follow-up 形态，不再重复维护 acceptance/recovery/read-model 主体正文
- `../70-roadmap/active/software-dev-dod-repair-closure-plan.md` 已压缩为 stable definition entry +
  DoD-repair follow-up 形态，不再重复维护 coverage/repair/read-model 主体正文
- `../90-archive/records/delivery-history.md`、`single-workspace-performance-optimization-record.md`、
  `workspace-concurrency-loadtest-record.md` 已补 archive boundary 与 active entrypoint 指引
- `../90-archive/records/overview.md` 与 `coverage-work-item-closure-history.md` 已继续统一
  stable entrypoint / historical-only 口径
- `../90-archive/superseded/overview.md`、`refactor-archive.md`、
  `workspace-data-layout-plan.md` 已补 superseded/archive boundary 与 stable entrypoint 指引

## B. 无法完整衔接的内容断点

### B1. 旧综合状态入口内部迁移已完成，当前仅保留外部兼容别名

结论：

- 当前事实、API、验收、历史交付和压测基线已分别迁入：
  - `current-state.md`
  - `../50-product-interface/service-api.md`
  - `../60-verification/service-acceptance.md`
  - `../90-archive/records/delivery-history.md`
  - `../60-verification/loadtest-baseline.md`
- 当前主要工作已经从“正文拆分”转为“历史旧引用清理”
- 当前内部迁移已完成；根目录别名仅保留外部旧引用兼容价值

补充进展：

- `../README.md` 与 `current-state.md` 已不再主动推荐旧综合状态入口
- 当前剩余保留已主要收缩为历史 audit / archive 语境和根兼容别名本身
- `status-reference-cleanup-audit.md` 已将其从“迁移未收口项”降级为“外部兼容别名”

### B2. `../00-core/runtime-stage-contract.md` 仍保留大量非 core 内容

当前 core 文档剩余主要集中在：

- checklist / audit inventory
- 少量 acceptance / coverage closeout 例外条目

结论：

- 当前 `20-runtime-flow/`、`30-module-architecture/`、`60-verification/`
  已承接主要 context / gate / recovery / request-audit 细节
- CWI 历史时间线已迁到 archive records
- 当前已不再属于“大块正文未迁移”，后续主要是最后一轮文案压缩与是否继续降层的取舍

当前判断：

- active 结构迁移已实质完成
- 本节剩余内容可视为编辑性收尾，而不是结构迁移阻塞

补充进展：

- `../00-core/runtime-stage-contract.md` 已补 non-core landing boundary
- `../00-core/runtime-request-cache-note.md` 已补上下文优先级与 request audit 字段分层
- `../20-runtime-flow/context-assembly-flow.md` 已承接 context gate 与 stage-specific required context
- `../20-runtime-flow/context-assembly-flow.md` 已继续承接 context bundle projection、
  canonical failure hydration、coverage target auto-rebuild、semantic admission 与 memory recall admission
- `../20-runtime-flow/repair-replan-flow.md` 已承接 concrete failure evidence acquisition 与 wrapper recovery inheritance
- `../20-runtime-flow/repair-replan-flow.md` 已继续承接 governance queue / resume 语义
- `../20-runtime-flow/repair-replan-flow.md` 已补 context-gate recovery routing 与
  `llm_context_integrity_failed` 恢复边界
- `../20-runtime-flow/verification-acceptance-flow.md` 已承接 assessment-to-continuation routing、coverage work item flow 与 completion focus priority
- `../20-runtime-flow/evidence-result-flow.md` 已承接 evidence quality / closure priority 与 result/artifact closeout 规则
- `../30-module-architecture/storage-readmodel.md` 已承接 coverage ledger persistence、projection layering、consistency repair boundary 与 raw payload ref 形态
- `../30-module-architecture/llm-provider-assembly.md` 已继续承接 request audit metadata、
  gate metadata 与字段级保护责任
- `../60-verification/core-runtime-verifier.md` 已补 canonical failure hydration、
  protected field / context-gate recovery 的验证映射
- `../90-archive/records/coverage-work-item-closure-history.md` 已承接 CWI 分批实现与回归历史
- `../90-archive/completed/core-runtime-closure-history.md` 已承接 IA / SNC / STG / CVC / CWI 的历史清单与落地轨迹
- 当前剩余深嵌内容主要已收缩到极少量 grouped boundary/checklist 文案和少量 closeout 例外条目

### B3. 旧文档普遍不是“单一职责文档”

当前大量旧文档同时混有：

- 设计原则
- 实施任务
- 历史进度
- 验收脚本
- UI 展示要求

典型来源：

- `../10-domain-model/context-capability-memory.md`
- `../10-domain-model/context-capability-memory.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
- `../50-product-interface/tui/requirements.md`
- `../30-module-architecture/storage-readmodel.md`

结论：

- 后续不能做机械搬运
- 需要按“对象 / flow / module / profile / product / verification / archive”重写式迁移

补充：

- `../10-domain-model/context-capability-memory.md` 已开始沉到 `../10-domain-model/context-capability-memory.md`
- `../20-runtime-flow/planning-execution-flow.md` 已开始沉到 `../20-runtime-flow/planning-execution-flow.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md` 的稳定 closure 规则已继续沉到 `../20-runtime-flow/verification-acceptance-flow.md`

### B4. Context 组织规则和 provider request rendering 规则仍分散

当前相关内容仍分散在：

- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../00-core/runtime-request-cache-note.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../60-verification/audits/codex-request-context-comparison.md`

结论：

- 后续需要统一拆成：
  - core contract
  - runtime flow 视角
  - module implementation 视角
  - audit/比较材料

### B5. Memory / Recall 的静态对象和动态行为仍缠绕

当前 Memory 相关规则同时横跨：

- domain model：`MemoryItem` / `MemoryRecallTrace`
- runtime flow：activation recall / step input refresh / recovery inheritance
- module：selector / writer / trace persistence
- profile：context UX / memory-facing policy

结论：

- 后续必须继续按层切开
- 当前 `../10-domain-model/context-capability-memory.md` 还不能单独替代旧 Memory 文档

### B6. Profile / Capability 与 Module / Product 边界仍有重叠

典型重叠包括：

- MCP capability catalog 既是 profile/capability 问题，也是 application/read-model/UI 实现问题
- workspace soul routing 既是 profile 选择问题，也是 activation flow 和 appservice 写入问题
- TUI 文档仍同时承载产品、实现、验收和 review

结论：

- 目前只是按语义归属初步放置
- 还需要继续补充跨层链接和落点说明

### B7. Roadmap / Audit / Archive 的旧正文还未真正迁出

虽然新目录已经建立，但以下问题仍存在：

- 部分旧 roadmap / completed / superseded 文档已迁出正文并降级为过渡索引，`../90-archive/completed/software-dev-code-agent-closure-plan.md` 与 `../90-archive/completed/tui-improvements-absorption-plan.md` 也已进入 `90-archive/completed/`
- archive 已开始接收 completed / records / superseded 正文，但仍未全部收口
- 审计文档与计划文档仍有角色交叉

结论：

- 第二阶段迁移仍需逐项清理旧互引，并继续收口 superseded / records 类旧正文

## C. 统一处理优先顺序

建议后续统一处理顺序如下：

1. 先继续评估 `../00-core/runtime-stage-contract.md` 中的剩余 grouped boundary/checklist 文案
   - 优先迁到 `20-runtime-flow/`
   - 再迁到 `30-module-architecture/`
2. 再处理 context/request rendering 相关文档的分层
3. 再处理 TUI / MCP / workspace 这类跨 profile-module-product 的重叠主题
4. 最后逐项压缩剩余历史 audit / archive 说明，完成真正归档

## D. 当前后续动作建议

当前最适合继续推进的两个方向是：

- 方向 1：围绕 `../00-core/runtime-stage-contract.md` 做最后一轮 grouped 文案压缩
- 方向 2：围绕历史 audit / archive 做最后一批旧引用清理

这两项继续收口后，其余大部分“不完整”和“衔接断点”会继续明显收缩。
