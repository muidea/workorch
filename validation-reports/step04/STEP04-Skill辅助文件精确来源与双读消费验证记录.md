# STEP04 Skill 辅助文件精确来源与双读消费验证记录

日期：2026-10-26。源码基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本轮按“除 Provider 外继续收口”实施，不将 HEAD 等同本轮已测源码，不表示整个 STEP04、全 CI 或发布验收通过。

## 本轮完成

- 从实际安装的完整 SKILL.md 的 `referenceRefs` 生成有界计划，仅以该技能目录为相对根。禁止 URL、绝对路径、父跳转、不规范路径、重复引用、symlink 和非普通文件；不猜 Workspace、cwd、源仓库或同名技能位置。
- Application 私有 SkillHub adapter 在两次安装指令观察之间，对辅助文件执行真实两轮读取。保留空文件、二进制、无效 UTF-8、CRLF、NUL 的原始 bytes、大小、摘要、完整性及失败事实，不执行文件内容。按 scope 最多 256 项、每文件 64 KiB、每轮累计 1 MiB；超界明确不完整，不将截断材料当作完整证明。
- 引用观察嵌入原 Installed 受限归档，沿用原准备操作、binding、reducer 和 CAS。Workspace 从完整选中 Content、provenance、sourceSkillID 和全部原始归档独立重编译、比较，不凭摘要或调用者成功标志授信。没有新 topic、运行单元、外部 status 重试或额外结算 revision。
- Registry Content 仍无 owner 定义的文件根；有辅助引用时明确 `source_unresolved`，不借用 SkillHub 来源。辅助材料失败不抹去原安装事实，也不把 `modified/conflict` 改成成功。
- HTTP、严格 TS DTO 与 Web 增加独立 `skillReferenceState`：`observed_checks_pending`、`sources_incomplete`、`source_unresolved`、`unavailable`。仅允许原 blocked 且完整选择投影的形状；不展示私有 root/path/raw，不授资源 PASS、currentness、ready、M3 或上下文消费权。
- GET、完成重放、原操作恢复和 owner 重开仅消费原持久材料；原文件移走后不补读、不重新 status、不回填已结算历史。篡改路径、删除引用或修改状态后重算 hash 仍被拒绝。

使用 go-multi-module-dev 核对责任边界：实现保留 Application 私有适配器与 Workspace 私有协调状态，跨 owner 仍走既有 concrete typed EventHub。三个新增状态字段已按实际派生投影责任登记在 state-model-owners manifest，没有扩大扫描豁免。Runtime SM-1～SM-4、Start/Restart 权限与 STEP05 均未改变。

现行合同同步至 `docs/10-domain-model/instruction-assets.md`、`docs/50-product-interface/service-api.md` 及 Workspace readme。本记录不是设计权威。

## 验证与原始证据

受限证据根：`/tmp/workorch-step04-skill-references-FCKcNv`，目录权限 0700。原始日志、快照和二进制保留于该目录，不提交 Git；临时目录不代表长期证据归档。

| 检查 | 结果与日志 |
| --- | --- |
| 受影响 Go 回归 | 20 个有测试包通过，2 个无测试包，`affected-go.log` |
| 定向回归 | 四包 `targeted-go-final.log`、Workspace `workspace-targeted.log` 通过 |
| 定向 race | `race.log` 中五个实际有匹配测试包通过；skillhubpreparation 当次无匹配用例，不计入 |
| 最终全包 race | skillhubpreparation、contextregistryports、Workspace models、managementcontract 四包通过，`pure-race-final.log`；包含最后追加的引用计划界限与原始诊断测试 |
| 回环 HTTP 与 file owners | `http-workflow.log` 及后续受影响回归、定向 race 通过。五个场景包含辅助引用存在与缺失、已有 MCP 分支、移走文件后查询/重放/恢复/owner 重开、篡改后重新计算 hash 的拒绝 |
| Web | TypeScript 检查及 13 文件 187 项组件测试通过，`web-components.log`；11 项规则测试通过，`web-rules.log` |
| 生产 Web 构建 | `console-build.log` 通过，内嵌 assets 已更新；保留既有 Ant Design/chunk 告警 |
| 静态检查 | `vet.log`、`boundary-final.log`、`fmt.log`、`diff-check.log` 通过；boundary 保留既有 pycache binary-match 提示 |
| 构建与字节身份 | `source-snapshot.json` → `build.log` → `build-identity.json`／`build-produce.log`；公开 Python `verify()` 固定四字段核验通过，`build-verify-function.log` |

