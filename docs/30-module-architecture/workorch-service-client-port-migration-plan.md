# WorkOrch Service / Client / Port 架构迁移最终方案

## 目标

本文件用于固化 `application/workorch`、`internal/services/workorch`、`internal/pkg/workorchclient`、`internal/pkg/workorchport` 与 `internal/modules/application/workorchd` 的最终责任边界，并作为后续逐项收口和验收依据。

当前阶段不考虑旧路径兼容性。迁移完成后，旧路径 `internal/modules/application/workorch` 应删除，所有引用同步切换到新结构。

## 背景结论

`internal/modules/application/workorch` 原先实际承担的是 workorch 可执行进程的应用壳职责：

- 解析启动参数和运行模式。
- 管理 remote / server / interactive 模式。
- 启动和关闭 `magicCommon/framework/application`。
- 启动 CLI/TUI。
- 为 CLI/TUI/agent 连接 remote HTTP backend 或 embedded EventHub backend。

该目录没有 `module.go`，不通过 `framework/plugin/module` 注册，也不拥有正式 owner state。因此它不应继续放在 `internal/modules/application`，而应调整为进程级 service。

`application/workorch` 也不应承载实质业务功能。它只保留可执行入口、signal context、显式 side-effect imports 和进程 service 调用。

## 设计原则

- `internal/modules` 只承载真正的 framework initiator / module / block 运行单元。
- `internal/services` 承载进程级 framework service 和 lifecycle shell。
- `application/<entry>` 只保留入口装配，不放 appservice、bootstrap、server、presenter、observed、CLI/TUI 实现。
- 进程级 service 可以实现 `framework/service.LifecycleService`，并由入口通过 `framework/application` 启停。
- plugin module 与 process service 是不同边界，不能因为 service 含有 CLI/TUI/remote/server 编排就注册成 module。
- CLI/TUI/agent 只依赖稳定 port 契约，不直接依赖 REST client、backend 实现、kernel owner service 或 repository。
- remote backend 只通过 REST client 访问 HTTP API。
- embedded backend 只通过 EventHub command/response 访问正式 owner state。
- module/block 间涉及正式状态读写、状态机转换、evidence 查询时，只能通过 EventHub event 交互。
- 如确需运行期注入，只注入 initiator 暴露的基础设施、repository provider、runtime policy、route registry、client factory 或 EventHub-backed adapter，不注入跨 owner service 本体。
- `Weight()` 只表达同类插件生命周期顺序，不用于隐藏架构依赖。
- 不因文件行数拆分 module/block。拆分依据是生命周期、状态权威、通信契约、复用价值和职责清晰度。

## 最终结构

```text
application/workorch/
  main.go
  main_test.go
  server_test.go

internal/services/workorch/
  service.go
  runtime.go
  options.go
  config.go
  logging.go

  biz/
    agent/
    config/
    session/
    backends/
      remote.go
      eventhub.go
      helpers.go
      runtime_kind.go
      snapshot.go
      port_alias.go

  service/
    cli/
    tui/

  pkg/
    common/

internal/pkg/workorchclient/
  client.go
  workspace.go
  run.go
  decision.go
  approval.go
  management.go
  errors.go
  path_helpers.go
  types.go

internal/pkg/workorchport/
  backend.go
  workspace.go
  run.go
  result_artifact.go
  observability.go
  definition.go
  governance.go
  context_registry.go
  config_events.go
  template_defaults.go

internal/modules/application/workorchd/
  module.go
  biz/
    bootstrap/
    appservice/
  service/
    server/
  pkg/
    common/
    observed/
    presenter/
```

`internal/pkg/workorchport` 的拆分只改变文件组织，不改变 port 契约边界。拆分规则如下：

