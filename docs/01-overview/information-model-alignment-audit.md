# WorkOrch 信息模型对齐审计

Status: active

Type: overview

Last Updated: 2026-05-30

## Purpose

本文件用于集中记录文档重组后的信息模型衔接核对结果，重点覆盖：

- 同一对象在 domain / flow / module / product / verification 层的边界是否一致
- 是否仍存在旧权威引用、旧对象归属或旧字段语义残留
- 哪些不一致已经完成同步修正
- 哪些问题仍属于“内容待补充”，而不是“定义冲突”

## 本轮已完成对齐

### 1. 核心权威入口

以下文档已统一以 `../00-core/runtime-stage-contract.md` 作为唯一 core contract
入口，不再把 `../workorch-runtime-stage-contract-neutrality-design.md` 当作并列权威：

- `../10-domain-model/run-step-result-artifact.md`
- `../10-domain-model/governance-decision-approval.md`
- `../10-domain-model/event-observability.md`
- `../10-domain-model/context-capability-memory.md`
- `../20-runtime-flow/verification-acceptance-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`

结论：

- core contract 的唯一入口已在关键 domain / flow 文档中统一
- 旧 root 文档当前仅应承担迁移期兼容或 archive/source record 角色

### 2. Artifact 拥有关系

已对齐：

- `Artifact` 不再被描述为“只属于 Result”
- `Artifact` 可以属于 `run`、`step`、`result` 三种持有关系之一
- runtime evidence artifact 与 deliverable artifact 使用同一对象族，但不共享同一语义层级

对齐原因：

- 领域层原先过度把 Artifact 绑定到 Result
- flow / storage / API 文档已经表达 artifact 同时服务运行证据和正式交付

### 3. EventRecord payload 表达

已对齐：

- `EventRecord` 稳定字段边界补入 `payloadRef` / `payloadSummary`
- 明确 `payload` 不是默认主投影载荷

对齐原因：

- 原领域层只列出 `payload`
- storage/read-model、service-api、evidence-result-flow 已要求默认走 summary/ref

### 4. Outcome / TaskProgress 选择规则

已对齐：

- `Outcome` / `TaskProgress` 优先服从最新 accepted Result
- 若尚无 accepted Result，则回退到最新 terminal Run 的缺口态
- 读模型不得用历史失败证据覆盖最新 accepted closure

对齐原因：

- 该规则此前散落在 verification/acceptance 与 event/read-model 文档
- 领域层未明确最新 accepted result 的优先级

### 5. Governance 恢复衔接

已对齐：

- `Resolution` 补入 continuation / lineage 字段边界
- 明确 `replan` 只表达治理结论，successor run 仍是独立对象
- 明确 `resumeRunId` / `successorRunId` 只能表达恢复落点引用，不替代 Run 本体

对齐原因：

- domain 层此前只定义了非常薄的 resolution payload
- repair/replan flow 已经依赖 successor / resumed run 的恢复分流

### 6. MemoryRecallTrace 字段命名

已对齐：

- `MemoryRecallTrace` 字段列表统一改为 lower-camel 风格
- 与其余领域对象的字段表示方式保持一致

对齐原因：

- 原字段列表使用大写命名，和同目录其他对象表示方式不一致

### 7. Service API / Acceptance 契约

已对齐：

- `../50-product-interface/service-api.md` 已补齐 query 参数、错误契约、raw content header 和主要写接口门禁
- `../60-verification/service-acceptance.md` 已把 `status` / `snapshot` / `observability` /
  `payloads` / `artifact content` / `events` / `decisions` / `approvals`
  的 direct coverage 改为基于脚本和 `http_test.go` 的实际证据

对齐原因：

- 之前 product-interface 层和 verification 层都承认这些端点存在，但对
  “是否 direct 覆盖” 和 “参数/错误契约是否已明确” 的描述不一致

### 8. Profile / Capability 交叉映射

已对齐：

- `software-dev/*` 已补 step kind、artifact、verifier、module/product 映射
- `../40-profiles-capabilities/software-dev/lsp-bridge-integration.md` 已补 MCP capability / step kind / verifier 映射
- `../40-profiles-capabilities/mcp/capability-management.md` 已补 adapter、governance、read-model、API 与 activation 字段映射
- `runtime-capabilities/*` 已补 file/git/web 第一轮 step kind / evidence / verifier 映射
- `workspace/*` 已补 soul routing、default Soul resource、setup draft 与 API / verifier 的衔接，以及 deliverable boundary / memory-context UX

