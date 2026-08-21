# Workspace Setup Draft Session

Status: normative

Type: profile-capability

Last Verified: 2026-08-18

## Purpose

Setup draft session 是可选的 application/UI authoring aid，用于把多轮对话整理成候选 Workspace Goal/DoD。它不是
Workspace owner aggregate、Definition admission、Runtime activation、Run 或 recovery state machine。具体 HTTP 路由由
[Service Command API](../../50-product-interface/service-api.md) 定义。

## Draft Semantics

Draft session 是 process-local、可丢失的临时会话。它可以接收 Template、标题/描述、可选目录、dialog language、用户
消息和上一版草案，输出有界的：

- Goal 文本和 DoD items；
- assumptions、clarifying questions、warnings；
- confidence、needs-clarification、generation summary 与来源标记。

Template/Soul 内容可以作为有界生成上下文。LLM 输出必须经过 strict draft parse/normalization，且只更新当前临时
草案；它不写 Workspace、Goal、DoD、Decision 或 Run owner 状态。

普通 setup 可以完全手工 authoring。只有用户显式启用 assisted draft 时才调用 authoring LLM；这一选择不限制 startup
input 封存后的 Runtime LLM processing。

## Hierarchical Assistance

大量 Goal/DoD 的 authoring 采用有界逐级处理：

1. Template、用户分组、semantic role 和 requires/provides 形成确定性局部分区；
2. 组内请求读取 exact item text 并产生草案关系；
3. 跨组请求先比较 bounded manifest/hash，只为候选关系端点重新加载 exact text；
4. UI 展示依赖、priority/cost、disposition、假设和 cycle/missing warning；
5. 用户明确采用后才把选中字段复制到正式 setup form。

raw draft、数组/ID 顺序和自然语言先后关系都没有 authority。authoring schema/UI 不得提供 `one_of`/`any_of` 作为
合法 relation；provider 返回这些 token 时只能形成不可采用 diagnostic。deterministic compile 仍有歧义时必须进入
Workspace clarification，不能自动择支、改写为 all-of、要求 Runtime JSON 或绕过 blocking ticket。

## Adoption Boundary

采用草案与生成草案是两个独立动作：

```text
temporary draft
  -> user selects and applies fields to setup form
  -> Workspace owner persists DefinitionAdmissionCandidate
  -> pure all-of admission
  -> admitted promotion or blocking clarification ticket
  -> activation eligibility
  -> later explicit execution intent
```

只有 Workspace owner command 持久化的 candidate 才进入 admission。只有 exact-current `admitted_all_of` outcome 可以
promotion 为 formal Definition/startup input；草案 endpoint 不得自动 promotion、Start、Restart 或创建 ticket resolution。

## Runtime Boundary

Draft session 不包含或生成：

- `RuntimeBootstrap`、Runtime focus、target、recipe、check 或 evidence binding；
- provider request history、candidate attempt、repair 或 governance continuation state；
- Run ID、target Run state、replacement/successor identity；
- `executionContext` / `ExecutionContextPatch` 或 selected Memory/capability evidence。

用户采用草案、完成 admission/promotion 并显式提交 execution intent 后，workorchd 才从 canonical owner state 组装 typed
activation。它可以封存 Soul、Skill、MCP、Memory、provider selection 和 execution policy，但这些都不是 draft-session state。

## Interface Rules

- manual 与 assisted authoring 可以共享 editable form，但不得静默覆盖未采用的用户输入；
- UI 必须在“apply to form”前展示 proposal、assumptions、questions 和 warnings；
- draft 不得被显示为 formal Definition、accepted evidence 或 activation eligibility；
- admission violation 必须显示 typed code 与 `BlockingTicketID`，且不能由确认草案绕过；
- generation failure 可以回到 manual editing，但不得制造成功草案或 owner acknowledgement。

## Related Documents

- [Definition Admission](../../00-core/definition-convergence/definition-admission.md)
- [Workspace Clarification](../../00-core/definition-convergence/clarification.md)
- [Service Command API](../../50-product-interface/service-api.md)
- [TUI Interaction Contract](../../50-product-interface/tui/interaction-contract.md)
- [Built-in Default Resources](builtin-default-resources.md)
