# MIG08 Eventstream 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | EventRecord、游标、append/read及pre-seal无Run澄清校验 |
| 本轮剩余适配/验收（已有实现见§0） | 审计关联、提交/投递间隙恢复、显式游标失效与可重建投影 |
| 移除或禁止继续依赖 | 通用业务Send门面、审计驱动重执行、普通payload传播原始成功document/wire |

- 批次归属：参与C12/C14；按能力需要参与B05/B06/B07/B09/B11，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先兼容新schema读取，再分事实族接入生产者；投影缺口显式可见且不改owner事实
- 升级/回退重点：不删除新事件迁就旧读者；tombstone与generation约束继续拒绝迟到复活。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-D](../validation/固定验收Fixtures.md#fx-d)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V38](../validation/验收规格.md#vs-v38), [VS-V40](../validation/验收规格.md#vs-v40), [VS-V34](../validation/验收规格.md#vs-v34), [VS-V39](../validation/验收规格.md#vs-v39)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：业务通知→Eventstream | [CT-decision-TicketCreated](../contracts/decision.md#ct-decision-ticketcreated)、[CT-decision-TicketResolved](../contracts/decision.md#ct-decision-ticketresolved)、[CT-llm-LLMUsageRecorded](../contracts/llm.md#ct-llm-llmusagerecorded) | pre-seal无Run澄清已有专门接收校验；不能扩为自动变更批准 |
| 入边：读取/留存→Eventstream | [CT-eventstream-EventStreamCommandList](../contracts/eventstream.md#ct-eventstream-eventstreamcommandlist)、[CT-eventstream-EventStreamCommandHighWatermarks](../contracts/eventstream.md#ct-eventstream-eventstreamcommandhighwatermarks)、[CT-eventstream-EventStreamCommandSealWorkspaceDeletion](../contracts/eventstream.md#ct-eventstream-eventstreamcommandsealworkspacedeletion) | scope/游标/删除身份由owner校验 |
| 出边：Eventstream→投影通知 | [CT-eventstream-EventStreamRecordAppended](../contracts/eventstream.md#ct-eventstream-eventstreamrecordappended) | 仅表明可读审计变化，不驱动业务重执行 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[eventstream具体合同页](../contracts/eventstream.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

<a id="clarification-audit"></a>

## 0. 2026-10-12固定SHA重核与剩余增量

新增owner_lifecycle，command与notification均记录订阅、真实在途以及释放阶段；原无Run定义澄清审计路由继续存在。

复用事件投影/游标/去重身份，局部集成验证停止期间既有事实投递、提交-投递间隙与重建；原文审计仍不赋业务准入权。

当前路径：`internal/modules/blocks/eventstream`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/module.go#L1-L55)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/common/common.go#L1-L3)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L1-L126)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

pre-seal定义澄清的created/resolved审计继续绑定Workspace/Ticket，RunID/StepID为空并拒绝continuation；这不赋合同变更approve或自动新Run权限。

## 1. 现状与目标

Eventstream拥有审计资源、顺序游标及对外读取，不拥有Workspace、Run、Approval或运行证明的状态转换。保留Block分类；日志/SSE丢失或延迟不能凭自身把业务标为失败、成功、已批准或已启动，重放审计也不能直接重做副作用。

前轮已有根文件及生命周期核查，本轮补查定义澄清接收校验；尚未全量审查每种事件、schema、脱敏和恢复路径，没有据此新增独立业务bug。适配重点是M1–M4、合同变更ticket、精确批准、新定义/新Run和终态交接的关联可见性。

## 2. 代码影响与复用

[internal/modules/blocks/eventstream/biz/event_stream_service.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/eventstream/biz/event_stream_service.go)、[internal/modules/blocks/eventstream/module.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/eventstream/module.go)、[internal/modules/blocks/eventstream/plugin.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/eventstream/plugin.go)为已核对入口。G0继续枚举各owner事件生产者、事件存储与游标读取、Web SSE适配及删除调用方；不虚构尚未定位的文件名。

复用既有EventRecord及append/read服务、Base与注册；适配事件schema和correlation、保留历史兼容读取；补充可重建投影。不得创建通用业务Send门面、跨owner直接repository读取或恢复决策引擎。

## 3. 事件与读取合同

下表保留目标语义；其中无Run定义澄清接收端已有上述实现，其余字段和topic仍须映射现行typed合同后冻结。

| 交互 | 身份/内容 | 接收端不变量 |
|---|---|---|
| owner提交已发生事实 | EventID或owner+sequence、Workspace/可选Run、对象revision、causation/correlation、脱敏payload | 同身份同内容幂等；异内容冲突，不覆盖历史 |
| 读取时间线/SSE续传 | scope、游标及访问主体 | 按实际保留策略返回事件或显式游标失效，不静默跳过 |
| 重建展示 | 原owner的typed查询回执+审计关联 | 投影标出版本/缺口；不得将投影写回业务owner |
| 删除/留存处理 | Workspace删除身份/generation和owner清理请求 | 仅清理本owner范围，保留允许的tombstone/审计边界；迟到事件不复活Workspace |

M3记录创建完成时的精确定义、必要准备和当时readiness；M4独立记录激活身份/Run。ticket要能关联提案、最终候选approve、物化及后继阶段，但不可把令牌、凭据、完整敏感工具参数灌入日志。Result保存成功而Run未终态时展示“终态交接中”。

## 4. 分批接入与一致性

1. 固定持久审计记录schema/元数据版本及事实来源，不新增跨owner通用Envelope；先适配读取端兼容，再启用生产者新字段。生产者与消费者的不可兼容typed变更须同批提交
2. 首批只接创建M1–M4的完整可见链；随后接ticket/新定义/新Run，再接终态和删除，各批保留累计回归
3. 明确owner状态提交与审计追加间隙：现有机制若不提供原子性，应使用owner可恢复的待投递标记或等价持久交付机制，选择前先映射现存存储能力；不得声称“数据库写完再Post”天然不丢
4. 对重复、乱序和丢响应以稳定身份去重并核对owner事实；SSE连接重建不驱动业务重执行。顺序仅对定义的owner/stream成立，不能用墙钟强造全局因果

审计失败对业务是否阻断需依该事实的已冻结审计合同：必需审计缺失可令阶段不完成，但不伪造已经成功持久化的业务事实。纯展示通知丢失不倒推撤销业务提交。

## 5. 兼容、保留与回退

旧事件保留原schema与来源；新版通过显式适配生成读视图，不能补造旧记录没有的批准主体或hash。新增字段的敏感性、留存和访问边界一并评审。未知schema可呈现安全摘要及“不支持解析”，不丢弃为正常空结果。

回退不删除新版本事件来迎合旧读者；必要时保留兼容读服务并停止不兼容新增投影。日志恢复不能替代Run aggregate恢复，更不能清零预算或忘记外部操作。删除恢复须核对tombstone、当前generation及跨owner回执，防止晚到流复活已删对象。

## 6. 逐项验收设计（未执行）

| 编号 | 场景 | 预期与证据 |
|---|---|---|
| MIG08-T01 | 同事件重复投递/应答丢失 | 只有一个审计事实，原游标可查 |
| MIG08-T02 | 同EventID不同payload | 明确冲突且保留原文，不静默覆盖 |
| MIG08-T03 | M3后SSE断线，再刷新 | 权威查询仍为新建完成、Run=0；不自动Start |
| MIG08-T04 | approved事件先于UI读到proposal | 显示待补齐或按版本重查，不给当前未知候选授权 |
| MIG08-T05 | SSE游标超过留存范围 | 显式失效并全量重建，不伪称连续完整 |
| MIG08-T06 | Result保存、Run终态前进程崩溃 | 展示交接中；恢复靠owner回执而非事件驱动终态 |
| MIG08-T07 | 模拟含凭据/敏感执行参数 | 审计和导出脱敏，必要关联hash仍可核对 |
| MIG08-T08 | 删除后旧generation事件迟到 | 不复活列表或slot；按策略留历史/拒绝 |
| MIG08-T09 | 重放历史执行成功事件 | 无新dispatch，无预算或credit变化 |

新增回归设计（均未运行）：合法无Run/Step created与resolved可记录；伪造Run/Step、错误通知身份、非法resolution或夹带continuation均被拒绝；普通运行ticket缺Run仍拒绝。Workspace用量包含pre-Run，窗口截断/未加载显式展示，重放澄清不触发Start。

## 本组件集成证据补充

必须注入提交/投递间隙、断线及乱序；记录连续性范围，不能用SSE截图替代owner证明。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
