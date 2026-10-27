# STEP04 组合选择持久编译与恢复验证记录

日期：2026-10-23。基线为 `834aa350cc4e77379819666540b9912a7c22b64c` 加当前累计未提交改动。本记录不将 HEAD 等同已测源码，也不表示完整 STEP04、全 CI 或发布验收通过。

## 本轮收口

原准备操作将已核验的 Registry 两轮完整记录、六阶段 Soul 及 SkillHub 原 status/安装双读材料，交 Workspace 私有 reducer 纯编译。未调用会重新执行 status 的独立 `ObservePreparationSkillSelection`。组合材料嵌入原 Registry archive，与原 blocked 结算同一次 CAS，不增加外部效果、topic 或结算 revision。

- 保留封存 Goal/DoD 原文及顺序、原 request/startup/definition/source 引用、execution Soul allowlist、完整候选身份、typed 匹配分数和理由、选中完整 Content/provenance/原 ObservedStatus。全部源材料仍在原受限 archive，不用摘要代替。
- 评分与 typed 匹配迁入 owner-neutral 纯包 `internal/pkg/skillselection`；Application 直接引用共享类型，不重导出兼容别名。既有展示选择与共享 typed 选择的规则等价回归保留。
- Workspace 独立从全部原依赖重算并比较完整值，不凭 hash、parsed inventory 或调用者成功 bool 授信。首次追加时另反查真实确认前像的 Goal/DoD、StartupInput 与原 binding，不能把自洽的替代 Goal 及新 hash 当成原确认。
- 缺来源为 `sources_incomplete`，来源/候选冲突为 `conflict`；完整编译仅为 `compiled_checks_pending`。空目录须全部来源实测完整，重复 ID 在 allowlist 过滤之前拒绝。
- HTTP/TS/Web 分轴展示 `skillSelectionState`，不泄露私有正文，不授 ready、freshness、资源 PASS 或 M3。原 `modified/conflict` status 保留为历史事实，不改写成成功。
- GET、完成重放、原操作恢复和 owner 重开不重执行 status、不补读安装文件、不回填缺失选择、不覆盖已结算历史。最终 revision/epoch 漂移仍保留 `binding_changed`，不编译新选择。

按 go-multi-module-dev 核对，跨 owner 仍为既有 concrete typed EventHub 合同；共享包仅做纯计算，没有新运行单元或 owner facade。本轮不改变 Runtime SM-1～SM-4、执行 reservation、Start/Restart 权限，不进入 STEP05。

## 验证与原始证据索引

受限证据根：`/tmp/workorch-step04-selection-evidence-SbY04M`，临时目录权限 0700；原日志和二进制不提交 Git。该临时目录不是长期归档，后续独立复核需另行保全原文件。

| 检查 | 本轮结果及证据 |
| --- | --- |
| 受影响 Go 回归 | 21 个有测试包通过、2 个无测试包，`affected-go-verified.log`；包括共享策略、Workspace、Application、HTTP server、file/memory/scoped repository、management DTO |
| 定向 race | 六个实际运行测试的包通过，`race-verified.log`；skillhubpreparation 因过滤器无匹配用例，仅编译，不计 race 测试通过包 |
| 最后追加的原确认前像反例 | `selection-cuts-final.log`，Workspace owner 原结算/写失败/落盘回执丢失矩阵在 race 下通过；追加发生在上述全包回归之后，最终构建包含此断言 |
| 真实 loopback HTTP 与 file owners | 包含在 `affected-go-verified.log` 及 `race-verified.log`；修正后的独立 HTTP 为 `http-corrected.log`。安装文件移走后 GET/check/recover/owner 重开仍保持相同投影，status 总数为 2，Soul/Registry 实读均为 2 |
| Web 组件／规则 | 13 文件 145 项组件测试、11 项规则测试通过，`web-components.log`、`web-rules.log` |
| 生产 Web 构建 | `web-build.log` 通过；生成的内嵌资源已更新，保留既有 chunk 大小告警 |
| 边界／vet／格式／diff | `boundary-final-verified.log`、`vet-final.log`、`fmt-final-verified.log`、`diff-final-verified.log`；最后追加断言另由 race 编译执行，后续未改生产 Go 逻辑 |
| 本地构建与内容身份 | `source-snapshot-final.json` → `build-final.log` → `build-identity-final.json`／`build-produce-final.log`；公开 Python `verify()` 使用固定四字段核验，`build-verify-function.log` |

