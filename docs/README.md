# WorkOrch Documentation

Status: normative

Type: documentation-index

Last Verified: 2026-08-21

## Purpose

`docs/` 只维护当前产品设计、领域模型、Runtime 合同、组件边界、接口与验证合同。
一次性实施过程、已被替代的方案、dated run、临时审计和阶段编号不在当前文档树长期保存；需要追溯时使用 Git 历史。

阅读按三级逐步展开：

```text
docs/README.md
  -> numbered area overview
     -> one focused topic document
        -> implementation SoT / typed contract when needed
```

根入口只回答“去哪里读”；区域 overview 只回答“本区域有哪些专题及其权威”；专题正文只维护一个稳定问题域。

## Authority Order

1. [Runtime core](00-core/overview.md)：Runtime 合同入口及其规范性专题。
2. `internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract`：Runtime 实现 SoT。
3. 各 Initiator/Module/Block `readme.md`：组件职责、owned state 和 concrete EventHub 合同。
4. [Design baseline](01-overview/current-state.md)：现行标准的整体摘要；不声明代码收口进度。
5. 产品、领域、flow、架构、profile/interface 文档：解释当前合同，不新增更高权威。
6. 与专题同目录的 verification/acceptance 文档：证明相应合同，不定义新的状态、DTO 或 owner。
较低层文档不得覆盖较高层合同。实施排期、候选方案、迁移过程和 closure 状态不属于本规范树。

## Documentation Tree

| 层级 | 入口 | 单一职责 |
| --- | --- | --- |
| Core | [00-core](00-core/overview.md) | Runtime 权威合同、Definition、request/cache、执行 flow 与对应验证 |
| Design baseline | [01-overview](01-overview/overview.md) | 现行设计摘要与组件 contract inventory |
| Product | [05-product](05-product/overview.md) | 产品目标、业务合同、产品架构 |
| Domain | [10-domain-model](10-domain-model/overview.md) | 按领域对象拆分的状态与不变量 |
| Module architecture | [30-module-architecture](30-module-architecture/overview.md) | 运行单元、process service、storage/read-model 边界 |
| Profiles/capabilities | [40-profiles-capabilities](40-profiles-capabilities/overview.md) | Template/Profile、具体 capability 语义及专项验证 |
| Product interfaces | [50-product-interface](50-product-interface/overview.md) | HTTP、Web、TUI、部署与 deployed acceptance |

## Read By Task

- Runtime 状态机或 Owner：先读 [Runtime core](00-core/overview.md)，再读 Orchestrator/Run 对应组件 README。
- LLM request、cache：读 [request/cache contract](00-core/runtime-request-cache-note.md) 和
  [context assembly](00-core/runtime-request-context-assembly.md)；长期 Run 的跨 provider/capability 消费读
  [execution budget](00-core/runtime/execution-budget.md)。
- Workspace、Goal、DoD：读 [Workspace domain](10-domain-model/workspace-goal-dod.md) 和
  [activation flow](00-core/definition-convergence/activation-flow.md)；运行期发现Definition歧义再读
  [runtime clarification](00-core/definition-convergence/runtime-clarification.md)。
- coverage、assessment、evidence或formal Result：读 [semantic convergence](00-core/runtime/semantic-convergence.md) 和
  [result closeout](10-domain-model/result-closeout.md)。
- Module/Block 落点：读 [module architecture](30-module-architecture/overview.md) 和对应组件 README。
- HTTP/TUI/Web：从 [product interfaces](50-product-interface/overview.md) 逐级进入。

## Verification By Subject

| Subject | Verification contract |
| --- | --- |
| Runtime SM-1～SM-4、owner、candidate、receipt、recoverable wait、execution budget、coverage/closeout、terminal | [Core Runtime Verification](00-core/runtime/verification.md) |
| Definition admission、all-of scheduling、clarification、context budget | [Definition Verification](00-core/definition-convergence/verification.md) |
| software-development specialization | [Software-Dev Verification](40-profiles-capabilities/software-dev/verification.md) |
| HTTP/Web、projection、persistence and deployed service | [Service Acceptance](50-product-interface/service-acceptance.md) |
| PRD多drive、crash-window、wait/budget、governance、revalidation、restart和terminal deployed gate | [Deployed Convergence Scenarios](50-product-interface/deployed-convergence-scenarios.md) |
| TUI static/model/backend/entry behavior | [TUI Acceptance](50-product-interface/tui/acceptance.md) |
| governed concurrency/load observation | [Loadtest Baseline](50-product-interface/loadtest-baseline.md) |

每项变更先运行受影响 package/contract tests，再执行仓库规定的 aggregate gate；零测试命中不能算通过。unit test
不能替代明确要求 process、provider 或 filesystem 的 deployed gate，反之亦然。real-provider checks 必须显式启用并与
CI-safe baseline 分离。验证正文与被验证专题同目录；本表只提供全局入口，不复制断言或命令清单。

## Maintenance Rules

1. 每个目录必须有一个直接入口；父级只链接下一层，不平铺列出所有叶子文档。
2. 一个专题文档只定义一个对象族、一次 flow、一组同 owner 合同或一个验收面。出现两个可独立演进的主题时必须拆分。
3. overview/index 不复制专题正文；current-state 不复制完整 Runtime 合同。
4. 默认在约 250 行时检查是否职责混杂；行数不是机械门禁，但超过 400 行必须说明为何仍是不可拆的单一合同。
5. 新文档必须由直接父级入口引用，并声明 `Status`、`Type`、`Last Verified`；孤立文档视为无效内容。
6. implementation plan、candidate、TODO/unclosed matrix、过渡性设计、superseded 方案、单次审计、dated run、临时
   benchmark、逐提交 closure diary 和旧 schema 说明不得留在当前树。
7. 已删除的 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter registry 和
   自动 replan successor 只可作为明确禁止项出现。
8. 文档移动或删除必须同步修复 Markdown 链接、根 README/区域入口、AGENTS/代码测试所使用的稳定路径。
9. 实现与规范不一致时，只更新确有错误的唯一规范；尚未符合的实现通过文档树外 issue/ticket 收口。不得为
   “当前实现”和“目标实现”各建一份平行设计或中间态文档。

## History And Recovery

历史正文从工作树删除并不丢失：Git 保留完整内容、作者和时间线。需要恢复时使用 `git log -- docs/`、
`git show <revision>:<path>` 或对应提交。单次验收证据、截图、原始输出和有保留期限的记录一律存放在 `docs/` 之外；
专题 verification/acceptance 只维护可重复执行的规范性断言，具体命令以 Makefile、package tests 和 scripts 为执行 SoT。
