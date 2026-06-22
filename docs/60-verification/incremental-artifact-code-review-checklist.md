# Incremental Artifact Code Review Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于按代码实现逐项核对 `incrementalArtifact` 是否符合当前设计合同。

它不负责重新定义设计，只负责把现有设计拆成可执行的代码核对清单。

## Canonical Sources

核对时以以下文档为准：

- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/planning-execution-flow.md`
- `../20-runtime-flow/verification-acceptance-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`
- `../30-module-architecture/storage-readmodel.md`
- `../50-product-interface/service-api.md`
- `../50-product-interface/web-console/overview.md`
- `../50-product-interface/tui/interaction-contract.md`
- `core-runtime-verifier.md`

## Review Order

建议按以下顺序核对：

1. core contract
2. orchestrator / runtime flow
3. provider assembly
4. repository / storage / read-model
5. service API
6. Web / TUI
7. verifier / tests

## 1. Core Contract

重点核对：

- 是否同时支持 `executionGraph` 和 `incrementalArtifact` 两种输出承载模式。
- `incrementalArtifact` 是否只在 `need_state_change` 下使用，而不是新的 runtime stage。
- `selectOutputMode` 是否只使用中立信号，不依赖业务类型、语言、框架、文件名。
- `outputMode` 是否不会改写 `ConvergenceContract.State`。
- response 顶层是否禁止返回 `outputMode`。
- `artifactPlan` / `artifactChunk` / `artifact_context` / `artifactChunkRepair` 是否互斥。
- plan/chunk 是否不会被直接当作 accepted evidence。
- completion follow-up 是否会清理：
  - `artifactOutputContract`
  - `artifactContinuationContext`
  - `artifactContinuationId`

常见偏差：

- `incrementalArtifact` 被实现成新的 stage/state
- `outputMode` 只存在内部变量，没有进入 LLM-facing stage contract
- follow-up 继续继承 chunk/plan contract

## 2. Orchestrator / Runtime Flow

重点核对：

- 是否存在明确主链：
  - `selector -> plan -> chunk -> assemble -> apply -> validation -> acceptance`
- `artifact_context` 是否只能做 bounded read，不能推进 chunk cursor。
- assemble 是否只是 internal payload 准备，不直接产生 accepted evidence。
- apply 是否才是真正 state-changing 落点。
- validation 是否发生在 apply 之后，而不是 chunk 之后直接发生。
- continuation 恢复时，是否优先恢复未完成 continuation，而不是错误走普通 graph follow-up。
- 重启 / retry / repair-of-repair 后，是否继续当前 continuation，而不是要求 LLM 重生成已确认 chunk。

常见偏差：

- chunk 收齐后直接进入 acceptance
- validation 绑在 chunk 之后
- continuation 恢复被误当成普通 follow-up

## 3. Provider Assembly

重点核对：

- 是否有专门的 `selectOutputMode` 组装落点。
- request-side stage contract 是否投影了：
  - `outputMode`
  - `requestedOutputMode`
  - `outputModeSelector`
  - `artifactOutputContract`
  - `artifactContinuationContext`
- mode 从 `incrementalArtifact` 回落到 `executionGraph` 时，incremental-only 字段是否被清理。
- 上一轮 `incrementalArtifact` 因 schema/mode/artifact contract failure 被拒绝时，下一轮是否继续 `incrementalArtifact` contract retry，而不是回退到 `executionGraph`。
- contract retry 时 `requiredOutput` 是否只保留 artifact plan/chunk/context 要求，并清理 graph-only 的 `mustIncludeStateChangingNode=true`。
- chunk 请求是否只带 metadata-only continuity 信息，而不是 raw tail 正文。
- request audit 是否记录当前 artifact phase / continuation metadata 摘要。
- schema 互斥是否在装配层就被保证，而不是只靠下游兜底。

常见偏差：

- raw tail 正文重新注入 prompt
- request audit 不记录 artifact phase
- mode 回落后残留 incremental-only 字段

## 4. ArtifactContinuation Persistence

重点核对：

- 是否存在 run-scoped 的 `ArtifactContinuation` model / repository。
- 是否持久化了：
  - metadata
  - accepted plan
  - chunk metadata
  - chunk content
  - assembled metadata
  - assembled content
- continuation 是否明确属于 internal state，而不是 workspace deliverable。
- reload/restart 后是否能恢复：
  - expected sequence
  - hash / bytes
  - targetRef
  - current phase/state
- retention 清理正文后，是否仍保留足够的 hash / bytes / evidence 元数据。

常见偏差：

- continuation 写入 workspace deliverable 目录
- 只存 chunk 内容，不存足够 metadata/hash
- 重启后 sequence/hash 无法恢复

## 5. Read Model / Projection

重点核对：

- `WorkspaceSnapshot` / `RunSnapshot` / `RunObservability` 是否存在 `artifactContinuations` 投影。
- 默认投影是否只返回 metadata，不内嵌 plan/chunk/assembled 正文。
- observability 是否只给 compact metadata + refs，而不是正文。
- raw/detail 打开是否走独立受控路径。
- continuation projection 是否不会替代正式 `Result` / `Artifact` / `Validation` 投影。

常见偏差：

- snapshot 默认返回 chunk content
- observability 混入 assembled 正文
- continuation 投影覆盖正式 artifact/result 视图

## 6. Service API

重点核对：

- `artifactContinuations` 是否出现在 `RunSnapshot` / `WorkspaceSnapshot`，且默认只返回 metadata。
- `runtime/payloads` / `artifacts/content` 是否是显式 raw/detail 打开路径。
- snapshot / observability 是否不会默认暴露 continuation 正文。
- raw 打开接口是否拒绝非受控 ref / uri。
- continuation 正文是否只能通过显式 raw/detail 访问。

常见偏差：

- API 默认返回 continuation 正文
- raw 接口接受任意文件路径
- snapshot 和 raw/detail 责任混淆

## 7. Web / TUI

重点核对：

- Web board/detail/observability 是否只展示 continuation metadata/progress。
- Web 是否不会把 plan/chunk/assembled 正文直接嵌入主页面。
- TUI 主视图是否只展示 active continuation 的 target/state/progress/failure summary。
- TUI review 是否不会默认展开 chunk/assembled 正文。
- raw/detail 打开失败时，前端/TUI 是否只显示错误，不自行拼装替代内容。

常见偏差：

- 页面默认直接展示正文
- TUI 主视图退化成 debug/raw 内容面板
- raw/detail 失败后前端尝试“猜测”替代内容

## 8. Evidence / Acceptance

重点核对：

- acceptance 是否只能引用 apply / validation evidence，不能直接引用 chunk。
- apply evidence 是否绑定 artifact 级 `coverageTargets`。
- validation evidence 是否引用 apply evidence 或同 scope coverage target。
- chunk/plan 失败是否进入 repair/governance，而不是被静默忽略。
- validation 失败后是否针对已落地 artifact 修，而不是回到 chunking。

常见偏差：

- 把 chunk 当成 satisfied evidence
- validation 失败后错误回退到 chunking
- apply evidence 没有绑定 coverage target

## 9. Failure Recovery

重点核对：

- provider timeout / schema invalid / parse failure 发生在 chunk 阶段时，是否只重试当前 sequence。
- 同一 sequence 多次不同 hash 是否进入 chunk repair / artifact replan。
- artifact apply 失败是否进入普通 state-changing repair。
- artifact plan 不合法时是否拒绝 plan 并重试 plan，而不是直接进入 chunking。
- 服务重启后是否不会丢失 sequence / hash / coverageTargets。

常见偏差：

- 错误重跑全部 artifact
- 不同 hash 被静默拼接
- plan 非法仍进入 chunking

## 10. Verifier / Tests

重点核对：

- 是否有单测覆盖 selector。
- 是否有单测覆盖 continuation lifecycle。
- 是否有 repository 测试覆盖 restart/reload。
- 是否有 appservice/read-model 测试覆盖 metadata-only projection。
- 是否有 e2e 覆盖：
  - continuation lifecycle
  - apply/validation routing
  - raw content 不泄漏
- 是否有断言保证 mode-specific schema 互斥。

常见偏差：

- 只有 happy-path e2e，没有恢复链和 redaction 断言
- 只有 orchestrator 测试，没有 read-model / API 投影测试

## 11. High-Risk Deviations

以下偏差应优先检查：

1. `outputMode` 只存在内部变量，没投影到 LLM-facing stage contract。
2. `incrementalArtifact` 被当成新的 stage/state。
3. snapshot/observability 默认返回 chunk content。
4. mode 回退到 `executionGraph` 后，遗留 `artifactOutputContract`。
5. apply 前就把 chunk 当 evidence。
6. completion follow-up 误继承 chunk/plan contract。
7. 服务恢复后重新让 LLM 生成已确认 chunk。
8. raw tail 正文被塞回 prompt / memory / event payload。

## 2026-06-22 Code Alignment Result

| 项目 | 代码位置 | 结果 | 备注 |
| --- | --- | --- | --- |
| core contract | `internal/modules/kernel/orchestrator/biz/runtime_output_mode_selector.go`, `runtime_artifact_output_contract.go`, `runtime_artifact_output_contract_test.go`, `runtime_output_mode_selector_test.go` | pass | `incrementalArtifact` 作为 output mode / request-side contract 存在，不是新的 run/step state；selector 使用 LLM output budget、open continuation、requested mode 和 structured retry 信号，不依赖模板名、业务语言或文件名；顶层 `outputMode`、graph/artifact 互斥、plan/chunk/repair 互斥和 chunk hash/sequence/tail 检查均有合同测试。 |
| flow routing | `runtime_artifact_assemble.go`, `runtime_artifact_assemble_test.go`, `runtime_resume_contract_integration_test.go` | pass | 主链按 plan -> chunk -> assemble -> apply -> validation/acceptance 推进；open continuation 恢复优先继续当前 continuation，terminal continuation 回落到 `executionGraph` 并清理 incremental-only 字段；chunk rejected 不推进 cursor。 |
| provider assembly | `internal/modules/blocks/llm/biz/prompt_messages.go`, `prompt_messages_test.go`, `runtime_adapter.go`, `runtime_adapter_test.go`, `docs/30-module-architecture/llm-provider-assembly.md` | pass | LLM-facing stage contract 投影 `outputMode`、`requestedOutputMode`、selector、artifact contract 和 metadata-only continuation context；LLM adapter 输出 artifact chunk / chunk repair structured stage；completion follow-up 不继承已完成 artifact 的 chunk/plan contract。 |
| persistence | `internal/pkg/workorchruntimecontract/artifact_continuation.go`, `internal/pkg/repository/artifact_continuation_repository.go`, `internal/pkg/repository/{file,memory}/artifact_continuation_repository.go`, `internal/pkg/repository/repository_contract_test.go`, `internal/pkg/repository/file/file_test.go` | pass | continuation 是 run-scoped internal state；file/memory repository 支持 metadata、plan、chunk content、assembled content、open listing 和 reload contract；file repository 测试证明 chunk/assembled 正文存储在 metadata 外。 |
| read-model | `readmodelcore/artifact_continuation_types.go`, `readmodelprojection/artifact_continuation*.go`, `readmodelprojection/run_snapshot_base.go`, `readmodelprojection/workspace_snapshot.go`, `workspace_snapshot_test.go`, `read_model_service_test.go` | pass | RunSnapshot / WorkspaceSnapshot 投影 `artifactContinuations` metadata/progress；默认 read model 不内嵌 plan/chunk/assembled 正文，continuation 投影不替代正式 Result / Artifact / Validation 视图。 |
| API | `service/server/http_run.go`, `http_test.go`, `artifact_content_service.go`, `runtime_raw_content_service.go`, corresponding service tests | pass | snapshot 默认暴露 metadata；正文通过 `/api/v1/workorch/artifacts/content?uri=...` 与 `/api/v1/workorch/runtime/payloads?ref=...` 显式 raw/detail 入口读取；service tests 覆盖 unsafe URI/ref 拒绝。 |
| Web/TUI | `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`, `web/workorch-console/src/api.ts`, `internal/services/workorch/service/tui/{model.go,view.go,inspect_commands.go,workspace_state.go}` | pass | Web detail 以 table/summary 展示 continuation metadata，并使用显式 content URL 打开正式 artifact；TUI 主视图只展示 target/state/progress/failure summary，不默认展开 chunk/assembled 正文。 |
| tests | `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz ./internal/pkg/repository ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui -count=1` | pass locally on 2026-06-22 | 覆盖 selector、contract enforcement、continuation lifecycle、repository metadata/content split、read-model/API raw/detail、TUI continuation display。 |

### Closed In This Pass

- 该审计文档此前只有核对模板，没有实际代码核对结果表；本轮将当前实现证据固化到具体代码和测试路径。
- 未发现需要立即修复的 `incrementalArtifact` core / provider / persistence / projection / API / TUI-Web 代码偏差。

### Remaining Follow-Up

- 后续如新增 continuation raw/detail UI，仍必须保持 metadata-first 主视图和显式 raw/detail 打开边界。
- 后续如新增新的 output mode selector 信号，必须继续保持业务中立，不能按 template、语言、文件名或具体工具输出文本分支。

## Suggested Code Landing Paths

核对时可优先检查以下代码目录：

- `internal/modules/kernel/orchestrator`
  - mode selector、dispatch、continuation lifecycle、repair/recovery
- `internal/modules/blocks/llm`
  - request assembly、schema、request audit、protected field / stage-contract projection
- `internal/pkg/repository`
  - artifact continuation persistence、reload/restart contract
- `internal/modules/application/workorchd`
  - snapshot / observability / raw content API projection
- `web/workorch-console/src`
  - metadata-first 展示与 raw/detail 打开边界
- TUI 相关入口
  - active continuation 的 compact progress 展示边界

## Review Output Template

逐项核对时，建议按以下格式记录：

| 项目 | 代码位置 | 结果 | 备注 |
| --- | --- | --- | --- |
| core contract | `path/to/code` | pass / partial / fail | 简述偏差 |
| flow routing | `path/to/code` | pass / partial / fail | 简述偏差 |
| provider assembly | `path/to/code` | pass / partial / fail | 简述偏差 |
| persistence | `path/to/code` | pass / partial / fail | 简述偏差 |
| read-model | `path/to/code` | pass / partial / fail | 简述偏差 |
| API | `path/to/code` | pass / partial / fail | 简述偏差 |
| Web/TUI | `path/to/code` | pass / partial / fail | 简述偏差 |
| tests | `path/to/code` | pass / partial / fail | 简述偏差 |