受影响回归命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/pkg/skillselection ./internal/pkg/skillhubpreparation ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistrycore ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/biz/appservice/skillselectcore ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo -count=1
```

定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/skillselection ./internal/pkg/skillhubpreparation ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/skillselectcore ./internal/pkg/workorchmanagementcontract -run 'TestExactSelection|TestPreparationSkillSelection|TestPreparationSkillRegistry|TestPreparationSkillHub|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestTypedSelection|TestSelection' -count=1
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/biz -run '^TestPreparationSkillHubReceiptSettlementImmutableAndCutRecovery$' -count=1
```

模型/策略断言覆盖完整空目录与缺失二轮的区别、top5/排序/源 ID allowlist、全目录重复 ID、原 status/version/CRLF/NUL 保真、配置/快照/请求漂移、deep clone。HTTP 校验篡改 score、正文、allowlist、来源 ref 后 rehash 仍拒绝，删除 Selection 并同步清理 summary/reason 仍不能改写完成历史。owner 切点使用明确 fixture 原事实，验证成功组合结算、写失败恢复不补造、落盘回执丢失后恢复保留；另验证自洽替代 Goal/重编译/rehash 在首次追加时仍被真实确认前像拒绝。该 reducer fixture 不代替实际资源来源观察；实际来源读取与文件移走后恢复由 HTTP 用例覆盖。

HTTP 使用本地 provider fixture 准备候选；准备检查不增加推理/embedding，不产生 Run。global 指令根显式指向临时目录，不读取用户实际安装指令。没有读改 live 配置、现有服务或外部 skill-hub 项目。owner 关闭重开不是独立 OS 进程重启，也不是生产 Chrome 验收。

### 失败与修正记录

- 初轮旧测试仍期望 Registry-only reason；完整 Registry 但缺 Soul/SkillHub 现在必须为 `sources_incomplete`。修正期望，未放宽门禁，见 `workorch-step04-selection-targeted.log`。
- HTTP 新增 clone 反例起初误改了本地 baseline；改为独立 caller copy 后通过，原失败见 `workorch-step04-selection-http-final.log`。
- 首轮边界拒绝 Application 重导出共享类型别名；已改为直接引用，未修改扫描规则，见 `boundary.log`。
- 后续边界要求登记原 ObservedStatus；已按 Workspace immutable_trace 登记，不改变状态权威，见 `boundary-corrected.log` 与最终通过日志。
- 补强绑定时曾错误要求受理后观察快照 hash 等于受理前 binding 快照 hash；两者含不同 Workspace revision。失败见 `http-final.log`、`affected-go-final.log`、`race.log`。移除错误的跨 revision 等值，保留同一次观察的完整源 hash 关联及真实 StartupInput/Goal/DoD 前像校验，最终受影响回归和 race 通过。
- 一次边界命令未指定 GOCACHE，触发只读默认 cache；改用 `/tmp/workorch-go-cache` 后通过。capture 曾误用 produce 专用参数，未生成有效快照；最终按 capture stdout 保存后完成构建链。
- Go build 有 module stat-cache 只读告警但退出 0。`build_identity.py verify` CLI 的既有重复 schema 处理问题本轮未修复、未重新宣称通过；本轮只使用其公开 Python `verify()`。未重跑全仓 CI，不称全 CI 通过。

## 最终本地构建内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加本轮最终未提交源码；不是单独验证该干净提交。
- buildInputSha256：`c86cdce227977072d3a6f9df707429998e66f2776205b0caeb64ab8bc27f2bfa`
- consoleTreeSha256：`40093428e68193c09a16f2f77926ed50261e28b8b1bac208d64911f57da09ac1`
- binarySha256：`88386b85dbb0ac2723fed663eed7771729ab1725e415e9b32f19902c5e357dbf`

内容身份只支持本地输入／产物字节一致性，不是可信构建 attestation、部署证明或验收授权。未运行该二进制的生产浏览器或独立进程 stop/start 验收；不沿用上一轮二进制结果。

## 剩余收口顺序

1. 选中 Skill 辅助资源、真实来源状态/freshness、daemon/local 一致性及既有 context 预算；历史编译不可代替这些检查。
2. MCP/Memory/provider 等 required owner 的真实观察及原操作持久消费。
3. 全 required 合取、no-Run/epoch/final CAS 与首次历史 M3 的生产正结算；当前仍无 ready/M3 正回执。
4. 补齐对应竞争/中断矩阵、生产 Chrome 和独立进程重启证据；未知外部效果不能靠重发或换 request 清除。
5. 独立跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计套件超时、build-identity CLI 等全量验证问题。

完整 Start/Restart 回执消费属于 STEP05。本轮完成的是历史组合选择持久链，不满足完整 STEP04 live 退出条件；没有自动提交改动。