### 9. Setup-Draft / Registry / Software-Dev Verifier

已对齐：

- `../50-product-interface/service-api.md` 已补 setup-draft session state/field 方向和 registry 写接口第一轮字段表
- `../60-verification/profile-software-dev-verifier.md` 已补 script -> fixture -> assertion 的第一轮矩阵

对齐原因：

- 之前接口层只知道“有这些请求体”，验证层只知道“有这些脚本”，但两者没有明确衔接到字段和断言

### 10. Builtin Templates / Core Verifier

已对齐：

- `../40-profiles-capabilities/builtin-templates-and-souls.md` 已补第一轮逐模板字段方向与 embedded defaults 代码证据
- `../60-verification/core-runtime-verifier.md` 已补 core script / fixture / assertion 的第一轮矩阵

对齐原因：

- 之前模板层只列模板名称，core verifier 只列脚本名称，缺少“字段 -> 验证入口 -> 读模型”的闭环映射

### 11. Service API Fields / Verifier Coverage Tables

已对齐：

- `../50-product-interface/service-api.md` 已补 `runStatus` / `runSnapshot` / `runObservability` /
  `taskProgress` / `contextBundle` / `workspaceSnapshot` / `DecisionPage` /
  `ApprovalPage` 的第一轮字段语义表
- `../60-verification/core-runtime-verifier.md` 与 `../60-verification/profile-software-dev-verifier.md` 已补第一轮
  script-to-rule coverage tables

对齐原因：

- 之前接口层和验证层都只有“对象名/脚本名”，缺少“字段或规则到底由谁验证”的映射

### 12. Web Console API Path Prefix And History Reference Alignment

已对齐：

- `../50-product-interface/web-console/overview.md` 中 Web Console 页面使用的 API
  路径已统一改为 `/api/v1/workorch/...` 正式前缀
- `../90-archive/records/delivery-history.md` 中 `advanced-runtime-scheduling-plan`
  的历史主文档引用已切换到归档后的正式路径
- `../90-archive/records/delivery-history.md` 已不再通过旧根路径承接当前事实，只保留普通历史迁移说明

对齐原因：

- 之前 product 层存在“service-api 用正式前缀，Web Console overview 用简写路径”的表述分叉
- 之前 archive 记录中个别历史链接仍停留在旧根路径，容易误导迁移后的阅读入口

### 13. Service Acceptance And TUI Interaction Alignment

已对齐：

- `../60-verification/service-acceptance.md` 已补 paginated detail page 的
  cursor/limit 断言矩阵、event/raw content 断言矩阵、file-backend reload matrix
  与 file-backend/deployed verification 对照表
- `../50-product-interface/tui/interaction-contract.md` 已补 workspace monitor、
  low-frequency interaction 和 local-vs-remote interaction boundary

对齐原因：

- 之前 verification 层仍把分页与 reload 对照视为“待补框架”
- 之前 TUI 产品层虽有原则，但对 monitor、低频交互和 local/remote 边界仍缺稳定正文

### 14. Web Console Low-Frequency Interaction And Form Boundary Alignment

已对齐：

- `../50-product-interface/web-console/overview.md` 已补 workspace create、Goal/DoD
  drafting、governance detail/batch resolution、runtime detail drill-down、
  result/artifact/raw payload open 的低频交互边界
- 已补 configuration/runtime-control/workspace/governance forms 的统一责任边界：
  前端只做最小校验、请求体组装、调用 API 与成功后回读

对齐原因：

- 之前 product 层只有页面/API 对应关系，缺少“哪些低频动作可以做、哪些规则不能在前端重演”的稳定说明
- 之前 Web Console 与 `../50-product-interface/service-api.md` 在 form responsibility 上仍容易被误读为“前端可复制部分业务规则”

### 15. Loadtest Baseline Metric And Record Layering Alignment

已对齐：

- `../60-verification/loadtest-baseline.md` 已补 active loadtest 入口、Make target、
  单一控制器脚本责任、指标口径、failure-class contract 与 records layering
