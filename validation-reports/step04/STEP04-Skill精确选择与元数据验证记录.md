# STEP04 Skill 精确选择与元数据验证记录

日期：2026-10-22。范围：F03 的 Skill 来源／元数据／选择观察子用例；不是完整 F03、STEP04、M3 或 live 退出报告。

## 1. 本轮交付

- 新增 `ObservePreparationSkillSelection`：真实读取同 revision 的 Workspace 封存快照，组合 Soul、Registry 和两个 SkillHub scope；采用封存 Goal／DoD 原文和 execution 阶段 Soul 的 allowlist，而非 Workspace 默认 Soul 的限制替代品。
- Registry 继续走 Skill owner 的 concrete bounded typed query；Soul 继续走自身 owner。SkillHub 是 Application-private adapter。未新增 owner、topic、跨 owner repository/service 或运行单元。
- SkillHub 每个 scope 显式执行一次现有 status，保留用户已接受的状态保存和服务端优先行为；后续实际安装文件读取不使用 source/repo fallback，不自动 retry、install、sync、use、apply 或 repair。整个选择观察最多五秒，继承父 scope／deadline。
- 准备专用 YAML AST 解析替代本子用例中的宽松列表解析：检查完整头、单文档、重复键、别名冲突、大小写变体、错误需求类型、深度／节点／字段上限；不支持 alias、anchor、merge、自定义 tag 时明确失败。原始文件 bytes 不变；错误仍保留完整已观察来源及首份诊断。
- 保留 trigger、reference、tool、MCP、capability、constraint 的 typed metadata。序列内带逗号的单项不再被拆成多个需求；兼容既有逗号 scalar-list。版本优先取实际安装 frontmatter，数字版本保持标量原拼写，不以 repo version 填补安装版本。
- 统一 Registry／global Codex／Workspace 候选，复用原 Goal/DoD 自动匹配评分、Workspace 优先、ID 排序和最多五项策略。typed matches 包含来源、分数、理由，不把旧展示 `map[string]any` evidence 用作准备依据。重复候选 ID、错误 scope/source、坏 allowlist 显式失败；完整选中内容独立复制，禁止静默跳过缺行。
- 原 status、所有 agent/orphan、目录 hash 与未知状态保留；`modified`／`conflict` 不因选中而改成成功。初始 stale／缺配置不执行 status；末次 seal 漂移保留选择材料，但不设置 `SealBindingCurrent`。

使用 go-multi-module-dev v2.6.0：保持既有 Application 编排及 owner typed EventHub 读取边界，不增加运行单元、跨 owner callback 或写入。未修改 Runtime／Start／Restart／LLM 请求，SM-1～SM-4 无行为变更。

YAML 依赖固定为 `gopkg.in/yaml.v3 v3.0.1`；原 `go.sum` 已含校验值。本轮使用本机缓存离线 `go mod vendor` 生成 16 个依赖文件及 modules 清单，没有手改 vendor 或下载新依赖。首次 vendor 命令因默认 Go cache 不可写而失败；指定 `/tmp/workorch-go-cache` 后成功，后续验证均使用该 cache。

## 2. 验证

受限证据根：`/tmp/workorch-step04-skill-selection-t1uuS6`。下列 `release` 日志是最终本轮源码的结果；无 release 的日志为较早候选运行，不能替代最终快照。

| 检查 | 结果／原证据 |
| --- | --- |
| 五个直接受影响包完整 `go test -count=1` | 全部通过；`affected-go-test-release.log`。core 0.009s、ports 0.385s、skillselectcore 0.005s、application biz 56.048s、Workspace biz 61.859s |
| 九个新增顶层测试的定向 race | 五包通过；`race-release.log`，包括元数据 27 类表驱动反例、真实 owner 七个组合场景、scope／取消／零额外 status、旧策略等价与来源隔离 |
| 两个 opt-in 真实 SkillHub CLI 测试 | 通过；`real-skillhub-release.log`；全新临时配置、安装目录和回环服务 fixture，不使用本机真实 SkillHub 配置或现有 daemon |
| YAML parser fuzz | 通过；`fuzz-release.log`，3 秒配置窗口、36,630 次执行；不把短 fuzz 视为穷尽证明 |
| 五包 `go vet` | 通过；`vet-release-confirmed.log`（成功无输出，命令退出 0） |
| `make fmt-check verify-boundary-contract` | 通过；`boundary-fmt-release.log`，含既有 Python cache binary-match 提示，不将提示当成业务测试失败 |
| 构建输入与逐文件身份 | 测试前后 snapshot 完全相同；12 项 `source-files-release.sha256` 均复核一致。未构建 Workorch 二进制，不能产生或引用新的 binary 验收身份 |

