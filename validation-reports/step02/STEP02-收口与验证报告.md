# STEP02 收口与验证报告

日期：2026-10-17。范围仅为已保存 M1 → 显式生成 → 派生 → proposal → 六审 → 持久化／只读展示／找回。

结论：**主要实现、确定性真实模块集成、独立 HTTP／进程及生产 Web 成功链已通过；真实 provider 成功链未通过，STEP02 尚未满足全部退出条件。** 未执行用户确认、正式封存、准入或 Run；未提交／推送本轮修改。

## 构建身份与证据保管

本报告证明的是基于 HEAD 的未提交候选，不是 HEAD 原提交通过。

| 字段 | 当前候选 |
|---|---|
| HEAD | `0df7498ae372ece1be9c63fb999c2b068889e31a` |
| buildInputSha256 | `c564a78c51f5ef7d8c598225306003e7b37c2a22658c3c2caa03e0d486d5fdac` |
| consoleTreeSha256 | `7930b2f77aeb24443c6b2cedc1ed63108cf02b15e93bd5257f76e5844d6fdbdc` |
| binarySha256 | `44179ec95bbe9187acf7223bc3fa18134a6135b12284928b906c5e1a728bfc8d` |
| fixture 配置 SHA256 | `12c240a6c6157c16d0a85707a382e7343f8c96799df39eb4354b3ce0975af9a5` |

证据根为本文件所在目录 `.validation-tmp-step02-Echood`，持久、受限且 Git 忽略。原始配置、诊断、运行数据不提交 Git。`fixture4/build-commands.json`、`pre-go-build.json`、`build-identity.json` 证明验证器自行先构建生产控制台再构建 Go；每次启动检查真实 `/proc/<pid>/exe`，结束重新校验源码／console／binary。`candidate.patch`、`new-source-files.tar.gz` 与 `embedded-console.tar.gz` 保留本轮改动与生成资产；不是工具链可复现构建证明。

首轮真实 provider 使用 verifier 的前一版本（buildInput `c996cbc0…`），后续仅调整 verifier 的仓库 envelope 断言与证据采集；两版生产 binary／console SHA256 完全一致。当前构建又对原失败事务重新执行了进程回读／重放，见 `live-failure-readback/result.json`。不能把旧 verifier 记为当前版本已运行成功。

## 已完成实现（F2-01～F2-10）

- 已保存输入的独立只读 STEP02 modal，通过正式 Workspace 摘要及草稿详情进入；不导入 create／PUT／confirm／activation API。
- compile 必填 `expectedInputHash` 和 `expectedTemplateSnapshotHash`，HTTP、同步／后台编排、实际派生／编译 query 后都检查；过期拒绝于 reserve 前。所有原相关 Go／Web／脚本调用方同步升级，不兼容“缺字段用最新输入”。
- 完整 canonical Dimensions／Requirements／Obligations 只读投影，保留 Goal 原文、required／all_of、来源、逐义务 DECLARED 绑定及六审 reason／coverage／refs／proposal hash；不传 File 正文。
- 202 受理与 owner 持久结果分离；200 不直接显示成功；准备失败、明确失败、审查阻塞和 reserve 未决分别呈现。无通用自动重试或换身份。
- query 串行、session fence、revision 防倒退；冲突只回读，迟到旧会话不能污染新页；完成重放沿既有 owner ledger／FreshPermit 防止重复调用。
- 使用既有 Module／typed EventHub／Workspace 与 File／LLM owner 边界；`go-multi-module-dev` 用于保持这些职责。不新增运行单元，不改变 Runtime SM-1～SM-4，不扩大到下一阶段。

## 分层验证与验收矩阵

