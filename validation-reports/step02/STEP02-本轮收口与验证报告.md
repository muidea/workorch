# STEP02 本轮收口与验证报告

日期：2026-10-17～18。仅覆盖已保存 M1 → 显式生成 → derive → proposal → 六类独立审查 → 持久化／只读展示／找回。
本报告补充 `.validation-tmp-step02-Echood/STEP02-收口与验证报告.md`，不覆盖或改写原始失败证据。

**结论：本轮对应代码收口、确定性／历史兼容性回归及一条限定真实 provider 成功链均已完成；本次 STEP02 验收退出通过。**
此结论仅适用于下列最终未提交候选构建、固定非敏感输入和 software-dev 模板，不表示任意输入／模型均必然成功，也不是确认、执行或发布验收。
没有执行用户确认、正式封存、准入或 Run；未提交／推送本轮修改。

## 1. 已完成的对应修复

1. 生成请求明确每项 Declaration 的具体非空说明、真实 Profile 适用性及合并贡献；DECLARED 是验证能力声明而非已执行。未执行不能单独作为 UNVERIFIED 理由；缺能力时保留诚实缺口、三个绑定为空，不造能力。
2. provider schema 增加 Declaration 的非空提示约束。保留既有 decoder／owner 的历史重判语义与首错顺序，不将旧 semantic failure 改判成 parser failure，不补写原文。
3. 六审请求携带同一合法、未评审 proposal 的 exact `reviewEvidenceRefs`，只含原 Goal／候选 ref 和规格对象 ID；哈希或 Profile 身份不是新证据 ID。引用集合不授予执行或确认权。
4. 将 Workspace 既有独立 exact Goal 结构合同明确给所有 critic：Goal 不是候选 DoD，不借用或补造 Goal obligation／逐义务 Verification；仍检查完整 Goal 是否由候选义务及适用能力承接，文本存在不等于语义完整，实际缺口仍 FAIL／UNKNOWN。
5. 修正验收脚本的观察时机：首个 FAIL 投影为 review_blocked 时，其他独立 critic 可能还在执行；须六审全部结算再重启比较。未结算超时保留事实，不重发。
6. 增补生成合同、引用空间、历史失败原文重判、实际 Profile 适用性及真实 owner＋HTTP 负例。空声明、诚实 UNVERIFIED、伪 Profile 均不获得 ready；合法草稿缺口沿原规则保存，不能改变历史语义凑绿。

使用 `go-multi-module-dev` 保持既有 application／Workspace／LLM／File owner 及 typed EventHub 边界；没有新增运行单元或修改 Runtime SM-1～SM-4。

## 2. 最终候选构建身份

此处证明 HEAD 上的**未提交候选源码**，不是 HEAD 原提交通过。

| 字段 | 值 |
|---|---|
| sourceCommit | `0df7498ae372ece1be9c63fb999c2b068889e31a` |
| buildInputSha256 | `68dd2fb9bc3c78ba0a660aa64843db19680fca58b2c7b5382f56bac1bcad9bfa` |
| consoleTreeSha256 | `7930b2f77aeb24443c6b2cedc1ed63108cf02b15e93bd5257f76e5844d6fdbdc` |
| binarySha256 | `d15e95f5ac693e70a76e52d7cbbbda1e84738274795407757e0ea6f3253e716c` |

`fixture-final` 和 `live-final` 各由验证器自行构建生产 console 与 Go binary，四身份完全一致；启动核对 `/proc/<pid>/exe`，结束核对当前 source／console／binary。不证明工具链可复现构建。原 live 配置未修改，只在受限隔离根中变更监听和数据根；未触碰用户 8090 服务。

## 3. 当前回归结果

