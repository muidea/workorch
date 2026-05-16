# Workspace File / Diff / Patch Capability Plan

## 背景

本文档用于跟踪 workorch 面向 Code Agent 场景补齐文件读写、diff 生成、
patch 应用和变更验收能力的需求、设计边界、任务状态和验收标准。

目标不是让 LLM 直接随意读写工作目录，而是把文件能力纳入现有
`Run / Goal / DoD / Ticket / Approval / Artifact / Memory` 治理闭环：

- 文件读取、diff、patch 都作为受治理的 runtime 工具能力执行。
- 所有文件变更都必须产生可追溯证据。
- 高风险文件操作必须通过 Approval 进入人工审批。
- 最终交付物必须引用 patch、affected files、测试命令和 DoD 验收结果。

## 目标

### 用户目标

用户只需要提交开发目标 `Goal` 和验收目标 `DoD`，workorch 能够自动：

- 分析代码仓库和相关文件。
- 读取必要上下文。
- 生成并预览变更 diff。
- 应用 patch。
- 执行构建、测试或验证命令。
- 根据证据完成 DoD 验收。
- 生成最终 Result、Artifact 和 Memory。

### 系统目标

- 文件能力必须限制在 Workspace `deliverableDirectory` 内。
- 读操作默认安全可执行；写、删除、大范围修改、敏感路径修改按策略进入
  Approval。
- 变更默认通过 `patch/apply` 落盘，避免不可审计的自由覆盖写。
- 每次 patch 都生成正式过程记录，并可在 Web/TUI/CLI 中追溯。
- 文件冲突必须显式返回，不允许静默覆盖用户或其他 Run 的变更。

## 非目标

- 不在初期支持任意二进制文件编辑。
- 不在初期开放无限制 `file/write` 覆盖已有文件。
- 不让 TUI 承载完整 diff review；TUI 默认只展示摘要和关键证据引用。
- 不通过外部 MCP 作为唯一文件系统能力来源。Workspace 文件能力应优先作为
  内置 block。
- 不引入独立于 workorch runtime 的并行 code-agent 状态机。

## 当前基线

已存在或可复用能力：

- `Run / Goal / DoD` 编排和验收模型。
- `executionGraph` 工具调用模型。
- `shell/command` 执行和 tool call evidence。
- `patchRequests` / patch 相关运行时表达基础。
- `Result / Artifact` 交付物生成。
- `Decision Ticket` 和 `Approval` 治理流程。
- `Memory` 写入和 recall trace。
- Web/TUI/CLI 对 Run、Artifact、Memory、Ticket、Approval 的基础展示。

待补齐能力与当前进展：

- 正式文件工具 block：`file/list`、`file/read` 已实现并接入本地
  bootstrap、plugin runtime 和 LLM runtimeCapabilities。
- 正式 patch 工具 block：`patch/preview`、`patch/apply` 已接入统一
  runtime registry；`patch/preview` 不落盘，`patch/apply` 继续复用
  既有 patch 证据落盘链路。
- Workspace path guard、`..`、绝对路径和 symlink escape 防护已补齐到
  文件读取和 patch 路径解析。
- Patch 一等记录先复用既有 `run.Patch` / `PatchRepository` /
  file backend，不新增并行 `PatchRecord` 模型，避免产生重复事实源。
- Conflict 检测和版本保护已支持 `baseHash` 防漂移；unified diff 继续通过
  diff context 校验。
- Patch EventHub topic、事件合同和 EventStream 持久化已补齐，
  `patch/previewed`、`patch/applied`、`patch/failed` 可进入
  RunObservability timeline。
- Patch read model 已复用现有 RunSnapshot / RunObservability / task progress /
  execution sections 投影，并已补齐 WorkspaceSnapshot patch 列表投影。
- Result / Artifact 已复用现有 patch diff artifact、deliverable manifest 和
  result summary 引用链路。
- 文件变更风险策略和 Approval 接入已覆盖敏感路径、大范围变更、
  lockfile 大范围变更、多文件变更和删除 diff；审批通过后再次 apply 必须携带
  approved `approvalId`。
