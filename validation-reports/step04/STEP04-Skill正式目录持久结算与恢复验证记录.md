# STEP04 Skill 正式目录持久结算与恢复验证记录

日期：2026-10-23（Asia/Shanghai）。
源码基线：`834aa350cc4e77379819666540b9912a7c22b64c` 加全部累计及本轮未提交修改。
本记录只说明本轮增量，不是 HEAD 单独通过、全部 STEP04、M3 或 live 准入通过声明。

## 本轮结果与责任边界

接通了原准备请求的 Skill Registry 完整历史观察、持久结算、随机引用与原操作恢复：

1. 原 helper 保留两轮完整 typed owner 返回值及前后精确封存 hash，不再只留下首轮 catalog 与成功 bool。
   每轮保留版本、Workspace scope、owner epoch、观察时间、完整 records 和 catalog hash；
   继续使用每轮 1024 项／8 MiB 上限。两轮材料完整保存，不静默截断；不是只保存 hash。
2. 将 owner 的纯有界快照和验证放在 Skill `pkg/models/preparation_catalog.go`；
   EventHub 仍是原 owner 的 concrete command/result，显式生成 detached snapshot，无类型 alias facade。
   校验仍覆盖完整内容、provenance、scope、排序唯一性、时间和界限。深拷贝保留 nil／empty 列表原形。
3. 仅首次明确受理的 Application 延续观察 registry，把完整 `PreparationSkillCatalogFacts`
   传给 Workspace；不传 `Reobserved`／`SealBindingCurrent` 成功凭据。
   Workspace 独立核验原 accepted request、payload、revision、时间以及当前 exact seal，
   通过私有 reducer 随原 blocked 结算同 CAS 保存 `WorkspacePreparationSkillRegistryEvidence`。
4. 私有历史从普通 Workspace JSON／公共 HTTP 投影中排除。file wire 显式保全，文件权限 0600，
   memory/file/scoped file 保持 deep clone、原引用绑定、append-only 与重开一致性。
   同请求完成重放、GET、显式恢复和 owner 重开不再次读取目录、不改写原档。
5. 公开 management DTO／TS 校验器／Web 只显示独立随机
   `skillRegistryEvidenceRef = skill-registry-observation:<32位小写hex>`，
   不公开原指令、私有内容摘要或 caller 选择声明；格式错误、错误状态、缺引用及伪 PASS 拒绝。

完整观察，包括真实空库，仍是
`blocked/skill_registry_observed_selection_pending`；
缺 owner、读取错误、完整性不足或漂移则为
`skill_registry_observation_unavailable_not_saved`，明确本次完整材料未保存。
后者没有补齐失败原文的受限诊断归档/读取，不将它说成全部失败诊断已收口。
结算 revision/epoch 漂移只保留已经核验的历史，并标记 `binding_changed`，
不把历史材料提升为 current。

本轮使用 go-multi-module-dev v2.6.0 约束职责与 EventHub 接线：
Application 编排查询，Skill owner 返回真实完整目录，Workspace 独立结算自身准备账本。
不新增运行单元或 topic，不跨 owner 直访 repository/service；测试 fixture 的直接仓库构造不进入生产路径。
不修改 Runtime SM-1／SM-2／SM-3／SM-4，不增加执行授权或 Run。

本持久路径没有调用 SkillHub status。其真实状态保存／服务端访问效果仍遵循用户已确认的
现行行为，不改 SkillHub、不自动 sync/use/apply/install/repair，也不把该外部效果偷偷接入
可重放 helper。完整 Registry＋SkillHub 组合选择仍未完成，不能将本轮空库正例当成 Skill 通过。

## 实际验证

受限证据根：`/tmp/workorch-step04-skill-registry-evidence-IOlebE`（临时目录，不保证长期保留）。
原日志、二进制和构建快照不提交 Git。当前工作树未提交，也没有 push。

| 验证 | 证据与实际范围 |
| --- | --- |
| 完整受影响 Go 回归 | `affected-go-release.log`：15 个有测试包通过，另 2 个包无测试文件；不是全仓 go test／全 CI |
| 定向 race | `race-release.log`：Workspace biz/models、repository、Application biz 的 registry／repository workflow／HTTP workflow 用例通过 |
| 真实回环 HTTP | `http-final.log`：正式 HTTP ports、真实 Workspace/File/Soul/Skill/Run owners、file-backed 数据；同一测试进程内多次 owner/HTTP 服务关闭重开，不是独立 OS 进程重启 |
| Web | `web-rules.log`：11 项规则通过；`web-components.log`：13 文件 142 项组件测试通过；未开真实 Chrome |
| 静态与边界 | `vet-release.log`、`fmt-release.log`、`diff-check.log`、`boundary-release.log` 均通过 |
| 构建与内容身份 | `web-build.log`、`build.log`、`source-snapshot.json`、`build-identity.json`、`identity-produce.log`：capture → Go build → produce 完成，未启动该二进制 |

