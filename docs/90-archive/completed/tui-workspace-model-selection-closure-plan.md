# TUI Workspace Model Selection Closure Plan

Status: completed

Type: product-closure-plan

Last Updated: 2026-07-16

## 1. Purpose

为 WorkOrch TUI 增加 workspace-scoped 的交互式 provider/model 选择能力：用户在当前激活 workspace 中执行 `/model`，系统自动列出当前可用的 chat provider/model target，用户选择并确认后，将完整 `providerId/modelId` 持久化到该 workspace；后续新建或 replacement Runtime Run 默认使用该 target。

本文是该功能代码收口的实施依据。实现仍必须遵守：

- 生产路径只使用 Runtime。
- 已创建 Run 的 `RuntimeBootstrap` / provider ceiling 保持封存，不允许中途漂移。
- workspace 正式状态归 `blocks/workspace` owner 持有。
- TUI 只依赖 `internal/pkg/workorchport`，不得直接访问 HTTP client、EventHub owner service 或 repository。
- remote 与 embedded TUI 必须使用同一稳定 port 语义。

核心运行约束继续以 [runtime-stage-contract.md](../../00-core/runtime-stage-contract.md) 和 [kernel/orchestrator/readme.md](../../../internal/modules/kernel/orchestrator/readme.md) 为准。

## 2. Current Facts

截至 2026-07-16，代码实现已具备：

- workspace owner 持久化 typed `DefaultLLMTarget`，并通过专用 EventHub command 原子更新或 clear。
- `workorchport.ConfigBackend` / `WorkspaceBackend` 已直接包含模型目录与 workspace target 稳定合同，不依赖可选 type assertion。
- embedded EventHub backend 与 remote HTTP backend 已共享同一 port 语义。
- TUI `/model`、`/model show`、`/model clear` 已实现完整 `providerId/modelId` 选择、双确认、刷新、取消与 stale response 防护。
- 新 Run 已按 explicit activation → workspace → current system `defaultChat` 解析；既有 sealed Run 保持不漂移。
- service API 文档、file-backed owner restart、HTTP/client、TUI transport integration 与 MODEL-01～18 直接测试已补齐。

发布收口结果：

1. 包含 Runtime `usedCapabilities` read-model 修复的最新二进制已部署。
2. 本功能专用 deployed e2e 与 `make verify-ci-mvp-file` 均已通过。
3. MODEL-01～18、全量测试、vet、boundary、build 与文档同步全部完成。

## 3. Final Product Contract

### 3.1 Canonical command

正式入口：

```text
/model
```

辅助命令：

```text
/model show
/model clear
```

不新增 `/workspace model` 作为并列正式入口，避免同一能力存在两套命令语义。

### 3.2 `/model` interaction

执行 `/model` 后：

1. 必须存在当前激活 workspace；否则返回明确错误。
2. 读取当前可用 chat target；每个选项必须是完整的 `providerId/modelId` 组合。
3. 只展示：
   - provider/model identity 完整；
   - 支持 `chat_completions`；
   - capability ready；
   - 未被禁用。
4. 列表按 provider 分组展示，单项标签至少包含 `providerId / modelId`；用户一次确认同时选择 provider 和 model，不存在“只保存 model、运行时再猜 provider”的路径。
5. 默认选中顺序：
   - workspace 当前持久化 target；
   - 系统 `defaultChat`；
   - 列表第一项。
6. 进入独立选择模式：
   - `↑/↓`、`j/k`：移动；
   - `Enter`：进入确认；
   - 再次 `Enter`：提交配置；
   - `Esc`：取消；
   - `r`：强制刷新模型目录；
   - `/`、普通输入和其他 workspace command 在选择模式结束前不执行。
7. 确认视图必须展示：
   - workspace ID/title；
   - 当前 `providerId/modelId`；
   - 将写入的新 `providerId/modelId`；
   - 生效范围说明。
8. 提交成功后退出选择模式，并刷新 workspace 投影。

### 3.3 `show` and `clear`

`/model show` 展示：

- workspace 持久化 target；
- 若未配置，展示“继承系统默认”；
- 当前系统 `defaultChat`；
- 当前 Run 已封存 target（若投影可用），并明确区分“当前 Run”与“后续 Run”。

`/model clear` 必须二次确认。成功后删除 workspace target，后续 Run 恢复继承届时最新的系统 `defaultChat`。

### 3.4 Submission side effects

