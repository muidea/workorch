# Runtime V2 单 Owner 状态机统一收口方案

Status: active

Type: implementation closure plan

Plan ID: `RUNTIME-SMO`

Last Updated: 2026-07-28

## 1. 目的与唯一性

本文是 Runtime V2 当前唯一活动代码收口队列。目标不是让某个 workspace 绕过当前错误继续执行，而是统一
候选 document、状态转换、operation target、receipt 和恢复路径，使不稳定 LLM 输出始终落入有限、可持久化、
可重启的状态机结果：

> 一个可变状态域只有一个 owner；跨组件只传 typed intent、不可变事实、完整 document 或 receipt；状态转换只能
> 由 owner Biz 将输入转换为 owner-local internal event 后触发，并由 owner 的唯一 transition coordinator 持久化。

权威顺序：

1. `docs/00-core/runtime-stage-contract.md`：四个状态机的中立设计合同；
2. `internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract`：实现 SoT；
3. 本文：唯一未关闭实施队列；
4. `docs/01-overview/current-state.md` 与 `docs/01-overview/current-unclosed-matrix.md`：当前事实入口。

旧 `runtime-v2-validation-progress-closure-plan.md` 已迁入 `docs/90-archive/superseded/`，不得再定义实施顺序。
不恢复 Runtime V1，不保留兼容双路径，不按 workspace、错误文本或 provider 增加特判，不用 Resume、Decision、
提高 token/retry 预算或放宽 owner authority 修补非法 aggregate。

## 2. 2026-07-28 fresh deployed live 结论

### 2.1 验证对象

本次 C7 使用重新构建、重新部署且与工作树最新构建一致的 binary，以及全新 workspace：

| 项目 | 值 |
| --- | --- |
| WorkOrch binary SHA256 | `e24fd2fc44d3f4a368434d2a46fbe28bec9547382d8e49bd4e0f1c556201eae8` |
| Workspace | `ws-8974c4e22de0b2a4` |
| Run | `run-001` |
| Definition revision | `goal:goal-bcb65f7df55e9a41\|dod:dod-09fbc40e7bc23d76` |
| Provider target | `ai-proxy/deepseek-v4-flash` |
| Workspace directory | `.workorch-workspaces/runtime-v2-c7-a-20260728-1145` |

实例 `/readyz` 为 200，projection source/projected revision 一致。默认模型已 ready，sealed provider ceiling 为
context 1,000,000、max output 65,536。本次失败不是实例版本、provider readiness、TUI/Web、SSE 或投影滞后。

### 2.2 已验证的正向路径

fresh Run 已证明以下路径符合设计：

1. Application activation 经 Run owner 创建并激活 `run-001`；
2. SM-2 产生 `produce_state_change` authority；
3. SM-3 接受首个 candidate 并 seal dispatch；
4. File owner 成功执行 `file/write go.mod`；
5. execution fence 为 1，expected/observed effect 均为 `workspace_durable`；
6. owner receipt 被幂等提交为 `evidence_committed`，accepted evidence sequence 增至 1；
7. workspace tree revision 改变后，旧 dispatch 被 invalidated，相关 prerequisite 精确退休并重新获取；
8. terminal delivery 按 `Result -> Workspace -> Run` 完成三方 acknowledgement；
9. 全程没有创建无信息 Decision/Approval ticket。

因此 SM-1 activation、首个 SM-2/SM-3 state-change、File owner receipt、resource-change freshness 和 SM-4 terminal
delivery 已取得 live 正向证据，但不代表 C7 完成。

### 2.3 当前阻塞反例

第二次真实 provider 调用成功完成，audit 显示：

- provider status：`completed`；
- response bytes：422；
- request session 已标记 provider submitted；
- provider 没有 authentication、configuration、transport、timeout 或 output-limit 错误。

返回的完整 JSON 可以完成 wire decode，但 operation `op-001` 的 `patch/apply` 只提供了外层 `targetRefs`，未提供
重复的 `parameters.targetRef/path`。当前生产路径最终形成：

```text
provider_protocol_failure:
runtime candidate operation "op-001":
kind "patch/apply" targetRefs must contain targetRef ""
```

