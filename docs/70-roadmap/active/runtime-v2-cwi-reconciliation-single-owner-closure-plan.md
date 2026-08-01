# Runtime V2 单 Owner、CWI Evidence 编译与正向收敛收口计划

Status: active

Type: closure-plan

Last Updated: 2026-08-01

## 1. 范围、事实与目标

本文件是唯一 active Runtime V2 收口队列。设计 SoT 仍为
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)、
`internal/modules/kernel/orchestrator/readme.md` 与
`internal/pkg/workorchruntimecontract`；本计划只记录尚未完成的实施顺序和验收。

2026-07-31 的 fresh deployed LSP Bridge workspace 已确认下列事实：

1. 新 workspace 的 `/run resume` 返回 `restart_required`，未创建 Run、未请求 LLM、未产生副作用；该行为正确。
2. `/run restart` 的 HTTP transport 返回 `202`，但 activation 在创建 Run 前被拒绝：

   ```text
   coverage_binding_unavailable:
   focus "focus-005" validation proof must be bound to one CWI;
   shared recipe/check declarations cannot close 4 CWI items
   ```

3. 拒绝发生在 bootstrap 编译阶段，故 SM-1 Run、SM-2 reconciliation、SM-3 candidate/dispatch、SM-4 lease/delivery
   均尚未进入。没有文件、LLM、治理票据或 capability 副作用。
4. `restart` 失败后的 scheduler 仍推荐 `run_resume`；当前没有 active Run，resume 只会再次得到 `restart_required`，
   因而反馈不具备可执行性。

本轮的目标不是放松 `CoverageEvidenceBinding`、让 generic validation 关闭多项 CWI，或新增兼容分支；而是使
activation 只封存完整、可执行、逐 CWI 的合同，随后由既有四个 owner 状态机持续正向收敛。

## 2. 已关闭基线与禁止回退

下列项目已经完成，不能再作为 active 缺口或重新引入旧路径：

| 基线 | 当前事实 |
| --- | --- |
| Runtime V2 production path | 仅 `WorkspaceExecutionIntent -> Run owner fact -> DriveRuntimeV2`；V1 StagePacket/NextAction/graph dispatch 已移除。 |
| SM-1 direct lifecycle ingress | application direct Create/Start/Resume 生产 facade 与 Run-ID lifecycle HTTP 写入口已删除；Run `activate`/`continue` 仅接受 Run internal source。 |
| ingress owner outcome | Workspace owner result 已携带 `RunID`、`Disposition`、`Diagnostic`；TUI/backend 不得以 transport accepted 代替 owner outcome。 |
| SM-4 producer trust | Drive 仅接受 Orchestrator internal source；termination 仅接受 Run owner 的 persisted supersede fact。 |
| C12 evidence baseline | CWI credit 只由 sealed binding、sealed operation 与匹配 owner receipt 派生；candidate raw CWI、claim、exit 0、Git/file existence 或 generic summary 均不得越权。 |
| 22-unit framework boundary | module/block/initiator 边界与 `pkg` purity 已有回归基线；`pkg` 不得承载状态迁移、CAS、EventHub、I/O、timer 或 retry 决策。 |

禁止恢复：application continuation queue、Run-ID lifecycle 写 API、自动 successor、V1 DTO、map/JSON/reflect authority
bridge、projection/TUI 反向驱动、跨 owner repository/service 调用、或以放宽 decoder/evidence gate 掩盖本计划的编译缺口。

## 3. 唯一状态 owner 与允许交接

每次跨组件输入只能是 immutable intent、已持久化 owner fact、receipt 或 acknowledgement；接收方先校验
`workspaceID/runID`、source、causation、definition/evidence revision 及 sealed identity，之后才转换为 owner-private
internal event。外部消息不得携带接收方目标状态。

```text
external intent / persisted owner fact
  -> receiver validates source + scope + identity
  -> receiver-private internal event
  -> receiver reducer + receiver CAS
  -> immutable notification / receipt / acknowledgement
```

