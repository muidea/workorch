# STEP04 SkillHub 严格只读观察与验证记录

> 更正（2026-10-22）：本文是原实现时点记录，不能再作为“真实 SkillHub 严格只读/零联网”的证明。
> 后续核对 v0.13.5 源码发现 status 可保存项目状态、CLI 可先访问服务端，且无 `--agent codex` 参数；
> global 数量和多 agent 身份合同也与旧 fixture 不同。原测试只证明 fixture 及 WorkOrch 处理边界，
> 未证明上述生产假设。用户已确认保留 SkillHub 当前行为；已按显式 status-effects 合同修订适配，
> 详见 `STEP04-SkillHub现行行为适配与验证记录.md`。下文保留为历史，不据此宣称该子能力完整通过。

日期：2026-10-21（Asia/Shanghai）。基线：`834aa350cc4e77379819666540b9912a7c22b64c`
加现有及本轮未提交修改；不是仅 HEAD 的验证，也不是最终提交的独立构建 attestation。

## 本轮收口边界

本轮完成 F03/A06 的 **SkillHub status 清单严格只读观察子能力**，不是全部 Skill readiness。
现有列表路径允许吞掉部分 SkillHub 错误、缺命令返回空集、跳过重复或空身份，并会尝试从来源路径
读取指令。新准备观察路径独立于该展示/执行选择路径；没有扩大修改现有执行选择行为。

- Application `ObservePreparationSkillHub` 在观察前后经现有 typed Workspace query 读取精确封存快照，
  绑定 Workspace/revision/snapshot hash 和已封存项目目录。起始 seal 无效时不启动子进程；结束漂移时
  保留已读诊断但不设置 `BindingCurrent`。这不是跨文件原子读取或 M3 final CAS。
- Application 自有进程适配器只执行 global Codex `status --global --json --agent codex` 和项目
  `status --json`。整体预算 5 秒，单次子进程 2 秒；不 retry、sync、use、install 或刷新。
  缺命令、失败、取消、缺失/非目录/悬空链接、项目路径不符均不能返回成功空清单。
- 严格检查完整 JSON、items 非 null、可选数量一致、最多 1024 项、非空唯一原身份、重复/大小写别名
  JSON key、尾随文档、最大嵌套深度、UTF-8 与未配对 UTF-16 surrogate；非法输入整体拒绝。
  未知字段保留在原始 bytes，不做摘要替代。有效 Unicode 转码同时保留原始 JSON 字节。
- 原 stdout/stderr 最多保存 1 MiB 原始前缀，同时记录已读取字节总数、保存部分 SHA256 与完整性。
  超限显式 `output_incomplete`，取消/管道未完整结束显式 `command_incomplete`；均不授 inventory 完整性。
  非零退出即使输出合法 JSON 仍为已知失败，保留原输出，不当作成功或无资源。
- `InventoryComplete` 只指一份完整无歧义的 status 文档；原 status/version/hash 原值保留，**不解释为
  指令文件内容、选择、freshness 或可用性已验证**。不尝试通过 source/repo fallback 补指令。
  `BindingCurrent` 只反映本次结束时 seal 未漂移，不声明资源 current。
- 诊断仅在受限内部结果返回，未加入 public GET、SSE、通用日志或持久准备账本。F05/F08 后续仍须
  正式接入检查结果与原始诊断保存；本轮没有声称观察可跨进程恢复。

遵循 `go-multi-module-dev` v2.6.0：没有新增运行单元/业务 topic，没有跨 owner service/repository 注入。
Workspace 交互沿用 concrete typed EventHub；外部 SkillHub 是 Application 已有职责的进程适配器。
SM-1/SM-2/SM-3/SM-4 均无迁移修改，不创建/驱动 Run，不授执行权或 M3。

## 源码与验证入口

- `internal/modules/application/workorchd/biz/activation_preparation_skillhub.go`
- `internal/modules/application/workorchd/biz/appservice/contextregistryports/skillhub_preparation{,_test}.go`
- `internal/modules/kernel/workspace/biz/activation_preparation_skillhub_test.go`
- `activation_preparation_query_test.go` 仅扩展现有测试 fixture 的封存前目录参数与指定读取漂移切点，
  原调用默认行为不变。

