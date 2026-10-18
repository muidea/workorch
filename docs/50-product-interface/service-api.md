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
- Restart 使用 fresh eligible bootstrap；runtime clarification predecessor 要求 Definition revision 与 StartupInputHash 均变化，
  任一未变化返回 `governance_required/definition_clarification_pending` 且零写入；通过后 Run owner先 seal predecessor
  `ReplacementIntent`，terminal delivery 完成后才创建 successor；
- handler 同步等待并校验 exact Workspace owner result。只有 `reconciliation_requested|replacement_pending` 返回
  `202`；`restart_required|fresh_activation_required|definition_changed_requires_restart|governance_required`
  返回 typed `409`。两类响应都原样携带 `workspaceId/runId/requestId/intent/outcome/reason`，不存在
  application-local queue/queued acknowledgement；`requestReason` 与 owner-authored `reason` 分离。

响应中的 `accepted`、`state=accepted|rejected`、HTTP status 和 display diagnostic 只由共享 pure admission classifier
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
`restartRequired` when persisted and active revisions differ。

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
标记未保存，已知失败仍为 failed。旧失败无诊断时不得回填猜测内容。文件私有权限和部署访问边界保护原始材料，
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
