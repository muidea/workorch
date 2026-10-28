# STEP04 SkillHub v0.13.7 登记接线与验证

日期：2026-10-28。源码基线：`884478cccb7db8bc15ab7bed8bed5b484745213e` 加本轮工作树修改；尚未提交、部署。本记录是开发验证记录，不是设计权威、可信构建 attestation 或完整 STEP04/live 退出证明。

## 升级核对

实际安装 `/home/fedquery/.local/bin/skill-hub` 为 v0.13.7，版本 commit 为 `3d51437b671490cc391ba5e30cbae20e08b4e245`。按该精确 commit 读取公开 CLI、spec、state 实现与发布说明，未将本机较旧的 skill-hub checkout 当作安装产物。

新接口为 `project register <path> --request-id <id> --json` 与同入口 `--query`。回执仅在同一 Hub home 内去重和查询；先持久 claim，再登记；终态重放原结果，异常中断保留 unknown；REQUEST_NOT_FOUND 不证明从未执行。它不提供历史 status 回执。

独立临时 Hub 的真实 CLI 实测：空目录登记成功；原查询和同 ID/同路径重放 stdout 完全一致，state bytes 不变；换路径冲突；中断 claim 查询 unknown；缺回执保持失败；登记后普通 status 可观察真实空 inventory。所有 fixture 均隔离，禁用 service bridge，不使用业务 Hub 配置或模型。

## 本轮实现

1. 现有显式 initialize-resources/recover-resources HTTP body 保持 `{requestId, expectedWorkspaceRevision}`。Application 从服务端取得 Hub home，Workspace 精确封存原 startup/config/Soul/epoch 与登记计划，生产初始化使用私有 journal v2；旧 v1 不自动补执行。
2. File 仍由其 owner 创建缺失资源且不覆盖既有内容。File 完成后，Workspace 另行 CAS 保存登记 attempt，才返回 Application 私有延续执行 CLI。登记不扫描、导入或安装 Skill。
3. 新增一个 Workspace 私有 state-lane 事件 registration-settle，唯一发送方 application/workorchd。结算反查原请求、计划、attempt、revision 和完整前像；保存原 stdout/stderr 与独立传输错误 bytes/count/hash/完整性。公开投影只增加有限 registrationState/registrationAttempts，不泄漏 home 或原文。
4. 首次登记之后，恢复只查询原回执，不重新登记；File 与登记/查询各有独立的 4 次持久预算，丢失响应不退还次数。普通重复请求、GET、已完成恢复均不执行 CLI。已确定失败不再查询。
5. 只有完整、无歧义、精确原身份/路径、executed/finished 与成功 registered 回执才能完成 v2 初始化。未知、运行中、缺失、scope 漂移及不完整输出继续阻塞。初始化完成仅证明历史基础设施行为，后续仍须显式真实准备检查。
6. Web 分别展示 File 和登记预算，恢复按钮使用当前阶段预算，确定失败/耗尽保持阻塞，不自动注册或检查。更新嵌入生产 assets。

责任边界：Workspace journal/reducer/CAS；Application 外部 CLI 适配；File 文件创建。未新增运行单元，不改变 SM-1～SM-4，不修改 provider、Runtime、Start/Restart 或收费验证范围。

## 验证结果

以下均为作者本地执行，非独立评审或 CI：

- 完整包回归通过：skillhubpreparation、Workspace models/events、managementcontract、contextregistryports（含真实 v0.13.7 opt-in）、file/memory repository、events 清单及边界。
- 准备累计回归通过：Workspace biz `-run Preparation`（68.789 秒）、workorchd biz `-run Preparation`（24.597 秒）。后续保全错误字段及有限投影补强通过新增定向回归。
- 最终定向 race 通过：Workspace biz、contextregistryports、skillhubpreparation、managementcontract，选择 `Registration|UnregisteredProject`。包含真实 CLI → 原结果丢失 → owner 关闭重开 → 仅查询恢复、预算、防篡改、scope 漂移、确定失败、不授 Run 及 repository 原字节克隆隔离。
- Web 准备组件 74 项通过；规则测试 11 个文件通过；生产构建通过。
- `make verify-boundary-contract cli-test` 通过（CLI guards 12/12）；受影响路径 `go vet` 通过。
- 未运行全仓慢 Runtime 套件或全 CI；未进行新版本生产进程重启/浏览器验收，不将 owner reopen 称作进程重启。无推理、embedding 或 Run 创建。

测试隔离纠正：首次接线回归遗漏 Hub home fixture，曾新增一个 `/tmp/TestPreparationInitializationProductionCreatesMissingPreservesEx…` 空测试项目。已持有原回执锁与 state 锁，按精确测试 request-id、路径、时间和空技能状态校验后删除该项及对应回执，其他项目数据值保留。随后修正所有经过 Application 初始化入口的测试，强制独立 Hub home 与 CLI fixture；真实二进制用例亦强制独立 home。该次失败不记为通过。

## 尚未解除的边界

- 新登记回执不能证明旧 v0.13.5 status 的原操作结果；现存无外部 request-id 的 unresolved 账本未删除、未重分类，旧 Workspace 仍不能绕过。需要独立、合法的旧操作对账依据或协议，不能借升级/登记成功直接解锁。
- Provider 就绪/授权是独立未通过项，本轮未触碰。
- 本轮新初始化主体可在构建部署后进行无收费 live 复验；不表示现存旧数据已可恢复，更不表示整体 STEP04 已完成或 ready/M3。