- `backend.go`: 只保留 `Backend` 聚合接口、订阅接口和跨资源共用类型。
- `workspace.go`: workspace、workspace context、workspace snapshot 相关 DTO 和接口。
- `run.go`: run lifecycle、run snapshot、run control 相关 DTO 和接口。
- `result_artifact.go`: result、artifact、deliverable、patch/diff 相关 DTO 和接口。
- `observability.go`: event stream、runtime event、LLM usage、coverage/progress 相关 DTO 和接口。
- `definition.go`: goal、DoD、template、soul、memory、skill、MCP capability 等定义类 DTO 和接口。
- `governance.go`: decision、approval、ticket/resolution 相关 DTO 和接口。
- `context_registry.go`: context registry 查询和注册契约。
- `config_events.go`: config、runtime config、system event 相关契约。
- `template_defaults.go`: port 层可复用的纯 helper。

如果后续继续拆分，只能依据资源边界、状态权威或契约语义拆分；不能为降低文件行数而增加只有转调意义的 wrapper。

## 责任边界

### `application/workorch`

职责：

- 显式 import 本入口需要启用的 initiator / module / block。
- 创建 signal context。
- 调用 `internal/services/workorch` 创建进程 service。
- 将进程 service 接入 `framework/application` 启停链路。

禁止：

- 承载 CLI/TUI 业务逻辑。
- 承载 backend 实现。
- 承载 appservice、bootstrap、server、presenter、observed 等实现包。
- 通过业务包间接触发 module 注册。

### `internal/services/workorch`

这是 workorch 可执行进程的 process service，不是 plugin module。

职责：

- 实现 `framework/service.LifecycleService` 语义的 `Startup(ctx)`、`Run(ctx)`、`Shutdown(ctx)`。
- 管理 remote / server / interactive 模式。
- 管理 `framework/application` runtime 启停。
- 连接 CLI/TUI/agent 与 backend 实现。
- 承载进程级配置、日志和默认 workspace session 策略。

禁止：

- 注册为 `framework/plugin/module`。
- 拥有正式业务状态。
- 直接访问 kernel owner repository。
- 暴露跨 owner service facade。
- 作为局部 runtime 的 service locator。

### `internal/services/workorch/service/cli`

CLI 是命令式管理入口。

职责：

- Cobra 命令注册和参数解析。
- 面向 workspace / goal / dod / run / decision / approval / soul / skill / mcp / memory / template / config 的一次性命令。
- 支持 remote、JSON 和脚本化调用。
- 作为 TUI 启动入口。

依赖规则：

- 只依赖 `internal/pkg/workorchport` 和进程内局部 config/helper。
- 不直接 import `internal/pkg/workorchclient`。
- 不直接 import backend implementation。
- 不直接 import kernel owner service/repository。

### `internal/services/workorch/service/tui`

TUI 是交互式工作台和 agent session UI。

职责：

- Bubble Tea 长会话状态机。
- active workspace / active run 管理。
- workspace context 聚合展示。
- goal 提交、run 启动、run 继续、run 重启。
- backend event 订阅和实时状态刷新。
- 展示 runtime events、decision、approval、artifact、patch、coverage、LLM usage 等。

依赖规则：

- 只依赖 `internal/pkg/workorchport`。
- 不直接 import `internal/pkg/workorchclient`。
- 不直接 import backend implementation。
- 不直接 import kernel owner service/repository。

### CLI 与 TUI 的功能区别

CLI 与 TUI 都是 workorch process service 的 adapter，但交互模型不同：

- CLI 面向一次性命令、脚本化调用和 CI 验证，命令执行完成即返回。
- CLI 优先关注参数解析、JSON 输出、远程服务管理、实体 CRUD、run/decision/approval 的显式操作。
- TUI 面向长会话工作台，维护屏幕状态、active workspace、active run、输入草稿和实时事件流。
- TUI 优先关注 run 执行过程展示、交互式继续执行、decision/approval 处理、artifact/patch/coverage/LLM usage 可视化。
- CLI/TUI 共享的 agent 用例只能下沉到 `biz/agent` 或 `biz/session`，不能让 TUI 直接复用 CLI command，也不能让 CLI 依赖 TUI model。
- 两者访问 workorch 能力的共同边界都是 `internal/pkg/workorchport`。