完整回归所指五包：

```text
./internal/modules/application/workorchd/biz/appservice/contextregistrycore
./internal/modules/application/workorchd/biz/appservice/contextregistryports
./internal/modules/application/workorchd/biz/appservice/skillselectcore
./internal/modules/application/workorchd/biz
./internal/modules/kernel/workspace/biz
```

定向 race 的选择表达式为：

```text
^(TestPreparationSkillMetadata|TestPreparationSkillHubCatalog|TestTypedSelection|TestPreparationSkillSelection)
```

真实 CLI 验证经显式环境 opt-in 执行；普通包级回归中的两项 real-CLI Skip 不记成执行通过。独立 opt-in 验证确认：

1. 项目 status 真正保存 inspection state；源文件和安装文件 bytes 不变；随后严格 metadata 映射保留安装正文与版本。
2. 全局一个 skill、两个 agent 行；只读取 Codex 安装正文，真实 `conflict/target_unmanaged` 未被掩盖，也未执行 apply/use。
3. 服务端优先路径只向隔离回环 fixture 发出一次 health 和一次 project-status；无本机配置 fallback。无 frontmatter 的原安装正文经映射保持不变。

原输出索引：`project.stdout.bin`／`stderr.bin`／`observation.json`，`global.*`，`service.*`。完整指令 bytes 在 observation JSON 中保留。上述证据仅证明隔离 CLI／读取／映射，不证明用户环境 daemon 和本机资源一致，也不是生产 Workorch HTTP／浏览器／进程验收。

## 3. 源码与产物身份

基线 HEAD：`834aa350cc4e77379819666540b9912a7c22b64c`，包含前序未提交改动和本轮改动；不是“该 HEAD 已独立验证”的说法。

- `source-snapshot-release.json` 与 `source-snapshot-after-tests.json` 完全相同。
- `buildInputSha256`：`19befba900d76ebe444dea1b8450172f0979191483b5bd4bcaac0e642e4c9b69`。
- `consoleTreeSha256`：`dac115ba997bdb66fa357a498b9a55515b33a3f8e37dc6083c47816d45fa1c4c`；本轮未重新构建或验证生产 Web。
- SkillHub 二进制：`/home/fedquery/.local/bin/skill-hub`，本轮复核 SHA256 为 `fcaec9f0697ec37d2277c924772e6133fc7fe579da078c09dbfbeea72130f40d`，见 `skillhub-binary.sha256`。

以上仅为本地 bytes 身份，不是构建因果或可信 attestation。报告位于验证区，不进入设计权威目录；报告本身不在构建输入扫描范围，写入报告后再次复核 snapshot。未提交凭据、live 配置或原始诊断到 Git。

## 4. 明确保留的退出门槛

本轮 `SelectionCompiled` 仅表示选择材料已生成，`SealBindingCurrent` 仅表示末次 Workspace seal 核对。它们不是 resource PASS、当前有效性证明、长期 freshness、Runtime context 封存或 M3。

1. F03 仍需辅助文件真实读取、Skill status／freshness 与 daemon-local 来源绑定、选择后的既有 context 预算核验；不把 `modified`／`conflict` 或只读到了 SKILL.md 当作 ready。旧展示／执行选择路径未被本轮替换，不能外推为其完整性已经修复。
2. F03 仍需目录权限、已有托管配置、MCP 和 Memory 的实际 owner 事实与原模式，及所有 required 检查的统一组合。缺资源仍须准确 BLOCKED，不扩大自动准备或付费调用。
3. F05/F08：显式检查接纳、精确幂等、私有 reducer/CAS、attempt 上限、受限原诊断持久化和原操作恢复尚未完整接线；当前子用例不是公开可重试事务。
4. F07：所有 required 事实与 no-Run/epoch 最终 CAS、唯一首次 M3，以及完整竞争／崩溃／unknown 验证矩阵仍需完成。
5. 增量三：HTTP 检查／恢复、Web 分轴与迟到 fence、生产构建 HTTP／浏览器、真实服务进程重启尚未完成。普通 GET 不触发本轮选择观察。

未运行收费模型、生产 Workorch 服务、全仓测试或 GitHub CI；未处理／复验既有公开 fullflow、orchestrator 累计超时及 build-identity CLI 问题；不声明全 CI 或 live 准入通过。未提交或推送本轮修改。
