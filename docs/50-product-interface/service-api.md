# WorkOrch Service Command API

Status: normative

HTTP/Web access uses the versioned [Local Operator Access](local-operator-access.md) boundary: loopback listener and peer,
local Host, same-origin browser requests and no proxy-provided identity. It does not substitute for exact confirmation,
bounded grants or Decision approval; non-loopback deployment requires a separately approved identity contract.

Type: product-interface

Last Verified: 2026-08-26

## Purpose And Authority

本文件是 HTTP/SSE surface 的稳定入口，并只展开 mutation/command contract。GET、SSE、pagination、raw-content 与
read-model 规则见 [Service Read API](service-read-api.md)。实际 route registration、DTO 和 handler tests 是协议实现事实源；
owner 状态机仍由 owner component contract 定义。

所有业务路径以 `/api/v1/workorch` 为前缀。HTTP payload 必须先转换为接收 owner `pkg/events` 的 concrete typed
command/result；handler 不得把 JSON DTO、display text 或 projection 当作 Runtime authority。

Read-side `displayOutcome`、`governanceDisplay` 与 `consistencyDiagnostic` 永远不是 command input。HTTP/Web/TUI 不能用它们
选择或构造 Drive/Resume/Restart，也不能从治理 display ID 伪造 resolution target；mutation 必须来自用户显式 intent 与
对应 owner resource/canonical state，最终 admission 仍由 owner 裁决。

## Workspace Execution Mutation

