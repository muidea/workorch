# 开发待办

本文件只记录后续工作和启动条件，不是现行设计合同或已支持能力清单。
现行设计仍以 [设计入口](docs/00-core/overview.md) 和各组件合同为准。

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
