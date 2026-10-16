# MIG13_mcpregistry MCP注册资产迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | mcp-registry持久/事件身份、RegistrySnapshot单聚合/CAS及clone |
| 本轮剩余适配/验收（已有实现见§0） | CAS revision→content revision→冻结选择逐字段映射 |
| 移除或禁止继续依赖 | 同名服务热替换旧Run、泄露Env秘密或可变Registry指针 |

- 批次归属：参与C05/C09/C13/C14；按能力需要参与B05/B07/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：选择快照生产消费两端就绪后再启用；配置可发现不开放调用
- 升级/回退重点：只从原持久内容确定性补content revision并标算法；回退不复活撤销，不覆盖并发注册。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-P](../validation/固定验收Fixtures.md#fx-p)、[FX-C](../validation/固定验收Fixtures.md#fx-c)；[VS-V06](../validation/验收规格.md#vs-v06), [VS-V20](../validation/验收规格.md#vs-v20), [VS-V37](../validation/验收规格.md#vs-v37), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：配置协调→MCPRegistry | [CT-mcpregistry-MCPCommandCreateServer](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandcreateserver)、[CT-mcpregistry-MCPCommandUpdateServer](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandupdateserver)、[CT-mcpregistry-MCPCommandUpdateCapability](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandupdatecapability) | 保留单聚合CAS/内容版本与现有owner身份 |
| 入边/返回边：Workspace/MCP→快照 | [CT-mcpregistry-MCPCommandReadRegistrySnapshot](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandreadregistrysnapshot)、[CT-mcpregistry-MCPCommandReadSelectionSnapshot](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandreadselectionsnapshot) | 出边是相应typed Result/深复制快照，不另造授权或执行通知 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[mcpregistry具体合同页](../contracts/mcpregistry.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

owner_lifecycle显式区分Get/List/ReadSnapshot收尾查询与fresh写入，逐topic登记；原mcp-registry身份与Registry聚合未变，已重核。

复用registry/selection snapshot，逐字段验证CAS revision→content revision→冻结选择；恢复不得默认新revision等价旧授权。

当前路径：`internal/modules/blocks/mcpregistry`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/module.go#L1-L52)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/pkg/common/common.go#L1-L21)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner_lifecycle.go#L1-L57)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcpregistry/biz/owner.go#L1-L335)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、现状与复用决策

目标保持MCPRegistry Block，拥有DM39服务/能力注册事实、配置版本与选择快照，不拥有MCP工具执行或操作许可。当前合同owner字符串为mcp-registry，不能因目录名mcpregistry而顺手更换持久/事件身份。

已查证：contracts.go有Server/Capability的Get/List/Create/Update、ReadRegistrySnapshot、ReadSelectionSnapshot、EnsureDefaults；RegistrySnapshot为服务与能力的单一持久聚合。Revision是仓库CAS fence，对外由owner派生content revision，不能把两者互换。CloneRegistrySnapshot深拷贝服务器Args/Env和允许列表；不应重新发明第二registry表。

复用现有单聚合、CAS、clone及快照查询；针对DM39补核冻结绑定、秘密引用和当前失效行为。注册更新并非已执行副作用；目录可发现并非授权。尚无本轮已证实的独立局部生命周期bug，全部注册更新及别名消费者仍待深核。

## 2. typed交互及版本规则

1. workorchd/System用owner命令维护或查询注册资产；命令带可信主体、注册键、expected revision及内容身份。并发更新冲突返回可查询现版本，不最后写入者胜出
2. Workspace读取选择快照，记录精确服务/能力、content revision和合同约束；Run保存冻结引用。不得保存原始secret、可变Registry指针或live MCP client。Server.Env等字段需区分非秘密配置与受控凭据引用，公开查询不得泄漏敏感值
3. MCP执行owner消费不可变选择描述，实际调用前仍验证当前授权、目标scope、能力状态和适用fence；Registry的AllowedTools说明注册配置范围，不能替代操作许可或当前运行约束
4. Registry更新不直接启动/停止Run，也不通过热指针影响已封存输入。配置漂移、撤销及不可用事实可导致当前readiness阻塞；不能静默选择另一个同名服务满足旧合同
5. 删除/禁用后的历史引用须可解释，不能把历史调用的服务名映射到新服务。查询快照仅证明当时配置，不证明远端实际响应或工具执行成功

## 3. 迁移顺序、兼容与恢复

- 先建立“仓库CAS revision→内容revision→冻结选择”的明确字段映射，并审查所有导入pkg/events的生产/消费方；保留既有事件owner与聚合ID
- 与Workspace/System成对收口定义准备和M3时点检查；再与MCP/Orchestrator成对验证调用前重查、配置漂移及撤销。若先改Registry后消费者仍读可变对象，该批不能退出
- 共享Base和停机适配集中在owner资源获取/释放，不将Registry提升为跨owner执行调度器；查询方不持有内部仓库或连接缓存
- 旧快照缺少选择content revision时，能从原持久内容确定性计算并保留算法版本才迁移；不能以当前Registry快照补写历史Run。不能确定的引用标为待重建/阻塞
- 迁移使用旧聚合备份、schema版本及CAS检查，支持同内容重放和中断续作；不能覆盖并发新注册记录。回滚前检查目标版本是否理解新引用和撤销状态，否则只读隔离并前向修复
- Registry回滚不能复活已撤销访问或清除MCP的在途/unknown操作；对账归原执行owner，保留原operation与绑定版本

## 4. 计划验收矩阵（未执行）

| 用例 | 确定预期及独立证据 |
|---|---|
| 正常选择快照进入定义 | content revision与实际服务/能力一致，CAS与内容版本分别可核对 |
| 两个管理员并发更新 | 仅合法CAS提交，失败者收到版本冲突；无拼接半个聚合 |
| 调用方修改返回的map/slice | 原Registry不变；前后owner快照及持久内容相等 |
| 服务可发现但未授权执行 | MCP拒绝执行，Registry存在不构成许可，远端无调用 |
| 封存后更新同名服务 | 原Run引用不变；漂移阻塞或走合法新定义，不热切换 |
| 禁用/撤销后旧快照Start | 服务端重查失败，历史M3仍可展示但不得启动 |
| 更新提交后丢响应 | 同身份读回相同revision，不能重复默认种子或覆盖新值 |
| 秘密配置错误/查询/导出 | 输出均脱敏，凭据不落合同和审计；缺引用明确不可用 |

## 6. 历史固定版本与影响路径（5554296）

- [当前注册与选择合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcpregistry/pkg/events/contracts.go)：`internal/modules/blocks/mcpregistry/pkg/events/contracts.go`
- [单聚合CAS及深拷贝](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcpregistry/pkg/models/mcp_registry.go)：`internal/modules/blocks/mcpregistry/pkg/models/mcp_registry.go`
- [资产服务影响面（叶子逻辑待深核）](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcpregistry/biz/mcp_service.go)：`internal/modules/blocks/mcpregistry/biz/mcp_service.go`

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