| 状态机或资源 | 唯一 owner | 允许的跨 owner 输入 | 明确禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | Workspace execution selection、Orchestrator lifecycle/delivery fact | application/TUI/Web 直接 start/resume/terminal Run；SM-1 选择 CWI/action |
| Workspace active-run slot | `blocks/workspace` | Run state fact、fresh-activation release、terminal delivery | 创建/替换 Run；读取或写 aggregate；解释 CWI |
| SM-2 authority/focus/coverage/reconciliation | `kernel/orchestrator` | capability observation/receipt、sealed definition/reconciliation contract | 执行 operation、直读 capability repository、observation 新增 credit |
| SM-3 candidate/sealed dispatch | `kernel/orchestrator` | LLM document、owner feasibility、operation receipt | LLM/owner 选择 retry、ticket、terminal 或下一 action |
| SM-4 drive/recovery/delivery ledger | `kernel/orchestrator` | Run active/continuation fact、persisted lease fact、delivery acknowledgement | 重建 authority、改 focus/CWI/candidate、写 Run/Workspace/Result |
| Result lifecycle | `blocks/result` | sealed terminal delivery | projection/UI/Run 设置结果状态 |
| Decision / Approval | 各自 Block | governance intent/resolution | Resume/Drive Run、修改 coverage/terminal |

Orchestrator 可以拥有 SM-2、SM-3、SM-4，但必须保留分离的 internal-event kind、reducer entry 与可写字段；共享
`RuntimeAggregate` 不是跨状态机写权限。

## 4. 根因：slice presentation 与 semantic proof 互相矛盾

当前 compiler 先按 `AcceptanceSlicePolicy.MaxWorkItems=4` 合并同 policy 的 DoD item，再由
`compileRuntimeV2CoverageEvidenceBindings` 正确拒绝“一个 focus 内多 CWI 共享 validation recipe/check”。

这不是 LLM 输出不稳定、SM-3 decoder 或 capability receipt 的问题，而是 activation 层在封存 authority 前产生了
自相矛盾的输入：

```text
DoD item CWI-1..4
  -> slice policy groups four items for presentation
  -> one focus copies one validation proof to four bindings
  -> binding compiler rejects, because one generic receipt could close four CWI
```

必须按以下规则归一：

1. `RuntimeAcceptanceSlice` 只表达排序、展示和批次上限，永远不能扩大 semantic evidence credit 的范围。
2. 任何需要 validation、executable behavior、metric 或 artifact proof 的 CWI 必须有单 CWI focus、单 CWI
   `CoverageEvidenceBinding`、单独的 target/selector、recipe/check/metric/artifact declaration。
3. wildcard DoD policy 只能表达通用质量门禁，不能以一个 generic recipe/check 关闭任意业务语义 CWI。
4. profile/template 必须为可语义关闭的 CWI 提供静态、可哈希、owner 可独立验证的 proof declaration；LSP Bridge
   的工具、语言、unsaved edit、输出限制、性能/并发、配置、文档与交付物必须逐项声明。
5. 缺 proof 的 CWI 必须在 provider 前以完整的 `coverage_binding_unavailable` report 拒绝。拒绝不得创建 Run、
   candidate attempt、dispatch、ticket 或副作用。

## 5. 目标执行路径

```text
TUI / Web / HTTP
  -> WorkspaceExecutionIntent{resume|restart, workspaceID, requestID}
  -> application adapter: restart only compiles pure bootstrap; returns diagnostic unchanged
  -> Workspace private event: validates definition + active slot
  -> WorkspaceExecutionSelection fact
  -> Run private event: selects same Run or creates replacement / predecessor lineage
  -> RunStateChanged or RunContinuationRequested fact
  -> Orchestrator SM-4 private schedule/lease
  -> SM-2 reconciliation (current_run | workspace_full)
  -> owner observations/receipts -> SM-2 authority
  -> SM-3 request/validate/seal/execute
  -> SM-2 evidence/coverage/focus convergence
  -> sealed terminal -> SM-4 delivery ledger
  -> Result private event -> Workspace private event -> Run private event
```

