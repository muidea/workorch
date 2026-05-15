# workorch 软件开发 Code Agent 能力收口计划

## 目标

本文档用于跟踪 `workorch` 基于内置 `software-dev` workspaceTemplate、
Soul、MCP、Memory、Run、Goal、DoD、Ticket、Approval 等能力，演进为面向
软件开发任务的自治 Code Agent 的剩余需求项。

目标形态：

- 用户提交开发目标 `Goal`，并定义验收目标 `DoD`。
- workorch 自动完成任务拆解、代码修改、构建、测试、修复、验收和交付物生成。
- TUI 负责命令和数据交互，不承担后端业务分叉。
- Ticket 只用于补充必要信息或解决真实决策阻塞。
- Approval 只用于高风险操作授权。
- 用户不需要 step by step 引导 agent 执行开发流程。

## 当前基线

已具备能力：

- 内置 `software-dev` workspaceTemplate。
- `software-dev` 已绑定 6 个阶段 Soul：
  - planning
  - execution
  - decision
  - approval
  - validation
  - artifact
- 运行时已具备 `Run / Goal / DoD` 编排能力。
- LLM 决策模型已支持：
  - `executionGraph`
  - `patchRequests`
  - `shellCommands`
  - `mcpToolCalls`
  - `memoryWrites`
  - `dodAssessment`
- 后端已具备 shell、patch、MCP、Memory、Result、Artifact、Decision Ticket、
  Approval 等基础模型和运行能力。
- TUI/CLI 本地模式已要求通过 EventHub 与后端交互，避免 HTTP client 分叉和
  定时轮询。

当前收口结论：

- `software-dev` 模板、阶段 Soul、Workspace 创建期 template 初始化、Run 侧
  Soul 路由边界、TUI Goal/DoD 入口、Result/Artifact 交付证据、Patch diff
  持久化、Memory run summary、shell 安全边界、自动修复循环和风险 Approval
  治理均已完成代码收口。
- 本地 TUI/CLI 仍通过 EventHub 与内置后端交互，未引入本机 HTTP client 分叉
  或定时轮询。
- 面向 software-dev 主路径和风险治理路径的确定性验收已补齐并通过；真实外部
  代码仓/真实 LLM 场景属于环境验收项，不再阻塞当前代码收口。

## 已确认架构边界

### workspaceTemplate 生命周期

- `workspaceTemplate` 只在创建 Workspace 时生效。
- Workspace 创建完成时，系统根据 template 初始化该 Workspace 的实际配置：
  - `DefaultSoulID`
  - 阶段级 `SoulBindings`
  - 默认 Goal/DoD 建议
  - 推荐 MCP/Skill 能力边界
- Workspace 创建完成后，Run 应复用 Workspace 级 Soul 路由。
- 禁止在 Run activation 阶段根据 template 临时写入或覆盖 SoulBindings。
- 禁止把 template 设计成运行时持续依赖；template 是初始化来源，不是后续
  执行状态机。
- 例外：Run activation 可以在不修改 Workspace 持久化绑定的前提下，根据仍与
  Workspace soul routing 匹配的当前 template 刷新中立 `executionContract`
  基线，用于获取最新 `profileContracts`、`deliverableExpectations` 与
  `evidenceRequirements`。该刷新不得重新绑定 template、SoulBindings、Skill、
  MCP 或任何软件开发专属流程。

### Web、TUI、CLI 边界

- Web/HTTP 创建 Workspace 符合约束，因为 Web 是 server 对外入口。
- Web handler 只能解析请求并调用统一 appservice/domain command，不能复制
  workspace/template/soul 绑定逻辑。
- 本地 TUI/CLI 不能通过 HTTP client 调用本机后端，必须继续走 EventHub
  backend。
- TUI 必须通过 EventHub 回调感知 Web 或其他端创建、删除、更新 Workspace，
  不允许使用定时轮询保持清单一致。

### TUI template 能力边界

