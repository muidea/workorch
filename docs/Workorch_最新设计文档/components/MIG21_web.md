# MIG21：web 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | safe fetch、URL/DNS/redirect政策、provider授权及receipt |
| 本轮剩余适配/验收（已有实现见§0） | 拨号目标/重定向实际核验、内容版本/截断与freshness |
| 移除或禁止继续依赖 | 以HTTP 200/摘要/URL或ETag授证明；将Block网络政策称进程隔离 |

- 批次归属：参与C09/C12/C13/C14；按能力需要参与B07/B08/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先验证catalog与实际fetch边界；未具备重定向许可/地址检查的路径保持阻塞
- 升级/回退重点：同URL变化形成新内容hash；旧网络观察保留时间语义，回退不能静默视为新鲜内容。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-P](../validation/固定验收Fixtures.md#fx-p)、[FX-O](../validation/固定验收Fixtures.md#fx-o)；[VS-V09](../validation/验收规格.md#vs-v09), [VS-V13](../validation/验收规格.md#vs-v13), [VS-V24](../validation/验收规格.md#vs-v24), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→Web能力 | [CT-web-AcquireRuntimeOperationAdmissibility](../contracts/web.md#ct-web-acquireruntimeoperationadmissibility)、[CT-web-ExecuteOperation](../contracts/web.md#ct-web-executeoperation) | URL/DNS/redirect每跳与精确provider绑定；不是UI入口 |
| 邻接许可合同：治理/运行协调→Approval | [CT-approval-OpenRuntimeApprovalRequest](../contracts/approval.md#ct-approval-openruntimeapprovalrequest) | 许可申请协调不迁入Web；Web消费封存授权并检查实际风险scope，fetch授权不能自动覆盖新重定向风险 |
| 返回边：web→执行调用方 | [CT-web-ExecuteOperation](../contracts/web.md#ct-web-executeoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[web具体合同页](../contracts/web.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

新增owner_lifecycle与逐topic记录，Module Teardown失败保留Biz；URL/DNS/重定向与sealed操作角色已重核不变，Web仍非用户UI。

复用网络能力，核验实际拨号/重定向目标、内容截断/freshness、迟到响应和关门后新请求拒绝；不把网络观察当行为PASS。

当前路径：`internal/modules/blocks/web`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/module.go#L1-L52)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/common/common.go#L1-L23)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner_lifecycle.go#L1-L56)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L1-L487)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、证据和复用决策

DM13/24：网络获取、搜索及实际操作观察；不是用户Web界面。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** 适配、深核：未证明存在Git/Shell同型started缺陷；已有URL/DNS地址分类、重定向检查及精确授权实现，重点补齐目标规则映射与故障证据，不预设全部重写。

**复用/适配/替换：** 复用safe fetch transport、URL政策、provider配置、sealed操作与receipt；适配资源版本、截断和依赖事实；只有实际核验失败的合同才替换。Web Block不承担Workspace创建页，Web-only创建入口属于workorchd前端/应用链。

## 2. 上下游合同与本组件规则

Orchestrator通过Web owner typed能力、准入、feasibility和执行合同获得网络事实；Web按适用风险合同查询Approval，不自行批准。现有授权绑定CatalogSnapshotID、AcquisitionBinding、PrerequisiteID、OwnerRevision、CatalogObservation、Policy；provider endpoint和credential不跨公共合同。

每个初始URL、DNS解析地址及重定向目标均需边界核验；允许公开入口不代表批准重定向至私网，fetch范围不代表整个进程网络隔离。实际结果记录最终URL、内容hash、时间、provider版本、截断/提取方式及错误；URL或ETag本身不替代内容版本。搜索排名/摘要只是来源候选，HTTP 200不证明目标完成。

## 3. 小批迁移及兼容策略

1. 先配对provider catalog事实与Orchestrator依赖；再核验safe fetch的DNS固定、连接目标、重定向和Approval作用域；最后闭合内容receipt与Evidence消费者。网络观察按时间语义判freshness，不能为追求复现而把旧网络内容静默当作当前。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 允许地址合法获取 | 返回精确最终URL/内容/版本事实 | URL链、内容hash、receipt |
| 重定向至禁止/新风险目标 | 拒绝或等待精确许可，不沿用原批准 | redirect链、审批scope |
| DNS重绑定/混合私网地址 | 按地址政策拒绝，不连接越界IP | 解析/拨号观察 |
| provider revision变化 | 旧授权失效，重新准入 | 旧新catalog、拒绝结果 |
| 大响应截断 | 显式truncated，不称完整页面 | 字节数、提取策略 |
| 超时或重复付费搜索 | unknown/费用可对账，不换key盲重试 | operation与provider用量 |
| 相同URL内容变化 | 新hash触发对应依赖重判 | 两次内容和binding |
| 删除/关闭时请求未退 | 保留transport依赖直至真实drain | 在途计数、退出记录 |

## 6. 历史固定源码与规范（5554296）

- [web/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/pkg/events/operation.go)
- [web/biz/safe_fetch_transport.go#L43-L115](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/biz/safe_fetch_transport.go#L43-L115)
- [web/biz/url_policy.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/biz/url_policy.go)
- [web/biz/runtime_fetch.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/biz/runtime_fetch.go)
- [web/biz/runtime_operation_receipt.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/biz/runtime_operation_receipt.go)

前轮读取operation合同；其余历史链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
