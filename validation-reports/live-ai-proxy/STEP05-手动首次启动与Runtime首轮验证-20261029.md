# 手动首次启动与 Runtime 首轮验证（2026-10-29）

## 结论与范围

当前生产服务的显式手动首次启动、配置模型自动封存、Runtime 初始化、首轮能力执行及评估已实测接通。唯一启动请求返回 HTTP 202，创建且关联 `run-001`；原确认、正式 Goal/DoD、StartupInput 和 activation admission 保持不变。

**本轮不是完整 STEP05 退出验收，也不是 AI Proxy 工程验收通过。** 当前执行入口仍是 intent-only，尚未实现完整准备 completion/M3 回执消费链；不能把历史 STEP04 completed+stale 回执算作本次执行授权。验证使用现有规范支持的手动 activation 路径，不补造准备证明。

观察截止 2026-10-29 08:18:26 UTC（北京时间 16:18:26），Run 仍 active，Runtime stateRevision=179，1/71 个 CWI 满足，4 个 dispatch 完成，普通 validation 接受数为 0；当前为 need_validation / semantic_plan_requested。尚未取得首次工程测试执行及修复闭环结果。本轮未发现可据此认定的具体终态失败，不能把过渡状态当作验收通过或失败。

后续观测已确认该 Run 于 08:19:27 UTC 因验证输入审查冲突进入 failed 终态；详见同目录《STEP05-首次Run终态失败与输入审查核对-20261029.md》。以下保留当时首轮观测事实，不代表当前仍在执行。

## 实际操作与部署身份

- 服务：`http://127.0.0.1:8090`，监听仅回环地址；未重启或修改用户服务。
- Workspace：`ws-8be495f73dbc315b2ab6204c85088f799e7ca6f3fb14ac9c4bb54932aeaba792`。
- 复用用户已人工精确确认的 candidate、正式 Definition 与原准备记录；没有重新 authoring 或修改准则。
- 只发送一次 `POST /api/v1/workorch/workspaces/{workspaceId}/execution-activations`，intent=`restart`。当前公开 API 的首次 fresh Run 也使用该 intent。
- requestId：`step05-first-start-72d617c3-ff89-4547-b1b5-b629fddee0ef`。exclusive attempted marker 防止脚本重复提交；没有重发、Resume 或创建第二个 Run。
- 请求未传入 provider、模型、RunID、Goal/DoD 或运行上下文，实际 Run/bootstrap 从现有配置封存 `ai-proxy/gpt-6.1-sol`。
- 实际 PID `1293404`，`/proc/1293404/exe` SHA256 为 `fe966bde39e7d4c15fc2ce6804a7797444e314d70c3c800ef4effefcc77f2bb5`。Go 构建元数据为 `0f4f2b8f0d8717e1b2e461e8d8ba63ea14ac26ba` 加 dirty，不能把 HEAD 当完整构建输入 attestation。

此前只读预检记录中的未执行事实适用于上一轮；本轮用户继续验证后才发起以上唯一启动请求。启动没有额外注入 budget，实际沿用封存默认预算；未宣称只运行一次模型调用，也未擅自停止或暂停 Run。Run 在本轮观察结束时继续执行。

## 内部事实核对

| 核对项 | 结果及适用边界 |
| --- | --- |
| 唯一激活与 Run | HTTP 202 / accepted / reconciliation_requested / new_run_activated；全局库存由 0 变为 1，Workspace ActiveRunID=run-001 |
| 原操作身份 | Run ActivationRequestID、持久 execution intent receipt 与唯一原请求一致；未凭后续 active 状态反推原回执 |
| 原人工确认与规格 | 原确认、规格 ledger、StartupInput、ActivationAdmission 的完整子树前后相等；正式 goals.json/dods.json 的 bytes/hash 不变 |
| StartupInput | Run 的 startup hash 保留 `workspace-startup-input-sha256:3c4017d939a9cc76013b122721fc2dbb78409344007177099fd31d9f8daddcd3` |
| 模型自动封存 | Run detail 与真实 bootstrap 均为 ai-proxy/gpt-6.1-sol，调用者没有注入模型配置 |
| 首轮执行 | 首个 sealed dispatch 按依赖顺序完成 8 个 file/write，全部有 accepted / workspace_durable owner receipt，execution key 唯一、fence=1 |
| 实际文件 | 8 个初始源码文件，加后续独立 input_test.go，共 9 个实际文件的 SHA256 与各自最新 File receipt AfterHash 一致 |
| 证据与评估 | 首个 source_change 交付项完成评估；仅 1 个 CWI 满足。测试输入写入并评估后，首个原 DoD CWI 仍保留普通 validation 缺口，没有被作为最终验收完成 |
| 前置条件刷新 | stateRevision 124→133→138 等观察显示 owner applicability 按 evidence revision / reconciliation generation 逐项刷新后继续推进；不是仅凭 running 文案判断存活 |
| 正式持久结构校验 | 对已保存的完整 Run 前像执行 ValidateRunRuntimeBootstrapsRaw 与 RuntimeStateRecord.Validate；stateRevision 106 和最终审计快照 169 均通过 |
| 预算来源 | 审计快照 169 中 8 个 provider call consumption 均 confirmed=1、unknown=0，具有 LLM owner audit 引用；与当时完成的 8 次真实调用数量一致 |
| 准备历史 | 查询仍为 completed+stale，未将过期观察恢复为 current |

