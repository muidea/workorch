# STEP04 SkillHub 效果持久阶段与恢复验证记录

日期：2026-10-23（Asia/Shanghai）。源码：`834aa350cc4e77379819666540b9912a7c22b64c` 加累计及本轮未提交修改。
本记录是增量验证，不是 HEAD 单独通过、完整 STEP04、M3、全 CI 或 live 准入通过声明。

## 本轮交付

- 原准备检查首次 accepted CAS 同时封存 SkillHub global/project status 目标、原请求/payload/revision、完整 binding、原交付目录与随机引用，不另增接纳 revision。
- 只有无歧义首次回执返回一次 Application 延续；生产端口使用当前 SkillHub status adapter，允许其既有配置服务访问和状态保存，不增加 init/install/sync/use/apply/repair。
- 派发前再次真实读取同 accepted revision 的 exact seal，核对原确认、candidate、configuration、startup 和目录；global 后 project，各最多两秒，共享原五秒观察预算，无重试。global 失败不派发 project。
- 只保存真实原始回执，不消费 inventory/PASS helper bool。原 stdout/stderr bytes、读取总数、完整性、退出事实、时间和 helper 错误 bytes 随原 blocked CAS 私有归档。
- 每流最多保存 1 MiB 捕获前缀并明确 incomplete；错误超 64 KiB 时不保存前缀，保留原总字节数/full SHA256 与明确未保存原因。不是静默截断或将前缀称完整。
- 随机 `skillHubEffectRef` 与 `reserved/recorded/unresolved` 单独投影；原文不进入普通 Workspace JSON/公开 HTTP。file 0600、deep clone、append-only、重开与重新计算 hash 后防改写保持。
- GET、同请求普通重放、显式恢复、owner/HTTP 重开不执行 status。中断缺回执按 unresolved 收口；已知非零退出、已捕获原始输出仍保持已知失败，不能改成无失败。
- 迟到结果不覆盖已结算历史；后端和 Web 都禁止新 request 绕过 reserved/unresolved。没有凭超时、空库存、换请求自动清除效果 fence 的入口。
- 相对/非规范交付目录保持原值，派发前记录 `directory_invalid_not_started`，不猜选其他目录。未注入 adapter 的手工 Biz 明确 not_started，不回落调用本机 CLI。

`recorded` 只指历史操作材料已经记账，可包含真实未启动；不是资源 ready。安装指令、metadata 与组合选择持久裁决未因此完成。
沿用 go-multi-module-dev 的 owner 分工：Application 管外部 adapter/编排，Workspace 私有 reducer/CAS 管操作账本；跨 owner 仍用 concrete typed EventHub。没有新运行单元/topic，不涉及 Runtime SM-1–4 或 STEP05 执行授权。

## 验证及证据索引

本机隔离证据目录：`/tmp/workorch-step04-status-effects-Pk3RyH`。原始材料/二进制不提交 Git。

| 检查 | 结果与原始索引 |
| --- | --- |
| Workspace、Application、server、repository、management contract 受影响包完整 Go 回归 | 通过，`go-affected.log`；另有 workspace-scoped repository 全包 `workspace-scoped-repository.log` |
| 最终新增断言与管理 DTO 定向复验 | 通过，`final-targeted.log` |
| 定向 race | 通过，`race.log`；其中 repository/file、memory 没有命中所选测试，不将它们称为已执行 race 用例 |
| Web 组件 | 13 文件、143 项通过，`web-components.log`；准备面 14 项 |
| Web 规则 | 11 项通过，`web-rules.log` |
| 边界、vet、格式 | 通过，`boundary-final.log`、`vet.log`、`fmt-final.log`；另执行 `git diff --check` |
| 生产 Web 和 Go 构建 | 通过，`web-build.log`、`build.log`、`source-snapshot.json`、`build-identity.json` |

完整 Go 命令：

