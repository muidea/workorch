# STEP04 完整原归档 Bootstrap 编译与生产验证记录

日期：2026-10-27（Asia/Shanghai）。HEAD 为 `834aa350cc4e77379819666540b9912a7c22b64c`，加累积未提交源码。本记录不是现行设计权威，也不是整个 STEP04 的退出报告。

## 本轮关闭的主体缺口

1. `activationports.CompileOriginalPreparedBootstrap` 从同一个已持久化原操作的完整私有来源组装 PreparedBootstrap，复用原 Definition、模板/策略、语言、修复预算、Soul、Skill context、Memory 原模式及 MCP 选择。原档案独立校验，缺失、错身份、错 revision 或重哈希替换均拒绝；不读取当前配置或环境，不查询 owner，不初始化 Runtime。
2. Workspace 显式 check/recover 的 typed reply 携带 detached 原 after-image 数据，仅供同次 Application continuation 纯编译。它不是调用者可声明的成功凭据，且不进入 JSON。普通 GET 不触发编译，也不回填旧记录。
3. HTTP/Web 只投影严格校验的版本、snapshotHash、manifestHash、bootstrapHash 或有限失败码，不返回完整 bootstrap、私有配置或原始诊断。编译结果只存在本次显式操作的页面内存；关闭重开/刷新仅查询既有记录，不补造编译结果。
4. Provider 在此编译器中只提供原配置精确 selection **声明**。原配置存在且选择合法时，cache miss/expired 或容量检查失败不等于缺少结构输入。此调整不修改 Provider 原单项判断、不证明授权、不刷新缓存、不授 ready/M3。
5. 生产验证模板在确认前显式声明 slice policy 和 managed validation 所需 owner fact 合同。没有修改原 Goal/DoD，没有向旧 archive 回填义务或减弱生产门禁。

本轮使用 go-multi-module-dev 保持 Workspace 原 owner-private 结算、Application 编排与 concrete typed EventHub 边界；无新增运行单元/topic，无跨 owner 直读写，无 SM-1～SM-4 行为变更，不进入 STEP05。

## 自动验证

以下为作者本轮累计运行记录，不是独立 CI：

| 检查 | 结果 |
| --- | --- |
| Application biz/server、Workspace 全子树、managementcontract、repository 全子树 | 17 个有测试包通过，2 个包无测试；Application biz 74.530s，Workspace biz 125.441s |
| activationports、activationcore 全包 | 通过 |
| 两个 biz 包定向 race（含完整 empty-selection HTTP/reopen） | 通过 |
| 生产 archive opt-in race | 显式指定下述 d/data，实际执行通过，21.519s；不是 Skip |
| 受影响 Workspace/Application biz tree/management 的 vet | 通过 |
| Web 组件/规则 | 13 文件、197 项组件及 11 项规则通过；既有 Ant Design warning 保留 |
| STEP04 verifier / build identity | 14 / 17 项通过 |
| STEP01 / STEP02 / STEP03 verifier | 5 / 6 / 7 项通过 |
| `make verify-boundary-contract`、`git diff --check` | 通过 |

完整 Go 命令：

```sh
GOCACHE=/tmp/workorch-go-cache go test ./internal/modules/application/workorchd/biz ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/workspace/... ./internal/pkg/workorchmanagementcontract ./internal/pkg/repository/... -count=1
```

生产档案测试：

```sh
WORKORCH_PREPARATION_ARCHIVE_ROOT=/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-full-bootstrap-20261027-d/data GOCACHE=/tmp/workorch-go-cache go test -race ./internal/modules/kernel/workspace/biz -run '^TestOriginalPreparedBootstrapArchivedProductionSource$' -count=1
```

普通 suite 另有完整 HTTP happy structural compile，opt-in 不替代默认回归。未运行全仓 Runtime 长耗时套件，不称全 CI 或发布验证通过。

## 生产 HTTP 与独立进程重启：d 批次通过

受限证据根：`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-full-bootstrap-20261027-d`。命令为 `GOCACHE=/tmp/workorch-go-cache python3 scripts/verify-workspace-preparation-step04.py --evidence-dir <该目录>`。

- `result.json` 为 `pass=true`、`ready=false`、`M3Issued=false`、`browserVerified=false`。
- 原3操作在两次真实 stop/start 后复现同一完整 bootstrap 内容；各自 snapshot/manifest 仍绑定自身原身份，不能互换。
- 原检查 10/11，缺 managed config 后新检查 9/11，人工恢复文件后新检查回到 10/11。缺配置不自动修复，旧历史不覆盖。
- 丢 HTTP body 后按原身份 GET 对账、check/recover 重放通过；重启/恢复不重复 SkillHub status，不修改业务文件。
- 3 个 PID：3839267、3839332、3839505，均已停止。
- authoring 准备阶段 8 次回环 fixture responses；STEP04 阶段 Provider HTTP 0（包含 models）、embedding 0、Run 0。不使用真实网关、模型或凭据，不启动 MCP。

