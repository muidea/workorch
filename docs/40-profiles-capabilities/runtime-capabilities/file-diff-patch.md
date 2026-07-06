# File Diff And Patch Capability

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 file diff / patch 相关能力的设计说明。

## Stable Rules

### Positioning

- 文件读取、diff、patch 都应作为受治理的 runtime capability 执行
- 文件能力默认限制在 Workspace `deliverableDirectory` 内
- 最终状态变更应优先通过 `patch/apply` 落盘，而不是开放自由 `file/write`
- patch 记录应复用现有 `run.Patch` / patch repository / patch evidence 链路，不引入并行事实源
- `blocks/file` 返回的是 patch 技术执行结果，不是正式 Patch 记录；正式 Patch 记录仍由 orchestrator 在结果准入后写入 kernel evidence 链路

### Capability Surface

当前稳定能力面应包括：

- `file/list`
- `file/read`
- `patch/preview`
- `patch/apply`
- `file/edit`
  - 仅负责把小范围文本替换转换成结构化 patch preview/request
  - 不直接落盘

### Safety Boundary

- 只允许 workspace 相对路径
- 必须拦截绝对路径、`..`、symlink escape 和 workspace 外路径
- 二进制编辑、无限制覆盖写、静默删除和高风险大范围修改不应默认开放
- 敏感路径、大范围变更、删除 diff、lockfile 大范围修改等风险操作应进入 Approval

### Conflict And Integrity

- `patch/preview` 不落盘，只返回影响范围、风险、approvalRequired 和冲突信息
- `patch/apply` 必须通过 diff context 或 `baseHash` 做冲突检测
- 文件已被用户或其他 Run 修改时，应返回 conflict，不得静默覆盖
- patch apply 成功后必须产生正式 patch record、EventHub 事件和读模型投影

### Evidence Contract

- 所有文件读取和 patch 操作都必须产生可追溯证据
- 最终 Result / Artifact 应可引用 patch、affected files、测试命令、DoD 验收结果
- TUI 默认只展示摘要和关键证据引用，不承担完整 diff review

### Architecture Boundary

- block 只负责工具执行、安全校验和结构化输出
- orchestrator 不直接写文件，不承载路径业务规则
- appservice/read model 负责投影 patch、approval relation、artifact 引用和摘要视图

## Step Kind / Adapter Mapping

当前稳定 step kind -> adapter 方向：

| Step Kind | Adapter Block | 说明 |
| --- | --- | --- |
| `file/list` | `blocks/file` | 目录扫描与相对路径枚举 |
| `file/read` | `blocks/file` | 正文读取与摘要证据 |
| `file/edit` | `blocks/file` | 小范围编辑请求，转换为 patch intent |
| `patch/preview` | `blocks/file` | 预演 diff、风险、审批需求 |
| `patch/apply` | `blocks/file` | 技术落盘并返回结构化执行结果；正式 patch record 由 orchestrator 生成 |

当前 profile 解释约束：

- `file/read` / `file/list` 只能提供只读证据
- `patch/apply` 才能支撑代码或文档交付面的正式状态变更

## Evidence / Artifact / Read-Model Mapping

文件与 patch 能力主要沉淀到：

- `Patch`
  - 正式 diff / apply 记录
- `ToolCall`
  - 文件读取、列表、编辑请求和预演动作
- `Approval`
  - 大 diff、删除、敏感路径、高风险覆盖写
- `Artifact`
  - 需要时承接 diff 摘要、交付文件索引或生成文本

产品层主要承接面：

- `patches` detail page
- `artifacts` detail page
- `run snapshot`
  - 查看 results / artifacts / taskProgress
- `run observability`
  - 查看变更和恢复链

## Verification Mapping

当前与 file / diff / patch 直接相关的验证入口：

- `make verify-workspace-file-diff-patch`
- `verify-software-dev-goal-dod-e2e.sh`
- `../../60-verification/service-acceptance.md`

这些入口分别回答：

- patch preview/apply 与审批边界是否成立
- patch 证据是否进入 software-dev 交付闭环
- `patches` / `artifacts/content` / raw detail 是否可被读模型读取

## Approval Matrix

| 能力动作 | 默认审批方向 | 说明 |
| --- | --- | --- |
| `file/list` | 不需要 | 只读目录扫描 |
| `file/read` | 不需要 | 只读正文读取 |
| `file/edit` | 视生成的 patch 风险而定 | 自身不落盘，但其 preview/apply 可能触发审批 |
| `patch/preview` 小范围文本修改 | 不需要 | 仅返回 diff 与风险摘要 |
| `patch/preview` 删除、大范围 diff、敏感路径、lockfile 大改 | 需要 | 由 `approvalRequired` / 风险字段表达 |
| `patch/apply` 普通小范围修改 | 视 profile/policy 决定 | 低风险可自动执行 |
| `patch/apply` 删除、覆盖、敏感路径、高风险变更 | 需要 | 必须进入 Approval |

## Product Examples

典型产品展示方向：

- TUI / Console patch 列表
  - 展示 `patchType`、`targetRef`、`summary`、`addedLines/deletedLines`
- patch preview modal / expanded detail
  - 展示 `affectedFiles`、`diffHash`、`conflicts`、`approvalId`
- result / artifact 面板
  - 只展示最终交付关联的 patch 摘要，不默认展开完整 diff

更具体的跨产品示例：

- Console Run Detail -> `patches`
  - 默认列表展示 `patchType/targetRef/state`
  - 展开行再展示 hash、approval、revert 关系
- TUI Result/Artifact 视图
  - 只提示“哪些文件被变更、是否有审批、是否存在冲突”
  - 不默认输出完整 unified diff

## Read-Model / UI Field Matrix

| 产品面 | 关键字段 | 当前方向 |
| --- | --- | --- |
| patches detail page | `patchType` / `targetRef` / `summary` / `state` / `addedLines` / `deletedLines` | patch 级主列表 |
| patch expanded detail | `beforeHash` / `afterHash` / `approvalId` / `revertApprovalId` | patch 冲突与审批链 |
| artifacts list | `artifactType` / `category` / `summary` / `uri` | patch 关联导出物 |
| run snapshot | `patches` / `results` / `artifacts` / `taskProgress` | 变更闭环聚合 |
| run observability | `timeline` / `repairLoop` / `closureTraces` | patch 相关恢复与收口过程 |

## Cross-Layer Mapping

- runtime flow：
  `../../20-runtime-flow/planning-execution-flow.md`
- block 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- 持久化与 detail page：
  `../../30-module-architecture/storage-readmodel.md`
- 服务级验收：
  `../../60-verification/service-acceptance.md`

## Directional Output

- `file/read` 应返回 `contentHash`、`lineCount`、`truncated`、`isBinary`
- `patch/preview` / `patch/apply` 应返回 `affectedFiles`、`addedLines`、`deletedLines`、`beforeHash`、`afterHash`、`contentHash` 或 `diffHash`、`conflict`、`approvalRequired`、`riskLevel`、`riskReasons`、`diagnostics`
- `patchId`、`approvalId` 和 accepted/rejected evidence relation 属于 orchestrator 准入后的正式状态字段，不由 block 自主生成
- 大 diff、大正文和二进制内容都必须受大小和类型限制

## Source Documents

- `../../60-verification/service-acceptance.md`
