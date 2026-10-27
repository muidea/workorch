# STEP04 生产浏览器阻塞主链与组合重启验证记录

日期：2026-10-27（Asia/Shanghai）。HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，加累积未提交修改。

## 本轮结论与范围

关闭此前尚缺的**生产 Chrome 阻塞主链验收**：实际打开准备面、只读刷新、显式检查、读取完整逐项结果、刷新整页后从服务端恢复同一操作。
随后同一批次完成真实 HTTP 原请求重放、丢弃响应体后对账、人工恢复资源后新检，以及两次独立 OS 进程重启。
这不是准备成功分支、完整 STEP04 或 M3 通过；Provider 仍排除在本轮实现范围之外。

本轮仅修改验收脚本、共用进程 helper 及定向测试，没有修改生产 Go/Web、Provider 实现、鉴权或 Runtime SM-1～SM-4。
按 go-multi-module-dev 保持现有 owner / typed EventHub 边界；没有新增运行单元或跨 owner 信任入口。

## 代码收口

- STEP01 共用进程 helper 增加可选的显式回环端口，严格校验端口并在启动前 bind 检查可用性；不替换已有监听服务。默认随机端口行为不变。
- STEP04 verifier 增加有界浏览器窗口及原操作接续：浏览器必须真实产生唯一 attempt，核对原 revision，之后采用其 requestId 继续重放。marker 只结束窗口，不能证明浏览器验收。
- 读取有效性按**原 owner 归档的精确缓存期限**核对：到期只能投影 stale，完整历史保持不变；重启后的明确 cache_miss 保持 not_evaluated/blocked，不伪造期限或触发刷新。未绑定、重复或丢失的归档拒绝。
- 明确区分整页既有 Skill 库存 status 与 STEP04 专用 status 对。逐项核对完整 argv、cwd、顺序与原日志前缀，恰好一次准备专用对；不删除或改写原调用日志。取证结束离开测试页，后续进程-only 重放与重启仍严格要求 status bytes 不变。
- 定向测试覆盖窗口边界、非法端口、归档到期/冷 miss/歧义、重复准备 effect、未知 status 命令、清单及汇总造假、原身份/历史/来源替换等。

## 实际 Chrome 与进程结果

最终浏览器地址为 `http://127.0.0.1:56147/console/`，使用已连接 Chrome 的原测试标签页；普通刷新进入本轮隔离服务，未修改或绕过本机鉴权。
浏览器原 requestId：`74e5ba75-1646-4af4-a025-9013091f2aad`，原 revision 22；结算后 revision 24。

1. 开准备面及只读刷新均为 not_checked；两份原始 DOM bytes 的 SHA256 相同。
2. 仅点击一次“显式检查准备”。实际展示 blocked / not_evaluated、原检查通过 10/11、4 个来源与 3 个义务的纯编译摘要，明确非完整 bootstrap、非 ready/M3。
3. “显式检查准备”可用，“显式恢复原准备操作”禁用；未点击“创建新执行”。
4. 已结算后的只读刷新保留相同完整 DOM bytes；关闭后整页 reload、重新进入 Workspace、重开准备面，仍显示同一 requestId、10/11 与全部原历史引用。
5. 保存原始 DOM/JPEG 后将测试标签页导航到 about:blank，停止整页自动重连对进程-only 检查的干扰。未关闭用户标签页。

随后脚本自动核验：

| 操作 | required/passed | 结果 |
| --- | --- | --- |
| 浏览器原检查 | 11/10 | provider_cache 阻塞 |
| 移走隔离配置后显式新检 | 11/9 | managed_configuration、provider_cache 阻塞 |
| 人工恢复配置后显式新检 | 11/10 | provider_cache 阻塞 |

原 request 的 GET/check/recover 不追加 attempt；同 ID 异 revision 为 409。
最后一次 POST 只读取 status/headers，丢弃 body 后 GET 对账；三份原历史在第二次独立重启后均可按原身份重放。
生产 PID 为 **3755021、3756008、3756063**，最终全部停止。
每次启动均核对构建身份与实际运行 executable bytes；结束再次核对源码、Web 与二进制内容身份。

STEP04 阶段 Provider HTTP（包括 models GET）0、embedding 0、Run 0，业务文件 bytes 不变。
前置候选准备有 8 次**本地模型 fixture**响应，不是远端调用；SkillHub 使用空集合 status-only fixture，不代表实际非空资源 readiness。
浏览器整页库存另有 4 对既有 status，完整保留并分开计数；不能声称整页导航没有任何 owner I/O。
没有真实模型配置、真实网关、收费调用、MCP 启动或 STEP05。

## 自动验证与失败保留

