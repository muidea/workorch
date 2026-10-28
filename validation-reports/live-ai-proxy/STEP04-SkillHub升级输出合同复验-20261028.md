# STEP04 SkillHub 升级输出合同复验

日期：2026-10-28。用户已升级 SkillHub 后进行验证。本轮仅隔离验证和读取现有 Workspace 记录，未修改生产代码、配置或业务数据，未执行生产 status、登记、初始化、模型调用或 Run。

## 实测结论

升级后 `/home/fedquery/.local/bin/skill-hub` 的 buildinfo 为 v0.13.6、revision `882a5e513f0a72557a758229209154d6ce53424f`、`vcs.modified=false`；二进制 SHA256 为 `acfc94758233c4880c4c6e365944a65fd03aaf79e30c7944ebbf13229d676a57`。这些是实际二进制声明与内容身份，不表示独立建立源码构建 attestation。

使用独立 SKILL_HUB_HOME、独立项目、已存在 `.agents/skills`、空登记表、DEVNULL 输入，并关闭隔离用例的 service bridge：

| 场景 | 结果 |
| --- | --- |
| 未登记项目首次 `status --json` | exit 1；stdout 为唯一合法 JSON 错误文档，`command_error.code=PROJECT_NOT_FOUND`；未登记项目、无交互提示混入；stderr 0 |
| 仍未登记的相同隔离项目 | 同样返回 PROJECT_NOT_FOUND；没有隐式登记 |
| 测试预先登记的空项目 | exit 0；stdout 为唯一合法 project status，project_path 精确一致，items=[]；stderr 0 |

第一次错误输出为 162 bytes，SHA256 `c31f090c9696a255d1957c0125648f8be6180b1736edad0236eb2b01d97a9b07`；预先登记后的成功输出为 147 bytes。长度包含隔离路径，不能直接与上一版本生产输出比较。

因此原 stdout 污染原因在上述隔离路径已不再复现，但生产者也明确改变了首次项目语义：机器 JSON 模式不再把 EOF 当成登记同意。合法 JSON 错误不能当作成功 inventory，Workorch 不应将 PROJECT_NOT_FOUND 转为空 Skill 来源。

## Workorch 真二进制适配回归

运行以下现有用例，显式启用升级后的真实 CLI：

```text
WORKORCH_TEST_SKILLHUB_BINARY=/home/fedquery/.local/bin/skill-hub go test ./internal/modules/application/workorchd/biz/appservice/contextregistryports -run '^TestPreparationSkillHubInstalledBinary(ProjectStatusEffects|UsesConfiguredService)$' -count=1 -v
```

- ProjectStatusEffects 通过：已登记项目的真实状态保存、安装指令原文观察、global 多 agent 行与冲突保留均通过。
- UsesConfiguredService 通过：仅一次 health 和一次 project-status 请求到隔离回环 HTTP fixture，无本地 fallback；不代表真实 SkillHub daemon 已实测。
- 首次沙箱运行的服务用例因禁止监听 socket 失败；随后在允许回环监听的执行环境中两个用例均通过。未将该环境失败认定为产品缺陷。

生产适配器没有关闭 service bridge，也没有追加未知 flag。本轮没有添加 parser 清洗、截取最后 JSON 或放宽退出码校验。

## 当前 Workspace 及后续范围

只读核对原 Workspace：revision 仍 28、两次准备 attempt，最后原 request `b3aab948-2cb0-4a5b-88cb-8f42753dde77` 仍 blocked，SkillHub effect 仍 unresolved，ActiveRunID 为空。该字段只表示未投影活动 Run，不作为完整 Run 库存证明。

升级不能改写旧失败或清除原效果栅栏。后续最小收口仍为：

1. 为新项目明确可用、显式且可审计的项目登记入口，接到允许的准备资源流程；只创建目录已不足以保证机器 status 成功。不得猜用全局 init 或技能 register 替代项目登记，也不得在 GET／status 检查中偷偷初始化。
2. 为当前原 unresolved 操作补精确对账依据和追加结算，保留旧输出、旧失败和 owner 历史；没有外部效果结束依据时不以升级或新请求绕过。
3. 两项完成后再用当前 Workspace 做新显式准备检查，验证完整 Skill 来源及 MCP 后续结果。Provider 缓存／授权消费者及 STEP04 正向 ready/M3 出口仍独立跟踪。

本次仅确认升级兼容性及新项目错误语义，不将它写成 Skill/MCP readiness、当前 Workspace 已解锁或整个 STEP04 通过。

## 受限证据

原始 fixture 配置／登记表、stdout/stderr、buildinfo 和 summary 位于忽略的 `.workorch-data/validation/step04-skillhub-v0136-upgrade-20261028/`，目录 0700、文件 0600；临时目录位于 `/tmp/workorch-validation/step04-skillhub-upgrade-mebo9t7m/`。真实生产配置和原始诊断没有提交到 Git。测试结论来自本轮工具输出，未声称上述目录包含完整 Go 测试日志。
