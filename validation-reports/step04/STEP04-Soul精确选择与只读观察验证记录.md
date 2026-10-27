# STEP04 Soul 精确选择与只读观察验证记录

日期：2026-10-21（Asia/Shanghai）。基线为 `834aa350cc4e77379819666540b9912a7c22b64c`
加现有及本轮未提交修改，不是仅 HEAD 的验证。

## 本轮完成范围

完成 F03 的 Soul 路由配置绑定、真实所选记录读取、版本/内容完整性检查与有界重读子能力。
未改既有执行路由、未调用完整 activation builder，未将该观察接入持久准备检查或 M3。

- `ObservePreparationSouls` 保留父 Workspace scope，设置整体 3 秒 context 预算，复用既有
  Workspace exact query、Workspace get 和 Soul get concrete typed EventHub command/result。
  未新增 topic、运行单元或跨 owner service/repository 依赖。
- 必须有 Application 显式注入的配置 loader；缺失或读取失败不被解释为“服务默认 Soul 为空”，
  不回退机器上的全局配置。只保留本次相关的 `Runtime.DefaultSoulID`，不传递其他配置或凭据。
- 从真实 Workspace typed 读取核验同 scope、revision 及完整 `WorkspaceConfigurationHash` 与
  原确认配置 hash 相等，再按现有阶段绑定 → Workspace 默认 → 服务默认优先级产生 typed 选择。
  没有 Run override，未知/空阶段绑定不静默丢弃，六个阶段与 unbound 均明确保留。
- 保持现有默认读取语义：有阶段绑定且没有显式 Workspace 默认时，不额外强制读取一个完全未使用的
  服务默认；有实际默认/阶段引用才读取对应 owner 记录，不要求安装未选资源。
- 每个唯一所选 Soul 读取一次，接收端检查 exact ID、非零 revision、来源、内容 hash、UTF-8 及
  owner 字节/列表边界，保留 owner 返回的完整内容和字段，不改写成摘要。深拷贝保留 nil/空列表形状。
- 对全部所选记录再真实读取，逐字段比较完整 detached 原值，而不只比较 hash；随后再核对服务默认及
  同 revision 的原 exact seal。缺失、错误、非法或漂移均阻塞；已观察到的原记录仍在失败结果中保留，
  不设置 `Reobserved`/`SealBindingCurrent`，不 retry 或安装默认。
- 显式配置且确实无所选 Soul 时，返回六个 unbound 阶段的真实事实，不虚构记录，也不自动授予 PASS/N/A。
  是否适用必须由后续 required 检查结合原 Profile 判定。

`Reobserved`/`SealBindingCurrent` 只描述本次有界读取，不是跨 owner 原子快照、长期 freshness、
资源授权、Skill/MCP 可用性、准备 PASS、M3 或执行许可。结果尚未进入 public GET、SSE、通用日志
或持久检查账本；F05/F08 后续仍需接入正式保存和恢复，不能声称观察本身已跨进程恢复。

遵循 `go-multi-module-dev` v2.6.0，Application 只组合资源事实，不写 sibling owner。
SM-1/SM-2/SM-3/SM-4 均无迁移修改；没有 Run、Runtime 初始化/驱动、provider/Memory 调用，
没有收费验证、live 配置读取或现有服务操作。

## 验证与证据

源码入口：

- `internal/modules/application/workorchd/biz/activation_preparation_souls{,_test}.go`
- `internal/modules/kernel/workspace/biz/activation_preparation_souls_test.go`
- 既有 `activation_preparation_query_test.go` 仅扩展封存前配置测试 fixture，默认调用行为不变。
- Application `readme.md` 已同步已落地边界，不将计划当作设计权威。

受限临时目录：`/tmp/workorch-step04-souls-jtMyAv`。仅索引本机原证据，不保证临时目录长期保留，
不提交原始诊断、配置或凭据。验证使用隔离 file-backed Workspace/Soul owners、非敏感 fixture 及
注入的测试配置；不是本机真实 Soul 库或真实 provider。累计已有 HTTP 测试仅使用隔离回环 fixture。