持久化结果进一步证明错误交接：

- 第二个 request session 被标为 `provider_protocol/provider_request_contract_error`；
- 第二份 422-byte document 未进入 Orchestrator artifact continuation；
- 第二份 document 没有 candidate attempt 0；
- aggregate 中只有第一份 accepted attempt；
- Run 直接进入 `failed/provider_protocol_failure`。

该结果不符合“complete candidate contract failure 必须由 SM-3 single evaluator 形成 rejection”的设计，C7 失败。

### 2.4 根因链路

确定调用链如下：

```text
ai-proxy complete response
  -> blocks/llm GenerateCandidate: transport success + complete bytes
  -> blocks/llm RequestCandidate: wire decode success，返回 Candidate + DocumentContent
  -> Orchestrator RequestRuntimeCandidate EventHub adapter
       -> 提前调用 Candidate.ValidateWithActivation
       -> semantic error 作为 Go error 返回，完整 response 被丢弃
  -> runtime_v2_mainline requestErr 分支
       -> provider_request_contract_error
       -> provider_protocol_failure terminal
  -> 未持久化 document，未进入 SM-3 attempt/rejection/retry
```

直接根因不是“合同校验存在”，而是同一 semantic validation 被放在错误边界：EventHub adapter 越过 SM-3 owner-local
evaluator 做了 candidate admission，并把普通 LLM 输出偏差升级为 provider 协议故障。

### 2.5 次生合同偏差

同一 operation target 当前存在重复表达：

- `RuntimeOperation.TargetRefs`；
- `parameters.targetRef`；
- `parameters.path`；
- 部分 operation 还有 `parameters.targetRefs`。

`patch/apply` contract 要求 `targetRef/path` 二选一且外层 `TargetRefs` 必须重复包含它；File owner 的执行 adapter
却已经可以从唯一外层 `TargetRefs` 推导 target。`file/write` 也已有相同 fallback，但 contract 形状并未统一。
这会让 LLM 在表达了正确 owner-grounded target 时，仍因遗漏重复字段而被拒绝。

## 3. 缺陷分类与处理原则

| 编号 | 缺陷类型 | 当前表现 | 处理方式 |
| --- | --- | --- | --- |
| D1 | 跨 owner 边界越权 | EventHub adapter 执行 candidate semantic admission | 全面修复：adapter 只验证 envelope/scope/identity |
| D2 | complete document 交接丢失 | semantic error 返回 Go error，raw document 未持久化 | 全面修复：完整 document 始终作为 typed result 交给 SM-3 |
| D3 | 错误分类污染 SM-2/SM-4 | candidate contract error 变成 provider protocol terminal | 全面修复：统一 outcome taxonomy 与 coordinator transition |
| D4 | target 合同重复 | 外层 target 与参数 target/path 双写 | 全面修复：一个 canonical target authority |
| D5 | evaluator 多入口 | LLM boundary、EventHub adapter、C5 分别校验 | 全面修复：complete document 只由 SM-3 evaluator 裁决一次 |
| D6 | 回归矩阵遗漏 | 局部测试全绿，真实 complete-semantic-invalid 未覆盖 | 全面修复：补 handoff、restart、attempt、target 矩阵 |

禁止以下单点处理：

- 只把错误文案从 `targetRef ""` 改得更友好；
- 只允许这一个 `patch/apply` payload 通过，但保留多套 target authority；
- 在 mainline 解析 error string 后改成 retry；
- 在 TUI/Web 收到 terminal 后自动 Resume；
- 把所有 candidate failure 都当 provider transient；
- 提高 candidate attempt、repair 或 provider transport budget；
- 保留旧 candidate EventHub 双合同或旧 target 参数作为兼容路径。

## 4. 单 Owner 与组件边界

### 4.1 状态 owner 矩阵

