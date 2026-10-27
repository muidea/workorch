# STEP04 SkillContext 预算编译与恢复验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本记录只证明本轮预算子链及所列自动验证，不表示完整 STEP04、M3、全 CI 或发布验收通过。

## 本轮收口

原准备操作的 Workspace 私有 reducer 在完整组合选择成功后，以选中完整 Content/provenance 调用既有纯 `SealRuntimeSkillContexts`，生成版本化 `workorch-preparation-skill-context-compilation/v1`。材料嵌入原 Registry archive 的 Selection.Context，与原 blocked 结算同一次 CAS，不新增 topic、外部效果、结算 revision 或调用许可。

- 复用现行 5 项、正文单项 8 KiB／合计 24 KiB、metadata 16 KiB 合同，不另造预算。既有规范化、有界裁剪只作用于独立预览；完整安装 bytes、选中 Content、原来源和原 status 不变。
- 私有材料保留完整投影、精确预算／裁剪记录，或原失败诊断。错误不超过 64 KiB 时保存完整 bytes、字节数及 SHA256；超界不保存前缀，显式记总字节数、完整 SHA256 和未保存原因。
- Workspace 从全部原来源重新选择、重新编译预算并比较完整值；改投影、预算、诊断或删除 summary 后 rehash 仍拒绝。不能以 summary、hash 命中或调用者成功标志授信。
- `Context=nil` 使用 omitempty 保持既有无预览 archive 编码，旧历史可以加载但不自动补算；即使删除 Context 和 summary 后形成合法旧结构，也不能通过 CAS 改写已结算历史。
- 公共 `skillContextState` 只允许 v2 blocked 且具有完整组合选择来源；分为 `compiled_checks_pending`、`bounded_checks_pending`、`rejected`。HTTP、TS 校验与 Web 独立分轴展示，不输出私有正文或原诊断，不授资源 PASS、ready、freshness、M3 或执行权。
- GET、完成重放、原操作恢复及 owner 重开不重新执行 status、不补读或补编译。写入失败不会凭恢复补造预算；已落盘但回执丢失则保留原预算和原身份。

按 go-multi-module-dev 核对，本轮仅扩展 Workspace 协调材料、Application 映射和 Web 展示，没有新运行单元、跨 owner 直写或 service facade。没有改变 Runtime SM-1～SM-4、Start/Restart 授权或执行 reservation，不进入 STEP05。

## 验证与证据

受限证据根：`/tmp/workorch-step04-context-evidence-WlpQlz`，目录权限 0700；原日志与二进制不提交 Git。临时目录不是长期归档，后续独立复核需另行保全。

| 检查 | 结果及原始索引 |
| --- | --- |
| 受影响 Go 全包回归 | 21 个有测试包通过，2 个无测试包；`affected-go.log` |
| 模型／owner 切点／management 定向 | `owner-context-cuts-final.log`；正常、bounded、rejected 各自覆盖正常结算、写入失败、落盘回执丢失与 file-owner 重开 |
| 定向 race | 六个实际运行测试的包通过；`race.log`，包含预算模型、选择、原操作切点、HTTP 与公开 DTO |
| 真实回环 HTTP＋file owners | `http-workflow.log` 及最终 `affected-go.log`、`race.log`；原材料移走后 GET/check/recover/reopen 投影不变、status 仍合计 2 次、Soul/Registry 实读各 2 次 |
| Web 全量组件／规则 | 13 文件 146 项组件测试、11 项规则测试通过；`web-components.log`、`web-rules.log` |
| 边界／vet／格式／diff | `boundary.log`、`vet.log`、`fmt.log`、`diff.log` 均通过 |
| 生产 Web 构建 | `web-build.log` 通过，保留既有 chunk 大小告警；内嵌产物已刷新 |
| 本地源码／产物身份 | `source-snapshot-final.json` → `build-final.log` → `build-identity-final.json`／`build-produce-final.log`；公开 Python `verify()` 的固定四字段核验为 `build-verify-function.log` |

受影响回归命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/pkg/skillselection ./internal/pkg/skillhubpreparation ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistrycore ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/skillselectcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/skillselection ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/skillselectcore ./internal/pkg/workorchmanagementcontract -run 'TestExactSelection|TestPreparationSkillContext|TestPreparationSkillSelection|TestPreparationSkillRegistry|TestPreparationSkillHub|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestTypedSelection|TestSelection' -count=1
```

模型断言包含五份大型 UTF-8 正文的公平预算、原文不变、全部 slice/Budget/诊断 clone 隔离、合法空选择、既有 CRLF/NUL 规范化、空正文／非法 UTF-8／重复 ID／超项／缺 provenance／metadata 超限拒绝，以及超界错误明确未保存。HTTP 篡改反例包括投影、预算、诊断、summary 缺失及删除历史预览；即使重新生成 archive hash 仍不能绕过全值反证或历史不可变门禁。旧无 Context 材料的独立持久校验与禁止改写完成历史分别断言。

HTTP 使用本地 provider fixture 准备候选，准备检查不新增模型推理／embedding、不产生 Run。global 安装根显式指向临时目录，没有读取用户实际安装指令或 live 配置，也未改现有服务及外部 skill-hub 项目。owner 关闭重开不是独立 OS 进程重启；本轮没有生产 Chrome 验收。

### 失败与修正

- `owner-context-matrix.log`：新增拒绝夹具以超长名称触发 metadata 上限，先违反安装 metadata 合同，不能到达 context 预算门禁。改用每项合法而合计超限的 constraints，不调整生产校验。
- `owner-context-matrix-corrected.log`：第一版 constraints 全部相同，经既有显式 context 规范化去重后并未超限，测试期望错误。改用唯一 constraints，保留既有规范化规则。随后 `owner-context-matrix-final.log`、新增完整切点矩阵及最终全包／race 通过。
- Go build 有 module stat-cache 只读告警但退出 0。`build_identity.py verify` CLI 的既有问题本轮未修复，不记 CLI 通过；使用公开 Python `verify()`。未重跑全仓 CI，不称全 CI 通过。

## 本轮最终本地构建身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终未提交源码，不是单独验证该干净提交。
- buildInputSha256：`da28a4772401146ed322ac01cd251a062bdf5dd7fe1c5f9bd4c4003b622951c1`
- consoleTreeSha256：`6ccaba4392f91a9b61a0c59858fd29d937604a7dec90ee77797a15c9d4097a6c`
- binarySha256：`a0fb20e02445560ebf986e1ff7badc96221d898eead10e37958aa9003179aac7`

此身份只支持本地输入／产物字节一致性，不是可信构建 attestation、部署或验收证明。不得沿用上一轮二进制结果。

## 尚未退出的门槛

1. 选中 Skill 辅助引用的真实资源读取、来源状态／freshness、daemon/local 一致性；历史选择和预算预览不能替代。
2. MCP／Memory／provider 等 required owner 的真实观察及原操作持久消费。
3. 全 required 合取、no-Run／epoch／final CAS 与首次历史 M3 的生产正结算；当前仍无 ready/M3 正回执。
4. 对应竞争／中断矩阵、生产浏览器和独立 OS 进程重启证据。
5. 独立跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计套件耗时及 build-identity CLI 问题。

完整 Start/Restart 准备回执消费属于 STEP05。当前预算子链已完成所列代码和验证，完整 STEP04 尚不满足 live 退出条件。未自动提交改动。