- Web/TUI/CLI 已补齐 patch 摘要、状态、增删行和 approval relation 展示。
- FDP 专用验收脚本已固化为 `scripts/verify-workspace-file-diff-patch.sh`。

## 架构边界

### 内置 block 边界

建议新增或收敛到以下内置 block：

- `internal/modules/blocks/file`
  - `file/list`
  - `file/read`
  - 后续扩展 `file/edit`：只生成受治理的 patch 请求，不直接写 workspace。
- `internal/modules/blocks/patch`
  - `patch/preview`
  - `patch/apply`
  - 后续扩展 `patch/revert`：只回滚 WorkOrch 已记录且未被外部改写的 patch。

要求：

- block 只负责工具执行、安全校验和结构化输出。
- block 不理解具体业务目标。
- block 不直接完成 DoD 判断。
- block 不绕过 orchestrator 写 runtime 结果。

### Orchestrator 边界

- 继续负责编排 `executionGraph`。
- 不承载文件路径业务规则。
- 不直接写文件。
- 工具结果必须回流给 LLM step，再由 LLM 决定继续读取、patch、测试或验收。

### AppService / Read Model 边界

- appservice 负责把 patch、affected files、risk、approval、artifact 引用投影到
  workspace/run snapshot。
- Web/TUI/CLI 只展示读模型，不复制 patch 解析逻辑。
- 本地 TUI/CLI 继续通过 EventHub backend 交互，不使用本机 HTTP client。

## 工具能力设计

### file/list

输入建议：

- `path`
- `recursive`
- `include`
- `exclude`
- `maxEntries`

输出建议：

- `entries`
  - `path`
  - `kind`: `file` / `directory` / `symlink`
  - `size`
  - `mtime`
  - `contentHash`
  - `isBinary`
  - `isIgnored`

规则：

- 只允许 workspace 相对路径。
- 默认跳过 `.git/`、依赖目录、构建目录和超大目录，除非策略明确允许。
- 返回结果必须有数量上限。

### file/read

输入建议：

- `path`
- `startLine`
- `endLine`
- `maxBytes`
- `encoding`

输出建议：

- `path`
- `content`
- `contentHash`
- `size`
- `mtime`
- `lineCount`
- `truncated`
- `isBinary`

规则：

- 默认限制大小和行数。
- 大文件只返回 range 或摘要。
- 二进制文件默认只返回 metadata。
- 读取结果作为 tool call evidence，不自动写入 Artifact。

### patch/preview

输入建议：

- `unifiedDiff`
- `baseHashes`
- `strict`

输出建议：

- `patchId`
- `state`: `previewed` / `failed`
- `affectedFiles`
- `addedLines`
- `deletedLines`
- `diffHash`
- `conflicts`
- `risk`
- `approvalRequired`

规则：

- 不落盘。
- 必须执行路径安全检查。
- 必须校验 diff context。
- 如检测到敏感路径或删除文件，返回 `approvalRequired=true`。

### patch/apply

输入建议：

- `unifiedDiff`
- `baseHashes`
- `approvalId`
- `strict`

输出建议：

- `patchId`
- `state`: `applied` / `failed`
- `affectedFiles`
- `addedLines`
- `deletedLines`
- `diffHash`
- `reversePatchRef`
- `conflicts`
- `approvalId`

规则：

- 默认要求 unified diff。
- 修改已有文件时必须通过 diff context 或 base hash 做冲突检测。
- 文件已被用户或其他 Run 修改时返回 conflict，不静默覆盖。
- 删除文件、敏感路径、大范围修改必须先获得 Approval。
- apply 成功后生成正式 patch record 和 EventHub 事件。

### file/edit

定位：

- `file/edit` 是面向 LLM 的精细编辑便捷能力，不是自由写文件能力。
- 它负责把小范围文本替换转换为结构化 patch preview / patch request。
- 它本身不落盘，最终写入仍必须通过 `patch/apply` 完成，以复用 patch record、
  Approval、Artifact、EventHub 和 read model。

输入建议：