- 已明确 `../60-verification/loadtest-baseline.md`、`../90-archive/records/workspace-concurrency-loadtest-record.md`、
  `../single-workspace-performance-optimization-record.md` 的职责边界

对齐原因：

- 之前 verification 层只有当前阈值，没有把脚本 summary 字段与指标口径正式对齐
- 之前 baseline 和 archive records 容易被混读为同一层结论

### 16. Archive Record Reference Cleanup Alignment

已对齐：

- `../90-archive/records/overview.md` 的 candidate input 已切换到 archive 内当前有效路径
- `workorch-status-split-audit.md` 已明确降级为“首次拆分时的历史审计快照”，并把实时缺口入口指向当前 overview/audit 文档

对齐原因：

- 之前 archive overview 仍残留个别旧根路径
- 之前专项 split audit 容易被误读为当前实时缺口，而不是历史迁移时点的审计记录

### 17. TUI History / Review Boundary Alignment

已对齐：

- `../50-product-interface/tui/interaction-contract.md` 已补 workspace switch、
  decision inline resolve、approval confirm、result/artifact review 的历史交互示例
- `../60-verification/tui/tui-agent-acceptance.md` 已补 scenario intent mapping 和
  review/history boundary
- `../60-verification/tui/display-closure-tracking.md` 已补 display/review closure
  的非目标边界

对齐原因：

- 之前 TUI 产品层有原则但缺少稳定交互样例
- 之前 verification 层有场景名，但缺少“这些场景到底在验证哪条 product boundary”的显式映射

### 18. Deployment And Service-Acceptance Boundary Alignment

已对齐：

- `../50-product-interface/deployment.md` 已明确只承接镜像构建、启动方式、配置挂载、
  backend 选择与 startup decision guide
- `../60-verification/service-acceptance.md` 已明确只承接“启动之后如何证明行为成立”的
  验收入口、脚本、fixture 和读写路径证明

对齐原因：

- 之前两份文档都保留了启动/验证命令，容易让读者混淆“如何启动”和“如何证明”
- 当前已通过 boundary/matrix 方式把 deployment 与 acceptance 的文档职责拆清

### 19. Archive Record Layering Alignment

### 20. Context Bundle / Gate / Recovery Routing Alignment

已对齐：

- `../00-core/runtime-stage-contract.md` 已把 context bundle / context gate /
  semantic admission 的细节降回边界级表达
- `../20-runtime-flow/context-assembly-flow.md` 已承接 bundle section projection、
  canonical failure hydration、coverage target auto-rebuild、semantic admission、
  memory recall admission 与 trigger matrix
- `../20-runtime-flow/repair-replan-flow.md` 已承接 `llm_context_integrity_failed`
  的恢复边界、gate action 路由与 `runtime_context_required` resume 语义
- `../30-module-architecture/llm-provider-assembly.md` 已承接
  `contextGateAction/contextGateReason`、protected field omission/truncation 和字段级保护责任
- `../60-verification/core-runtime-verifier.md` 已补对应 verifier coverage

对齐原因：

- 之前 core contract 仍直接承担 bundle schema、gate action、failure hydration、
  protected field、恢复路由等 flow/module 细节
- 迁移后 core 只保留不可降级约束，flow / module / verification 分层更清晰，
  信息模型衔接也更一致

### 21. Coverage Ledger History And Active Authority Alignment

已对齐：

- `../00-core/runtime-stage-contract.md` 已不再保留 `coverageWorkItems`
  的分批实现时间线，只保留 grouped checklist 与 active authority 指引
- `../20-runtime-flow/verification-acceptance-flow.md` 已补 coverage ledger audit families
- `../30-module-architecture/storage-readmodel.md` 已补 coverage ledger 稳定持久化约束
- `../90-archive/records/coverage-work-item-closure-history.md` 已承接 CWI 分批收口历史

对齐原因：

- 之前 core 同时混有 active contract 与 dated implementation history
- 迁移后 active 规则、持久化约束与历史收口记录分层更清晰，避免把 archive 内容误读为当前权威定义

### 22. Roadmap / Archive Entry Layering Alignment

已对齐：

- `../70-roadmap/active/overview.md` 已明确 active roadmap 只承接未完成计划，不承接稳定设计定义或归档记录
- `../70-roadmap/candidates/overview.md` 已明确 candidate 只承接未晋级需求候选
- `../90-archive/records/delivery-history.md`、`../90-archive/records/single-workspace-performance-optimization-record.md`、
  `../90-archive/records/workspace-concurrency-loadtest-record.md`
  已补 active entrypoint 和 archive boundary