模型配置提交本身：

- 不自动创建 Run；
- 不自动 Resume；
- 不修改 active Run；
- 不修改系统 `defaultChat`；
- 不修改其他 workspace。

提交完成后的提示：

- workspace 无 active Run 或历史 Run 已终态：提示使用 `/run resume`；
- workspace 有 active Run：提示当前 Run 不变，使用 `/run restart` 创建 replacement Run 后生效；
- workspace 处于 decision/approval blocker：提示先完成治理，再创建后续 Run。

## 4. Persisted Data Contract

### 4.1 Workspace owner field

在 workspace 正式模型增加可选 typed target：

```go
DefaultLLMTarget *workorchruntimecontract.ProviderModelTarget
```

产品命令名虽然是 `/model`，其正式语义是选择完整 provider/model target；持久化必须保存 `providerId + modelId`，原因是：

- 不同 provider 可能存在同名 model；
- model readiness、capability revision 与 provider 路由绑定；
- Runtime provider ceiling 必须封存明确 identity；
- 避免系统 default provider 改变后把旧 model ID 错误路由到另一 provider。

该字段不得保存 API key、base URL、provider client 或 catalog payload。

### 4.2 Compatibility and migration

- 旧 workspace 缺少字段时按 `nil` 处理，即继承系统默认。
- file/memory repository 使用现有结构序列化，无需数据回填任务。
- clone、mapper、projection、EventHub payload 必须深拷贝 target pointer。
- 删除偏好写入 `nil`，不得写空 provider/model 对象。

### 4.3 Dedicated owner command

不得让 TUI/application 通过“读完整 workspace → 改字段 → 全量 UpdateWorkspace”完成更新，避免覆盖并发变化的 `ActiveRunID`、state、bindings 或 context hints。

新增 workspace owner 专用 typed command：

```go
type UpdateWorkspaceLLMTargetCommand struct {
    WorkspaceID common.WorkspaceID
    Target      *workorchruntimecontract.ProviderModelTarget
}
```

要求：

- 使用 workspace state lane；
- owner 内原子读取、校验 scope、只更新目标字段；
- 返回更新后的 workspace；
- 发布 workspace changed notification；
- duplicate same-target update 幂等成功；
- `Target=nil` 表示 clear。

## 5. Model Catalog Contract

### 5.1 Stable port DTO

在 `internal/pkg/workorchport` 增加面向 TUI/CLI 的中立 DTO：

```go
type ChatModelTargetOption struct {
    ProviderID         string
    ModelID            string
    DisplayName        string
    ContextWindowTokens int
    MaxOutputTokens    int
    CapabilityReady    bool
    CapabilityRevision string
    Source             string
}
```

稳定 port 不暴露 blocks/llm owner model、HTTP response 或 provider adapter 类型。

### 5.2 Config backend surface

扩展 `ConfigBackend`：

```go
type ConfigBackend interface {
    Get(ctx context.Context) (*ServiceConfig, error)
    ListChatModelTargets(ctx context.Context) ([]ChatModelTargetOption, error)
    RefreshChatModelTargets(ctx context.Context) ([]ChatModelTargetOption, error)
}
```

实现责任：

- embedded/eventhub backend：发送现有 typed catalog command，并组合 custom provider 中声明 `chat_completions` 的 ready target；
- remote backend：通过 `workorchclient` 调用 configuration model catalog API，并映射为 port DTO；
- fake/test backend：提供确定性选项。

列表必须先按 provider 分组，再稳定排序：

1. 当前 workspace target 所属 provider/group；
2. 系统 defaultChat 所属 provider/group；
3. provider ID；
4. 同 provider 下的 model ID。

去重 key 为规范化后的 `providerId/modelId`。

### 5.3 Refresh semantics

`r` 触发 force refresh。刷新失败时：

- 保留当前已加载列表；
- 展示 refresh error；
- 不清除已选择项；
- 若无任何可用旧列表，则保持选择模式并允许取消。

## 6. Workspace Update Surface

### 6.1 Stable workspace port

扩展 `WorkspaceBackend`：

```go
type WorkspaceBackend interface {
    // existing methods omitted
    UpdateLLMTarget(
        ctx context.Context,
        workspaceID common.WorkspaceID,
        target *workorchruntimecontract.ProviderModelTarget,
    ) (*WorkspaceInfo, error)
}
```