| 状态域 | 唯一语义 owner | 唯一允许的迁移位置 |
| --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | `blocks/run/biz` owner-local reducer |
| Workspace execution projection | `blocks/workspace` | `blocks/workspace/biz` owner-local reducer |
| SM-2 authority/focus | `kernel/orchestrator` | `kernel/orchestrator/biz/runtimev2` internal transition |
| SM-3 candidate/dispatch | `kernel/orchestrator` | `kernel/orchestrator/biz/runtimev2` internal transition |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | `kernel/orchestrator/biz` 输入适配 + `biz/runtimev2` internal transition |
| Decision lifecycle | `blocks/decision` | `blocks/decision/biz` owner-local reducer |
| Approval lifecycle | `blocks/approval` | `blocks/approval/biz` owner-local reducer |
| Result lifecycle | `blocks/result` | `blocks/result/biz` owner-local reducer |

SM-2、SM-3、SM-4 继续由同一个 Orchestrator Module 拥有，不拆成新 Module/Block。它们共享同一 Runtime
aggregate，跨 SM 交接由一个 Orchestrator-local coordinator 完成。Run owner 只通过 typed CAS command 持久化
opaque aggregate，不解释 candidate、dispatch、evidence 或 next action。

### 4.2 `internal/pkg/workorchruntimecontract` 边界

允许：

- DTO、枚举、sealed contract；
- document framing、strict decode、normalize、clone、hash、validate、compare；
- action/effect policy、operation descriptor、target cardinality等纯函数。

禁止：

- 读取 repository、执行 CAS、投递 EventHub；
- 分配 attempt、扣 retry/recovery budget；
- 决定下一 action、创建 dispatch 或 terminal；
- 保存 request session、内部 event queue 或任何 workspace mutable state。

纯 validator 可以被 SM-3 evaluator 调用，但 validator 自身不是状态机 owner。问题在调用位置与 transition owner，
不是把所有 validation 从 `pkg` 移走。

### 4.3 LLM Block 边界

LLM Block 只负责：

- sealed provider request rendering；
- provider transport、audit、usage、ceiling；
- bounded response bytes；
- empty/incomplete/complete document framing；
- continuation fragment 返回。

LLM Block 不负责：

- candidate kind、operation、target、coverage、effect 或 feasibility admission；
- candidate attempt/retry disposition；
- SM-2 failure/terminal；
- 选择下一 action；
- 把 complete candidate 内容错误当 provider failure。

## 5. Complete candidate document 唯一处理拓扑

### 5.1 新的唯一主链

```text
SM-3 persisted request session
  -> Orchestrator 发送 typed RequestRuntimeCandidateCommand
  -> LLM owner 返回且只返回四类 typed outcome之一
       1. CompleteDocument(bytes, audit, usage)
       2. IncompleteFragment(bytes, framing evidence, audit, usage)
       3. PrerequisiteStale(typed stale)
       4. ProviderDependencyFailure(typed auth/config/transport/empty/unknown)
  -> Orchestrator EventHub adapter 只校验：
       scope + transaction + logical/physical request identity + outcome互斥性
  -> CompleteDocument 必须先写 Orchestrator artifact store
  -> owner-local CandidateDocumentCompleted transition
       request session -> document_complete
  -> SM-3 single evaluator 从已持久化 document 读取
       strict JSON/schema/manifest decode
       candidate semantic validation
       action/effect/coverage/target/dependency validation
       rejected-shape validation
  -> owner-local CandidateAdjudicated transition
       accepted: attempt accepted + seal dispatch
       correctable_once: attempt rejected + sealed correction input
       rejected_final: attempt rejected + typed terminal proposal
  -> SM-4 只根据已持久化结果继续 drive
```

跨 owner result 中的完整 raw document 是唯一 C4 handoff。`RuntimeCandidate` 不再作为 LLM→Orchestrator
EventHub 响应中的权威值；由于当前阶段不考虑兼容，优先从该响应 DTO 删除 convenience `Candidate` 字段，避免调用方
再次对内存 candidate 做第二次 admission。Candidate 只在 document 持久化后由 SM-3 evaluator 生成。

### 5.2 document framing 与 candidate evaluation 分离

LLM Block 只需要判断字节流是否形成一个完整 JSON document：