### `internal/services/workorch/biz/agent`

职责：

- 承载 CLI/TUI 共享的 agent 用例编排。
- 将用户目标转为 workspace goal / dod / run 操作。
- 根据 port 返回的状态判断下一步。

依赖规则：

- 只通过 `workorchport.Backend` 调用 workorch 能力。
- 不直接访问 owner module service/repository。

### `internal/services/workorch/biz/session`

职责：

- 根据工作目录、CLI 配置和 backend workspace 列表解析默认 workspace。
- 必要时创建默认 workspace。
- 为 TUI/agent 提供启动上下文。

边界：

- 这是进程启动 session 策略，不是正式 workspace owner。
- 创建、读取 workspace 必须通过 `workorchport.Backend`。

### `internal/services/workorch/biz/backends`

职责：

- 实现 `workorchport.Backend`。
- `remote.go` 基于 `workorchclient` 实现 remote backend。
- `eventhub.go` 基于 EventHub command/response 实现 embedded backend。
- `snapshot.go`、`helpers.go`、`runtime_kind.go` 只承载 backend 内部 helper。

禁止：

- 暴露 owner service facade。
- 直接 import kernel owner repository。
- 将 owner service command 类型作为跨模块契约。
- 对同步 command 缺失 `events.Response` 时默认为成功。

### `internal/pkg/workorchclient`

REST API client 包。

职责：

- HTTP client。
- REST path 构建。
- request / response 编解码。
- HTTP error 映射。
- remote API DTO。

禁止：

- 依赖 CLI/TUI。
- 依赖 `internal/services/workorch`。
- 依赖 `internal/pkg/workorchport`。
- 依赖 `framework/application`。
- 依赖 EventHub。
- 访问 kernel owner service/repository。

### `internal/pkg/workorchport`

CLI/TUI/agent 的稳定 port 契约。

职责：

- 定义 `Backend` 及各资源 backend interface。
- 定义 CLI/TUI/agent 使用的 request / info / snapshot / event 类型。
- 定义 remote backend 和 embedded backend 共同遵守的契约。
- 放置与 port 直接相关的纯 helper，例如模板默认 DoD 推导。

禁止：

- 依赖 `internal/services/workorch`。
- 依赖具体 backend implementation。
- 依赖 `internal/pkg/workorchclient`。
- 依赖 kernel owner service/repository。
- 定义 remote/backend 专属类型。
- 把 owner service command 类型作为跨模块契约。

### `internal/modules/application/workorchd`

这是 HTTP/API 侧 framework plugin module，继续保留在 `internal/modules/application`。

职责：

- `module.go` 接入 `framework/plugin/module` 生命周期。
- `biz/bootstrap` 完成 backend assembly、repository wiring、EventHub/runtime dependency export 和 application runtime construction。
- `biz/appservice` 承载应用级 use-case 编排、read model、runtime activation 和 EventHub-backed owner ports。
- `service/server` 承载 HTTP handler、REST path、request validation、response mapping、SSE。
- `pkg/observed`、`pkg/presenter` 承载观测和展示投影。

边界：

- workorchd 可以作为应用层 module 组合 owner capabilities，但正式 owner state 仍归 kernel modules。
- appservice 需要读写 owner state 时应通过 owner command/read ports 或 EventHub-backed reader，不直接访问 owner repository/service。
- server 只做协议适配，不承担状态机裁决。

### `internal/services/workorch/biz/backends` 与 `workorchd` 的关系

`biz/backends` 是 process service 内部的 backend implementation，负责把 CLI/TUI/agent 的 port 调用落到 remote HTTP 或 embedded EventHub。

`workorchd` 是 HTTP/API framework plugin module，负责服务端 API、application use-case、runtime activation 和 owner EventHub port。

两者不能互相变成对方的 service locator：