对齐原因：

- 之前 roadmap / archive 文档虽然大体分层正确，但 overview/record 边界说明不够统一
- 迁移后“活跃计划入口”和“历史追溯记录”职责更清楚，降低把 archive 记录误读为 active authority 的风险

### 23. Active Roadmap Stable-Entry Alignment

已对齐：

- `../70-roadmap/active/llm-context-budget-todo.md` 已把稳定正文回指到
  core / flow / module / verification 正式文档，只保留 follow-up gap 与待办
- `../70-roadmap/active/unified-tui-console-migration-plan.md` 已把 TUI/shared runtime
  的稳定正文回指到 module / product / verification 正式文档，只保留迁移关注点、
  phase 跟踪和验收条件

对齐原因：

- 之前个别 active roadmap 文档仍同时承担“稳定设计正文”和“后续待办”
- 迁移后 active roadmap 更符合执行清单角色，减少与正式设计文档的并行权威风险

### 24. Governance / Software-Dev Roadmap Stable-Entry Alignment

已对齐：

- `../70-roadmap/active/auto-governance-ticket-approval-plan.md` 已把稳定正文回指到
  governance domain / flow / module / product / verification 正式文档，只保留自动化 follow-up
- `../70-roadmap/active/software-requirement-development-orchestration-plan.md`
  已把 software-dev 模板 / flow / verifier 的稳定正文回指到正式结构，只保留 gap、
  回归差距和阶段任务跟踪

对齐原因：

- 之前这两份 active roadmap 仍同时承担“稳定设计正文”和“执行计划”
- 迁移后减少了 governance / software-dev 规则在 roadmap 与正式结构中的并行权威

### 25. MVP / Closure Roadmap Stable-Entry Alignment

已对齐：

- `../70-roadmap/active/next-mvp-acceptance.md` 已把 Run evidence / snapshot /
  state projection / control-plane 的稳定正文回指到 domain / flow / module / product / verification 正式文档
- `../70-roadmap/active/run-closure-quality-improvement-plan.md` 已把
  acceptance / closure / recovery / read-model 的稳定正文回指到正式结构，只保留
  closure-quality follow-up 与 work breakdown

对齐原因：

- 之前这两份 active roadmap 仍同时承担“稳定设计定义”和“后续优化清单”
- 迁移后减少了 MVP / closure 规则在 roadmap 与正式结构中的并行权威

### 26. Software-Dev DoD Repair Roadmap Stable-Entry Alignment

已对齐：

- `../70-roadmap/active/software-dev-dod-repair-closure-plan.md` 已把
  software-dev DoD / acceptance / repair / coverage ledger 的稳定正文回指到
  profile / flow / module / verification 正式文档
- 该文档当前只保留专项 repair follow-up、回归差距、任务清单和验收跟踪

对齐原因：

- 之前该专项计划仍同时承担长期规则说明和专项收口跟踪
- 迁移后减少了 software-dev DoD repair 规则在 roadmap 与正式结构中的并行权威

### 27. Superseded Archive Authority Wording Alignment

已对齐：

- `../90-archive/superseded/overview.md` 已补 superseded 目录不承接 active 入口的边界
- `../90-archive/superseded/refactor-archive.md` 已把 “Current authoritative”
  改成 stable entrypoint / source layout summary
- `../90-archive/superseded/workspace-data-layout-plan.md` 已补 historical-only /
  archive boundary，并把当前稳定入口指向 domain / module / product 正式文档

对齐原因：

- 之前 superseded 文档虽已归档，但个别措辞仍可能被误读为当前权威入口
- 迁移后 superseded 目录的“已被替代”语义更明确

### 28. Records Archive Stable-Entry Wording Alignment

已对齐：

- `../90-archive/records/overview.md` 已把 records 目录中的 active wording 统一为稳定入口口径
- `../90-archive/records/delivery-history.md` 已把并发基线与后续跟踪入口改为稳定入口表述
- `../90-archive/records/coverage-work-item-closure-history.md`、
  `../90-archive/records/workspace-concurrency-loadtest-record.md`、
  `../90-archive/records/single-workspace-performance-optimization-record.md`
  已统一为 stable baseline / stable definition wording