- TUI 必须支持创建 Workspace 时选择 template。
- TUI/CLI 首启为当前目录自动创建默认 Workspace 时使用通用治理模板
  `generic-governed`，避免全新部署首次启动直接进入软件开发模板。需要软件开发
  能力时，用户应在创建 Workspace 时显式选择 `software-dev` template，或通过
  Web 创建时选择对应 template。
- TUI 应展示当前 Workspace 的 template 初始化结果，例如 default soul 和阶段
  SoulBindings。
- TUI 不提供普通“重新绑定 workspaceTemplate”入口。
- 历史 Workspace 如需迁移到 template，应作为受控高级迁移能力单独设计：
  默认不覆盖用户已修改配置，不影响历史 Run，并需要明确迁移记录或 Approval。

## 任务状态规则

状态取值：

- `pending`：已确认需求，尚未开始。
- `in_progress`：正在开发或验证。
- `implemented`：代码已实现，尚未完成完整验收。
- `verified`：代码、文档、e2e 验证均完成。
- `blocked`：存在外部依赖或设计问题需要先确认。
- `deferred`：确认延期。

## 可复用能力

| 能力 | 当前结论 | 后续要求 |
| --- | --- | --- |
| Run / Goal / DoD | 可复用 | software-dev 路径必须继续使用，不新增并行状态机 |
| workspaceTemplate | 可复用 | 当前目录自动创建默认 Workspace 使用 `generic-governed`；需要软件开发能力时在创建 Workspace 时显式选择 `software-dev` |
| Soul | 可复用但需强化 | 补充自治开发、验收、风险边界提示 |
| MCP | 可复用 | 用于外部工具、仓库服务、测试服务等能力扩展 |
| Memory | 可复用 | 用于项目约定、历史决策、失败修复经验 |
| Ticket | 可复用 | 仅用于缺失信息或真实方案决策 |
| Approval | 可复用 | 仅用于高风险操作 |
| EventHub | 可复用 | TUI/CLI 本地交互继续基于异步事件回调 |
| Result / Artifact | 可复用但需强化 | 必须输出真实交付内容、diff、测试证据 |
| shell / patch | 可复用但需强化 | 补齐安全策略、最终 diff 和证据持久化 |

## 需求项状态