| 案例 | 实际证据与结论 |
|---|---|
| A2-01 显式边界 | 生产 Web 先仅保存草稿，再经规格入口打开／只读刷新；owner record SHA256 前后均 `fd11dc5f…`，provider 请求仍仅原 HTTP 样本的 8 次。显式点击后该 Web 草稿才新增 derive＋proposal＋六审 8 次。PASS |
| A2-02 精确输入 | 新增 HTTP tests 覆盖缺 body／缺 hash 400，错 input/template 409，排队后 A→B→A／模板漂移，无 reserve／provider；内部真实 query 后再核验。PASS（确定性，不冒称浏览器延迟注入） |
| A2-03 源承诺 | 固定清单 `fixed-source-commitments.json` 在生成前保存；owner 累计测试覆盖 missing／duplicate／mixed／dangling／来源漂移、独立 Goal；人工对正常 fixture 的原 Goal／两用户要求及模板必需义务逐项复核，required 与 all_of 不弱化。PASS（fixture） |
| A2-04 六审 | 真实模块集成及 fixture POST 原文证明六个精确 kind 同 proposal，均不含其他 reviews；Web 完整呈现 reason／required coverage／refs／hash。owner 累计负例覆盖缺类、重复、空依据、少 Goal、错 hash。PASS（确定性＋Web） |
| A2-05 声明 | fixture 逐 obligation 均为已发布 software-dev／`*`／dodItem 的 DECLARED；页面明确“不是运行验证已通过”。已有 owner applicability／UNVERIFIED 阻塞用例随累计回归通过。PASS（确定性），真实 provider 未取得合法绑定 |
| A2-06 受理／重放 | Web 先显示 draft＋“已受理不等于成功”，后只读观察 ready；组件双击／202／200 原态／409 回读，以及真实 HTTP 并发 FreshPermit、完成重放零增量。PASS |
| A2-07 分类 | provider malformed／截断／拒绝／未决的 owner＋受控 adapter 回归通过；真实 provider 本轮 invalid_proposal 保留原事实，未冒充未决或 PASS。PASS（负例），不能代替 live 成功 |
| A2-08 持久切点 | 累计 File／HTTP 用例覆盖 reserve／settlement／diagnostic CAS／file reopen；fixture4 真实 stop/start；Web 原候选再次两个新 PID 回读与重放，owner record 字节不变。PASS（P；浏览器未做故障丢包注入） |
| A2-09 准备恢复 | 既有 typed preparation／no-permit／旧诊断观察窗及恢复、Web 假时钟累计回归通过。15 秒不改为全流程超时，无超时重发。PASS（确定性） |
| A2-10 预算／漂移 | 原 owner budget／配置绑定／CAS 门禁保留，累计 suite 覆盖 typed budget、unknown reserve、竞争及配置漂移。真实失败只有原两次调用，重启重放 owner ledger 不变。PASS（确定性及本轮负例） |
| A2-11 找回／会话 | 生产 Web 关闭 modal 重开、真正 reload 后从列表→草稿详情→候选，找回同 input／proposal／六审；109 组件测试包含跨 Workspace 迟到／串行 GET／revision 倒退。PASS |
| A2-12 累计／真实 | Stage1 原件核对及受影响累计通过；真实 ai-proxy/deepseek-flash 在 proposal 阶段失败，六审 0。**成功门槛未通过** |

### 生产 Web 原候选身份

- Workspace：`ws-ca235ee32103469bbfc4d10cca2724fdeafd26c4c11019acae0b3b4ad2cef4ba`
- 创建 requestId：`f018b58c-96f0-4d1a-a39f-d5ffbb971552`；input revision 1。
- inputHash：`definition-authoring-sha256:3e54bf4542853535fa07a658c85eb44a6be1e9976461b2827effbbedc5b4e44b`
- proposalHash：`goal-specification-sha256:a2a77a1413d69fcfce43c3830d602fd93efadde55d037b2dc18ddce1c1a2cdfb`
- specificationHash：`goal-specification-admission-sha256:fedc61994099167db7ea32fbd2e1f1ed508289a42df8f35f1b1052a5a873fb3a`

原始截图／DOM：`fixture4/web-draft-open.*`、`web-after-explicit-generation.dom.txt`、`web-ready.jpg`、`web-reopened.dom.txt`、`web-after-reload.dom.txt`、`web-final.*`。原始 HTTP：`web-candidate-http.json`。独立重启 readback：`web-result-readback`；PID `1786496 → 1786522`，同 owner record SHA256 `20a25260…`。fixture4 主 HTTP 样本另有 PID `1784007 → 1786283`，6/6 PASS；两样本总 16 次物理 provider POST。

浏览器导航／modal 动画间有一次 selector deadline，随后根据真实 DOM 重新定位；未将失败点击算成完成。生产 Web 请求以 `fixture4/process-1.log` 与 owner ledger 为准，脚本 `http-trace.json` 仅记录脚本自己的请求，不虚称捕获全部浏览器请求。

## 真实 provider：失败且未闭合成功门槛

