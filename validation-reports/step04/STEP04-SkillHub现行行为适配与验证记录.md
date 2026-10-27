# STEP04 SkillHub 现行行为适配与验证记录

日期：2026-10-22（Asia/Shanghai）。源码基线：`834aa350cc4e77379819666540b9912a7c22b64c`
加现有及本轮未提交修改；本记录不是仅 HEAD 的验证或完整 STEP04 通过声明。

## 用户确认与边界修订

用户原文：“当前skill-hub的的执行符合预期，按照当前skill-hub的运行效果进行对应内容收口”。

本轮不改造 SkillHub，撤销前轮工作区中“生产适配器因非严格只读而一律不可用”的临时处理。
现行 `skillhub-status-effects/v1` 明确允许 SkillHub status 保存项目检查状态、优先访问自身配置服务端。
这是 STEP04 显式检查的窄例外，不是 GET、刷新、导航、重启读取、provider catalog 刷新、收费推理、
embedding、MCP 启动或任何 Run/runtime 调用的许可，也不允许追加 init/install/sync/use/apply/repair。
SkillHub 原状态保存不被描述成 WorkOrch owner 的幂等事务或跨 owner 原子提交；终止 CLI 不证明服务端
未执行/未保存，未知效果不自动重发。持久检查与原操作恢复仍须后续 F05/F08 接入。

前轮《STEP04-SkillHub严格只读观察与验证记录》的生产只读假设已在原文件顶部明确更正，历史保留。
计划/合同审阅稿顶部也增加本次批准修订；现行规范入口为 `docs/10-domain-model/instruction-assets.md`，
Application 交互说明为 `internal/modules/application/workorchd/readme.md`。

## 本轮落地

1. 准备专用生产 runner 恢复执行实际支持的 `status --json`／`status --json --global`，
   不发送不存在的 `--agent codex`。每次命令 2 秒、文件观察子过程 3 秒、Application 总计 5 秒，
   无自动重试；命令退出后有界排空。结果携带 v2 observation、effect contract、真实启动事实及可确认的退出码。
2. 项目原 `scope=project` 不静默改成 workspace；WorkOrch 自身来源域单独保留。global 的
   `skill_count` 按不同 skill_id 数校验，条目按 skill_id＋agent 唯一；保留全 agent、orphaned、
   generated_at、原 reason/legacy_status/status/version/hash。合法多 agent 条目不当作重复，也不静默丢弃。
3. stdout/stderr 各保存最多 1 MiB 原 bytes、实际已读字节数、所存前缀 SHA256 和完整性，
   成功 stdout 不被 stderr 警告污染；非法 UTF-8 诊断仍保留原 bytes。超限、未完整结束和失败都不授清单
   credit；诊断不完整不能抹掉已知非零退出结果。未知 JSON 字段仍保留于原 stdout。
4. Application 已接入已安装指令观察：项目 `.agents/skills`，global 按 SkillHub 的 Codex 路径规则
   独立解析安装根；global 仅读取 Codex 条目的指令，其他 agent 计数与 orphaned 原清单保留。
   仅接受与安装根精确绑定的 LocalPath/TargetPath，不使用 SourcePath/RepoPath fallback。
5. `os.Root` 限制路径逃逸，逐段拒链接/非目录父项/非普通文件；Linux 只读非阻塞打开防 FIFO 等待。
   每文件最多 64 KiB、首轮累计最多 8 MiB，额外文件在读取前拒绝。原 bytes/大小/SHA256/完整性保留，
   逐条真实重读完整内容，并核对打开前后文件身份、大小、模式、mtime 及安装根 pathname 身份。
   失败重读另保留至多一份 64 KiB 原值。非 Linux 没有降低防护的 fallback。
6. 前后通过既有 concrete typed Workspace query 核对同 revision 精确 seal。
   起始 stale 不执行 SkillHub，结束 drift 保留已取事实但不授 BindingCurrent。
   普通持久状态 query 不调用 status；检查既不改 Workspace 正式状态，也不创建 Run。

`InventoryComplete` 仅指完整无歧义 status 文档；`InstructionsReobserved` 仅指本机正式指令两次完整读取一致；
`BindingCurrent` 仅指此次结束时 seal 绑定未漂移。三者均不证明长期 freshness、目录其他文件、metadata
语义、服务端观察与本机内容一致、精确选择、资源可用、准备 PASS 或 M3。真实 modified/conflict 不改写为就绪。
诊断观察上限不放宽 activation 的最多 5 项、单项 8 KiB／总计 24 KiB 和 metadata 16 KiB 封存预算。

按 go-multi-module-dev v2.6.0 保持 Application-private external adapter，不新增运行单元或业务 topic，
不直接访问其他 WorkOrch owner 的 repository/service。SM-1/SM-2/SM-3/SM-4 无迁移修改，不授执行权。

## 实际验证与原始证据

受限证据根：`/tmp/workorch-step04-skillhub-current-QUbbtb`。临时目录不保证长期保留；
原诊断不提交 Git。初始 `affected-go-test.log` 保留 sandbox 禁止临时 HTTP fixture 监听导致的失败；
允许隔离回环 fixture 后执行成功，未通过修改鉴权、跳过断言或连接现有服务解决。