| 输入 | LLM outcome | 是否分配 candidate attempt |
| --- | --- | --- |
| 空白/空响应 | provider empty dependency failure | 否 |
| EOF / unexpected EOF，明显未闭合 | incomplete fragment | 否 |
| 非空且 framing 完整，但 JSON/schema 不合法 | complete document | 是，由 SM-3 rejection |
| JSON/schema 合法但 candidate contract 不合法 | complete document | 是，由 SM-3 rejection |
| candidate contract 合法 | complete document | 是，由 SM-3 accepted/rejected admission |

`DecodeRuntimeCandidatePayloadWithManifest`、`ValidateWithActivation`、effect policy 与 feasibility 不得再出现在
LLM owner或 EventHub adapter 的返回门禁。若 continuation 需要判断未闭合 JSON，应提供独立纯 framing helper，不能
通过完整 candidate semantic decode 间接决定 provider outcome。

### 5.3 错误分类唯一矩阵

| 条件 | persisted domain | failure/rejection | candidate attempt |
| --- | --- | --- | --- |
| provider authentication | request session / SM-4 dependency | provider authentication | 不创建 |
| provider configuration/ceiling stale | prerequisite/request session | typed stale/configuration | 不创建 |
| transient transport | SM-4 provider transport ledger | provider transient | 不创建 |
| submitted outcome unknown | request session | provider submission unknown | 不创建 |
| empty response | request session | provider empty response | 不创建 |
| incomplete response | request session fragments | continuation或output exhausted | 不创建 |
| EventHub scope/identity/type不匹配 | runtime invariant/protocol | internal boundary defect | 不创建 |
| complete malformed JSON | SM-3 attempt | schema rejection | 创建 |
| unknown/forbidden field | SM-3 attempt | schema rejection | 创建 |
| operation/target/effect/coverage违例 | SM-3 attempt | contract rejection | 创建 |
| owner feasibility拒绝 | SM-3 attempt | feasibility rejection/stale/prerequisite | 创建或精确退回prerequisite |

`provider_protocol_failure` 只能表示 provider/跨 owner response envelope 不满足 sealed request identity 或互斥合同，
不能表示 complete document 的内容不符合 candidate contract。

### 5.4 attempt 0/1 与重启

- complete document 必须在 attempt 分配前持久化；
- `CandidateAdjudicated` 应在一次 coordinator transition 中写入 attempt final state以及 accepted dispatch或rejection；
- 不留下需要下一次调用补齐的无语义 `pending` attempt；
- attempt 0 `correctable_once` 才允许 attempt 1；
- attempt 1 再次 complete rejection 形成 `candidate_retry_exhausted`；
- `rejected_final` 不创建 attempt 1；
- attempt 1 保持相同 authority contract、transaction、manifest hash 与 stable prefix；
- retry C4 只增加 typed diagnostic/directive，不拼接 raw error或整份旧document；
- crash 可发生在 provider返回后、document写入后、document_complete CAS后、adjudication CAS前后；每个点重启均从
  persisted session/document/attempt恢复，不重放已知provider call，不重复分配attempt。

## 6. Operation target canonical 合同

### 6.1 唯一 authority

`RuntimeOperation.TargetRefs` 是 operation target identity 的唯一 canonical authority：

- manifest决定该 kind 的 target cardinality与owner-grounded policy；
- SM-3验证 `TargetRefs` 与 sealed owner inventory/selector；
- capability owner从 sealed operation `TargetRefs` 构造自己的 typed command；
- receipt的 target refs必须与 sealed operation相容；
- operation parameters只描述操作内容，不重复描述同一target identity。

不再要求 LLM 同时维护外层 `targetRefs` 与 `parameters.targetRef/path/targetRefs`。当前阶段不保留旧参数兼容分支。

### 6.2 各类 operation 规则

| operation 类别 | canonical target 规则 | 参数规则 |
| --- | --- | --- |
| `file/read`、`file/write`、`file/edit` | `TargetRefs` 恰好1项 | owner从外层target派生path；参数只保留内容/范围/替换数据 |
| `patch/apply`、`patch/preview` | `TargetRefs` 恰好1项 | 删除参数 `targetRef/path`；保留patchType/content/diff/old/new/baseHash等 |
| state-changing `shell/command` | `TargetRefs` 至少1项，可多项 | 删除参数 `targetRef/targetRefs`；command/args只描述执行负载 |
| non-mutating validation/session | `TargetRefs` 按evidence scope可为空或多项 | 不借用target声明workspace副作用 |
| `file/list` | path是query scope，不是durable target identity | 可保留明确命名的query scope参数 |
| Git/MCP/Web/SSH | 按descriptor声明资源/endpoint语义 | 不把host/url/server/tool/pathspec误当workspace target alias |

