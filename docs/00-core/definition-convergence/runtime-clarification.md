# Runtime Definition Clarification Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-18

## Scope

本合同只定义 startup input 与 Run 已封存后，由 bounded semantic processing 暴露的 implicit semantic gap。
它与 pre-seal Workspace admission ticket 使用不同 scope、command/result 和恢复动作，不能用 optional RunID
或普通 governance request 合并。

## Runtime-discovered semantic clarification

startup input 与 Run 已封存后，SM-3 的 bounded semantic document 可以返回
`definition_clarification_required` factual result，指出 current item/ref 可能存在 `implicit_one_of |
undeclared_prerequisite | semantic_ambiguity`。这只是 provider original diagnostic，不是 relation、Runtime action 或 ticket
command；LLM 不携带或生成 WorkspaceID、RunID、authority、ticket、replacement intent 或 target state。

1. SM-3 先把 complete document 与 original diagnostic 持久化；Orchestrator 从 current aggregate 派生并绑定
   `WorkspaceID + RunID + DefinitionRevision + StartupInputHash + AuthoritySnapshotID + FocusRef + LogicalRequestID`，形成
   typed `RuntimeDefinitionGapObservation`；
2. SM-2 只接受 exact-current scope、manifest-authorized offending item refs 和完整 document hash。stale/mismatched/duplicate
   observation 不能建票、改 graph 或增加 revision；不要求 Orchestrator 解释领域文本，**不能证明 all-of 完整**本身就是
   fail-closed clarification 事实；
3. SM-2 进入 canonical `need_governance`，SM-3 通过唯一 candidate/sealed-dispatch transaction 持久化专用
   `RuntimeDefinitionClarificationDispatch`。它不是 operation candidate，也不是普通
   `ExternalDependency/GovernanceRequirement`；dispatch 必须绑定 gap observation、current authority、startup input、focus、
   semantic request/document 和 deterministic gap signature；
4. Orchestrator 只能在上述 dispatch 已封存后，通过同步 EventHub `Send` 向 Decision owner 的 `pkg/events` 提交 concrete typed
   `OpenRuntimeDefinitionClarificationRequestCommand` 并取得 concrete
   `OpenRuntimeDefinitionClarificationRequestResult`。该合同固定为：

   ```text
   OpenRuntimeDefinitionClarificationRequestCommand
     SchemaVersion
     TicketID
     WorkspaceID
     RunID
     DefinitionRevision
     StartupInputHash
     AuthoritySnapshotID
     ClarificationDispatchID
     FocusRef
     LogicalRequestID
     SemanticDocumentHash
     GapKind = implicit_one_of | undeclared_prerequisite | semantic_ambiguity
     OffendingItemRefs[]
     CanonicalGapSignatureVersion
     CanonicalGapSignature
     CanonicalGapSignatureHash
     RequiredActionKind = revise_definition_and_restart

   OpenRuntimeDefinitionClarificationRequestResult
     TicketID
     WorkspaceID
     RunID
     AuthoritySnapshotID
     ClarificationDispatchID
     Disposition = created | duplicate
     ClarificationRequestBindingHash
   ```

   command/result 不含 `ContinueResolutionType`、`FailResolutionType`、operation candidate、`ExternalDependency`、target state 或
   map payload；不得复用 `OpenRuntimeGovernanceRequestCommand`。LLM 不能直接提交该 command，Workorchd/Application 也不能伪造
   authority/dispatch；
5. canonical gap signature 由 version、gap kind、exact startup/authority/dispatch/focus/logical-request/document identity 与排序去重后的
   offending refs 组成；不得包含 raw provider text、credential、endpoint 或未脱敏正文。ref 数量、canonical bytes 和 safe diagnostic
   必须受 `RuntimeDefinitionClarificationBudget` 约束，超限在 Decision `Send` 前返回 typed guard failure，零 ticket/CAS；
6. `TicketID` 由 `WorkspaceID + RunID + ClarificationDispatchID + CanonicalGapSignatureHash` 确定性派生；Decision owner 必须校验
   canonical signature bytes、派生 identity 和 exact command binding。result 的 `ClarificationRequestBindingHash` 覆盖完整 normalized
   command；Orchestrator 只有在逐字段与 binding hash 匹配后，才可通过 private internal event/CAS 把 typed clarification request
   record 绑定回 persisted dispatch。ticket 由此绑定 current Run、Definition revision、StartupInputHash、authority snapshot、focus、
   semantic request/document 和 deterministic gap signature；同一 lineage 重驱返回相同 `BlockingTicketID`，不得重复 provider
   request 或 ticket write。若 crash 发生在 Decision ticket CAS 后、Runtime request record CAS 前，re-drive 只重发 exact command，
   接收 duplicate result 后补齐同一 record；
7. 从 observation 被接受开始，不得为 affected focus 创建 operation candidate/dispatch/effect；较早已封存的其它 focus effect
   保留审计事实，但不能因此把 gap 视为满足。current Run 不扩图、不重编译 Definition，也不从 provider 文本自动 replan；