`resume` 在 current non-terminal Run 上执行 `current_run` reconciliation；`restart` 创建 replacement Run，首轮执行
`workspace_full` reconciliation。两者都不继承 predecessor credit、dispatch、repair ledger 或 focus。

## 6. 实施阶段

### Phase A — 纯 CWI contract 的编译归一

**主要组件：** `application/workorchd/biz/appservice/activationcore`、
`internal/pkg/workorchruntimecontract`、`internal/pkg/workorchdefaults/defaults/templates.json`。

1. 在 focus compiler 中将 presentation slice 与 semantic focus 分离：需 validation/behavior/metric/artifact proof 的
   item 一律分割为单 CWI focus；不得靠 `MaxWorkItems`、同 policy hash 或相同 recipe 将其合并。
2. 在 `workorchruntimecontract` 仅增加纯 DTO 的 normalize/validate/hash 不变量：一个 validation/behavior binding
   精确对应一个 CWI；slice 多 item 不可隐式复制 proof。不得将 reducer、I/O 或 profile/domain 判断放入 `pkg`。
3. 将 profile/template 的 semantic proof 声明改为 CWI 级输入。LSP Bridge 所有 required behavior 各自具有精确的
   target/selector、允许 operation/effect、recipe、required check，以及适用 metric/artifact。
4. 对 generic `software-dev` wildcard 规则降级为不授予 semantic CWI credit 的通用质量门禁；它可约束候选和 receipt
   quality，但不能代替 profile-specific binding。
5. activation 预检必须收集、稳定排序并报告全部不可编译 CWI（CWI ID、source、缺 target/recipe/check/metric/artifact
   字段、declaration ref），而非仅报告首个 focus。

**门禁：**

- 含四个 validation CWI 的 slice 能产生四个 independent bindings/focuses，且任何单 receipt 最多获得其 binding 的
  一项 credit。
- wildcard/generic validation、TODO test、exit 0、summary、文件存在、Git revision、candidate claim 均不能关闭 CWI。
- 没有 proof 的 profile 在 provider 前失败，且零 Run/LLM/operation/governance/effect。
- `pkg` 静态检查确认无 EventHub/CAS/I/O/state transition。

### Phase B — profile semantic proof 与 owner receipt 对齐

**主要组件：** default profile/template、`blocks/file`、`blocks/shell`、`blocks/mcp`、`blocks/git`、
`kernel/orchestrator`。

**2026-08-01 已收口的前置链：** workspace 的 `contextHints.profileContracts` 现在会按
`(scope,id)` 合并 `evidenceRequirements`，因此 workspace/template 可以在保留 `*` 质量门禁的同时追加或替换
精确 CWI semantic proof；它不再被模板同名 collection 静默丢弃。managed validation 的 candidate manifest
同时只暴露当前 sealed binding 对应、且已由 Shell owner 确认的 recipe，禁止同一 admissibility snapshot 中其他
CWI 的 recipe 被模型选择后再在 receipt 阶段失败。该变化只影响 activation 编译和 SM-3 纯 manifest admission；
不新增状态迁移、不向 `pkg` 写入 I/O/CAS，也不放宽 wildcard credit。

activation 还会在创建 Run 前拒绝一个静态矛盾：声明 Shell/managed validation 的 exact proof 若引用的
`validationRecipeId` 没有对应 `RuntimeEvidencePolicy{recipeDeclared=true}`，会以
`coverage_binding_unavailable` 给出 CWI、declaration 与 recipe 原始信息。Shell 对命令可执行性的实际确认仍由
owner admissibility 完成；此处不将预检当作 owner receipt。

