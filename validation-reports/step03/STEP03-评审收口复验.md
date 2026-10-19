# STEP03 评审收口复验

日期：2026-10-19。评审基线：`ec7ca2565cbfa79159fcb850aa2c1964f015e43f`；以下为其上的未提交修改。

## 本轮结论

P3 文案与评审所列非阻塞验证补强已完成；定向及累计回归、新生产构建 HTTP／真实重启复验通过。未修改确认／准入权威、owner 边界、Runtime SM-1～SM-4 或激活／执行行为，不增加 Run 查询或浏览器持久状态。使用 go-multi-module-dev 核对现有 Workspace／File／Decision 与 typed EventHub 边界，没有新增运行单元。

本轮没有收费模型调用或 Run；也没有重跑实际 Chrome。原浏览器证据仍明确归属原快照，本轮文案通过组件回归和新嵌入产物构建核对，不将旧截图当新文案的浏览器实测。

## 修正与覆盖

1. 成功提示改为“合同已确认、封存且 eligible；确认操作不会启动 Run”。authoring/admission 没有 Run inventory，页面不再声称当前没有 Run。新增 eligible 重开定向测试，核对新文案、排除“尚无／当前无 Run”、只读且不确认／派发；无需为了复现该文案缺陷实际启动 Run。
2. STEP03 verifier 对正式 DoD、journal 及已审 proposal 逐项核对原文与顺序，并核对正式行 ID／Workspace／revision；candidate 另按 ref 比较完整来源／文本集合，保留其既有规范排序，不排序或改写正式内容。新增反例覆盖文本、顺序、缺失／多余项、scope、ID／revision、来源漂移和重复。
3. verifier 核对 activation 的 CandidateID／CandidateRevision／CandidateHash 与所选 candidate 精确一致；核对 candidate／startup／activation scope、持久 eligible 状态、startup SpecificationHash 与原确认 specificationHash，以及既有 startup／definition 身份。新增每字段漂移反例，明确不能以 proposalHash 冒充 specificationHash。
4. 票据纯投影门禁新增表驱动反例：nil／缺 binding／缺通知标记、resolved／superseded／unknown 状态、Step scope、resolved 时间／主体／通知，以及全部 resolution／replacement／successor 残留字段。开放且交付标记存在仅通过生命周期子门禁，调用方仍须执行原有完整 candidate／ticket 绑定核验；未放宽任何生产逻辑。

## 原提交与已测快照的内容等价核对

原证据根：`.validation-tmp-step03-browser-20261019-01/`。本轮只读比较 `git ls-tree`／`git cat-file --batch` 获取的固定 ec7ca256 Git blob 与 `build-source-snapshot.tar.gz` 内对应文件原 bytes，范围为既有 build_identity allowlist 加嵌入 console 产物。共 5,164 个文件；缺失 0、字节不一致 0、额外范围内归档文件 0。

本轮另直接从 Git blob 重算源码／console 树内容身份，读取归档 binary 字节核对原 manifest：

| 身份 | 重算结果（与原记录相同） |
|---|---|
| buildInputSha256 | `ccc5c138113f073a68e87fe7a49e7919401790fa2e561f60323421730f2cf7bd` |
| consoleTreeSha256 | `48b72908f1db2d16b1a621245efd14e67773af5a4186a5176147daeb16da02e1` |
| binarySha256 | `c54ead7ea44bb8082122844184e19b92428f8d5f8fb38a681a3edef48cc5a648` |

核对脚本和完整 JSON：`.validation-tmp-step03-review-20261019-01/compare-ec7ca256-snapshot.py`、`ec7ca256-snapshot-recheck.json`。脚本 SHA256：`f592891a4e5bf411ec156c08906dc875be0a2b3773ba8c44e6273fc1621a31c1`；原 tar SHA256：`6a2e63aeb959744c501dd57ba627fa4dbe87fd41f9e8e86976ba3ff4cca7a78d`。可在仓库根执行该只读脚本复核固定提交，不依赖当前修改树等于旧源码。

这补齐的是原已测内容与 ec7ca256 构建输入范围的等价关系，不包括 docs／报告等非构建输入；不修改原 sourceCommit，不将作者验证冒称独立评审方实测，不证明工具链来源或二进制构建因果关系，不是可信 attestation。本轮新修改具有下列独立构建身份，不能沿用旧快照通过结论。

## 本轮执行与原始证据

回归证据根：`.validation-tmp-step03-review-20261019-01/`；HTTP 根：`.validation-tmp-step03-review-http-20261019-01/`。原始日志、请求／响应、配置和 owner 数据保留受限且 Git 忽略；本报告只提供索引。

