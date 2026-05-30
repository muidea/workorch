# Workspace Data Layout Plan

Status: superseded for new implementation by
`../../10-domain-model/workspace-goal-dod.md` and
`../../30-module-architecture/storage-readmodel.md`.

The current runtime no longer places user deliverables under
`.workorch-data/workspaces/<workspace-id>/working`. New workspaces use
`.workorch-workspaces/<workspace-id>` as the deliverable root and
`.workorch-data/internal/workspaces/<workspace-id>` as WorkOrch internal state.
This document remains as historical context for the earlier unified-data-root
proposal.

It is retained only as historical context, not as the current storage/layout
authority.

Current stable entrypoints:

- `../../10-domain-model/workspace-goal-dod.md`
- `../../30-module-architecture/storage-readmodel.md`
- `../../50-product-interface/service-api.md`

## Archive Boundary

本文件只承接：

- 早期统一数据根目录方案
- 已被替代的目录布局草案
- 历史背景说明

本文件不承接：

- 当前稳定 workspace 数据布局定义
- 当前稳定 storage/read-model 契约
- 当前稳定 API 投影语义

本文定义 workorch 后续统一数据目录规则。目标是在不改变业务目标的前提下，
将服务持久化数据、workspace 工作目录、运行产物和正式交付物统一纳入
`repository.dataDir`，并以 Workspace 为最小管理单位进行归档、迁移、清理和排障。

## Goals

- `repository.dataDir` 是 workorch 的唯一持久化根目录。
- 每个 Workspace 拥有独立目录，包含业务元数据、运行证据、工作目录和正式交付物。
- `working` 保存可持续迭代的业务工作区，例如源码、博文草稿、素材和运行时临时文件。
- `artifacts` 保存经过 Result/DoD 接受后的正式交付物快照。
- Run 运行证据按 `workspaceId/runId` 分区，避免单个全局 JSON 文件持续膨胀。
- Web/API 不暴露本机绝对文件路径，Artifact 通过内部 URI 引用。

## Non-Goals

- 不把 workspace 目录策略下沉到 `kernel/run`、`kernel/orchestrator` 或 runtime block。
- 不把 `artifacts` 当作源代码仓库、文稿主目录或长期编辑目录。
- 不在当前设计中要求兼容旧 `.workorch-data/*.json` 扁平结构；当前开发阶段可清空本地数据后按新结构启动。
- 不由 LLM 或 block 自行决定最终交付物目录；是否晋级为 Artifact 由 workorch Result/Artifact 流程决定。

## Root Layout

推荐最终结构：

```text
.workorch-data/
  system/
    workspace_index.json
    souls/
      soul-general/
        metadata.json
        soul.md
    skill.json
    mcp_server.json
    mcp_capability.json
    mcp-endpoints/
      mcp-xxx/
        helper.py
    event_record.json
    configuration_snapshot.json
  workspaces/
    ws-xxx/
      workspace.json
      config/
        soul_bindings.json
        runtime_profile.json
      working/
      artifacts/
      goals.json
      dods.json
      memory_item.json
      runs/
        run-aaa/
          run.json
          steps/
            <step-id-safe>/
              metadata.json
              input.json
              output.json
          tool-calls/
            <tool-call-id-safe>/
              metadata.json
              arguments.json
              output.json
          llm-calls/
            <step-id-safe>/
              request.json
              response.json
              response.sse
              usage.json
          patches/
            <patch-id-safe>.json
          validations/
            <validation-result-id-safe>.json
          checkpoints/
            <checkpoint-id-safe>.json
          decisions/
            tickets/
              <ticket-id-safe>.json
            resolutions/
              <resolution-id-safe>.json
          approvals/
            <approval-id-safe>.json
          results/
            <result-id-safe>.json
          artifacts/
            <artifact-id-safe>.json
          events/
            events-000001.jsonl
          payloads/
            <event-id-safe>.json
```

### `system`

`system` 保存不天然归属于单个 Workspace 的服务级数据：

- Workspace 索引：用于快速列出 workspace、定位 workspace 目录和投影状态。
- Soul、全局 Skill、MCP Server 等服务级配置。
- 没有 Workspace / Run 归属的服务级 EventRecord。
- 可选的配置快照或运行时诊断数据。

Workspace 本地 Skill 不放在 `system/skill.json`，而是通过对应 Workspace 的
`working/.agents/skills` 由 skill-hub 管理。

### `workspaces/{workspaceId}`