- remote backend 访问 `workorchd` 只能通过 `workorchclient` 和 REST API。
- embedded backend 访问正式状态只能通过 EventHub command/response。
- `workorchd` 不能 import `internal/services/workorch`。
- `internal/services/workorch` 不能直接 import kernel owner repository/service。
- 如必须运行期注入，只能通过 initiator 暴露基础设施或 EventHub-backed adapter，不能注入跨 owner service。

## 依赖方向

允许：

```text
application/workorch
  -> internal/services/workorch

internal/services/workorch
  -> internal/pkg/workorchport

internal/services/workorch/service/cli
  -> internal/pkg/workorchport

internal/services/workorch/service/tui
  -> internal/pkg/workorchport

internal/services/workorch/biz/agent
  -> internal/pkg/workorchport

internal/services/workorch/biz/session
  -> internal/pkg/workorchport

internal/services/workorch/biz/backends
  -> internal/pkg/workorchport
  -> internal/pkg/workorchclient
  -> internal/pkg/events

internal/pkg/workorchclient
  -> internal/pkg/common
  -> net/http

internal/pkg/workorchport
  -> internal/pkg/common
  -> stable DTO packages when required

internal/modules/application/workorchd
  -> internal/modules/kernel/* via EventHub-backed ports or explicit owner adapters
```

禁止：

```text
internal/pkg/workorchclient -> internal/services/workorch
internal/pkg/workorchclient -> internal/pkg/workorchport
internal/pkg/workorchclient -> framework/application
internal/pkg/workorchclient -> EventHub

internal/pkg/workorchport -> internal/services/workorch
internal/pkg/workorchport -> internal/pkg/workorchclient
internal/pkg/workorchport -> backend implementation

internal/services/workorch/service/cli -> internal/pkg/workorchclient
internal/services/workorch/service/tui -> internal/pkg/workorchclient
internal/services/workorch/biz/agent -> internal/pkg/workorchclient

internal/services/workorch -> kernel owner repository/service
kernel owner module -> internal/services/workorch
embedded backend -> kernel owner repository
embedded backend -> kernel owner service
```

## EventHub 交互规范

module/block 间交互必须遵循以下规则：

- 正式 owner state 的读写、状态机转换、evidence 查询、run snapshot 查询都使用同步 EventHub command/response。
- 通知类、不依赖返回结果的事件使用 `Post`。
- 需要同步结果、错误或强一致反馈的事件使用 `Send`。
- 同一 workspace/run/step 内需要顺序一致的事件必须绑定稳定 lane key。
- command topic 应按业务意图拆分，不使用万能 command。
- payload 使用稳定 DTO、owner model 或 primitive payload map，不使用 owner service command 类型作为跨 module 契约。
- owner module 在自己的 command handler 内调用自己的 service/repository，并返回 `events.Response`。
- 调用方必须检查 response。无 response、response 类型错误或 response 中包含错误都应视为失败。
- observer 只做轻量转发或投影，长耗时 continuation、resume、snapshot warm、governance continuation 应提交到 `BackgroundRoutine`。
- Teardown 后不得继续订阅事件或提交后台任务。

## 迁移阶段

### Phase 1: 抽出 REST client

动作：

- 将 `internal/modules/application/workorch/biz/client` 迁移到 `internal/pkg/workorchclient`。
- 更新 remote backend 引用。
- 保持 REST 行为不变。
- 删除旧 client 目录。

验收：

```bash
rg "internal/modules/application/workorch/biz/client" .
go test ./internal/pkg/workorchclient -count=1
```

### Phase 2: 抽出 port 契约

动作：

- 从旧 backend 包中抽出 interface、DTO 和 port helper 到 `internal/pkg/workorchport`。
- CLI/TUI/agent 改为依赖 `workorchport`。
- remote/eventhub backend 实现新 port。
- 按资源或能力把 `workorchport` 拆成 focused files，避免单文件继续膨胀。

验收：

```bash
rg "internal/modules/application/workorch/biz/backend" internal/services application internal/pkg -g '*.go'
go test ./internal/pkg/workorchport -count=1
rg "internal/services/workorch|internal/pkg/workorchclient" internal/pkg/workorchport -g '*.go'
```

