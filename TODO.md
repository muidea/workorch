# 开发待办

本文件只记录后续工作和启动条件，不是现行设计合同或已支持能力清单。
现行设计仍以 [设计入口](docs/00-core/overview.md) 和各组件合同为准。

## RT-001：CWI 内开发迭代与验收闭环收口

- CL-01 工作范围观察子切片：exact CWI/Definition/Profile 与 owner manifest 绑定的只读请求及 File EventHub 适配层已建立，完整范围不被单轮目标数裁剪；独立于最终证明 target selection。尚未接入 drive/private-event consumer、aggregate 写集合和 working binding，不视为一般工作已可执行。

- 状态：已实施 CL-00/CL-01 的 Profile 无损准入与完整封存身份切片、CL-02 的 semantic repair 因果执行反馈及只读全量 CWI 进度上下文；CL-08 已补齐 semantic 预检/session 顺序及 session owner 的当前请求意图准入。完整工作循环尚未实现。
- CL-01 追加：Profile 原始配置的封闭字段、精确值类型及 merge 前/activation 双重检查；移除内置模板未消费的环境策略副本，保留实际 validation policy 与完整提示。CL-03 前置的工作范围/共享预算合同仍未完成，未开放一般实现权限。
- CL-01 物理预算子切片：显式 `executionLimits` 已接入准入封存、bootstrap policy 和既有 Run 共享账本；只允许收紧调用/操作总上限，不修改默认模板。工作范围、逐 CWI 总轮数及草稿/诊断/补充业务额度仍待实现，CL-03 尚未开放。
- CL-01/08 证明批次子切片：可选逐原 CWI proof-batch 上限复用当前/退休 semantic 批次消费，SM-3 编译前检查、精确重放零重复占额；不是 reopen 次数或一般实现迭代，CL-03 及草稿/诊断/补充业务闭环仍未完成。
- CL-01/08 持久化工作封存子切片：可选逐原 CWI `MaxDurableDispatchesPerCWI`，以原 accepted dispatch 计数，final seal 同 CAS 占额；治理恢复不重复占额，失败/失效/重启不清零。它不是通用工作授权，CL-01 工作范围与 CL-03 有界实现循环仍未完成。
- CL-01 一般工作声明子切片：独立 `ImplementationWorkPolicy` 已无损进入 Profile candidate/hash，具备 owner/operation/selector/有限轮次与输入保护校验；Workspace 与 Runtime 仍明确拒绝启用未实现协议。尚缺 exact CWI 工作意图、owner preimage、scope/binding 和轮次消费，不能计为 CL-03 已能持续实现。
- CL-01 exact CWI 投影子切片：补齐 Profile→不可变 Definition 的逐 CWI 编译及 Runtime 独立 source 反证、深拷贝/equality/重载；拒绝 wildcard/exact 冲突、遗漏/注入/漂移及合并后的非 validation-only 终结义务。没有打开执行门禁；CL-03 的工作意图→working binding→回执→同 CWI 重观测尚待实现。
- CL-02 操作历史子切片：普通成功/失败回执已通过 Run 级有界历史进入 semantic C4，带来源、提交序号和省略数，历史绑定不等于当前 credit；仍不构成 CL-03 一般实现循环或完整诊断/交付反馈。
- CL-02 预期/实测与故障事实：历史区分 sealed requested targets/expected effect 与实际回执；历史及 exact repair feedback 补齐同源 owner code/revision/hash、截断/脱敏及 typed validation diagnostic。自报退出码不冒充执行/验收，仍缺完整材料受控读取和新工作路径的反馈合同。
- CL-02 预算观察子切片：现有 Run 物理账本、原 CWI proof-batch 及 durable-dispatch 用量/余额进入 semantic C4，绑定 exact pre-submission 快照；unknown、历史批次及失败/失效封存继续占额，null 附加上限不是无限执行。一般工作范围、每轮目标选择及业务额度仍未完成。
- CL-09 当前版本反馈子切片：移除旧版 Result renderer，非当前版本在记录/aggregate/initialize/material 入口拒绝，不改写旧归档；同版本落盘后确认丢失仍须幂等交付。新工作路径的 Result/UI 与整体目标闭环仍未完成。
- 实施入口：[Runtime 开发与验收闭环收口计划](plans/runtime-development-convergence/README.md)。
- 任务范围：CL-00～CL-12，覆盖基线、单 owner 合同、事实上下文、同 CWI 开发循环、验证草稿/合取补充、诊断修复、治理预算恢复、Result/UI、当前配置准入与真实部署验收。
- 当前阶段不考虑数据兼容性：不开发旧数据迁移、旧 hash/renderer 兼容或跨版本恢复；同版本重启、幂等、预算和封存输入不变仍为必需验收，不删除现有数据。
- 单独准入：错误验证实现的不可变版本更替（CL-07）默认关闭，不扩大普通实现修复权限；原 Goal/DoD 与有效补充不得弱化。
- 完成门槛：真实 owner 正反例及重启验证、真实模型重复试验、原 Workspace 完整 Goal/DoD 和 full DCE；mock CI 或安全终止不能替代目标完成。
- 约束：不热升级旧 Run，不回填历史 payload，不向 Runtime core 注入 LSP-Bridge 业务；继续保持一个状态机一个 owner、只由 owner 内部事件触发转换。