对齐原因：

- 之前 records 文档虽然已经定位为历史记录，但仍残留零散 `active` 口吻
- 迁移后 records 更明确地只承担历史追溯角色

### 29. Core Closure History Archive Alignment

已对齐：

- `../00-core/runtime-stage-contract.md` 已把 `incrementalArtifact` 的历史验收矩阵、
  IA checklist、SNC / STG-10 收口表和长篇 dated closure 记录迁出
- `../90-archive/completed/core-runtime-closure-history.md` 已承接上述历史清单、
 阶段性收口轨迹和 archive boundary

对齐原因：

- 之前 core contract 仍混有大量实现历史、验收清单和 dated closure 记录
- 迁移后 core 更接近纯合同文档，历史收口信息则归入 completed archive

已对齐：

- `../90-archive/records/overview.md` 已补 records layering，并把 active 入口显式指回
  `current-state.md`、`../60-verification/loadtest-baseline.md`、`../60-verification/service-acceptance.md`
- `../90-archive/records/delivery-history.md` 已把并发压测部分明确降级为历史结论，
  并把当前 active baseline 入口指向 `../60-verification/loadtest-baseline.md`

对齐原因：

- 之前 records 层虽然已经迁入归档目录，但各文档之间的职责分层还不够显式
- 之前 `../90-archive/records/delivery-history.md` 中的 loadtest 摘要仍容易和当前 baseline 混读

### 20. Legacy Status Supersede Readiness Alignment

已对齐：

- `status-reference-cleanup-audit.md` 已补 `../workorch-status.md` 的
  supersede readiness checklist
- `current-state.md` 已明确当前仍未归档的具体原因：
  root-level 过渡入口与历史来源追溯尚未完全退出

对齐原因：

- 之前“是否可以正式归档”主要停留在方向描述，缺少可执行判据
- 当前已能明确区分：
  - 功能性降级已完成
  - 无损归档条件尚未全部满足

补充进展：

- `../README.md` 与 `current-state.md` 已不再把 `../workorch-status.md` 当推荐入口
- `../90-archive/records/delivery-history.md` 已不再显式保留旧根路径来源说明，
  只保留普通 migration note
- `batch-1-migration-audit.md`、`workorch-status-split-audit.md`、
  `batch-2-domain-migration-audit.md` 等历史审计已把后续提法统一降级为
  “旧综合状态文档”或 superseded 副本路径
- 当前 `../workorch-status.md` 已从内部迁移未收口项降级为外部兼容别名

### 21. Current-State Repository Summary Alignment

已对齐：

- `01-overview/current-state.md` 已补 stable repository facts、main module set
  snapshot 与 migration progress snapshot

对齐原因：

- 之前 `current-state.md` 更偏迁移入口说明，缺少足够稳定的仓库事实摘要
- 当前已可作为“主入口 + 稳定事实摘要”共同承载文档，而不是只承担过渡导航

### 22. README Role-Based Navigation Alignment

已对齐：

- `README.md` 已补按角色阅读入口：
  - 架构/框架设计
  - 业务功能/对象建模
  - 产品/API/交互设计
  - 验收/验证/运维
  - 规划/历史追溯
- 已补 current-state / unclosed-matrix / roadmap / archive 的快捷跳转

对齐原因：

- 之前 README 已有阅读顺序和按任务阅读，但缺少角色化入口
- overview 层此前仍把这部分视为未收口项

### 23. Core Note / Core Contract Landing Alignment

已对齐：

- `00-core/runtime-request-cache-note.md` 已补 provider-neutral vs provider-specific
  boundary、module landing matrix 与 verification landing
- `00-core/runtime-stage-contract.md` 已补 non-core landing boundary，明确
  context assembly、provider request、coverage ledger、repair flow、result open
  链路等内容必须优先下沉到 flow/module/product 文档

对齐原因：

- 之前 core-note 仍被总表视为缺少模块级落点说明
- 之前 core contract 仍缺一层明确的“哪些内容不应继续在 core 中扩展”的承接边界

补充进展：

- `20-runtime-flow/context-assembly-flow.md` 已继续承接 context gate 与
  stage-specific required context
- `20-runtime-flow/repair-replan-flow.md` 已继续承接 concrete failure evidence
  acquisition 与 wrapper recovery inheritance