| ID | 需求项 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| DEV-01 | 固化软件开发 Code Agent 独立需求和验收范围 | 文档 | P0 | verified | 文档明确 Goal/DoD 输入、自治流程、交付物、Ticket/Approval 使用边界 |
| DEV-02 | TUI 支持开发 Goal + DoD 输入 | TUI | P0 | verified | 可在 TUI 中通过 `/dod` 定义或更新 DoD，并用该 DoD 启动开发 Run；未显式输入 DoD 时使用 Workspace template 默认 DoD |
| DEV-03 | 当前目录默认 Workspace 使用 `generic-governed` 模板 | TUI/后端 | P0 | verified | 当前目录无绑定且需要自动创建 Workspace 时传 `templateId=generic-governed`；`software-dev` 只在显式创建选择 template 时绑定；已有 Workspace 不被重新绑定 |
| DEV-04 | 强化 `software-dev` Soul 自治开发约束 | Soul/模板 | P0 | verified | Soul 明确要求自动分解、开发、测试、修复、验收；只在必要时创建 Ticket/Approval |
| DEV-05 | 补齐 shell 安全策略 | 后端 | P0 | verified | shell runtime 已具备环境白名单、超时限制和危险命令 deny 策略；破坏性命令阻断路径有测试覆盖；secret/redaction 策略仍需在后续细化 |
| DEV-06 | 补齐代码变更交付证据 | 后端/Artifact | P0 | verified | 最终 Result/Artifact 包含实际 unified diff 或可追溯 diff artifact |
| DEV-07 | 建立 build/test/lint 失败后的自动迭代修复循环 | 后端/Soul | P0 | verified | 测试失败时 agent 能基于失败证据继续修复，直到 DoD 满足或阻塞 |
| DEV-08 | 强化最终 Result/Artifact 输出 | Result/Artifact | P0 | verified | 输出改动摘要、测试命令、测试结果、失败/跳过原因、残余风险和交付物索引 |
| DEV-09 | 建立代码任务 Memory 策略 | Memory/Soul | P1 | verified | 自动记录项目约定、验证命令、历史决策、失败修复经验，并在下一次 Run 召回 |
| DEV-10 | 优化 TUI 开发任务展示 | TUI | P1 | verified | TUI 收敛展示 Goal、DoD、阶段进度、阻塞项和最终交付，不展示详细事件流 |
| DEV-11 | 增加 software-dev 专用 e2e | e2e | P0 | verified | 沙箱 repo 中自动改代码、跑测试、修复失败、生成 diff、完成 DoD 验收 |
| DEV-12 | 增加风险操作 e2e | e2e | P1 | verified | 依赖升级、删除公开接口、迁移等操作必须触发 Approval，Approval 持久化 action/context；批准/拒绝路径均可验证 |
| DEV-13 | Workspace 创建支持 template 初始化 | 后端/Web/EventHub | P0 | verified | Web、远端 backend、本地 EventHub backend 和 appservice 创建 Workspace 时均可传 `templateId`；创建完成后 Workspace 持久化 default soul 与阶段 SoulBindings |
| DEV-14 | 移除 Run activation template/SoulBindings 误接线 | 后端 | P0 | verified | TUI/agent 启动 Run 不传 template SoulBindings；Run 只使用显式 run override 或 Workspace 级路由，默认路径不做 template 绑定 |
| DEV-15 | TUI Workspace template 创建入口与只读展示 | TUI | P1 | verified | 已支持 Workspace 详情展示 default soul / SoulBindings；可通过 `/workspace create ... --template ...` 在创建时选择 template；不提供常规重新绑定入口 |

## 实施顺序记录

1. `DEV-01`：已固化开发型 Code Agent 的验收合同。
2. `DEV-13`、`DEV-14`、`DEV-03`：已收口 template 创建期绑定和 Run 侧边界。
3. `DEV-02`、`DEV-04`、`DEV-15`：已补齐 TUI 输入入口、模板选择展示和 Soul 行为约束。
4. `DEV-05`、`DEV-06`、`DEV-07`、`DEV-08`：已补齐代码交付闭环。
5. `DEV-11`、`DEV-12`：已将主路径和风险路径固化为确定性验收。
6. `DEV-09`、`DEV-10`：已完成上下文复用与 TUI 展示收敛的当前阶段要求。

## 验收分层

静态验收：

- `software-dev` 模板和 Soul 均可被默认加载。
- Workspace 创建期 template 初始化可被静态和单元测试覆盖。
- Run activation 默认路径没有 template/SoulBindings 初始化逻辑。
- TUI/CLI 不引入本地 HTTP client 分叉。
- TUI/CLI 不引入定时轮询刷新。
- shell 安全规则具备单元测试覆盖。

单元/集成验收：

- Goal + DoD 创建、更新、激活路径可测。
- `software-dev` 在 Workspace 创建期完成 default soul 和阶段绑定可测。
- shell、patch、Result、Artifact、Memory 写入均可测。
- Ticket/Approval 只在明确阻塞或风险场景产生。
- Approval 必须保留高风险操作的结构化 `action` 与 `context`，并在恢复 Run 时
  传回后续 Step，保证批准的具体风险对象可追溯。

端到端验收：

- 给定一个带失败测试的沙箱仓库，提交 Goal 和 DoD 后，workorch 能自动：
  - 读取项目上下文；
  - 制定实现计划；
  - 修改代码；
  - 运行测试；
  - 根据失败继续修复；
  - 生成最终 diff；
  - 输出测试报告；
  - 通过 DoD assessment；
  - 创建 accepted Result 和 Artifact。

