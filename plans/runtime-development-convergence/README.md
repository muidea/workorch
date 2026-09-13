# Runtime 开发与验收闭环收口计划

Status: in-progress — 仅实施计划，非现行设计合同

Baseline: `9b5371271dddadc034357f61a45f840f8ba044c5`（2026-09-10）

本文及同目录文件用于安排下一轮代码收口。创建本文不代表功能已实现、合同已生效，也不授权修改部署数据、启动 Run、发布服务或提交代码。

当前执行约束：按用户后续要求，本阶段不考虑数据兼容性，不实现旧 hash/renderer/归档迁移或跨版本恢复；使用当前协议的新测试数据。此约束不授权删除现有数据，也不取消同版本重启、幂等、预算及封存准则保护。

## 1. 收口结论与目标

最新代码切片在 exact CWI 声明与能力清单投影之上，增加独立的 owner 工作范围只读观察请求/适配层，绑定
当前 manifest/revision 并与最终证明的 target selection 隔离；详细实现与验证见 [CL-01](implementation-plan.md#cl-01)。
尚未接入 drive/private-event consumer 或工作 observation 的 aggregate 写集合。执行门禁仍关闭，
工作意图、working binding、回执后同 CWI 重观测尚未接通，不代表 CL-03 或完整开发循环已完成。

保留现有单 owner、typed EventHub、封存 dispatch、owner receipt、Run CAS 和恢复底座，补齐 **CWI 内有界开发迭代与验证输入生命周期**。不再只为最新 diagnostic 增加特例，不建立第二套任务调度器。

WorkOrch 的核心目标是：对基于不同 template 创建的 Workspace，按其封存 Goal/DoD，在明确能力、授权与预算内推进实际工作，取得可追溯证据并完成逐项验收。当前 MCP LSP-Bridge MVP 是检验该能力的交付案例，不是要写进 WorkOrch core 的业务实现。

本轮成功必须同时满足：

1. 正确且可完成的开发案例具有可执行的“实现—验证—诊断—修复—重新验证—验收”路径。
2. 普通实现缺口、验证草稿缺项和可解释的执行失败，不因缺少下一步合法合同而立即退出。
3. 原 Goal/DoD、原有效验收义务及已接纳的补充不被删除、弱化或互相替代。
4. 每个正式状态只有一个 owner；状态转换只由该 owner 的内部事件触发。
5. 能力不足、信息不足、真正冲突、预算耗尽能精确等待或终止，不伪造进展、不承诺无限自治。

## 2. 基线事实与设计偏差

下表记录本轮方案依据，不是持续更新的运行看板。运行事实来自此前对该基线部署的只读核对；CL-00 负责保存可复验的脱敏证据，后续部署需要重新观测。

| 基线事实 | 说明与偏差 | 对应收口 |
| --- | --- | --- |
| `ws-a585a713bc3db2f0` 的 24 个已核对 Run 均未成功；最新 `run-024` 于 2026-09-10 07:28:03–07:30:18 UTC 执行后失败 | 多轮局部修复尚未形成完成该 Goal 的能力证据；不能仅据次数断言所有失败同因 | CL-00、CL-11、CL-12 |
| `run-024` 有 1 个 File durable effect，1/27 CWI 满足；24 个 DoD 均未评估，未取得 validation receipt | 有活动及局部写入，不等于功能验证或目标推进已闭环 | CL-02、CL-03、CL-09 |
| 最新原因是 `validation_input_review_coverage_incomplete`：`dod-001` 的输入覆盖工具清单，但未覆盖“并文档化” | 是验证输入覆盖遗漏；不能据此断言交付项目一定没有文档。当前 reducer 按合同明确失败 | CL-04、CL-05 |
| 默认 software-dev 顺序是 source change → 每项 DoD → report → Git snapshot | 顺序本身保留；但后续一般实现工作主要依赖特殊失败修复合同，不能把第一次 source change 完成当成开发工作全部完成 | CL-03、CL-06 |
| 既有 input addition 只接受 exact behavior-unobserved 失败与 typed attestation；unattributed failure 不授予修改目标 | 守住了授权边界，但缺少从真实遗漏报告及只读诊断获得新合法工作范围的通路 | CL-05、CL-06 |
| 最新 Result 已 rejected，终结交付 ack 完整，投影追平，active Run 已清空 | 此次不是靠重连 UI、补一次 drive 或继续 Resume 就能推进的非终态卡住 | CL-09、CL-10 |
| 边界检查可通过；固定 fixture / scripted provider 已能验证若干修复与重启流程 | 证明安全边界和特定事务，不证明真实模型可完成任意 Goal；DCE producer registry 仍为空 | CL-11、CL-12 |

直接代码依据：

- [输入评审 reducer](../../internal/modules/kernel/orchestrator/biz/runtime/validation_input_review.go) 与 [Orchestrator 合同](../../internal/modules/kernel/orchestrator/readme.md)：v2 纯遗漏分类仍为明确失败，不是补充或重试授权。
- [软件开发验收合同](../../docs/40-profiles-capabilities/software-dev/goal-coverage-and-dod.md)：默认 CWI 顺序、输入补充/维护边界、实际交付入口证据要求。
- [Shell 诊断实现](../../internal/modules/blocks/shell/biz/shell_diagnostic.go) 与 [修复归约](../../internal/modules/kernel/orchestrator/biz/runtime/validation_failure_repair.go)：诊断位置不能自动变成实现修改授权。
- [DCE verifier](../../scripts/verify_deployed_convergence_manifest.py)：exact producer registry 当前未登记生产者。

根本问题不是所有 owner 需要重划，而是：**安全的证据提交协议尚未覆盖足够完整的开发工作闭环；验证输入的身份/结构正确，也被过度当作验证能力已经充分。** 本计划同时处理合法推进能力、验证有效性和发布证据，三者不可互相代替。

## 3. 文档入口

| 文档 | 用途 |
| --- | --- |
| [合同与数据边界](contracts-and-migration.md) | owner/SM 边界、工作循环、草稿/补充/版本更替三条路径、预算与当前协议 |
| [逐项实施任务](implementation-plan.md) | CL-00～CL-12 的依赖、代码落点、合同变化、测试和完成标准 |
| [验证与发布矩阵](verification-matrix.md) | 正反例、crash cut、真实模型试验、原 Goal/DoD 回归与 DCE 门槛 |

阅读顺序：本文件 → 合同与数据边界 → 实施任务 → 验证矩阵。执行时按任务依赖逐项提交可验证的垂直切片；不能只完成类型或 helper 就宣称闭环。

## 4. 职能主体与不变边界

| 职能 | 唯一职能主体 | 其他组件的协作边界 |
| --- | --- | --- |
| 基于事实的现状与上下文 | capability/resource owner 生产各自事实；Orchestrator 形成 Runtime 的权威事实视图 | LLM 负责指定层的请求表达；Memory、Eventstream、UI 不补写事实或重建 authority |
| 完整执行反馈 | capability owner 提供结果及有界诊断；Orchestrator 关联本次 dispatch、准则和缺口 | Result/UI 保真呈现；stdout 和模型总结不能替代 owner receipt |
| 下一轮推进目标选择 | Orchestrator SM-2 私有事件 | LLM 只提建议，SM-3 只完成候选事务，SM-4 只驱动交付 |
| 整体目标跟踪与验收 | Workspace 拥有封存定义；Orchestrator SM-2 裁决 CWI/证据及整体结论 | Run 拥有 SM-1；Result 存已裁决结论，不能重新判分 |

完整组件覆盖和接线检查见 [组件责任表](contracts-and-migration.md#component-boundaries)。不增加 framework unit；保持 1 Initiator、4 Modules、17 Blocks。

禁止生产路径引入 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、自动 successor replan、map-based authority rebuild、跨 owner service/repo 调用，以及 UI/脚本直接推进状态。

## 5. 阶段与发布门槛

| 阶段 | 任务 | 出口门槛 |
| --- | --- | --- |
| G0：事实与合同冻结 | CL-00、CL-01 | 原定义与差异清单固定；每个字段/转换有唯一 owner；容量与预算政策明确；不改默认协议 |
| G1：最小完整工作闭环 | CL-02～CL-06，相关 CL-08 测试同步完成 | 同一 Run/CWI 内先实现、再验证，纯遗漏可补充，无归因可诊断；每条路径真实 owner 正例、拒绝例及重启均通过 |
| G2：治理与产品完整性 | CL-08～CL-10；CL-07 单独准入 | 预算不刷新，同版本恢复，Result/TUI/Web 一致；错误验证实现的版本更替未审定前仍显式禁用 |
| G3：能力与发布验证 | CL-11 | deterministic 合同闭环、重复真实模型试验、DCE producer 自证与 exact registry 均有证据；不得把几者混为一次通过 |
| G4：原案例受控验收 | CL-12 | 用户授权后部署/激活；原 24 DoD 逐项取得证据、正式 Result 完成；full DCE 通过后才宣称本轮 release 收口 |

CL-07 不是 CL-05 的隐含权限。若原项目确需更替错误验证实现而该合同尚未准入，G4 必须明确阻塞，不能跳过对应 DoD 或用人工改旧文件绕过。

## 6. 范围控制与文档纪律

- 不将 LSP-Bridge 代码、九工具名、五语言后端或业务性能指标写入 Runtime core；它们只用于当前 Workspace 的验收案例及 profile/adapter 测试。
- 不扩展 software-dev 的其他实现语言 adapter；[SD-001](../../TODO.md#sd-001软件开发-template-多语言能力扩展) 仍待完整 Go 开发闭环验证。Go 实现的 LSP 产品应支持的语言范围不因此减少。
- 不热替换运行中的模板/Soul/policy，不重写历史 Definition、评审和证据，不复活 terminal Run，不依靠自动创建新 Run 刷新预算。
- “新增补充”只能合取细化；“错误验证实现更替”必须采用单独、保留原义务的版本化合同。评审无法判断走事实 ticket，不走验收豁免审批。
- 每项代码变更同时更新其现行 Core/Profile/Interface 及组件合同；未实施内容留在本目录，不能提前进入 `docs/` 或组件 `readme.md` 成为规范。
- 合同变更、测试和启用策略应同一切片评审；禁止只放开入口而把 authority/recovery 测试推迟到最后。
- 本目录是临时实施材料。任务完成后在 TODO 关闭入口，并按需要移除本计划，历史由 Git 保存；不形成第二套长期设计权威。

## 7. 任务完成记录规则

当前已开始 CL-00/CL-01 的 Profile 无损准入与完整 hash 切片：修复空/重复声明被归一化吞掉、配置 merge 隐藏重复项以及 proof 约束未全部进入 hash 的问题。补充 shared contract、Application→Workspace 准入、final bootstrap 和 file-backed owner 重启回归。它不是 CWI 工作循环完成，也不能关闭 CL-00/CL-01 的剩余任务。

CL-01 后续补齐 raw Profile 封闭字段与精确类型检查，并将默认模板未被消费的环境策略副本收口到已有 typed validation
policy 的指导文本；未知新工作合同不再静默忽略。该切片没有开放 CL-03，工作范围与有限共享预算前置仍待完成。

CL-01 已继续接线显式 Run 级物理限额（调用/能力操作），复用现有 allocation/consumption ledger；逐 CWI 工作范围、
总迭代和草稿/诊断/补充等业务额度仍未完成，不能据此开放 CL-03。默认模板没有切换或提高预算。

后续可选逐原 CWI proof-batch 限额复用 semantic 当前/退休批次，已接线准入、SM-3 消费、重启和现有耗尽交付；它是
CL-01/08 的有限子切片，不是 CL-03 一般实现迭代，不能把 planning 批次数当作工作完成或实质进展。

后续已接线 CL-02 的 semantic repair 因果反馈及只读全量 CWI 进度上下文，以及 CL-08 的 semantic 预检/session 顺序与 session writer 当前意图
准入切片。各次实际验证见 [实施条目](implementation-plan.md#cl-08)；完整任务索引仍未关闭，不把安全拒绝或请求恢复
当作一般开发工作循环、验证输入草稿/纯缺项补充或真实部署验收已完成。

CL-02 还已接线 Run 内已提交操作的有界历史反馈：成功工作与失败回执通过原 typed request 进入 C4，明确省略数和
历史绑定身份；它补足普通请求的执行事实，不把 owner accepted 当作验收，也不开放 CL-03 的一般实现权限。

历史反馈进一步区分封存的预期作用/请求对象与实际回执，并与 exact repair feedback 共用 owner diagnostic 窄投影，
补足故障码、revision、来源 hash、截断/脱敏和 typed validation diagnostic。仍不是完整诊断材料读取、一般工作授权
或 CL-02/03 整项完成；自报退出码不产生执行证明和验收 credit。

CL-02 的 Run 物理预算及可见原 CWI proof-batch 用量也已进入 C4，复用现有账本、明确 pre-submission 时点与 null
附加上限；unknown/重启/退休不刷新余额。该反馈不替代 CL-01 的一般工作合同或 CL-03 实施循环。

勾选完整任务必须附提交号（未提交时明确标记工作区变更）、受影响 SM、实际执行命令、结果与证据索引；文档检查不能计作代码完成。真实模型或部署测试未执行必须标为未验证。
