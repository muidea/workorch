# Workspace Reasoning Effort 单 Owner 收口计划

Status: archived code-closure record

Type: implementation closure plan

Last Updated: 2026-08-08

Archived: 2026-08-16

本文保留 Workspace reasoning effort 的设计与 code closure 过程。当前未关部署矩阵见
[Workspace Reasoning Effort Deployed Verification](../../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)。

## 1. 目标与范围

本计划根据 `ai-proxy/docs/integration.md` 收口 WorkOrch 的 workspace 级 reasoning effort 选择，建立一条唯一、
可恢复、可审计且不跨 owner 管理状态的生产链：

```text
ai-proxy /v1/models
  -> LLM owner 发布模型 reasoning capability
  -> Web/TUI 提交 workspace inference preference intent
  -> application 编排校验与 Workspace owner 写入
  -> Run owner 在 fresh activation/restart 时封存 canonical selection
  -> Orchestrator 只携带并校验 sealed fact
  -> LLM owner 构造 /v1/responses reasoning.effort
```

产品目标：

1. 用户可以在 Web 启动或 Restart workspace 执行时选择 reasoning effort。
2. 用户可以通过 TUI `/model` 子命令查看和修改当前 workspace 的下一次运行偏好。
3. workspace 偏好变更不得影响当前 active Run；Resume 必须继续使用当前 Run 的封存值。
4. Restart 必须读取并封存 workspace 当前偏好，形成新的 lineage。
5. 多 workspace 的选择、异步目录响应和运行请求不得互相污染。
6. WorkOrch 只依据 ai-proxy 公开模型目录判断能力，不根据模型名称、Provider 名称或历史配置猜测 effort。

本计划不增加 Block、Module 或第五个状态机，不改变 CWI、semantic-plan、candidate、dispatch、evidence 或 terminal
权威。reasoning effort 只是一项 provider inference selection，不是 Runtime action、coverage state 或重试指令。

## 2. 权威与单 Owner 边界

| 状态或事实 | 唯一 Owner | 允许行为 | 禁止行为 |
| --- | --- | --- | --- |
| 模型 reasoning capability | `blocks/llm` | 从 `/v1/models` 解析、规范化、校验和发布 typed fact | Workspace、Web、TUI 自行维护 effort 白名单 |
| 下一次运行推理偏好 | `blocks/workspace` | CAS 持久化 `DefaultLLMSelection` | Orchestrator、Run、Web/TUI 直接写 repository |
| 当前运行封存选择 | `blocks/run` | fresh activation 时封存，Resume 时保持 | Workspace 修改 active Run；Resume 重新解析偏好 |
| provider ceiling / request identity | `kernel/orchestrator` | 从 sealed activation 与 LLM owner fact 派生、封存和恢复 | 动态选择、降级或改写 effort |
| Responses 请求编码 | `blocks/llm` | 将 sealed effort 编码为 `reasoning.effort` | 从全局配置或 mutable workspace 重读 effort |
| Web/TUI 展示与输入 | application adapter | 展示 owner projection，提交 typed intent | 保存独立 UI 状态作为生产事实 |

跨 owner 交互继续只使用 EventHub concrete typed Command/Result。接收 owner 在自己的 `pkg/events` 定义 topic 和
合同；不得引入全局 event index、通用 envelope、`map[string]any`、JSON bridge 或跨 owner service/repository。

## 3. 当前事实与待清理偏差

当前代码已有 provider/model target 选择，但尚不满足本计划：

1. Workspace 只持久化 `DefaultLLMTarget *ProviderModelTarget`，不能表达 workspace 级 reasoning 偏好。
2. `RuntimeActivationContext` 只封存 `ProviderTarget`，active Run 无 reasoning 的不可变来源。
3. ai-proxy catalog DTO 没有解析 `capabilities.reasoning`。
4. catalog、management view、backend option、Web/TUI 类型均未暴露 reasoning capability。
5. Responses adapter 使用顶层 `reasoning_effort`，与最新 ai-proxy OpenAI Responses 合同不一致。
6. `ReasoningEffort` 仍来自全局 inference profile，可能让一个进程配置影响所有 workspace。
7. Web activation 当前先写 `/llm-target` 再提交 execution activation，且 activation body 仍携带 form-derived
   `llmTarget`；canonical owner result 与本次 Run 封存来源没有形成唯一链。
8. TUI `/model` 只选择 target，无法显示 provider default、允许 efforts 和当前 Run sealed effort。

