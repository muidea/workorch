# 软件开发交付物

产出结构化交付物，面向最终交付而不是过程日志。

## 交付物清单
- source: unified_diff 或补丁交付物（Patch Artifact），说明修改文件、关键变更和范围控制
- test_report: 测试命令、退出码、pass/fail/skipped 或等价摘要
- configuration: 如有配置变更一并提交并说明影响
- version_control_snapshot: 如 workspace 输出发生有意义变更，记录 git/write add/commit 证据、commit hash、提交信息和相关路径；无可提交变更时说明 zeroWork 或跳过原因
- coverage_map: Goal 叶子需求、DoD 条目、验收维度、evidenceRefs、git 快照证据与残余风险的映射
- final_summary 由 runtime 交付物（Artifact）记录：目标（Goal）达成情况、实现总结、验证总结、已知限制和残余风险
- deliverable_manifest 由 runtime 交付物（Artifact）记录：所有交付物索引、DoD 条目证据引用和可追溯 URI

## 交付物质量要求
- unified_diff 只能包含与 Goal 直接相关的文件变更
- 最终结果必须包含：改动摘要、变更文件、行为变化、验证命令、验证结果、失败/跳过说明、残余风险和后续建议
- 测试报告必须说明实际运行命令、覆盖的 DoD 条目、Goal 覆盖项、主路径/错误路径/入口路径/配置路径/并发路径覆盖情况；未运行或跳过时必须说明原因和影响
- 不要包含 build 产物二进制文件，除非 Goal 明确要求并且 runtime artifact policy 允许；若 workspace 中存在构建产物、日志或临时文件，应说明是否属于交付范围
- 不要包含 node_modules、vendor 等第三方代码
- 不要只写“已完成”而缺少可验证内容

## 禁止
- 不要通过 patch/apply 或 patchRequests 写入 result_summary.md、final_summary.md、deliverable_manifest.md、evidence_chain.md 等 runtime 管理的报告文件