`workspaces/{workspaceId}` 是一个 Workspace 的完整数据边界。删除或归档该目录
应能覆盖该 Workspace 的业务定义、运行证据、工作区文件和正式交付物。

### `config`

`config` 保存 Workspace 级运行策略配置。它不是业务工作区，也不是正式交付物目录。

推荐内容：

```text
.workorch-data/workspaces/ws-xxx/config/
  soul_bindings.json
  runtime_profile.json
```

`soul_bindings.json` 用于定义该 Workspace 针对不同任务环节启用哪个 Soul：

```json
{
  "defaultSoulId": "soul-general",
  "bindings": {
    "planning": "soul-planner",
    "execution": "soul-engineer",
    "decision": "soul-analyst",
    "approval": "soul-reviewer",
    "validation": "soul-validator",
    "artifact": "soul-writer"
  },
  "priorityPolicy": [
    "run_stage_override",
    "workspace_stage_binding",
    "run_default",
    "workspace_default",
    "system_default",
    "unbound"
  ]
}
```

Soul 定义本身继续存放在 `system/souls/{soulId}/soul.md`，结构化元数据存放在
`system/souls/{soulId}/metadata.json`；Workspace 只保存绑定关系。完整启用规则见
`../../40-profiles-capabilities/workspace/soul-routing.md`。

### `working`

`working` 是可持续迭代工作区，供 shell、MCP、SkillHub、本地工具和用户查看修改。
它不是交付快照目录。

示例：

```text
.workorch-data/workspaces/ws-xxx/working/
  repo/
  content/
  tmp/
  .agents/
```

建议约定：

- 软件开发类 Workspace 使用 `working/repo` 作为源码根目录。
- 博文/文档类 Workspace 使用 `working/content` 作为正文、素材和参考资料根目录。
- 临时工具输出可以放入 `working/tmp`，但只有被验收接受的内容才复制或写入 `artifacts`。
- SkillHub workspace-local skills 继续位于 `working/.agents/skills`。

### `artifacts`

`artifacts` 保存正式交付物文件。它与 `runs/{runId}/artifact.json` 中的 Artifact
元数据配套使用。

推荐结构：

```text
.workorch-data/workspaces/ws-xxx/artifacts/
  run-aaa/
    result-summary.md
    patch.diff
    test-report.json
    delivery.zip
    logs/
```

Artifact 元数据保存：

- `id`
- `workspaceId`
- `runId`
- `resultId`
- `title`
- `summary`
- `mimeType`
- `storageURI`
- `evidenceRefs`
- `createdAt`

Artifact 文件引用使用内部 URI：

```text
workorch://workspaces/ws-xxx/artifacts/run-aaa/result-summary.md
```

服务侧负责将内部 URI 解析到真实文件路径。Web/API 只展示 URI、标题、摘要、
MIME 类型和下载/查看入口，不直接暴露本机绝对路径。

### `runs/{runId}`

`runs/{runId}` 保存该 Run 的运行状态、步骤、证据、治理项、正式结果和事件记录。
这使得单个 Run 可以被独立排障、导出和归档。

Run 目录中的 `artifact.json` 只保存元数据，不保存大体积文件内容。真实文件进入
`artifacts/{runId}`。

## Business Type Examples

### Software Development

```text
.workorch-data/workspaces/ws-dev/
  working/
    repo/
      go.mod
      internal/
      web/
  artifacts/
    run-implement-feature/
      patch.diff
      test-report.json
      delivery-summary.md
  runs/
    run-implement-feature/
      result.json
      artifact.json
      steps/
        <step-id-safe>/
          metadata.json
          input.json
          output.json
```

源码主状态在 `working/repo`。Artifact 只记录某次 Run 被验收接受的补丁、测试报告、
构建包或交付摘要。

### Blog Writing

```text
.workorch-data/workspaces/ws-blog/
  working/
    content/
      post.md
      images/
      references/
  artifacts/
    run-final-review/
      post-final.md
      publish-preview.html
      review-report.md
  runs/
    run-final-review/
      result.json
      artifact.json
      steps/
        <step-id-safe>/
          metadata.json
          input.json
          output.json
```

博文源稿在 `working/content/post.md` 持续编辑。`artifacts/run-final-review` 保存
验收通过后的正式发布稿、预览文件和审校报告。

## Runtime Boundaries

- `internal/modules/application/workorchd/biz/appservice` 负责 Workspace 默认目录策略、
  Artifact URI 生成策略和 API 读写编排。
