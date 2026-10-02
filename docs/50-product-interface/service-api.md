# WorkOrch Service Command API

Status: normative

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
| `POST` | `/workspaces/:workspaceId/authoring/compile` | 托管派生与六类独立审查；202 仅为任务接受回执 |
| `POST` | `/workspaces/:workspaceId/authoring/confirm` | 精确确认 input／permit／proposal／specification hashes，物化并准入 |
| `DELETE` | `/workspaces/:workspaceId` | submit one Workspace-owner delete intent; optional typed purge mode |
| `PUT` / `DELETE` | `/workspaces/:workspaceId/llm-selection` | owner-CAS update/clear selection |
| `PUT` | `/workspaces/:workspaceId/soul-bindings` | update owner-managed Soul routing |
| `POST` | `/workspaces/:workspaceId/template-binding-refresh` | explicitly refresh template-bound policy; does not mutate a sealed Run |
| `POST` | `/workspaces/:workspaceId/execution-activations` | submit `resume|restart` intent |

创作输入为 RequestID、名称、Template、Goal、资源引用及可选用户验收要求；工作目录可留空，解析为
`.workorch-workspaces/<workspace-id>`。正式 Goal／DoD 仅在精确确认事务中物化，随后由 owner 准入；创建、编译、确认均不启动 Run。
普通 setup 不要求 raw context/evidence/target/recipe/check/metric/artifact JSON。

旧 `POST /workspaces`、Goal／DoD POST／PUT、`goal-dod-quality-check` 和内存 draft session 写路由均已退休，返回 `404`。
正式定义 GET 保留作读取；同 Run 规格修订不通过旧 CRUD 暴露。
Go client 只提供正式 authoring 写方法，退休创建／Goal／DoD 写方法不再存在。
TUI 的旧创建、目标提交及 DoD 保存命令明确拒绝，不调用 backend 写操作、不生成默认 DoD，也不自动 Restart；
当前 TUI 未提供完整创作向导，帮助、空态和补全不推荐退休写命令；用户通过 Web 完成规格创作后，可在 TUI 读取并显式提交执行。
CLI 的 `workspace create` 与 Goal-first `agent [goal]` 已隐藏，并在访问 backend 或订阅事件之前直接拒绝；agent 不再生成默认 DoD 或自动提交 Restart。CLI／TUI 尚未交付完整规格创作向导。保留旧命令解析仅用于返回明确退休提示，不构成兼容创建路径。
local EventHub 与 remote backend 对退休写接口均在发送前返回冲突提示；五个退休 CLI 定义写 topic 不再注册为 production facade 或 dispatcher 订阅，handler 的直接防御分支仍拒绝，不能绕过 backend 调用旧创建或 Goal／DoD 写用例。Workspace owner 内部 typed command 保留供受控事务与测试夹具使用。

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
确认必须绑定当前输入及完整审查身份。浏览器本地编辑不是 owner 草稿或准入凭据。

保存请求必须显式提供非空 `templateId`，不得用现行记录补齐原请求身份。保存响应丢失时，浏览器保留原 pendingSave 的 `requestId`、模板、输入、expected revision 与 inputHash，优先重放原请求。
Workspace owner 在同一 CAS 中持久有界保存回执（最多 128 条，不淘汰迟到重放身份）；相同 RequestID 的不同 payload 返回 `409`。
原请求精确重放不增加 revision、不调用 provider；原回执已被后续输入替代时 HTTP 返回 `409`，不能把 ABA 后相同文本当原保存成功。
新等价保存仍校验当前 CAS，并持久其独立回执；旧 schema 或伪造 expected hash 不进入等价旁路。
保存后的返回查询必须仍匹配原回执的输入及模板快照；同输入的编译推进可返回最新状态，被后续修订替代则返回 `409`，不得作为原保存成功。
不同输入的过期 CAS 仍返回 `409`。冲突后用户可显式读取当前草稿并放弃原保存重放身份，保留本地编辑；
该操作只读取，不自动保存或覆盖。用户核对 owner 当前目标后，另行提交修订并重新审查。
详情页编辑恢复记录绑定 exact Workspace，不能跨工作空间复用保存或确认凭据。

## Command Scope And Responses

- path scope wins over body scope；unknown JSON fields or cross-workspace redirect are rejected；
- owner-generated identity/revision/hash 必须从 concrete result 返回，handler 不自行生成替代 authority；
- created=`201`、accepted execution=`202`、successful update=`200`、empty delete=`204`；
- invalid=`400`、not found=`404`、blocked/stale/conflict=`409`、admission wait canceled=`408`、unexpected dependency=`500`；
- credentials、raw prompt/provider body、unsafe diagnostic 和 internal aggregate 不进入 response。

## Verification

- route/DTO/removed-route guards：`internal/modules/application/workorchd/service/server/http_*.go` and tests；
- deployed transport/owner integration：[Service Acceptance](service-acceptance.md)；
- Runtime/owner invariants：[Core Runtime Verification](../00-core/runtime/verification.md)。

Route change必须在同一变更中更新 route tests、command/read contract、Web/TUI client contract 与 service acceptance。
