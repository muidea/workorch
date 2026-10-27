# STEP04 创建栅栏与私有 reservation 验证记录

日期：2026-10-21（Asia/Shanghai）。源码基线：`834aa350cc4e77379819666540b9912a7c22b64c` 加未提交树。

## 结论与边界

本轮落实已批准 no-Run 协议的 creation 标识、首个未决 reservation 持久基础和 owner-private CAS。
不是完整协议退出证据，**生产执行入口尚未接入 reserve-before-forward，不能签发 M3 或宣布 STEP04/live-ready**。

现有未提交的 cache-only LLM / Run 完整历史观察及计划文档迁移全部保留；不把它们说成本轮新增。
本轮未提交 Git，未改现有服务、live 配置或数据，未使用收费 provider/embedding，未提交真实执行意图。

使用 `go-multi-module-dev`：沿用 Workspace owner 和其 repository，不增加运行单元、跨 owner repository、
EventHub topic 或执行权限。`workspace.fresh_intent_reservation` 登记为同 owner 私有事务。
SM-1、SM-2、SM-3、SM-4 均不新增转换；Run 生命周期、Runtime authority/dispatch/drive 不变。

## 本轮实现

1. 唯一 Workspace creation dispatcher 生成随机 128-bit `CreationEpoch`，与首次 metadata 一起持久化。
   authoring 同键创建重放直接复用原记录；bindings/index 部分持久化后 reopen 仍保留原 epoch。
2. `PreparationFence` 独立于 admission、StartupPreparation、Run lifecycle。
   nil 明确为 legacy；普通 Update/CAS 不可把 legacy 补成首次准备正例。
3. fresh typed command 先做完整 raw/结构校验，再规范化并对完整 payload 计算域分离身份。
   scope、request、kind、StartupInputHash 及所有 bootstrap 字段参与身份，不读取配置或保存配置凭据摘要。
   Resume 不被当成 fresh。该内部身份没有公开恢复/执行权限。
4. 私有 reducer 先识别原 pending 身份，后检查新请求的 revision/admission/startup。
   原 Definition 漂移不能令相同事务再次登记；同键异 payload、新 fresh intent 均不能绕过 pending。
5. 只有实际 CAS 成功的本次调用获得新登记标记；CAS 前失败、写后丢回复、重复与 owner 重建均不产生第二个标记。
   dispatcher 不转发 Run、不调用其他 owner，不持锁做跨 owner I/O。
6. memory/file/scoped repository 的 clone、Create、Update、CAS 和 file raw reload 保全该字段。
   Update 不允许新增 reservation；CAS 不能抹除/替换原 pending、重置 epoch、改 scope/版本/creation 标识。
   同 Template / 换 Template 的 detached authoring view 均隔离嵌套 reservation。
7. reservation CAS 推进 Workspace revision，已有准备观察投影为 stale，同时完整保留原 unknown/blocked 等历史与请求。

当前 v1 基础只支持创建标识和首个未决 reservation，**没有结算/清除 API、完成历史或第二次 fresh 登记**。
这不是自动解锁策略，也不以 TTL、空 Run 列表、请求缺失、重启或调用失败清除 pending。
精确结果消费者未完成前，不把此私有入口接到现有生产执行流程。

## 验证与证据索引

受限本地证据根：`/tmp/workorch-step04-reservation-KH4t6q`。
该目录及二进制/日志未提交，目录仅本机可读；Git 中保存本记录而非原始诊断。

| 验证 | 结果 | 原始索引 |
| --- | --- | --- |
| Workspace 全子包、repository 全子包、scoped repository、Application biz、Run/LLM 全子包、events 累计回归 | 24 个包通过，5 个包无测试 | `affected-go-test-final.log` |
| 本轮定向 race（包含 memory/file/scoped reopen 与创建 cutpoints） | 3 个包通过 | `focused-race-final.log` |
| Workspace/repository/scoped/Application biz vet | 通过 | `vet.log` |
| fmt-check + boundary contract | 通过 | `fmt-boundary-final.log` |
| git diff --check | 通过 | 本轮命令输出 |
| 隔离 Go binary build | 退出 0，未启动 | `build.log`、`workorch` |
| build identity capture/produce | 构建前后输入一致，身份清单生成成功 | `source-snapshot-final.json`、`build-identity.json` |