- `internal/modules/application/workorchd/biz/bootstrap` 与 runtime initiator 负责根据
  `repository.dataDir` 构造 file backend repository。
- `internal/pkg/repository/file` 可以提供分区文件 repository 能力，但不理解具体业务目标。
- `kernel/orchestrator` 继续只处理 Run、Step、Decision、Approval、Result、Artifact 等模型，
  不解析 workspace 目录业务语义。
- `blocks/shell` 只使用传入的 `WorkspaceWorkingDirectory` 执行命令，不决定 Artifact 晋级。
- `blocks/mcp` 和 `blocks/llm` 只产生结构化输出和证据，不直接绕过 Result/Artifact 流程写正式交付物。

## Implementation Plan

1. Workspace root resolver
   - 已新增基于 `repository.dataDir` 的 Workspace root 解析能力。
   - 默认 `workspace.workingDirectory` 已调整为
     `.workorch-data/workspaces/{workspaceId}/working`。
   - 创建 Workspace 时已同时创建 `working`、`artifacts`、`runs` 基础目录。

2. Artifact storage service
   - 已增加 file backend Artifact 摘要文件写入与 URI 生成能力。
   - Result 被接受并生成文本/报告 Artifact 时，将正式摘要内容写入
     `workspaces/{workspaceId}/artifacts/{runId}`。
   - `artifact.json` 保存 `workorch://workspaces/{workspaceId}/artifacts/{runId}/...`
     形式的内部 URI 和文件摘要信息。
   - 已提供 `GET /api/v1/workorch/artifacts/content?uri=...` 读取/下载内部 URI
     对应的 Artifact 文件；服务端会校验 URI 必须匹配已登记 Artifact 元数据。

3. Workspace-scoped file backend
   - 已在 `system/workspace_index.json` 维护 Workspace 索引，用于列表化和
     定位 Workspace 目录；Workspace 元数据仍以
     `workspaces/{workspaceId}/workspace.json` 为准。
   - Workspace Soul binding 已从 Workspace 元数据中拆出，独立保存到
     `workspaces/{workspaceId}/config/soul_bindings.json`；读取时由 application
     bootstrap repository 合并为 Workspace 模型。
   - 已将 Workspace / Goal / DoD / MemoryItem 等 Workspace-scoped 模型从
     全局扁平 JSON 文件迁移到 `workspaces/{workspaceId}`。
   - 已将 Run-scoped 模型迁移到
     `workspaces/{workspaceId}/runs/{runId}`，覆盖 Run、Step、ToolCall、
     Patch、ValidationResult、RuntimeCheckpoint、Result、Artifact、Ticket、
     Resolution、Approval 和带 Run 归属的 EventRecord。
   - 没有 Run 归属的 EventRecord 保留在 `system/event_record.json`，避免
     服务级事件被错误归入某个 Workspace。
   - Soul / Skill / MCP Server 等服务级模型已保留在 `system`。

4. Read model and API
   - Workspace snapshot 从分区 repository 聚合。
   - Run snapshot 从 workspace/run 分区读取。
   - Artifact API 已支持按 URI 下载正式交付物；后续可按需要补充 inline preview。

5. Verification
   - 新增 file backend 目录结构单元测试。
   - e2e 验证创建 Workspace 后默认目录位于 `.workorch-data/workspaces/{workspaceId}/working`。
   - e2e 验证软件开发类 Workspace 的源码在 `working/repo`。
   - e2e 验证博文类 Workspace 的正文在 `working/content/post.md`。
   - e2e 验证完成 Run 后 Result/Artifact 元数据存在，Artifact 文件存在于
     `artifacts/{runId}`，Web/API 可通过内部 URI 读取。

## Acceptance Criteria

- 新 Workspace 不再默认创建仓库根目录下的 `workspaces/{workspaceId}`。
- `.workorch-data/workspaces/{workspaceId}` 包含该 Workspace 的工作目录、运行证据和正式交付物。
- 软件开发产生的源码位于 `working/repo`，不位于 `artifacts`。
- 博文编写产生的源稿位于 `working/content`，不位于 `artifacts`。
- 正式交付物文件位于 `artifacts/{runId}`，并通过 Artifact 元数据的 `storageURI` 引用。
- 删除或归档 `.workorch-data/workspaces/{workspaceId}` 可以完整处理该 Workspace 的数据。
- `make verify-deployed-full` 在新目录结构下通过。
