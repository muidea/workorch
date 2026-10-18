# WorkOrch Web Console

Status: normative

Type: product-interface

Last Verified: 2026-08-23

## Purpose

本文记录 `web/workorch-console/src` 当前实际提供的页面、HTTP/SSE 依赖和产品边界。Web Console 是远程控制与
projection 观察面，不是第二套业务实现或 Runtime owner。

## Navigation

当前主导航由 `main.tsx` 注册：

| Entry | Page | Current responsibility |
| --- | --- | --- |
| 业务总览 | `WorkspaceBoardPage` | Workspace 聚合列表、当前运行摘要、创建入口和执行提交 |
| 工作空间 | `WorkspaceDetailPage` | 单 Workspace 的定义、Run、证据、上下文、资源和诊断详情 |
| 治理队列 | `GovernanceQueuePage` | Decision / Approval 分页、详情及单个/批量处理 |
| 运行控制 | `RuntimeControlPage` | MCP server/capability 管理、发现结果和 metrics 摘要 |
| 系统配置 | `ConfigurationPage` | 支持的 server、LLM、Web Search、Soul 与 Template 配置 |
| 服务状态 | `ServicePage` | health、ready 和 metrics |

路由采用前端 hash/navigation state；生产静态资源由 workorchd 在 `/console/**` 提供。

## Page Contracts

### Business Board

`WorkspaceBoardPage` 的主读入口是 `GET /api/v1/workorch/workspaces` 返回的聚合列表与 projection status。它不会
为了绘制每一张卡片分别读取 Goal、DoD 或 Run child pages。

概览与详情复用 Workspace 的 `authoringSummary` 展示当前创作阶段、失败类别和已记录审查数，不逐卡请求 authoring。Workspace active、创作失败和当前 Run 状态分别呈现；不能把 0 个治理 blocker 解释为创作成功，也不能把审查数量解释为全部 PASS。摘要缺失或未知时不补造草稿／完成状态，不据摘要授予确认或执行权限，具体恢复与诊断从完整创作入口核对。

用户提交执行时，页面按需读取 Workspace snapshot：

1. 无 active Run、active Run 已终态或 owner 已标记 `fresh_activation_required` 时，主操作提交 `restart`；
2. current-baseline 非终态 Run 的主操作提交 `resume`，同时提供独立的显式 `restart`；
3. Resume 只读展示 Run 已封存 selection，不读取模型目录、不更新 Workspace preference；
4. 只有显式 Restart 弹窗允许先通过 Workspace owner API 保存模型/reasoning 与 Soul，再提交 immutable intent；
5. activation body 只发送 `intent`、`requestId`、`reason`、`source`。

Web 使用共享 admission classifier：`reconciliation_requested|replacement_pending` 显示 accepted，其他五类 owner outcome
显示 rejected，并保留 exact `outcome`/`admissionReason`。HTTP `202|409` 只是同一 owner result 的 transport presentation；页面不得将
409 当作 transport failure、自行重判 progress/governance，也不得发送 caller Run priority。

Board/Detail 读取 `snapshot.runCommand` 时直接展示 projection 的 exact `state`、owner `outcome`、`admissionReason` 与
`diagnostic`；caller `requestReason` 只能独立展示，不能参与 accepted/rejected 分类。Web type 不保留重复
`disposition`、歧义 `reason`、`error`、`definitionRevision` 或 `decisionId`，accepted/rejected 时间戳在类型上互斥。
LLM usage 明细直接展示可用的 `logicalRequestId` 与 `physicalCallId`，只按 `physicalResponseMode` 展示
streaming/non-streaming，并单独展示 exact `providerSubmitted`。历史或 generic trace 缺少双层身份时显示 `-`，不得从
event ID、request bytes 或 streaming 补造；旧 `streaming` boolean 即使存在也不能成为页面判断来源。