- `path`: workspace 相对路径。
- `oldText`: 要替换的原文本，必须非空。
- `newText`: 替换后的文本，可为空以表达删除片段。
- `baseHash`: 来自 `file/read.contentHash`，修改已有文件时建议必填。
- `occurrence`: 可选，指定第几次匹配，默认要求唯一匹配。
- `previewOnly`: 可选，默认 `true`。

输出建议：

- `status`: `completed` / `failed`。
- `stage`: `file_edit_preview`。
- `path`
- `baseHash`
- `contentHashBefore`
- `contentHashAfter`
- `matchCount`
- `selectedOccurrence`
- `patchPreview`
  - `patchType`: `unified_diff`
  - `targetRef`
  - `diff`
  - `summary`
  - `addedLines`
  - `deletedLines`
  - `diffHash`
  - `approvalRequired`

规则：

- 必须复用 Workspace Path Guard，禁止绝对路径、`..` 和 symlink escape。
- 只支持文本文件；二进制文件返回 `ErrInvalidArgument`。
- `oldText` 默认必须唯一匹配；多处匹配且未指定 `occurrence` 时返回 conflict。
- `baseHash` 不匹配时返回 conflict，不生成可 apply 的 patch 请求。
- 不提供 `file/write` 覆盖已有文件能力。
- 不直接向 step output 写入 `patches`，避免绕过 preview 和 approval 判断。
- 如编辑命中敏感路径、大范围变更或删除语义，输出 `approvalRequired=true`，
  后续由 `patch/apply` 触发或复用 Approval 策略。

### patch/revert

定位：

- `patch/revert` 是对 WorkOrch 生成 patch 的受治理回滚能力。
- 它只回滚已经成功应用、状态为 `applied`、且具备可验证反向信息的 patch。
- 它不负责“恢复任意文件到任意版本”，也不替代 VCS。

输入建议：

- `patchId`: 要回滚的 WorkOrch patch record。
- `approvalId`: 可选；回滚敏感路径、删除/重建文件或大范围变更时必填。
- `strict`: 可选，默认 `true`。

输出建议：

- `patchId`
- `state`: `reverted` / `failed`
- `targetRef`
- `revertPatchId`: 回滚动作对应的新 patch record，可选。
- `addedLines`
- `deletedLines`
- `beforeHash`
- `afterHash`
- `conflicts`
- `approvalId`

规则：

- 只能回滚 `PatchRepository` 中存在且属于当前 workspace/run 的 patch。
- 原 patch 必须是 WorkOrch 生成的 `applied` 状态。
- 回滚前必须校验当前文件 hash 等于原 patch 的 `afterHash`；不匹配时返回
  conflict，禁止覆盖用户或其他 Run 的后续修改。
- 回滚成功后必须更新原 patch 状态为 `reverted`，并发布 `/patch/reverted`
  EventHub 事件。
- 回滚动作本身应形成独立证据，可选择创建 `revertPatchId` 或在原 patch 上记录
  `RevertedAt` / `RevertPatchRef`。
- 对敏感路径、删除/重建、大范围回滚继续复用 Approval 策略。
- 不回滚 WorkOrch 管理的 Result / Artifact 文件；这些仍由 runtime 交付物链路管理。

## 路径与安全策略

### Path Guard

所有文件路径必须满足：

- 输入是 workspace 相对路径，或可规范化到 workspace 内。
- `..`、绝对路径、空路径、非法路径返回错误。
- symlink 必须解析真实路径，禁止逃逸 workspace。
- 禁止访问 `.git/objects` 等内部数据，除非后续定义只读例外。

### 风险分级

建议风险等级：

- `read`: 只读，默认允许。
- `create`: 新建普通文件，默认允许或按路径策略限制。
- `modify`: 修改普通文件，按 Soul / workspace policy 控制。
- `delete`: 删除文件，默认需要 Approval。
- `sensitive`: 敏感路径，必须 Approval。

敏感路径示例：

- `.git/`
- `.env`、`.env.*`
- SSH key、证书、私钥
- CI secret 配置
- 生产配置文件
- 包管理 lockfile 的大范围重写

## Patch 一等模型