### Phase 3: 迁移 process service

动作：

- 新建 `internal/services/workorch`。
- 将 process service、CLI/TUI、agent、backend implementations 迁移过去。
- `application/workorch/main.go` 改为引用 `internal/services/workorch`。
- 删除 `internal/modules/application/workorch`。

验收：

```bash
rg "internal/modules/application/workorch(?!d)" -n application internal scripts Makefile README.md docs --pcre2
find internal/modules/application -maxdepth 2 -type d | sort
go test ./internal/services/workorch/... ./application/workorch -count=1
```

### Phase 4: service 职责瘦身

动作：

- 将 lifecycle shell 拆为 `service.go`、`runtime.go`、`options.go`、`config.go`、`logging.go`。
- 将默认 workspace 解析/创建逻辑迁移到 `biz/session`。
- 将 CLI/TUI 共享 agent 编排保留在 `biz/agent`。
- 确保 `Startup/Run/Shutdown` 只做生命周期编排。

验收：

```bash
go test ./internal/services/workorch/... -count=1
rg "ResolveDefaultWorkspace|DefaultWorkspace" internal/services/workorch -n
```

### Phase 5: EventHub 边界复核

动作：

- 全面核对 embedded backend。
- 全面核对 CLI/TUI/agent import 边界。
- 全面核对 process service 是否直接访问 owner repository/service。
- 确认同步 command 必须要求 `events.Response`。

建议扫描：

```bash
rg "kernel/.*/(biz|service|repository)" internal/services/workorch internal/pkg/workorchport internal/pkg/workorchclient -g '*.go'
rg "repository\\." internal/services/workorch -g '*.go'
rg "workorchclient" internal/services/workorch/service internal/services/workorch/biz/agent internal/services/workorch/biz/session -g '*.go'
rg "EventHub|Send|Post|Response" internal/services/workorch/biz/backends -g '*.go'
```

### Phase 6: 文档和 skill 更新

动作：

- 更新架构文档，明确 `application`、`internal/services`、`internal/modules`、`internal/pkg/*client`、`internal/pkg/*port` 的边界。
- 更新 checklist，纳入 process service、REST client、port、backend implementation 验收项。
- 更新 skill，使后续应用开发能正确创建和使用：
  - process lifecycle service
  - framework plugin module
  - initiator
  - block
  - EventHub-backed port
  - REST client
  - CLI/TUI adapter

验收：

```bash
skill-hub validate --pattern go-multi-module-dev --links
skill-hub validate --pattern go-application-event-runtime --links
skill-hub validate --pattern go-module-initiator-lifecycle --links
skill-hub status --json
```

### Phase 7: 全量测试和 e2e 回归

动作：

- 先跑受影响 Go 包。
- 再跑全量 Go 测试。
- 构建 console。
- 使用脚本自动管理 workorch 实例，执行完整 e2e。
- 失败时保留 server log、mock log 和中间 JSON 以便定位。

验收：

```bash
go test ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/... ./application/workorch -count=1
go test ./... -count=1
make console-build
make verify-managed-full
git diff --check
```

### Phase 8: 迁移后缺陷回填

动作：

- 对 `make verify-managed-full` 暴露的问题做服务端根因修复，不能只修改脚本绕过失败。
- 跨 workspace、run、ticket、artifact 等对象生成 ID 时必须包含足够 scope，避免同一 managed full run 内不同 workspace 的治理产物冲突。
- 对 e2e 中暴露的 REST URL、query string、状态等待、决策处理、run 继续执行路径补回归测试。
- 将缺陷类型回填到本设计文档、验证 checklist 和对应 skill。

验收：

```bash
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/... ./application/workorch -count=1
make verify-managed-full
```

## 完成验收清单

