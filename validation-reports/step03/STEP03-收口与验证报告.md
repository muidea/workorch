# STEP03 收口与验证报告

日期：2026-10-19。开发基线：`32c88cfc173a47531d88b6c53228d8abe2105277`，本轮修改尚未提交。

## 结论与范围

独立精确确认入口、原意图恢复及限定票据补交已实现。确定性真实 owner/HTTP、生产进程成功/阻塞链、实际 Chrome 显式确认及其后真实重启验证均通过。**本轮 STEP03 计划范围内 S3-A／S3-B／S3-C 收口与验收退出完成，无已知未关闭的本步阻塞项。** 这是对下列具体证据范围的结论，不代表全库或完整 Runtime 已验收。

本阶段只确认、物化、封存和准入；不开展运行资源就绪检查、不调用 execution bootstrap，不启动 Run，不修改 Runtime SM-1～SM-4。完整 M3、激活准备、真实收费模型和全库 CI 不在本轮通过结论内。未修改用户现有服务、数据或 `~/.workorch` 配置，未自动提交 Git。

## 实现

1. 总览/Workspace 详情的已保存候选面增加纯导航，进入独立 STEP03 确认面。打开/刷新/关闭只 GET；不串联 create、PUT、compile 或 execution。完整候选沿用来源、required/all_of、义务、六审、DECLARED 展示。
2. 显式审阅后冻结 Workspace 及原五字段；双击单飞。任一 POST 结果后独立 GET 对账，包括 409/断网；保留原 revision 和四项身份，不自动换到最新候选或重发 POST。
3. 重开仅采用经 scope、状态、模板、hash、安全 revision 核验的 owner recovery；迟到 GET/POST 和 revision 倒退不能改写新会话/原意图。另一个 eligible 输入不能冒充未决原确认成功。
4. 限定 materialized、原 authoring candidate 精确绑定且票据缺失/未验证的显式票据交付恢复；复用已有后端确认/admission 幂等路径，不扩展通用 clarification 重试。不修改规格或解决票据，恢复后仍阻塞。
5. 收紧票据投影：仅落盘不代表完成交付；创建通知标记缺失、非开放/部分 resolved 生命周期不能展示为已验证 blocker，GET 不补交。规范见 `docs/00-core/definition-convergence/clarification.md`。
6. 独立修复 STEP02 R2：15 秒只结束主动观察，保留原诊断基线，后续显式 GET 读到新诊断可解除受理锁；旧诊断和单纯超时不解锁，零自动 POST。

使用 go-multi-module-dev 保持现有 Workspace/File/Decision owner 及 concrete typed EventHub 接线，未新增运行单元、跨 owner repository/service 或 Runtime 权威路径。

## 当前构建与原始证据

生产进程证据根：`.validation-tmp-step03-http-20261019-03/`。

浏览器验收/归档根：`.validation-tmp-step03-browser-20261019-01/`，目录权限 0700。其 `build-identity.json` 与上一根完全一致：

| 项目 | 身份 |
|---|---|
| sourceCommit | `32c88cfc173a47531d88b6c53228d8abe2105277`（包含本轮未提交源码，不把 HEAD 单独当构建身份） |
| buildInputSha256 | `ccc5c138113f073a68e87fe7a49e7919401790fa2e561f60323421730f2cf7bd` |
| consoleTreeSha256 | `48b72908f1db2d16b1a621245efd14e67773af5a4186a5176147daeb16da02e1` |
| binarySha256 | `c54ead7ea44bb8082122844184e19b92428f8d5f8fb38a681a3edef48cc5a648` |

验证器自行先构建 production console、再构建 Go binary，检查构建前后源码一致性，每次启动验证真实 `/proc/<pid>/exe`，结束重新校验完整身份。对应 `build-commands.json`、`pre-go-build.json`、`console-build.log`、`go-build.log`；不是工具链可复现构建或可信 attestation。浏览器根另存 `candidate.patch`、`build-source-snapshot.tar.gz`，不含 live 配置/凭据或 runtime 数据。

HTTP 根中的 `prepared-candidates.json`、`confirmed-owner-facts.json`、`http-trace.json`、逐次 `http-*-{request,response}.bin`、`process-*.log` 和 file repository 保留完整链：input → derivation permit → proposal → specification → materialized journal → authoring candidate → startup/eligible；另有 blocked candidate/ticket，正式 Goal/DoD 仍可见但无 promotion。

## 实际执行结果