## 已确认问题

| ID | 问题 | 默认建议 |
| --- | --- | --- |
| Q1 | TUI 中 DoD 输入采用命令还是专用编辑区？ | 先支持 `/dod` 命令和模板默认 DoD，后续再做专用编辑区 |
| Q2 | `software-dev` 是否应成为代码仓目录默认模板？ | 否。全新部署首次启动默认使用 `generic-governed` 通用治理模板；代码任务需要通过显式创建 Workspace 选择 `software-dev` |
| Q3 | lint 命令如何确定？ | 优先使用项目脚本或 Makefile；不存在时跳过并在 Result 中说明 |
| Q4 | 是否允许 agent 自动安装依赖？ | 默认需要 Approval，除非项目已有明确本地缓存或 CI 脚本约定 |
| Q5 | 最终 diff 是否必须写入 Artifact？ | 是，作为 software-dev 任务的核心交付证据 |
| Q6 | TUI 是否提供 workspaceTemplate 重新绑定？ | 否。只提供创建时选择和只读展示；历史迁移作为高级能力另行设计 |
| Q7 | Web 创建 Workspace 是否符合约束？ | 是。Web 是外部入口，但必须调用统一后端能力；本地 TUI/CLI 仍走 EventHub |

## 跟踪记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-03 | 初始创建。基于当前 `software-dev` 模板、Soul、Run/Goal/DoD、Ticket/Approval、Memory、Result/Artifact 能力核对结果形成任务清单。 |
| 2026-05-03 | 补充确认：workspaceTemplate 只在 Workspace 创建期绑定；Web 创建 Workspace 符合约束但必须复用统一 appservice；TUI 不提供常规 template 重新绑定入口；本地 TUI/CLI 继续使用 EventHub 且禁止轮询。 |
| 2026-05-03 | 已实现 Workspace 创建期 `templateId` 传递和绑定：appservice、本地 EventHub backend、远端 backend、Web handler、CLI workspace create 均接入；Run activation 默认路径不再携带 template SoulBindings；TUI 增加 `/dod` 命令并在提交 Goal 时使用用户 DoD 或 Workspace template 默认 DoD；workspace-template CRUD 已从 no-op 收口为持久化 appservice/client 调用。 |
| 2026-05-04 | 调整首启默认 Workspace 策略：全新部署首次启动绑定当前目录并使用 `generic-governed`，不隐式选择 `software-dev`；软件开发模板仍通过显式 Workspace 创建时的 `templateId` 绑定。 |
| 2026-05-03 | shell runtime 增加危险命令阻断策略：拒绝 sudo/su/shutdown/reboot/mkfs/mount 等系统级命令，拒绝 `rm -rf` 针对根目录、`.git`、当前目录或 workspace 外绝对路径，拒绝 `chmod 777` 和 `git push --force`；新增测试覆盖阻断路径。 |
| 2026-05-04 | TUI 新增 `/workspace create <title> --template <templateId> [--dir <path>]` 创建路径，并在主视图中展示当前 Workspace 的 default soul 与 SoulBindings 摘要；创建后会切换到新 Workspace 并刷新清单。 |
| 2026-05-04 | 强化内置 `software-dev` 六个阶段 Soul：明确自治开发闭环、自动修复、DoD 证据、Ticket/Approval 使用边界和最终交付物要求。 |
| 2026-05-04 | 交付证据与结果收口：最终 Result 现在包含交付物索引与默认风险/后续动作，Patch 持久化 diff/content，新增 Code Change Diff artifact，completion memory 自动写入 run summary。 |
| 2026-05-04 | 增加软件开发闭环集成测试：覆盖缺陷实现自动修复、unified diff 应用、测试恢复、Result/Artifact 输出与 run summary memory 生成。 |
| 2026-05-04 | 风险操作治理收口：Approval 模型、仓储、事件、appservice、backend、TUI/agent 均贯通 `action/context`；LLM `requiredApproval` 输出会映射为 `blockApprovalAction/blockApprovalContext`，orchestrator 创建 Approval 时持久化风险元数据，批准后将同一元数据写入 `approvalResolution` 传递给后续 Step；批准和拒绝路径均增加测试覆盖。 |
| 2026-05-04 | `scripts/verify-llm-workspace-execution.sh` 补充 Approval `action/context` 断言，覆盖直接 Approval reject 和 Decision 后 Approval approve 两条 e2e 路径。 |
| 2026-05-04 | 最新部署服务验证通过：真实 LLM executionGraph + shell + DoD assessment 闭环、mock LLM 工作区 e2e、software-dev code agent 收口脚本均通过。 |