所有 operation descriptor 必须显式声明 `zero/one/many` target cardinality，不在 validator、renderer、File adapter
各自猜测。若某 kind 确需 owner payload 中的 target 字段，由 Orchestrator/capability adapter从 canonical `TargetRefs`
填充 typed command，不能要求 provider重复输出。

### 6.3 同源消费者

canonical target/effect policy 必须由以下消费者共同引用：

1. `RequiredOutputContract`；
2. candidate manifest和provider schema；
3. C0/C2 renderer instruction；
4. SM-3 strict evaluator；
5. owner feasibility probe；
6. sealed dispatch与execution command mapper；
7. receipt adjudication；
8. restart invariant与fixtures。

任何消费者自行维护另一份 target required/alias/fallback 表即视为合同漂移。

## 7. 四个状态机的影响与禁止转换

| SM | 本次影响 | 收口要求 |
| --- | --- | --- |
| SM-1 Run lifecycle | 无直接语义修改 | 保持 Start/Resume只提交Run owner intent；不能用Resume修补candidate失败 |
| SM-2 authority/focus | 当前被错误provider failure污染 | 只消费SM-3持久化的adjudication/fact proposal，不接收adapter error字符串 |
| SM-3 candidate/dispatch | 直接缺陷owner | 唯一document evaluator、attempt disposition、target/effect admission与dispatch seal |
| SM-4 drive/recovery/delivery | 当前把adapter error归为provider protocol terminal | 只处理typed request outcome和persisted SM-3 result；各预算独立 |

禁止：

- SM-4 根据 `error.Error()` 选择 SM-2 terminal；
- EventHub adapter 直接创建 rejection、attempt、dispatch 或 failure fact；
- LLM Block 选择 `correctable_once/rejected_final`；
- SM-2 在 active dispatch 存在时单独替换 decision；
- capability owner 修改 Runtime/Run/Workspace/Result 状态；
- projection/TUI/Web 从错误文本重建 authority 或触发隐式 Resume。

## 8. 组件级收口范围

### 8.1 P0：complete document 与 SM-3

| 组件/路径 | 修改内容 | 单组件退出条件 |
| --- | --- | --- |
| `kernel/orchestrator/biz/runtime_candidate_event_adapter.go` | 删除candidate semantic validation；只做typed envelope/scope/identity/互斥校验 | complete semantic-invalid document以typed result返回，Go error为nil |
| `kernel/orchestrator/pkg/events` | 收紧candidate response为complete document/fragment/stale/provider failure互斥union；删除权威`Candidate`字段 | DTO自身Validate覆盖全部非法组合 |
| `kernel/orchestrator/biz/runtime_v2_mainline.go` | complete document先持久化；只按typed outcome分流；禁止把candidate内容错误归类provider protocol | schema/contract错误均进入SM-3 attempt |
| `kernel/orchestrator/biz/runtimev2` | 增加/收紧`CandidateDocumentCompleted`与`CandidateAdjudicated` coordinator transition | attempt final state与dispatch/rejection原子、restart幂等 |
| `internal/pkg/workorchruntimecontract` | 拆分document framing、strict decode、semantic diagnostic纯helper | helper无I/O/CAS/retry/next-action |

### 8.2 P0：canonical target

