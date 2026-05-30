# WorkOrch TUI Display Closure Tracking

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录默认 TUI agent 体验在 display/result-first 收口范围内的闭环项和验收记录。

## Confirmed Product Decisions

- 默认 TUI 必须是 result-first
- 详细 run/step/decision/approval/provider stream 事件保留在 backend records
- 默认 TUI 不提供 detailed event-stream/debug-event view
- 运行中只展示 compact progress/status summary
- 输入区固定在底部 help bar 上方
- 启动 Workspace 选择优先当前目录绑定，其次 last active
- 本地 TUI 继续使用共享 backend 和 EventHub callback，不引入 HTTP 或 polling

## Closure Items

- `TUI-DISPLAY-01` Result-first main view
- `TUI-DISPLAY-02` Compact progress summary
- `TUI-DISPLAY-03` Bottom input area
- `TUI-DISPLAY-04` Clear Workspace identity
- `TUI-DISPLAY-05` Simplified Workspace monitor
- `TUI-DISPLAY-06` Workspace startup reuse strategy
- `TUI-DISPLAY-07` Error display deduplication
- `TUI-DISPLAY-08` Decision/Approval current-state handling
- `TUI-DISPLAY-09` Terminal Result and Artifact context
- `TUI-DISPLAY-10` Layout stability
- `TUI-DISPLAY-11` Documentation sync
- `TUI-DISPLAY-12` Test and verification coverage

## Acceptance Record

当前状态：verified

已确认：

- 默认主视图不再显示 raw event history 噪音
- 进度更新以 compact state 呈现
- 底部输入区、Workspace 监视、错误去重和 Artifact context 已作为当前 closure scope 收口
- 文档与当前实现均使用 result-first 默认视图描述

## Review Boundary Note

当前 closure 里的“display/review”专指：

- accepted result / artifact 的主阅读路径
- compact progress / blocker / failure 的当前态阅读
- workspace monitor 的 bounded summary

不包括：

- 默认全量 event stream review
- 独立 debug 面板
- 历史 step/tool/provider 逐条回放

若后续需要更深诊断，应继续通过 backend records、Web Console 或专门 verifier，
而不是把默认 TUI display closure 扩展成新的调试产品面。

## Verification Commands

```bash
GOCACHE=../../../../../../../tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui --count 1
GOCACHE=../../../../../../../tmp/workorch-go-build make verify-tui-agent
GOCACHE=../../../../../../../tmp/workorch-go-build make cli-test
```

## Source Documents

- `../../50-product-interface/tui/display-refresh-consistency-design.md`
- `../../50-product-interface/tui/requirements.md`