activation form 不提供 Goal/DoD、Skill、MCP 或 Memory 的 per-run override；这些定义和策略只从 owner 当前状态编译。
Workspace 创建使用共享的 `WorkspaceSetupModal`，用户提交 Template、Goal、资源引用及可选额外验收要求，不手填正式 DoD。
工作目录可留空，解析为 `.workorch-workspaces/<workspace-id>`。持久草稿经派生、六类独立审查与用户精确确认后，才物化
正式 Goal／DoD 并由 owner 准入；创建、编译、确认均不启动 Run。FAIL／UNKNOWN／资源或模板漂移不得确认。
页面发送前仅在内存保留原创建、待保存与确认身份，不使用浏览器持久存储；业务 API 请求、JSON 与 SSE 响应禁止 HTTP 缓存。新建向导重开为空，编辑已有草稿通过列表／详情明确选择 Workspace 并读取 owner。关闭有未保存输入或未决事务时须确认，刷新后页面输入与客户端身份不恢复；已接受的服务端事务不取消，用户须核对 owner 当前状态。旧回执不得覆盖新会话表单。
恢复原保存后，创建页和详情页仅在规范化输入或模板快照变化时提交新保存；未改变输入的审查恢复直接 compile，不以 edited 标记生成新 RequestID 或消耗回执额度。
存在页面内存 pendingConfirmation 时，两页暂停保存修订和重新审查，保留原 hashes 与 expected revision，并提供原确认恢复；edited 标记和查询结果均不替换原确认身份。刷新后已开始的确认仅使用 owner 校验的 `confirmationRecovery` 参数，由用户显式恢复；缺失或身份不匹配保持阻塞，不能通过普通审查动作静默放弃确认请求。
旧浏览器恢复记录不读取、不迁移或自动清理，不能因旧记录仍存在而锁定新向导，也不能将历史记录当作当前业务状态。
创作查询跨 effect 单飞，发送前再次检查会话与查询代次；失效排队不得发出 GET，单次查询超时有界。已接受的写事务不因关闭页面而取消。
已准入结果的进入操作要求 matching Workspace、eligible 与非空 StartupInputHash，不重新编译或启动 Run。

### Workspace Detail

STEP02 的合同候选页从正式草稿详情或 Workspace 规格摘要进入，只查询 owner 已保存的精确输入。
打开、刷新、关闭重开不生成；显式生成只提交该输入和模板快照前置身份，不隐式 create／PUT／confirm／准入或启动 Run。
过期冲突后只回读并展示新版本，不能自动重编；新版本须另行显式生成。页面展示完整维度、required 要求、all_of 义务、
源承诺、逐义务声明及六审的理由／覆盖／引用／hash，不以 DECLARED 或任务受理宣称执行成功。
`ready_to_confirm` 仅表示候选可审阅，本步无确认按钮；既有可写澄清入口与此只读候选入口分离。
失败、审查阻塞、准备失败和 reserve 未决分别呈现，不提供“统一重试”。只读轮询串行且受会话／revision 门禁保护；
准备恢复的 15 秒观察窗结束不是事务取消或重发授权，仍须核对 owner 事实。

`WorkspaceDetailPage` 先读取 Workspace list/detail projection，再按当前选择加载：

- Workspace snapshot、Soul bindings、服务配置、Soul、Skill、MCP catalog 与 Memory list；
- 当前 Run snapshot、observability 与 context bundle；
- current requirement/CWI的coverage、assessment、evidence三轴状态，recoverable wait condition/deadline与execution-budget摘要；
- 按需分页的 Step、ToolCall、RuntimeCheckpoint、Result、Patch、ValidationResult 与 Artifact；
- workspace/run-scoped EventRecord 诊断列表。

页面支持创建期持久规格创作／澄清、Workspace LLM/Soul selection、execution activation 与 Workspace delete。
正式 Goal／DoD 只读展示，不提供旧 CRUD 或质量评分入口。创作编辑器展示审查、验证声明、失败请求与准入阻塞。
这些表单只组装已公开的 HTTP DTO；owner 校验、revision CAS 和状态转换全部留在服务端。

`failed` 与 `preparation_failed` 的恢复规则必须分开。已 reserve 请求失败后，同输入再次编译只重放原失败，不因刷新、重开页面或部署修复重新调用 provider。创建页和编辑器统一提示保留原诊断，真实输入／模板修订仍受既有预算；仅代码修复后的验证须由用户显式新建事务，不提示通过无意义修改输入绕过失败。失败提示不授予确认或执行权限。

前置准备失败与已 reserve 的规格请求失败分别展示。前者只展示 owner 返回的 metadata，原始原因从独立 diagnosticId 查询按需校验与下载；当前／历史身份不能混用，也不把 diagnosticId 填入 permit 查询。`preparation_failed` 允许用户修复资源或配置后显式 compile，已有持久请求仍按原身份恢复。相同原因重放不增加 owner revision；页面对已受理的重复准备只进行 15 秒有界内存观察，每次重新受理均以页面观察代次重建截止，独立截止不依赖查询成功。期限结束只说明最近一次可用 owner 状态仍为原准备失败；查询失败或挂起时不能冒充已重新核对当前状态，也无法单凭查询判断后台是否在途。不声称已完成、已中断或已取消，不自动重发 provider；迟到查询仍受原会话与 scope 栅栏限制。