- `internal/modules/application/workorch` 已删除。
- `application/workorch` 只保留入口装配和启动调用。
- `application/workorch/main.go` 显式 import 本入口启用的 initiator/module/block。
- `internal/services/workorch` 存在，且只承担 process service、CLI、TUI、agent、backend implementation 和进程 session 策略。
- `internal/services/workorch` 未注册为 plugin module。
- `internal/services/workorch` 未直接访问 owner repository/service。
- `internal/pkg/workorchclient` 存在，且不依赖 service、framework application、EventHub、CLI/TUI。
- `internal/pkg/workorchport` 存在，且不依赖 service、REST client、backend implementation。
- `internal/pkg/workorchport` 已按资源或能力拆成 focused files，且没有 wrapper-only 过度拆分。
- CLI/TUI/agent 只依赖 `workorchport`。
- remote backend 只通过 `workorchclient` 访问 HTTP API。
- embedded backend 只通过 EventHub command/response 访问正式 owner state。
- 同步 EventHub command 必须检查 `events.Response`。
- kernel owner module 不反向依赖 `internal/services/workorch`。
- `internal/modules/application/workorchd` 保留为 HTTP/API framework module。
- README、架构文档、验证 checklist 和相关脚本均已同步新路径。
- 对应 skill 已刷新并通过 `skill-hub validate`。
- 全量 Go 测试、console build、managed e2e 验证通过。

## 逐项核对表

| 编号 | 核对项 | 期望结果 | 建议命令 |
| --- | --- | --- | --- |
| C01 | 旧 process service 路径 | `internal/modules/application/workorch` 无生产引用 | `rg "internal/modules/application/workorch(?!d)" -n application internal scripts Makefile README.md docs --pcre2` |
| C02 | 入口职责 | `application/workorch` 只保留入口、测试和 side-effect imports | `find application/workorch -maxdepth 2 -type f | sort` |
| C03 | process service 职责 | `internal/services/workorch` 不注册 plugin module | `rg "plugin/module|module.Register|func New\\(" internal/services/workorch -g '*.go'` |
| C04 | CLI/TUI 依赖 | CLI/TUI 不直接依赖 REST client 或 backend implementation | `rg "workorchclient|biz/backends" internal/services/workorch/service -g '*.go'` |
| C05 | agent/session 依赖 | agent/session 只通过 port 调用能力 | `rg "workorchclient|kernel/.*/(biz|service|repository)|repository\\." internal/services/workorch/biz/agent internal/services/workorch/biz/session -g '*.go'` |
| C06 | port 契约 | `workorchport` 不依赖 service/client/backend | `rg "internal/services/workorch|internal/pkg/workorchclient|biz/backends" internal/pkg/workorchport -g '*.go'` |
| C07 | client 契约 | `workorchclient` 不依赖 port/service/framework/EventHub | `rg "workorchport|internal/services/workorch|framework/application|EventHub|event\\.Hub" internal/pkg/workorchclient -g '*.go'` |
| C08 | owner state 边界 | process service 不直接访问 owner repository/service | `rg "kernel/.*/(biz|service|repository)|repository\\." internal/services/workorch -g '*.go'` |
| C09 | EventHub 同步命令 | embedded backend 对同步 command 检查 response | `rg "Send|Response|Post" internal/services/workorch/biz/backends -g '*.go'` |
| C10 | workorchd 反向依赖 | `workorchd` 不依赖 process service | `rg "internal/services/workorch" internal/modules/application/workorchd -g '*.go'` |
| C11 | REST URL | 前后端路径和 query string 正确 | `go test ./internal/pkg/workorchclient ./internal/modules/application/workorchd/service/server -count=1` |
| C12 | e2e | 自动管理 workorch 实例并完成全量回归 | `make verify-managed-full` |
| C13 | 文档/skill | 架构文档、checklist、skill 同步新边界 | `skill-hub validate --pattern go-multi-module-dev --links && skill-hub validate --pattern go-application-event-runtime --links && skill-hub validate --pattern go-module-initiator-lifecycle --links` |

## 收口记录

### 2026-06-08