| 检查 | 结果与证据 |
|---|---|
| 全部 Workspace/Decision/workorchd biz 相关 Go 包累计，management contract | PASS；HTTP 根 `owner-regression.log`。命令：`go test ./internal/modules/kernel/workspace/... ./internal/modules/blocks/decision/... ./internal/modules/application/workorchd/biz/... ./internal/pkg/workorchmanagementcontract -count=1` |
| STEP02/03 production owners | PASS；浏览器根 `production-owners.log`，`go test ./internal/integration -run 'TestAuthoring(CandidateStep02\|ConfirmationStep03)' -count=1` |
| HTTP 确认/票据恢复 race | PASS；HTTP 根 `race-http-allowed.log`，`go test -race ./internal/modules/application/workorchd/biz -run 'TestAuthoringHTTPStep03TicketDeliveryRecovery\|TestAuthoringHTTPFileBackedCompilationConfirmationAndRecovery' -count=1` |
| Web 组件累计 | PASS，11 文件、127 项；`.validation-tmp-step03-http-20261019-02/web-components.log`。`npm run test:components -- --reporter=dot` |
| Web 纯规则/API 累计 | PASS，11 测试文件；浏览器根 `web-rules.log`，`npm test` |
| STEP01/02/03 verifier 规则 | PASS，14 项；浏览器根 `verifier-tests.log` |
| 构建身份防旧产物测试 | PASS，16 项；浏览器根 `build-identity-tests.log` |
| 相关 Go vet / 边界检查 / diff whitespace | PASS；浏览器根 `vet.log`、`boundary.log`；`git diff --check` 退出 0 |
| 新鲜生产构建、真实 HTTP 与进程重启 | PASS；HTTP 根 `result.json`、PID 2671617 → 2673668（均正常停止）。运行 `python3 scripts/verify-workspace-confirmation-step03.py --evidence-dir <new-dir>` |
| 实际 Chrome + 同环境真实重启 | PASS；浏览器根 `browser-observations.md`、完整 DOM/截图、`confirmed-owner-facts.json`、`result.json`。PID 2674601 → 2685794，均正常停止。用户手动打开并共享隔离页面后完成成功链、阻塞链；未修改鉴权或伪造请求头 |

生产进程准备两个候选共 16 次 loopback fixture 推理（每个 derive/proposal/六审），这只是 STEP02 准备，不是真模型质量验证。确认/准入/重放/重启阶段新增 provider 推理 0，embedding 0；fixture 记录全部 POST 路径（含拒绝端点），只有准备阶段的 16 个 `/v1/responses`。两个 Workspace 及全局 Run inventory 均 0、交付目录为空，未提交任何 execution intent。确认阶段允许必要 File owner 资源读取，不能宣传零资源查询。

### 实际浏览器证据索引

本轮地址 `http://127.0.0.1:56147/console/`，验收后隔离服务已停止。浏览器观察在实际共享的 Chrome 标签页完成，不是模拟 DOM 或仅组件测试：

- `browser-ready-dom.txt`、`browser-ready.jpg`：成功候选完整原文/来源/六审；未勾选审阅时确认禁用，显式只读刷新不确认。
- `browser-eligible-dom.txt`、`browser-eligible.jpg`：审阅后显式确认一次，显示 admitted/materialized/eligible、startupInputHash，并明确尚无 Run。
- `browser-reopened-dom.txt`：关闭重开后从 owner 恢复同一封存身份，不再点击确认。
- `browser-blocked-ready-dom.txt`：独立阻塞候选原身份、ready_to_confirm、revision 19，未审阅时确认禁用。
- `browser-blocked-dom.txt`、`browser-blocked-refreshed-dom.txt`、`browser-blocked.jpg`：一次显式确认后 materialized、clarification_required，缺少必需 Profile deliverable proof；violation 为 `definition_profile_contract_invalid`，offendingRef 为 `profile/software-dev/deliverable/step03-required-unproven`，已验证 ticket 为 `workspace-definition-clarification:d478404193f29331889c6b1fdf43e1bf`。刷新仍阻塞、恢复按钮禁用，无 startupInputHash，未解决票据。
- `browser-observations.md`：操作事实摘要；`result.json`、`confirmed-owner-facts.json`、`http-trace.json`：随后独立 owner 核对、原 tuple 重放、真实进程重启及重启后的 recovery 重放，身份和事实不变。

浏览器全部 DOM 保留在受限目录，以上只是索引，不以摘要替代原始证据。实际浏览器未注入响应丢失或迟到请求；这些故障由定向组件/真实 owner 与 HTTP 测试覆盖，不扩大浏览器通过范围。

### 首次失败与保留边界