若 current Run 投影为 runtime-definition clarification，详情页显示 typed gap、offending refs，并禁用普通 Decision answer 与 Resume。
当前 Web 不支持同 Run 规格修订，不能用创建期 authoring 或旧 CRUD 修改 Run seal。
详情页保留独立、显式 Restart 入口，但不得根据 Goal/DoD revision、SSE、display 字段或前端比较猜测 admission eligibility；
每次 Restart 均交由 Workspace/Run owner 判定并原样显示 typed rejection。页面不直接关闭 ticket、不把新 Definition 写入
predecessor，也不构造 replacement-resolution payload。Board 遇到 exact canonical Definition blocker 时只导航到详情，
绝不直接发送 Resume 或 Restart。

`steps`、`runtime-checkpoints`、`taskProgress`、`taskDecompositions` 和 `artifactContinuations` 是历史/诊断或事实展示数据。
Runtime `nextAction` 兼容字段已删除。TaskProgress/ContextBundle 只展示 exact blocker、owner 要求/允许动作与独立导航提示；
它们不能驱动按钮可用性、恢复 Runtime authority 或替代 formal Result/evidence。

`workspace_defined` 明确表示正式定义已封存，不把 active 草稿当作定义完成；`formal_result` 只展示当前适用 Run 的 owner 已验收终态结果。库存项中的 Result／Artifact 数量表示记录存在，不能解读为验收通过。Runtime 成功终态但正式 Result 尚未交付、错 scope 或只有历史成功结果时，里程碑不得显示完成；上下文与页面使用同一事实投影。

详情页明确把 Workspace/Run owner `state`、Workspace `activeRunId` 与 Run blocker refs 标为 canonical；Workspace 没有
blocker 字段。`governanceDisplay` 是只读 lifecycle handoff，`consistencyDiagnostic` 是只读偏差，`displayOutcome` 是非 owner
展示摘要；formal output 只读独立 `result/results`。上述 display 字段不得选择 execution intent、生成 governance mutation，
也不得触发自动 Resume/Restart/Drive。

recoverable runtime wait显示为非治理、非终态状态：页面可以展示safe reason、condition、next eligible drive/deadline和
budget guard，但不提供“继续等待”“强制重试”“清零预算”或自动Resume按钮。coverage/assessment/evidence必须逐项分栏，
`partially_covered`和`indeterminate`不能由单个进度条显示为成功。

### Governance Queue

Decision 和 Approval 页面分别使用分页 list，显式打开单项 detail，并提交：

- `POST /decisions/:ticketId/resolve` 或 `/decisions/resolve-batch`；
- `POST /approvals/:approvalId/approve|reject` 或 `/approvals/resolve-batch`。

页面可以帮助组织 resolution payload，但不得推导 Run 下一状态，也不得在 resolve 成功后自行发送 Resume。通用
Decision UI 中的历史 `replan` 选择会由服务端合同规范化；它不授权 application 创建 successor。
Decision/Approval owner受理resolution后，persisted continuation自动收敛；成功提示只要求等待projection refresh。若
canonical Run持续阻塞，界面将其标记为resolution delivery diagnostic，不得提示用户额外发送Resume或把Resume当作第二条
scheduler路径。

单项按钮、checkbox与批量提交只在Workspace snapshot中的canonical `activeRunId`、matching blocked Run exact blocker ref与
真实open Decision/pending Approval完全相交时启用；`governanceDisplay`不参与选择。提交前重新读取所有涉及Workspace并整体
preflight，任一extra/stale/conflicting target都不发送batch。跨Workspace同名resource保留composite row identity；公共batch
DTO无法消歧的重复bare ID必须fail-closed。

Run-scoped runtime-definition clarification不进入通用answer/batch-resolution可选集合；治理队列只显示其binding状态并链接到
Workspace Definition修订入口。pre-seal clarification同样不提供one-of确认，只有corrected candidate的专用admission
resolution/ack链可以关闭原ticket。

Decision Queue与Workspace Detail必须直接读取typed `definitionClarification`并展示`class/bindingState`、
`requiredAction`、gap/violation、offending refs及至少`definitionRevision/authoritySnapshotId/clarificationDispatchId/focusRef`
四项runtime identity；不得从`context`、`reason`、`decisionKind`或UI文案重建这些字段。缺少typed binding的记录不属于
当前数据结构，必须在owner/repository读边界失败，UI不得保留文本分类兼容分支。

Workspace Detail与Board同样只从canonical Workspace/Run exact blocker和concrete definition Decision识别该状态；即使
`governanceDisplay`声称存在clarification也不能改变intent。exact runtime-definition blocker存在时主操作只能是Definition
修订或详情页的显式Restart，任何已打开的stale Resume表单在提交时也必须fail-closed。普通
`blocked_on_decision|blocked_on_approval` Run同样不能由Board/Detail生成Resume；治理owner的persisted continuation自行收敛。

