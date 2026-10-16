# Bound CWI Suspension And Ordinary Revalidation

Status: normative

Type: core-contract-part

## Scope And Ownership

`runtime/cwi-suspension/v1` 是 Orchestrator SM-2 的有界暂停来源合同，不是第五状态机、新 owner、
LLM 调度指令或新的执行权限。SM-1 不变；SM-3 保留原 binding、dispatch、execution 与 receipt；
SM-4 只从原 aggregate 重驱。所有提交仍经原 owner-private event 和同一个 aggregate CAS。

## Admission

只有真实 stale owner dependency observation 精确重开一个原已结算 direct predecessor，且后继仍持有
唯一 current bound binding 和已完成 durable effect，才可以暂时离开后继焦点。暂停前不得有 active dispatch、
未结算 submitted provider、unknown execution、administrative termination 或 immutable terminal seal。
候选、dispatch、receipt、预算和 binding 历史不能删除、重绑、退款或从投影重建。

首版只支持一个已声明 `validation-only + assessment` 的 ordinary predecessor，且它已 eligible；
不猜选多个来源、不嵌套暂停、不把 state-change-only 或历史后像提升为当前验收。无普通来源、无原 binding/effect、
unknown 或容量/repair exhaustion 必须明确以 `cwi_revalidation_source_unavailable` 失败，
不能再请求 provider 或重发已完成 effect。其它 reopen 原因不能借用本 lane。

## Durable Pause Source

`RuntimeFacts.CWISuspensions` 最多 16 项，至多一项未恢复。原字段一经封存不可变：
版本、暂停 revision、完整 frozen focus/current binding、原 effect evidence、唯一 predecessor 与原 reopen lineage、
repair debt/directive/failure、被退休的 exact focus-scoped prerequisites 和 owner observations。
source 从原 accepted semantic/owner preimage、completed execution/receipt/evidence、direct edge 和真实 stale observation
反向核验。自报 hash、旧 satisfied 标签、文本或缓存不提供来源。

暂停记录不授予当前资格。被暂停 focus 的 target selection/snapshots、feasibility 和 failure target 清除；
其当前 owner observations 退休。前驱沿原 Definition 普通 proof transaction 获取新 binding、真实 owner receipt、
独立 all-of assessment，并对新 evidence revision 完成本轮 fresh owner sampling。
暂停期间 planning 的 eligible 集合只含精确前驱；其它 ready CWI 不能越过该安全恢复事务。

## Resume

只有 exact predecessor 的新 binding 不等于被重开的旧 binding，且全部原准则和 direct prerequisites 已真实 current
settled/satisfied，才恢复原 frozen successor。resume 只封存新 binding、assessment、evidence revision 与 observation prefix；
完整来源由原 assessment dispatch/adjudication 与实际选中 ordinary proof 反证。未来观察不能补成旧 resume proof。

恢复原 repair debt，不重置 provider、capability、proof、durable-dispatch 或 recovery budget，不重发原 effect。
旧 owner qualification 不能恢复；exact retired prerequisite ID 可从当前 retirement index 移除以重新获取，
其完整旧正文仍在不可变暂停记录保全，不把它再次放入 Satisfied/current observations。
暂停/恢复任一步来源缺失、漂移、stale、unknown、歧义或超预算都 fail-closed；不改变 startup input 或原准则。

## Verification Boundary

私有 transition 必须独立加载持久前像，核对原 focus/repair/owner archive 与完整写集；暂停不夹带 receipt/effect/budget
变化，resume 不改原记录。CAS 前失败与 CAS 后响应丢失、同版本 JSON reload、duplicate、来源替换、old credit、
unsafe window、unknown 和容量 guard 均需回归。真实 File/Shell/Run/Workspace/Result 与 scripted provider 主链
必须证明 single durable effect、真正 stale、普通重验、新 binding、原 budget 与有序三 ACK。
脚本 provider/owner-object 重建不是 fresh OS-process、公开 authoring fullflow 或 live 验收。