Go 回归命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test +  ./internal/modules/blocks/skill/... +  ./internal/modules/kernel/workspace/{biz,pkg/models,pkg/events,pkg/startuppreparation} +  ./internal/modules/application/workorchd/{biz,service/server} +  ./internal/pkg/repository/... ./internal/pkg/workorchrepo +  ./internal/pkg/workorchmanagementcontract -count=1 -timeout=5m
```

定向 race：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race +  ./internal/modules/kernel/workspace/biz +  ./internal/modules/kernel/workspace/pkg/models +  ./internal/pkg/repository ./internal/modules/application/workorchd/biz +  -run 'TestPreparationSkillRegistry|TestPreparationWorkflowRepository|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen' +  -count=1 -timeout=3m
```

新增 owner 场景覆盖完整目录、空目录、缺 owner、第二次读取漂移／失败、结算写失败／回执丢失、
结算期间 revision 漂移。反例覆盖完整原值缺失、重算 hash、错 scope／epoch／时间／原请求、
错引用、改写完成历史。三个 repository backend 覆盖材料与引用删除、重新计算全部 hash 后改写、
深拷贝和 file 重开。原读取 helper 的错误 DTO／排序／重复／超界／取消测试仍执行。

HTTP 用例准备候选阶段使用本地非敏感文档 fixture，共 8 次 fixture 推理；
准备阶段计数不再增加，cache query 1 次，Soul Get 2 次，Skill List 2 次，Run inventory 为 0。
首次准备后移开隔离测试的 Skill 文件（保留为同目录 `.preserved`），移除测试 Soul/config；
GET、同请求 check/recover、owner/HTTP 关闭重开后仍保留原完整材料及引用，Skill List 不再增加。
这些结论来自自动化真实 HTTP／owner 断言，不冒充另存的逐项 HTTP body 或外部服务实测档。
未访问用户 live 配置、现有服务、真实模型或外部 SkillHub。

初次 HTTP 编译因测试误用不存在的 Skill repository Delete 方法失败，修正为仅移开隔离 fixture 后通过；
`http.log` 保留该失败。初次边界检查因默认 Go 缓存只读失败（`boundary.log`），
随后门禁把纯值类型名中的 Registry 误识别为资源句柄（`boundary-final.log`）；
明确改名为纯目录事实 `PreparationSkillCatalogFacts` 后原门禁通过，没有修改或放宽门禁。
build 的模块 stat-cache 有只读告警但命令退出 0，produce 成功；边界日志保留既有 pycache
binary-match 提示，Web 保留 Ant Design 与 chunk 大小告警。不将这些告警抹掉或称全 CI 通过。

## 构建内容身份

以下身份覆盖当时完整 dirty 源码，不是仅靠 Git HEAD 证明，也不是可信构建 attestation：

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c`
- buildInputSha256：`3be7b2115bea9c96df17301d4a7e3717d4956baa45d0c19eea9a1f22ab82cf48`
- consoleTreeSha256：`4d7cee7e0fe2f6f44ae097a9d1fb1804114283e14b0b4d602a8aa7be8d532c89`
- binarySha256：`da37d236858a5610f0a9c90a7e9ddd5299d92bfb3aedb53061bd93710b668a75`

本轮执行 capture/produce，未调用 build_identity 的 CLI verify；
不移用旧产物结果，也不声称修复其既有 CLI verify 问题。

## 仍待收口

1. SkillHub 外部效果前持久阶段、未知效果不重发、原操作恢复，以及完整组合目录／精确选择事实的结算。
2. MCP／Memory／provider 的实际 required 观察、来源与 freshness 精确绑定及持久消费。
3. 全部 required 合取、no-Run／reservation epoch／final CAS 与唯一首次历史 M3；
   当前 production 仍只有 blocked 结算，没有 ready/M3 正状态。
4. 失败原文受限诊断及细分展示、完整竞争／中断矩阵，生产浏览器及独立进程 stop/start 验收。
5. 已披露的 public fullflow 默认 Profile 断言、Orchestrator 累计套件超时等全 CI 阻塞，本轮未修改／重跑。

完整 Start/Restart 回执消费仍属于 STEP05，不进入本轮。
