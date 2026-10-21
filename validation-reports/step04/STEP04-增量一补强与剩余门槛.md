# STEP04 增量一补强与剩余门槛

日期：2026-10-21（Asia/Shanghai）。HEAD：`d93463f37983575723393578c15faff51cf1d844`，加未提交改动。

本记录接续 `STEP04-增量一收口与验证记录.md`，只记录本轮新增事实，不替代独立评审。
结论：**修复一项 exact snapshot 目录保真边界，补齐定向反例；作者累计验证通过。
尚无增量一独立复核结论，未进入增量二，不能声明 STEP04/M3/live 已完成。**

## 1. 本轮修复和补强

- exact snapshot 的 `InternalDirectory` 原先只要求非空，会接受无效 UTF-8 或 NUL。
  Go JSON 对无效 UTF-8 使用替换字符，不能保持该路径的原 bytes；NUL 也不能用于真实文件路径。
  现在明确拒绝，不清洗、替换或截断原值。合法中文目录仍可完整往返。
- 新增 public checksum 重算测试：排除“仅因旧 hash 不匹配而拒绝”的假覆盖。
  错来源、正式状态/谱系、DoD 顺序、candidate/template/seal 等仍须通过完整反证。
- 新增两次 owner 读取间 revision 漂移的故障注入：最终返回 conflict，无 exact 结果、无写入。
- 新增 2 MiB snapshot、256 KiB manifest 各自的超限反例；超限不返回部分成功、不改写输入。
- 补齐 candidate ID/hash、原 permit/specification、confirmation phase 漂移的只读投影断言：
  只标记 stale，保留原 blocked 状态及原 request 身份。

首次定向测试实际失败于 `invalid_utf8` 与 `nul`，随后生产校验修复，累计回归覆盖最终代码。
该首次结果保留于本次工具执行输出，未另建原始失败日志；不称所有首次运行均通过。

按 go-multi-module-dev v2.6 保持 Workspace owner、typed EventHub 和纯编译边界；未新增运行单元，
未修改 SM-1～SM-4 或 fresh intent 入口，不以准备读取授予执行权。
原用户 STEP03/STEP04 计划文件未改；通用 build_identity CLI 缺陷仍未纳入修复。

## 2. 实际验证

受限本地证据根：`/tmp/workorch-step04-increment1-followup-9Oqd40`（mktemp 创建的 0700 目录）。
原始日志和二进制不提交 Git。

累计回归 exit 0，17 个有测试包通过、2 个包无测试：

```bash
GOCACHE=/tmp/workorch-go-cache go test \
  ./internal/modules/kernel/workspace/... \
  ./internal/modules/application/workorchd/biz/appservice/activationcore \
  ./internal/modules/application/workorchd/biz \
  ./internal/modules/application/workorchd/service/server \
  ./internal/pkg/repository/... ./internal/pkg/events \
  ./internal/pkg/workorchmanagementcontract -count=1 -timeout=10m
```

其中包含已有真实回环 HTTP/file owner reopen 测试；本轮未运行独立服务进程、浏览器或收费模型。
累计套件还包含原执行回归，不能据此声称整个测试套件都没有测试 Run。

定向 race 两包通过，exit 0：

```bash
GOCACHE=/tmp/workorch-go-cache go test -race \
  ./internal/modules/kernel/workspace/biz \
  ./internal/modules/kernel/workspace/pkg/startuppreparation \
  -run '^(TestExactConfirmedStartup|TestPreparationState|TestPreparationPureCompilationBoundary)' \
  -count=1 -timeout=3m
```

上述两包 `go vet`、`make fmt-check`、`git diff --check` 和 `make verify-boundary-contract` 均 exit 0。
边界脚本仍有既有 __pycache__ binary-match 提示，末尾为 verification passed。
未运行全 CI、全仓库 `go test ./...` 或 managed deployed。

| 原始文件 | SHA-256 |
| --- | --- |
| affected-go-tests.log | `132e42e7f84537cf036fdc74ca443f14e979611039711667ad4eaf2c93953651` |
| targeted-race.log | `187ad2d60f15f23fb07b69630bf424b99a38d3593aee8a56781d8c7ed37ba952` |
| boundary.log | `837a8475a3e2c9760fbd33b5e047ac5089a0e49285c87bd3c283d79116054a81` |
| build-final.log | `14aa2272af126f822cdebda4925302af6b1b70fd53bf7d568efbdbaa715fda2a` |

vet.log、fmt.log 为空。build-final.log 保留只读 module stat cache 的非致命写入警告，构建 exit 0。

## 3. 当前源码与新二进制关联

在最终代码和 owner readme 固定后重新 capture，再执行 Go build，produce 复读并核对构建前后输入。
没有启动二进制、重建控制台或修改现有服务。最终关联文件为 `source-snapshot-final.json`、
`build-identity-final.json`、`build-identity-produce-final.log`；不沿用上轮二进制身份代表新树。

| 字段 | 当前值 |
| --- | --- |
| sourceCommit | `d93463f37983575723393578c15faff51cf1d844` 加未提交源码 |
| buildInputSha256 | `fe2ccc9412a1d2447f354b62f0d560037ddb4a96621fb63b5190f2f1538212e8` |
| consoleTreeSha256 | `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b` |
| binarySha256 | `a82ac7ebeb92473e3515eee63e73c341cfc1e1a4853022aef145ee6908a981d2` |

仍按公开 Python `build_identity.verify()` 四字段输入合同独立提供上述固定身份，完整复读校验；
不使用有重复规范化缺陷的 CLI verify，不把本地字节关联称为可信 attestation 或部署验收。
函数校验 exit 0；`build-identity-function-verify-final.log` SHA-256 为
`2c2c009badcd19941bb5f6bb4e0a87de4b9835795e2f0504efc0d2d1e115948f`。
本轮较早的非 final 构建身份只属于中间树，日志保留，不作为最终树证据。

## 4. 剩余项及下一步门槛

| 顺序 | 未完成项 |
| --- | --- |
| 前置 | 按计划 §9/§11 取得增量一独立复核结论；作者自测和本记录不代替独立复核 |
| 增量二 F03 | 真实 owner 资源观察、目录/配置完整性、cache-only freshness、Memory/MCP 原模式与具体阻塞 |
| 增量二 F05/F08 | 显式检查接纳、精确幂等、私有 reducer/CAS、失效和原操作恢复；目前仍无生产准备写入口 |
| 增量二 F07/§7 | 精确 no-Run epoch/reservation、retirement-aware Run 观察、unknown/legacy 门禁和历史 M3；不得凭空库存签发 |
| 增量三 | Web 分轴视图、显式检查/恢复、迟到响应 fence；生产构建真实 HTTP/浏览器及独立进程 stop/start |
| 最终出口 | A01–A20 适用证据闭合，独立审查无 P0/P1/P2；不冒称 STEP05 完整 Start/Restart receipt consumption 已接入 |

没有自动物化资源、刷新外部 catalog、推理/embedding、MCP 启动或准备路径 Run/runtime 派发。
未满足 required 的场景继续阻塞。未提交修改。