当前实现决策：不新增独立 `PatchRecord`，先将既有 `run.Patch` 作为正式
Patch Record 使用。原因是当前代码已经具备 `run.Patch` 模型、
`PatchRepository`、file backend 和 orchestrator 证据记录链路；再新增一套
模型会形成并行事实源，增加 Web/TUI/CLI 投影和验收追溯复杂度。

后续如需要更完整的 diff metadata，可在 `run.Patch` 上扩展字段，而不是新建
互相分叉的模型。

字段建议：

- `ID`
- `WorkspaceID`
- `RunID`
- `StepID`
- `ToolCallID`
- `ApprovalID`
- `State`: `previewed` / `applied` / `failed` / `reverted`
- `BaseRevision`
- `AffectedFiles`
- `AddedLines`
- `DeletedLines`
- `DiffHash`
- `ReversePatchRef`
- `Conflicts`
- `Risk`
- `BeforeHash`
- `AfterHash`
- `OriginalContentRef`
- `ReverseDiff`
- `RevertedAt`
- `RevertPatchID`
- `CreatedAt`
- `UpdatedAt`

`AffectedFile` 字段建议：

- `Path`
- `ChangeType`: `create` / `modify` / `delete` / `rename`
- `OldPath`
- `AddedLines`
- `DeletedLines`
- `BeforeHash`
- `AfterHash`
- `IsSensitive`

### Revert 数据要求

为了让 `patch/revert` 不依赖猜测，`patch/apply` 在落盘时需要补齐可验证的反向信息：

- 对 `unified_diff`：
  - 记录 apply 前后的 content hash。
  - 记录可直接应用的 reverse unified diff，或记录生成 reverse diff 所需的
    before/after content refs。
- 对 `file_write`：
  - 新建文件时记录 `beforeExists=false`，revert 时删除该文件。
  - 覆盖已有文件时记录 apply 前内容引用或 reverse diff。
  - 删除语义后续如纳入 patch 类型，也必须记录删除前内容引用。
- 对所有 patch：
  - 回滚前校验当前文件 hash 与原 patch `AfterHash` 一致。
  - 反向内容不能只存在 step 临时 output 中，必须持久化到 patch record 或
    WorkOrch-managed Artifact / content storage。

当前阶段不要求兼容旧 patch 数据；缺少反向信息的历史 patch 不允许 revert，
返回明确错误。

## 事件与投影

建议新增 EventHub topic：

- `/patch/previewed`
- `/patch/applied`
- `/patch/failed`
- `/patch/reverted`

Run / Workspace snapshot 应投影：

- latest patch summary
- affected files
- patch state
- risk
- approval relation
- artifact refs
- test evidence refs

Timeline 应包含 patch 事件。

Context Bundle 应可展示当前 Run 的 patch refs，用于后续 LLM validation 和
artifact 阶段追溯。

## Approval 策略

以下情况必须触发 Approval：

- 删除文件。
- 修改敏感路径。
- 修改超过阈值的文件数量。
- 单个 patch 增删行超过阈值。
- 修改 CI/CD、部署、权限、安全配置。
- patch 无法通过 base hash 或 diff context 完整校验。

Approval ticket 必须包含：

- 操作类型。
- affected files。
- diff summary。
- 风险原因。
- 可选 preview artifact 或 patch ref。

审批通过后，`patch/apply` 必须引用 `approvalId`。

## 展示要求

### TUI

默认展示：

- 当前 Run 修改了哪些文件。
- patch state。
- 增删行数量。
- 是否需要 Approval。
- 测试/验证命令摘要。
- Result / Artifact refs。

不默认展示完整 diff。

可通过命令查看：

- `/patch`
- `/artifact`
- `/status`

### Web

Web 应展示：

- Run 详情中的 Patch 区域。
- affected files 表格。
- diff summary。
- patch state 和 approval relation。
- Artifact 中的完整 diff 或 patch 文件引用。
- 与 DoD 验收项的 evidence refs 关系。

### CLI

CLI 应支持：

- 查看当前 Run patch 摘要。
- 查看 patch 详情。
- 导出或打印 artifact diff。
- 提交 approval 后继续 apply。

## 任务状态规则