| 验证 | 内容/证据 |
| --- | --- |
| 定向验证 | `targeted.log`：本轮新增 Soul 观察测试；真实 owner、file repository 重开、持久文件逐字节不变、Workspace 不变、默认/阶段优先级及 unbound |
| 累计包回归 | `affected-go-test.log`：Application biz、Workspace biz、Soul 全子包、activationports、ownerreadports，`-count=1 -timeout=5m` |
| 定向 race | `race.log`：Application biz 与 Workspace biz，`^TestPreparationSoul\|^TestPreparationSkillHub`，覆盖本轮及相邻准备观察，不是全包全套 race |
| 静态/边界 | `vet.log`、`fmt.log`、`boundary.log`、`diff-check.log` |
| 路由等价 | 与既有 `ResolveWorkspaceSoulRouting` 无 Run 输入的结果比较：Workspace/服务默认、部分/全部阶段覆盖、无服务默认及全部 unbound |
| 精确失败 | 无 loader/配置失败、缺 Soul、typed 错误/nil/错 ID/错 hash/零 revision/未知来源、scope/cancel、服务默认漂移、结束 seal 漂移；无自动重试 |
| 全记录反证 | 重读时内容/revision 漂移，以及 allowlist 字段移动不能只靠 hash 授信；保留首次独立观察，无成功标记 |
| 传输边界 | 内容/列表/UTF-8 边界、clone 与空列表形状；未将列表声明当作其他资源已可用 |

开发中的初次测试曾因 file repository 构造参数实际为根目录、以及 fake EventHub 错误类型不符失败；
修正测试后运行上述最终定向与累计验证，不将这些中间尝试称为验收通过。
文件 owner 的重开不是独立服务进程 stop/start，更不是生产浏览器验证。

源码身份使用 `build_identity.py capture`，另用 `source-files.sha256` 标识本轮源码及测试。
没有构建/启动新二进制，不将此前产物或构建哈希记到本轮源码：

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c`＋dirty tree
- buildInputSha256：`6ebf3c8a5c5489bae475fae896bfe29ccb0ff6359d73e6ea5e76b4d10cc13ff9`
- consoleTreeSha256：`dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c`

## 最终执行结果

7 个有测试包累计回归通过，2 个包无测试；2 个包定向 race 通过。
vet、fmt-check、diff-check 和 boundary gate 通过，最终源码/测试逐文件 SHA256 复核一致。
boundary 日志保留已有 `__pycache__` binary-match 提示，未写成无警告运行。

日志 SHA256：

- `affected-go-test.log`：`b82dd3caf732cb40c0654eb9f6429046c66b4160918a81a208af05770e6cd353`
- `race.log`：`0ef453c73904af698c019855550a655ef9c45e9e87720e46b00d6f4d83a2677f`
- `boundary.log`：`66d7a134aeee457b32d9198dc0e20293273b1fa27de8bfb7589201b469a79160`

## 下一轮剩余门槛

1. F03：目录/权限与托管配置；Skill 正式库完整性、SkillHub 指令内容与完整目录上的精确选择；
   MCP/Memory 的原模式、版本、完整性、freshness 及准确阻塞。本轮 Soul 事实不能替代这些 owner 事实。
2. F05/F08：显式检查受理、精确幂等、有界 attempts、private reducer/CAS、原始诊断/观察持久化与原检查恢复。
3. F07：汇总全部 required 事实，在完整 no-Run/epoch 观察和 final CAS 下签发唯一首次 M3，补完整顺序、
   unknown、retirement、崩溃/迟到矩阵。此前 fresh intent reservation/结算不等于 M3 完成。
4. 增量三：HTTP 检查/恢复、Web 分轴及迟到 fence、生产构建 HTTP/浏览器和真实服务重启验收。
   完整 Start/Restart 准备回执消费属 STEP05，不在本轮。

继承 public fullflow 默认 Profile 断言、Orchestrator 累计套件超时及 build-identity CLI verify 问题
没有在本轮修复/重跑。不能据此宣称全 CI、完整 STEP04、M3 或 live 准入通过。