`WorkspaceInfo` 增加 `DefaultLLMTarget` 投影。

### 6.2 Remote HTTP API

新增：

```text
PUT /api/v1/workorch/workspaces/{workspaceId}/llm-target
DELETE /api/v1/workorch/workspaces/{workspaceId}/llm-target
```

PUT body：

```json
{
  "providerId": "ai-proxy",
  "modelId": "DeepSeek-V4-Flash"
}
```

DELETE 清除 workspace target。

HTTP handler 只负责 request/response、scope 和 error mapping；业务校验和 owner command 发送落在 application use-case/port。

### 6.3 Client/backend implementation

- `internal/pkg/workorchclient` 增加 get/list catalog、update/clear workspace target 方法。
- `internal/services/workorch/biz/backends/remote.go` 仅做 client ↔ port DTO mapping。
- `internal/services/workorch/biz/backends/eventhub.go` 通过 application/owner typed EventHub contract 更新，不直接访问 workspace repository。

## 7. Activation Resolution

新 Run 构造 typed activation 时，provider target 优先级固定为：

```text
activation request explicit llmTarget
→ workspace.DefaultLLMTarget
→ current system llm.defaultChat
```

规则：

- Web 启动弹窗的显式选择仍是本次 Run override，不自动改写 workspace 默认。
- TUI `/model` 更新 workspace 默认，不生成本次 activation override。
- activation 解析出最终 target 后执行现有 readiness check。
- readiness 失败必须返回明确错误，不允许静默换到其他模型或系统默认。
- 解析后的 target 继续封存到 Run `RuntimeBootstrap.Activation.ProviderTarget`。
- active Run、governed wait Run 与重启恢复中的既有 aggregate 不读取更新后的 workspace target。

这一规则保持 Runtime 的 authority/ceiling/cache audit 一致性，同时允许后续新 Run 使用 workspace 最新选择。

## 8. TUI State Design

在 `AppModel` 增加独立 model selection state，建议至少包含：

```go
modelSelectMode       bool
modelSelectLoading    bool
modelSelectRefreshing bool
modelSelectConfirming bool
modelSelectCursor     int
modelSelectOptions    []backend.ChatModelTargetOption
modelSelectCurrent    *runtimecontract.ProviderModelTarget
modelSelectPending    *runtimecontract.ProviderModelTarget
modelSelectError      error
```

要求：

- workspace 切换时清除选择状态，避免跨 workspace 提交。
- 异步 catalog/update response 必须携带 workspace ID 和 request sequence；过期响应不得应用。
- selection mode 的键盘处理优先于普通输入、slash suggestion、decision/approval 输入。
- 提交期间禁止重复 Enter。
- 视图宽度不足时仍必须显示 provider/model 的可辨识截断值。

消息建议：

```go
type ChatModelTargetsLoadedMsg struct { ... }
type WorkspaceLLMTargetUpdatedMsg struct { ... }
```

## 9. Error and Edge Cases

必须显式覆盖：

| 场景 | 预期行为 |
| --- | --- |
| 无 active workspace | `/model` 返回错误，不加载目录 |
| catalog 空 | 显示无可用 chat model，并提供 `r`/`Esc` |
| 当前 workspace target 已从 catalog 消失 | `show` 仍显示 persisted target；列表标记 unavailable，不允许直接确认旧 target |
| catalog refresh 失败 | 保留旧列表并显示错误 |
| workspace 在选择期间切换 | 取消旧选择，丢弃旧响应 |
| workspace 被删除 | 更新返回 not found，退出提交状态 |
| target 在确认后失效 | application readiness 校验失败，不写入 workspace |
| active Run 存在 | 配置成功；当前 Run 不变，提示 restart |
| same target 重复提交 | 幂等成功 |
| clear 未配置 target | 幂等成功 |
| remote 连接中断 | 保留选择状态，允许重试或取消 |

## 10. Module Landing Matrix

| 责任 | 正式落点 |
| --- | --- |
| `/model` 命令解析、选择状态、键盘和视图 | `internal/services/workorch/service/tui` |
| 稳定 TUI/CLI DTO 与 backend interface | `internal/pkg/workorchport` |
| Remote HTTP 编解码 | `internal/pkg/workorchclient` |
| Remote/EventHub backend 实现 | `internal/services/workorch/biz/backends` |
| HTTP route/request-response | `internal/modules/application/workorchd/service/server` |
| workspace 更新 use-case/owner port | `internal/modules/application/workorchd/biz/appservice` focused packages |
| 正式 target 状态和原子字段更新 | `internal/modules/blocks/workspace` |
| shared provider/model identity | `internal/pkg/workorchruntimecontract` |
| activation 优先级与 readiness | `internal/modules/application/workorchd/biz/appservice/activationports` |

