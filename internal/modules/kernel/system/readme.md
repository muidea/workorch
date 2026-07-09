# System Module

## 功能定位

`kernel/system` 是系统级 module identity、基础健康状态或 infrastructure contract surface 的窄模块。它用于保持系统 module 在 plugin inventory 中有明确身份。

## 拥有边界

- 只承担 system/infrastructure 级别的只读状态、健康或诊断入口。
- 不拥有 Workspace、Run、Result、Decision、Approval、Memory 等业务状态。
- 不处理 runtime completion、governance 或 evidence 判定。

## 主要交互

- 被 `application/workorchd` 或健康检查面用于 system/status 类信息。
- 与 `workorch/runtime` 共享基础设施生命周期语义。
- 可作为未来 infrastructure command/query 的稳定落点。

## 组件交互合同

- Input contract: 只接受 system/infrastructure 范围内的 command/query。
- Output contract: 输出必须是 read-only health/status/diagnostic 数据。
- Application contract: application 可读取 system status，但不能通过 system module 修改 business owner state。
- Initiator contract: system module 不替代 `workorch/runtime` 的 repository provider 或 runtime policy。

## 已知缺口

- 当前盘点未发现 System module 职责缺失；若未来新增命令，需要先声明 infrastructure-only 合同。

## 边界约束

- 不得承载业务完成判断。
- 不得变成跨 owner service registry。
- 若新增命令，必须保持 read-only 或 infrastructure-only 边界。