已完成第一轮结构收口：

- `internal/modules/application/workorch` 已迁移到 `internal/services/workorch`，旧目录已清空。
- `internal/modules/application/workorch/biz/client` 已迁移为 `internal/pkg/workorchclient`。
- `internal/modules/application/workorch/biz/backend/backend.go` 的 port 契约已迁移为 `internal/pkg/workorchport`。
- `DefaultDoDItemsForWorkspace` / `TemplateDoDItemsForWorkspace` 已下沉到 `internal/pkg/workorchport`，供 CLI/TUI/agent 通过 port 契约复用。
- remote / embedded backend 实现已迁移到 `internal/services/workorch/biz/backends`。
- CLI/TUI/agent 已改为依赖 `internal/pkg/workorchport`，不直接依赖 backend 实现或 HTTP client。
- process service 根包已拆为 `service.go`、`runtime.go`、`options.go`、`config.go`、`logging.go`。
- 默认 workspace 解析策略已下沉到 `internal/services/workorch/biz/session`。
- `internal/pkg/workorchport/backend.go` 已拆成 `workspace.go`、`run.go`、`result_artifact.go`、`observability.go`、`definition.go`、`governance.go`、`context_registry.go`、`config_events.go` 等 focused files。
- `Makefile` 与 TUI/CLI 相关验证脚本已更新为新路径。
- `README.md` 与 framework/EventHub 边界核对清单已同步新结构。

已通过阶段验证：

```bash
go test ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/... ./application/workorch -count=1
./scripts/verify-cli-architecture.sh
./scripts/verify-tui-agent.sh --static-only
make cli-test
go test ./... -count=1
make console-build
```

最终验收结论：

- `make verify-managed-full` 暴露的服务端治理 ID / 状态流转 / e2e 回归问题已进入最终方案回填项。
- 后续逐项收口时仍需按本文核对表重复执行结构扫描、`git diff --check` 和 managed e2e。

### 2026-06-08 最终方案回填

已根据 managed e2e 暴露的问题补充最终收口要求：

- decision resolution ID 必须包含 workspace scope，避免不同 workspace 中相同 ticket/run 编号产生治理产物冲突。
- 无 workspace scope 的 decision resolve 入口必须优先解析唯一 open ticket；如果存在多个同 ID open ticket，必须返回冲突，不能误解析到其他 workspace。
- owner service 在处理治理命令时必须从 command context 绑定 workspace scope，再执行 repository get/create/update。
- e2e 脚本中的 REST query string 必须使用正确的 `?` 分隔符，不能把 query 参数拼成 path 片段。
- 上述缺陷类型已纳入 Phase 8 和逐项核对表，后续迁移必须把 managed e2e 暴露的问题回填到设计文档、验证 checklist 和 skill。

已刷新并归档 skill：

- `go-multi-module-dev` 更新到 `2.2.5`，补充 process service / port / client / EventHub scope / REST query string 验收规则。
- `go-application-event-runtime` 更新到 `1.0.8`，补充 command scope、无 scope conflict 和 REST query string 验收规则。
- `go-module-initiator-lifecycle` 更新到 `1.1.8`，补充 process service 与 plugin module 边界、运行期窄注入和 scope 验收规则。
- `workorch-core-template-boundary` 更新到 `1.0.3`，补充 WorkOrch process service、port/client、EventHub owner state 和跨 workspace scope 边界。

已通过最终验收：

```bash
make verify-managed-full
```

本次 managed full 覆盖并通过：

- fmt / build / vet / 全量 `go test ./...`
- console build
- HTTP verification
- workspace template / soul e2e
- LLM workspace execution
- incremental artifact e2e
- MCP stdio e2e
- SkillHub e2e
- auto-governance ticket/approval
- software-dev code agent
- software-dev Goal/DoD e2e
- SRD orchestration e2e
- workspace file / diff / patch
- workspace web search e2e
- weather ticket/approval mock e2e
- TUI agent acceptance
- TUI long output e2e
