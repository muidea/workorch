# TUI Workspace Runtime Refresh / History Closure Verification

Status: completed

Type: verification-record

Last Updated: 2026-07-15

Review State: completed

Related:

- `../90-archive/completed/tui-workspace-runtime-refresh-history-closure-plan-2026-07-15.md`
- `../50-product-interface/tui/display-refresh-consistency-design.md`
- live artifact: `artifacts/tui-runtime-refresh-live-20260715T132455Z/`

## 1. Purpose

记录第五次核对（FR01–FR07）收口后的可复核验证证据。第四轮 artifact/命令仅作历史参考，完成判定以本节与 plan §17.10 为准。

## 2. FR01–FR07 Results

| ID | 结果 | 证据 |
| --- | --- | --- |
| FR01 | completed | session Close 5s timeout + approval t.Cleanup；`go test ./internal/modules/application/workorchd/...` 连续两轮 PASS |
| FR02 | completed | comparator 另覆盖 RepairLoop 与 ProjectionInfo；相同 revision 的字段 mutation 回归测试 |
| FR03 | completed | store-cap Truncated=true + empty NextCursor；unknown cursor 空页终止；Truncated 进入 content equality |
| FR04 | completed | production `execution-activations` 驱动真实 Runtime completed（state revision 26）并执行强制 PTY；seed 仅用于 restart fixture |
| FR05 | completed | StreamGeneration 进入 RuntimeView/SSE/TUI；generation-scoped Last-Event-ID 跨 restart 不碰撞 |
| FR06 | completed | length-prefix map/slice facts；delimiter 负向测试 |
| FR07 | completed | NewStoreWithRevisionFloor/SeedRevisionFloor/highWaterMark；时钟回拨/floor 测试 |
| TC10 | completed | live script + versioned artifact |

## 3. Commands

```bash
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
make fmt-check
make vet
./scripts/verify-tui-runtime-refresh-history-live.sh
# or make verify-tui-runtime-refresh-history-live
```

结果：全部 PASS（2026-07-15）。

## 4. Live Artifact

`docs/60-verification/artifacts/tui-runtime-refresh-live-20260715T132455Z/`

| 检查 | 结果 |
| --- | --- |
| production Runtime | completed, RuntimeStateRevision=26 |
| RuntimeStateRevision seeded/advanced | 7 → 11（restart fixture） |
| RuntimeStateRevision post server restart | 11 |
| Workspace/Activity revision post restart | 高于 pre（epoch/high-water） |
| SSE generation pre → post reconnect | `6e5e6203…` → `e8c66569…`，generation-scoped IDs distinct |
| actual TUI/PTY | PASS；scroll、真实 HTTP projection update、unread 与 PgDn follow-tail 均有 summary |
| non-active B history readback | PASS |

详见 `SUMMARY.md` 与 `report.json`。

## 5. Implementation Notes

- HTTP RuntimeView 使用 store-owned activity（与 CLI typed path 一致）。
- Store：`StreamGeneration` + `revisionEpoch` + `highWaterMark`；generation 是跨进程 revision domain。
- SSE：generation-scoped `projection:<generation>:<workspace>:<sequence>`；remote/TUI 在 generation 切换时做 readback。
- Activity store-cap：Truncated=true，NextCursor 空；分页 unknown cursor 返回空页。
- Live seeder：`scripts/seed-runtime-run.go` 只用于 restart/readback fixture。
- PTY 为 mandatory；live run 通过 production activation 和 mock provider 产生 typed Runtime evidence。

## 6. Completion Gate

FR01–FR07 与 TC01–TC10 全部 completed；关联 plan Status=`completed`。
