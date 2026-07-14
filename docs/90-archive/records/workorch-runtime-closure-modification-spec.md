# WorkOrch Runtime Closure Modification Spec

Status: active

Last Updated: 2026-06-15

## Purpose

本规范用于收口 WorkOrch 运行期问题时的修改与审核。目标是避免在 prompt、framework、TUI、projection 和 runtime repair 之间反复绕圈，确保每次修改都直接命中已确认的 blocker，并保持 WorkOrch core 业务中立。

## Scope

适用于以下变更：

- `internal/modules/kernel/orchestrator/**`
- `internal/modules/blocks/**`
- `internal/modules/application/workorchd/**`
- `internal/services/workorch/**`
- `internal/pkg/workorchclient/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchdefaults/defaults/**`
- Web / TUI projection、命令提交、状态刷新、LLM context/cache、runtime repair、watchdog、CPU/IO 收口

## Required Closure Flow

1. 先记录真实执行事实。
   - workspaceId、runId、current step、decision/approval id、failureClass、failureSignature、最近事件时间、当前 projection state、最新 workspace diff。
   - 明确问题属于：调度未推进、LLM/provider 失败、runtime 合同失败、validation 策略失败、projection/TUI 展示失败、端侧命令未提交、CPU/IO 扫描过重。

2. 再判定修改落点。
   - Core：状态机、coverage ledger、repair/replan、projection、EventHub、BackgroundRoutine、REST/port、block 合同。
   - Template/Soul/Profile：软件开发验证习惯、服务/代理测试策略、LSP 使用顺序、版本快照、测试与提交证据要求。
   - Live workspace：只能作为样本与验证对象，不能把其文件名、端口、语言、框架或目标文本写入 core。

3. 修改必须命中 blocker 路径。
   - 说明旧失败进入哪个函数或合同。
   - 说明新代码如何分类、拒绝、恢复或投影。
   - 说明下一轮自动推进路径是什么。

4. 补测试和文档。
   - Core 合同变更必须补单测或集成测试。
   - Template/Soul 变更必须补默认资源测试或说明绑定到 executionContext 的路径。
   - TUI/Web 变更必须证明端侧只消费 workorchd projection，不自行推导业务状态。

## Runtime Contract Rules

- `need_validation` / concrete validation evidence 合同只接受 bounded validation evidence graph、真实治理 blocker，或带 concrete evidenceRefs 的 satisfied assessment。
- concrete validation graph 可包含只读辅助节点，但必须至少包含 `shell/command`、`mcp/tool_call` 或 profile/capability 显式允许的等价验证动作。
- `shell/command` 验证节点必须声明 `validationEvidence.mode` 与 `validationEvidence.observationType`。
- concrete validation shell 不允许 `nohup`、`setsid`、后台 `&` 驻留、`kill` / `pkill` / `killall` / `fuser -k`，也不允许用进程终止 approval 作为验证策略。
- 服务、代理、流式、长运行入口的具体验证方式属于 profile/template/Soul：优先项目测试 harness、前台 timeout harness、临时端口、deterministic fake/stub、fixture 或等价 MCP/tool 证据。

## MVVP And Projection Rules

- workorchd 是 workspace/run/view projection 的 ViewModel owner。
- Web/TUI 只负责命令输入、状态显示和流式交互，不维护第二套 workspace 状态机。
- TUI prompt、workspace list、main view 必须来自同一个 port DTO/projection revision。
- projection 未 ready 时应显示 projection health 和可用 partial detail，不应触发端侧重型聚合。
- request handler、TUI Presenter、Web 页面不做跨 owner repository fan-out；重型读取由 projection manager 或后台任务有界执行。

## CPU / IO Rules

- 禁止在高频 HTTP/TUI 刷新路径中全量扫描 workspace、run events、steps 或 evidence。
- projection rebuild 必须按 workspace/run scope 去重；不同 workspace 不共享无必要全局 lane。
- watchdog 只能触发必要的有界恢复或 projection repair；不能每轮全量读写所有 workspace。
- LLM context 组装必须使用 compact refs、projection 和必要 evidence，不得反复读取大文件或历史 raw 输出。

## Review Checklist

- 改动是否直接命中最新 blocker，而不是只改善相邻 prompt 或文档？
- Core 中是否新增了模板、语言、命令、业务对象、端口、供应商或 live workspace 文件名特判？
- Template/Soul 规则是否能通过 workspace activation 进入 run executionContext 或 LLM input？
- EventHub command 是否按 owner `pkg/events` 定义，调用方是否检查同步 response？
- TUI/Web 是否仍只消费 workorchd projection/port DTO？
- concrete validation graph 是否有非破坏性 bounded validation 约束？
- 受影响包测试、全量测试或 e2e 验证是否覆盖旧失败路径？

## Skill

对应审核 skill：

- `/home/rangh/aispace/.agents/skills/workorch-core-template-boundary/SKILL.md`

后续涉及 WorkOrch core/template 边界、runtime repair、projection/TUI、LLM context/cache、CPU/IO 或 live workspace 收口时，必须先使用该 skill。