- `.validation-tmp-step03-http-20261019-01/`：新鲜构建完成后，沙箱禁止创建 socket；未启动验收进程，不是产品成功证据。
- `.validation-tmp-step03-http-20261019-02/`：初版阻塞 fixture 试图覆写父模板的不兼容 deliverableExpectations，被继承门禁 409 正确拒绝。改为独立 proof-incomplete 模板，不放宽产品门禁；原 409 保留在 `http-001-response.bin`。
- HTTP 02 根的首次 broad owner 回归受 socket 禁止影响失败；HTTP 03 根 `race-http.log` 首次碰到 console rebuild 的短暂无嵌入资产窗口。均保留，随后串开资产构建/Go 编译并在获批本地 socket 环境重跑成功。不把这些环境失败抹成 PASS。
- 构建仍有 Vite chunk 大小提示，组件测试仍有既有 `useForm` 提示。没有运行完整 `make ci`、全 Runtime fullflow 或发布验收，不能称全 CI 通过；此前评审的继承 CI 红灯不在本轮擅自修复。

## A3 验收映射

U=纯规则/单 owner，P=真实模块持久边界，H=HTTP/真实进程，Wc=组件，Wb=实际浏览器。以下是具体证明范围，不将每一行声称为独立新建的端到端用例。

| 编号 | 已执行证据映射 | 当前边界 |
|---|---|---|
| A3-01 | `authoringConfirmation.test.tsx` 开页只读/五字段/单飞；production owner 精确确认/重放；生产 HTTP | P/H/Wc/Wb 通过；浏览器执行显式审阅确认与只读刷新 |
| A3-02 | `TestAuthoringConfirmationRejectsIncompleteAndDriftedInputWithoutRows`、现有 route DTO/身份测试、HTTP stale input/revision prepare 前拒绝、物化后四 hash 错误拒绝 | U/P/H 通过各既有断言；不把物化后的 409 说成无写 |
| A3-03 | `TestConfirmableSpecificationRequiresUniqueCompletedPASSReviews`、authoring specification/model 覆盖与声明反证、`definition_authoring_unverified_test.go`、declaration applicability 及 HTTP review FAIL/UNKNOWN | U/P 通过；六审 fixture 不代替真实语义质量验证 |
| A3-04 | `TestAuthoringConfirmationRecoversEveryFileBackedCutpoint`、`TestAuthoringProcessConfirmationRecovery`、隐藏部分行/冲突正式行测试 | P/子进程通过；生产成功/阻塞 materialized 重启 H 通过 |
| A3-05 | 模板/配置漂移测试、`TestResourceBoundAuthoringConfirmationAndAdmissionRecheckFileAfterReopen`、resource stale proof、current configuration directory fence、HTTP resource_drift | U/P/H 通过；不声称 File 最后读至 CAS 间跨文件原子性 |
| A3-06 | 完整 candidate 展示；production owners 核对 required 原文/声明，HTTP journal/candidate/startup 身份及正式行；source-by-source specification tests | U/P/H/Wc/Wb 通过；浏览器原文审阅与完整 DOM 可复核 |
| A3-07 | `TestAuthoringConfirmationStep03ProductionOwners`、file-backed eligible/replay、production HTTP/真实重启身份不变 | U/P/H/Wc/Wb 通过 |
| A3-08 | alternative/Profile proof/prerequisite 既有 admission tests；独立缺必需交付证明模板真实 HTTP 阻塞，无 startup/Run | U/P/H/Wc/Wb 通过；浏览器展示实际 proof 缺失阻塞及 verified ticket |
| A3-09 | `TestAuthoringHTTPStep03TicketDeliveryRecovery` 两切点，GET 不补交，原五字段恢复同 ticket；Decision 实际进程 kill 切点；未持久/伪造/已 resolved fact 拒绝与 pending/ack tests | U/P/H/Wc 通过；不是通用修订入口 |
| A3-10 | file/process 切点、响应丢失重放、CAS/断档反证；页面 409/网络失败后 GET 对账原意图、无自动 POST | U/P/H/Wc 通过 |
| A3-11 | 组件跨 Workspace 迟到 GET/POST、倒退 revision、恢复异常、重开/pending 只读、另一个 eligible 不能冒充原意图 | Wc + owner 持久回读通过；Wb 完成成功候选关闭重开及切换阻塞 Workspace，不冒称浏览器注入故障 |
| A3-12 | STEP01/02 受影响累计、R2 截止后旧/新诊断测试、生产 fixture 全 POST 计数/无 Run/空交付目录、边界检查 | P/H/Wc/Wb 通过；同构建浏览器后真实重启 result.pass=true |

## 剩余项

本轮 STEP03 范围内无已知剩余阻塞项。未验证/未声明通过的范围：真实模型语义质量重新验证、全库 CI、完整 M3/Runtime fullflow、激活/执行与发布验收；不得将本步 eligible 解释为运行资源就绪或 Run 已启动。既有 CI 红灯仍按继承问题披露，本轮未默认重跑或扩展修复。

本轮修改尚未提交；用户提供的计划原文件保留不变。原始诊断、运行仓库和配置仍保存在 Git 忽略目录，仅报告/索引进入可提交文件。
