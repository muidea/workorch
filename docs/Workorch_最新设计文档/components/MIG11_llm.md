# MIG11_llm LLM调用与配置绑定迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | provider、配置脱敏视图、结构化候选与调用事实 |
| 本轮剩余适配/验收（已有实现见§0） | 定义scope用量/unknown归属、冻结模型选择、实际输入与审查范围追踪 |
| 移除或禁止继续依赖 | LLM自批合同/Start/Goal成功；把not_loaded/partial显示零调用 |

- 批次归属：参与C05/C09/C13/C14；按能力需要参与B01/B05/B07/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：定义scope预算持久归属未明确前阻塞有费定义能力；只读preflight可独立使用，不暗发付费请求
- 升级/回退重点：旧审计保留未知与原attempt；无历史模型版本不以当前配置补写。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V15](../validation/验收规格.md#vs-v15), [VS-V23](../validation/验收规格.md#vs-v23), [VS-V37](../validation/验收规格.md#vs-v37), [VS-V40](../validation/验收规格.md#vs-v40), [VS-V45](../validation/验收规格.md#vs-v45)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：定义/运行→LLM | [CT-llm-LLMCommandCompleteProviderDocument](../contracts/llm.md#ct-llm-llmcommandcompleteproviderdocument)、[CT-llm-PreflightRuntimeCandidate](../contracts/llm.md#ct-llm-preflightruntimecandidate)、[CT-llm-RequestRuntimeCandidate](../contracts/llm.md#ct-llm-requestruntimecandidate) | 绑定候选/运行版本；preflight不等于允许有费执行 |
| 入边：配置读取 | [CT-llm-LLMCommandGetActiveConfiguration](../contracts/llm.md#ct-llm-llmcommandgetactiveconfiguration)、[CT-llm-LLMCommandValidateInferenceSelection](../contracts/llm.md#ct-llm-llmcommandvalidateinferenceselection) | 返回脱敏/固定选择，凭据不入定义或Run |
| 出边：LLM用量事实 | [CT-llm-LLMUsageRecorded](../contracts/llm.md#ct-llm-llmusagerecorded) | 保留submitted/unknown、实际scope和窗口；定义期预算归属未明确则阻塞启用 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[llm具体合同页](../contracts/llm.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。


## 0. 2026-10-12固定SHA重核与剩余增量

owner_lifecycle已跟踪定义与运行调用至真实完成，模块Teardown错误保留Biz；定义authoring映射和用量窗口合同继续复用。

核验取消后provider真实结束、unknown用量归属与重复permit恢复；逐要求审查覆盖已由Workspace增强，LLM意见不直接授封存或执行权。

当前路径：`internal/modules/blocks/llm`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/module.go#L1-L62)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/common/common.go#L1-L44)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner_lifecycle.go#L1-L70)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1-L1435)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

<a id="usage-scope"></a>
定义用量继续复用Workspace/workorchd的有界窗口：availability=complete仅指当前窗口，not_loaded/partial及无记录不能推出无调用或零费用。Workspace/Run scope分别查询；document与wire成功事实分开，受限原文查询还须核验主体权限和脱敏。

## 1. 责任、现状与复用决策

目标保持LLM Block，拥有DM14/24/39的provider配置、调用事实、能力与用量；产出规划候选和审查意见。Workspace决定合同准入/封存，Orchestrator决定证明采纳/闭包；LLM不得自批合同、调用Start或自判Goal成功。

已查证：contracts.go区分执行、查询、用量通知，包含CompleteProvider、CompleteProviderDocument、GenerateEmbedding、RequestRuntimeCandidate及Preflight；ProviderConfig含APIKey，另有RedactedProviderConfig；embedding结果已有ProviderSubmitted、字节数和usage。字段存在不证明所有审计路径均脱敏或提交后故障均可对账。

直接复用候选边界、配置快照、调用审计和模型目录结构；适配复用冻结选择/版本绑定及未知用量传播。审查意见仅供权威owner校验，不另建LLM审批状态机。未深审的provider、流式协议与全部降级路径先保持待核验，不笼统重写。

## 2. typed交互与不可越权合同

1. Workspace/workorchd请求定义审查：绑定draft/candidate版本、content hash和适用Requirement/Obligation集合；返回结构化意见、覆盖范围、来源及失败原因。六类意见均PASS仍须Workspace检查完整覆盖，模型意见不代替精确合同确认
2. Orchestrator请求运行候选：携带Workspace/Run、冻结provider/model/context引用、任务身份和允许的操作边界；LLM返回proposal，不发送正式终态或扩大权限。相同模型多次自检不得冒充独立外部验证
3. System读取脱敏配置、目录和当前readiness；配置查询不得回传APIKey，快照持久化只记录受控凭据引用及非秘密字段，错误/trace也需脱敏。现有APIKey可存在于owner受保护运行内存，这不授权将其复制到合同、Run或日志
4. 每次物理调用区分逻辑请求、attempt和provider提交事实；超时后usage未知不能按零计费释放预算。有Run时由现有运行预算权威接收用量事实。定义生成/审查在M3前也可能收费：绑定定义作用域request、candidate版本、明确授权及预算；启用前必须映射真实usage/unknown的持久归属和查询身份，不挂到不存在的runtime ledger、不制造Run或另建预算owner。此映射未明确即为定义切片G0阻断项。provider不支持查询/幂等时保留未知及风险，不声称exactly-once
5. 配置更新只影响新解析；冻结Run不悄悄换模型/降低验证等级。凭据撤销或provider禁用可阻塞当前执行，不能覆盖原合同。M3检查只读检查必要能力，不创建Run或暗发实际有费用的工作请求

## 3. 定向步骤与数据恢复

- 第一步在定义审查切片列出现有所有CompleteProviderDocument消费者，固定输入hash、意见来源及错误类型；与Workspace审查覆盖检查同批验收
- 第二步在运行/证明切片，与Orchestrator、Run及用量接收者成对收口provider选择、提交事实和预算未知处理。保留实际provider-specific差异，不用统一success布尔掩盖部分响应
- 第三步接入共享生命周期协议：停止接单后等待在途调用或保留可恢复未知事实，再排空审计与共享任务；不以关闭HTTP响应等同取消远端工作
- 老Run缺少精确模型/配置版本时标记绑定不充分并阻塞依赖该绑定的动作；不得把当前配置倒填为历史事实。老audit保留原身份、已知usage及未知项，新增字段做版本化读取
- 回滚只能回到理解新快照/未知记录的版本；不能重置调用身份、用量和预算。不能安全降读时保留只读历史并前向修复。上线切换前准备旧格式、新格式和中断恢复样本

## 4. 计划验收矩阵（未执行）

| 用例 | 确定预期及独立证据 |
|---|---|
| 精确候选正常审查 | 意见绑定目标hash及要求集合；Workspace采纳记录与provider审计可关联 |
| 意见PASS但漏一个required要求 | 准入拒绝；缺项列表可核对，无新seal |
| LLM输出越权Start/成功结论 | 仅保留无效proposal与拒绝原因，无Run/closure副作用 |
| 定义/运行请求已提交后断流 | 各自scope原attempt保留submitted/unknown及预算；定义阶段Run数仍为0，账本可查询 |
| 配置变更后恢复旧Run | 仍使用冻结选择或明确阻塞；前后快照hash相同 |
| provider失效及凭据撤销 | 当前readiness失败，历史M3回执不被改写，日志无秘密 |
| 停止期间迟到响应 | 原身份审计至多采纳一次，退出失败仍保留恢复依赖 |
| 同一请求重放、同键不同内容 | 原身份可查询；冲突被拒绝，不以换key掩盖重复费用 |

## 6. 历史固定版本与影响路径（5554296）

- [当前typed合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/pkg/events/contracts.go)：`internal/modules/blocks/llm/pkg/events/contracts.go`
- [配置、脱敏视图与提交事实](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/pkg/models/provider_config.go)：`internal/modules/blocks/llm/pkg/models/provider_config.go`
- [现有快照测试参考（未运行）](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/biz/owner_configuration_snapshot_test.go)：`internal/modules/blocks/llm/biz/owner_configuration_snapshot_test.go`

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
