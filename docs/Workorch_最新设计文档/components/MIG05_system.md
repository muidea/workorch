# MIG05 system组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Module → Module；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | typed聚合调用及来源owner |
| 本轮剩余适配/验收（已有实现见§0） | 有时点/绑定/unknown的readiness与LLM窗口视图 |
| 移除或禁止继续依赖 | 缓存准入、隐匿失败的全ready及第二配置账本 |

- 批次归属：参与C04/C09/C14/C15；按能力需要参与B05/B11，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：只读投影可独立升级；任何客户端激活入口仍依赖Workspace/Run动态准入
- 升级/回退重点：旧bool-ready客户端与新unknown语义须成对迁移；不能用缺字段默认true。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-P](../validation/固定验收Fixtures.md#fx-p)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V26](../validation/验收规格.md#vs-v26), [VS-V38](../validation/验收规格.md#vs-v38), [VS-V15](../validation/验收规格.md#vs-v15), [VS-V19](../validation/验收规格.md#vs-v19), [VS-V35](../validation/验收规格.md#vs-v35)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：系统内部应用调用 | [CF00](../Workorch_组件固定合同与分层验收规范.md#cf00) | System无自身pkg/events topic，按CF00定位真实装配/服务边界；不虚构SystemReady命令 |
| 目标消费边：System→资源/配置owner查询 | [CT-llm-LLMCommandGetActiveConfiguration](../contracts/llm.md#ct-llm-llmcommandgetactiveconfiguration)、[CT-mcpregistry-MCPCommandReadRegistrySnapshot](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandreadregistrysnapshot)、[CT-workspace-WorkspaceCommandGet](../contracts/workspace.md#ct-workspace-workspacecommandget)、[CT-run-RunCommandGet](../contracts/run.md#ct-run-runcommandget) | 这些是真实owner查询ID；本次不声称所有System调用点已接线，实施须逐字段登记来源/缓存与错误 |


## 0. 2026-10-12固定SHA重核与剩余增量

当前System Module实际组合Soul/WorkspaceTemplate/MCPRegistry的EnsureDefaults启动用例，并无自身查询topic或现成全局readiness聚合API；根module只做Biz依赖/生命周期桥。已重核现角色和公共ID，未新增System业务topic，不把配置或读投影包算新owner。

复用typed读取，核验部分失败/unknown、动态readiness与LLM窗口呈现；不另建准入权威或把projection成功当执行许可。

当前路径：`internal/modules/kernel/system`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/module.go#L1-L55)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/pkg/common/common.go#L1-L3)；[biz/system.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L1-L136)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 当前与目标责任

保持kernel Module，路径 internal/modules/kernel/system，当前负责默认资产的跨owner初始化；DM14/39能力汇总与查询视图是目标映射，现实现主要在workorchd读投影，不应虚构System现有接口。目标保持，但优先复用已有读投影而非另造System账本或第二启动准入。

已抽样确认biz/system.go采用owner定义typed事件并检查返回存在性、错误及类型；未发现足以要求重写的独立角色缺陷。全部配置写入、查询缓存、别名、异常与持久化路径仍待深核，不能据抽样宣布全部合规或必然要改业务代码。

## 2. 复用与具体影响面

- 直接复用 internal/modules/kernel/system/biz/system.go的typed调用及结果检查模式、现有Module/Biz/Base结构
- 核验 internal/modules/kernel/system/module.go、plugin.go的装配与生命周期；共同Base错误API变更时盘点本组件全部调用方
- 适配查询输出：区分能力声明、当前readiness、历史检查、当前正式定义、M3历史完成回执和Run实际状态
- 具体缓存与配置处理器路径在G0源码清单中补齐，未核验前不臆造service文件或新增权威表

本组件可在某批仅完成合同核验和测试补充；“进入22组件验收清单”不等于必须更改每个组件的业务算法。

## 3. 交互合同与视图规则

以下是待映射的typed语义，不代表已有符号名称。请求和返回使用owner pkg/events，不把所有响应塞入无约束any/map。

| 生产者→消费者 | 输出及边界 |
|---|---|
| capability owner→System | operation版本、scope、观测时间、资源/配置绑定、ready/blocked/unknown及原因；不可用不得转换成默认ready |
| Workspace→System | 定义版本、M3历史事实、当前eligibility等只读数据；System不修改定义或slot |
| Run→System | 权威RunID、SM-1及关联定义；没有Run应显式为空，不制造占位Run |
| System→workorchd/Web | 带来源和新鲜度的可恢复展示；各子查询失败必须保留诊断，不把部分成功包装为全部ready |

M3检查通过只是某时点事实。用户稍后点击激活，启动服务端必须按精确输入重新核验动态条件；System缓存不能作为启动许可证。Config写入若存在，应路由真正配置owner并遵循权限/版本，System不得静默跨owner更新。

## 4. 增量调整步骤

1. 盘点所有查询/配置事件、producer和消费者，明确每个字段来源及缓存时效；先不改组件角色
2. 与Workspace、实际资源owner和workorchd固定readiness展示合同，将unknown和过期事实显式返回
3. 配合M3切片证明“创建已完成但Run为0”可稳定展示；当前资源随后失效仍保留历史回执而显示不可激活
4. 随Base/进程生命周期小批适配错误与停止行为；若本owner已经满足合同，保留实现并提供证据
5. 加入累计启动和治理回归；诊断可见性提升不能替代Run/Workspace服务端准入

仅有UI查询不需增加新的持久状态机；若为性能引入投影，必须可由owner事实重建，不能依赖投影恢复业务权威。

## 5. 兼容、部分失败和回退

现有消费方如把bool ready解释为永久允许启动，需要成对迁移到明确状态/绑定/时点，不能只改服务端JSON而遗漏客户端。新增字段保持清晰版本与缺省语义，缺失状态按unknown展示，不默认true。

多个owner查询中某一个失败，返回局部结果和该owner错误；不能隐匿失败导致全部ready。读视图重试只重读事实，不触发资源准备/Start副作用；启动EnsureDefaults是独立有写入的幂等用例，须核验部分成功后从原owner事实恢复。缓存重建丢失不修改定义或Run。回退到旧读视图前确认其不会将新unknown当ready；必要时禁用旧激活展示但保留真实后端校验。System本身不应该拥有需要回滚的执行副作用账本；若发现则作为越界迁移缺陷处理，账本转移必须单权威且可恢复。

## 6. 验收用例（拟定，未运行）

1. 全部必需owner返回准确ready时视图展示绑定、时点和来源
2. 任一provider不可用或typed返回类型错误，显示blocked/unknown及诊断，不默认ready
3. M3成立且Run为0时正确显示可激活，不显示“执行中”
4. M3后资源失效，历史创建完成仍成立，当前readiness失效；Start被服务端拒绝
5. 缓存的旧定义ready不能为新定义激活，客户端刷新不改变业务状态
6. 部分查询失败保留其他合法结果和失败标记，不伪装聚合全部成功
7. 重复查询、超时重试或SSE重连不创建Run、不重复准备资源
8. 配置变更若涉及目标/约束不得借System直改绕过ticket；普通配置权限仍独立验证
9. 停止期间任务提交拒绝显式可诊断，未排空不释放共享资源
10. 缓存清空后从各owner重建视图，Workspace/Run/Decision权威数据完全不变

## 8. 历史固定版本证据（5554296）

- [typed结果检查](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/system/biz/system.go#L51-L108)
- [显式组件装配](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/application/workorch/main.go#L10-L34)
- [共同Base任务合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/base/biz/base.go#L119-L135)
- [全局生命周期现状](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/vendor/github.com/muidea/magicCommon/framework/application/application.go#L270-L313)

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