在代码收口完成前，`docs/01-overview/current-state.md` 与 `docs/50-product-interface/service-api.md` 中的
`DefaultLLMTarget`、`/llm-target` 描述仍代表当前代码事实；对应实施项完成时必须原子刷新为本计划的最终合同，不能长期
保留两套正式叙述。

## 4. ai-proxy 外部合同

WorkOrch 只读取以下公开能力：

```json
{
  "capabilities": {
    "reasoning": {
      "supported": true,
      "default_effort": "low",
      "efforts": ["none", "low", "high", "max"]
    }
  }
}
```

Responses 请求只使用：

```json
{
  "reasoning": {
    "effort": "high"
  }
}
```

必须区分：

- `capabilities.reasoning.efforts` 是模型允许值集合，使用复数；
- `reasoning.effort` 是单次请求的一个选择，使用单数；
- 空 workspace 选择表示省略整个 `reasoning` 对象，让 ai-proxy 使用 provider 默认策略；
- `none` 是显式值，不等于空值；
- capability 字段缺失表示未声明支持，不得按模型名称推断；
- `/v1/models` 的未来新增字段必须被忽略，不能导致 DTO 解码失败。

## 5. 目标合同

### 5.1 Owner-neutral immutable selection

在 `internal/pkg/workorchruntimecontract` 增加：

```go
type ProviderInferenceSelection struct {
    Target          ProviderModelTarget
    ReasoningEffort string
}
```

该类型只提供纯函数：

- `Normalized()`：trim effort；target 使用现有规范化规则；不改变大小写、不映射同义词；
- `Validate()`：校验 target identity 和 effort 的基本形状，不读取 catalog；
- `Same()`、`Clone()`：用于 owner 持久化、activation equality 和 projection detach；
- stable hash helper：只有确实需要 sealed identity 时提供，不拥有状态或 I/O。

空字符串和 `none` 必须保持不同 identity。`ProviderModelTarget` 继续只表达 provider/model identity，不向其中塞入
inference policy。WorkOrch 当前只允许 ai-proxy，但内部 target identity 仍保留 `ProviderID=ai-proxy`；Web/TUI 不再向用户
暴露自定义 Provider 选择。

### 5.2 LLM owner reasoning capability

在 `blocks/llm/pkg/models` 增加 owner-specific capability：

```go
type ProviderReasoningCapability struct {
    Supported     bool
    DefaultEffort string
    Efforts       []string
    Valid         bool
    InvalidReason string
}
```

规范化规则：

1. `supported=false` 或字段缺失：`Supported=false`，显式 effort 不可用。
2. 去掉空 effort 和重复项，保留 ai-proxy 返回顺序，不改变大小写。
3. `supported=true` 但 efforts 为空：reasoning capability invalid。
4. 非空 `default_effort` 不在 efforts 中：reasoning capability invalid。
5. reasoning capability invalid 不应让普通 Responses 文本能力整体变成 unavailable；只有显式选择 reasoning 时拒绝。
6. catalog descriptor 和管理视图分别展示 base readiness 与 reasoning readiness，不能复用一个
   `capability_incomplete` 模糊两种语义。

### 5.3 Workspace owner state

将：

```go
DefaultLLMTarget *ProviderModelTarget
```

替换为：

```go
DefaultLLMSelection *ProviderInferenceSelection
```

owner command：

```go
type UpdateDefaultLLMSelectionCommand struct {
    WorkspaceID      common.WorkspaceID
    Selection        *runtimecontract.ProviderInferenceSelection
    ExpectedRevision uint64
}
```

owner result 返回 detached canonical Workspace 或 selection、owner revision 和 `Changed`。相同 normalized selection
重复提交必须幂等；stale revision 必须返回 typed conflict。`nil` 表示后续 fresh activation 继承系统 `defaultChat` 且不显式
指定 reasoning。

Workspace owner 只校验值形状和 CAS。模型是否存在、是否 ready、effort 是否允许由 LLM owner typed validation fact 决定，
application 在 owner 写入前完成编排；Workspace 不读取 LLM catalog 或配置 service。

### 5.4 Run activation state

`RuntimeActivationContext` 将 `ProviderTarget` 替换为：

```go
ProviderSelection ProviderInferenceSelection
```

Run projection 将 `SealedLLMTarget` 替换为 `SealedLLMSelection`。fresh activation 的 canonical selection 解析优先级：