用户明确允许现有配置及固定非敏感草稿，未另设费用上限；仍保留原 owner／请求预算。原 `~/.workorch/application.toml` 未修改，只在隔离文件仓配置中替换 listener 和数据根。未操作用户 8090 服务。

原 Workspace 为 `ws-01bae6399aab591c074c722e3ce885f7ae0be9e589eda242dabeb10adabcb17f`，状态 `failed`，failureCode=`invalid_proposal`。共两次 provider-native usage：

| 阶段 | input tokens | output tokens | total | 原始响应 bytes |
|---|---:|---:|---:|---:|
| derive | 961 | 3240 | 4201 | 79542 |
| proposal | 11105 | 10099 | 21204 | 560071 |

合计 25405 tokens。provider 传输完成不等于业务候选通过；proposal 返回的 22 项义务均 `UNVERIFIED` 且 Declaration 为空，因此 owner 在“traceable obligation and verification”检查拒绝，未启动任何 critic。

完整 proposal document 11880 bytes 已保存，SHA256 `a98884d27702decf0f56506e69cddbb30f6e63124844e937ad9616e4d887c8e6`；解析错误 169 bytes 已保存。wire 虽完整观察到 560071 bytes，但超过原存储上限，明确 `saved=false / observation_storage_limit`，不能宣称原始 wire 已保全。诊断 HTTP 原件位于 `live-failure-readback/diagnostic.json`；usage 原件位于 `live/data/system/events/events-000001.jsonl`。报告不改写原材料。

随后使用当前同字节 binary 对原事务执行两次进程启动、同身份 GET／compile 重放与诊断读取；原 owner record 字节完全不变，仍失败，provider usage 总数仍 2。没有改 Goal、换模型／key、清预算、创建替代 live 草稿或将失败改成 ready。PID `1785569 → 1785596`，owner record SHA256 `12bfd9bc…`。

遗留项：核对真实模型在当前映射提示／Profile 声明下为何输出空声明和全 UNVERIFIED，并在保留失败及全部原准则、预算和许可边界的前提下确定后续修复／新验证事务；之后还需合法 proposal、六审、持久找回与人工语义复核成功。当前不宣称这一定是代码门禁错误，也不以 fixture 成功替代。

## 累计回归与 Stage1 承接

- 受影响 workorchd／workspace／managementcontract／client Go suite：`affected-go-final.log` 全部 PASS。
- production owners 集成（只替换外部 provider HTTP 响应）：`production-owners.log` PASS。
- 新 STEP02 HTTP 定向：`step02-http-tests.log` PASS；race：`race.log` PASS；vet：`vet.log` 退出 0；boundary：`boundary.log` PASS。
- 前端组件 10 files／109 tests PASS：`web-components-final2.log`；规则 11 test files PASS：`web-rules.log`。既有 useForm warning 不算失败，不替代浏览器证据。
- live client Python 26 tests、build identity 16 tests、STEP01 controlled-build 5 tests、STEP02 verifier 最终 4 tests 全通过，分别见同根对应日志。`git diff --check` 通过。
- 初始旧 Web compile 断言和 TS 字段落点错误均已修复并重跑；旧失败日志保留。前两轮 fixture 因模拟 `/models` 缺少 `object:list` 被真实 owner 受理后在 provider 出站前拒绝，状态未决、0 POST；保留原文件仓，不重发旧 permit。fixture3 成功但使用较早 verifier；最终以 fixture4 的精确 envelope 断言为准。
- STEP01 归档原件 `docs/archive/step01-验收记录.md` 已核对；原 `/tmp/workorch-step01-m5uurybi` 尚存在，已原样复制到本证据根 `step01-baseline`。原 manifest 的 source/build/console/binary 四身份与归档一致，实际原 binary SHA256 `804e61d7…` 一致。本次受影响累计包含创建、部分持久化身份、列表／查询和构建拒绝回归；不把原 binary 冒作新源码验证。
- 所有本轮隔离进程已正常停止，数据保留。未跑全库 CI、全部 Runtime fullflow 或任何确认／执行 live；不形成发布或 STEP03 结论。

机器可读汇总：`final-summary.json`。最终证据校验脚本：`finalize-step02-evidence.py`。最终状态：S2-A 成功；S2-B 的确定性及受影响累计通过；S2-C 成功退出未通过，**STEP02 未全部验收退出**。