**2026-08-01 已收口的声明入口：** `WorkspaceTemplate` 的 HTTP owner command 一直持久化
`contextHints` 与 `evidencePolicies`，但 Web 编辑器和 TUI/remote 的 typed template transport 曾遗漏
`contextHints`；编辑后会无声丢失 `profileContracts`，或使精确 CWI declaration 根本无法配置。本轮已将该字段
贯穿 `workorchport.TemplateInfo`、CLI EventHub adapter、remote client DTO 与 Web template manager。Web 以 JSON
显式编辑并在提交前校验 object/array 形状，保存/读取保持原始 declaration；该入口只写 Workspace Template owner
资源，既不写 active Run，也不触发 Drive。模板或 workspace declaration 改动只能通过后续 explicit `restart`
形成新的 bootstrap，不能以 `resume` 偷换已封存合同。

**仍未关闭：** `software-dev` 默认 profile 仍只有 `dodItem/*` 通用质量门禁，未提供 LSP Bridge 各 CWI 的
exact target/selector、recipe、check、metric/artifact declaration。因此 fresh LSP workspace 继续在 provider 前
返回完整 `coverage_binding_unavailable`；这不是可通过默认猜测或宽松 validator 解决的项。必须由该 workspace
选择的用户 template，或该 workspace 自身的 exact overlay，提供声明和相应可执行 Shell/MCP recipe 后，才可进入
owner receipt / live convergence 验收。

`LSP-Bridge MVP` 是当前用户 workspace，不是 WorkOrch 产品默认。其 Goal、DoD、target、recipe、check、
语言、目录和任何 CWI matrix 都只能保存为该 workspace 的 owner resource，或由用户创建并显式选择的可复用
template；不得写入 `internal/pkg/workorchdefaults/defaults/templates.json`，也不得由 Runtime V2 从 workspace
名称、目录或 Goal 文本反推。内置 LSP/MCP server 与 capability 注册仅描述平台可用能力，不能隐含或生成任何
workspace deliverable declaration。

**2026-08-01 已收口的 workspace 声明入口：** 创建 Workspace 的 typed HTTP、TUI/EventHub、remote backend
合同均可携带 `contextHints` object 与 `evidencePolicies` object array。application 严格执行
“create -> optional template -> Workspace owner metadata update”顺序；workspace-specific `profileContracts`
按既有 `(scope,id)` 规则与模板质量门禁合并，evidence policy 按 `id` 稳定替换/追加。该入口不创建 Run、不调度
Drive、不请求 LLM；声明只会在用户后续 explicit `restart` 的新 bootstrap 中封存。Web 新建 workspace 向导提供
对应的可选 JSON 输入和对象/对象数组校验。

**2026-08-01 已收口的已创建 Workspace 更新入口：**
`PUT /api/v1/workorch/workspaces/:workspaceId/execution-contract` 只接受
`contextHints` 与 `evidencePolicies`，经 application 的 Workspace owner read/update command
持久化该 workspace 的完整合同快照。HTTP client、remote read DTO 与 Web “编辑执行合同”均保留原始声明；接口不含
Run ID、activation、lifecycle 或 target-state 字段，不读写 Run aggregate、不调度 Drive、不请求 LLM。保存后的
声明只可通过下一次 explicit `restart` 封存；`resume` 必须继续使用当前 Run 的 sealed contract。对应回归覆盖 owner
持久化、活动 Run 不变、HTTP typed-context 注入与 client wire contract。

1. File 只报告 owner-grounded artifact、target、selector 和 workspace revision；Shell 只执行 sealed recipe 并报告
   exact executed/passed/failed checks、metric 与 artifact；MCP 只报告 sealed behavior probe。三者均不得修改 CWI。
2. Git 只提供 repository/revision/commit owner fact；Git evidence 不能替代 LSP behavior、validation 或 artifact proof。
3. Orchestrator SM-2 只以 sealed binding + sealed operation + matching owner receipt 逐字段派生 credit。`current_run`
   reconciliation 只 retain/reopen 当前 credit；`workspace_full` 不继承 predecessor credit。
