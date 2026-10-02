# WorkOrch Product Architecture

Status: normative

Type: product-architecture

Last Verified: 2026-08-25

## Product Interaction View

```text
user through TUI / Web / API
  -> edit Workspace candidate or submit owner intent
  -> workorchd composes typed use case
  -> sole owner validates and persists
  -> Runtime/capability/governance owners converge
  -> owner facts become projection
  -> user observes progress, blocker and formal Result
```

本图描述产品交互，不定义代码层或状态机。framework unit、1+4+17 与依赖方向见
[Module Architecture](../30-module-architecture/overview.md)；SM-1～SM-4 见 [Runtime Core](../00-core/overview.md)。

## User-Owned Inputs And Observable Resources

| Product resource | User action | Authority result |
| --- | --- | --- |
| Workspace Definition | 提交 Template、Goal、Resources、可选用户验收要求和 directory；精确确认派生与审查结果 | Workspace-owned immutable admission candidate and promoted startup input |
| Execution | submit `resume|restart` intent | Run owner lifecycle/replacement plus Orchestrator convergence |
| Governance | answer Decision/Approval | corresponding owner resolution; no client auto-Resume |
| Context resources | manage Soul、Skill、Memory、MCP registry and model selection | each resource owner revision/snapshot |
| Progress/outcome | inspect three-axis coverage/evidence/assessment、wait、budget、artifact、Result | read-only derived views of owner/aggregate facts |

User text、UI state、HTTP response、Eventstream and projection never become Runtime authority by themselves。

## Fresh Execution

1. 用户提交 Template、Goal、Resources、可选用户验收要求和 directory；Workspace owner 持久化创作输入，解析默认目录。
2. 系统派生 DoD 并完成六类独立审查；用户精确确认当前 revision、input、permit、proposal 和 specification 身份后，
   才物化正式 Goal/DoD 和 pre-seal candidate。
3. Pure admission 仅接受完整 `all_of`；显式歧义产生阻塞 clarification ticket，不创建 Run。Workspace owner promotion
   封存 startup input；修正后的 ticket lineage 只有经 Decision resolution 和 Workspace acknowledgement 才具备执行资格。
4. 用户另行显式提交 execution intent；workorchd 从 eligible owner state 组装 typed bootstrap。
5. Run owner activates SM-1；Orchestrator drives SM-2/3/4；capability owner executes only sealed operation。
6. Orchestrator accepts matching evidence and seals terminal；Result→Workspace→Run owners acknowledge in order。

创建、编译和确认均不自动执行。TUI 不提供创作 wizard，也不接受已退休的直接定义写入。startup seal 后允许 bounded
Runtime LLM processing，但不得改写启动输入、选择 action、授予 credit 或裁定 terminal。

执行期 semantic processing 若发现隐含 one-of、undeclared prerequisite 或 Definition ambiguity，只能形成 Run-scoped
runtime-definition clarification ticket。受影响 focus 在新 operation 前停止；客户端不能回答该票据以继续 predecessor。
用户必须提交修正后的 all-of candidate 并显式 Restart，随后由 Run/Decision owner 的专用 replacement command/result
把 ticket 与 `ReplacementIntent` 绑定。current Run 不扩图、不修改 Definition，也不吸收修正后的 revision。

运行中的不可推进状态必须对用户明确区分：真实外部选择/授权进入 governance wait；具有 exact condition、trigger/deadline
和 budget 的暂时条件进入 recoverable runtime wait；其余合同、guard 或预算不安全情况进入 typed failure。客户端不提供
clear wait、force retry、budget top-up 或用普通 Resume 解除这些状态。

## Resume And Restart

`resume` only continues the Workspace-selected active/recoverable Run from its original aggregate、Definition and sealed
provider/context selection。Caller does not supply revision、next action or authority patch。

`restart` requires a fresh eligible startup input。Run owner persists predecessor `ReplacementIntent`；only after predecessor
terminal delivery is complete may its private reducer create the successor。The replacement is a same-owner SM-1 subordinate
transaction. Orchestrator's `RuntimeTerminationRecord` is only an SM-4 delivery handoff; only a Run-private SM-1 event changes
the predecessor lifecycle。Application/projection does not queue、infer or pre-create successor。

成功不是 operation、artifact 或全部 CWI 执行结束的别名。产品 Result 必须逐 requirement/CWI 展示独立的 coverage、evidence
和 assessment，并引用 current accepted evidence 与实际依赖的 owner revision set；required item 的 `indeterminate` 不能进入
successful closeout。Result owner只保存 Orchestrator 已裁决的密封 closeout。

## Read And Write Semantics

- write：interface → workorchd use case → receiving-owner concrete EventHub command → owner-local event/repository；
- read：owner facts/reads → workorchd detached projection/SSE → interface；
- Eventstream is audit only；projection can rebuild/cache but cannot repair formal state；
- command acknowledgement means accepted for processing；final progress/outcome comes from owner projection；
- each interface exposes the same business semantics regardless of local/remote transport。
- every state transition belongs to exactly one owner and is triggered through that owner's private internal event; cross-owner
  commands carry intent/fact only, never a target state。