```bash
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/kernel/workspace/... ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/biz/appservice/contextregistryports ./internal/modules/application/workorchd/service/server ./internal/pkg/repository/... ./internal/pkg/workorchmanagementcontract -count=1
GOCACHE=/tmp/workorch-go-cache go test ./internal/pkg/workorchrepo -count=1
```

定向 race：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/biz ./internal/modules/application/workorchd/biz ./internal/modules/kernel/workspace/pkg/models ./internal/pkg/repository/file ./internal/pkg/repository/memory ./internal/pkg/workorchmanagementcontract -run 'TestPreparationSkillHub|TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen|TestPreparationWorkflowTypedApplicationBlockedReadbackAndReopen|TestPreparationObservationContinuationOnlyInitialWinnerAndLateSettlementNoop' -count=1
```

新增/扩展断言覆盖：初次 accepted CAS 先于 status、两个 scope 的准确目录和顺序、GET/replay/recover/重开零重发、缺回执恢复、结算写失败/回执丢失、已知非零退出、原始 bytes、超界/不完整/错误原文保存边界、0600、深拷贝、历史删除/rehash 改写拒绝、未决效果换 request 拒绝、公开 DTO 非法组合拒绝、Web 分轴与禁止新检查。

真实 loopback HTTP 使用正式 Application ports 和 file owners；仅 SkillHub adapter 为本地非敏感替身。替身读取实际持久 plan 后才能返回，两 scope 总调用数严格为 2；GET/原请求 check/recover 及关闭重开不增加。既有候选准备使用本地 provider fixture，准备阶段不新增推理/embedding，Run inventory 仍为 0。
HTTP 结论来自测试断言与日志，不冒充独立保存的每次 HTTP body、真实 SkillHub 服务实测、生产 Chrome 或独立 OS 进程 stop/start。
没有读取 live 配置、调用真实模型/embedding 或改动外部 skill-hub 项目。

初轮定向测试暴露相对交付目录使新 plan 过早拒绝，已按“原值保存、派发前明确 not_started”修正并复验；早期失败保留于 `initial-targeted-failures.log`。该早期非提权运行也包含回环端口被沙箱拒绝，随后授权隔离运行通过（`preparation-regression.log`、正式完整日志）。首次边界检查因默认 Go 缓存只读失败，使用既定 `/tmp/workorch-go-cache` 后通过。构建保留 Go 模块 stat-cache 只读告警但退出 0；Web 保留既有 Ant Design/chunk 警告，边界保留 pycache binary-match 提示，不抹掉这些告警或称全 CI 绿。

## 构建内容身份

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c`，加当前未提交源码
- buildInputSha256：`2081a4c059c6364d4d68ab061d644a2f4f0bafaf11473c0d051c081f35d54ed3`
- consoleTreeSha256：`4a077c5aa9fb2656f93902ec3e1f22a411d2fab0a2813277807f902b861cd6e9`
- binarySha256：`d239d0a9e955b5bdd49ba854eb1cfe49d6268389e538e461858343a5689c4571`

执行 capture→隔离路径 build→produce，构建前后输入一致。没有启动这个二进制，也未调用既有 CLI verify；不是可信构建 attestation，不移用上轮旧产物结果。

## 剩余项

1. SkillHub 安装指令、metadata、完整组合目录和精确选择事实接入原持久操作；当前只完成 status 效果阶段与原回执，不授选择 credit。
2. MCP/Memory/provider 的全部 required 真实观察、来源/freshness 绑定及持久消费。
3. 全 required 合取、no-Run/epoch/final CAS 与唯一首次历史 M3，仍没有 production ready/M3 正结算。
4. 更完整的诊断细分、竞争/中断矩阵、生产浏览器和独立进程重启验收；未决外部效果需要可信对账依据，不能自动重发或清 fence。
5. 已披露 public fullflow Profile 断言、Orchestrator 累计套件超时等全 CI 阻塞，本轮未修复或重跑全仓 CI。

完整 Start/Restart 回执消费仍属于 STEP05，不在本轮范围。