- `20-runtime-flow/verification-acceptance-flow.md` 已继续承接
  assessment-to-continuation routing、coverage work item flow 与 completion focus priority
- `20-runtime-flow/evidence-result-flow.md` 已继续承接 evidence quality /
  closure priority 与 result/artifact closeout 规则
- `30-module-architecture/storage-readmodel.md` 已继续承接 coverage ledger
  persistence、projection layering 与 consistency repair boundary
- `20-runtime-flow/repair-replan-flow.md` 已继续承接 governance queue / resume 语义
- `30-module-architecture/storage-readmodel.md` 已继续承接 raw payload ref 形态
- `00-core/runtime-request-cache-note.md` 已继续承接上下文优先级与 request audit 字段分层
- `30-module-architecture/llm-provider-assembly.md` 已继续承接 request audit metadata direction

### 24. Event / Outcome / TaskProgress Domain Alignment

已对齐：

- `10-domain-model/event-observability.md` 已补：
  - `payload` / `payloadSummary` / `payloadRef` 的对象层边界
  - latest accepted Result / latest terminal Run 的 outcome/taskProgress 选择规则
  - run/workspace/product 读模型衔接方向

对齐原因：

- 之前该文档仍停留在原则层，缺少足以与 flow/product 层闭环的对象说明

### 25. Governance Recovery Domain Alignment

已对齐：

- `10-domain-model/governance-decision-approval.md` 已补：
  - governance trigger boundary
  - `Resolution` / recovery mapping
  - duplicate failure / repair exhausted / auto-governance 的对象层语义
  - `successorRunId` / `resumeRunId` 的恢复落点边界

对齐原因：

- 之前 governance 对象只定义了最薄的 ticket/approval/resolution 外形
- 当前已能与 `repair-replan-flow.md` 的恢复链形成稳定衔接，而不再把这些语义留在 TODO 里

### 26. Run / Step / Result / Artifact Evidence Object Alignment

已对齐：

- `10-domain-model/run-step-result-artifact.md` 已补：
  - `ToolCall`
  - `Patch`
  - `ValidationResult`
  - `RuntimeCheckpoint`
  的稳定字段方向与对象边界
- 已补这些对象与 Step / Result / Artifact 的 `Ownership Mapping`

对齐原因：

- 之前该文档已明确 Artifact 的三种归属，但过程证据对象仍只有名称级列举
- 当前对象层已能与 `evidence-result-flow.md` 和模块层的 run/result 职责稳定衔接

### 27. Context / Capability / Memory Recall Alignment

已对齐：

- `10-domain-model/context-capability-memory.md` 已补：
  - recall ordering policy
  - type quota
  - semantic recall rule

对齐原因：

- 之前该文档已定义 `MemoryRecallTrace` 基本字段，但 recall 进入运行态上下文的排序与裁剪原则仍停留在 TODO
- 当前对象层已足以与 `context-assembly-flow.md` 和相关 product / module 文档形成稳定衔接

### 28. Workspace / Goal / DoD Revision Alignment

已对齐：

- `10-domain-model/workspace-goal-dod.md` 已补：
  - `definitionRevision`
  - `previousDefinitionHash`
  - Goal / DoD lineage 字段与状态边界
  - definition change impact 的对象层前提

对齐原因：

- 之前 Workspace / Goal / DoD 文档只表达当前正式定义，缺少版本与 lineage
- 当前对象层已能与 activation / coverage baseline / successor run 继承边界稳定衔接

### 29. Activation Definition Boundary Alignment

已对齐：

- `20-runtime-flow/activation-definition-flow.md` 已补：
  - definition change impact
  - successor activation inheritance
  - activation vs workspace setup boundary

对齐原因：

- 之前 activation 只说明“从正式定义到首个 step”的主链，缺少定义版本变化和 setup 区分
- 当前该 flow 已能与 Workspace / Goal / DoD revision 边界形成稳定衔接

### 30. Context Assembly Recall Ordering Alignment

已对齐：

- `20-runtime-flow/context-assembly-flow.md` 已补：
  - step input refresh
  - selectedContext ordering
  - repair / retry / replan memory inheritance

对齐原因：

- 之前 recall 排序和 memory 继承规则主要停留在 domain model / core contract 的补充描述
- 当前 flow 层已经能独立说明“下一轮 LLM 输入如何刷新、排序和继承上下文”