## 当前验证记录

已通过：

```bash
GOCACHE=/tmp/workorch-go-build go test ./application/workorch ./internal/modules/application/workorch/biz/backend ./internal/modules/application/workorch/service/tui ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1
./scripts/verify-cli-architecture.sh
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/blocks/shell/biz -count=1
GOCACHE=/tmp/workorch-go-build go test ./application/workorch ./internal/modules/application/workorch/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/blocks/shell/... ./internal/modules/kernel/orchestrator/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorchd/biz/bootstrap -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff|TestRuntimeOrchestrator_ReusesValidatedDoDAssessmentForCompletionEvidence|TestRuntimeOrchestrator_ResultArtifactsIncludePatchDiff|TestRuntimeOrchestrator_CompletionEvidenceWritesResultAndSkillFeedbackConcurrently' -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/kernel/orchestrator/... ./internal/pkg/repository/... -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui -count=1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/blocks/llm/... ./internal/modules/blocks/approval/... ./internal/pkg/events ./internal/pkg/repository/... ./internal/modules/kernel/orchestrator/... ./internal/modules/application/workorch/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/pkg/observed/... -count=1
WORKORCH_LLM_VERIFY_BASE_URL=http://127.0.0.1:18080 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh
GOCACHE=/tmp/workorch-go-build go test ./application/workorch ./internal/modules/application/workorch/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/blocks/shell/... ./internal/modules/blocks/llm/... ./internal/modules/blocks/approval/... ./internal/modules/kernel/orchestrator/... ./internal/pkg/events ./internal/pkg/repository/... -count=1
./scripts/verify-cli-architecture.sh
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build go test ./... -count=1
GOCACHE=/tmp/workorch-go-build make verify-software-dev-code-agent
WORKORCH_REAL_LLM_BASE_URL=http://127.0.0.1:8080 ./scripts/verify-real-llm-workspace-execution.sh
WORKORCH_LLM_VERIFY_BASE_URL=http://127.0.0.1:8080 WORKORCH_LLM_VERIFY_MOCK_ADDR=127.0.0.1:18181 ./scripts/verify-llm-workspace-execution.sh
```

最新部署环境验收：

- 已部署服务健康检查通过：
  - `GET /healthz`
  - `GET /readyz`
- 真实 LLM e2e 已通过：
  - provider：`local`
  - workspace：`ws-22d69a03ed899f23`
  - run：`run-99d5c8111f2eed52`
  - run state：`completed`
  - steps：`3`
  - toolCalls：`3`
  - validations：`3`
  - runtimeCheckpoints：`9`
  - results：`1`
  - artifacts：`2`
- mock LLM 工作区 e2e 已通过，覆盖部署服务上的 Decision/Approval/Result/Artifact
  主路径和风险治理路径。
- software-dev code agent 收口验收脚本已通过，覆盖软件开发主路径、Approval
  action/context 契约、TUI/CLI 架构守卫和 TUI 接受测试。

仍建议上线前按目标部署环境补充一次真实外部代码仓 e2e；该项依赖指定样例仓库或
私有仓库授权，不影响当前代码收口结论。

新增可复用的收口验收入口：

```bash
GOCACHE=/tmp/workorch-go-build make verify-software-dev-code-agent
```