| 组件/路径 | 修改内容 | 单组件退出条件 |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract/runtime_operation_contract*` | 增加target cardinality；删除File mutation与state-changing shell的重复target参数合同 | descriptor/hash/validator只认外层TargetRefs |
| `internal/pkg/workorchruntimecontract/candidate_contract_manifest*` | schema/manifest从同一descriptor生成，不再要求重复target参数 | renderer与decoder target规则一致 |
| `blocks/file/biz` | File command mapper只从唯一TargetRefs派生target；删除alias/fallback分支 | read/write/edit/patch各自可独立回归 |
| `blocks/shell/biz` | state-changing target只读取sealed operation TargetRefs | shell参数不再携带第二份target authority |
| `kernel/orchestrator/biz/runtime_v2_operation_executor.go` | typed owner command使用canonical target，无运行期猜测 | sealed operation与owner command target完全一致 |

### 8.3 P1：LLM boundary 与 renderer

| 组件/路径 | 修改内容 | 单组件退出条件 |
| --- | --- | --- |
| `blocks/llm/biz/runtimev2/candidate_boundary.go` | 只做framing；完整内容无论schema/semantic是否合法都返回DocumentContent | complete-invalid不返回provider error |
| `blocks/llm/biz/owner.go` | 保持EventHub typed result与audit；不转译candidate rejection | handler不选择attempt/retry/terminal |
| `blocks/llm/biz/runtime_candidate_provider.go` | C0/C2 target说明从manifest/descriptor渲染 | 不出现已删除target参数或第二份effect规则 |

### 8.4 P1：错误分类、恢复与交付

| 组件/路径 | 修改内容 | 单组件退出条件 |
| --- | --- | --- |
| `kernel/orchestrator/biz/runtime_v2_drive.go` | drive只消费typed internal outcome，不解析candidate error字符串 | candidate rejection不扣provider recovery budget |
| `kernel/orchestrator/biz/runtimev2/recovery_*` | provider transport、candidate attempt、repair、CAS、delivery预算继续分离 | 各类失败只扣自己的ledger |
| `blocks/run` | 继续opaque CAS aggregate与owner-local SM-1 | duplicate Resume不改变非法/terminal aggregate |
| `blocks/result/workspace/run` | 保持terminal delivery幂等 | candidate terminal正常完成三方ack且不改terminal语义 |

### 8.5 P2：只读消费者

TUI、Web、eventstream、read model/SSE、system、soul、skill、memory、mcpregistry只需核对没有：

- 解析candidate/provider错误文本决定Resume/Decision；
- 展示已删除的重复target参数为用户必填项；
- 从projection重建Runtime authority；
- 为candidate contract rejection创建无信息治理ticket。

## 9. 唯一串行实施队列

同一时刻只允许一张卡 `in_progress`。每张卡必须完成代码、直接测试、相关集成测试、文档与静态 gate 后，才能
开始下一张。旧 C0～C6 的单-owner/effect/receipt/delivery成果保留，但“全部 code-closed”结论被本次 fresh live证据
重新打开。

| 顺序 | 卡片 | 状态 | 修改范围 | 退出条件 |
| --- | --- | --- | --- | --- |
| C7-A | live反例归档与方案重开 | completed | 本文 | 反例、根因、owner和唯一新队列无歧义 |
| C7-B | complete document边界归一 | completed | LLM boundary、events DTO、EventHub adapter、framing helper | complete-invalid document不丢失、不变provider error |
| C7-C | SM-3 single evaluator与原子attempt | completed | mainline、runtimev2 coordinator/controller | schema/contract rejection持久化，attempt 0/1与restart闭合 |
| C7-D | canonical target合同归一 | completed | runtimecontract、manifest/renderer、File/Shell/executor | target只有一个authority，重复参数清零 |
| C7-E | 全矩阵与静态门禁 | completed | tests、scripts、core/readme/current-state | 定向、race、全量、boundary gate全部通过 |
| C7-F | fresh deployed live | pending | 全新workspace及restart/concurrency fixtures | 本文第12节全部通过，无人工补偿Resume |

实施顺序禁止改为先放宽 `patch/apply` 再处理 document handoff。C7-B/C7-C 先保证任何不稳定输出都进入正确状态机，
C7-D 再降低合同冗余；否则只会把当前错误推迟到下一种 candidate shape。

## 10. 强制回归矩阵

### 10.1 LLM owner 单组件

- empty、incomplete、complete-valid、complete-malformed JSON、complete-schema-invalid、complete-contract-invalid；
- complete document defensive copy且不进入audit/eventstream payload；
- continuation suffix按logical request/sequence/physical call有序；
- complete-invalid不返回Go error/provider failure；
- auth/config/transient/stale仍保持typed dependency outcome；
- provider response size受sealed ceiling约束。

### 10.2 EventHub adapter 单组件

- forged workspace/run、transaction、logical request、sequence、physical call被拒绝；
- complete document与fragment/stale/provider failure互斥；
- complete semantic-invalid document原样返回，adapter不调用candidate validator；
- wrong result type/nil result返回boundary error，零Runtime状态变化；
- raw document不写日志、不进入error text。

### 10.3 SM-3 evaluator与attempt

- malformed JSON、unknown field、missing parameter、target cardinality、coverage、effect、dependency、rejected shape；
- 每种complete rejection先有document_complete，再有唯一attempt rejection；
- attempt 0 correctable只开启一次attempt 1；
- attempt 0 rejected_final不请求provider第二次；
- attempt 1再次rejection形成candidate_retry_exhausted；
- rejection不创建dispatch、不调用capability owner、不扣provider transport/repair budget；
- accepted attempt与sealed dispatch在同一合法aggregate中；
- stable prefix、authority contract、manifest hash在attempt 0/1间不漂移。

### 10.4 canonical target矩阵

- File read/write/edit/patch apply/preview的zero/one/many TargetRefs；
- exact target、prospective owner target、existing owner scope；
- operation TargetRefs与owner command、feasibility、receipt完全一致；
- provider只输出外层TargetRefs即可执行，不需要targetRef/path重复字段；
- state-changing shell无TargetRefs拒绝，多TargetRefs正确seal；
- non-mutating shell不借用TargetRefs伪装durable effect；
- renderer/schema/decoder/validator/executor/receipt使用同一descriptor revision。

### 10.5 crash/restart/idempotency

- provider返回后但document写入前崩溃；
- content写入后但document_complete CAS前崩溃；
- document_complete后但adjudication前崩溃；
- candidate rejection CAS响应丢失；
- accepted attempt/dispatch seal响应丢失；
- duplicate complete document、duplicate adjudication、duplicate Resume；
- fresh process只从aggregate与artifact恢复，不重放已知physical provider call；
- active dispatch、owner receipt、resource-change invalidation和terminal delivery原有restart矩阵继续通过。

### 10.6 四状态机组合

- SM-3 rejection只产生SM-2 typed fact proposal，不直接选择下一action；
- SM-4 request failure不覆盖已有active dispatch；
- provider transport与candidate correction预算互不消费；
- candidate terminal后Result/Workspace/Run delivery duplicate、乱序、ack loss幂等；
- Run terminal后Resume保持terminal；
- 多workspace并发时document、request session、attempt、dispatch、receipt无scope串扰。

## 11. 静态门禁与验证命令

`verify-boundary-contract.sh` 至少新增或维持：

- LLM boundary和EventHub adapter不得调用`ValidateWithActivation`、operation semantic/effect/feasibility admission；
- complete document只有Orchestrator artifact store写入路径；
- `pkg`无repository、CAS、EventHub、retry、transition reducer；
- owner-local internal event未出现在`pkg/events`或跨owner topic；
- candidate response DTO不存在多权威`Candidate + DocumentContent`生产合同；
- File mutation与state-changing shell manifest不存在重复target parameter authority；
- mainline不得将complete document rejection映射为provider protocol；
- controller不存在active dispatch保留时只替换decision的入口；
- application/TUI/Web/governance/decision/approval无Runtime/Run/Workspace/Result state setter。

最低验证命令：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/{file,shell}/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/blocks/{run,workspace,result,decision,approval,git,mcp,ssh,web}/... -count=1
make verify-boundary-contract
go test -race ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/{llm,file,shell,run,workspace,result}/... -count=1
go test -p 1 ./... -count=1
```