唯一 workspace execution 写入口是：

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
```

```json
{
  "intent": "resume",
  "requestId": "optional-request-id",
  "reason": "optional-reason",
  "source": "optional-source"
}
```

- `intent` 空值规范化为 `resume`，只允许 `resume|restart`；
- Goal-only authoring 的 Restart 只复用 exact confirmed、eligible 的 owner candidate：typed 查询绑定同一 Workspace revision，正式 Goal／DoD、模板、目录与 Definition policy 必须保持封存身份。首次启动保存默认 provider selection 不使原 RequestID 重放失效；它不授权替换已确认规格或重新调用创作 provider。
- execution admission 不接受 body/query/header priority；Run 顺序与 owner activation identity 不由 caller priority 控制；
- caller 不能注入 RunID、Goal/DoD、provider/Soul/Skill/MCP/Memory selection、Runtime context、target state 或 successor identity；
- fresh Start/Restart 必须绑定 exact `WorkspaceActivationAdmission=eligible`；pending 返回
  `governance_required/definition_clarification_pending`，missing/mismatch 分别返回
  `fresh_activation_required/definition_admission_missing|definition_admission_mismatch`，三者均零 Run/replacement intent；
- Resume 只复用 existing Run 的 original sealed startup input，不吸收 newer candidate/revision；
- 新 Restart 在任何 Run forward 前由 Workspace 私有 CAS 持久登记完整 typed payload 身份及 preparation epoch；
  只有首次 CAS 明确成功的调用可转发一次。同键异完整 payload 冲突；丢回复、unknown、重启或空库存不清 pending、不重发。
  公开 POST 在重新编译当前 bootstrap 前先对账原登记；原完成结果仍按原身份返回，不受当前规格漂移影响。
  未决项阻塞新请求并返回 `fresh_intent_unresolved/fresh_intent_result_unknown`、`409`、`accepted=false`、`state=pending`；
  `requestId` 保持本次调用身份，`pendingRequestId/pendingStartupInputHash/pendingPayloadHash` 保留阻塞原身份。
  这不声明确定拒绝、没有 Run 或本次没有转发。只有独立 Run owner 的精确持久原回执可结算。
  缺创建期 preparation fence 的 Workspace 新 Restart 返回 `fresh_activation_required/preparation_creation_fence_missing`，
  不自动迁移或补标记；Resume 维持原义，但不得复用已登记 fresh RequestID。此合同不签发 M3 或增加执行授权。
- Restart 使用 fresh eligible bootstrap；runtime clarification predecessor 要求 Definition revision 与 StartupInputHash 均变化，
  任一未变化返回 `governance_required/definition_clarification_pending` 且零写入；通过后 Run owner先 seal predecessor
  `ReplacementIntent`，terminal delivery 完成后才创建 successor；
- handler 同步等待并校验 exact Workspace owner result。只有 `reconciliation_requested|replacement_pending` 返回
  `202`；`restart_required|fresh_activation_required|definition_changed_requires_restart|governance_required`
  返回 typed `409`。两类响应都原样携带 `workspaceId/runId/requestId/intent/outcome/reason`，不存在
  application-local queue/queued acknowledgement；`requestReason` 与 owner-authored `reason` 分离。

响应中的 `accepted`、`state=accepted|rejected|pending`、HTTP status 和 display diagnostic 只由共享 pure admission classifier
从已校验 owner outcome 生成。HTTP、projection、client、Web/TUI 不得各自重判 outcome；projection 还保留 exact
`outcome/admissionReason`，SSE 只发布 immutable projection invalidation。

以下 mutation 永久不属于当前 API：run-addressed create/start/resume、`execution-restarts`、
`ExecutionContextPatch`、caller-selected target state 或 successor。

## Mutation Inventory

### Configuration

| Method | Path | Command semantics |
| --- | --- | --- |
| `PUT` | `/configuration/server` | update supported persisted server fields |
| `PUT` | `/configuration/runtime` | update Runtime configuration source |
| `PUT` | `/configuration/llm` | update persisted ai-proxy selection/key/budget for next active revision |
| `POST` | `/configuration/llm/models/refresh` | request LLM-owner catalog refresh when revision permits |
| `PUT` | `/configuration/web-search` | update Web-search provider configuration |

LLM configuration save does not mutate the running LLM owner snapshot or any sealed Run selection；read side exposes
`restartRequired` when persisted and active revisions differ or the active observation is unavailable。
`llm.defaultChat/defaultEmbedding` 是保存的选择，`llm.active.defaultChat/defaultEmbedding` 仅来自 LLM owner 当前进程观察，
不携带 API key。`active` 缺失表示未知，不得回填保存值；存在且无 `defaultEmbedding` 表示当前未配置。

### Workspace And Definition

| Method | Path | Command semantics |
| --- | --- | --- |
| `POST` | `/workspace-authoring` | 持久创建 Goal-only 创作草稿，按 RequestID 与原 payload 重放 |
| `PUT` | `/workspaces/:workspaceId/authoring` | 保存创建期输入修订，绑定 expected revision／inputHash |
| `POST` | `/workspaces/:workspaceId/authoring/compile` | 必填 `expectedInputHash`、`expectedTemplateSnapshotHash`；托管派生与六类独立审查；202 仅为任务接受回执 |
| `POST` | `/workspaces/:workspaceId/authoring/confirm` | 精确确认 input／permit／proposal／specification hashes，物化并准入 |
| `DELETE` | `/workspaces/:workspaceId` | submit one Workspace-owner delete intent; optional typed purge mode |
| `PUT` / `DELETE` | `/workspaces/:workspaceId/llm-selection` | owner-CAS update/clear selection |
| `PUT` | `/workspaces/:workspaceId/soul-bindings` | update owner-managed Soul routing |
| `POST` | `/workspaces/:workspaceId/template-binding-refresh` | explicitly refresh template-bound policy; does not mutate a sealed Run |
| `POST` | `/workspaces/:workspaceId/execution-activations` | submit `resume|restart` intent |

创作输入为 RequestID、名称、Template、Goal、资源引用及可选用户验收要求；工作目录可留空，解析为
`.workorch-workspaces/<workspace-id>`。正式 Goal／DoD 仅在精确确认事务中物化，随后由 owner 准入；创建、编译、确认均不启动 Run。
普通 setup 不要求 raw context/evidence/target/recipe/check/metric/artifact JSON。

Web 的独立“保存草稿”仅调用创建接口，停在 `authoring.state=draft`、`inputRevision=1`；不调用修订、编译、确认或激活。
compile 前置身份来自同一 authoring GET 的 `inputHash` 与 `templateSnapshotHash`，不是会因 ledger 写入而变化的 Workspace revision。
缺字段返回 400，不允许降级为读取最新输入；输入／模板快照过期返回 409，必须在任务受理、reserve／provider 前拒绝。
延迟后台任务仍反查相同输入及模板，后续 reserve 受 owner CAS 与精确绑定保护；冲突后只读回读，不能隐式生成另一版本。
查询的 `proposal` 返回 canonical `version`、`goalItemRef`、`goalTextHash`、完整 `dimensions`／`requirements`／`obligations`，
保留 required、来源、资源引用与逐义务 all_of 关系；`verifications` 是声明而非执行证据，review 的 `specificationHash` 仍绑定 proposalHash。
生成提示与 provider schema 要求每项 `Verification.Declaration` 有非空、非纯空白的具体说明（至多4096 UTF-8 bytes）。
`DECLARED` 精确引用适用且合并后仍有贡献的 Profile 能力，并说明对原义务的验证方式；未执行不等于能力缺失。
无适用能力或依据不足时保留 `UNVERIFIED`，说明具体缺口且三个 Profile 绑定字段为空；六审 PASS 也不能消除该阻塞。
schema 是生成侧约束，不改变既有 decoder／owner 的历史失败重判语义及首错顺序；空声明不得取得可确认资格。
decoder／owner 不自动补写说明、制造绑定或改写原始模型材料。
独立审查请求的 `reviewEvidenceRefs` 由 owner 从同一合法、未评审候选投影，仅含原 Goal／候选 ref 和规格对象 ID。
审查 `EvidenceRefs` 只能逐字引用该集合；输入、模板、资源及 proposal 的 hash、Profile 身份不因此成为新的审查引用。
`requiredRequirementIDs` 仍独立规定每类必审范围；提供引用集合不代替语义审查，不授予运行证据或确认资格。
六审请求同时明确既有独立 exact Goal requirement 结构：Goal 不是候选 DoD，不补造其 DoD obligation 或逐义务验证；
每类仍审完整 Goal 的语义是否被候选义务及适用声明承接，保留原文不等于覆盖充分，实际遗漏／弱化／依据不足仍阻塞。
派生与规格请求同时公开原 request／permit、provider binding 和资源快照 hash；这些是只读来源，不产生确认或执行权威。
派生须先从 Goal／资源提取尚未被具体承接的原验收主项，再补充边界条件；完整 Goal 和通用模板不能替代逐项覆盖。
proposal／review 动态输入的 `sourceApplicableVerificationDeclarations` 从封存 Profile 经相同来源匹配及合并保留规则纯计算，
逐候选给出声明身份选项。它不证明语义可验、资源就绪或运行成功，不自动填写 `DECLARED`；六审及 owner 校验保持独立。

201 表示 owner 已保存、允许的目录准备已完成并经过独立读回；`Workspace.state=active` 不代表合同或执行就绪。
authoring GET 公开 `title`、`description`、`deliverableDirectory` 与 `creation`（`requestId`、`inputHash`、`directoryPrepared`）。
后者是原创建阶段历史事实，不是当前目录／资源 readiness。未确认完成时 GET 不补目录；仅当 owner 能精确重建原创建 hash
时提供 `creation.replay`，可显式重放相同 POST；否则保持待核对，不能用新 requestId 盲目创建。
目录准备失败仍可能已保存；503 的 `creationRecovery` 保留原身份、`persisted|unknown` 和失败阶段，不表示从未保存。
创建 command 一旦投递 owner，`ErrInvalidArgument`／`ErrNotFound` 也可能来自持久化后的 bindings/index 处理，
不能据错误类别声明未保存。只有创建入口在投递前拒绝且返回 `workspace_creation_rejected_before_dispatch` 才表示
本次未提交 owner；Web 仅在首次请求收到该明确 400 标记时释放创建身份。普通 400/404、缺失恢复字段或先前已 unknown
均保留原 requestId 与 payload，等待核对／原身份恢复，不生成替代身份。
刷新／关闭后从服务端列表发现草稿，不依赖浏览器持久化；列表投影暂未收敛时显示其真实状态，再刷新核对 owner。

解析失败诊断随原规格请求由 Workspace owner 持久记录。普通 authoring GET 仅返回错误分类及诊断元信息；
`GET /workspaces/:workspaceId/authoring/diagnostics/:permitId` 按精确 Workspace/permit scope 返回单条原始错误与响应的可逆编码、
字节数和 SHA256。读取不调用 provider、不推进确认、不启动 Run。诊断不脱敏、不替换、不静默截断；原文超限时明确
标记未保存，已知失败仍为 failed。旧失败无诊断时不得回填猜测内容。文件私有权限和部署访问边界保护原始材料。
Web 不因元数据未含原文而报完整性错误；按需读取后核对精确 scope、阶段、失败码、全部保存／观测元数据及真实 bytes／SHA256。
Web 只作纯文本展示，原文永不作为准入或执行权威。详细边界见
[Workspace 规格创作](../40-profiles-capabilities/workspace/setup-draft-session.md)。

成功提案原文独立使用 `GET /workspaces/:workspaceId/authoring/success-byte-audits/:permitId` 受限读取。Application 经 concrete typed EventHub 查询 Workspace owner；返回该当前已完成提案的 canonical hash、document／wire 可逆编码及 observed／complete／saved／hash／bytes／reason。普通创作 DTO 只携 metadata，不复用失败诊断、不接受跨 scope；重复及重开读取均不写 owner、不调用 provider、不创建 Run，响应禁止缓存，沿用管理接口部署访问边界。

原始前置失败与 provider permit 诊断分别管理。资源／配置／请求准备的已知失败使用 Workspace owner 的 `PreparationDiagnostics`；常规 GET 只返回有界 metadata 与 current／history 标记，`GET /workspaces/:workspaceId/authoring/preparation-diagnostics/:diagnosticId` 返回原始原因的可逆编码、字节数与 SHA256。此身份不是 permit，不授权 provider 调用。当前目标尚未 reserve 时可显示 `preparation_failed`，允许用户修复问题后显式 compile；目标已有持久请求或确认后，以其 ledger 为准，旧诊断仅历史。未知 reserve 结果不能冒充前置失败，保存失败也不能冒充可查询诊断。

旧 `POST /workspaces`、Goal／DoD POST／PUT、`goal-dod-quality-check` 和内存 draft session 写路由均已退休，返回 `404`。
正式定义 GET 保留作读取；同 Run 规格修订不通过旧 CRUD 暴露。
Go client 只提供正式 authoring 写方法，退休创建／Goal／DoD 写方法不再存在。
TUI 的旧创建、目标提交及 DoD 保存命令明确拒绝，不调用 backend 写操作、不生成默认 DoD，也不自动 Restart；
当前 TUI 未提供完整创作向导，帮助、空态和补全不推荐退休写命令；用户通过 Web 完成规格创作后，可在 TUI 读取并显式提交执行。
CLI 不注册 `workspace create`；`workspace create`、`ws create`（含大小写变体与旧 flags）在参数解析与未知命令 TUI fallback 前直接返回中文 Web 提示，不访问 backend、不回退为列表或选择。Goal-first `agent [goal]` 保持隐藏，并在访问 backend 或订阅事件之前直接拒绝；agent 不生成默认 DoD 或自动提交 Restart。Workspace 创建仅通过 Web 完成，终端保留已有 Workspace 的读取、选择、执行与观察能力，不提供创建向导。
终端 `WorkspaceBackend` 不暴露 `Create`；local EventHub 与 remote backend 无对应实现或创建回执 decoder。退休 Goal／DoD 写接口仍在发送前返回冲突提示；五个退休 CLI 定义写 topic 不注册为 production facade 或 dispatcher 订阅，handler 的直接防御分支仍拒绝，不能绕过 backend 调用旧创建或 Goal／DoD 写用例。Workspace owner 内部 typed command 保留供受控事务与测试夹具使用。

Workspace delete 默认为 retain：完成后隐藏 Workspace 及各 owner 的 workspace-scoped 资源，保留物理
记录与永久 fence。显式 purge 仅请求每个 owner 幂等清理自身数据。Application 不执行
跨 owner saga；它只向 Workspace 发送一个带 stable `requestId`/optional expected revision 的 command。初始
`activeRunId` 非空时零 intent 写入并返回 `409`；已接纳流程在部分失败、响应丢失或重启后由
Workspace owner 从 durable deletion aggregate 幂等重驱，exact retry 不创建第二个删除链。

确认事务向 Workspace owner 提交 immutable `DefinitionAdmissionCandidate` 并执行 pure admission。只有 `admitted_all_of`
可以 promotion；`definition_one_of_unsupported` 或 declared ambiguity 返回 typed `BlockingTicketID`。Corrected
candidate 仍需 Decision resolution 与 Workspace acknowledgement 才能变为 eligible。

candidate 还必须封存 exact normalized typed Profile/policy snapshot、`RuntimeProfileContractsHash`、required owner-fact bindings
与唯一 one-shot acquisition/reobserve bindings。每个 Profile-required acceptance/deliverable source 缺 exact proof declaration、
受支持 evidence kind 或 owner-receipt-capable operation 时，返回
`definition_profile_contract_invalid + select_or_fix_template_profile + BlockingTicketID`；Profile-derived owner-fact closure
缺失/重复/悬空/成环，或 candidate manifest 与 snapshot 派生值不一致时，返回
`definition_profile_prerequisite_unresolved + select_or_fix_template_profile + BlockingTicketID`。两者都在
promotion/Run/provider/dispatch/effect 前停止。candidate-owned relation/ref closure failure 仍使用
`declared_prerequisite_unresolved + rewrite_definition_all_of`。HTTP/Application 不得查询 live catalog、使用 wildcard evidence、
接受 caller 提供的 acquisition action，或仅凭 hash 代替该静态合同。
Template 与 Workspace 的 `EvidencePolicies` 分别形成 candidate-bound `AdmissionPolicyBindings`；任一 snapshot 中出现退役
`prospectiveTargetRef` 时，API 在 promotion/Run 前同步拒绝，不能将其解释为 Runtime target。双方
policy snapshot ref/revision/hash 均进入 immutable candidate hash，后续 live policy 漂移不能修改已封存 admission。

必须区分两类 Definition clarification：

- pre-seal ticket由新candidate的all-of admission、专用Decision resolution和Workspace acknowledgement收口；不存在
  “接受one-of后继续”的payload；
- Run-scoped runtime-definition ticket不提供公共continue/fail resolution。当前公共 API 不提供同 Run 规格修订入口；
  显式 `restart` 仍由 owner 校验 fresh eligible bootstrap。内部 replacement 合同中，Run owner在持久化matching `ReplacementIntent`后，通过
  `ResolveRuntimeDefinitionClarificationByReplacementCommand/Result`与Decision owner收口。HTTP handler、Web/TUI和
  workorchd均不得代发该owner-to-owner command或伪造replacement binding。

不存在独立的`/runtime-definition-clarifications/:id/continue`、直接ticket-close或Definition patch route。
创建期 authoring 确认、promotion 和 execution activation 分别经过各自 owner 合同；旧 Goal／DoD 写路由不再公开。
确认前派生和审查允许按持久 permit 调用 provider，但不启动 Run 或执行其 operation。

### Governance

| Method | Path | Command semantics |
| --- | --- | --- |
| `POST` | `/decisions/:ticketId/resolve` | submit typed resolution for one Decision ticket |
| `POST` | `/decisions/resolve-batch` | resolve exact ticket IDs with one typed resolution |
| `POST` | `/approvals/:approvalId/approve` | submit approval fact |
| `POST` | `/approvals/:approvalId/reject` | submit rejection fact |
| `POST` | `/approvals/resolve-batch` | approve/reject exact approval IDs |

Resolution只提交对应 owner fact并刷新 projection。Application 不发送 Resume；ordinary Runtime governance 经
Orchestrator typed resolution收口。通用Decision resolve route必须拒绝pre-seal与runtime两类Definition clarification ticket；
前者只由corrected admission专用合同收口，后者只由Run owner发起的dedicated replacement-resolution contract收口。

对新的状态转换，单条与批量route在任何owner write之前都必须用typed EventHub owner read完成同一admission：concrete
Decision=open或Approval=pending、Workspace owner的`activeRunId`、Run owner的matching blocked state与exact blocker ref必须
完全相交；同Run额外资源、stale scope、缺失resource或冲突blocker返回typed `409`，批量请求完整preflight且零部分写入。
这不是第二套状态机：已terminal的exact response-loss retry仍转交Decision/Approval owner，由owner按完整resolution
type/payload或action/principal/context做幂等readback；Workorchd不得重判或吞掉该重试。

### Resource Registries

| Resource | Mutation routes |
| --- | --- |
| Soul | `POST /souls`; `PUT, DELETE /souls/:soulId` |
| Workspace Template | `POST /workspace-templates`; `PUT, DELETE /workspace-templates/:templateId` |
| Builtin Template/Soul | `POST /builtin-defaults/upgrades` |
| Skill | `POST /skills` |
| MCP server | `POST /mcp/servers`; `PUT /mcp/servers/:serverId` |
| MCP capability | `POST /mcp/capabilities`; `PUT /mcp/capabilities/:capabilityId` |
| Memory | `POST /workspaces/:workspaceId/memory`; `PUT, DELETE /workspaces/:workspaceId/memory/:memoryId` |

Registry DTO 只表达 declarative owner resource。MCP Registry owns metadata；MCP capability Block owns execution。
`validationProofApplicability=direct|auxiliary|not_declared` 是 Registry declaration，technical readiness 不是 API 可写字段。
Server DTO只接受`environmentRefs`而不接受environment value；supported ref由MCP execution最后解析，plaintext credential在
persistence/EventHub前拒绝。Capability以`enabled`作为唯一availability字段；owner-issued
`source/sourceRef/revision`只读provenance不能作为mutation authority。

Builtin upgrade body 如 `{"kind":"soul","id":"...","expectedContentHash":"...","targetContentHash":"..."}`，kind 仅允许 `soul|workspace_template`。
先从 `GET /builtin-defaults` 取得 preview；owner 写入时重新比较，只有 exact reviewed baseline 可升级。成功返回
`{default: <current owner observation>, upgraded: true|false}`；已到同一目标的响应丢失重试为 `false`。
定制/未知内容、stale expected hash 或 changed target 返回 `409`，缺资源为 `404`，无 force 或 batch-atomic promise。
此操作不刷新 Workspace、不创建 eligible admission、不触发 Run activation；详见
[Built-In Default Resources](../40-profiles-capabilities/workspace/builtin-default-resources.md#explicit-guarded-upgrade)。
新 Start/Restart 读取当前 Template/Soul，但不能自动替换 Goal-only authoring 的确认身份：模板或 Definition policy 漂移时阻塞，须经受支持的显式创作修订、六审查与重新确认后才能使用新准入。其他受控 owner 定义路径仍按当前配置重新准入；Resume/recovery 与旧归档使用原 Run seal。服务启动时仅由资源 owner 自动更新 exact reviewed 未定制的内置版本，未知/定制内容不覆盖；catalog 升级本身不重新确认 Workspace。

### 持久规格创作

创作查询为 `GET /workspaces/:workspaceId/authoring`；结果以 Workspace 持久 ledger 为准，不以内存会话作为权威。
派生、提案及六 critic 分别先持久 reserve 再调用 provider；未决请求不得自动重发。FAIL／UNKNOWN 保持阻塞。
同输入的 `failed` 编译返回原失败（HTTP 200），不是新的 202 受理或 provider 重试；刷新、重启与 renderer 升级不改变该规则。真实输入／模板修订使用既有保存合同，保留历史失败与累计预算，不得通过随机 RequestID 或无意义修订清除失败。
确认必须绑定当前输入及完整审查身份。页面编辑不是 owner 草稿或准入凭据。Web 不使用浏览器持久存储保存输入、读模型或恢复身份；未保存编辑及客户端请求身份仅存在于当前页面内存，关闭／刷新后不恢复。已保存草稿须从 Workspace 列表／详情显式打开并读取 owner。

保存请求必须显式提供非空 `templateId`，不得用现行记录补齐原请求身份。保存响应丢失时，当前页面内存保留原 pendingSave 的 `requestId`、模板、输入、expected revision 与 inputHash，显式重试优先重放原请求；刷新后先读取 owner 当前草稿，不自动发送替代保存。
Workspace owner 在同一 CAS 中持久有界保存回执（最多 128 条，不淘汰迟到重放身份）；相同 RequestID 的不同 payload 返回 `409`。
原请求精确重放不增加 revision、不调用 provider；原回执已被后续输入替代时 HTTP 返回 `409`，不能把 ABA 后相同文本当原保存成功。
新等价保存仍校验当前 CAS，并持久其独立回执；旧 schema 或伪造 expected hash 不进入等价旁路。
保存后的返回查询必须仍匹配原回执的输入及模板快照；同输入的编译推进可返回最新状态，被后续修订替代则返回 `409`，不得作为原保存成功。
不同输入的过期 CAS 仍返回 `409`。冲突后用户可显式读取当前草稿并放弃原保存重放身份，保留本地编辑；
该操作只读取，不自动保存或覆盖。用户核对 owner 当前目标后，另行提交修订并重新审查。
详情页的页面内存请求绑定 exact Workspace，不能跨工作空间复用保存或确认凭据。创作查询可返回 `confirmationRecovery`：仅从 owner 已验证的确认 journal 投影四项确认身份与当前 Workspace revision。revision 只约束首次准备，既有 journal 的恢复仍要求 exact hashes。无页面内存原确认时，用户可显式使用该参数恢复已开始的确认；读取本身不确认、不调用 provider，也不启动 Run。原页面 pendingConfirmation 不得被查询结果静默替换。

### 激活前准备状态读取

`GET /workspaces/:workspaceId/activation-preparation` 返回 `{preparation: ...}`：
`workspaceId`、`workspaceRevision`、可选 `activeRunId`、`operationState`、`currentValidity`、可选 `record`。
它仅投影 Workspace owner 已有准备操作；无记录时两个状态均为 `not_checked`，不创建账本或启动检查。
记录保存 schema version、有界 attempts、原 request/payload 身份、预期/提交 revision、精确确认与封存 binding、
操作时点和状态。已知时间/版本/配置/封存漂移可以使 `currentValidity=stale`，
但不把原 `accepted/unknown` 改成已完成，也不因此允许重新提交或恢复未知效果。
最新操作已保存的完整 Provider catalog 观察到达原 `ExpiresAt`（含等于）时，
GET/原操作重放/恢复只投影 `currentValidity=stale`，不改原检查原因、状态、请求或引用，也不刷新目录。
新显式检查在结算前由 File 再读完整原目录/资源/托管配置目标，保留原观察和后像。
File 的进程 epoch、精确 scope 和从观察开始起 30 秒年龄窗口随受限原档保存；后像变化、年龄到期、
File 重启或当前 epoch 不可读均使当前有效性 stale。GET/重复 check/recover 仅查询内存 epoch，不读文件、
不续期、不补造新证据。该窗口不是文件状态租约，时点相等也不代表全部资源有效、ready 或 M3；
响应不公开 token、原文或诊断，Start/Restart 回执消费仍不属于本步。
旧操作到期不绑定到没有新观察的后继操作；缺材料历史不补造期限，原材料绑定冲突拒绝读取。
未到期仍是 `not_evaluated`，不是当前 owner epoch、授权或 readiness 证明；不据此填充全 required 的最早期限。

新显式操作在首次接纳时固定 `workorch-preparation-owner-revalidation/v1`。
原 Soul、Skill Registry、含原模式的 Memory 库存和 MCP Registry 材料完整时，Application 在同一原请求预算内，
通过既有 typed query 再读这四类 owner；不重复 SkillHub status、不刷新目录、不执行 MCP 连接或模型调用。
最终完整原值与原观察一起在 Workspace 原结算 CAS 中受限保存；只排除观察时间差异，
epoch、revision、完整内容、选择及配置来源均精确反查。缺少最终材料、原值漂移、时钟回退或从最终观察开始
达到 30 秒时，GET/原操作重放/恢复投影 `currentValidity=stale`，不改历史逐项结论。
原材料不齐仍按其原阻塞处理，不为填充结构重复失败查询。旧操作不补挂该版本或补发观察。
这只是已知漂移与观察年龄的投影，不保证观察后资源不变，也不签发 readiness、M3 或执行许可；
最终原值和该内部合同标记不加入公共 HTTP 参数或原文投影。

该读取不编译 bootstrap、不查询外部资源、不刷新模型目录、不调用推理/embedding、不写配置、不创建 Run。
`activeRunId` 是独立已有事实，缺省不能证明没有历史或在途 Run；`eligible`、结构校验和准备操作状态均不是 M3。
响应禁止缓存，owner 原始错误不进入公共 payload。GET 不提供 M3/权限字段或 Start/Restart 回执消费承诺。

生产 Web 的工作空间详情开页、SSE／重连刷新及准备弹窗开关不读取含模型目录的系统配置，
也不查询 SkillHub 库存。库存指标未显式查询时显示“未加载”，不以零代替未知；
“更多操作”中的 SkillHub status 查询是独立显式动作，可能保存状态或访问其已配置服务。
仅显式打开“创建新执行”编辑器读取执行配置；等待或失败时不能提交，关闭／切换工作空间丢弃迟到响应，
重开重新读取。打开编辑器不提交执行意图，也不改变准备记录或授予执行权。

新原结算可保存 `assessment`：版本 `workorch-preparation-all-of-assessment/v1`、原 `manifestHash`、
`outcome=blocked|checks_passed`、`requiredCount`、`passedRequiredCount` 及完整 `outstanding` 列表
（每项 `id/owner/state/reason`）。它是原单项结果的合取摘要，不是 ready、当前有效性、M3 或执行许可。
owner 按原 candidate 反查完整必需清单；HTTP/客户端再次核对汇总与全部单项结果，不能只信通过数。
旧无汇总历史保持缺省，不因 GET、重放或恢复补算。

同次正常结算可保存 `definitionCompilation`，仅在完整原封存来源可反查且没有漂移时生成。
字段为 `compilerVersion=workorch-preparation-definition-compiler/v1`、原 `snapshotHash/manifestHash`、
`sourceInventoryHash/profileContractsHash` 与 `sourceCount/requirementCount/obligationCount/verificationCount`。
Workspace 从原 candidate/startup/admission 纯编译完整来源义务图，持久读取独立复算；
即使 Provider 检查受阻仍可完成此非 Provider 编译。它不是完整 bootstrap、资源 freshness、M3 或执行权限。
公共 DTO 不输出原 Goal/DoD、配置或资源 bytes，GET 不生成旧历史缺失的摘要。

版本 `emptySelectionEvaluationVersion=workorch-preparation-empty-selection-evaluation/v1` 限定精确空选择：
只有完整来源与原 seal/Goal/DoD 绑定、六阶段 Soul 及 selection/context/references/status 独立重编译后，
空 Skill 才可投影 `passed/skill_empty_selection_observed_remaining_checks`；完整 MCP 双读和配置/依赖
编译的 server/capability 集合也为空时，MCP 才可投影 `passed/mcp_empty_selection_observed_remaining_checks`。
缺来源、未知、漂移与非空连接待证仍阻塞；不把空列表直接当 N/A，不删除原编译诊断，不补做外部 probe。
该消费不证明持续有效或授予 M3；`operationState/currentValidity/activeRunId` 各轴语义不变。

显式 `POST /workspaces/:workspaceId/activation-preparation/initialize-resources` 与
`POST /workspaces/:workspaceId/activation-preparation/recover-resources` 只接受
`{requestId, expectedWorkspaceRevision}`。它们分别初始化缺失的准备基础设施、恢复服务端已受理的原初始化身份；
不接受路径、配置正文或检查结论。Workspace 独立保存原计划及每次派发预算后，由 File 创建缺失的托管配置和
项目 `.agents/skills`，保留已有文件与冲突，不安装或同步 Skill，不调用 provider，不创建 Run。

响应为 `{requestId, preparation}`。准备 GET 的可选 `resourceInitializations` 仅含原 request/revision、
committed revision、`pending/blocked/completed` 及已消耗派发次数；配置原文与错误只保留在受限 owner 存储。
同身份普通重复请求只读回；显式恢复保留原 expected revision，最多合计 4 次派发，丢失回执也不恢复预算。
未完成（pending/blocked）的初始化阻止新检查和 fresh execution reservation；completed 仅代表基础设施创建完成，仍须显式检查
实际资源，不授予 ready/M3。原失败检查不改写。页面只读刷新及关闭重开不自动提交；POST 响应丢失后从 GET
找回原身份，错误或 GET 尚未见到原记录时不换身份重发。所有这些响应均为 `no-store`。

当前生产初始化使用 `workorch-preparation-resource-initialization/v2`：File 阶段完成后，另行先 CAS
保存 SkillHub 登记派发，再由 Application 调用 SkillHub v0.13.7 的
`project register <精确交付目录> --request-id <稳定原身份> --json`。身份由 Workspace＋初始化 requestId
确定，并绑定服务端原 Hub home；HTTP 不接受 home、路径或外部回执。登记不扫描/导入/安装技能。
普通重放及 GET 均不执行 CLI；显式恢复在登记已派发后只用 `project register --query --request-id ... --json`
查询原回执，绝不重发登记。首次登记及后续查询共用独立的 4 次持久预算，不重置 File 的 4 次预算。
只有原 request、项目路径、完整终态、executed/finished 与成功 registered 结果全部匹配时，才能完成 v2
初始化；该回执仅证明历史登记，不能替代随后真实 status 或授予 readiness。failed、running、unknown、缺回执、
截断/无效输出或 scope 漂移均保持阻塞，不把 REQUEST_NOT_FOUND 当作从未执行。已确定失败不重复查询。
`resourceInitializations` 另含可选 `registrationState`（waiting_for_files/not_started/pending/unresolved/running/unknown/failed/succeeded）
和 `registrationAttempts`；不包含 Hub home、CLI 原文或结果正文。旧 v1 记录仍只代表原 File 操作，不会在
重放时升级或补执行登记。新登记回执不能补证历史无 request-id 的 status，既有未决 fence 保持不变。


显式 `POST /workspaces/:workspaceId/activation-preparation/check` 与
`POST /workspaces/:workspaceId/activation-preparation/recover` 只接受
`{requestId, expectedWorkspaceRevision}`；恢复必须携带原请求及原 revision，不能替换为查询得到的新 revision。
两者返回 `{requestId, preparation, compilation?}`，其中 requestId 指向原操作，preparation 同 GET 的持久读模型，
可以同时包含后续历史。`200` 表示原操作已有确定阻塞/失败结果，**不表示资源 ready**；
`202` 仅表示原操作 accepted/unknown。所有响应 no-store，拒绝 caller manifest、PASS 或 activation 字段。

可选 `compilation` 为 `workorch-original-prepared-bootstrap/v1` 的独立结构诊断轴，绑定原
request/revision 对应 attempt 的 `snapshotHash`、`manifestHash`。成功只带 `bootstrapHash`
（`prepared-bootstrap-sha256:` 加 64 位小写 hex）；失败只带有限 `failureCode`，不得同时有 hash。
失败码为 `original_inputs_unavailable`、`original_policy_unavailable`、`soul_source_unavailable`、
`skill_source_unavailable`、`memory_source_unavailable`、`memory_semantic_evidence_required`、
`mcp_source_unavailable`、`provider_declaration_unavailable`、`activation_structure_rejected`、
`bootstrap_structure_rejected`。只从同一回复的完整原私有归档重编；缺来源不补默认值或空集合。
不返回原始诊断、配置、资源正文或可执行 bootstrap；GET 无此字段且不触发编译。
该结果不改变原逐项检查、blocked 状态或 currentValidity，不是 readiness、M3 或执行权限。

当前 v2 主体流程先 CAS 保存原身份、完整稳定清单、creation/preparation epoch 及封存绑定，
只有首次 CAS 明确成功的 Application 调用继续执行有界 LLM cache-only 与 Run 完整历史 query，
再经内部 `settle-blocked` 合同由 Workspace 私有 reducer/CAS 记为 blocked。
provider 项分别记录 `provider_cache_miss/expired/unavailable` 或 `provider_declaration_only`；
新原非 stale 操作在同次冻结的 provider 默认项/override 声明和原 Workspace 选择完整时，
由 Workspace 私有编译精确模型来源，仍始终 blocked。旧无容量观察的成功编译为
`provider_selection_authorization_pending`，只代表目标及能力声明已绑定。
新首次受理 continuation 携带原精确目标，由 LLM owner 从同一次已有 fresh cache 与冻结配置
调用既有纯 resolver，给出版本化容量观察；Workspace 封存并核验原目标及原容量值。
`provider_selection_budget_observed_authorization_pending` 只表示容量预览已观察，
不是 Runtime ceiling、请求 allocation、Run 物理预算、远端授权或持续有效性证明；仍不授 PASS。
容量不可用或目标冲突分别为 `provider_selection_budget_unavailable`、
`provider_selection_budget_source_conflict`，不以默认值或相邻模型代替。
有限失败码为 `provider_selection_source_binding_conflict`、`provider_selection_configuration_source_unavailable`、
`provider_selection_invalid`、`provider_selection_model_missing`、`provider_selection_capability_incomplete`、
`provider_selection_operation_unsupported`、`provider_selection_reasoning_unsupported`；cache miss/expired 保留原分类。
新原因只允许原 evaluation v1、required/llm/blocked 检查与 blocked attempt，并必须有
`providerCacheEvidenceRef`；Web 保留原 code 并显示有限说明，不接收 caller PASS、任意后缀或原始配置。
原配置声明与选择只存私有材料，不增加公共字段；旧无来源历史不在 GET/replay/recover 时升级。
no-Run 项分别记录 `run_history_unavailable/retired/present` 或 `run_history_empty_fence_not_final`。
新原非中断操作的完整 Run owner query 结果以 `workorch-preparation-run-history-evidence/v1`
随原结算 CAS 私有保存，保留原 request、观察时间、scope、generation head、完整计数和 retirement fence，
以及原 binding/revision/creation/preparation epoch；clone 隔离、0600 保存、历史不可覆盖。
公开仅投影随机 `runHistoryEvidenceRef`（`run-history-observation:` 加 32 位小写 hex），
对应 v2 blocked attempt 和 required/run/blocked 的上述三类已观察原因，或漂移后的 `binding_changed`。
该引用不输出完整原材料、内容 hash 或权限，不证明当前无 Run；无法完整保存的观察不附引用。
旧无材料分类不在 GET/replay/recover 时升级或补取；恢复和重放均保留原身份与材料。
新鲜缓存仅为能力声明，仅空历史仍 blocked，不替代 Workspace reservation/epoch 与 final CAS。
原结算另带 `noRunEvaluationVersion=workorch-preparation-no-run-final-fence/v1` 时，允许
required/run 单项为 `passed/run_history_empty_final_fence_observed`：必须同时具备完整原 Run 空历史、
真实精确封存与持久完整性两个 passed 项、原 accepted 操作与完整最终 CAS 前像。
Workspace 私有 archive 保全前像，Repository 独立反查实际 revision（expected+1→expected+2）、
同 creation epoch、零 preparation epoch、无 pending/settled 与空 ActiveRunID；原操作及完整材料不匹配即拒绝。
公共 DTO 只输出有限版本、原随机引用与逐项结果，不包含完整 fence。
总体仍 blocked，无 M3 或执行授权；该 passed 只描述原结算时点，后续 reservation 可使 currentValidity stale，
即使历史 passed 仍在，也不能显示“当前无 Run”。旧空历史不补算、不升级，恢复不补造正例。
未接入的其余项为 `check_pipeline_incomplete`。
新受理 attempt 可带 `evaluationVersion=workorch-preparation-check-evaluation/v1`：目录、托管配置和
规格资源检查可分别显示原观察时点的 `passed`、确定性不满足的 `failed` 或缺事实的 `blocked`。
passed 原因仍为各自 `*_observed_remaining_checks`，failed 保留目录权限不足、配置格式/内容冲突、资源内容漂移的原原因。
非 blocked 的单项必须有相应 File owner 历史引用及正确 owner/required 身份；公共校验不代替 Workspace 的完整原材料反证。
原 manifest 对象身份只在 owner 侧封存；公共 DTO 不输出目录路径、原配置、资源全文或这些材料的通用诊断。
无该版本的历史保持旧语义，不在 GET/replay/recover 时回填单项通过；binding_changed 和中断恢复不能授通过。
单项 passed 不代表当前仍有效、整体 ready、首次 M3 或执行许可，整体操作仍 blocked；当前有效性仍单独投影。
同版 `exact_seal` 与 `persistence_integrity` 可在 Workspace owner 两次完整精确封存读取相等、
且原创作/正式行/候选/StartupInput/admission 全部精确关联后显示 passed，原因分别为
`seal_observed_remaining_checks`、`persistence_binding_observed_remaining_checks`；必须为 required/workspace，
并携带 `sealEvidenceRef`（`seal-observation:` 加 32 位随机小写 hex）。原两份完整材料保持私有；
该引用不是成功凭据，公共 DTO 校验不代替 owner 对原账本的反查。旧无引用记录不在读取/恢复时补编；
漂移保留历史且 blocked，中断恢复不补取。此处持久完整性仅指原 owner 绑定，不是磁盘可靠性、整体就绪或 M3。
同版 `soul_selection` 也可在完整精确六阶段均有明确选择、原封存绑定与双读匹配后显示 passed，
任一阶段 unbound 仍 blocked，不自动授 PASS/N/A；
须携带 Soul owner 原历史引用；接纳后 snapshot 身份只在 owner 侧冻结和反证，不暴露原 Soul 内容。
Provider 同一次 cache-only 完整点时观察可与原 accepted binding、request/payload、revision
在同一 blocked CAS 保存为 Workspace 0600 私有历史，不增加查询/refresh。
公共 v2 blocked attempt 仅增加 `providerCacheEvidenceRef`（`provider-cache-observation:` 加 32 位随机小写 hex）；
仅允许 required `provider_cache`/`llm` 检查及 miss/expired/declaration_only/binding_changed 原因。
引用不输出目录原值或配置，不是持续 freshness、连通性、授权、selection 或 M3 证明；
缺引用不证明缓存为空，旧记录不补造历史。GET、重放、恢复及 owner 重开不重查；
revision/epoch 漂移只保留历史并标 `binding_changed/stale`，不能把历史 epoch 当当前。
Memory 专用观察在原整体预算内一次读取完整本地库存（最多 256 项/1 MiB），不分页拼接、
不搜索、不召回、不调用 embedding、不将 required semantic 改成 list-only。
完整库存与原 accepted binding/request/revision 在同一 blocked CAS 保存为 0600 私有历史；
公共 v2 blocked attempt 仅含 `memoryInventoryEvidenceRef`（`memory-inventory-observation:` 加 32 位随机小写 hex），
且必须对应 required `memory_selection`/`memory` 检查。无精确模式来源仍为
`memory_inventory_observed_selection_pending`；缺失/错误/库存超限为 `memory_inventory_unavailable_not_saved`，不能伪装空库存。
`workorch-preparation-memory-selection/v1` 来源绑定原 accepted snapshot、库存与前后 active LLM owner 配置观察，
只在 defaultEmbedding 确实未配置时按原 list-only quota/预算纯编译；非空 semantic 库存保持
`blocked/memory_semantic_evidence_required`，选择候选超过 200 为 `blocked/memory_selection_candidate_limit`，不截断。
同一 `evaluationVersion` 下，完整 list-only／真实空库存可显示 `passed/memory_list_selection_observed_remaining_checks`
或 `passed/memory_empty_selection_observed_remaining_checks`；真实空库仍保存原 mode，沿用 activation 空库 fast path。
以上四种原因均须有原库存引用；单项通过还须有裁决版本与精确 owner/required，完整私有来源和选择由 owner 读回复算。
配置不可用、错误 DTO 或漂移不能被当作无 embedding 配置；完整库存可保留但不得授选择通过。
历史引用不输出原文、向量、配置或 epoch；不写 Run recall trace，semantic 非空证明仍待完成，不授 ready/M3。
GET/重放/恢复/owner 重开不重查或补造，漂移仅保留历史并标 stale。
MCP 准备观察只双读完整 owner registry（最多 256 个 server、4096 个 capability、完整 JSON 1 MiB），
不以 selected-only 子集代替，不分页拼接，不连接/initialize/refresh/EnsureDefaults 或执行工具。
原 accepted snapshot/request/revision/binding、时序和完整相等前后值随原 blocked CAS 归档为 Workspace
0600 私有历史；读回独立核验完整内容 revision 与原操作关联，不把该 revision 当作连接 epoch。
公共 v2 blocked attempt 仅含 `mcpRegistryEvidenceRef`（`mcp-registry-observation:` 加 32 位随机小写 hex），
对应 required `mcp_selection`/`mcpregistry` 的有限 blocked 分类；无选择裁决版本的历史保留
`mcp_registry_observed_connection_pending`。空 registry 也不能授 PASS/N/A。
错 DTO、超限、双读漂移或未保存为 `mcp_registry_observation_unavailable_not_saved`，
不能伪装成功空值或截断归档。原非 stale 结算另在私有历史保存版本化精确 selection 编译：原配置默认项
按 Workspace-over-Template 递归覆盖，与同次完整 Skill selection 的必需引用取并集，核对对象存在、enabled、
capability 所属 server 与 allowlist。缺来源、歧义、超限或不满足均失败，不自动补 server、不连接，
不把空集合或编译成功授为 PASS/N/A。原 CAS 独立核验声明与原 accepted 配置身份，读回重编译并核对完整
Skill 历史。新原操作以私有 `workorch-preparation-mcp-selection-evaluation/v1` 消费编译，
公共原因必须由原材料重算，且只允许下表有限值。均要求 v2、`evaluationVersion` 为原逐项裁决版本、
blocked、required/mcpregistry 与完整 `mcpRegistryEvidenceRef`；引用和裁决版本不是连接成功凭据。
不公开选中对象或原失败材料，不接收 caller 分类、任意前缀、伪造 passed/failed 或 N/A。

| `mcp_selection_` 后缀 | 历史结论（所有项均 blocked） |
| --- | --- |
| `compiled_connection_pending` | 非空选择编译完成；尚缺真实连接与持续有效性证明 |
| `empty_policy_pending` | 原选择确实为空；尚未证明可豁免 MCP，不授 N/A |
| `source_binding_conflict` | 原操作、版本或来源绑定不符 |
| `configuration_source_unavailable`、`configuration_declaration_invalid` | 配置来源缺失或声明错误/歧义 |
| `selection_limit` | 选择数量超限，不截断授信 |
| `skill_source_incomplete`、`skill_requirement_invalid` | 完整 Skill 选择不满足前置或必需引用无效 |
| `server_disabled`、`server_missing` | 所需 server 禁用或缺失 |
| `capability_disabled`、`capability_missing` | 所需 capability 禁用或缺失 |
| `capability_server_not_selected` | 所属 server 未选中，不自动补选 |
| `capability_not_allowed` | server allowlist 不允许该 capability |

新归档不得省略选择消费者版本；旧未裁决历史可读但不自动升级，完成原操作不可通过降级或重哈希改写。
Web 同时展示原代码与有限说明，只读查看不触发检查、恢复、修配置或执行。
引用不输出 endpoint、环境引用、schema 等原材料，也不能证明 required connection、持续 freshness 或 M3。
GET/重放/恢复/owner 重开不重读或补造；旧 registry-only 记录不补算、不升级，
已知 revision/epoch 漂移仅保留历史并 `binding_changed/stale`。
内部 settlement 另由 Workspace 从原 accepted 操作的当前精确封存解析目录，向 File 请求实际只读身份与有效权限观察。
File 仅接受 Workspace source，逐段拒绝软链接，不写探测文件、不创建目录、不修改权限；有效权限由实际进程凭据观察。
目录项分别为 `directory_observation_unavailable/incomplete`、`directory_permissions_denied` 或
`directory_observed_remaining_checks`，均保持 blocked；目录读取成功不能替代管理配置、规格资源或其余 required 检查。
原观察绑定原 RequestID/payload hash、受理 revision、时间、目录身份及诊断 bytes/字节数/SHA256；
随原结算 CAS 原子保存到 Workspace 私有材料。公共 attempt 只含 `directoryEvidenceHash` 历史引用，
不返回材料原文；引用不认证当前文件状态、不授予 PASS。材料文件要求 0600，重开须校验原文 hash 与原操作关联。
结算 revision/epoch 漂移只保留历史观察，逐项记 `binding_changed/stale`；材料未提交不得在恢复中补造。
同一原检查还从确认规格解析精确资源引用，由 File 在交付目录内实读完整普通文件（无软链接、硬链接、跨挂载或路径逃逸）。
每份最多 32 KiB、总计 128 KiB；超限/失败不保存部分前缀。完整原字节、版本与原错误 bytes 仅在 Workspace 0600 私有材料中归档，
公共 attempt 只含 `resourceEvidenceHash` 历史引用，不输出资源原文或诊断。
原诊断超过 4096 字节时不保存/截断原文；私有材料显式记录未保存原因、原字节数与 hash，不能标为完整观察。
`resource/*` 项分别记录 `resource_observation_unavailable/incomplete`、`resource_content_changed`、
`resource_expected_hash_unavailable` 或 `resource_observed_remaining_checks`，均保持 blocked。
内容相等不是持续有效性或 M3；原规格声明的 Available 不作事实，无法解释的引用/哈希不猜选。
托管配置另从精确确认的 Template snapshot 与 Workspace 配置声明解析目标，File 分别读取 system/workspace 根，不作 fallback。
system 核对完整默认对象；workspace preserve 允许已有覆盖，mergeMissing 只检查原补缺规则的缺键，不覆盖现有值。
不生成、合并、修复配置。非法/歧义声明、非法 JSON、读取不完整或内容冲突分别记录
`managed_config_declaration_invalid`、`managed_config_content_invalid`、`managed_config_observation_incomplete`、
`managed_config_content_conflict`；owner 无有效回执为 `managed_config_observation_unavailable`。
内容匹配或确认无声明仍保持 `blocked/managed_configs_observed_remaining_checks`。
每份/总字节上限、受限原文和 append-only CAS 同上述文件材料；公共 attempt 仅增加
`managedConfigEvidenceRef`（`managed-config-observation:` 加 32 位随机小写 hex），与配置内容无关，
不公开配置原文或内容派生摘要。该引用不是持续有效性、成功或权限证明。
GET、完成重放、显式恢复与 owner 重开不重读；未提交材料不补造，新检查保留全部旧原文及引用。
Soul 精确选择观察也汇入同一原请求结算。进程实际配置 accessor 不可用不等于空 default；
六阶段按原 stage/Workspace/system 优先级选择，向 Soul owner 实读两轮完整 typed record，
保存原 revision/provenance/指令/列表、前后 system default 与 exact seal 绑定。无裁决版本的历史完整相等仍为
`blocked/souls_observed_remaining_checks`，不授 ready/M3；缺失、错误或漂移为
`soul_observation_unavailable_not_saved`，明确未保存完整材料，不用部分材料冒充完整观察。
公共 attempt 仅含 `soulEvidenceRef`（`soul-observation:` 加 32 位随机小写 hex）；
原值只在 Workspace 0600 私有历史中保存，完整前后值由 owner 反证，引用与原 request/revision/configuration 同 CAS。
GET、完成重放、显式恢复和 owner 重开不重读，未提交材料不补造；已知 revision/epoch 漂移仅保留历史。
完整资源事实组合尚未完成，整体操作仍为 blocked；版本化单项 File/Soul/Memory 裁决不能替代全 required 合取。
Soul/Registry 查询本身不调用 SkillHub、外部刷新、推理/embedding 或执行入口。
内部接纳延续提示不进入公共 HTTP，也不能由调用者提交作为授权凭据。
普通同键同输入重放只返回原记录；同键异输入冲突。accepted/unknown 未结算时，新键不能绕过原操作。
显式恢复只将中断的原记录收口为 `original_check_interrupted` 阻塞，不重跑观察或外部效果；
已知 revision/epoch 漂移记为 `binding_changed` 且 currentValidity=stale。终态原记录不可改写，
资源变化后的检查需使用新的显式请求与当前 revision，且不得存在未决 SkillHub 效果；保留全部旧记录，最多 64 次，不淘汰旧身份。
v1 只读 trace 不升级成 v2 正例，缺 creation fence 的输入不能开启该流程。

错误响应（包括 400/409/500）不证明之前没有部分持久化。Web 保留页面内存中的原意图，POST 后 GET 对账，
未见原请求或仍 unknown 不解锁新身份；重开页只从服务端记录获取已受理原操作，GET 不代替显式恢复。
当前没有正 readiness、历史 M3 签发或 STEP05 消费链；不能把框架阻塞闭环当成完整 STEP04 退出证据。

显式新检查的首次 CAS 还封存 SkillHub status 两个目标（global、原交付目录 project）与原 request/binding。
仅无歧义首次延续可各执行一次，沿用现有 SkillHub 服务访问及状态保存行为，不执行 init/install/sync/use/apply/repair。
GET、普通重放、显式恢复或重开不调用 status。公开 attempt 可有 `skillHubEffectRef`
（`skillhub-status:` 加 32 位随机小写 hex）和独立 `skillHubEffectState`：
accepted/unknown 对应 reserved；blocked 对应 recorded 或 unresolved。recorded 只指历史已记账，
可包含真实 not_started，不代表 Skill readiness。已知失败保留原退出事实；命令失败、退出未知、
流不完整或中断缺少回执为 unresolved，后端和 Web 均禁止换 request 绕过，无自动对账清除入口。
原 stdout/stderr/错误 bytes、完整性、总字节数、时间与退出事实仅保存在 Workspace 0600 私有历史；
每流最多保存 1 MiB 捕获前缀并显式标记 incomplete，错误超 64 KiB 时不保存前缀，明确原因及原总大小/full SHA256。
这些材料不进入普通 Workspace JSON/HTTP，迟到回执不覆盖已结算历史。
准备视图可有 `skillHubProjectIssue=project_registration_required`，仅由 Workspace owner 从最近
blocked 原操作的 project 完整双流、真实 exit 1 和唯一 `command_error.code=PROJECT_NOT_FOUND`
文档反查生成。它是有限历史诊断，不输出原 message/路径，不把错误当作空库存，不修改原
attempt、效果 unresolved 或 currentValidity。SkillHub v0.13.6 的 JSON status 不隐式登记新项目；
仅创建 `.agents/skills` 目录不等于已登记。GET、刷新和恢复不登记、不重跑 status，也不清除旧效果栅栏。
相对或非规范交付目录保留原值，派发前记录 directory_invalid_not_started；不猜选其他目录。
成功 status 后仅使用该次原始回执观察已安装指令，不再次执行 status；两轮真实文件读取及严格
metadata 核验的材料随原操作私有封存。公开可有 `skillHubInstalledState`：
`incomplete` 表示部分/失败材料；`observed_selection_pending` 表示两个 scope 指令双读及元数据完整，
仅为历史材料，不是当前 freshness、选择通过或资源就绪。非空值只允许 v2 blocked attempt 且必须
有原 `skillHubEffectRef`；缺材料不造此字段。每文件 64 KiB、每 scope 每轮总计 8 MiB，
GET/重放/恢复/重开不补读安装文件，原文不进公开 DTO。原私有结算从 Registry、Soul、SkillHub
完整材料纯编译组合选择并随同一 blocked CAS 归档，独立重算原来源、allowlist、候选、分数／理由与
选中内容；不再执行 status，不新增结算 revision。公开可有 `skillSelectionState`：
`sources_incomplete|conflict|compiled_checks_pending`，缺来源不冒充空成功；非空值须为 v2 blocked
且带原 `skillRegistryEvidenceRef`，完整编译另须 Soul、recorded effect 和完整安装材料引用。
原 status 不变；编译不授辅助资源、当前状态／freshness、context 预算或 readiness credit。
GET/重放/恢复/重开不补编译或改写历史，状态展示不能当成完整 STEP04 通过。
同次原操作还可有独立 `skillReferenceState=observed_checks_pending|sources_incomplete|source_unresolved|unavailable`，
仅允许 v2 blocked 操作及完整 `skillSelectionState=compiled_checks_pending` 来源。
该轴消费私有安装根下声明辅助文件的两轮原读取；Registry 引用没有根目录来源时明确 source_unresolved，
不猜路径、不改原 status、不授 Skill PASS 或持续有效性。公共字段不含路径、原文、内容 hash 或诊断。
旧无材料历史不在 GET/replay/recover/reopen 时补读，原材料及选择归档保持 append-only。

独立 `skillStatusState=observed_freshness_pending|status_blocked|version_conflict|sources_incomplete`
只允许 v2 blocked 操作及完整组合选择来源。该轴从原完整 status 与实际安装指令重编译选中来源诊断：
非精确 synced（含未知／空状态）明确受阻，安装 frontmatter 与非空 status 本地版本不同明确冲突。
仓库版本不代替本地版本，缺少一侧版本不证明一致；synced 和 Registry 观察也不授 freshness、
daemon/local 一致性、ready 或 M3。公共字段不包含原 status、版本、路径或诊断正文。
GET/replay/recover/reopen 不重新检查或补写原诊断。

同一次结算可有 `skillContextState=compiled_checks_pending|bounded_checks_pending|rejected`：
表示既有纯 context 预算合同的独立预览或拒绝，非空须绑定 v2 blocked 及
`skillSelectionState=compiled_checks_pending` 的完整来源。正文原材料不变，预算预览及完整诊断留在
受限 archive，由 Workspace 重编译全值核验；公开字段不携带正文、诊断或可用于执行的凭据。
有界投影明确标记裁剪，不表示辅助资源／freshness／M3 通过；GET/replay/recovery/reopen 不补算或覆盖。

## Command Scope And Responses

- path scope wins over body scope；unknown JSON fields or cross-workspace redirect are rejected；
- owner-generated identity/revision/hash 必须从 concrete result 返回，handler 不自行生成替代 authority；
- 业务 JSON、原始内容、SSE 和指标响应使用 `Cache-Control: no-store`，Web API 请求禁止浏览器 HTTP 缓存；页面内存中的在途 GET 合并不构成持久存储；
- created=`201`、accepted execution=`202`、successful update=`200`、empty delete=`204`；
- invalid=`400`、not found=`404`、blocked/stale/conflict=`409`、admission wait canceled=`408`、unexpected dependency=`500`；
- credentials、raw prompt/provider body、unsafe diagnostic 和 internal aggregate 不进入 response。

本机服务停机先关闭新 HTTP 准入及 listener，并取消只读 SSE 的等待和历史回放；不能依赖浏览器先关闭标签页。
取消不是回放读取或 handler 的完成回执：必须等待真实返回及连接排空后才能释放传输依赖。
普通已接纳 mutation 不受 SSE 取消影响；超时仍保留依赖并按原停机合同重试，不强制关闭或假报成功。

## Verification

- route/DTO/removed-route guards：`internal/modules/application/workorchd/service/server/http_*.go` and tests；
- deployed transport/owner integration：[Service Acceptance](service-acceptance.md)；
- Runtime/owner invariants：[Core Runtime Verification](../00-core/runtime/verification.md)。

Route change必须在同一变更中更新 route tests、command/read contract、Web/TUI client contract 与 service acceptance。