不新增 framework module、block 或 initiator。

## 11. Implementation Sequence

### Phase M1 — Owner and data contract

- [x] workspace model 增加 `DefaultLLMTarget`。
- [x] 增加 clone/mapping/projection/repository 覆盖。
- [x] 增加专用 `UpdateWorkspaceLLMTargetCommand`。
- [x] owner command 实现字段级原子更新与通知。
- [x] 旧 workspace nil compatibility 测试。

### Phase M2 — Application and API

- [x] 增加 update/clear workspace target use-case。
- [x] 更新前通过 LLM typed catalog/readiness contract 验证 target。
- [x] 增加 HTTP PUT/DELETE route。
- [x] 更新 service API 文档和 client types。

### Phase M3 — Stable ports and backends

- [x] `ConfigBackend` 增加 list/refresh chat target。
- [x] `WorkspaceBackend` 增加 update/clear target。
- [x] 完成 eventhub backend。
- [x] 完成 remote backend/client。
- [x] 更新 fake backend 与 adapter contract tests。

### Phase M4 — Activation semantics

- [x] 实现 explicit → workspace → system 优先级。
- [x] 保持 readiness、provider ceiling、RuntimeBootstrap seal。
- [x] 增加 active Run 不漂移测试。
- [x] 增加 clear 后回退系统默认测试。

### Phase M5 — TUI interaction

- [x] 注册 `/model`、`show`、`clear` suggestions/help，并明确 `/model` 选择完整 `providerId/modelId`。
- [x] 实现 catalog loading/refresh messages。
- [x] 实现选择、确认、提交、取消模式。
- [x] workspace switch/stale response 防护。
- [x] 成功后刷新 workspace projection并给出 resume/restart 指引。

### Phase M6 — Verification and docs

- [x] targeted owner/application/port/client/backend/TUI tests。
- [x] embedded TUI integration verification。
- [x] remote TUI integration verification。
- [x] file-backed restart persistence verification。
- [x] 更新 `current-state.md`、service API 与 TUI help 文档。
- [x] code-closed 后迁入 `docs/90-archive/completed/` 并从 active matrix 移除。

## 12. Verification Matrix

| ID | 验证项 | 层级 |
| --- | --- | --- |
| MODEL-01 | `/model` 无 workspace 明确失败 | TUI unit |
| MODEL-02 | 目录只列 ready chat `providerId/modelId` target，并按 provider 分组 | port/backend |
| MODEL-03 | 默认光标优先 workspace target | TUI unit |
| MODEL-04 | Enter 二次确认后才写入 | TUI unit |
| MODEL-05 | Esc 不产生写入 | TUI unit |
| MODEL-06 | `r` 强制刷新且失败保留旧列表 | TUI/backend |
| MODEL-07 | workspace 切换丢弃 stale response | TUI unit |
| MODEL-08 | target 持久化后服务重启仍存在 | file repository/integration |
| MODEL-09 | duplicate update 幂等 | workspace owner |
| MODEL-10 | clear 恢复 nil/inherit | owner/application |
| MODEL-11 | explicit activation 覆盖 workspace target | activation |
| MODEL-12 | workspace target 覆盖 system default | activation |
| MODEL-13 | 无 workspace target 使用最新 system default | activation |
| MODEL-14 | active Run bootstrap 不因配置变化漂移 | Runtime contract |
| MODEL-15 | unavailable target 不写入且明确报错 | application/TUI |
| MODEL-16 | embedded 与 remote TUI 行为一致 | integration |
| MODEL-17 | HTTP PUT/DELETE scope 与 error mapping 正确 | server/client |
| MODEL-18 | workspace owner 更新不覆盖 ActiveRunID 等并发字段 | owner regression |

### 12.1 Direct Evidence

