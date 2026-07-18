# workorch Process Service

`internal/services/workorch` 是进程级生命周期 shell：选择 server、interactive TUI
或 remote CLI mode，装配 framework application，并把端侧适配器连接到稳定 port。
它不是 framework module，不拥有 owner 状态，也不向 application 导出 owner service。

在 server mode，process service 在 framework `Run` 前订阅 workorchd 的
`ServiceStoppedData`。它只在自身维护一次性的 local wait channel/once，并把有界
stopped data 映射为进程退出结果；channel、HTTP server、repository、registry 和
error object 都不得进入 EventHub command/data/result。Shutdown 先取消 observer，再
释放 local wait state。

TUI/CLI 只依赖 `internal/pkg/workorchport` 的稳定 contract；embedded backend 通过
typed EventHub command/result，remote backend 通过 client，不直接调用 owner `biz`、
`service` 或 repository。
