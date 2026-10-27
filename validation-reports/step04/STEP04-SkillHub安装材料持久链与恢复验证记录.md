# STEP04 SkillHub 安装材料持久链与恢复验证记录

日期：2026-10-23（Asia/Shanghai）。源码基线为 `834aa350cc4e77379819666540b9912a7c22b64c`，加累计及本轮未提交修改。
本记录是安装材料链的增量验证，不是 HEAD 单独通过、完整 STEP04、M3、全 CI 或 live 准入通过声明。

## 本轮收口

- 原准备检查的 status 效果 reservation/一次延续保持：只有无歧义首次延续执行 global→project，各最多一次，仍共享原五秒观察预算；GET、普通重放、显式恢复和 owner 重开不执行 status。
- 成功 status 后仅消费同一次原始回执，新增无 runner 的安装观察入口，不再调用 status。调用方 parsed inventory 先清空，再从完整原 JSON 独立解析；不相信 InventoryComplete、Reobserved 或 PASS bool。
- 只在独立确定的 Codex 全局根／原交付目录 `.agents/skills` 下真实双读对应 SKILL.md，沿用 os.Root、安全打开、symlink/非普通文件拒绝、inode/size/mode/mtime 与根目录复核。没有 source/repo fallback。
- 两轮实际读值分别保留原 bytes、路径、大小、计数、完整性、hash、失败和时间。每文件 64 KiB、每 scope 每轮总计 8 MiB；失败部分不冒充完整。metadata 严格解析保留原文，取消或超预算明确失败，不授选择 credit。
- 纯解析集中于 `internal/pkg/skillhubpreparation`，Application observer 与 Workspace owner 共用相同的有界 JSON/YAML 合同；没有新运行单元、EventHub topic、独立 archive/ref 或额外 CAS revision。
- 安装材料嵌入原 `PreparationSkillHubStatusFacts.Installed`，随原 effects archive/blocked 结算同 CAS 保存。Workspace 独立反查原 raw membership/path、两轮值、metadata、时序、界限与 summary，不接收调用方“已验证”标志。
- 原 status 失败、安装观察失败、安装材料完整分开保留；安装失败不篡改真实 status 退出事实。私有材料不进入普通 Workspace JSON/公开 HTTP；file 0600、deep clone 和已结算历史不可替换保持。
- 公共 `skillHubInstalledState` 只允许 v2 blocked attempt 且绑定原 `skillHubEffectRef`：`incomplete` 或 `observed_selection_pending`。缺材料不造此字段。Web 独立展示材料不完整／已封存但组合选择待裁决，均不表示 ready。
- 状态归属清单将该 summary 登记为 Workspace derived projection；Application 仍只做观察和编排，Workspace 私有 reducer/CAS 仍是账本权威。未改 Runtime SM-1–4、稳定 LLM cache 前缀、执行 reservation 或 STEP05 授权链。

组合选择现有独立 helper 会执行自己的 status，不能直接塞进原操作或恢复链。本轮只持久封存安装材料与 metadata 核验事实，不将该 helper 成功混同为持久选择结算。

## 验证及原始证据

本机隔离证据根：`/tmp/workorch-step04-installed-evidence-ulMHWy`（目录 0700）。日志、快照、身份清单和二进制不提交 Git。

| 检查 | 结果与索引 |
| --- | --- |
| 受影响 Go 包完整回归 | 19 个有测试包通过、2 个包无测试，`workorch-step04-installed-affected.log`；随后时间/预算防护及最终切点用例又执行定向复验，`workorch-step04-installed-final-targeted.log` |
| 最终定向 race | 7 个包通过，`race-final.log`；覆盖纯解析、原 metadata 消费者、安装观察、Workspace 持久结算与 HTTP 恢复，不宣称完整套件 race |
| 安装材料 owner 切点矩阵 | 通过，`workorch-step04-installed-owner-matrix.log`；成功、写失败、落盘后回执丢失、owner 重开、历史删除及 rehash 替换拒绝 |
| 真实 loopback HTTP + file owners | 通过，`workorch-step04-installed-http-workflow.log`；正式 ports/HTTP、两个真实临时安装文件、双读、移走文件后 GET/原请求 check/recover/owner 重开，status 总数仍为 2、材料原值仍保留 |
| Web 组件 | 13 文件、144 项通过，`workorch-step04-installed-web.log`；准备组件新增完整/部分安装材料投影及非法状态拒绝 |
| Web 规则 | 11 项通过，`workorch-step04-installed-web-rules.log` |
| 边界、vet、格式、diff | 通过，`workorch-step04-installed-boundary-final.log`、`workorch-step04-installed-vet.log`、`fmt-final.log`、`diff-check.log` |
| 生产 Web / Go 构建 | 通过，`workorch-step04-installed-web-build.log`、`build.log`、`source-snapshot.json`、`build-identity.json` |
| 本地身份核验 | capture→build→produce 通过，`build-produce.log`；公开 Python `verify()` 通过，`build-verify-function.log`。CLI verify 失败，见下述限制 |