受影响 Go 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/service/server ./internal/pkg/skillhubpreparation ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo ./internal/pkg/events -count=1
```

定向与最终全包 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/skillhubpreparation ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/workorchmanagementcontract -run 'TestPreparationSkill|TestPreparationSelectedSkill|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen' -count=1
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/skillhubpreparation ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/kernel/workspace/pkg/models ./internal/pkg/workorchmanagementcontract -count=1
```

HTTP 通过真实回环 listener 和持久 owner，部分资源使用明确 fixture；不能冒充全部真实资源 owner 验证。候选准备使用本地 provider fixture，准备检查不新增推理或 embedding，Run 数保持 0。未读取或改写 live 配置、现有服务和外部 skill-hub 项目。owner/repository 关闭重开不是独立 OS 进程重启，组件测试不是实际 Chrome 验收。没有运行最终二进制的生产 HTTP／浏览器／独立进程 stop/start，不沿用旧产物的结果。

### 失败与修正

- `initial-go.log`：新增模型文件未使用 import，删除后通过。
- `targeted-go.log`：测试误按固定位置截取选中项；改为按真实来源筛选，未改生产排序，最终通过。
- `boundary.log`：缺少三个新派生状态字段的 owner 登记；审核责任后精确登记，`boundary-final.log` 通过，未跳过门禁。
- `build-verify-invocation-error.log`：调用公开 verify 函数的临时命令多一个括号，Python 尚未执行函数；修正命令后 `build-verify-function.log` 通过。
- Go build 有 module stat-cache 只读告警但退出 0。既有 build-identity CLI verify 问题本轮未修复，不能写作 CLI verify 通过。本轮未重跑全仓 CI，不宣称全 CI 通过。

## 最终本地内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加最终累计未提交源码。
- buildInputSha256：`3f1453d188f04ce9a89b7bbae94d6cd10ec5ae394888336c74f2012c0a7160cc`
- consoleTreeSha256：`31eac8cc3c73b7d50a4d336b8ecdc4095da045ac0e00c2fcb26ef2079e06e6f9`
- binarySha256：`3ab795bbcb886e78752a9a80bd15bbd729ab73fd15a3e58ec92dde946f0d2c35`

上述核验只证明固定本地源码／控制台／二进制字节与清单一致，不证明可信构建 attestation、部署来源或验收授权。报告不在构建输入扫描范围。

## 尚未关闭的非 Provider 门槛

1. Registry 辅助引用的 owner-defined 唯一来源根；Skill 真实状态可接受判断、daemon/local 来源一致性、持续 freshness 和最终 currentness。历史双读不等于最终仍有效。
2. MCP required connection 的真实事实／epoch 及空选择适用性。在禁止启动／probe 的本阶段，缺少真实事实仍 blocked，不能造 Run 补证。
3. 非空 semantic Memory 的精确 query、target、预算及真实语义证明；不能偷偷执行新 embedding 或降级为 list-only 后授成功。
4. 全 required all-of、no-Run reservation/epoch 有序观察、final CAS 和首次历史 M3 的生产成功结算，以及并发／断连／迟到／漂移矩阵。
5. 在最终源码与产物绑定后补生产 HTTP 成功／阻塞、实际 Chrome 和独立 OS 进程 stop/start 证据。
6. 独立跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计成本和 build-identity CLI 等全量验证问题；本轮未将它们标为解决。

Provider 完全未改，其原有阻塞保持。完整 Start/Restart 回执消费属于 STEP05，不纳入本轮。本轮收口的是 Skill 辅助文件历史观察及精确消费链，仍不能宣称完整 STEP04 已具备 live 退出条件。没有自动提交修改。