测试通过不能替代 C7-F。必须保留至少一个真实 complete-semantic-invalid provider fixture和一个 deployed fresh live，
防止 fake provider直接构造已验证Candidate而绕开 raw document主链。

## 12. Fresh deployed live 退出条件

C7-F 必须重新构建、重新部署，并创建新的无历史 workspace。禁止复用
`ws-8974c4e22de0b2a4/run-001`，也不对旧 aggregate 做兼容迁移。

每次 live 保留：

- binary hash、workspace/run ID、definition revision、provider target；
- authority snapshot、action、focus、state revision；
- request logical/physical identity、manifest/effect/descriptor revision、stable prefix hash；
- raw document artifact identity/hash，不记录敏感正文到audit；
- candidate attempt/disposition/diagnostic；
- dispatch/operation/execution key/fence/receipt hash；
- receipt adjudication与跨SM transition；
- startup/Resume/governance continuation来源；
- terminal delivery identity与Result/Workspace/Run ack顺序。

至少完成：

1. 空workspace自动创建首个durable deliverable；
2. durable state change后进入独立validation；
3. 人为或真实制造一次 complete candidate schema/contract偏差，系统自动形成attempt rejection并按policy纠正；
4. validation失败后产生bounded repair并改变有效输入；
5. governance wait/continue不产生无信息ticket；
6. `active_sealed_dispatch`在owner receipt持久化后重启可继续；
7. document_complete但尚未adjudicate时重启可继续且不重放provider call；
8. normal terminal与administrative supersede均完成`Result -> Workspace -> Run` delivery；
9. 多workspace并发时scope不串扰；
10. 重复`/run resume`只触发幂等re-drive，不用于修补非法aggregate；
11. workspace最终满足Goal/DoD或形成有实质信息的typed terminal，不出现相邻合同逐轮暴露。

