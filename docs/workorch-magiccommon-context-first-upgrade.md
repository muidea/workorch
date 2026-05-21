# WorkOrch magicCommon 升级说明（2026-05）

## 目标

本次升级将 WorkOrch 运行时切换到本地 `magicCommon` 的 `context-first` 生命周期实现，并同步收口依赖、vendor 和调用方式。

## 升级背景

`magicCommon` 已将以下核心生命周期改为显式接收 `context.Context`：

- `application.Startup(ctx, service)`
- `application.Run(ctx)`
- `application.Shutdown(ctx)`
- `service.Startup(ctx, ...)`
- `service.Run(ctx)`
- `service.Shutdown(ctx)`
- `plugin / initiator / module` 的 `Setup / Run / Teardown`
- `event.Hub.Terminate(ctx)`
- `task.BackgroundRoutine.Shutdown(ctx)`

WorkOrch 原有实现里还残留：

- 入口使用旧 application API
- service teardown 内部私造 shutdown budget
- 部分 plugin / module 仍沿用无参 Run / Teardown
- `MagicHub` wrapper 仍使用无参 `Terminate()`
- vendor 里的 `magicCommon v1.5.8` 与本地开发版不一致

这些差异会导致：

- shutdown 预算被分裂
- 测试可以通过，但运行时仍可能卡住
- 本地改动不生效，最终编译和部署使用的仍是旧 API

## 当前收口范围

本轮 WorkOrch 需要完成三件事：

1. 依赖切换到本地 `magicCommon`
2. 生命周期调用全部改为 context-first
3. 更新 vendor 和验证脚本，确保默认构建也使用新语义

## 依赖切换

### go.mod

WorkOrch 使用本地开发版 `magicCommon`：

```go
replace github.com/muidea/magicCommon => /home/rangh/codespace/magicCommon
```

说明：

- 这是一轮开发期切换，不是长期发布约定
- 目的只是让 WorkOrch 当前工作区直接消费最新框架语义
- 代码验证完成后，可根据发布策略再决定是否保留本地 replace

### vendor

由于仓库保留 vendor，必须同步更新：

```bash
go mod vendor
```

否则默认 `go test` 会报：

- `inconsistent vendoring`

## 生命周期迁移

### application 入口

旧：

```go
application.Startup(service.DefaultService())
application.Run()
application.Shutdown()
```

新：

```go
application.Startup(ctx, service.DefaultService())
application.Run(ctx)
application.Shutdown(ctx)
```

要求：

- 不再用外层 goroutine 包裹 `application.Shutdown`
- shutdown budget 由调用方 ctx 决定
- 入口层只做 signal/context 组织，不做额外兜底

### WorkOrch service

`workorchd/service/server.Service` 已改为：

```go
Startup(ctx, serviceName, eventHub, backgroundRoutine)
Run(ctx)
Shutdown(ctx)
```

关键点：

- `Startup` 用传入 ctx 做验证
- `Shutdown` 直接使用传入 ctx 关闭 HTTP server
- 如果 `Run()` 从未调用，`Shutdown()` 不能再等待 `done` channel

### plugin / module / initiator

WorkOrch 所有注册到 `magicCommon` 的入口都必须改成：

```go
Setup(ctx, eventHub, backgroundRoutine)
Run(ctx)
Teardown(ctx)
```

包括：

- application/workorch plugin
- application/workorchd module
- kernel / blocks 各 plugin
- runtime initiator

### MagicHub wrapper

`internal/pkg/events/MagicHub` 已改为：

```go
Terminate(ctx context.Context)
```

要求：

- 不再桥接到无参 `Terminate()`
- WorkOrch runtime shutdown 时必须把同一个 ctx 传入 wrapper

## 迁移结果

这轮修复已经解决了一个真实阻塞点：

- `workorchd.Service.Shutdown()` 在 `Setup` 后未 `Run` 的场景会等待无人发送的 `done`，导致测试和关闭路径卡死

已改为：

- 只有 `Run()` 启动后才等待 `done`
- 未运行时直接走 HTTP shutdown，不再阻塞

## 验证方式

建议按以下顺序验证：

```bash
cd /home/rangh/aispace/workorch
go test ./internal/modules/application/workorch/app ./internal/modules/application/workorchd ./internal/modules/application/workorchd/service/server ./internal/pkg/events ./internal/initiators/workorchruntime -count=1
go test ./... -run '^$' -count=0
go test ./... -count=1
```

## 约束

- 不要再把 `context.Background()` 当作关闭协议本身
- 不要再用外层 timeout goroutine 伪装 shutdown 成功
- 不要再保留旧签名作为长期兼容层
- 不要在 core runtime 里引入业务特化语义

## 结论

WorkOrch 已进入 `magicCommon context-first lifecycle` 迁移收口阶段。后续任何 lifecycle、shutdown、terminate、teardown 相关修改，都必须先对齐本说明和 `magicCommon` 的升级文档。
