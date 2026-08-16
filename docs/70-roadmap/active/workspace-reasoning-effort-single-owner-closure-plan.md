# Workspace Reasoning Effort Deployed Verification

Status: active

Type: deployed-verification-plan

Last Verified: 2026-08-16

## Purpose

代码和 typed contracts 已完成收口；本计划只保留 Workspace model/reasoning selection 的部署态验证。历史设计、
代码切片和已完成测试见
[归档记录](../../90-archive/records/workspace-reasoning-effort-code-closure-record-2026-08-16.md)。

## Owner Contract

- Workspace owner 持久化下一次 fresh activation 使用的 `DefaultLLMSelection`。
- Run owner 在 `RuntimeBootstrap.Activation.ProviderSelection` 中封存本 Run selection。
- LLM owner 根据 active ai-proxy catalog 校验 provider/model/reasoning effort，并返回 capability/ceiling fact。
- Resume 只使用 Run 已封存 selection；Restart 从 Workspace 当前 selection 构造 fresh bootstrap。
- Application、TUI、Web 与 Orchestrator 均不得维护第二份 provider/model/reasoning authority。

## Deployed Matrix

1. Web 更新 workspace model/reasoning 后，下一次 Start/Restart 使用新 selection；当前 Run 不漂移。
2. TUI `/model show` 明确区分 next-run selection 与 current-run sealed selection。
3. TUI `/model reasoning <effort|default>` 只更新指定 Workspace，不污染第二个 Workspace。
4. Resume 不携带 selection patch；Workspace preference 改变后仍使用 current Run sealed selection。
5. Restart 创建 fresh activation 并使用最新 Workspace selection；普通 current-baseline predecessor 仍遵守
   Run-owned replacement intent 和 terminal delivery 顺序。
6. 保存 LLM configuration 后，运行中 LLM owner snapshot 不热切换；管理投影显示 `restartRequired`，进程重启后
   才使用新 active configuration。
7. invalid/unready provider、model 或 unsupported reasoning effort 在 provider submission 前 fail-closed，physical
   provider call 为 0。

## Exit

在同一已部署构建上完成 Web、TUI、Resume/Restart 和双 Workspace 隔离证据后关闭。单元测试、mock catalog 或只验证
一个端侧入口不能替代该 deployed matrix。