首个 dispatch 的 8 个目标为 `cmd/gateway/main.go`、`go.mod`、`internal/archive/archive.go`、`internal/config/config.go`、`internal/gateway/proxy.go`、`internal/gateway/router.go`、`internal/observability/metrics.go`、`internal/usage/recorder.go`。

结构校验通过不能证明生成业务代码正确；文件回执证明真实修改，不等于编译、行为测试或原 DoD 已通过。各 owner 文件与 HTTP 响应分别读取并保留原始 bytes，不宣称跨文件原子快照，也不将观测调用当作 Runtime 写入权威。

## 模型用量与未完成验证

审计时已完成 8 次 provider-native responses 调用，输入共 529742 token，输出共 8566 token；8 条 preflight 记录没有真实提交，不计为另外 8 次收费调用。上述数量仅为本轮观测时点，Run 后续仍可按原预算继续调用。

这 8 次 Runtime 调用的 provider-native cacheReadTokens 均为 0。它与早先 authoring 六审的缓存命中证据不同，不能沿用之前约 98.6% 的命中率描述 Runtime。尚未完成精确请求前缀、阶段差异与网关缓存路由归属核对，不能仅据 0 命中认定同一具体根因。

仍待验证：首次普通工程 validation、真实失败后的修复与重验、剩余 CWI 及终态 Result/交付链；完整 STEP05 准备回执消费者另有实现缺口。未运行全仓测试或 CI，未改变生产代码、配置、原规格或预算。

本轮生产浏览器页面观察被 origin 权限拒绝，未取得新的 DOM 或截图，没有尝试浏览器替代入口绕过。HTTP 与 owner 核对是独立的服务端验证，不替代生产 Web 本轮验收。

## 受限证据索引

临时目录：`/tmp/workorch-validation/step05-first-start-deployed-live-20261029-01/`。

长期归档：Git 忽略的 `.workorch-data/validation/step05-first-start-deployed-live-20261029-01/`；目录 700、文件 600。归档时 95 个文件加 `evidence-index.json`，逐文件 bytes/SHA256 复核通过，不含浏览器截图。

- `activation.attempted`、`activation-intent.json`、`activation.request.bin`、`activation.response.bin`、`activation.http.json`：唯一操作及真实响应。
- `before-owner.raw.json`、`after-activation-owner.raw.json`、`final-owner.raw.json`、`run-owner-*.raw.json`：Workspace 与 Run 完整原像。
- `run-observability-*.response.bin`、对应 HTTP meta、`runs-*.response.bin`：真实投影及唯一库存，最后一次观测为 `run-observability-10`。
- `audit.py`、`audit-facts.json`、`audit.log`、`final-goals.json`、`final-dods.json`：精确保全断言与正式行核对。
- `validate_snapshot.go`、`validate-overlay.json`、`formal-validation-final.log`：在临时 overlay 下调用正式纯校验器，没有新增仓库验证入口或写 owner。
- `deliverable/`、`deliverable-files-final.json`、File receipt hash 核对：当时实际生成文件原始 bytes。
- `final-run-events-000001.jsonl`、`assessment-history.raw.json`、用量材料：原事件与原评估；不把投影当权威。
- `deployment-identity.json`、`running-buildinfo.txt`、`listener-final.txt`、`browser-access-limitation.json`：实际产物、监听及浏览器验证限制。

以上原始材料仅进入受限运行存储；本报告不携带凭据、完整请求或原始诊断。
