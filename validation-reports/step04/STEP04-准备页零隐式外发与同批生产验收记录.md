# STEP04 准备页零隐式外发与同批生产验收记录

日期：2026-10-27（Asia/Shanghai）。HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，包含累积未提交修改；本轮未提交 Git。

## 结论与边界

关闭上一轮实际浏览器中发现的准备导航额外 `/v1/models` 请求问题，并完成同一批生产构建的实际浏览器、真实 HTTP 和两次独立 OS 进程重启验证。不是通过修改 Provider 合同、扣除额外请求或放宽鉴权获得通过。

本轮有界样例的非 Provider 原时点检查为 10/10，整体为 10/11，唯一未通过的清单项是 `provider_cache`。完整原归档 PreparedBootstrap 纯编译、原请求重放／恢复、丢回复对账和人工资源恢复后的新检查均通过。`operationState=blocked`，`currentValidity=not_evaluated|stale`，`ready=false`，`M3Issued=false`。这证明非 Provider 有界阻塞主链已可进行且已执行 live 验证，**不证明除 Provider 外所有通用功能均已收口，也不证明完整 STEP04 或正向 M3 通过**。

本轮未读取真实 Provider 配置或凭据，未调用外部模型／embedding，未启动 MCP，未创建 Run，未进入 STEP05。未改变 Runtime SM-1～SM-4、正式 owner 裁决或执行授权链。沿用 go-multi-module-dev 的 owner／typed EventHub 边界，没有新增运行单元或跨 owner repository/service。

## 代码收口

1. `WorkspaceDetailPage.tsx` 的普通详情开页、SSE／重连刷新不再调用 `readConfiguration()`。该接口可能在 catalog 缓存到期时访问远端；现在仅显式打开“创建新执行”编辑器时读取，打开不提交执行意图。等待或失败时禁止提交，不沿用旧配置；关闭、重开和切换 Workspace 使用序列／scope fence 丢弃迟到响应。Resume 不额外读取模型目录。
2. 普通导航不再调用 `listSkills()`。库存独立为“更多操作”的显式 SkillHub status 查询，并提示可能保存状态／访问已配服务。未查询显示“未加载”，不以零假装已知空库存；切换 Workspace 清除数量，迟到结果不能进入新 Workspace。这不删除准备检查中已封存的真实 status 观察。
3. 新增 `preparationNavigation.test.tsx` 五项定向测试：只读导航／SSE／准备开关；显式配置读取及关闭重开；跨 Workspace 配置迟到与失败；显式库存动作；跨 Workspace 库存迟到。仅将无关指标卡样式替为保留原 label/value 的轻量渲染，页面动作、弹窗及异步请求逻辑仍真实挂载。未修改 5 秒单测期限。
4. 生产验证器保留完整 SkillHub 命令历史、精确顺序／目录与单次准备效果，新增要求导航期间额外库存调用为零；Provider HTTP 仍要求完整增量为零，包含 catalog GET。对应反例测试拒绝额外库存、重复准备、缺失配对和未知命令。
5. 现行 service-api 同步上述 Web 请求边界。旧批次 c 的失败证据与报告保持原样，没有改成通过。

## 同批次实际浏览器及进程验证：e

受限原始证据根：`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-navigation-20261027-e`。

构建并启动仅回环 `http://127.0.0.1:59929/console/`，通过本轮内置浏览器操作实际嵌入生产 Web。无需关闭鉴权、伪造请求头或重新借用旧服务。

- 开页及准备面 GET：revision 22、`not_checked`，没有检查记录。
- 明确点击一次检查：原 request `ac16f627-e432-47c8-a2e9-268d82444cd6`，原 revision 22，提交 revision 24；显示完整 Bootstrap 编译产物和 10/11 的 blocked 原检查。
- 关闭重开、准备只读刷新、整页重载后重新导航：保持原 request/revision，未追加 attempt；没有借 GET 补造页面内存编译结果。
- 浏览器退出到 `about:blank` 后，原脚本继续同批 HTTP 重放／恢复、缺配置阻塞、人工恢复后显式新查、丢 HTTP body 对账，以及两次真实进程 stop/start。
- 3 个原检查，10/11 → 缺配置 9/11 → 人工恢复 10/11；历史不重写，恢复不重跑原 status；原 Bootstrap 在重启后按原归档重编一致。
- 新增 Provider HTTP 0、embedding 0、Run 0；浏览器额外库存 status 配对 0。准备检查自身每次一对明确的 global/project status，未混为导航库存。
- PID：3873192、3874499、3874580，最终均已停止。

证据索引：

| 材料 | 文件 |
| --- | --- |
| 控制构建、源码和二进制身份 | `build-commands.json`、`pre-go-build.json`、`build-identity.json`、`console-build.log`、`go-build.log`、`workorch` |
| 真实会话 scope／端口 | `browser-session.json` |
| 开页未检查 | `browser-not-checked.txt` |
| 实际显式检查及画面 | `browser-explicit-check.txt`、`browser-explicit-check.png` |
| 关闭重开、只读刷新 | `browser-reopen.txt`、`browser-refresh.txt`、`browser-readonly-refresh.png` |
| 整页重载后原身份 | `browser-page-reload.txt`、`browser-page-reload.png` |
| 完整原请求与响应／owner readback／进程／调用计数 | `http-*-request.bin`、`http-*-response.bin`、`original-preparation.json`、`data/`、`process-*.log`、`skillhub-calls.jsonl`、`result.json` |