| 验证 | 实际结果 |
|---|---|
| `npm run test:components -- --reporter=dot` | PASS，11 文件 128 项；`web-components.log`，包含文案定向断言 |
| `npm test` | PASS，11 测试文件；`web-rules.log` |
| `go test ./internal/modules/application/workorchd/biz -run TestAuthoringTicketDeliveryCompleteRejectsRetiredAndPartialResolution -count=1` | PASS，正例及 21 个反例 subtests；`ticket-projection-allowed.log` |
| `go test ./internal/modules/application/workorchd/biz/... -count=1` | PASS，含真实持久 owner／HTTP 恢复测试；`workorchd-regression.log` |
| `python3 -m unittest discover -s scripts -p 'test_verify_workspace*.py'` | PASS，18 项，其中 STEP03 7 项含逐字段反例；`verifier-tests.log` |
| `python3 -m unittest discover -s scripts -p test_build_identity.py` | PASS，16 项；`build-identity-tests.log` |
| `make verify-boundary-contract` | PASS；`boundary.log` |
| `python3 scripts/verify-workspace-confirmation-step03.py --evidence-dir .validation-tmp-step03-review-http-20261019-01` | PASS，生产构建、成功及阻塞链、精确重放、真实进程重启后重复全部加强断言；HTTP 根 `result.json`、`confirmed-owner-facts.json`、`http-trace.json` |

新构建身份：sourceCommit 为 ec7ca256 加本轮未提交构建输入；buildInputSha256=`118b7f2837bff282a61d63091935e68cc1bddffc821e22d61a4855cdcc015d91`，consoleTreeSha256=`045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b`，binarySha256=`3dc8e021d2a1f9e2f46f6a39207da1e766197a9bf1c25977ecb0eb7ab3474dfc`。完整身份及构建命令在 HTTP 根 `build-identity.json`、`build-commands.json`。

进程 PID 2705381 → 2705478，正常停止。准备两个候选共 16 次 loopback fixture 调用；确认／准入／重放／重启新增推理 0、embedding 0、Run inventory 0，交付目录为空。确认仍允许必要 File 资源查询；非真实模型语义质量验收。

首次纯票据测试因沙箱无法写既有 Go build cache 而失败，保留 `ticket-projection.log`，随后获准使用原缓存重跑通过，不将原失败覆盖为 PASS。首次临时离线核对命令遗漏 PYTHONPATH，未进入业务核验，修正导入路径后原两个快照均通过新断言；新生产进程复验不依赖该离线结果。原快照逐文件比较的首次随机 gzip 读取被主动中止，随后单次流式读取完整比较通过。构建 Vite chunk 提示、既有 useForm 提示仍保留，不修改无关依赖。

## 独立 CI 测试合同修复（另行授权）

用户追加的 CI 日志对应 `test_release_convergence_gate.py` 旧数量／位置断言。核对现有 Makefile readiness 已为 10 条，新增的是 Goal execution、历史用途证明和暂停／恢复等门禁；并非 STEP03 引入额外派发。修复前定向重跑该 Python 文件为 3 项中 1 失败，原错误 `AssertionError: 10 != 3`，日志 `inherited-release-gate.log`；`readiness-dry-run.log` 保留实际命令清单，不覆盖原失败证据。

用户随后明确授权“一并修复 CI 测试合同”。仅修改该测试文件：以独立声明的完整 10 条检查合同取代过期的三行数量和位置断言，逐条比较 shell tokens，包括包集合、原精确 -run 选择器、`--each-package`、`-count=1` 与超时；原有 CI readiness 前置顺序及缺证据不得打包测试保留。Makefile、wrapper、生产代码及 fullflow 本身均未修改。

新增反例验证逐条删除或重复任何检查都会失败；另覆盖移除 each-package、禁止测试实际运行的 count、关闭／放宽截止、漏包、删公开 fullflow／unsupported fail-closed／CommitEvidence，以及追加失败掩码。固定 3m 约束不变，变量超时分别 dry-run 验证 15m／20m 既有配置传递，不执行这些长时测试，也不提高默认预算。

本轮实际结果：

- `python3 scripts/test_release_convergence_gate.py`：PASS，5 项（含以上反例 subtests），`release-gate-fixed.log`。
- `make verify-deployed-convergence-manifest-tests`：PASS，原失败聚合目标实际完成其全部六个 Python 套件，共 90 项：构建身份 16、STEP01 verifier 5、LSP PRD 20、blackbox producer 21、部署 manifest 23、release gate 5；`deployed-manifest-gate-fixed.log`。
- `git diff --check`：PASS。本次独立测试文件不在既有产品 build_identity 源码范围内；上述 STEP03 生产构建身份仍对应其受测构建输入，不将本次 CI 合同测试冒称产品二进制功能实测。

这关闭了本地复现的 `10 != 3` 失败，未重跑托管 Actions 全流水线，不能声称远端全 CI／发布通过。完整 M3、Runtime fullflow、此前 CI 跳过的持久规格及 managed 阶段仍未作为本次合同修复执行，也未调用收费模型、修改现有服务／配置或提交 Git。