8. ticket 唯一恢复要求是 `revise_definition_and_restart`。用户通过新的 pre-seal candidate、all-of admission 和 Workspace
   promotion 取得新 Definition revision/startup input 后，ticket 仍不允许 `continue` 或 Resume；用户必须显式提交 Restart；
9. Run owner 验证 predecessor 当前正被 exact clarification ticket 阻塞、新 bootstrap 绑定新
   `DefinitionRevision + StartupInputHash` 且与旧 startup input 不同后，先 CAS 持久化 matching `ReplacementIntent`。该 intent
   必须新增并封存 `ClarificationTicketID + PredecessorStartupInputHash + SuccessorStartupInputHash`；在 Decision owner
   acknowledgement 前状态为 `pending_clarification_resolution`，不得开始 administrative supersede/terminal delivery；
10. intent CAS 成功后，Run owner 通过同步 EventHub `Send` 向 Decision owner 提交：

   ```text
   ResolveRuntimeDefinitionClarificationByReplacementCommand
     TicketID
     WorkspaceID
     PredecessorRunID
     ReplacementIntentID
     PredecessorDefinitionRevision
     PredecessorStartupInputHash
     SuccessorDefinitionRevision
     SuccessorStartupInputHash
     ReplacementIntentHash

   ResolveRuntimeDefinitionClarificationByReplacementResult
     TicketID
     WorkspaceID
     PredecessorRunID
     ReplacementIntentID
     ResolutionID
     ResolutionType = definition_replacement_intent_bound
     Disposition = resolved | duplicate | stale
     ConflictKind = none | ticket_already_bound | scope_mismatch
     CurrentReplacementIntentID      optional; required for ticket_already_bound
     CurrentReplacementIntentHash    optional; required for ticket_already_bound
     ResolutionBindingHash
   ```

   该 command/result 由 Decision owner `pkg/events` 定义；只有已经持久化 matching intent 的 Run owner 可以调用，
   Application、Workorchd、Orchestrator、LLM 或 projection 均不得代发；
   Decision owner 只校验自身 ticket binding 与 Run-owner persisted fact identity，不读取 Run/Workspace repository；matching
   command 以 owner-local CAS 收口 ticket，duplicate 返回同一 resolution，stale/mismatch 不写。该路径不向 Orchestrator 提交
   `RuntimeGovernanceResolution`，绝不产生 `RuntimeContinuationEligibleFact`，也不把 predecessor 恢复为 active；
   `definition_replacement_intent_bound` 只表示 ticket 已绑定一个持久化且可恢复的 replacement intent，不表示 successor 已创建；
11. Run owner 逐字段校验 ticket/workspace/predecessor/intent identity、resolution type，以及覆盖 normalized command 与 result
   identity/type/disposition 的 `ResolutionBindingHash`；收到 matching `resolved|duplicate` result 后，CAS 将 intent 标为
   `clarification_resolution_bound`，随后才可进入既有
   administrative supersede 和 predecessor `Result -> Workspace -> Run` delivery；只有 delivery 完成后才创建 successor。successor 使用新
   startup-input contract，不继承 predecessor scheduling contract、binding、credit、LLM document 或 context expansion；
   predecessor 的既有 owner effects 只能由 successor reconciliation 重新观察，不能直接继承 credit；
12. `stale` 不是可重试成功。`ticket_already_bound` 必须返回 Decision 已封存的 current intent identity/hash：Run owner 若有
   matching persisted intent，直接恢复该 intent 的 acknowledgement；若 current intent 不同，则 CAS 退休本次 intent 并恢复已绑定
   intent。`scope_mismatch` 必须把本次 intent CAS 为 `clarification_resolution_rejected` 并返回 typed conflict；只有用户再次显式
   Restart、且 Run owner 已退休 rejected intent 后才能创建新 intent。稳定 stale 不进入 retry budget，也不得永久停留在
   `pending_clarification_resolution`；
13. crash 发生在 intent CAS、Decision resolution 或 Run acknowledgement 任一边界时，SM-1/owner recovery 只重放 exact
   command/result identity；Decision 已收口但 Run 未 ack 时 duplicate join 后继续，Decision 未收口时 predecessor 保持 governed
   wait。ticket persistence、resolution delivery 或 replacement-intent binding 失败时走有界 recovery；
   不得降级为 operation、自动 Resume、自动 Restart、静默 terminal success 或无票据继续执行。

Runtime clarification 只复用 Decision/Run owner 与现有四状态机：它使用 canonical `need_governance` wait、SM-3
sealed-dispatch 和 SM-4 recovery/delivery，但必须走专用 clarification command/result，不复用普通 governance
request/resolution/continuation。与 pre-seal Workspace admission 的协调只通过新 admitted startup input 和 explicit Restart；
本合同不增加 Runtime 状态机、framework unit 或第二个 Definition owner。

## Related Contracts

- pre-seal explicit relation violation：[Workspace Definition Clarification](clarification.md)
- candidate、promotion 与 activation admission：[Definition Admission](definition-admission.md)