```text
application 已通过 Workspace owner 保存并返回的 selection
  -> workspace DefaultLLMSelection
  -> system defaultChat + empty reasoning effort
```

不再接受 caller 直接提交一个仅对本次 Run 生效但不属于 Workspace owner 的 target/effort patch。

### 5.5 Provider ceiling

Provider ceiling 或等价 sealed request contract 必须包含：

```go
Selection          ProviderInferenceSelection
CapabilityRevision string
ConfigurationRevision string
```

reasoning effort 参与 ceiling identity、activation equality 和 request audit identity，但不进入 C0/C1/C2
`stablePrefixHash`。Orchestrator 不根据 effort 调整 CWI、action、output schema、semantic batch 或 retry。

## 6. 唯一交互流程

### 6.1 独立修改 workspace 偏好

Web 设置页或 TUI `/model` 使用同一 application use case：

```text
typed selection intent
  -> LLM owner ValidateProviderInferenceSelection
  -> Workspace owner UpdateDefaultLLMSelection CAS
  -> workspace projection/event
  -> adapter 展示 canonical next-run selection
```

application 只编排，不缓存 selection、不写 repository、不发布目标 Workspace 状态。

### 6.2 Web Start / Restart 时选择

Web activation dialog 可以同时提交模型和 effort，但 HTTP body 只是 application intent。application 必须：

1. 将 body 转为 typed `ProviderInferenceSelection`；
2. 向 LLM owner校验模型、Responses endpoint、base readiness 和 effort；
3. 向 Workspace owner CAS 保存选择；
4. 只使用 Workspace owner 返回的 canonical selection 构造 `RuntimeBootstrap`；
5. 向 Run owner 提交 fresh activation/restart intent；
6. 返回 workspace preference outcome 与 activation outcome。

这是 application 跨 owner 用例，不是跨 owner 状态机。步骤 3 成功、步骤 5 失败时不得回滚 Workspace owner；响应必须明确
“偏好已保存、Run 启动失败”，后续 fresh activation仍使用已保存值。

### 6.3 Resume

Resume 请求禁止携带 selection：

```text
/run resume
  -> Run owner current non-terminal Run
  -> existing sealed ProviderInferenceSelection
  -> Orchestrator recovery/re-drive
```

若 Web/TUI 在 active Run 期间修改 workspace 偏好，该修改只影响未来 fresh activation/restart。Resume 不比较 mutable
workspace selection，也不因其变化产生 drift。

### 6.4 Restart

Restart 创建新 lineage并封存 workspace 当前 canonical selection。predecessor 的 selection 只用于审计，不继承、不覆盖新
workspace preference。Restart 时 Web 可先提交新的 selection；TUI 则由 `/model ...` 保存后再显式执行 `/run restart`。

### 6.5 Catalog drift

active Run 的 selection 不可修改。LLM catalog revision 变化时：

- 同一 effort 仍受支持：重新取得 capability fact/ceiling freshness，不修改 selection；
- effort 不再受支持：返回 typed `provider_reasoning_capability_drift`，不得降级到空值、default 或相邻档位；
- 用户修改 workspace preference 后显式 Restart 才能采用新值。

## 7. HTTP / Web 收口

### 7.1 Workspace preference API

将 `/llm-target` 替换为：

```http
PUT    /api/v1/workorch/workspaces/{workspaceId}/llm-selection
DELETE /api/v1/workorch/workspaces/{workspaceId}/llm-selection
```

PUT body：

```json
{
  "providerId": "ai-proxy",
  "modelId": "DeepSeek-V4-Flash",
  "reasoningEffort": "high",
  "expectedRevision": 12
}
```

DELETE 清除 workspace override。代码完成后直接删除旧 route、request DTO、client method 和测试，不保留 alias、forwarder、
fallback 或兼容说明。

### 7.2 Model catalog API

模型 descriptor 增加 reasoning capability view，至少包括：

```json
{
  "reasoning": {
    "supported": true,
    "valid": true,
    "defaultEffort": "low",
    "efforts": ["none", "low", "high", "max"]
  }
}
```

base model readiness 继续描述 Responses 请求是否可用；reasoning validity 单独展示。

### 7.3 Activation dialog

Web Start/Restart 弹窗：

