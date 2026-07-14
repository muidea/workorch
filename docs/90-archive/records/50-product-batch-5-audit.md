# Batch 5 Profile And Product Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 5` 建立 profile/capability 与 product/interface 层时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 旧模板/Soul 文档同时承担目录、示例、详细内容、规则说明多重角色

`builtin-templates-and-souls.md` 同时包含：

- 模板目录
- 6 阶段 Soul 说明
- 大段具体 Soul 文本
- 模板之间边界

处理结论：

- 本批先抽总入口
- 具体模板和 Soul 细节后续按模板继续细分
- `builtin-templates-and-souls.md` 已补入模板总览、阶段职责和稳定边界

### 1.2 software-dev 文档原先混有 core 和 profile 规则

`software-dev/goal-coverage-and-dod.md` 虽声明自己不是 core，
但仍混入：

- core runtime 改进设计
- profile 约束
- checklist

处理结论：

- 当前先抽 software-dev profile 入口
- core 中立规则继续留在 core/flow
- `software-dev/code-agent.md` 已补入第一轮稳定 profile 边界，closure 历史迁入 archive

### 1.3 deployment / status / acceptance 之间职责仍重叠

`../50-product-interface/deployment.md`、旧综合状态文档（后续归档副本见
`../90-archive/superseded/workorch-status.md`）、后续 `service-acceptance`
都在承载：

- 运行方式
- 验收命令
- API / service 入口

处理结论：

- `../50-product-interface/deployment.md` 只保留部署和运行入口
- API 和 acceptance 后续继续拆分

## 2. 内容不完整，需要补充

### 2.1 `../50-product-interface/service-api.md` 目前还是占位入口

它还缺：

- 完整资源清单
- 读写接口分组
- detail / payload / paginated endpoint 说明

### 2.2 TUI 产品层已完成第一轮正文迁移，但仍未收口

当前已迁入：

- requirements
- interaction contract
- display refresh design

并已将：

- TUI acceptance
- display closure tracking
- `2026-05-03` review / task tracking

仍缺：

- workspace switching / monitoring
- result-first display
- decision/approval interaction
- local vs remote mode product边界

补充：

- `../50-product-interface/tui/interaction-contract.md` 与
  `../50-product-interface/tui/display-refresh-consistency-design.md` 已降级为过渡索引

### 2.3 Web Console 产品层仍不完整

当前已迁入打包与 UI 容器层级优化记录，但仍缺：

- Web Console overview
- 关键页面/模型映射
- Event/SSE 与 read model 的产品说明

### 2.4 Workspace profile 仍缺 deliverable boundary / memory recall 入口

当前 workspace 子目录已迁入：

- soul routing
- soul md default resource
- setup draft session

仍缺：

- deliverable boundary profile-facing说明
- memory recall / context UX 说明

补充：

- `workspace/soul-md-default-resource.md` 已迁入默认 Soul 资源拆分规则，但仍缺与 bootstrap / verifier 的更细实现映射
- `../10-domain-model/workspace-goal-dod.md` 的稳定边界已迁入
  `../10-domain-model/workspace-goal-dod.md` 与 `../30-module-architecture/storage-readmodel.md`

### 2.5 software-dev profile 正文已开始迁入，但交叉链接仍不完整

当前已迁入：

- `software-dev/code-agent.md`
- `software-dev/goal-coverage-and-dod.md`
- `software-dev/lsp-bridge-integration.md`

仍缺：

- 与 verifier / module / product 层的更细交叉引用
- 旧设计文档中的实施与验收历史去向说明

### 2.6 runtime-capabilities 已迁入第一轮稳定正文，但实现落点仍缺

当前已迁入：

- `runtime-capabilities/builtin-git-tool.md`
- `runtime-capabilities/file-diff-patch.md`
- `runtime-capabilities/web-search.md`

仍缺：

- step kind / adapter / read model 的实现映射
- approval / verifier / artifact 的交叉引用

### 2.7 mcp capability management 已迁入稳定正文，但 catalog/display 映射仍缺

当前已迁入：

- `mcp/capability-management.md`

仍缺：

- capability catalog 与 application/read-model/UI 的实现映射
- 审批策略、运行时展示和 verifier 的交叉引用

## 3. 无法完整衔接的内容断点

### 3.1 旧综合状态文档已不再承载正文，但旧说明仍需继续清理

当前 `../50-product-interface/service-api.md` 和 `../60-verification/service-acceptance.md` 已承接主要正文；
剩余问题主要是旧迁移说明和历史文档仍引用旧综合状态文档。

### 3.2 TUI 主题仍是跨 product / verification / audit 的重叠区域

`../50-product-interface/tui/requirements.md` 同时包含：

- 产品要求
- 当前实现快照
- 任务跟踪
- review 记录

当前已拆分到：

- `../50-product-interface/tui/requirements.md`
- `../60-verification/tui/tui-agent-acceptance.md`
- `../60-verification/tui/display-closure-tracking.md`
- `../60-verification/audits/tui-agent-requirements-review-2026-05-03.md`

但产品要求、验收矩阵、closure 记录和 review 之间仍有剩余互引待清理。

### 3.3 profile/capability 与 module boundary 仍有重叠

例如：

- MCP capability catalog 既是 profile/capability 问题，也是 application/read-model/UI 的实现问题
- workspace soul routing 既是 profile 选择问题，也是 activation flow 和 appservice 写入问题

当前处理方式是先按“语义归属”放在 profile/product 层，
后续仍需在 flow/module 文档中补链路和落点。