受限证据目录：`/tmp/workorch-step04-skillhub-zhRVcH`，临时目录不保证长期保留。不包含真实配置或凭据，
不将原始诊断提交 Git。测试仅用临时目录、自建 shell status fixture、缺失可执行文件和真实 owner fixture；
没有运行本机实际 SkillHub、读取其 home 清单或调用真实 provider/embedding。

最终验证记录（不是全仓 CI、生产 HTTP、浏览器或服务重启验收）：

最终三个受影响包完整回归、两个包定向 race、vet、fmt-check、diff-check 与 boundary gate
均通过。`source-files.sha256` 在最终运行后逐文件校验一致。boundary 日志保留既有
`__pycache__` binary-match 提示，不写成无警告运行。

| 检查 | 证据与范围 |
| --- | --- |
| 三个受影响包完整回归 | `affected-go-test-release.log`：contextregistryports、Application biz、Workspace biz，`-count=1 -timeout=5m` |
| 定向 race | `race-release.log`：contextregistryports 与 Workspace biz，`^TestPreparationSkillHub\|^TestExactConfirmedStartupQueryRejectsMixedRevisionObservation`，`-count=1 -timeout=2m` |
| vet | `vet-release.log`：上述三个受影响包 |
| 格式/边界 | `fmt-release.log`、`boundary-release.log`、`diff-check-release.log` |
| 精确 seal 接线 | 真实 file-backed Workspace owner → Application → 隔离 status 子进程；重复查询无 Workspace 修改，首次 stale seal 不探测，结束 revision 漂移保留诊断且阻塞 |
| 子进程与保真 | stdout/stderr 非零退出原 bytes、缺命令、取消真实 sleep 子进程、原文 hash/clone、显式超限，以及数量/身份/JSON/Unicode/目录反例 |

最初 sandbox 内累计尝试 `affected-go-test.log` 因既有 HTTP fixture 被禁止监听回环地址而失败；保留该失败。
后续只为隔离 fixture 在允许监听的环境执行最终累计；不修改鉴权、不连接现有服务。
中间 `*-final*` 日志不代替增加 Unicode 保真反例后的 `*-release*` 日志。

最终日志 SHA256：

- `affected-go-test-release.log`：`edcae82da99691c468ebe4ef30e3bd2897840bc86ebf6f4bef0c38200d26b699`
- `race-release.log`：`36587ba17d19f56fd79e0af23240b97f765b3ab5ca7788d3ab1cf46643893fe9`
- `boundary-release.log`：`c12faddca90ce91e33d37fdee8fe42fe15f4d07364791a9f770d03d1fa23521a`

`source-snapshot.json` 来自 `build_identity.py capture`，`source-files.sha256` 额外标识本轮 Go 源码及测试文件。
没有重建或启动二进制，没有 `produce`/二进制验收结果，不将旧二进制哈希移记到本轮源码。

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c`＋dirty tree
- buildInputSha256：`383f78ec5c989e9f277bcc60b9ea1b353dbc7a8cb4e74a254b624ebb371067c6`
- consoleTreeSha256：`dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c`

## 仍需收口

1. F03：目录权限/边界、托管配置、Soul、Skill 正式注册与精确选择/指令内容，以及 MCP/Memory 的
   完整性、版本、原模式、freshness 和具体阻塞。当前 status inventory 不能替代这些事实。
2. F05/F08：显式检查接纳、精确幂等、有界 attempts、私有 reducer/CAS、观察/原诊断持久化与原检查恢复。
3. F07：聚合全部 required 真实事实、完整 no-Run/epoch observation，在 final CAS 下签发唯一首次 M3，
   及两种排序、崩溃、unknown、retirement、迟到回执矩阵。此前 fresh reservation/结算接线不等于该项完成。
4. 增量三：HTTP 检查/恢复、Web 分轴展示与迟到 fence、生产构建浏览器和真实服务 stop/start 验收。
   完整 Start/Restart 准备回执消费仍属 STEP05，不在本轮实施。

继承 public fullflow 默认 Profile 断言、Orchestrator 套件累计超时、build-identity CLI verify 问题
未在本轮修复或重跑。**不能据此宣称完整 STEP04、M3、全 CI 或 live 准入已经通过。**
