# STEP04 Skill 来源状态诊断与构建身份 CLI 验证记录

日期：2026-10-26。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动，不将 HEAD 等同本轮已测源码。用户范围为“继续收口除 Provider 外的其他遗留项”。本轮不改 Provider、不调用真实推理或 embedding、不启动 MCP、不创建 Run、不进入 STEP05。

## 本轮完成

### 原选中 Skill 来源状态的持久诊断

在原 Workspace 私有结算中，从完整选择及完整 SkillHub effect 归档独立编译 `workorch-preparation-skill-status-compilation/v1`：

- 每条选中来源保存原 ObservedStatus 与有限诊断；按真实 scope/sourceSkillID、原 status、双读安装 bytes 反查，不借用相邻技能或其他 agent 来源。Registry 行只保留 owner 原观察，不伪造 SkillHub 状态。
- 非精确 `synced` 的状态（包括 modified/conflict/空/未知及大小写空白变体）明确为 status_blocked，原状态不变，不自动 repair。
- 实际安装 frontmatter 与非空 status local_version 不同为 version_conflict；不以仓库 version 替换本地版本。缺少一侧版本不证明一致，也不授版本完整性 credit。
- 即使为精确 synced，也仅为 observed_freshness_pending；材料缺失或来源不匹配为 sources_incomplete。上述四类都不是资源 PASS、持续有效性、daemon/local 一致性、ready、context 或 M3 证明。
- 原选择 archive 内保存完整诊断，同原操作同一次 CAS。读取时独立从全部原依赖重编译并比较；新字段 nil 时省略，旧历史不自动补算。完成历史不可删除或改写，GET、重放、恢复、owner 重开均不重跑 status、不补读。
- HTTP/严格 Go 与 TS DTO/Web 增加独立 skillStatusState 展示；非空字段仅允许 v2 blocked、完整组合选择来源和有限枚举，公共值不包含原状态、版本、路径或诊断正文。

按 go-multi-module-dev 核对，未新增运行单元、topic、跨 owner facade 或资源直写；仍由 Workspace 私有 reducer/CAS 维护协调事实，三个 state-like 字段按派生投影／不可变 trace 精确登记。Runtime SM-1～SM-4、执行 reservation、Start/Restart 授权均未改变。技能指引用于约束 owner/事件/持久化边界，没有把诊断投影变成执行权威。

这关闭的是“原状态与安装版本差异未被精确诊断、持久消费和展示”的代码项，不等于完整 Skill readiness 已关闭。当前 skill_selection 检查仍 blocked。

### 构建身份 CLI

修复 `scripts/build_identity.py` 的四字段外部固定身份处理：先完整验证，但返回独立四字段原值，不把 schemaVersion 注入外部 pin。此前 CLI 读取后添加 schema，传入 verify 再次校验时被自身四字段合同拒绝；直接 Python verify 调用没有覆盖该双重入口。

不放宽字段、hash、commit、scope、路径、symlink 或构建前后检查。新增真正 subprocess CLI 正例，逐一漂移四字段及改写二进制的反例。既有重复键、缺失/多余字段、UTF-8、非有限数及路径检查保留。本轮最终产物使用修复后的 CLI 实际核验，不再仅记录公开 Python 函数通过。

## 验证与原证据索引

受限证据根：`/tmp/workorch-step04-nonprovider-vW7B3r`，临时目录权限 0700。日志、源码快照、固定身份与二进制不提交 Git；临时目录不是长期归档，独立复核需另行保全。