定向命令：

```sh
GOCACHE=/tmp/workorch-go-cache go test -race \
  ./internal/modules/kernel/workspace/biz \
  ./internal/modules/kernel/workspace/pkg/models \
  ./internal/pkg/repository \
  -run 'Test(FreshReservation|WorkspaceCreationMints|PreparationFence|DefinitionAuthoringCreationFile)' -count=1
```

新增 10 个测试函数，并补强原有 2 个 authoring file creation 测试。
覆盖：32 个并发相同请求只产生一次新登记、不同请求竞争、同键异 payload、原 admission/Definition 漂移后重放、
CAS 前失败/写后丢回复、owner 重建、legacy/stale/scope/cancel 拒绝、完整 payload 身份、Resume 排除、
准备历史失效但不改写、内存隔离、file/scoped 重开、非法 raw 只读拒绝、普通更新与 CAS 防抹除、创建部分写入恢复。
这些不是进程 stop/start 或生产 Web 验收。

首次累计测试在受限沙箱中因 `listen tcp 127.0.0.1:0: socket: operation not permitted` 导致 Application 失败；
随后仅放开本地测试监听能力复验，Application 全包通过（`application-go-test.log`），最终累计也通过。
首次 boundary 检查发现新增私有事件未登记唯一 owner；已补精确 manifest 行，原门禁不放宽，最终通过。
原首次失败输出保留在本轮工具记录；未将最终成功反写成首次即成功。

build 产生 module stat cache 只读写失败警告，原文保留于 `build.log`；退出状态为 0。
没有声称 build_identity CLI verify 通过，也不把本机内容身份称为可信 attestation 或部署来源证明。

| 身份 | 本轮值 |
| --- | --- |
| sourceCommit | `834aa350cc4e77379819666540b9912a7c22b64c` 加已捕获未提交源码 |
| buildInputSha256 | `389b9267ebf0a16feebfcb293400e9179f6551e9a2e12cb48614d0db77591b8b` |
| consoleTreeSha256 | `045e1a379991d29078f1bbab1752b882e2986cad8b770b4be2dc3f40df60960b` |
| binarySha256 | `d73428c7aa6d20cbad2d853bb3ce72101d2d8eed97a713879c13f71d4eccb099` |

Web 未修改/重建；使用现有 embedded assets，仅验证本次 Go 构建输入与产物，不声明新的浏览器验收。

## 必须继续的剩余项

1. Run owner 持久精确原请求结果和只读查询，不能用现有有激活副作用的 restart replay 充当查询，
   也不能由当前 Run 状态伪造历史 outcome；缺结果仍保持 unknown。
2. 在上述消费者可用后，完整 reservation 结算/历史与所有生产 fresh ingress 的 reserve-before-forward 接线。
   先识别原请求再检查当前 Definition，验证转发接纳后丢回复、迟到回复、进程重启与新请求竞争。
3. F03 其余资源 owner 实测、配置/目录完整性和准确阻塞；已完成的只读子能力不能代替完整 readiness。
4. F05/F08 显式检查、attempt reducer/CAS、原操作恢复；F07 no-Run final CAS、唯一历史 M3。
5. 增量三 HTTP/Web 分轴展示、显式检查/恢复、迟到响应 fence、真实独立进程 stop/start 与生产浏览器验收。

STEP05 Start/Restart 消费准备回执仍不在本轮范围。
未重跑全仓 CI，也未处理继承的 fullflow 默认 Profile 断言、orchestrator 套件累计超时和 build_identity CLI verify 问题；
本轮通过项不能扩展为全 CI、发布或完整第四阶段通过。
