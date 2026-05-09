---
name: workorch-core-template-boundary
description: "在 WorkOrch 开发、评审或排查中涉及 kernel/orchestrator/appservice、workspaceTemplate、Soul、DoD、evidence policy、prompt/context、ticket/approval、runtime block、memory 或 e2e 时使用；用于强制核对 WorkOrch 基础能力与 workspaceTemplate 业务策略边界，避免把具体场景、语言、工具或模板规则写入 core 通用流程。"
metadata:
  version: "1.0.0"
  author: "rangh"
  created_at: "2026-05-09T17:01:44+08:00"
---

# WorkOrch Core / Template Boundary

本技能用于在 WorkOrch 代码开发、代码评审、缺陷排查和需求收口时，先确认“基础能力”和“模板业务策略”的职责边界，再决定修改落点。

## 适用场景

- 修改 `internal/modules/kernel/**`、`internal/modules/blocks/**`、`internal/modules/application/workorchd/biz/appservice/**`、`internal/pkg/repository/**`、`application/workorch` 或 CLI/TUI/Web 入口时。
- 涉及 `workspaceTemplate`、Soul、Goal、DoD、Run、Ticket、Approval、Memory、Artifact、EvidenceQuality、runtime block、LLM prompt/context、e2e 验收时。
- 排查“某个模板或业务场景执行异常”时，特别是软件开发、天气查询、代码审查、文档生成等模板相关问题。
- 发现通用代码中出现具体业务名、模板名、编程语言、命令、外部服务、城市、服务器地址、测试输出 marker 等内容时。

## 核心原则

- WorkOrch core 只提供中立的编排、状态机、事件、治理、证据、上下文传递、运行时调度和质量门禁能力。
- `workspaceTemplate`、Soul、默认 DoD、template-bound policy、Skill、MCP 配置和 e2e 数据承载具体业务策略。
- 通用代码可以消费结构化字段，但不能根据模板名、Soul 名称、业务文本、命令输出文本或具体场景字符串推断行为。
- 如果需要支持某类场景规则，优先把规则做成数据配置，再由通用能力读取数据执行。
- 不为了快速修复在 core 中加入临时兼容、场景特判或 “contains xxx” 逻辑。

## 职责边界

WorkOrch core 可以包含：

- 通用状态：Goal、DoD、Run、Step、Ticket、Approval、Memory、Artifact、EvidenceQuality。
- 通用机制：事件发布订阅、异步回调、上下文预算、traceability、质量门禁、repository clone、read model 投影。
- 通用 block 能力：shell/file/web/llm/mcp/approval 等能力的参数执行、结构化输出和错误处理。
- 通用策略解释器：例如读取 `evidencePolicies` 并按统一 schema 标记 `evidenceQuality`。

WorkOrch core 不应包含：

- 具体模板 ID 或 Soul ID 的业务判断，例如 `software-dev`、`weather-query`、`code-review`。
- 具体语言、框架或命令策略，例如只针对 Go、Python、Node、`go test`、`npm test` 的判断。
- 具体业务实体，例如城市、服务器地址、SSH 账号、比赛、某个应用配置。
- 具体输出 marker 的硬编码，例如某测试框架的“未执行测试”文本。
- 只为某个 e2e 场景通过而加入的流程分支。

Template / Soul / policy 可以包含：

- 模板 ID、阶段 Soul、默认 Goal、默认 DoD、业务提示词和执行约束。
- template-bound policy，例如 `workspaceTemplate.evidencePolicies`。
- 特定模板的验收标准、e2e fixtures、示例仓库、场景化测试数据。

## 工作流程

1. 先定位需求属于基础能力还是模板策略。
   - 如果多个模板都应复用，优先放在 core 通用能力。
   - 如果只对某类 workspaceTemplate 生效，优先放在 template/Soul/policy/e2e 数据。
   - 如果不确定，默认不要写入 kernel/orchestrator，先抽象成结构化数据和通用解释器。

2. 修改前执行边界搜索。
   - 使用 `rg` 检查目标通用目录是否已有或即将加入场景字符串。
   - 对涉及证据、prompt、DoD、ticket、approval 的修改，检查是否有按文本内容推断业务语义。

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

3. 选择正确落点。
   - 通用字段和状态：放在 `internal/pkg/common`、kernel model、repository、read model。
   - 通用执行与门禁：放在 orchestrator 或 block 通用解释器。
   - 模板策略：放在 `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`、`souls.json`、Skill/MCP 配置或 e2e fixture。
   - workspace 创建期绑定：模板数据需要随 workspace 固化时，创建 workspace 时从 template 拷贝到 workspace，运行期只读取 workspace 绑定数据。

4. 修改中保持数据化。
   - 使用结构化字段，例如 `evidenceQuality`、`zeroWork`、`evidenceTags`、`policyId`。
   - 避免在通用代码里解析自然语言业务结论。
   - 避免通过 Soul ID、template ID、workspace title 推断功能分支。

5. 修改后做反向核对。
   - 搜索通用目录中的业务词、模板词、语言词和命令词。
   - 确认 template 专项内容只出现在 `templates.json`、`souls.json`、template/e2e 测试或专用 fixture。
   - 确认新增单测至少覆盖：通用解释器、template 数据绑定、非目标模板不被影响。

## 排查流程

1. 先复盘异常来自哪个层级：template 数据、workspace 绑定数据、run executionContext、block 输出、orchestrator 门禁、read model 展示。
2. 如果通用层误判业务语义，优先删除通用层判断，并改为结构化 metadata 或 template-bound policy。
3. 如果 template 数据未生效，检查创建 workspace 时是否已绑定到 workspace；不要在激活 run 时按模板名临时补策略。
4. 如果 e2e 依赖某个具体场景，测试名和 fixture 可以具体，但断言应证明通用能力消费的是结构化字段或模板数据。

## 验证清单

- `go test ./... -count=1` 通过，或至少先跑受影响包再跑全量。
- 通用目录搜索没有新增不该出现的具体业务/模板/语言/命令特判。
- 新增模板策略能从 template 创建期绑定到 workspace，并能在 run executionContext 中追溯。
- Web/TUI/CLI 只展示业务模型字段，不复制后端的业务推断逻辑。
- 文档同步说明策略归属：core 通用能力、template 数据、Soul 约束或 e2e fixture。

## Formatter

- `SKILL.md` / Markdown / YAML: 保持标题、列表和代码块格式稳定；归档前运行 `skill-hub validate workorch-core-template-boundary --links`。
- `scripts/`: 当前技能未包含脚本；新增脚本时，必须在本段补充项目可运行的具体 formatter 命令。
- Go 代码示例或辅助脚本使用 `gofmt -w <files>`；Shell 脚本至少保持 Bash 语法一致并运行可用的语法检查。
- 不要声明当前项目无法执行的 formatter；如果对应文件类型没有 formatter，明确写出人工格式要求。

## 输出要求

- 开发或排查完成时，明确说明修改落点为什么属于 core 或 template。
- 列出发现并清理的边界偏差。
- 给出执行过的边界搜索命令和测试命令。
- 如果新增或调整 template/Soul/policy，必须说明该策略如何绑定到 workspace、如何进入 run context、如何被通用能力消费。

## 禁止事项

- 禁止在 `kernel/orchestrator`、通用 `appservice` 或 repository 中写入具体模板、业务、语言、命令输出 marker 的特判。
- 禁止用“先兼容一下”的方式绕过 template/Soul/policy 归属。
- 禁止让 TUI/Web/CLI 形成另一套业务决策逻辑。
- 禁止把 e2e fixture 中的业务文本复制到 core 作为判断依据。
