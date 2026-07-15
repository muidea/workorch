# TUI Runtime Refresh / History Live Verification

Stamp: `20260715T132455Z`

Base URL: `http://127.0.0.1:40093`

Artifact dir: `/home/rangh/aispace/workorch/docs/60-verification/artifacts/tui-runtime-refresh-live-20260715T132455Z`

## Workspaces

- A: `ws-8c31db3fcee13ffc`
- B: `ws-0af29aa3ef2ca8b0`

## SSE

- Last-Event-ID before restart: `projection:6e5e6203c98bee1620407e63fb169deb:ws-8c31db3fcee13ffc:1784121903426890079`
- Last-Event-ID after reconnect: `projection:e8c66569a9525fd80eed3ee34f11cc46:ws-8c31db3fcee13ffc:1784121906297584208`
- boot1 log: `sse/06-sse-boot1.log`
- reconnect log: `sse/16-sse-boot2-reconnect.log`

## Revisions

- A before: workspace=1784121903426890074 runtime=7 activity=1784121903426890074 items=1 summaries=['active']
- A after mutate: workspace=1784121904422382196 runtime=11 activity=1784121904422382196 items=1 summaries=['active']
- A pre-restart: workspace=1784121904422382196 runtime=11 activity=1784121904422382196 items=1 summaries=['active']
- A post-restart: workspace=1784121906297584201 runtime=11 activity=1784121906297584201 items=1 summaries=['active']
- A post-reconnect: workspace=1784121906297584207 runtime=0 activity=1784121906297584205 items=1 summaries=['workspace definitions changed; execution must be rerun']
- B before: workspace=1784121903426890076 runtime=3 activity=1784121903426890076 items=1 summaries=['active']
- B background mutate: workspace=1784121904422382198 runtime=0 activity=1784121904422382198 items=1 summaries=['workspace definitions changed; execution must be rerun']
- B post-restart: workspace=1784121906297584203 runtime=0 activity=1784121906297584203 items=1 summaries=['workspace definitions changed; execution must be rerun']

## Checks

- server restart revision acceptable (CR01): PASS
- SSE connect / live: PASS
- SSE interrupt + reconnect + readback: PASS
- runtime-view history readback A/B: PASS
- real Runtime V2 activation + TUI PTY: PASS (see `real-runtime-v2-tui/`)
- TUI automated restart/history gates: PASS (supplemental; see `tui-automated-gates.log`)

## Plan mapping

Covers plan §15.5 automated/local-deployed portion for TC10:

1. runtime-view progress/history readback
2. non-active workspace activity recoverable on switch/readback
3. server restart while client would keep AppliedRevision
4. SSE interruption / reconnect / Last-Event-ID
5. production Runtime V2 execution through execution-activation
6. actual Bubble Tea renderer via mandatory PTY

Interactive TUI viewport (PgUp unread banner) remains covered by package tests
`TestRuntimeViewFollowTailAndUserScrolledUnread` in the automated gate log.
