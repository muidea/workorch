# workorch Process Service

Status: active

Type: process-service-contract

Last Verified: 2026-09-11

## 功能说明

`internal/services/workorch` 是进程级生命周期 shell：选择 server、interactive TUI
或 remote CLI mode，装配 framework application，并把端侧适配器连接到稳定 port。
它不是 framework module，不拥有 owner 状态，也不向 application 导出 owner service。

在 server mode，process service 在 framework `Run` 前订阅 workorchd 的
`ServiceStoppedData`。它只在自身维护一次性的 local wait channel/once，并把有界
stopped data 映射为进程退出结果；channel、HTTP server、repository、registry 和
error object 都不得进入 EventHub command/data/result。Shutdown 先取消 observer，再
释放 local wait state。

magicCommon v1.5.17 的订阅/取消订阅结果必须检查；失败不得发布 observer 或报告 projection `live`。
组合事件订阅后半段失败时须撤销前半段，返回原错误及必要的清理错误。无错误返回值的端侧取消回调仍须
记录失败或发布 degraded 观察，不能伪造取消成功。资源 owner 的必需订阅沿共享 Base Biz fail-fast，
由 framework lifecycle guard 处理；端侧可恢复订阅直接消费 checked Hub/SimpleObserver 结果。

进程通过 `frameworkModuleService` 显式委托 DefaultService 的 `Quiescer` 与 `CheckedShutdown`。
全部 owner 先 BeginShutdown 关闭 fresh 入站并取消工作，再 Quiesce 等待真实退出；随后 Application
排空共享 BackgroundRoutine 与 Hub，最后执行检查式 Module Teardown、Initiator 释放和 Hub terminate。
该 shell 不遍历插件、不维护第二份 owner 状态。Setup/Run 返回错误或 panic 都进入同一检查式停止路径，
原失败与停止失败保持 cause；停止 panic 也必须作为失败返回，不能当作成功。停机预算独立于已取消的
运行 context，并受配置超时限制。`ShutdownChecked` 失败保留 started 标记；端侧 close 失败保留
backend/close 句柄，framework 停止失败保留 logging 句柄，允许新预算重试。
SM-1～SM-4、归档和 owner authority 不受该基础设施接线影响。

回归入口：`framework_service_test.go` 的 checked error/worker drain、`runtime_test.go` 的 server-stop
订阅失败，以及 `biz/backends/eventhub_subscription_failure_test.go` 的 closed-Hub/partial rollback。

TUI/CLI 只依赖 `internal/pkg/workorchport` 的稳定 contract；embedded backend 通过
typed EventHub command/result，remote backend 通过 client，不直接调用 owner `biz`、
`service` 或 repository。

TUI `inspect acceptance` 只读显示 sealed original diagnostic 中完整的 input-review check/obligation、
原 CWI/check、输入 refs、subject/report hash 与原因；紧凑进度视图只给出报告数量和查看提示。
评审不是实际验证或验收证据，端侧不能据此选择重试、自动解决 ticket 或重建 authority。

`/exit` 是 TUI 唯一显式进程退出命令，由根按键入口在 Help、model-selection 等 modal
分流之前统一识别。modal 只能暂存该完整命令，不得各自复制退出语义或吞掉它的按键；
Bubble Tea 返回后再有界地取消 projection observer，最后交由 process service 执行 framework Shutdown。
相关验收覆盖普通界面、Help、model-selection、真实 Bubble Tea 程序循环以及 observer 取消超时。

## 非职责

- 不注册 framework Module/Block，不拥有任何正式聚合或 Runtime 状态机。
- 不把 local wait channel、HTTP listener、repository、EventHub 或 Session 作为跨组件合同传递。
- 不在端侧重建 authority、自动 Resume/Restart 或补偿 owner 状态。