### 31. Repair / Replan Recovery Action Alignment

已对齐：

- `20-runtime-flow/repair-replan-flow.md` 已补：
  - recovery action matrix
  - decision / approval / auto-governance routing

对齐原因：

- 之前 repair / replan 已有恢复原则，但缺少足够稳定的动作分流表
- 当前 flow 层已经能明确区分“补信息、重试同一步、重规划、终止、审批”的恢复语义

### 32. Verification / Acceptance Coverage Alignment

已对齐：

- `20-runtime-flow/verification-acceptance-flow.md` 已补：
  - current-run evidence gate
  - `missingDoDItems` 的缺口视图边界
  - claim-to-evidence alignment
  - `coverageWorkItems` 的 acceptance ledger role

对齐原因：

- 之前 verification / acceptance 已有主链，但 coverage 与 assessment 的细化规则仍停留在待补状态
- 当前 flow 层已经能稳定表达“何时继续 repair、何时 assessment-only、何时 accepted/rejected closeout”

### 33. Evidence / Result Closeout Alignment

已对齐：

- `20-runtime-flow/evidence-result-flow.md` 已补：
  - raw payload ref taxonomy
  - event payload summary contract
  - final result summary schema
  - artifact manifest classification / profile boundary

对齐原因：

- 之前 evidence-result flow 已有主链，但 payload/ref/manifest/summary 细节仍停留在待补状态
- 当前 flow 层已经能稳定表达 evidence 进入 Result / Artifact / outcome 的分类与边界

### 34. Planning / Execution Capability Alignment

已对齐：

- `20-runtime-flow/planning-execution-flow.md` 已补：
  - patch preview / apply / revert 区分
  - file / git / web capability participation
  - graph parallel dispatch

对齐原因：

- 之前 planning/execution 只有主链，缺少 execution capability 与 graph 并发的稳定分工说明
- 当前 flow 层已经能表达“结构化建议 -> graph 调度 -> capability 执行 -> 证据沉淀”的主要职责边界

### 35. Block Adapter Registration Alignment

已对齐：

- `30-module-architecture/blocks-runtime-adapters.md` 已补：
  - step kind -> block kind 注册矩阵
  - block-specific normalization differences
  - capability boundary summary

对齐原因：

- 之前模块层只保留 block kind 级映射，无法稳定回答“某个 step kind 到底由谁执行”
- 当前模块层已经能与 runtime capability / service-api / evidence 链路形成更清晰的实现映射

### 36. Storage / Read-Model Projection Alignment

已对齐：

- `30-module-architecture/storage-readmodel.md` 已补：
  - snapshot projection structure
  - detail endpoint support mapping
  - repository/read-open contract

对齐原因：

- 之前 storage/read-model 文档已定义方向，但缺少足够稳定的 projection / detail / raw-open 层级说明
- 当前模块层已经能更清楚地与 service-api、service-acceptance、evidence/result flow 对接

### 37. LLM Provider Assembly Landing Alignment

已对齐：

- `30-module-architecture/llm-provider-assembly.md` 已补：
  - provider-specific cache mapping
  - request-log verifier landing
  - model capability table / revision cache 的模块层职责

对齐原因：

- 之前 request-cache note 已定义 provider-neutral 规则，但模块层还缺 provider-specific 落点
- 当前该模块已能更清楚承接 provider-facing request rendering 与 request-log 审计责任

### 40. Storage / Read-Open Detail Alignment

已对齐：

- `30-module-architecture/storage-readmodel.md` 已补：
  - path-safe readable naming
  - segment overflow fallback

对齐原因：

- 之前 storage/read-model 已有 projection / repository 方向，但对大对象分段和对外命名约束仍停留在待补状态
- 当前模块层已能更稳定地说明 detail/raw open 如何与 summary projection 分层

### 41. Built-in Template / Soul Matrix Alignment

已对齐：

- `40-profiles-capabilities/builtin-templates-and-souls.md` 已补：
  - 逐模板 setup-draft / activation 字段矩阵
  - formal split / cross-reference

对齐原因：

- 之前模板总入口已有模板族与字段方向，但缺少“创建前草案”和“activation 输入基线”的逐模板说明
- 当前该文档已经能更稳定地连接 workspace / software-dev / MCP / runtime-capabilities 子正文

