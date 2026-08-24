# Runtime Invariants And Change Gates

Status: normative

Type: core-contract-part

Last Verified: 2026-08-24

## 跨状态机不变量

1. SM-1 不选择 action；SM-2 不执行 operation；SM-3 不选择下一 action；SM-4 不重建 authority，亦不修改别的 owner state。
2. 每次跨状态机交互都携带 workspace/run scope 以及适用的 definition/evidence revision、authority
   snapshot、dispatch/operation identity。
3. 正式事实必须先持久化，后产生外部可见 projection；projection 可最终一致，authority 不可反推。
4. candidate、dispatch、governance resolution、evidence commit、terminal delivery 都必须幂等。
5. provider target、definition 与 activation 在 Run 内封存；变化需要新的 typed activation，不能由 Resume
   patch。
6. contract/invariant violation 必须形成 typed diagnostic/failure 并沿以上方向收敛；不得旁路某个状态机
   修补正式状态。
7. recoverable runtime wait 必须携带 typed reason、recovery condition、trigger/deadline、budget 和 persisted successor；
   它不创建 SM-1 state、Decision/Approval 或 application scheduler authority。
8. execution-budget view 只能从 owner/Runtime sub-ledger 纯派生；allocation 先于 physical call/effect，unknown consumption
   不得被解释为 zero。
9. `target_grounding_unresolved` terminal delivery 必须同时满足 canonical terminal 与 current semantic-authority readiness；
   binding-free semantic generation pending 时只能请求 semantic plan，不能投递该临时终态或请求 operations candidate。
   其它 canonical terminal 不得因此被改写或重规划。

## 中立 effect、evidence 与治理

| Effect scope | 可关闭 durable state change |
| --- | --- |
| `none`、`ephemeral_session`、`governance_only`、`unknown` | 否 |
| `workspace_durable`、`remote_durable` | 仅在 RequiredOutput 接受、owner receipt 证明且 sealed binding 逐字段匹配时 |

`need_state_change` 的 sealed candidate 至少含一个与 RequiredOutput 兼容的 durable operation；
`need_validation` 只能含 `none` 或显式允许的 `ephemeral_session` operation。assessment 只消费当前
Run 的 accepted evidence；assessment-only focus 不执行 executable operation。

operation-scoped governance必须保存恰好一个ticket/approval owner identity，以及resolution、dependency、subject、
receipt hash、ExecutionKey与fence binding。continue以同一ExecutionKey的下一fence执行，owner必须preflight exact grant；
persisted grant与query/caller grant必须同时缺失或逐字段完全一致，不能通过省略grant绕过治理。

validation recipe identity与check identity必须分离。required check只有在owner receipt中真实passed，且不同时出现在
failed/skipped时才能贡献credit；exit 0、recipe ID、非空PassedChecks或冲突outcome均不能替代matching check fact。

state-changing capability operation 的 receipt 必须由 owner 对真实 effect 做有界观察。对 workspace durable Shell
operation，真实 changed refs 必须非空且属于 sealed `RuntimeOperation.TargetRefs`；存在未声明 changed ref、只有
unknown effect 或 zero-work 时不能形成 accepted durable evidence。owner observation 只返回 typed receipt，不得
解释 Goal/DoD 或选择下一 Runtime action。

remote/network durable capability同样必须返回owner校验的changed/no-op、before/after revision与bounded resource changes，
不能把sealed expectation或catalog declaration回显为observed effect。没有end-to-end idempotency key的外部request必须
发布`at_most_once`；提交后receipt丢失进入`outcome_unknown`，不能以receipt query miss重复请求。

## LLM request、cache 与上下文

LLM 是不可靠的 protocol client，不是 authority。请求必须由 sealed typed context 渲染：

- C0：stable runtime/schema；C1：current immutable Definition partition exact text；C2：declarative capability/governance contract；
  C3：current focus/required output；C4：failure/evidence/diagnostic；C5：有界 ref。
- P0–P4 的必要字段缺失时不得提交 provider。raw prompt、raw provider body、credential、endpoint、
  snapshot content 不得进入 aggregate、audit、projection 或 memory。
- unexpected stable-prefix drift 是 renderer defect。完整 request/cache 规则以
  [runtime-request-cache-note.md](../runtime-request-cache-note.md) 为准。

## Execution budget

provider/capability consumption、candidate/semantic generation、repair/reopen 和 recovery 使用独立 logical identity 与
persisted ledger；duplicate、stale、CAS reload、projection refresh 和 accepted resource invalidation不得重复或错误扣减。
统一 allocation、unknown-consumption、wait/exhaustion、restart 和 projection合同见
[Execution Budget](execution-budget.md)。

## Current persisted-data baseline

当前阶段只维护一套持久化结构，不维护历史 aggregate、resource、event 或 repository layout 的 migration/reducer。
`RuntimeStateRecord`、`RuntimeBootstrap` 与 `Run` 没有用于选择历史解码分支的顶层版本字段；持久化 JSON 必须只包含
当前字段且只能包含一个值。未知/退役字段、旧目录、空文件、旧 payload 表示或当前不变量缺失都在 owner/repository
读取边界直接失败，不生成 Run fact、projection disposition、governance ticket 或 recovery lease，也不调用 provider/capability。

- `fresh_activation_required` 只表示一个可读的 current Run 缺少其 sealed `RuntimeBootstrap`，不得用于分类旧数据；
- 数据清理是部署者删除所配置 data root/workspace data 后重新创建 Workspace/Run 的外部操作，不是 SM-1 transition；
- fresh Run 只从 current bootstrap 开始，不继承既有 request、candidate、CWI credit、receipt 或 recovery budget；
- convergence、recovery 与 execution-budget policy 必须由 fresh activation 显式封存；normalize/clone/read 不补零值、
  不从其他 policy 派生缺失 policy，任一缺失都按无效 current data 失败；
- nested schema identity 只校验当前 typed document/ledger，不授权版本协商、默认值补齐或原地升级。

## Forbidden surface

以下路径已从生产与现行规范中删除：`StagePacket`、`NextAction`、`DispatchNextStep`、
`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application Start/Resume queue、
自动 replan successor，以及 checkpoint replay。

这些结构不得用于 DTO、EventHub 合同、状态迁移、配置或 prompt。

## 变更门禁与文档纪律

所有 Runtime 变更必须：

1. 标明影响的 SM-1～SM-4、owner、persisted fact、合法/禁止迁移；
2. 为每个 owner-defined typed command/result 添加 producer/consumer 回归；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失和 fresh-process reload；不可达语义必须
   由测试证明；
4. 覆盖预算/guard 与零副作用断言：在 prerequisite/authority 失败时不得产生 provider request、attempt、
   dispatch、ticket 或 effect；
5. 同步更新当前事实、对应 module readme、flow 与稳定 verification contract；
6. 通过 `make verify-boundary-contract`，确认没有将状态迁移放入 `pkg`、没有 forbidden surface、没有让
   projection/TUI/Web 成为 authority。

`docs/` 不保存实施顺序、closure diary 或 live 热修计划。若需要跟踪工程工作，应使用外部 ticket/issue；最终合同只在本规范树
维护一份。发现第二份 Runtime 状态迁移、graph/step continuation 或自动 successor 设计时，必须直接作废并删除，而不是并列保留。