## RT-002：EventHub 订阅可靠性与整仓回归

- 本轮反馈合同扩展回归中 appservice 包一次失败，单独全包复跑通过（43.796s）；原因尚未定位，不用复跑通过掩盖失败，也暂不归因于 framework owner。
- 整仓基线发现 `TestDriveRuntimeInputReviewV2OmissionAndRestart/after_review_failure` 在 File native preparation 入口返回 `missing observer`；完整父测试后续重复 3 次通过，原因尚未唯一确认，不能视为已修复。
- 已从 v1.5.15 升级至正式 v1.5.17，并重新生成 vendor；旧版静默超时缺陷已确定性复现，新版明确拒绝且保持原订阅状态。原诊断已升格为普通 [依赖回归](scripts/diagnostics/README.md)，与 Base 错误传播一起连续 20 轮 race 通过。
- 已修复 omission/restart 回归的兄弟子测试依赖；同步适配 Hub/BackgroundRoutine 替身、Base 必需订阅 fail-fast、端侧失败反馈和检查式停机。保持单 owner，不编辑模块缓存或依赖源码，不添加业务 retry/sleep。
- 已修正新版 Hub 暴露的异步 Run 通知 caller-context 取消问题：SM-4 admission 使用 owner lifecycle，保留同步取消、关闭/退役/source/singleflight 门禁；专项 race 3 轮通过。
- 新版服务在隔离临时目录完成两轮启动/接口验证/退出；隔离 `verify-ci-mvp-file` 全部通过（含 LLM 与 MCP stdio completed）。修正测试 owner/Hub 清理顺序后，最终 `go test -json ./... -count=1 -timeout=25m` 全仓通过，fmt/vet/boundary 与模块校验通过。依赖接入和当前整仓基线已验证；尚不能据此声称原 Workspace/full DCE 已完成，历史失败的唯一归因也未建立。

## SD-001：软件开发 template 多语言能力扩展

- 状态：延期，尚未启动。
- 当前顺序：先完成基于 Go 的软件开发与验收闭环验证，再补充多语言方案、实现和验证。
- 扩展范围：现有 `software-dev` template 的开发验证能力；保持模板身份和 Runtime core 语言中立。
- 当前代码基线：`6b17e84b`，包含 Go native readiness、行为执行观测、owner 诊断与进度统计收口。

### 启动前置条件：完整 Go 开发验证

- [ ] 在真实部署实例上完成 Goal/DoD → 项目实现 → 实际交付入口验证 → 必要的失败修复与 fresh validation → 逐项 CWI 验收 → 正式 Result 的闭环。
- [ ] 正确实现取得对应证据；错误实现、源码字符串检查、测试内替代实现、依赖绕行、零测试或跳过不能冒充功能验收通过。
- [ ] 核对单 owner 状态转换、预算边界、重启恢复、stale/duplicate 回执，以及 Result/TUI/Web 的原因和进度一致性。
- [ ] 保存可追溯的 owner receipt、逐项验收与部署验证证据，再确认 Go 基线完成；仓库单元测试或 mock CI 通过不能替代该确认。

### 前置条件满足后的收口方向

- [ ] 在 Shell owner 内收拢版本化的语言／runner adapter 合同和显式注册表，先迁入 Go 实现，再逐个增加经验证的组合；不为每种语言新增状态机或 framework Block。
- [ ] 统一项目识别、exact check 绑定、readiness、结构化执行观测、实际交付物身份、诊断和资源回收；新增语言不能只有一条测试命令。
- [ ] 补齐空 workspace 的技术选型缺失／歧义处理和多语言子项目范围绑定，避免默认选择 Go 或跨项目借用证据；不要求用户填写底层 recipe/check JSON。
- [ ] 明确各 runner 的工具链版本、依赖准备、隔离、缓存、构建产物／源码映射和受保护验收判据。会执行项目代码的发现或准备必须进入授权的受管操作。
- [ ] 复用 typed owner 事实和既有 SM-2/3/4；Shell 不选择下一动作，File 确认修改范围，Profile 声明验收与修复合同，Result 和界面只消费已裁决结果。
- [ ] 为每个受支持组合补充正反例、失败修复、旧报告拒绝、重启和回执幂等测试，按“语言＋runner＋版本＋证据能力”发布支持范围。
- [ ] 保留旧归档和已封存协议；区分已有 adapter 的配置更新与新增 adapter 实现所需的服务发布，不把 Soul 更新当作新增解析能力。

后续方案参考现行 [软件开发验收合同](docs/40-profiles-capabilities/software-dev/goal-coverage-and-dod.md)
和 [Shell owner 合同](internal/modules/blocks/shell/readme.md)。本待办不表示多语言、通用可信 grader 或完整 release DCE 已实现。
