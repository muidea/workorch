# Orchestrator Code Audit Alignment Closure

Status: active

Type: closure-plan

Last Updated: 2026-07-06

## Contract Audit

- Scope: `docs-product/90-appendix/orchestrator-code-audit-report.md` vs current `workorch/docs` and code.
- Observed state: `docs-product` 是审计输入，不是当前会话的权威文档；当前权威合同位于 `docs/05-product/`、`docs/10-domain-model/`、`docs/30-module-architecture/`、`docs/40-profiles-capabilities/`。
- Expected state: 代码、产品架构、领域模型、数据流合同必须以同一业务语义收敛；不能只按 `docs-product` 删除能力，也不能只按现有代码反向刷新文档。
- Deviation: 报告中混合了真实代码缺陷、官方 docs 未完全表达的实现语义、以及 `docs-product` 自身过严或矛盾的判断。
- Owner: kernel/orchestrator、kernel approval/governance、common state contract、official docs。
- Event path: audit item -> active architecture decision -> code/doc classification -> targeted closure or questionable summary。
- Contract gap: canonical 状态归一覆盖不全、CoverageWorkItem 仍接受旧终态别名、Approval 创建缺少 auto-governance 审计 seed；同时 patch/file side-effect owner 与事件 topic 粒度需要继续按模块合同独立收口。
- Proposed change: 本轮只收口确认属实且低风险的代码缺陷；对官方 docs 中未表达清楚但与当前架构相符的语义做刷新；对正确性存疑或需要大范围迁移的项进入下方总结。
- Boundary check: 不修改 `docs-product`；不在 core 中加入模板、语言、命令或场景特判。
- Verification: 定向 Go 测试、`make fmt-check`、`make verify-boundary-contract`。

## Classification Matrix

| Item | Active decision | Classification | Action |
| --- | --- | --- | --- |
| F-05 / F-17 replan lineage | Run model 已有 `Lineage`，但 `replanLoadedRun` 仍通过 `ExecutionContext.replanContinuation` 承载部分恢复上下文 | code gap, not fixed in this batch | 需要单独做 run lineage migration，避免破坏 replan/resume 恢复链 |
| F-07 kernel file I/O | 产品原则是 blocks own external side effects；patch 当前存在历史 controlled executor 例外，但 repair target snapshot 直接读文件不应长期留在 orchestrator | confirmed architecture/code gap | 进入独立 P0 migration，不在本轮用小补丁伪装完成 |
| F-08 / N-01 event topics | 当前正式读模型主要消费 owner aggregate notification 与 projection；versioned topic 表示可识别合同，不等于每个状态转换必须同时发布离散 topic | docs clarification | 刷新 Event/Observability 文档，后续如产品需要离散 topic 再做 publisher 收口 |
| F-09 CoverageWorkItem status | 官方状态集是 `open / in_progress / evidence_ready / satisfied / failed / blocked`；`closed / completed` 不应作为 canonical terminal | code defect | 移除旧终态别名，补回归测试 |
| F-10 / F-11 / F-23 | ConvergenceContract 与 Recovery Action 是不同抽象层级：业务合同 vs runtime 细化动作 | docs-product stale | 不改代码 |
| F-12 MemoryRecall source | 当前生产写入使用 `activation / step_input / recovery`，符合 memory recall source 语义 | no defect | 不改代码 |
| F-14 Approval autoGovernance | Approval model/command 支持 `AutoGovernance`，orchestrator 直接创建 Approval 时应写入可审计 seed | code defect | 创建 approval 时写入 pending evaluation seed，后续 governance trace 继续由 governance owner 承接 |
| F-19 Patch fields | 当前正式模型是 patch record: `patchType / targetRef / state / summary`；多文件展示可由 projection/affectedFiles 表达 | docs-product stale | 不改代码 |
| F-20 / N-03 ArtifactContinuation state | ArtifactContinuation 是 runtime internal state；public lifecycle 与 internal phase 可以分层表达 | docs clarification | 刷新领域模型文档 |
| F-22 ValidationResult state | 当前正式代码用 `passed / failed / inconclusive / warning`；`pass/fail` 更像产品展示标签，不宜破坏持久化状态 | docs clarification | 刷新领域模型文档 |
| F-24 comment language | 风格问题，不影响业务状态机或数据流合同 | questionable / low priority | 仅记录，不作为本轮代码缺陷 |
| N-02 canonical states coverage | canonical helper 只覆盖 run/step/workspace，不利于统一状态合同搜索 | code defect | 补齐 governance/evidence/outcome canonical helper 与测试 |
| N-04 CoverageWorkItem active | 报告描述自相矛盾；代码把 `blocked` 视为非 active，与 docs 方向一致 | audit questionable | 不改代码 |
| N-05 workspace/run approval block | 当前设计要求 approval blocker 使 workspace owner 进入 `blocked_on_approval`，避免 UI 自行推断局部阻塞 | no defect | 不改代码 |

## Remaining Focused Closures

### P0 File / Patch Side-Effect Owner Migration

当前不能用“把 `os.ReadFile` 包一层 helper”当作收口。真正收口必须满足：

1. `file/read`、`file/list`、target snapshot acquisition 通过 `blocks/file` 或正式 file capability 执行。
2. `patch/preview`、`patch/apply`、`patch/revert` 的文件落盘技术执行迁移到 block adapter，kernel/orchestrator 只负责合同准入、approval、patch evidence、Result gate 和 coverage ledger。
3. patch record、approval relation、baseHash/conflict、event topic、read-model 投影保持单一事实源，不新增 parallel patch state。
4. 所有 contract violation 输出在副作用前被拒绝，不能绕过 block adapter。

### P1 Run Lineage Explicitization

`Run.Lineage` 字段已经存在，下一步应把 replan successor/predecessor 的正式 lineage 写入 owner model；`ExecutionContext.replanContinuation` 只保留运行期恢复材料，不再承担 lineage truth source。

### P1 Optional Discrete Topic Publishing

如果产品/API 明确需要 `/run/completed`、`/step/succeeded`、`/approval/approved` 等离散 topic 作为订阅面，应以 projection/event replay 顺序为约束做 publisher 收口；不能直接在所有 state changed 后复制发事件导致重复投影和测试顺序漂移。

## Questionable Items

- F-24: 注释中英文混用不是当前导致 workspace 无法正向推进的业务缺陷。
- N-04: 报告文字称代码把 `blocked` 视为非 active，同时又说 docs 中 `blocked` 不在 active 判定中；该项作为审计问题本身存疑。
- F-07 中“`make verify-boundary-contract` 会因 orchestrator `os.ReadFile` 失败”的表述与当前脚本实现不符；脚本目前主要检查业务/profile 偏置、event contract、旧 endpoint、readmodel presenter 依赖等，不检查 `os.ReadFile`。