状态取值：

- `pending`：已确认需求，尚未开始。
- `in_progress`：正在开发或验证。
- `implemented`：代码已实现，尚未完成完整验收。
- `verified`：代码、文档、e2e 验证均完成。
- `blocked`：存在外部依赖或设计问题需要先确认。
- `deferred`：确认延期。

## 需求任务清单

| ID | 需求项 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- |
| FDP-01 | Workspace path guard 与 symlink escape 防护 | P0 | verified | 越权路径、绝对路径、symlink 逃逸均被拒绝并有单测 |
| FDP-02 | `file/list` 内置 block | P0 | verified | 可列 workspace 内目录，返回 metadata、hash、binary 标记和数量限制 |
| FDP-03 | `file/read` 内置 block | P0 | verified | 支持 range/大小限制/二进制识别/contentHash |
| FDP-04 | `patch/preview` 内置 block | P0 | verified | 可校验 file_write/unified diff、返回 target/hash/增删行摘要，不输出 `patches` 且不落盘；risk/conflicts 归入 FDP-09/FDP-16 |
| FDP-05 | `patch/apply` 内置 block | P0 | verified | 可应用合法 diff，冲突时失败，成功后有 patch record |
| FDP-06 | PatchRecord 模型、repository、file backend | P0 | verified | 复用现有 `run.Patch` / `PatchRepository` / file backend，patch 记录可持久化 |
| FDP-07 | Patch EventHub topic 与事件合同 | P0 | verified | preview/apply/fail/revert 事件可被订阅、持久化并进入 timeline |
| FDP-08 | Patch read model / snapshot / observability 投影 | P0 | verified | RunSnapshot、RunObservability 可查看 patch refs、事件 timeline 和执行证据 |
| FDP-09 | Approval 风险策略接入 | P0 | verified | 删除/敏感路径/大 diff 必须阻塞到 Approval；审批通过后携带 `approvalId` 才能 apply/revert |
| FDP-10 | Result / Artifact 引用 patch 和测试证据 | P0 | verified | 最终结果包含 patch refs、affected files、test evidence refs，并生成 patch diff artifact |
| FDP-11 | TUI patch 摘要展示 | P1 | verified | TUI 能展示 patch state、文件清单、增删行和 artifact refs |
| FDP-12 | Web patch 区域展示 | P1 | verified | Web Run 详情可查看 patch 摘要、affected files、approval relation |
| FDP-13 | CLI patch 查看命令 | P1 | verified | CLI 可查看当前 Run patch 摘要和详情 |
| FDP-14 | `patch/revert` 能力 | P2 | verified | 可回滚 WorkOrch 生成的 patch，并记录 reverted 状态 |
| FDP-15 | `file/edit` 精细编辑能力 | P2 | verified | 支持小范围编辑并生成 diff，不绕过 patch record |
| FDP-16 | 大文件和 lockfile 策略 | P1 | verified | 大文件不直接注入 LLM，lockfile 大范围改动按策略处理 |

### FDP-14 `patch/revert` 细化任务

| ID | 任务项 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| FDP-14.1 | 扩展 `run.Patch` 可回滚元数据 | verified | patch/apply 持久化 beforeHash、afterHash、reverseDiff 或原始内容引用 |
| FDP-14.2 | 实现 `patch/revert` runtime block | verified | 可按 patchId 回滚已 applied patch，缺失或状态非法时失败 |
| FDP-14.3 | 回滚冲突保护 | verified | 当前文件 hash 与原 patch afterHash 不一致时返回 conflict 且不落盘 |
| FDP-14.4 | Approval 复用 | verified | 敏感路径、删除/重建、大范围回滚必须先 Approval |
| FDP-14.5 | 事件与投影 | verified | 回滚成功发布 `/patch/reverted`，Run/Workspace snapshot 显示 reverted |
| FDP-14.6 | 展示与 CLI | verified | TUI/Web/CLI 可看到 reverted 状态和回滚证据引用 |

### FDP-15 `file/edit` 细化任务

