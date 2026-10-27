# STEP04 no-Run 原观察与最终 CAS 栅栏验证记录

日期：2026-10-26。源码基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交修改，不能以 HEAD 代替本轮已测源码。范围为用户要求的“继续收口除 Provider 外的其他遗留项”。本轮不改 Provider、不调用真实推理或 embedding、不启动 MCP、不提交生产执行意图，不进入 STEP05。

## 本轮关闭的代码项

将已保存的完整 Run owner 原历史，与原结算时点的真实 Workspace CAS 前像联合裁决。沿用已批准的 no-Run v1 reservation/epoch 边界，不新增 owner、topic、跨 owner facade 或 Runtime authority。

- 仅原 accepted 操作可以编译 `workorch-preparation-no-run-final-fence/v1`。必须有完整原空 Run 历史，以及可独立反查全部来源的精确封存、持久完整性两个 passed 项；仅空库存或 ActiveRunID 为空不能通过。
- 私有 Run-history archive 保存完整 detached `FinalFence`：原 Workspace revision、ActiveRunID 和完整 creation/reservation fence。必须为同一原 request/payload/binding、expected+1 前像、expected+2 提交、同 creation epoch、preparation epoch=0、无 pending/settled、ActiveRunID 为空。
- Workspace 私有 reducer 将 archive、版本和 `first_activation_no_run=passed/run_history_empty_final_fence_observed` 同一次 CAS 提交。Repository 独立核对完整持久材料，并比较实际 CAS 前像，不能用调用者编译结果或重算 hash 替代。禁止同一提交引入 Run 或改变 reservation fence。
- 新版版本／引用／封存／检查身份缺失、孤立版本、unknown、历史 Run、retirement、legacy、revision 或 epoch 漂移均不能取得该单项通过。原无 FinalFence 的历史仍按旧阻塞语义读取，不自动升级。
- 结算先赢时保存历史通过；其后的 fresh reservation 保留全部原材料和 passed，并使 currentValidity stale。reservation 或 metadata CAS 先赢时，旧结算 CAS 失败，重新读取后按 binding_changed 保存历史、不授通过。
- 写前失败后的显式恢复不能补造观察或通过；写后丢回执只重放持久原结果。GET、同键重放、恢复、owner/repository 重开都不重取原资源、不发执行。
- Go/TS 严格公共 DTO 仅增加有限 `noRunEvaluationVersion`，不输出完整 fence 或内容 hash。Web 将“原 no-Run 结算栅栏”、历史逐项结果、currentValidity 和真实 ActiveRunID 分轴展示，明确不是“当前没有 Run”。

总体操作仍为 **blocked**；这不是所有资源 ready、首次历史 M3、Start/Restart 回执消费或执行授权。Runtime SM-1～SM-4 无迁移变更；改动属于 Workspace 准备协调账本及其私有 CAS、查询投影。使用 go-multi-module-dev 技能核对 owner、concrete typed EventHub 与持久边界。

已落地规范入口：`internal/modules/kernel/workspace/readme.md`、`docs/50-product-interface/service-api.md`。候选批准记录不替代它们。

## 验证与证据

受限原证据根：`/tmp/workorch-step04-norun-final-Gt83V4`。临时证据不是长期归档；源码快照、产物、原日志不提交 Git。没有读取或修改用户 live 配置、现有服务或外部 SkillHub 项目。