| 检查 | 结果与日志 |
| --- | --- |
| 累计受影响 Go 回归 | 20 个有测试包通过、2 个无测试包。`affected-go.log` 首轮其他 19 包通过，Application 因下述测试 fixture 缺字段失败；修正后全 Application 重跑通过，`application-go-final.log`（59.947s），不能将首轮总命令写成成功 |
| 定向模型／DTO | `targeted-go.log` 通过；涵盖精确 synced、modified、conflict、missing、空／未知／大小写／空白、版本冲突、缺版本不授 credit、缺二轮／nil 来源和深复制 |
| 定向 race | `race.log` 中 Workspace models、Workspace biz、managementcontract 三包通过，首轮 HTTP 同 fixture 失败；修正后 Application HTTP race 通过，`http-race-final.log`（35.227s） |
| 真实回环 HTTP 与 file owners | 五个场景通过，`http-corrected.log`（6.346s）及最终 Application 全包／HTTP race；覆盖 status_blocked、version_conflict、synced 仍 pending、文件移走后 GET/check replay/recover/owner 重开不改变原材料、篡改原状态／诊断／summary 或删除材料后重算 hash 仍拒绝 |
| Web | TypeScript 检查及 13 文件 191 项组件测试通过，`web-components.log`；11 项规则测试通过，`web-rules.log` |
| 构建身份脚本 | 17 项测试通过，`build-identity-tests-final.log`；包含真实 CLI subprocess 成功与四字段漂移、二进制漂移拒绝 |
| 静态／边界 | `boundary.log`、`vet.log`、`fmt.log`、`diff-check.log` 通过，未豁免扫描规则；boundary 保留既有 pycache binary-match 提示 |
| 生产控制台及二进制 | `console-build.log`、`build.log` 通过，内嵌 assets 已更新；保留既有 Ant Design／chunk 及 Go module stat-cache 只读告警 |
| 构建身份 CLI | `source-snapshot.json` → `build.log` → `build-identity.json`／`build-produce.log` → 固定四字段 `expected-identity.json` → `build-verify-cli.log`，退出 0 |

受影响 Go 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/skillhubpreparation ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/application/workorchd/biz -count=1
```

race 与 HTTP 最终命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/workorchmanagementcontract -run 'TestPreparationSkill|TestPreparationSelectedSkill|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen' -count=1
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/application/workorchd/biz -run '^TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen$' -count=1
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/application/workorchd/biz -run '^TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen$' -count=1
```

首次 HTTP/race 的失败源于 MCP 场景会重新覆盖 installedRaw，遗漏新增“版本冲突”断言所需的 frontmatter version。该场景缺少一侧版本时不能被标成已观察到冲突，生产分类行为正确；已补齐 fixture 的显式版本，未放宽生产门禁。原失败保留在 affected-go.log 和 race.log。

HTTP 使用真实回环 listener 与持久 owner，但部分资源及候选准备仍用明确的本地 fixture，不能冒充全部真实资源 owner。保留原调用计数约束，准备检查不新增推理/embedding，Run 保持 0。未读改 live 配置、现有服务或外部 SkillHub 项目。owner/repository 重开不是独立 OS 进程重启，组件测试不是实际 Chrome。没有运行最终二进制的部署 HTTP／Chrome／OS stop/start；不复用旧产物的部署结果。未重跑全仓 CI。

## 最终本地内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终累计未提交源码。
- buildInputSha256：`b45c445cd03833ae98bcd0dc3563d2b56b30abf1626b1cd223a028a8a79e4388`
- consoleTreeSha256：`c50be7d96d6843f2495f7b5a869848c7c81b45f75c32153c742ddc2ae3ec961e`
- binarySha256：`afbf974a58775db012cc75491f32437b6a9720c32e6d8bf888bb2ca790b9facd`

本地固定身份只证明源码／控制台／二进制与清单的字节一致性，不是可信构建 attestation、部署来源或验收授权。报告不在构建输入范围。

## 仍未关闭的门槛

1. Skill Registry 辅助引用的 owner-defined 唯一文件根，以及持续 freshness、最终 currentness 和 daemon/local 来源一致性的真实证明。本轮诊断不能替代它们。
2. MCP required connection 的真实事实／epoch；空选择仍缺“不适用”的精确策略证明，不能由空集合直接授 N/A。
3. 非空 semantic Memory 的精确 query／target／预算与真实语义证明；本阶段禁止新 embedding，不能改 list-only 凑通过。
4. 全 required all-of、no-Run reservation/epoch 有序观察与 final CAS、首次历史 M3 生产成功结算及竞争／中断矩阵。Provider 按用户范围保持不变，其阻塞不能被其他检查绕过。
5. 最终产物的生产 HTTP、实际 Chrome 与独立 OS 进程重启验收。完整 STEP04 尚未达到 live 退出条件。
6. 已披露的独立 public fullflow Profile 断言及 Orchestrator 累计成本／全 CI 问题，本轮未处理；构建身份 CLI 的上述具体缺陷已修复并验证，不再沿用此前“CLI 未通过”的旧结论，但不宣称所有构建／CI 问题都已消除。

本轮没有自动提交修改。
