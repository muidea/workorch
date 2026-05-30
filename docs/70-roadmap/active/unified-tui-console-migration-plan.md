# workorch Unified TUI / Console Migration Plan

Status: active

Type: roadmap

Last Updated: 2026-05-30

## Stable Definition Entry

以下稳定定义已迁入正式结构，不再由本 roadmap 文档重复展开：

- `../../30-module-architecture/application-workorchd.md`
- `../../30-module-architecture/repository-structure.md`
- `../../50-product-interface/tui/requirements.md`
- `../../50-product-interface/tui/interaction-contract.md`
- `../../50-product-interface/tui/display-refresh-consistency-design.md`
- `../../60-verification/tui/tui-agent-acceptance.md`
- `../../60-verification/tui/display-closure-tracking.md`

## 1. 背景

`workorch` 需要从现有的 CLI / TUI 原型，迁移为一个统一的本地优先 Agent 入口：

- 默认以 **前台 TUI 交互模式** 启动。
- 同一进程内复用原 `workorchd` 的 runtime、业务能力、EventHub、仓储和控制台能力。
- `daemon/server` 仅作为显式启动模式，不再作为默认路径。
- TUI 只做交互与展示，不通过 HTTP client 访问本地后端。
- 状态变化通过 EventHub 异步回调驱动，不通过定时轮询刷新。

这份文档用于记录迁移路线、实现拆分、验收标准和后续跟踪项。

当前 roadmap 只保留尚未完全收口的迁移跟踪项；默认模式、shared runtime、
EventHub callback、本地/远程边界、TUI 产品交互和 TUI 验收主体设计以
stable definition entry 中的正式文档为准。

## 2. 目标形态

### 2.1 默认模式

默认执行 `workorch` 时：

- 启动本地 embedded runtime。
- 启动/托管原 `workorchd` 的 console / API / HTTP 服务能力。
- 启动 TUI 作为前台主交互界面。
- TUI 和 console 共享同一套 runtime、session、EventHub 和 backend。
- 当前工作目录优先绑定为默认 Workspace；未绑定时复用上一次 active Workspace。

### 2.2 显式服务模式

通过额外参数进入 daemon/server 模式：

- 仅启动 `workorchd` 后端服务。
- 不启动前台 TUI。
- 供远端客户端或外部系统接入。

### 2.3 远端访问模式

远端访问仍然保留为显式能力：

- `workorch --remote <addr>` 作为远端客户端入口。
- 本地 TUI / CLI 不直接依赖 HTTP client 进行本地交互。
- 远端 transport 如果继续使用 HTTP，必须被隔离在 backend 实现内部。

## 3. 当前稳定基线

以下基线已稳定，不再作为 roadmap 中的待定设计：

- 默认 `workorch` 是本地 agent-first TUI 入口
- 本地模式复用 shared backend / EventHub，不通过本机 HTTP client
- `--server` / `serve` / `--daemon` 作为显式后端模式保留
- `--remote <addr>` 作为显式远程入口保留
- TUI / CLI / HTTP console 共享同一 backend contract

更细架构拆分与产品边界详见 stable definition entry。

## 4. 剩余迁移关注点

### 4.1 入口收敛

当前只保留仍需持续核对的内容：

- 模式参数和默认行为后续演进不能再次分叉
- 默认前台模式、server-only 模式和 remote 模式的行为守卫需继续保留

### 4.2 模式划分

当前只跟踪：

- 各模式后续修改时不能破坏本地/远程边界
- transport 差异不能渗透到业务 contract

### 4.3 Shared Runtime 组件

当前只跟踪：

- shared runtime 生命周期后续修改不能出现重复初始化或子进程回退
- workspace resolver / backend contract 不得在 TUI / CLI / HTTP 之间重新分叉

### 4.4 Workspace 策略

当前只跟踪：

- 当前目录优先绑定策略不能被后续便利性改动破坏
- workspace switch / monitor 继续复用统一 read model 和 backend contract

## 5. 迁移步骤

### Phase 1: 统一模式与入口 contract

目标：

- 固化默认前台模式和 server-only 模式。
- 明确 `--remote` 仅作为远端客户端入口。
- 把现有参数解析收敛到统一入口层。

产出：

- 启动模式文档
- 统一 flag 约定
- 入口行为测试

### Phase 2: 提取共享 runtime host

目标：

- 将 runtime/session/EventHub/backend 的组装逻辑从 `main` 中抽出。
- 让 `workorch` 和 `workorchd` 共用同一套启动路径。

产出：

- `runtime host` 包
- 统一 shutdown 路径
- 共享 workspace resolver

### Phase 3: 托管原 workorchd console

目标：

- 让前台 TUI 在同进程内托管原 `workorchd` 的 console / API / HTTP 能力。
- 保持 HTTP console 可用于排障和运维。

产出：