`result.json` 的 `pass=true` 仅表示脚本确定性门禁通过；`browserVerified=false` 保持不变，因为脚本自身不验证浏览器操作。浏览器结论来自本轮实际操作及上述 DOM／截图，`browser-done` 只是恢复进程验证的同步标记，不是验收证明。

## 最终源码重建与身份：f

e 完成后修正新增组件测试中的精确 label 断言，并拆除一项与后继测试重复的整页切换步骤；没有修改生产行为或放宽期限。随后从最终树重新构建，并在全新隔离目录执行非浏览器 HTTP／两次进程重启复验：

`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-navigation-20261027-f`

f 的 `result.json` 为 `pass=true`，3 attempts，Provider HTTP 0、embedding 0、Run 0，仍为 10/11、blocked、无 ready/M3。PID 3887710、3887771、3888120 均已停止。

| 身份 | e 实际浏览器批次 | f 最终源码批次 |
| --- | --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`＋当时 dirty tree | 同 HEAD＋最终 dirty tree |
| buildInputSha256 | `044ee7bde2ac1fadd85aeca22212b7299b77214987eccfa42caf9f0313902691` | `af7f45feb898dd2d79f551479e155fba213b835e404fe7bd356ad0960c03e100` |
| consoleTreeSha256 | `558280f9f714068c07b1c0f15ce1122e0484cbca022657ea35547220d6f4aa28` | 同 e |
| binarySha256 | `d37f3eb033c6f3b135390528ebc46ebe8d3f71ec98b8bea5e8499a8a4ff8cfe9` | 同 e |

实际执行 `cmp e/workorch f/workorch`，退出 0；完整 SHA256 另存 e 的 `binary-equivalence.sha256`。因此实际浏览器使用的产物与最终树重建产物逐字节相同；没有把两个不同 build-input hash 写成相同源码树。

f 固定 `expected-identity-final.json` 后运行公开 `build_identity.py verify` CLI，退出 0，输出保存在 `identity-verify-final.log`。这是本机内容关联记录，不是可信构建 attestation，也不是最终已提交 SHA 的发布验收。

e/f 的每批候选准备各有 8 次仅回环 fixture responses（合计 16），前序 catalog 读取另记基数。它们不是真实模型验证；STEP04 增量 Provider HTTP 始终为零。

## 最终自动验证

日志保存在 e 证据根，不将原始诊断提交到 Git。

| 检查 | 结果 | 日志 |
| --- | --- | --- |
| Web 组件完整集合 | 14 文件、202 项通过 | `web-components-final.log` |
| Web 规则 | 11 项通过 | `web-rules-final.log` |
| STEP04 verifier 单元 | 14 项通过 | `verifier-unit-final.log` |
| 构建身份单元 | 17 项通过 | `build-identity-tests-final.log` |
| `GOCACHE=/tmp/workorch-go-cache make verify-boundary-contract` | 通过 | `boundary-final.log` |
| 生产 console／Go build | e、f 均通过 | 两批 `build-commands.json` 和构建日志 |
| 最终构建 CLI verify | 通过 | f `identity-verify-final.log` |
| `git diff --check` | 通过 | 本轮命令结果 |

首次 boundary 命令未设置 GOCACHE，因默认缓存目录只读而失败；切换任务专用 `/tmp/workorch-go-cache` 后重跑通过。组件测试保留既有 Ant Design useForm／deprecated 告警；未据局部通过宣称全 CI。没有重跑全仓 Go／继承 fullflow／orchestrator 超时问题，也没有运行真实 Provider。

## 仍未完成的主体与 live 边界

当前可以复验的是**已封存的有界 profile、无 Run、空 Skill/MCP/Memory 选择、Provider 精确阻塞**的生产主链，不是完整正 readiness。以下仍属代码主体，不能包装为仅报告整理或 Provider 接口配置：

1. 非 Provider 资源的统一 currentness 消费：完整 owner scope/revision/epoch、有界 token／期限、所有 required 最早 expiry、提交前复核及重启失效。既有前后像和时点 passed 不能直接当作长期 current；本轮没有为它们发明 lease。
2. 正式成功准备／历史 M3：将完整原 Bootstrap、全 required 有效事实、已批准 no-Run final CAS 组合成可持久恢复的历史 completion；唯一 completionID、当前失效分轴和相应 HTTP/Web 正例尚未接通。现行 v2 记录故意不能表示该成功。
3. 若扩展到非空 Skill、非空 MCP 或语义 Memory，须分别完成原事实消费者及相应资源正例；不能从本轮精确空选择证明推及非空资源，也不能通过 list 代替连接／选择／授权。

Provider 只读授权合同仍单列 blocked，本轮不处理。后续优先实现上述 currentness→成功结算/M3 主体，再验证正向 production Web／HTTP／重启；不把 STEP05 Start/Restart 消费或收费 fullflow混入本轮。