4. 缺 target、recipe、check、metric、artifact 或 owner behavior probe 时 owner 返回 factual
   `unverifiable`/`insufficient`/`stale`；Orchestrator 依既有 internal event 分类 prerequisite 或 typed terminal，不创建
   decision ticket 作为合同修复手段。

**门禁：**

- 删除 LSP artifact/config/docs 后 resume 精确 reopen 关联 CWI。
- generic build/test 只能作为其被封存的单一 check 的 receipt，不能扇出关闭异构 CWI。
- LSP Bridge 的每项 required behavior 至少有一次 binding-matched deployed receipt；缺任何一项不得 complete。

### Phase C — activation outcome 与产品反馈归一

**主要组件：** `application/workorchd/biz`、`service/server`、`workspaceviewcore`、
`internal/services/workorch` TUI/backend、Web console、`internal/pkg/workorchclient`。

1. 维持 HTTP transport `202` 与 owner outcome 的区别；projection 必须显示 `queued`、`owner_accepted`、
   `owner_rejected`、`replacement_pending`、`reconciliation_requested` 中的真实后继状态，并绑定 requestID/workspaceID/runID。
2. 将 activation compile failure 映射为稳定 category + 保真 diagnostic；错误包含完整 CWI preflight report。
3. 推荐动作只能根据 owner outcome 的有限表生成：没有 active Run 的 `coverage_binding_unavailable` 只能引导
   “修复 profile/definition 后 restart”；不得推荐 resume。
4. 删除 `internal/pkg/workorchclient/run.go` 中指向已删除 Run create/start/resume HTTP 写路由的 client facade，或改为
   仅提供 workspace `/execution-activations` typed client。不得保留失败兼容 API。
5. TUI/Web 只渲染 projection/outcome；不得把 `202`、queued、generic error text 或 view state 反向投递 lifecycle/drive。

**门禁：**

- `restart` preflight fail 在 TUI/Web 显示真实 category、full diagnostic 和可执行下一步，且不显示 running。
- no-active-run resume 显示 `restart_required`；definition drift resume 显示 `definition_changed_requires_restart`；二者均零 drive/LLM/effect。
- 删除 client 写 facade 后不存在对 `/workspaces/:workspaceId/runs` POST、`/start`、`/resume` 的 production 调用。

### Phase D — owner transition、recovery 与 scope 回归

**主要组件：** `blocks/workspace`、`blocks/run`、`kernel/orchestrator`、`blocks/result`、
`decision`、`approval`、`kernel/governance`、`eventstream`。

1. 为所有 SM-1/2/3/4 entry 复核 source allow-list、checked workspace context、workspace/run lane/singleflight/lease key
   和 owner-private internal event。任何不匹配 source/scope/identity 只能拒绝，不能产生 CAS。
2. 保持 SM-4 可写范围仅为 lease/recovery/delivery acknowledgement；重驱动仅从 persisted aggregate 读取，不得选择
   focus、CWI、candidate 或 target。
3. terminal delivery 固定 `Result -> Workspace -> Run`，每个 owner 先验证 sealed identity 再进行本地 reducer/CAS；
   单边 delivery failure 只形成 pending lease。
4. 清理 stale plan、readme、projection 字段和测试 fixture 中任何“application direct lifecycle 仍未收口”的表述。

**门禁：**

- forged source、workspace mismatch、stale/duplicate receipt、CAS response loss、crash restart 均不造成跨 owner state write 或
  durable operation replay。
- `restart` predecessor supersede 后 replacement 只被激活一次；active Run resume 保持同一 Run。
- terminal delivery retry 不改变 sealed SM-2 terminal；无 revision 进展只保留 bounded persisted successor，零 busy loop。

### Phase E — fresh deployed 验收

**主要组件：** 全部 22 个已注册运行单元；其中未在 Phase A-D 改代码的能力 owner 只承担回归验证，不新增状态机。