### 42. Default Soul Resource Bootstrap Alignment

已对齐：

- `40-profiles-capabilities/workspace/soul-md-default-resource.md` 已补：
  - bootstrap 默认资源加载链
  - defaults loader / `EnsureBuiltinDefaults` 映射
  - 默认 Soul 测试 / fixture 映射

对齐原因：

- 之前该文档已定义 `soulMDRef` 规则，但缺少足够明确的 bootstrap 与 defaults_test 承接关系
- 当前 profile 文档已经能更稳定地连接 application bootstrap、API 暴露和验证入口

### 43. Software-Dev Profile Mapping Alignment

已对齐：

- `40-profiles-capabilities/software-dev/goal-coverage-and-dod.md` 已补
  acceptance dimensions、evidence policy、deliverable expectation 与 verifier 映射
- `40-profiles-capabilities/software-dev/code-agent.md` 已补
  TUI / artifact / memory / verifier 的跨层映射
- `40-profiles-capabilities/software-dev/lsp-bridge-integration.md` 已补
  MCP capability、私有配置、session 生命周期和 verifier 边界

对齐原因：

- 之前 software-dev 组三篇正文虽然已成型，但 tracker 仍按旧状态把它们列为待补
- 当前核对后，剩余内容已经收缩到更细 fixture 与跨产品示例补充，而不是核心结构缺口

### 38. Application Workorchd Layering Alignment

已对齐：

- `30-module-architecture/application-workorchd.md` 已补：
  - bootstrap exports grouping
  - request-scoped runtime session 方向
  - observed / presenter / read-model split

对齐原因：

- 之前应用层职责已经分块，但缺少足够明确的 request scope 与 read-model 分层说明
- 当前应用层文档已经能更稳定地和 service-api / service-acceptance / current-state 对接

### 39. Kernel Responsibility Split Alignment

已对齐：

- `30-module-architecture/kernel-workspace-run-result.md` 已补：
  - run evidence / result / eventstream 的更细责任拆分
- `30-module-architecture/kernel-orchestrator-governance.md` 已补：
  - decision / approval / auto-governance 相关的实现边界与责任分工

对齐原因：

- 之前 kernel 文档虽然按主题拆分，但 eventstream / result / run evidence / governance 的责任还偏粗
- 当前模块层已经能更清楚地区分“正式状态本体、恢复协调、事件持久化、产品交互面”各自的位置

## 仍待继续补充，但不属于当前冲突

- `../10-domain-model/workspace-goal-dod.md` 仍缺 definition revision / previous definition hash / lineage
- `../20-runtime-flow/verification-acceptance-flow.md` 仍需补 `coverageWorkItems`、更细 claim-to-evidence alignment 细节
- `../20-runtime-flow/evidence-result-flow.md` 仍需补 final result summary schema 与 artifact manifest 分类
- `../30-module-architecture/storage-readmodel.md` 仍需补 payload/detail endpoint 的完整 repository contract
- `../50-product-interface/service-api.md` 已补 query 参数、错误契约、response field semantics、setup-draft / registry 字段表、raw/SSE 使用示例、file-backend ref/uri 约束、trace/capability/result 语义、quality-check request 方向和典型错误示例；active 设计层已收口，后续只剩实现枚举补全类细节
- `40-profiles-capabilities/` 已补第一轮交叉映射、逐能力审批矩阵、product examples、setup-draft 表单字段表、deliverable boundary / memory-context UX、read-model/UI 字段表与 activation 字段映射；当前主要剩余是个别能力文档的少量补细示例
- `../60-verification/core-runtime-verifier.md` / `../60-verification/profile-software-dev-verifier.md` 已补 fixture matrix、script-to-rule coverage、fixture/deployed mapping、unit-test-to-rule inventory、rule family -> execution path 与 representative rule-to-test mapping；active 设计层已收口，后续只剩穷举索引式补写

这些问题属于“定义未展开”，不是“多份定义互相冲突”。

## 下一轮建议

1. 视需要继续压缩 `01-overview` / `90-archive` 中仅剩的历史追溯旧引用。
2. 继续收缩 `90-archive/records/*` 的旧路径互引和历史说明重复。
3. 最后决定 `../workorch-status.md` 的正式归档时机。