| ID | 任务项 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| FDP-15.1 | 定义 `file/edit` block 输入输出 | verified | 支持 path、oldText、newText、baseHash、occurrence、previewOnly |
| FDP-15.2 | 文本编辑转 unified diff | verified | 唯一匹配时生成可被 `patch/preview` / `patch/apply` 复用的 unified diff |
| FDP-15.3 | 冲突与多匹配保护 | verified | baseHash 不匹配、多处匹配未指定 occurrence、二进制文件均失败且不落盘 |
| FDP-15.4 | runtime registry / LLM capabilities / soul 权限 | verified | `file/edit` 只在允许的 soul 中暴露，LLM 只能看到声明能力 |
| FDP-15.5 | 不绕过 patch record | verified | `file/edit` 不直接写文件，不输出 `patches` 触发落盘，只输出 patchPreview |
| FDP-15.6 | 展示与验收 | verified | 最终变更仍通过 `patch/apply` 形成 patch record、artifact 和 DoD evidence |

## 分阶段实施计划

### Phase 1: 只读与安全边界

目标：

- 建立 path guard。
- 提供 `file/list`、`file/read`。
- 补齐单测和基础 e2e。

验收：

- 普通文件可读。
- 大文件被截断或要求 range。
- 二进制文件只返回 metadata。
- path escape 和 symlink escape 被拒绝。

### Phase 2: Patch preview/apply 与证据记录

目标：

- 新增 `patch/preview`、`patch/apply`。
- 新增 `PatchRecord`。
- 接入 EventHub 和 repository。

验收：

- 可从 Goal 自动生成 patch 并应用。
- patch record 可在 Run 结束后读取。
- 冲突场景不覆盖用户改动。
- patch refs 出现在 Result/Artifact。

### Phase 3: Governance 与 Approval

目标：

- 接入风险策略。
- 删除、敏感路径、大范围 patch 进入 Approval。
- 审批通过后继续 apply。

验收：

- 写 `.env` 或删除文件会阻塞到 Approval。
- 拒绝 Approval 后 Run 进入明确失败或等待重新规划状态。
- 通过 Approval 后 patch apply 继续执行并记录 approvalId。

### Phase 4: Web/TUI/CLI 展示

目标：

- Web 展示 patch 区域。
- TUI 展示 patch 摘要。
- CLI 支持 patch 查看。

验收：

- 用户可从 Run 详情看到 affected files、patch state、approval relation。
- TUI 不展示过量 diff，但能看到最终变更摘要和 artifact refs。
- CLI 能打印 patch 详情或 artifact diff。

### Phase 5: Revert 与高级编辑

目标：

- 支持 WorkOrch 生成 patch 的安全 revert。
- 支持 `file/edit` 精细编辑转 patch preview。
- 确保 revert/edit 都不绕过 patch record、Approval、Artifact 和 EventHub。

验收：

- revert 不影响非 WorkOrch 生成的变更。
- 当前文件已被用户或其他 Run 后续修改时，revert 返回 conflict 且不落盘。
- edit 本身不写文件，最终仍通过 `patch/apply` 生成 patch record。
- Web/TUI/CLI 可追溯 reverted 状态、edit 生成的 patch 和对应证据引用。

## e2e 验收场景

### FDP-E2E-01 普通代码修复

流程：

- 创建 software-dev workspace。
- 提交 Goal：修复一个失败测试。
- Agent 读取相关文件。
- 应用 patch。
- 运行测试。
- DoD accepted。

验收：

- Run completed。
- patch record 存在。
- affected files 正确。
- result summary 引用 patch 和测试 evidence。
- memory 写入 run summary。

### FDP-E2E-02 冲突保护

流程：

- Run 读取文件并准备 patch。
- 在 apply 前模拟用户修改同一文件。
- 执行 patch/apply。

验收：

- apply 返回 conflict。
- 文件不被覆盖。
- Run 进入可恢复状态，允许重新读取并重试。

### FDP-E2E-03 敏感路径 Approval

流程：

- Goal 要求修改 `.env` 或删除配置文件。
- Agent 生成 patch preview。
- 系统判定需要 Approval。

验收：