| 场景 | 必须断言 |
| --- | --- |
| empty LSP workspace + restart | 编译完整且可执行的 CWI contract；首个 owner-grounded artifact 有 durable receipt；不存在 generic CWI credit |
| active Run + resume | 同一 Run、`current_run` reconciliation、无 replacement；真实 owner outcome 可见 |
| completed/terminal predecessor + restart | replacement Run、`workspace_full` reconciliation、零 predecessor credit inheritance |
| definition drift + resume | `definition_changed_requires_restart`，零 Run/drive/LLM/effect |
| invalid profile proof + restart | complete preflight diagnostic，零 Run/LLM/operation/ticket/effect，下一步不得建议 resume |
| duplicate/stale/CAS/crash | request/generation/lease 幂等，既有 receipt 复用，不重放 durable operation，不丢 lineage |
| terminal delivery failure | Result、Workspace、Run 各自独立 ack/retry；sealed terminal 不被改写 |
| two workspaces concurrent | scope、requestID、singleflight、lease、projection、receipt 完全隔离 |
| CPU guard | repeated schedule/CAS/pending 不出现无 lease 循环或持续高 CPU |
| LSP Bridge semantic matrix | 十工具、五语言、unsaved edit、bounded output、性能/并发、build/race、config/docs 各有独立 binding-matched proof |

执行顺序：相关 package tests → `make fmt-check` → `make vet` → `make verify-boundary-contract` →
`go test -count=1 ./...` → `go test -race -count=1 ./...` → `make verify-ci-mvp-file` → fresh deployed matrix。
任一 deployed 条件未完成，本计划保持 active。

## 7. 22 组件责任矩阵

| 组件 | 本计划中的职责 | 是否应新增状态迁移 |
| --- | --- | --- |
| `internal/initiators/workorchruntime` | 基础设施接线与 scoped runtime factory | 否 |
| `internal/modules/application/workorchd` | 纯 bootstrap 编译、typed intent/outcome、HTTP/projection adapter | 否 |
| `internal/modules/blocks/workspace` | Workspace slot 的 owner-private transition | 仅自身 slot |
| `internal/modules/blocks/run` | SM-1 owner-private activation/replacement/terminal transition | 仅 SM-1 |
| `internal/modules/kernel/orchestrator` | SM-2/SM-3/SM-4 internal reducers、binding credit、lease/delivery | 仅 SM-2/3/4 各自字段 |
| `internal/modules/blocks/file`、`internal/modules/blocks/shell`、`internal/modules/blocks/mcp` | binding-grounded observation/receipt | 否 |
| `internal/modules/blocks/git`、`internal/modules/blocks/llm`、`internal/modules/blocks/ssh`、`internal/modules/blocks/web` | 能力 owner fact/receipt/candidate document | 仅各自技术资源状态 |
| `internal/modules/blocks/result` | Result owner-private terminal delivery | 仅自身 lifecycle |
| `internal/modules/blocks/decision`、`internal/modules/blocks/approval`、`internal/modules/kernel/governance` | governance resource/resolution fact | 仅自身治理状态 |
| `internal/modules/blocks/eventstream`、TUI/Web projection | 审计和只读呈现 | 否 |
| `internal/modules/blocks/memory`、`internal/modules/blocks/skill`、`internal/modules/blocks/soul`、`internal/modules/blocks/mcpregistry`、`internal/modules/kernel/system` | 配置、资源或基础系统 owner | 仅自身资源状态 |

## 8. 文档与清理规则

1. 本文替代旧的 SO-01～SO-07 “均未完成”叙述；已关闭的 lifecycle/source/outcome/SM-4 ingress 项只保留回归基线。
2. 同步更新 `docs/70-roadmap/active/overview.md`、`docs/01-overview/current-unclosed-matrix.md`、受影响组件 readme
   与测试。归档文档不能重新定义状态机或实施队列。
3. 不新增 compatibility/migration；旧 schema 仍为 `fresh_activation_required` + explicit restart。
4. Phase A-D 均 code-closed 且 Phase E 通过后，方可将本文移入 `docs/90-archive/completed/`。