只有上述矩阵持续通过，才能关闭 `RUNTIME-SMO`。

## 13. 当前判定

| 范围 | 当前判定 | 未关闭项 |
| --- | --- | --- |
| SM-1 Run lifecycle | code-closed/live-partial | duplicate Resume、并发与restart live证据 |
| Workspace/Result lifecycle | code-closed/live-partial | normal与administrative terminal完整live矩阵 |
| SM-2 authority/focus | code-closed/live-pending | 需在 C7-F 验证只消费 SM-3 typed adjudication/fact proposal |
| SM-3 candidate/dispatch | code-closed/live-pending | complete document evaluator、attempt/restart与canonical target待 fresh live复核 |
| SM-4 drive/recovery | code-closed/live-pending | typed request outcome分类与预算隔离待 restart/concurrency live复核 |
| LLM owner | code-closed/live-pending | framing-only complete document handoff待真实provider复核 |
| File/Shell capability owner | code-closed/live-pending | canonical TargetRefs与typed command映射待 fresh workspace复核 |
| `internal/pkg/workorchruntimecontract` | code-closed/live-pending | framing/evaluator API分离、target cardinality同源待端到端复核 |
| deployed持续正向推进 | 未验收 | 仅 C7-F fresh deployed live 未完成 |

当前不得声明“全部组件code-closed”：C7-B～C7-E 已完成代码与回归门禁，但 C7-F fresh deployed live 仍未验收。
后续只能按 C7-F 的全新 workspace、重启与并发矩阵验收，不得以兼容分支、旧 aggregate 或人工 Resume 跳过该卡。

## 14. 完成定义与变更约束

单张代码卡完成必须同时满足：

1. 标明受影响SM；
2. 生产代码只有一个owner transition入口；
3. typed合同、renderer、validator、owner adapter同步；
4. transition/invariant/idempotency/restart/stale/duplicate/budget测试按影响补齐；
5. 单组件测试可独立运行；
6. 跨组件集成测试从raw EventHub document/receipt进入，不直接构造最终aggregate绕过主链；
7. core contract、Orchestrator readme、current-state与active matrix同步；
8. static gate与全量测试通过；
9. 不提交`.workorch-data`、`.workorch-workspaces`、`bin/`或provider敏感内容；
10. 不加入旧schema/旧target alias/旧candidate DTO兼容分支。

若实施中发现本文与 core contract 冲突，先暂停代码修改并更新权威设计；不得让两个解释同时存在。若新的 live
反例属于本文既有 D1～D6，扩充对应矩阵，不新增旁路计划。只有出现新的状态owner或不可归类的状态域，才允许
重新评审本文结构。