| 检查 | 结果及证据 |
| --- | --- |
| 累计受影响 Go 回归 | 24 个有测试包通过、3 个包无测试。`owner-go.log` 中 18 包通过，Workspace biz 因沙箱 socket 权限中断；获准相同隔离测试后，`workspace-go-final.log` 全包通过（104.759s）。`application-go.log` 另外 5 包通过，含 Application biz（63.743s）和 server |
| 最终 no-Run 定向回归 | `targeted-final-all.log` 三包通过；含新增模型、三种 repository、重哈希篡改、CAS 先后、写前/写后回执丢失、原恢复及 owner 重开 |
| 最终定向 race | `race-final.log` 四包通过：Workspace models/biz、Application biz、managementcontract；含真实回环 HTTP 五场景与 no-Run 原观察／竞争／恢复 |
| Run owner 原观察 | `TestPreparationRunHistoryArchiveActualOwnerAndStoppedOwnerReplay` 使用真实 Run owner；封存完整空历史和最终 CAS 前像。停止 Run owner 后同键重放／恢复不查询它，准备不创建 Run |
| 公共 HTTP | 现有 `TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen` 五场景更新断言：完整来源时新版本 no-Run 单项 passed，但总体 blocked；原完整 archive 仍保持其空历史原事实。真实 listener＋持久 owners 的回归/race 通过，不冒充最终二进制部署验收 |
| Web | `web-targeted.log` 63 项通过；`web-components.log` TypeScript 检查及 13 文件 192 项组件测试通过；`web-rules.log` 11 项规则测试通过。含已有 Run/stale 与历史通过并存、畸形版本／引用／owner／required／epoch／状态拒绝，零自动 check/recover/执行 POST |
| 静态/边界 | `vet-final.log`、`fmt-final.log`、`diff-check.log` 通过；`boundary-final.log` 通过，最终快照另复验见 `boundary-final-snapshot.log`。保留既有 pycache binary-match 提示，没有豁免规则 |
| 构建身份脚本 | `build-identity-tests.log` 17 项通过 |
| 生产 assets/二进制 | `console-build.log`、`build.log` 通过。保留既有 Ant Design/chunk 及 Go module stat-cache 只读告警；不是构建失败 |
| 内容身份 CLI | `source-snapshot.json` → `build.log` → `build-identity.json` / `build-produce.log` → 固定四字段 `expected-identity.json` → `build-verify-cli.log`，CLI 实际 verify 通过 |

主要命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/blocks/run/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/repository/... ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/biz -count=1
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/skillhubpreparation -count=1
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/workorchmanagementcontract -run 'TestPreparationNoRun|TestPreparationRunHistory|TestPreparationObservation|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen' -count=1
```

首轮失败保留：`initial.log` 为旧断言仍把完整 final-fence 正例按空历史阻塞处理；`targeted.log` 有新增测试误用不存在的 Workspace.Name 编译失败；`targeted-final.log` 有隐私测试将公共 expectedWorkspaceRevision 的子串误判为私有字段；已分别修正明确测试语义／字段名／精确私有字段断言，没有削弱仅空历史的旧负例。`owner-go.log` socket 权限与 `boundary.log` 默认 Go cache 只读是环境失败，不能记录为成功；上述最终回归使用获准回环权限及 `/tmp` GOCACHE 后通过。

## 最终本地内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终累计未提交源码。
- buildInputSha256：`4601046ede9ee79b773ec189ac3010c406cb60e4f72b6ea8e04803598e6f6bed`
- consoleTreeSha256：`bbb286c2049a174751381d8ef31def1a81fd112de28cb9002ae25fa688a6d848`
- binarySha256：`de8bc2c222dfbf986a06e47cca2165bec19f03c619a11ba2cd4cf0301b3a0d98`

上述是本地字节一致性，不是可信构建 attestation 或部署来源。没有沿用上一轮 Skill 状态诊断产物的验证结果；报告不在构建输入扫描范围。

## 仍未关闭

1. Skill Registry 辅助引用缺少 owner-defined 唯一文件根；持续 freshness、最终 currentness、daemon/local 来源一致性缺少完整真实证明。现行 status 诊断及历史双读不能替代这些事实。
2. MCP required connection 的真实准备态事实／epoch，以及空选择的精确适用策略。现有阶段禁止启动/probe，不能创建 Run 补证。
3. 非空 semantic Memory 的精确 query、target、预算及真实语义证明。本阶段禁止新增 embedding，不能改成 list-only 凑通过。
4. 全 required all-of、首次历史 M3 的生产成功结算及完整竞争矩阵。本轮只关闭其中 no-Run 原观察与 final CAS 的单项链；Provider 保持原阻塞，不能绕过其他 required 项。
5. 最终二进制的生产 HTTP、实际 Chrome 和独立 OS 进程 stop/start；本轮没有执行这些，不以组件、回环测试或 owner 重开替代。
6. 独立 public fullflow Profile 断言、Orchestrator 累计成本及全 CI 问题；未重跑全仓 CI，不能宣称全 CI 或完整 STEP04 通过。

没有自动提交修改。