- `make verify-build-identity-tests` 通过：构建身份 17 项、STEP01 helper 5 项、STEP04 verifier 12 项，共 34 项。
- `GOCACHE=/tmp/workorch-go-cache make verify-boundary-contract` 通过；`git diff --check` 通过。
- 最终生产 Web/Go 构建通过；保留 Vite 既有大 chunk 告警。未执行全仓 Go/全 CI，不称发布验收通过。
- 非浏览器批次 `step04-browser-20261027-e` 在最终 verifier 下独立通过全部 HTTP/两次重启；最终组合批次 `step04-browser-20261027-f` 也通过。
- 开发批次 a 的组合验收因旧 verifier 固定要求 not_evaluated、未区分真实到期 stale 而失败；b 在浏览器操作前主动取消；c 因页面整页重连增加库存 status 导致严格计数失败；d 在第二次新检处暴露 verifier 未处理冷 cache_miss 的零期限。失败目录保留，不改成通过；相关 verifier 修正、反例测试与最终 e/f 验证分别记录。
- a 中实际 stale 展示仅作为额外现象证据，不替代最终源码整批验收；未因此修改 Provider 门禁或增加外发。

## 内容身份与受限证据

最终证据根（Git 忽略、目录 0700）：
`/home/fedquery/codespace/agentSuite/workorch/.workorch-data/step04-browser-20261027-f`。

| 字段 | 值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c`，含构建时未提交输入 |
| buildInputSha256 | `6cc38467f331f837d80c6ba4d4c8720fb1913a94c4009a76c88cebc034f80ca0` |
| consoleTreeSha256 | `7d6f3b3d9d7a583b2b457775ba9c8b86105e34e575a9719f4d757bbfaf431f33` |
| binarySha256 | `3191f13430a32423d6c5ffa4e288e0847c254cd2edd89ec8e2517f1fdf42ba46` |

这只是本机内容绑定，不是可信构建 attestation 或最终 Git 提交的独立实测。

| 原证据 | 用途 |
| --- | --- |
| browser-before.txt、browser-before-refreshed.txt | 未检查态与只读刷新，SHA256 均为 `dff41f0e74b436fd63664340a28067747054bc0d78c619c1e86b12d5821434a9` |
| browser-blocked.txt、browser-refreshed.txt | 原检查完整展示与只读刷新，SHA256 均为 `4ef8f32d98b481b7883ae2b58773acb00d79c9931cfc6841a4fe51d372b56f5d` |
| browser-reloaded.txt、browser-blocked.jpg、browser-reloaded.jpg | 整页刷新后的原身份、完整 DOM 与实际截图；最终截图 SHA256 `17787fda5042c7641de63285626fbdfc2c66655f6cf80335003efb2947c879af` |
| browser-session.json、browser-done、result.json | 窗口身份、操作交接及自动 verifier 结果；marker 不是浏览器证明 |
| http-trace.json、http-*.bin、lost-response-*.json/bin | 原始 HTTP bytes/状态；主动丢弃的响应体不称完整保存 |
| original/missing-config/manually-restored-preparation.json、data/、*-owner-record.bin | 三次操作公共投影、真实 owner 归档与原持久 bytes |
| provider-http-paths.json、provider-*.bin、skillhub-calls.jsonl | 完整 fixture 请求/响应和精确 status 序列 |
| process-*.log、build-identity.json、pre-go-build.json、build-commands.json、workorch | 独立进程、构建与运行产物 |
| verifier-tests.log、boundary.log | 最终自动门禁日志 |

`result.json` 中 `browserVerified=false` 是脚本不根据 marker 自认证的刻意边界；本次浏览器结论来自上述实际 Chrome DOM/截图及可见结果，不把该字段改成 true。
原始诊断、隔离配置、日志、截图和二进制不提交 Git；本记录只保存索引。

## 仍未完成的主体项及下一步

本记录关闭此前报告的“生产浏览器阻塞主链”缺口，不表示除 Provider 外全部完成。

1. **完整 PreparedBootstrap 生产输入与归档接线**：已存在的纯编译函数未接到完整持久结算。模型选择声明不等于授权，不能据此笼统认为无法结构编译；另外仍缺执行策略、dialog language、运行 repair/recovery limits 等非 Provider 输入的历史归档，不能用当前配置反推旧 attempt。下一步应先补齐这些真实输入的单次绑定和恢复，再接既有纯编译器，不写占位选择或复制 Runtime 编译权威。
2. **owner 当前有效性合取与唯一历史 M3 成功结算**：原时点 PASS/双读不等于持续有效性证明；F07 final no-Run fence/CAS 与历史 M3 的成功路径尚未完成。先完成代码及确定性反例，在真实 required 事实齐备后再验证成功分支；Provider 继续排除时不构造成功事实。这些未实现代码不能都归类为 Provider 遗留。

本轮未提交 Git，保留所有既有累积修改。