| 层级 | 当前结果／证据 |
|---|---|
| 生成合同、历史重判、引用空间 | `final-focused-tests.log` PASS |
| workorchd／workspace／managementcontract／client 累计 | `final-affected-go.log` PASS |
| 真实 owner＋EventHub＋File 持久＋HTTP（仅 provider 出站 fixture） | `final-production-owners.log` PASS，含主链及四类声明负例 |
| race | `final-race.log` PASS |
| vet／边界扫描 | `final-vet.log` 退出0；`final-boundary.log` PASS |
| Web 组件／规则 | `final-web-components.log`：10 files／109 tests PASS；`final-web-rules.log`：11 test files PASS |
| 验收脚本／构建身份／STEP01 累计 | `final-verifier-tests.log` 6 PASS；`final-build-identity-tests.log` 16 PASS；`final-step01-verifier-tests.log` 5 PASS |
| 隔离生产 HTTP／真进程停启 | `fixture-final/result.json` PASS；8调用、六审PASS、ready、完成重放零新增；PID `1849054 → 1850445` |
| 原失败回读兼容性 | `readback-final-original-failed/result.json` PASS；PID `1851284 → 1851314`，usage仍2，原owner SHA256 `12bfd9bcacba2bf67b3264da00e046960e74e1469ab6b16b3e570d61e99630bf` 不变 |
| 六审阻塞历史回读 | `readback-final-six-review-blocked/result.json` PASS；PID `1851369 → 1851429`，usage仍8，原owner SHA256 `206f8b2b775650c6b5ec7d5be663520b3702942d70894510e04ab133e17b4817` 不变 |

STEP01 原件与生产 Web fixture 成功链承接上一轮报告及其受限原始材料；本轮未修改 Web 源码或嵌入产物，console 哈希完全一致，且重跑组件／规则与受影响创建累计。组件通过不代替浏览器或真实 provider 验收。

## 4. 中间失败与修复依据（不计为成功退出）

- 原失败（上一轮）：proposal 22项空声明／UNVERIFIED，2调用、25405 tokens，invalid_proposal；原文与失败身份保留。实际 default Profile 审计确认22/22来源有适用且合并后贡献的声明，见 `prior-live-profile-applicability.json`；这是适用性证明，不代替语义审查／运行验证。
- 本轮 `live`：合法 proposal 后，第一个 critic 用 hash／自造 Profile ref 导致 invalid_review；3调用、59818 tokens。完整 proposal document 已保存；wire 755968 bytes 超限未保存，不宣称完整 wire 已保全。
- 本轮 `live2`：引用修复后六审完整结算，5 PASS＋dimension_coverage FAIL，8调用、211618 tokens；真实失败理由要求独立 Goal 有 DoD obligation，与既有结构合同不符。FAIL原文不修改、不补成PASS；截图 `live2/web-candidate-blocked.png`。
- `live2` 原脚本在仅5审时过早进入浏览器／重启阶段，后第6审真实结算，因此 equality 比较失败；不是数据丢失的证明。最终版脚本已修正，本轮最终二进制对其结算后的6审记录两次重启与重放全部一致。
- 本轮曾尝试将空声明提前拒绝，发现改变旧失败重判顺序使查询500；已撤回该 decoder／owner 强化。最终回读原失败及原字节均通过，不以 migration、自报错误或放宽旧身份门禁处理。

所有原始材料位于受限且 Git 忽略的本证据根，原配置／凭据／provider 原文不进入 Git。超出明确存储边界的 wire 仅保留 observed byte count／SHA256／not-saved 原因，不能据此宣称 bytes 已存。

## 5. 最终真实链与退出结论

`live-final/result.json`：PASS，真实 ai-proxy／deepseek-flash，六类各一份 PASS，最终 `ready_to_confirm`。

| 身份／结果 | 值 |
|---|---|
| Workspace | `ws-01bae6399aab591c074c722e3ce885f7ae0be9e589eda242dabeb10adabcb17f` |
| inputHash | `definition-authoring-sha256:0b06be7de01d21e63256a941dfb0e39645cb9d97e2487c51afbb3a158f43e9f4` |
| templateSnapshotHash | `template-snapshot-sha256:4c35a968e4683abb2243aa3272ed545761f2d58a5c91d49ae887c3d986a22b72` |
| proposalHash | `goal-specification-sha256:f77e62674f45c1a15df32d0b33604bf2b85686e336035aa07470da5715ca9ce6` |
| specificationHash | `goal-specification-admission-sha256:7d7fd81c8407163e2701b6fc68bbf6bb7359f28929f8df57d8a67c907b16d2a0` |
| 隔离验证配置 SHA256（仅身份，不公开内容） | `5f82b9dc23c8bd771f2812f32b095755d170932ae7c76cf13808ed9ce8734b81` |
| 真进程启动 PID | `1851687 → 1855400` |
| 结算后 owner record SHA256 | `08d2bcb3258ea90792b5eb247359226787267e221d0494dad245f6a110b116a0` |
| 调用及 usage | 8次：derive、proposal、六个独立critic；181386 provider-native tokens，非估算 |