- Approval ticket 包含 affected files 和风险原因。
- 未审批前 patch 不落盘。
- 审批通过后可继续执行。
- 审批拒绝后不会修改文件。

### FDP-E2E-04 只读安全

流程：

- 尝试读取 `../outside.txt`。
- 尝试通过 symlink 读取 workspace 外文件。

验收：

- 两类访问均失败。
- 返回明确错误。
- 无文件内容泄露。

### FDP-E2E-05 安全回滚

流程：

- 创建 workspace 并通过 `patch/apply` 修改普通文本文件。
- 查询 patch record，确认状态为 `applied` 且具备 before/after hash。
- 执行 `patch/revert`。

验收：

- 文件内容恢复到 patch 前状态。
- 原 patch 或回滚 patch 记录包含 `reverted` 状态。
- EventStream 持久化 `/patch/reverted`。
- Run/Workspace snapshot、TUI/Web/CLI 均可看到 reverted 状态。

### FDP-E2E-06 回滚冲突保护

流程：

- 创建 workspace 并通过 `patch/apply` 修改普通文本文件。
- 在 revert 前模拟用户或其他 Run 修改同一文件。
- 执行 `patch/revert`。

验收：

- revert 返回 conflict。
- 当前文件内容不被覆盖。
- patch 状态不被错误标记为 `reverted`。
- Run 可继续通过 Decision 或重新规划恢复。

### FDP-E2E-07 精细编辑转 Patch

流程：

- 创建 workspace，准备一个文本源文件。
- 执行 `file/edit`，输入唯一 `oldText`、`newText` 和 `baseHash`。
- 将 `file/edit` 生成的 unified diff 交给 `patch/preview` / `patch/apply`。

验收：

- `file/edit` 本身不落盘。
- `patch/preview` 可展示增删行和 diffHash。
- `patch/apply` 后文件内容符合预期。
- patch record、Result、Artifact 和 DoD evidence 均引用该变更。

### FDP-E2E-08 精细编辑冲突保护

流程：

- 对不存在唯一匹配的 `oldText` 执行 `file/edit`。
- 对 baseHash 已过期的文件执行 `file/edit`。
- 对二进制文件执行 `file/edit`。

验收：

- 多匹配未指定 occurrence 时返回 conflict。
- baseHash 不匹配时返回 conflict。
- 二进制文件返回 invalid argument。
- 三类场景均不修改 workspace 文件。

## 当前验证记录

- `./scripts/verify-workspace-file-diff-patch.sh` 已通过，覆盖 FDP-E2E-01 到
  FDP-E2E-08 的文件读取、精细编辑、patch preview/apply、Approval、冲突保护、
  安全回滚、事件持久化、读模型、TUI 和 CLI 投影验证。
- `go test ./internal/modules/blocks/file/... ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/blocks/llm/biz -count=1`
  已通过。
- `go test ./application/workorch ./internal/modules/application/workorch/... ./internal/initiators/workorchruntime/... -count=1`
  已通过。
- `go test ./internal/pkg/events ./internal/modules/kernel/eventstream/biz ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice -count=1`
  已通过。
- `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorch/biz/backend ./internal/modules/application/workorch/service/tui ./internal/modules/application/workorch/service/cli -count=1`
  已通过。
- `npm run build`（`web/workorch-console`）已通过，并刷新嵌入式 Web console dist。
- `go test ./... -count=1`
  已通过。
- 最新 `npm run build`（`web/workorch-console`）已通过，并刷新嵌入式 Web console dist。

已覆盖单测：

- `file/read` 返回内容、range、contentHash 和二进制标记。
- `file/list` 返回 workspace 内文件并跳过生成目录。
- `file/edit` 可生成不落盘的 unified diff patchPreview。
- `file/edit` 对多匹配未指定 occurrence、baseHash 不匹配返回冲突且不落盘。
- `file/edit` 支持显式 `occurrence` 选择匹配位置，且仍然不落盘。
- `file/edit` 拒绝二进制文件并保持原文件不变。
- `file/read` 拒绝 `..` 路径逃逸。
- `file/read` 拒绝 symlink escape。
- `workspaceTargetPath` 拒绝 patch symlink escape。
- `patch/preview` 不输出 `patches`，不触发落盘副作用。
- `patch/apply` 在 `baseHash` 不匹配时失败且不覆盖目标文件。
- bootstrap registry 同时注册 `file/list`、`file/read`、
  `patch/preview`、`patch/apply`。