| 验证 | 最终证据与范围 |
| --- | --- |
| 4 个受影响包完整回归 | `affected-go-test-release.log`：contextregistryports、contextregistrycore、Application biz、Workspace biz，`-count=1 -timeout=5m` |
| SkillHub 定向 | `targeted-release.log`：23 个顶层测试函数通过，包含 2 个显式 opt-in 的真实 CLI 测试；其余为确定性 fixture/文件边界 |
| 真实 owner 接线 | `owner-targeted-final.log`：两个测试；真实 file-backed Workspace owner＋生产 runner＋隔离 CLI fixture，覆盖纯状态 query 零 status、同 seal、stale、结束漂移和缺命令 |
| race | `race-release.log`：contextregistryports、Workspace biz 的 `^TestPreparationSkillHub`；普通 race 不设置真实二进制 opt-in，不把这两项说成 real CLI race |
| 静态/格式/边界 | `vet-release.log`、`boundary-fmt-release.log`；边界日志有既有 `__pycache__` binary-match 提示，最终 gate 通过 |
| 源码 | `source-files-release.sha256` 逐文件核对；`source-snapshot-release.json` 为 build_identity capture，不是可信构建 attestation |

实际 SkillHub 二进制：`/home/fedquery/.local/bin/skill-hub`，v0.13.5；`go version -m`
记录 vcs revision `625b2477b7d456136ec0cdd2bf0109cb3bc9b8e8`、modified=false，
与只读核对的 SkillHub 源码 HEAD 一致。完整模块信息在 `skillhub-build-info.txt`；
binary SHA256：`fcaec9f0697ec37d2277c924772e6133fc7fe579da078c09dbfbeea72130f40d`。

真实二进制验证不是仅 shell 模拟：

- **本地项目路径**：仅用临时 SKILL_HUB_HOME/配置、注册状态、来源与安装文件；显式选择 SkillHub 自有
  local 模式隔离真实 daemon。确认原 `project` scope、真实状态保存与版本更新，来源/安装指令原 bytes 未变。
- **本地 global 路径**：临时 enabled_skills 声明 1 项技能、2 个 agent；真实 CLI 返回 skill_count=1、items=2。
  未调用 apply/use：Codex 未托管副本真实返回 conflict/target_unmanaged；检查保留该事实，不宣称 readiness。
- **服务优先路径**：真实 CLI 连接临时 loopback HTTP fixture，恰好一次 health 和一次 project-status，
  无其他 endpoint；隔离本地配置缺失，成功结果没有来自 local fallback。HTTP 响应是 fixture，不冒充真实
  SkillHub 服务端持久化/网络部署验收。生产 adapter 不强制关闭用户原 service bridge。

对应原始材料为 `project.stdout.bin`、`global.stdout.bin`、`service.stdout.bin`，以及各自同名前缀的
`.stderr.bin` 和 `.observation.json`；
JSON 内的原始文件/输出 bytes 可逆保留。它们来自固定非敏感临时 fixture，未读取或修改现有项目、
真实 SkillHub home 配置/管理状态、凭据、线上服务或收费模型。未修改 SkillHub 仓库。

源码快照（本轮 dirty tree，不是仅 HEAD）：

- sourceCommit：`834aa350cc4e77379819666540b9912a7c22b64c`
- 测试时 `source-snapshot-release.json` 的 buildInputSha256：`d9504a4a8c2ac1d324e7c5d55ca646470ee58bcc22320315a81502c415a92c2b`
- 报告整理后 `source-snapshot-after-report.json` 的 buildInputSha256：`39ff757028d0f5afdae5bf5a3f011a07f747f77756d4f6c4d9c29bf00063590c`
- consoleTreeSha256：`dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c`
- 本轮未重新构建/启动 WorkOrch 生产二进制，不移用旧 binary 验收结论；上述 SkillHub binary 是外部 CLI，二者不混用。

两次 capture 不相同：测试后补充了 Application readme 中的诊断预算说明，内部 readme 也属于该脚本
构建输入树。该差异如实保留，不把两份 capture 说成逐字节等价；本轮 Go 实现和测试文件随后经
`source-files-release.sha256` 逐项复核未变，没有因报告整理修改 Go 源码或重跑收费验证。

主要日志 SHA256：

- `affected-go-test-release.log`：`3b08d72476c9008a496f1ada7a8dc994a728fd1b31cddbe748bec390ae7dbfbf`
- `targeted-release.log`：`2f6ffe5d2865ea9d0d576810067e469777b5c1147fe5f3a6f4c624491f2faab9`
- `race-release.log`：`b88cbb2d7ccf58e4620589e7778d84010d4a298f221f637217a1334480018696`
- `boundary-fmt-release.log`：`dcc5fc79063c3d97393bcf955487a17646af40c65793eff0d2d0531c6ce933ec`

## 仍需收口

1. F03 的正式 Registry＋SkillHub 完整目录精确选择、metadata/附属文件与 freshness／服务端来源绑定，
   以及目录权限/托管配置、MCP、Memory；本轮只完成现行 status-effects 适配和本机安装指令观察。
   旧展示/执行选择路径没有因本轮自动改成新准备路径，不将其 fallback/吞错/旧命令参数当作本步证明。
2. F05/F08 显式接纳、幂等、private reducer/CAS、有界 attempts、受限诊断持久化和原操作恢复。
   status 的既有保存副作用必须如实计入未知结果/恢复，不恢复“超时即没发生”的假设。
3. F07 全部 required 实际事实＋no-Run/epoch/final CAS 的唯一首次 M3 及完整竞争/崩溃矩阵。
4. 增量三 HTTP/Web 检查与恢复、迟到 fence、生产构建/浏览器/真实服务 stop-start 验收。
   完整 Start/Restart 回执消费仍为 STEP05，不在本轮实现。

继承 public fullflow 默认 Profile 断言、Orchestrator 累计超时、build_identity CLI verify 问题未在本轮修复或重跑。
不宣称全 CI、全部 STEP04、M3 或 live 准入已通过；本轮没有默认提交或 push。