源承诺在调用前写入 `live-final/fixed-source-commitments.json`。独立对照原 Goal，逐项核对正常参数、UTF-8、原顺序、逐行、不排序／去重／截断、无参数明确错误及非零状态；用户两项原要求、模板13项、派生5项均有唯一 exact obligation，合计20项、20份非空 DECLARED。独立 Goal 完整 required，未补造其 DoD obligation。`source-inventory-audit.json` 对真实 owner 原文完成完整 ledger Validate、原来源逐字比对、实际 Profile 最终合并适用性、可确认条件及只读前后原值不变反证；这不是用户确认或执行证据。

人工阅读六审理由、来源和声明：完整原 Goal 的行为约束由用户／派生义务承接，各模板义务原文保留，未以 Profile 绑定或历史行为替代实际运行验证。**traceability 的原 Reason 中把义务数写成22，是模型描述计数错误；canonical 实际为13＋2＋5＝20，引用与逐源映射完整，审计按真实20项通过。** 不改写原 Reason，不把该计数当权威，也不宣称模型文字绝无错误；用户仍须审阅后才能在下一阶段确认。

生产 Web 仅经正式入口 GET／只读刷新，显示精确输入、四组 requirements、20项 obligations／verification、六审及来源哈希；确认对话框关闭后重开，以及真正页面 reload 后从总览→Workspace→候选，均找回相同 inputHash／proposalHash 和6/6结果。原始完整 DOM：`web-ready.dom.txt`、`web-reopened.dom.txt`、`web-after-reload.dom.txt`；截图：`web-ready.png`。本轮 Web 没有点击生成／确认／执行，显式生成的生产 Web 业务动作承接上一轮同字节 console fixture 证据，当前真实链由正式 HTTP 显式生成。

最终验证器在六审结算后执行同身份 compile 重放、真实停机／新进程启动、完整 authoring view equality 比较及无确认／准入／Run断言，`restartReadbackPassed=true`；最终 usage仍8。源码／console／binary结尾与补充复核均一致。浏览器页关闭，所有本轮隔离服务进程正常停止，原始数据保留。

完整 proposal document **10298 bytes** 已保存并重新核验 SHA256 `579939a9e8c05e736eb9aa7c5e697546fb49f4d89fb23cf6e74272ea44ff453f`。wire完整观察到 **504497 bytes**，但超过原256KiB存储边界，`Saved=false / observation_storage_limit`，仅保留精确 byte count／SHA256 `4b14f864b1f9ca0568aa48ea5311540a0ffcaee5321ad703116a2b51776a4e45`。六份 typed review 原结论／Reason／refs／coverage/hash 已持久化；不能据此宣称所有 provider 原始 wire／critic 原始 JSON bytes 已单独保全。边界不完整性如实披露，不改存储预算凑绿。

附加只读审计脚本最初误把 owner `reviewed` 摘要当作管理 DTO 的 `ready_to_confirm`，又引入了不属于合同的 Clone nil/空集合 DeepEqual 假设，均是审计脚本断言错误；已按现有语义纠正，并以原输入完整字节前后比较完成通过，没有改产品状态／门禁或重新调用 provider。

本轮总共三次有明确代码修复依据的独立隔离 live 事务，物理调用3＋8＋8＝19次，provider-native token总数59818＋211618＋181386＝452822；上一轮原失败2次／25405 tokens另计。未清 ledger、重发失败 permit、改原 Goal／验收、换模型凑绿或放宽 owner 预算。相同固定 RequestID 在不同隔离数据根可得到相同 WorkspaceID，不能把不同事务的状态或费用混为一份记录。

最终状态：S2-A、S2-B 的相关累计与 S2-C 限定成功链均具备证据；本轮核对的代码阻塞项已收口。审查文字计数笔误与原存储边界作为明确的验收说明保留，不是隐藏的运行成功或无限推广保证。源快照见 `candidate-verified.patch` 和 `new-source-verified.tar.gz`；原配置／诊断不进入 Git。

未运行全库CI、全部 Runtime fullflow、确认或执行 live；这些不属于本轮 STEP02 退出结论。