- console/server 绑定到 shared runtime
- TUI 启动后可同时访问原控制台能力

### Phase 4: 完成 TUI 与 backend 的事件化接线

目标：

- TUI 只消费 backend contract。
- 进度、决策、审批、结果通过 EventHub 异步回调更新。
- 禁止 TUI 通过 HTTP client 访问本地后端。

产出：

- 事件订阅与回调通道
- 无轮询刷新
- 统一状态驱动

### Phase 5: 完成 CLI / TUI 的统一操作面

目标：

- 命令行子命令可完成 `workorchd` 暴露的业务交互。
- TUI 内提供常用操作的快速入口。

产出：

- workspace / run / decision / approval / result / artifact 操作一致化
- 交互行为测试

### Phase 6: 验收和回归

目标：

- 完成设计文档验收。
- 固化 E2E 测试。
- 确认默认模式、server 模式和 remote 模式无功能分叉。

产出：

- 迁移验收文档
- TUI E2E
- CLI smoke / architecture guard

## 6. 验收标准

必须满足以下条件后才能视为收口完成：

- 默认 `workorch` 启动的是前台 TUI 交互模式。
- `workorchd` console 能在同进程共享 runtime 下工作。
- `--server` / `serve` / `--daemon` 可以只启动后端。
- 本地 TUI 不依赖 HTTP client，不依赖轮询。
- TUI、CLI、HTTP console 使用同一 backend contract。
- 当前目录优先绑定 workspace，且支持切换与状态观察。
- 不再产生 `workorchd` 子进程。
- 相关 E2E / smoke / architecture guard 通过。

## 7. 主要风险

- 入口收敛时，必须避免 runtime/session 重复初始化。
- HTTP console 和 TUI 共享事件通道时，要避免订阅泄漏和重复消费。
- 默认前台模式若在无 TTY 环境运行，需要明确降级或报错策略。
- workspace 切换必须与“当前目录优先”策略一致，不能回退成新建优先。
- 如果后续保留 remote 模式，必须继续保证 transport 与业务 contract 解耦。

## 8. 建议的落点目录

相关稳定落点当前已集中在以下位置：

- `internal/modules/application/workorch/app`
- `internal/modules/application/workorch/runtime`
- `internal/modules/application/workorch/service/cli`
- `internal/modules/application/workorch/service/tui`
- `internal/modules/application/workorchd/service/server`
- `application/workorch/main.go`

## 9. 跟踪建议

当前应主要与以下正式文档联动核对，而不是重复维护同一正文：

- `../../50-product-interface/tui/requirements.md`
- `../../60-verification/tui/tui-agent-acceptance.md`
- `../../60-verification/tui/display-closure-tracking.md`

## 10. 已确认的实现约束

以下约束已确认并作为后续实现前提：

- 退出语义固定为输入 `/exit` 退出整个进程。
- 前台 TUI 的进程级退出验收以 `/exit` 为准；其他快捷键只作为交互细节，不替代 `/exit`。
- HTTP 监听与当前配置规则保持一致，不额外改变默认暴露策略。
- TUI 只负责命令输入与数据展示，不改变其他后端行为、生命周期或业务规则。
- 其他未特别说明的内容，均沿用本方案中的统一 runtime / shared backend / EventHub 设计。

## 11. 实现记录

2026-05-03 收口项：

- 新增共享启动层 `internal/modules/application/workorch/app`。
- `application/workorch` 默认通过共享启动层进入前台 TUI 模式，并在同进程启动后端 console / API / HTTP 能力。
- `application/workorch` 顶层入口已清理，server-only 改为 `workorch --server`。
- 显式 server 模式支持 `--server` / `serve` / `--daemon`。
- TUI 支持输入 `/exit` 退出整个进程。
- 非 TTY 默认启动会给出明确提示，要求使用 `--server` 进入后端服务模式。
- `scripts/verify-tui-agent.sh` 已更新为验证新的非 TTY contract。
- 前台 TUI 模式会把标准 `log` 和 `log/slog` 输出写入
  `<config-dir>/logs/workorch.log`，避免后端启动日志污染 TUI console；
  `--server` 模式保持原有 console 日志输出。

验证命令：

```bash
GOCACHE=../../../../../../../tmp/workorch-go-build make cli-test
GOCACHE=../../../../../../../tmp/workorch-go-build go test ./application/workorch --count=1
GOCACHE=../../../../../../../tmp/workorch-go-build go build -o ./bin/workorch ./application/workorch
GOCACHE=../../../../../../../tmp/workorch-go-build make verify-tui-agent
```

补充手工验证：

- 使用 `./bin/workorch --server --config-dir <tmp>` 启动后，`/healthz`
  可访问。
- 使用 `./bin/workorch --config-dir <tmp> </dev/null` 启动时，console 只输出
  非 TTY 提示，不出现 `slog` route/server 日志。
