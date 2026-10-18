# STEP02 P2 恢复修复与既有证据复核

日期：2026-10-18。评审基线：`1f9d708e3417d0f88ab9de72082d712a878dcbe6`。

结论：首次生成进入 `preparation_failed` 后恢复按钮持续禁用的问题已修复，定向与 Web 累计回归通过；原已验证候选与评审基线的源码构建输入、生产 Web 字节等价，关键原始证据仍在。本轮未默认重跑收费 live，也不将此前候选的 live 结果记作本轮修复已执行 live，更不宣称全 CI 或全部阶段通过。

## 1. P2 原因与修复

原触发顺序：打开已保存的 `draft` → 显式生成返回 `compilationAccepted=true` → 页面 `accepted=true` → GET 读到 `preparation_failed`。旧准备观察器只在生成前已经是 `preparation_failed` 时激活；首次失败既不产生 `freshLedger`，又被排除在清理 accepted 的状态之外，因此恢复按钮持续禁用，刷新也无法解除。

修复仅在 STEP02 候选页面记录本次受理是否源于旧准备失败：首次生成得到的新准备失败解除本次受理锁；从旧失败显式恢复仍观察新诊断／新状态，不将旧诊断或 15 秒截止当作再次受理授权。页面会话切换重置该内存标志，原精确输入／模板身份、单飞、防重复、会话栅栏和 owner 边界不变。

没有修改后端、provider 合同、Runtime SM-1～SM-4、预算或准入；没有新增浏览器持久存储。

## 2. 本轮最小复验

| 检查 | 结果 |
|---|---|
| `npm run test:components -- tests/components/authoringCandidate.test.tsx` | 12/12 PASS（新增 4 例）；先以新增用例复现首次失败禁用问题，再修复 |
| 新增定向覆盖 | 首次 202 后轮询读到失败；compile 直接返回准备失败（accepted=false/true）；显式恢复的精确身份及双击单飞；旧诊断保持锁、新诊断解除锁；GET 失败时独立截止、旧诊断刷新不授再次受理；均断言无 create／save／confirm／execution 副作用 |
| `npm run test:components` | 10 files／113 tests PASS |
| `npm test` | 11 test files PASS |
| `make verify-boundary-contract` | PASS |
| `make build` | 退出 0，生产 console 和 Go binary 构建完成 |
| 构建前快照 → 独立 `go build` → `build_identity.py produce` | PASS，核验前后源码、生产 console 字节不变并记录实际 binary SHA256 |
| `git diff --check` | PASS |

`useForm` 与 Vite 大 chunk 提示仍存在；Go 输出 module stat cache 只读提示，但两个构建命令均退出 0。它们没有被记作全 CI 通过。评审所述父提交同样存在的 CI 失败不在本次 P2 修复范围，本轮未复跑或修复全库 CI。

本轮修复后的未提交候选身份（不是 `1f9d708` 原提交的构建身份）：

| 字段 | 值 |
|---|---|
| sourceCommit（构建时 HEAD） | `1f9d708e3417d0f88ab9de72082d712a878dcbe6` |
| buildInputSha256 | `75a04f6c757ebc5e87d6c18654ecbc7f50361ad35c5d60e514da79ad274370eb` |
| consoleTreeSha256 | `0470236fa1f91ee4898e54e7d01cba7efabfa3a2851014d401872ff8b4327bc8` |
| binarySha256 | `ef437cbf25e6df02c0a8bc801457c7245ee8b40b6cf3ebfbbc25c256cde983ba` |

本轮日志、构建身份及修复源码 patch／新增 console 资产快照的字节数和 SHA256 见 [P2 验证索引](P2-recovery-validation-index.json)。这些检查不替代新二进制的 HTTP／重启／生产浏览器 live；本轮没有执行该类新验收或物理 provider 调用。

## 3. 原候选与提交等价性

修复前、工作区干净时，`build_identity.py capture` 对 `1f9d708` 的现场计算已与原成功候选一致。为避免依赖随后变化的工作区，又从固定提交导出源码快照，对 5268 个归档文件逐一核对 Git blob 与 executable mode，并独立重算构建身份：

- 5145 个构建输入文件：`68dd2fb9bc3c78ba0a660aa64843db19680fca58b2c7b5382f56bac1bcad9bfa`。
- 16 个嵌入 console 文件：`7930b2f77aeb24443c6b2cedc1ed63108cf02b15e93bd5257f76e5844d6fdbdc`。
- 原 `candidate-verified.patch` 的 46 个路径与父提交 → `1f9d708` 相同路径 diff 逐字节一致；原 `new-source-verified.tar.gz` 的 8 个新增源码逐文件匹配提交。
- fixture-final 与 live-final 的原 build identity 完全一致；原 binary 重新计算为 `d15e95f5ac693e70a76e52d7cbbbda1e84738274795407757e0ea6f3253e716c`。

原清单的 sourceCommit 仍是 `0df7498ae372ece1be9c63fb999c2b068889e31a`，当时验证的是它上面的未提交候选；本次没有改写清单，而是证明候选的构建输入和 console 与后来的 `1f9d708` 等价。等价范围按既有 build-identity allowlist，不声称配置、工具链、环境或整个仓库历史全部相同；归档报告、`.gitignore` 及既有计划等不因此变成运行验收输入。

固定基线源码快照（含 vendor、Web、scripts 和构建入口；不含 live 配置／运行数据）：

`.validation-tmp-step02-p2-V9zirl/1f9d708-source.tar.gz`

只读复核脚本：`.validation-tmp-step02-p2-V9zirl/audit_existing_evidence.py`；仓库根执行 `python3 .validation-tmp-step02-p2-V9zirl/audit_existing_evidence.py` 可重新核验并输出元数据，不启动服务或调用 provider。脚本及快照的 SHA256 已列入原证据索引。

## 4. 原始证据索引与保留范围

[既有证据索引](1f9d708-existing-evidence-index.json) 列出 45 个关键证据文件的原路径、bytes、SHA256，包括两轮报告、源码 patch／tar、成功与失败的 result、构建链、HTTP trace、原生产 Web DOM／截图、实际 owner、usage 事件及累计检查日志。路径相对仓库根，原文仍在受限且 Git 忽略的原证据根；索引不是把原始数据移动或提交 Git。

本次只读核验还确认：

- 原 live-final 六审均 PASS，原 candidate-result 与 restarted-candidate 解析后的完整值相同，原 result 标记 restartReadbackPassed；owner record SHA256 仍为 `08d2bcb3258ea90792b5eb247359226787267e221d0494dad245f6a110b116a0`。
- 原已保存 proposal document 实际 base64 解码后重算：10298 bytes，SHA256 `579939a9e8c05e736eb9aa7c5e697546fb49f4d89fb23cf6e74272ea44ff453f`，与原 SuccessAudit 一致。
- 原 wire 504497 bytes 超存储边界，仍为 `Saved=false / observation_storage_limit`。没有可重算的完整 wire bytes；索引明确 `rehashed=false`，不声称该原文已保存。六审 typed 内容存在不等于所有原始 wire 都已保存。
- 原 metadata 的 8 次调用／181386 provider-native tokens 与原报告一致；本轮只读复核不新增费用。没有将 metadata 比对冒称新的独立计费审计。

原报告中的 traceability 计数笔误、原始 wire 保留边界和限定输入／模板范围继续有效；历史报告原样保留。P2 修复与既有证据核对完成，全部验收退出仍应结合本次评审复核，不以此前“全部收口”的概括取代具体通过范围。