1. 只列出 ai-proxy 且支持 `/v1/responses` 的 ready 模型。
2. 模型下方提供 Reasoning Effort Select。
3. 首项固定为 `Provider default`，值为空。
4. 其余选项严格来自当前模型 `efforts`。
5. `default_effort` 只作为说明展示，不自动变成显式持久化值。
6. 切换模型后，旧 effort 不被新模型支持时清为 `Provider default` 并给出明确提示。
7. Resume 不出现可编辑 Select，只读展示当前 Run sealed selection。
8. Workspace 详情同时展示“下次运行模型/reasoning”和“当前 Run 封存模型/reasoning”。
9. 表单关闭、workspace 切换和 catalog refresh 必须丢弃旧 workspace/request sequence 的异步结果。

## 8. TUI `/model` 收口

reasoning 统一纳入 `/model` 命令族，不新增顶级 `/reasoning`：

```text
/model
/model list
/model refresh
/model use <model-id>
/model use <model-id> --reasoning <effort>
/model reasoning
/model reasoning default
/model reasoning none
/model reasoning <catalog-effort>
/model reset
```

语义：

- `/model`：展示 workspace next-run selection、provider default、允许 efforts 和 active Run sealed selection；
- `/model list`：列出 base readiness 与 reasoning capability，不把 capability 缺失显示为模型整体不可用；
- `/model refresh`：刷新 ai-proxy catalog，保留 workspace/request sequence fence；
- `/model use`：交互选择模型后进入 reasoning 第二步；
- `--reasoning`：模型和 effort 作为一次 Workspace owner CAS 提交，禁止先写 target 再写 effort；
- `/model reasoning`：只展示当前 workspace 选择模型的 reasoning capability；
- `default` 映射为空字符串，`none` 保持显式值；
- `/model reset`：清除 workspace override，后续 Run 继承 system defaultChat 和 provider default reasoning；
- active Run 期间修改后显示“当前 Run 不变，下一次 Start/Restart 生效”；
- `/model` 不隐式执行 `/run restart`，避免一个 adapter command 同时修改 Workspace preference 和 SM-1 lifecycle。

模型 ID 使用 ai-proxy 返回的精确值，大小写敏感。当前 workspace 继承 system default 且用户直接执行
`/model reasoning high` 时，TUI 必须先将有效 system default target解析为完整 selection 后一次提交；没有 ready target 时拒绝，
不能持久化只有 effort、没有 target 的半合同。

## 9. Responses adapter 与配置清理

删除当前错误 wire 字段：

```go
ReasoningEffort string `json:"reasoning_effort,omitempty"`
```

替换为：

```go
type responsesReasoning struct {
    Effort string `json:"effort"`
}

type responsesRequest struct {
    Reasoning *responsesReasoning `json:"reasoning,omitempty"`
}
```

编码规则：

- sealed effort 为空：`Reasoning=nil`，序列化结果完全没有 `reasoning`；
- sealed effort 为 `none` 或目录允许值：发送嵌套对象；
- adapter 收到未由 sealed selection/ceiling 提供的 effort：typed contract error；
- adapter 不从进程全局配置、provider profile、workspace repository 或 model name 回填 effort。

同步删除：

1. `workorchconfig` custom-provider/model inference profile 的 `ReasoningEffort`；
2. configuration contract、provider config model、Web ConfigurationPage 对应字段；
3. `ProviderInferenceProfile.ReasoningEffort` 及其 clone/merge/update helper；
4. 旧 JSON/TOML 示例、测试 fixture 和兼容描述。

WorkOrch 仅通过 ai-proxy OpenAI `/v1/responses` 访问模型；不得为 reasoning 增加 direct provider、Chat Completions fallback
或 Provider 私有请求分支。

## 10. 四状态机影响

| 状态机 | 允许修改 | 必须保持的不变量 |
| --- | --- | --- |
| SM-1 Run lifecycle | fresh activation 封存 selection；projection 展示 sealed selection | 不选择下一 action；Resume 不重写 selection |
| SM-2 authority/focus | 消费 LLM owner capability fact并封存 provider ceiling | 不根据 effort 改 CWI/coverage；只有 internal event 写 authority |
| SM-3 candidate transaction | candidate/provider request携带 sealed selection | 不从 workspace/global config重读；不选择 effort |
| SM-4 drive/recovery | 从 aggregate 恢复 selection、处理 capability drift | 不从 UI/projection/cache 重建；不静默降级 |

本计划不改变 semantic-plan batch、candidate evaluator、sealed dispatch、operation receipt 或 evidence gate。reasoning 值不同
不能成为重新排序 CWI、扩大输出预算、增加 retry 或跳过 incomplete response 的依据。