| ID | 直接证据 |
| --- | --- |
| MODEL-01 | `TestModelCommandRequiresActiveWorkspace` |
| MODEL-02 | `TestNormalizeChatModelTargetOptionsKeepsOnlyReadyCompleteUniqueTargets`、`TestEmbeddedAndRemoteModelTargetBackendsSharePortSemantics` |
| MODEL-03 | `TestModelOptionsPreferCurrentWorkspaceProviderGroup`、`TestModelCommandSelectsAndPersistsProviderModelPair` |
| MODEL-04 | `TestModelCommandSelectsAndPersistsProviderModelPair` |
| MODEL-05 | `TestModelEscCancelsWithoutWrite` |
| MODEL-06 | `TestModelRefreshFailurePreservesLoadedOptions`、backend refresh parity |
| MODEL-07 | `TestModelSelectionDiscardsStaleWorkspaceResponse`、`TestWorkspaceDeletedDuringModelUpdateExitsSelection` |
| MODEL-08 | `TestWorkspaceLLMTargetSurvivesFileBackedOwnerRestart`、`TestWorkspaceRepositoryPersistsAndClonesDefaultLLMTarget` |
| MODEL-09 | `TestUpdateWorkspaceLLMTargetCommandPersistsCompleteTargetAndClears` duplicate update branch |
| MODEL-10 | `TestModelClearRequiresConfirmationAndPersistsNil`、owner clear branch |
| MODEL-11 | `TestResolveActivationProviderTargetPrecedence` explicit branch |
| MODEL-12 | `TestResolveActivationProviderTargetPrecedence` workspace branch |
| MODEL-13 | `TestResolveActivationProviderTargetUsesLatestInjectedSystemDefault` |
| MODEL-14 | `TestSealedRunProviderTargetDoesNotDriftAfterWorkspacePreferenceChanges` |
| MODEL-15 | `TestUpdateWorkspaceLLMTargetRejectsUnavailableTargetBeforeOwnerWrite`、`TestUnavailablePersistedTargetIsShownButCannotConfirm` |
| MODEL-16 | `TestModelSelectionFlowMatchesEmbeddedAndRemoteBackends`、backend port parity |
| MODEL-17 | `TestWorkspaceLLMTargetRoutesValidateScopeAndPersistCompleteTarget`、`TestClientWorkspaceLLMTargetRoutesCarryProviderAndModel` |
| MODEL-18 | `TestUpdateWorkspaceLLMTargetCommandPersistsCompleteTargetAndClears` 保留 `ActiveRunID` |

### 12.2 Verified Commands

已通过：

```bash
make fmt-check
make vet
make verify-boundary-contract
make build
make test
make verify-ci-mvp-file
```

最新部署实例的 feature-specific e2e 已通过：从 ready chat catalog 自动选择
`ai-proxy/DeepSeek-V4-Flash`，创建临时 workspace，验证 PUT target、GET 持久化投影、
DELETE clear 与无 active Run 副作用，随后清理 workspace。

最新部署实例上的 `make verify-ci-mvp-file` 已通过：

- WorkOrch HTTP verification passed。
- workspace template and soul e2e verification passed。
- LLM workspace execution verification passed。
- MCP stdio verification passed；Runtime committed MCP dispatch 已正确投影到 `usedCapabilities`。

推荐命令：

```bash
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/pkg/workorchclient ./internal/pkg/workorchport -count=1
go test ./internal/services/workorch/biz/backends/... -count=1
go test ./internal/services/workorch/service/tui/... -count=1
make fmt-check
make vet
make verify-boundary-contract
make test
```

## 13. Definition of Done

本计划仅在以下条件全部满足时关闭：

1. `/model` 在 embedded 与 remote TUI 中均可自动列出、选择、确认和提交完整 `providerId/modelId`。
2. workspace target 由 workspace owner typed state 持久化，服务重启后可读回。
3. 新 Run 遵守 explicit → workspace → system 优先级。
4. 既有 active/sealed Run 不发生 provider/model 漂移。
5. target unavailable、workspace switch、refresh failure、clear 等边界有直接测试。
6. Web 单次 activation target 行为无回归。
7. 全量测试、vet、boundary contract 通过。
8. 事实文档更新，计划迁入 completed archive，active overview 与 unclosed matrix 清零该项。

## 14. Non-Goals

- 不在本计划中实现运行中热切换 provider/model。
- 不自动重启或 Resume workspace。
- 不修改系统 `defaultChat`。
- 不实现 provider failover、负载均衡或按成本自动选模。
- 不把 provider API key/base URL 写入 workspace。
- 不让 TUI 直接访问 blocks/llm、workspace repository 或 HTTP implementation。