### Runtime Control

`RuntimeControlPage` 管理 MCP Registry 的 server/capability metadata，并按用户操作读取 tools、resources、prompts
发现结果。Server表单只接受`KEY=env://ENV_NAME`或受支持workspace placeholder形式的environment reference，不接收或回显
credential明文；Capability使用`enabled`作为唯一availability字段并展示owner provenance。页面不直接发起Runtime MCP
operation；具体执行仍由sealed dispatch进入`blocks/mcp` owner。

### Configuration

`ConfigurationPage` 读取统一 configuration projection，并仅写当前 UI 支持的字段：

- supported server configuration；
- ai-proxy LLM model/key/budget configuration；
- Web Search provider configuration；
- Soul CRUD 与 Workspace Template CRUD。

页面展示的 runtime/git/LLM-output 配置摘要不等于它支持修改全部 runtime 字段。LLM save 只更新 persisted
configuration；active owner snapshot 仍需进程重启才变化。

默认模型下拉框区分目录发现与调用资格：已声明对应 operation 但 `capabilityReady=false` 的模型保留为禁用项，
说明 owner 投影的不可用原因及缺失容量；不得因此成为执行／激活候选。目录缺失时保留已配置模型身份，允许
保留原选择进行密钥修复，但不得自动选择其他默认模型。Embedding 的 `contextWindowTokens` 表示单条输入 token
上限，是 WorkOrch 当前调用预算所需元数据，不是所有模型列表 API 的通用必填字段；Embedding-only 模型不要求
`maxOutputTokens`。只有 endpoint 声明而没有有效输入容量时，可枚举但不可启用；不按模型名猜值或绕过 owner 门禁。

### Service Status

`ServicePage` 只读取 `/healthz`、`/readyz` 与 `/metrics`。它不把 process readiness 推导为某个 Run 已完成。

## Refresh And Event Model

Board、Workspace detail、Decision 和 Approval 页面订阅 `GET /api/v1/workorch/events/stream`：

- SSE 是刷新提示，不是 owner state；
- board/detail 主要消费 workspace projection-changed notification；
- governance pages 只对相关事件重新读取分页数据；
- revision gap、重连或 retry exhaustion 触发受影响的 HTTP projection 回读；
- Workspace detail 另有显式 EventRecord 查询用于诊断。

页面必须按 workspace ID 过滤事件并对 burst 做有界 debounce。SSE payload、浏览器 cache 或当前选中 workspace
都不能成为跨 Workspace 的隐式全局状态。

## Pagination And Raw Content

Run child lists 使用服务端 `limit` / `cursorAfterId`；“加载更多”不能依赖一次全量读取。大正文只通过用户显式动作
打开：

- `GET /api/v1/workorch/artifacts/content?uri=...`；
- `GET /api/v1/workorch/runtime/payloads?ref=...`。

URI/ref 由服务端 allowlist 校验。前端不得根据 filesystem path 自行拼接下载地址，也不得把 raw payload 作为默认
页面状态或 Runtime authority。

## Form And State Boundary

所有 create/update form 统一遵守：

- 只收集输入、做最小字段校验、调用正式 API，并在成功后回读 list/detail/snapshot；
- 失败时保留输入并展示服务端错误，不在浏览器构造替代结果；
- 不复刻 appservice、DoD quality、governance 或 owner reducer 规则；
- 不直接执行 Shell、Git、File、Web、MCP 或 provider side effect；
- 不维护并行的 Workspace/Run/Decision/Approval 状态机；
- 不构造或转发已删除的 `executionContext` / `ExecutionContextPatch`；
- 不从 projection compatibility fields 创建 successor、选择 action 或接纳 evidence。

Web 与 TUI 共享同一 HTTP/projection 语义：TUI 偏 agent-first 本地交互，Web 偏多 Workspace 观察、治理和配置。

## Source And Verification

- API client：`web/workorch-console/src/api.ts`
- navigation：`web/workorch-console/src/main.tsx`
- pages：`web/workorch-console/src/pages`
- types：`web/workorch-console/src/types.ts`
- command contract：[Service Command API](service-api.md)
- read/projection contract：[Service Read API](service-read-api.md)
- acceptance：[Service Acceptance](service-acceptance.md)

变更页面调用时必须同步更新 API client/types、对应 server DTO/route test、本文件和服务验收；一次性性能截图或 dated
bundle 清单不进入 `docs/`。
