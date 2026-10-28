# STEP04 SkillHub 错误合同接线与回归

日期：2026-10-28；代码基线 `884478cccb7db8bc15ab7bed8bed5b484745213e` 加本轮未提交改动。

## 已落地行为

- Application 的真实 status 适配器识别 v0.13.6 的完整 `PROJECT_NOT_FOUND` JSON 错误。仅在 project、命令确实启动、退出码明确为 1 且 stdout/stderr 完整时分类为 `project_not_registered`；其他错误、流不完整、歧义 JSON 和异常退出保持原拒绝路径。
- 共享纯解析器要求完整唯一文档，拒绝提示前缀、尾随文档、重复键／大小写别名、混入成功 inventory 和未知字段。不截取／改写原 stdout、stderr 或错误材料。
- Workspace 从最近 blocked 原 effect 的真实私有材料派生有限只读诊断 `skillHubProjectIssue=project_registration_required`。不持久改写旧 attempt、旧 hash 或失败，不将错误变为空库存，不改变 unresolved 门禁或检查分数。
- typed EventHub → Application DTO → HTTP/Web 投影保留这一诊断，仅公开有限 code，不输出原 message、路径或配置。Web 明示项目未登记与目录存在的区别，保持禁止新检查绕过和禁止自动重发／登记。
- 服务 API 与 Workspace readme 已更新现行投影合同；生产 Web 资源已经重新构建。

本轮不改变 Runtime SM-1/2/3/4、执行授权、Run 创建、Provider 权限或 M3；没有新增业务写接口，也没有改现有 recover 的不重派发语义。

## 验证

以下均为本轮本地验证，不是全 CI 或生产 live 全部通过：

| 验证 | 结果 |
| --- | --- |
| `go test ./internal/pkg/skillhubpreparation ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/pkg/events ./internal/pkg/workorchmanagementcontract -count=1` | 通过；新增管理 DTO 测试后独立重跑 managementcontract 通过 |
| `go test ./internal/modules/kernel/workspace/biz -run 'Preparation' -count=1` | 通过，65.230 秒；后续增加原负例的跨重开测试单独通过 |
| `go test ./internal/modules/kernel/workspace/biz -run 'TestPreparation(UnregisteredProject\|SkillHub)' -count=1` | 通过，3.254 秒 |
| `go test -race ./internal/modules/kernel/workspace/biz -run '^TestPreparationUnregisteredProjectDiagnosticSurvivesReopenWithoutEffects$' -count=1` | 通过；GET、recover 和新身份拒绝后 owner 完整值保持不变，真实 file owner 重开后诊断仍在 |
| `WORKORCH_TEST_SKILLHUB_BINARY=/home/fedquery/.local/bin/skill-hub go test ./internal/modules/application/workorchd/biz/appservice/contextregistryports -count=1` | 通过；升级后真实 CLI 的未登记新项目负例、已登记项目及回环 fixture 服务模式均执行 |
| `go test ./internal/modules/application/workorchd/biz -run 'Preparation' -count=1` | 通过，23.917 秒；包含现有 HTTP 准备测试；新增有限诊断投影及错误绑定的 Application 用例另行定向运行通过（0.010 秒） |
| `npm run test:components -- --run tests/components/startupPreparation.test.tsx` | 72 项通过；新增未登记诊断不自动 POST、不解锁及错误绑定拒绝 |
| `npm test` | 11 个测试文件通过 |
| `npm run build` | TypeScript 与 Vite 生产构建通过；保留既有 bundle size 提示 |
| `make cli-test` | 通过，CLI 架构检查 12/12 |
| `make verify-boundary-contract` | 通过 |

首次管理投影新增测试因 fixture 缺少 v2 必需检查清单失败；补齐测试原输入后重新执行通过，没有放宽生产校验。受限沙箱无法写 Go 缓存／监听本地 fixture 的运行限制经允许执行环境解决，未当作产品失败。

## 尚未接通的主体出口

精确核对已安装 v0.13.6 提交 `882a5e513f0a72557a758229209154d6ce53424f` 的公开源码：

1. JSON status 不自动创建项目；独立项目登记命令不存在。`init` 初始化全局 Hub／仓库，`register` 登记具体 Skill，不能作为仅登记空项目的替代。
2. 没有原 status 操作身份与效果完成回执查询。当前 Workspace 的旧解析失败仍为 unresolved；新二进制成功、登记表当前有项目或进程 exit 0 均不足以抹除可能的外部未决保存。

因此尚不能安全实现“自动登记接线＋当前旧操作解锁”。已向用户请求确认将最小登记／原效果对账接口一并补到外部 skill-hub 项目；本轮没有修改该项目或生产登记表，未使用未知 flag、直接写 state、init/import/use 等旁路。

在外部接口可用并验证前，当前 Workspace 仍受原 fence 保护；MCP 仍需完整 Skill 来源后重验。Provider 缓存／授权消费者和 STEP04 正向封存继续独立跟踪。本文不将有限错误诊断改进标为主体解锁完成，也不将准备检查标为通过。

## 证据范围

本轮 Go/Web 测试输出保留于会话工具结果，未声称已归档每条原日志。上一轮真实 v0.13.6 隔离 stdout/stderr、buildinfo 和 fixture 材料见忽略的 `.workorch-data/validation/step04-skillhub-v0136-upgrade-20261028/`；外部精确版本公开源码只读下载至 `/tmp/workorch-validation/skillhub-v0136-source/`。没有生产写入、收费调用、清空数据、重启服务或 Run。报告及代码尚未提交。


后续进展：已安装 v0.13.7 后，登记及原回执接口已核对并接入，见同目录《STEP04-SkillHub-v0.13.7登记接线与验证-20261028.md》。本记录的“接口缺失”描述适用于当时 v0.13.6，不代表最新状态；旧 status 未决边界仍保留。