## 11. Cache、预算与审计

1. reasoning effort 不进入 C0 system/schema、C1 Goal/DoD 或 C2 declarative capability prefix。
2. effort 进入 volatile activation/request identity 和 provider ceiling hash。
3. 修改 effort 不得导致 `stablePrefixHash` 漂移；如果发生，应判为 request rendering bug。
4. request audit记录 selected effort、capability revision 和 omitted/explicit disposition，不记录 raw prompt、API key 或 raw
   provider body。
5. reasoning effort 不自动扩大 `max_output_tokens`、semantic-plan batch 或 timeout；预算仍由 sealed阶段预算决定。
6. ai-proxy 返回 incomplete、timeout 或 conversion degradation 时继续使用现有 typed provider outcome，不按 effort 猜测成功。

## 12. 分阶段代码收口

### WRE-1：Canonical value 与目录能力

- [x] 新增 `ProviderInferenceSelection` 及 normalize/validate/same/clone 测试；selection 直接参与现有 sealed identity，无独立重复 hash helper。
- [x] catalog external DTO 解析 `capabilities.reasoning`，允许字段缺失和未来扩展。
- [x] `ProviderModelDescriptor`、management view、backend option 透传 reasoning capability。
- [x] reasoning invalid 与 base readiness 分离。

完成门槛：目录缺失/supported/invalid/default/duplicate/future-field matrix 全部通过。

### WRE-2：Workspace owner

- [x] `DefaultLLMTarget` 替换为 `DefaultLLMSelection`。
- [x] owner-local concrete update Command/Result、CAS、`Changed`、幂等和 clone/detach 收口。
- [x] memory/file repository 持久化测试更新。
- [x] workspace projection/event payload更新。
- [x] 删除旧 target update topic/helper/route 合同。

完成门槛：两个 workspace 独立写入；duplicate 无额外 mutation；stale CAS 拒绝；owner restart 后值保持。

### WRE-3：Application validation 与激活

- [x] LLM owner 定义 `ValidateProviderInferenceSelection` concrete EventHub contract。
- [x] application selection use case 编排 LLM validation -> Workspace update；Run activation入口保持 intent-only并随后读取 owner canonical selection。
- [x] Web Start/Restart 先完成 application selection use case，activation wire不携带 form-only selection。
- [x] Resume body不包含 selection patch。
- [x] selection保存与异步 activation分别返回明确结果；activation失败不跨 owner回滚已保存偏好。

完成门槛：unsupported effort 在 Workspace 写入前拒绝；caller 无法提交完整 Workspace/Run owner entity。

### WRE-4：Run / Orchestrator / LLM request

- [x] `RuntimeActivationContext` 和 Runtime aggregate改为 sealed selection。
- [x] Run fresh activation/restart/resume/recovery 语义固化。
- [x] Provider ceiling、freshness、request identity包含 sealed effort。
- [x] Responses adapter改用 `reasoning.effort` 嵌套结构。
- [x] capability drift形成 typed outcome，不静默降级。
- [x] 删除 global/custom-provider reasoning source。

完成门槛：空/default、none、显式 effort、resume/restart、catalog drift、crash recovery matrix 全部通过。

### WRE-5：TUI / Web / projection

- [x] TUI `/model` 命令族与交互 selector 支持 reasoning。
- [x] TUI workspace/request sequence fence覆盖目录刷新和提交响应。
- [x] Web activation modal支持 Start/Restart reasoning selection。
- [x] Web Resume只读展示 sealed selection。
- [x] Workspace/Run detail投影区分 next-run 与 current-run。
- [x] 多 workspace 切换不串 selection。

完成门槛：Web/TUI adapter 测试、console build 和双 workspace交互回归通过。

### WRE-6：清理、文档与全链验证

- [x] `rg` 清零代码、正式文档与验证脚本中的 `DefaultLLMTarget`、`SealedLLMTarget`、`/llm-target` 和 wire `reasoning_effort`（本计划的历史偏差/删除清单除外）。
- [x] 删除旧 alias、forwarder、compatibility/fallback、Web失效自定义 Provider编辑器和全局 provider reasoning配置。
- [x] 同步刷新 `current-state`、`service-api`、LLM assembly、orchestrator readme；request/cache规则无需改变，effort不进入稳定 prefix。
- [x] 更新 active overview、未收口矩阵和 todo 索引状态。
- [x] 完成 focused、全仓、boundary、console 和 managed verifier 验证。