- LLM runtimeCapabilities 包含 `file/list`、`file/read`、
  `patch/preview`、`patch/apply`。
- Patch EventHub 合同已覆盖 `patch/previewed`、`patch/applied`、
  `patch/failed`、`patch/reverted`。
- EventStream 可持久化 patch 事件。
- Orchestrator 在 `patch/preview`、`patch/apply` 成功和 patch apply 失败时
  发布 patch 事件。
- `patch/apply` 持久化 beforeHash、afterHash、OriginalContent、OriginalExists、
  reverseDiff 等可回滚元数据。
- `patch/revert` 可回滚 WorkOrch applied patch，成功后记录 reverted 状态并发布
  `/patch/reverted`。
- `patch/revert` 在当前文件 hash 与原 patch afterHash 不一致时返回 conflict，
  且不会覆盖用户后续修改。
- RunObservability timeline 可展示持久化 patch 事件。
- WorkspaceSnapshot 可投影当前 workspace 下的 patch 列表，TUI/CLI 可直接复用该
  读模型展示 patch 摘要。
- TUI 可展示当前 Run 的 patch state、target、增删行和 approvalId。
- CLI 新增 `run patches <run-id>` 查看当前 Run patch 摘要，支持 `--json`。
- Web Run 详情 patch 表格补齐增删行和 approval relation。
- Result / Artifact patch diff 引用链路已有测试覆盖。
- 敏感路径 patch（例如 `.env`）会在落盘前阻塞到 Approval，未审批时不会写入
  workspace，也不会生成 applied patch record。
- 已审批的敏感 patch 可携带 `approvalId` 通过风险门控。
- lockfile 大范围变更会进入 Approval，小范围 lockfile 变更不会误判为大范围风险。

## 待确认问题

| ID | 问题 | 建议 |
| --- | --- | --- |
| Q1 | 是否允许 `file/write` 覆盖已有文件 | 初期不允许；只允许 patch/apply 修改已有文件 |
| Q2 | 是否允许新建文件走 `file/write` | 可作为 Phase 2 后的补充；新建代码文件也建议优先走 patch |
| Q3 | 是否支持删除文件 | 支持，但默认必须 Approval |
| Q4 | 是否支持二进制文件修改 | 初期不支持；只允许 metadata 读取 |
| Q5 | TUI 是否展示完整 diff | 不默认展示；只展示摘要和 artifact refs |
| Q6 | patch revert 是否进入 P0 | 已按 FDP-14 实现并验收；当前作为 P2 高级能力保留优先级标注，但不再是未实现项 |
| Q7 | file/edit 是否允许直接落盘 | 不允许；只生成 patch preview / unified diff，最终写入必须走 patch/apply |
| Q8 | 缺少反向信息的旧 patch 是否可回滚 | 当前阶段不考虑兼容旧数据；缺少 before/after 信息时拒绝 revert |

## 当前结论

已完成并验收的实现顺序：

1. `file/list`
2. `file/read`
3. `patch/preview`
4. `patch/apply`
5. `PatchRecord`
6. Approval 风险策略
7. Result/Artifact/Web/TUI/CLI 投影和展示
8. `file/edit` 精细编辑转 patch preview
9. `patch/revert` 安全回滚

初期不开放自由 `file/write` 覆盖已有文件。所有代码变更优先通过 unified diff
和 `patch/apply` 完成，以保证可审计、可验收、可回滚。

`file/edit` 和 `patch/revert` 的收口原则：

- `file/edit` 是 LLM 友好的编辑入口，但不是写入口。
- `patch/revert` 是 WorkOrch patch record 的反向应用，不是通用版本恢复工具。
- 两者都必须复用现有 `Run / Goal / DoD / Ticket / Approval / Artifact / Memory`
  治理链路，不能产生独立状态机或第二套文件变更事实源。