索引：`result.json`、`build-identity.json`、`http-trace.json`、`original-owner-record.bin`、`final-owner-record.bin`、`process-1.log` 至 `process-3.log`、逐请求原始 bytes 和 fixture 原始材料。目录 Git ignored；原始输入/诊断不提交 Git。

## 真实 Chrome：页面断言通过，但 c 组合批次失败

受限证据根：`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-full-bootstrap-20261027-c`。与 d 使用相同构建输入、console 和 binary 内容身份。

用户手动打开 `http://127.0.0.1:59929/console/`；未修改本机跨站边界，未点击“创建新执行”。实际执行一次显式检查，原 requestID 为 `513bbcd3-7bc9-4107-bcec-20c26ba578ca`，expected revision 22，结算 revision 24。

实际 DOM 与截图确认：

- 显示“完整 PreparedBootstrap 结构编译已复现；不是 ready、M3 或执行授权”，bootstrapHash 为 `prepared-bootstrap-sha256:d399bdea2548763fc3225253aca82828f7154bdb4862c43768a785a79ce5a0aa`。
- 原检查 10/11、操作 blocked，显式回复 currentValidity 为 not_evaluated，Provider 为 `provider_selection_budget_observed_authorization_pending`。
- 关闭重开和只读刷新保持同一 requestID、revision24、原10/11记录，不再显示页面内存中的完整编译结果。查询实际呈现 stale，未升级为 ready。
- 验证器从持久记录核对只有1个原 attempt；浏览器结束后已导航 about:blank，释放页面连接。

证据：`browser-explicit-check.txt/.png`、`browser-reopen.txt`、`browser-refresh.txt`、`browser-readonly-refresh.png`、`original-preparation.json`、`browser-session.json`、`http-trace.json`。`browser-done` 仅是恢复脚本的同步 marker，不是浏览器成功证明。

**该组合批次不得记为通过。** `result.json` 保持 `pass=false`、`browserVerified=false`。全批次原始 Provider 记录为2次 `/v1/models` GET及8次 fixture responses，比纯 HTTP d 批次多1次 models GET。脚本在“STEP04 不刷新 catalog 或调用 Provider”的原断言处正确停止，c 没有执行后续进程重启。不能把 d 的独立重启归到 c，也不能删除这次请求、重置基线或放宽门禁以拼成通过。

浏览器整页还实际调用了3对原库存 SkillHub status，脚本独立记录 `browserPageInventoryStatusPairs=3`，不计作 preparation GET/recover 的重复执行。额外 models 请求的精确触发归属本轮未独立定位完成；不把它未经证明地归因于准备页或指定接口。它属于后续整页零 Provider I/O 验证障碍，本轮遵守“Provider 外”边界不修改其行为。隔离 fixture 不通向真实网关，没有收费推理或 embedding；PID3837489已停止。

## 精确构建身份

| 字段 | c/d 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` + 未提交源码 |
| buildInputSha256 | `b08262c4031c7c81cca5f8993ae69f9a5ac93a15013dc5fa3edf2fdedd366ef4` |
| consoleTreeSha256 | `d3f4c0301d5bcb816509ef396fc89fe41b8f721f5c2176ba2a37734a1febcb9f` |
| binarySha256 | `40d0ed8047b72c4206393d2d33e873dc65863460695fa5225030bb94c905a8a9` |

d 结束时由公开 Python `build_identity.verify()` 重新核对实际源码、console及binary；浏览器结束后另固定完整四字段 `expected-identity-final.json`，再次运行 `build_identity.py verify` CLI，退出0，与上表一致。不是仅依赖 HEAD，不称可信构建 attestation，也不宣称某个尚未产生的最终提交已实测。

先前失败原样保留：旧 execution-inputs fixture 缺 slice policy；full-bootstrap a 缺 owner fact；b 重启冷缓存触发过严的声明判定。修正后新建 d 完整重跑，未改写这些失败结果。c 的新增整页 Provider 请求失败同样保留。

## 剩余主体出口与 live 边界

完整原档案结构编译这一主体缺口已关闭；现已实际运行无收费、有界、阻塞型生产 HTTP/live 页面验证。**这不等于“除 Provider 外全部完成”，也不是完整 readiness/M3 正向 live。**

接下来仅沿主体出口推进：

1. 原 required 事实的完整当前有效性合取，以及成功 owner 事务接线；不能用历史单项 passed 或结构 hash 代替。
2. creation epoch 下唯一历史 M3 completion 的正向 final CAS及竞争/恢复验证。已有 no-Run 单项结算 fence不等于此成功事务已实现。
3. 有界空选择之外的非空 Skill/MCP、semantic Memory，按原模式消费真实 owner 事实；缺证仍明确阻塞，不能自动降级或收费补绿。这些细节不应替代前两项主体推进。
4. c 暴露的整页额外 models 请求需在 Provider 范围另行定位处理并复验，才能声称生产 Web 整页零 Provider I/O 联合验收通过。

Provider 授权合同仍阻塞；本轮不扩大该项、不创建 Run、不进入 STEP05。继承的 integration/orchestrator CI 阻塞未在本轮修复或重新验证。