当前验证状态：focused、全仓、`fmt-check`、`vet`、console build、boundary，以及
`make verify-managed-full` 已通过。managed verifier 在一次性本地部署实例上完成 HTTP、LLM 与 MCP stdio
验证，覆盖本项的编译、owner 合同和进程装配基线；它不替代真实部署实例上的 Web Start/Restart、TUI
`/model reasoning`、Resume sealed selection 与双 workspace 隔离交互矩阵。因此 WRE 仍为
`code-closed / live-open`，直到该专门 live 矩阵取得新证据。

完成门槛：代码、owner readme、核心从属文档和产品接口只描述一套正式合同。

## 13. 必需测试矩阵

| ID | 场景 | 预期 |
| --- | --- | --- |
| WRE-T01 | catalog capability 缺失 | base model可用；只能 provider default |
| WRE-T02 | supported=true + valid efforts | 按目录原顺序展示和校验 |
| WRE-T03 | invalid default/empty efforts | reasoning invalid；base readiness不被覆盖 |
| WRE-T04 | empty effort request | JSON 中不存在 `reasoning` |
| WRE-T05 | explicit `none` | `reasoning.effort=none` |
| WRE-T06 | explicit supported effort | 精确嵌套编码 |
| WRE-T07 | unsupported/case-mismatched effort | owner 写入前 typed rejection |
| WRE-T08 | duplicate workspace update | 幂等、无额外 revision/event |
| WRE-T09 | stale update | CAS conflict，不覆盖新值 |
| WRE-T10 | active Run 后修改 workspace | current sealed selection不变 |
| WRE-T11 | Resume | 使用当前 Run sealed selection |
| WRE-T12 | Restart | 新 lineage使用最新 workspace selection |
| WRE-T13 | process restart recovery | 从 aggregate恢复，不读 mutable workspace |
| WRE-T14 | catalog drift仍兼容 | freshness更新，selection不变 |
| WRE-T15 | catalog drift不兼容 | typed drift；不降级、不请求 provider |
| WRE-T16 | 双 workspace不同 efforts | 请求与投影完全隔离 |
| WRE-T17 | TUI切换 workspace时旧响应到达 | request sequence/workspace fence丢弃 |
| WRE-T18 | Web切换模型 | 不兼容 effort清为 provider default并提示 |
| WRE-T19 | preference保存成功、activation失败 | 偏好保留；响应区分两个 outcome |
| WRE-T20 | stable prefix | effort变化不改变 C0-C2 `stablePrefixHash` |

SM 级回归必须同时包含 transition、invariant、idempotency/restart、stale/duplicate 和 budget/guard 测试；不能只验证
HTTP body 或 UI 表单。

## 14. 删除与冲突清理清单

代码完成时直接删除，不保留兼容层：

- `DefaultLLMTarget` / `SealedLLMTarget` 字段及 clone helper；
- `/llm-target` PUT/DELETE route、API client 和 backend method；
- activation body 中 target-only patch；
- provider/custom model `ReasoningEffort` 配置、更新与投影字段；
- Responses 顶层 `reasoning_effort` wire DTO；
- Web “高级（含自定义 Provider）”相关失效入口；
- 只验证旧 target-only 行为的 tests/fixtures；
- 描述 explicit activation target -> workspace target -> system default 的旧文档规则。

归档历史文档可以保留当时事实，但不得从 archive 恢复旧接口或兼容要求。active、core、current-state、module readme 和
product-interface 必须只描述最终 selection 合同。

## 15. 验证命令与最终 DoD

按阶段先跑 focused tests，最后执行：

```bash
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/blocks/run/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
make console-build
make fmt-check
make vet
go test ./... -count=1
make verify-boundary-contract
make verify-ci-mvp-file
```

最终完成条件：

1. Web Start/Restart 和 TUI `/model` 都只能通过 Workspace owner canonical selection 生效。
2. active Run 的 sealed selection 在 workspace 修改、进程重启和 Resume 后保持不变。
3. Restart 使用最新 workspace selection 并形成新 lineage。
4. LLM request只发送 ai-proxy 目录允许的 nested `reasoning.effort`；空值完全省略。
5. 无全局 reasoning source、无跨 workspace共享 mutable selection、无跨 owner状态写入。
6. 四状态机 owner、转换、恢复和证据链没有新增旁路。
7. 代码与正式文档中的旧 target-only/reasoning wire 合同清零。
8. focused、全仓、boundary、console、managed deployed验证全部通过。
