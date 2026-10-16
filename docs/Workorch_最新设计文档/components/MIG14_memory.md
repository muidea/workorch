# MIG14_memory 记忆与召回追踪迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | MemoryItem、RecallTrace、已有seal/purge合同 |
| 本轮剩余适配/验收（已有实现见§0） | 真实采用上下文版本、索引代次与删除fence核验 |
| 移除或禁止继续依赖 | 旧Run结论直接授当前credit；用搜索候选冒充已输入集合 |

- 批次归属：参与C12/C13/C14；按能力需要参与B05/B09/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：新旧embedding索引按模型/维度隔离；上下文链与删除链可分批，但删除scope不能重启写入
- 升级/回退重点：缺版本的旧trace保持不明；回退不重召已撤销内容或删除新tombstone。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-D](../validation/固定验收Fixtures.md#fx-d)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V20](../validation/验收规格.md#vs-v20), [VS-V24](../validation/验收规格.md#vs-v24), [VS-V34](../validation/验收规格.md#vs-v34), [VS-V37](../validation/验收规格.md#vs-v37), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：上下文消费者→Memory | [CT-memory-MemoryCommandSearch](../contracts/memory.md#ct-memory-memorycommandsearch)、[CT-memory-MemoryRecallTraceCommandUpsert](../contracts/memory.md#ct-memory-memoryrecalltracecommandupsert) | 查询集与真实采用集分别保留；有来源不授证明credit |
| 入边：Workspace删除→Memory | [CT-memory-MemoryCommandSealWorkspaceDeletion](../contracts/memory.md#ct-memory-memorycommandsealworkspacedeletion)、[CT-memory-MemoryCommandPurgeWorkspace](../contracts/memory.md#ct-memory-memorycommandpurgeworkspace) | 保留本owner阶段回执与tombstone，不创建第二删除journal |
| 出边：Memory审计事实 | [CT-memory-MemoryRecallRecorded](../contracts/memory.md#ct-memory-memoryrecallrecorded)、[CT-memory-MemoryDeleted](../contracts/memory.md#ct-memory-memorydeleted) | 通知/trace与实际输入一致；不因重放恢复已删知识 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[memory具体合同页](../contracts/memory.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

owner_lifecycle把合法Get/List/Search/RecallTrace查询与trace收尾记录纳入drain，写入仍由原owner授权；原召回追踪和clone继续复用。

核验真实采用内容/索引代次、删除fence、trace最后落盘和重启；Memory历史不能变成当前Run的proof。

当前路径：`internal/modules/blocks/memory`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/module.go#L1-L52)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/common/common.go#L1-L65)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner_lifecycle.go#L1-L59)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L1-L108)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、现状与复用决策

目标保持Memory Block，拥有DM15/38的MemoryItem和RecallTrace，提供有来源的上下文。知识可被参考，不能因被召回或来自旧成功Run便直接取得当前Goal验收credit。最终证明适用性仍由Orchestrator裁决。

已查证：现有合同有Memory CRUD/Search、RecallTrace Upsert/List、Workspace删除Seal/Purge及事实通知；MemoryRecallTrace已记录Workspace/Run/Step、候选/选中/排除IDs、查询hash、选择策略、模型维度、评分、预算、fallback和EvidenceRef。不能因字段名EvidenceRef就推导其具有目标验收权威。

复用现有追踪、clone和删除专用合同，适配来源版本及冻结上下文；既有trace不是待重新发明的功能。索引、召回算法和降级行为仅在合同不符时定向调整；全部查找/更新、向量存储一致性和嵌套clone仍需深核，不据根结构评审宣称完备。

## 2. typed交互与适用性

1. workorchd/Orchestrator通过Memory的Search和查询合同请求允许scope内知识，结果携带item身份、内容版本/摘要及来源；定义阶段可无Run，不制造假Run以填字段
2. 本次选择生成RecallTrace，固定query hash、筛选政策、选择与排除原因、context预算及实际采用条目版本。LLM使用的是被记录的上下文，不得只记录搜索候选就声称全部已提供
3. 与LLM/上下文组装消费方成对保证trace和真实输入对应；超预算截断、模型降级、检索失败必须可见。RecallTrace证明来源与采用过程，不证明目标行为已实现
4. 条目更新/删除影响未来召回；已封存上下文按保留/隐私政策保留最小审计引用，不能静默替换已运行内容。安全撤销可使后续访问阻塞，不能用旧缓存绕过
5. Workspace删除由唯一Workspace journal协调，Memory仅执行本owner的seal/purge和结果回报；晚到upsert不得在已删除scope复活条目。共享知识与Workspace-local知识须区分归属，不能删除整个外部知识库

## 3. 定向迁移与历史处理

- 先盘点MemoryItem实际来源/版本和RecallTrace字段到DM38的映射，保留已有检索流程与ID；缺字段是否可从原记录确定性补出须逐类说明
- 在上下文切片中成对改Memory与LLM/Orchestrator的采用记录；在删除切片中成对改Workspace协调命令及Memory的fence/tombstone处理，不新建第二删除journal
- 调整检索算法不能修改已冻结required集合或默认提高旧证据等级；替换embedding模型需保留原模型/维度及索引版本，新旧查询正确路由，禁止混合维度蒙混返回
- 旧trace缺内容版本时保留“版本不明”，不能用当前item覆盖历史结果。迁移可以创建新索引代次并核对条目数/归属，原条目和审计记录不被覆盖
- 回滚保留新增条目、更新版本及删除tombstone，避免旧索引重新召回已撤销内容。若旧程序不识别新维度或删除fence，保持只读/禁用该检索分支并前向修复
- 停机先拒绝新写、等在途索引/trace写完成或保留可恢复状态，再释放仓库；不能已返回trace成功却只写入不可恢复的内存

## 4. 计划验收矩阵（未执行）

| 用例 | 确定预期及独立证据 |
|---|---|
| 正常召回并进入模型上下文 | 条目版本、selected集和实际输入摘要一致，可沿trace追溯 |
| 召回旧Run成功报告 | 只能作为参考；当前required义务仍未满足，closure不增加credit |
| 跨Workspace/越权知识查询 | 拒绝或按明确scope过滤，返回和trace无越权正文 |
| 召回后条目修改 | 既有trace保持原引用，新召回采用新版本；不回填旧内容 |
| 预算截断/检索降级 | 真实采用集和fallback原因记录，未采用条目不得标已输入 |
| 新embedding维度与旧索引不兼容 | 明确拒绝/隔离重建，不输出错误相似度或伪空命中 |
| trace提交丢响应后重放 | 同身份同内容可读回，不重复制造采用事实；异内容冲突 |
| 删除seal后迟到更新、恢复 | 不复活已删除scope；原journal可核对Memory完成/阻塞 |

## 6. 历史固定版本与影响路径（5554296）

- [记忆、追踪与删除合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/memory/pkg/events/contracts.go)：`internal/modules/blocks/memory/pkg/events/contracts.go`
- [已有召回trace与clone](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/memory/pkg/models/memory_recall_trace.go)：`internal/modules/blocks/memory/pkg/models/memory_recall_trace.go`
- [删除交接影响路径](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go)：`internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go`

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