完整受影响回归命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/pkg/skillhubpreparation ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistrycore ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/workorchrepo -count=1
```

最终定向 race 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/pkg/skillhubpreparation ./internal/modules/application/workorchd/biz/appservice/contextregistrycore ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/pkg/workorchmanagementcontract -run 'TestInstalled|TestStatusRaw|TestPreparationSkillHub|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestPreparationSkillMetadata' -count=1
```

新增/扩展断言包括：完整 raw 重解析和忽略伪造 parsed 字段、歧义 JSON/大小写重复/非法 Unicode/错误 scope/count/重复成员拒绝；无 runner 入口零 status 重试；原双读 bytes、缺二轮/漂移/错误 hash/path/metadata 拒绝；完整空 inventory 必须有两轮非 nil 实际读集合；私有 partial/error bytes、clone 隔离；public 非法 ready/accepted/无引用/v1 组合拒绝；写失败恢复不制造安装材料，回执丢失恢复保留原材料；已结算材料删除或一致 rehash 替换仍被拒绝。

HTTP 断言使用现有候选准备本地 provider fixture，不使用真实模型；准备阶段不增加推理/embedding，不新增 Run。global 安装目录显式指向临时目录，不读取用户安装指令。文件移走后原操作仍返回同一投影且原双读值仍可从私有持久状态读取，支持“恢复不重读”结论。
这里的 owner/HTTP 关闭重开不是独立 OS 进程 stop/start；没有生产 Chrome 验收，也没有真实 SkillHub 服务验收。未读改 live 配置或外部 skill-hub 项目。

### 失败记录与限制

- 初轮安装观察测试替身没有提供真实命令启动/已知退出字段，被新 capture 边界拒绝；修正替身而非放宽生产门禁。保留 `workorch-step04-installed-first.log`、`workorch-step04-installed-second.log`。
- 新 owner installed 场景初轮仍以旧 stdout fixture 为期望，导致 `original capture lost`；改为比较该场景真实原回执后通过，失败保留 `workorch-step04-installed-owner.log`。
- 初轮边界检查缺少新 summary 的状态归属登记，补入 derived projection 后通过；保留 `workorch-step04-installed-boundary.log`。
- CLI verify 两次均退出 2，保留 `build-verify.log`、`build-verify-final.log`。第一次误传含 schema 的 manifest；第二次使用仅四个身份字段的固定输入仍触发既有 `_read_expected_identity()` 与 `verify()` 重复处理身份问题。未改该脚本；公开 Python `verify()` 用固定四字段输入成功。不写成 CLI verify 通过。
- Go build 有模块 stat-cache 只读告警但退出 0；Web 保留现有 Ant Design/chunk 告警；边界保留 pycache binary-match 提示。未重跑全仓 CI，不写成全 CI 通过。

## 本轮构建内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c` 加当前未提交源码
- buildInputSha256：`fb47598366b1f731ba6469de96758950ee77db3c44fc81d1fa73ddea2bcf70e8`
- consoleTreeSha256：`fc85c726537f13b32daef13570353daf878cbc9d0d5619208c9749ecbdcd42d8`
- binarySha256：`deda3d335c8c56cf9b133f0f49eec467e0e80a021e320a4e5f07b20d33976399`

构建前后输入一致；身份核验只证明当前本地源码/控制台/二进制字节一致性，不是可信构建 attestation。
该二进制没有启动，不将 HTTP 测试二进制的结果归给它，也不沿用上一轮构建的通过记录。

## 剩余收口顺序

1. 用原操作中已持久材料编译完整 Registry＋SkillHub 组合目录、封存来源与精确选择事实；不能调用会重复 status 的独立 helper，也不能把历史安装材料当当前 freshness 或 readiness。
2. 补齐辅助文件、来源/状态/freshness、一致性与 context 预算；MCP/Memory/provider 等 required owner 观察及原操作持久消费。
3. 全 required 合取、no-Run/epoch/final CAS 与首次历史 M3 的 production 正结算；当前没有 ready/M3 正回执。
4. 完成相应竞争/中断矩阵、生产浏览器和独立进程重启验收；未决外部效果不能靠重发或换 request 清除。
5. 独立跟踪已披露的 public fullflow Profile 断言、Orchestrator 累计套件超时、build-identity CLI 等全量验证问题。

完整 Start/Restart 回执消费属于 STEP05，不在本轮范围；当前仍不满足完整 STEP04 的 live 退出条件。
