# WorkOrch Codex Request Context Comparison

## Purpose

This document records the context-management lessons from comparing WorkOrch runtime execution with Codex request/response logs captured under:

```text
/home/rangh/aispace/ai-proxy/logs/codex
```

The goal is to guide WorkOrch context, evidence, and repair-input improvements without copying Codex private formats or embedding software-development-specific logic in the WorkOrch core framework.

This document complements:

- `docs/workorch-runtime-stage-contract-neutrality-design.md`
- `docs/codex-like-llm-prompt-context-design.md`
- `docs/model-aware-llm-context-budget-design.md`
- `docs/workspace-data-layout-plan.md`

## Observed Codex Request Pattern

The sampled Codex logs record each LLM request as one numbered round directory:

```text
logs/codex/000014/request.json
logs/codex/000014/request.meta.json
logs/codex/000014/upstream_request.json
logs/codex/000014/upstream_response.json
logs/codex/000014/response.sse
logs/codex/000014/response.json
logs/codex/000014/metadata.json
```

Across sampled rounds `000001`, `000006`, `000010`, `000014`, and `000018`:

| Round | Input Items | Tools | Instruction Bytes | Input Payload Bytes | Reasoning Effort |
| --- | ---: | ---: | ---: | ---: | --- |
| `000001` | 386 | 12 | 21335 | 418202 | high |
| `000006` | 406 | 12 | 21335 | 438912 | high |
| `000010` | 416 | 12 | 21335 | 442386 | high |
| `000014` | 426 | 12 | 21335 | 445674 | high |
| `000018` | 431 | 12 | 21335 | 446812 | high |

Round request file sizes grew slowly, from about `721KB` to `771KB`, while the conversation progressed through implementation, testing, failures, and fixes.

The context is not a single raw state snapshot. It is a structured message band containing:

- stable developer instructions;
- user goals and environment context;
- assistant progress messages;
- tool calls with `call_id`;
- tool call outputs with bounded summaries, exit status, wall time, and token count;
- patch calls as concrete diffs;
- compact opaque reasoning items;
- streaming response persisted separately as `response.sse`.

For sampled round `000014`, the input distribution was:

| Type | Count | Approx Content Bytes |
| --- | ---: | ---: |
| `developer` | 1 | 19225 |
| `user` | 14 | 4264 |
| `assistant` | 46 | 13039 |
| `function_call` | 94 | 15195 |
| `function_call_output` | 94 | 228065 |
| `custom_tool_call` | 49 | 159401 |
| `custom_tool_call_output` | 49 | 6485 |
| `reasoning` | 79 | 0 visible content |

Important pattern: Codex keeps process continuity, but tool results and patch operations remain structured and bounded. Raw streams are archived outside the main request body.

## WorkOrch Current Deviation

Recent WorkOrch validation on `ws-e2cae665a588d9f1` exposed these issues:

1. Runtime state transitions can continue, but repair convergence is weak.
   The run can move from `running` to `superseded` and create a replan run, which means the state machine is not fully stuck. However, repeated invalid repair outputs still cause excessive loops.

2. Context compression and history injection are mixed together.
   Some repair requests show `truncated=true`, while large historical governance and superseded-run context still enters the prompt. This means key patch evidence can be weakened while low-value process history remains.

3. Repair input is not consistently centered on the current failure target.
   Some repair attempts include `failureTargetScope` but no current `patchRepairContext`; other attempts include target snapshots but still allow old rejected output and historical repair context to dominate.

4. Memory recall includes too much process trace.
   Low-value superseded-run and tool-feedback records can appear in the prompt, reducing signal density for actual implementation decisions.

5. Large runtime JSON files amplify CPU and memory pressure.
   The observed run generated large files such as `event_record.json`, `step.json`, and `tool_call.json`. The current file repository rewrites full JSON maps, which is unsuitable for long-running autonomous workflows.

## Design Direction

WorkOrch should not simply increase context size. It should adopt a Codex-inspired structure:

```text
stable contracts
+ current goal and DoD
+ current runtime stage
+ current failure packet
+ bounded recent execution evidence
+ referenced historical evidence
+ raw artifacts by ref
```

The key is to preserve continuity without turning every LLM call into a full run dump.

## Absorbed Neutral Lessons

The reusable lessons from the Codex logs have been absorbed into the canonical runtime contract in:

```text
docs/workorch-runtime-stage-contract-neutrality-design.md
```

The absorbed rules are intentionally neutral:

- build each LLM request as layered context, not one raw runtime snapshot;
- put stable runtime contract, current Goal / DoD, stage bundle, current failure and required output before historical context;
- include an ordered `processEvidenceTimeline` so the LLM can see existing outputs, recent attempts, validation results, current failures, current gaps and the next required action;
- preserve command/tool/validation failure utility with bounded summaries, key excerpts and full payload refs;
- keep raw streams, full provider payloads and large artifacts outside the prompt by default, addressable through refs;
- keep profile-specific behavior such as software-development workflow discipline in workspaceTemplate / Soul / skill, not in core runtime.

The comparison document remains an implementation note and evidence source. It must not be treated as a second runtime contract. If a future Codex lesson should change WorkOrch behavior, it must first be translated into neutral stage / artifact / evidence / result / context-gate terms and then merged into the canonical design contract.

## Required Improvements

### 1. Context Projection By Stage

WorkOrch must build different prompt projections for different runtime stages.

Suggested projections:

| Stage | Primary Context | Historical Context |
| --- | --- | --- |
| `planning` | Goal, DoD, workspace profile, available capabilities | prior accepted plans and constraints |
| `execution` | current executable graph, target files, capability schema | recent successful/failed steps |
| `repair` | current failure packet and current target snapshots | only relevant invalid strategies and recent failures |
| `verification` | DoD, evidence candidates, validation commands/results | accepted implementation evidence |
| `acceptance` | final artifacts, DoD assessment, evidence refs | summarized lineage only |

No stage should receive an undifferentiated full workspace/run snapshot.

### 2. Deterministic Repair Packet

Repair input must be generated as a minimal deterministic packet centered on the current failed target.

Required fields:

```json
{
  "failedStepId": "...",
  "failedStepKind": "...",
  "failureClass": "...",
  "failureReason": "...",
  "failureTargetScope": ["..."],
  "currentTargetSnapshots": [],
  "lastFailedArtifact": {},
  "lastVerificationFailure": {},
  "requiredOutput": {
    "kind": "patch/apply",
    "patchType": "unified_diff"
  },
  "forbiddenActions": [],
  "verificationCommand": "...",
  "evidenceRefs": []
}
```

Rules:

- If exact patch context is required and no complete target snapshot is available, require a `file/read` step before patch generation.
- Do not allow redacted, truncated, or omitted content to be copied into a patch.
- Do not let superseded-run history override the current target snapshot.
- Preserve failed artifact metadata, but mark failed patch/diff/oldText as stale unless proven current.

### 3. Layered Context Budget

Budgeting must be layer-aware rather than uniformly trimming the entire prompt.

Recommended layers:

| Layer | Content | Compression Rule |
| --- | --- | --- |
| P0 | Runtime contract, output schema, neutrality rules | no semantic compression |
| P1 | current Goal and DoD originals | no compression |
| P2 | current stage request and failure packet | no loss of required fields |
| P3 | current target snapshots | complete for small files, window/ref for large files |
| P4 | latest validation output | bounded head/tail plus full ref |
| P5 | recent useful execution evidence | bounded summary |
| P6 | historical memory and superseded runs | top-k summarized facts only |
| P7 | raw streams and full tool output | ref only |

If P0-P2 exceed provider budget, WorkOrch should block or request a larger model/smaller task instead of silently trimming required contracts.

### 4. Tool Output Summary Contract

Codex tool outputs preserve execution utility without flooding the prompt. WorkOrch should standardize tool evidence summaries:

```json
{
  "toolCallId": "...",
  "stepId": "...",
  "kind": "shell/command",
  "state": "failed",
  "exitCode": 1,
  "durationMs": 1234,
  "originalTokenCount": 394,
  "stdoutPreview": "...",
  "stderrPreview": "...",
  "summary": "...",
  "fullOutputRef": "artifact://..."
}
```

Full stdout/stderr should not be injected by default. The LLM can request or be provided targeted excerpts when needed.

### 5. Patch Evidence Contract

Patch and file-edit evidence should follow these rules:

- Preserve concrete diffs as artifacts.
- Keep target snapshots separate from failed patch text.
- For exact repair, prefer current `targetSnapshotRef` plus small visible context.
- If the file is too large, provide a focused window with line anchors and content hash.
- `file_write` for existing targets is only valid when a complete, safe snapshot is available and replacement is explicitly required.
- `unified_diff` must use real hunk headers and current context.

### 6. Memory Recall Signal Filter

Memory recall must prioritize decision-useful information:

- accepted input constraints;
- Goal and DoD derived requirements;
- verified root causes;
- valid repair strategies;
- invalid strategies to avoid;
- tool/capability lessons that directly affect current target or stage.

Memory recall should avoid injecting:

- generic process progress;
- superseded run traces;
- repeated low-value tool feedback;
- old runtime-policy values that conflict with current configuration;
- verbose historical governance payloads.

### 7. Runtime State Capsule

WorkOrch should maintain a compact state capsule for each active run:

```json
{
  "currentObjective": "...",
  "stage": "repair",
  "knownFacts": [],
  "acceptedDecisions": [],
  "invalidStrategies": [],
  "currentFailureTargets": [],
  "nextRequiredAction": "...",
  "evidenceRefs": []
}
```

The capsule is updated after each meaningful step and becomes the primary continuity mechanism. Full history remains available by reference.

### 8. Raw Stream And Large Payload References

Codex stores streaming responses separately. WorkOrch should do the same:

- raw LLM stream -> `streamRef`;
- raw provider request/response -> `llmCallRef`;
- large step input/output -> `payloadRef`;
- full command output -> `outputRef`;
- full file snapshot -> `snapshotRef`.

Read models should default to summary views and load raw content only on demand.

Governance and replan continuity should follow the same pattern. Historical
tickets/resolutions are useful only as accepted decisions and failure lineage;
they should carry compact failure summaries and refs, not old step inputs,
target content, raw LLM previews, stream events, or provider/model bindings.
Runtime ToolCall records should also point at step input/output refs instead of
copying the complete step payload into `arguments.json` and `output.json`.

### 9. Storage Layout Alignment

To support the context model above, runtime persistence should stop rewriting large JSON maps.

Recommended layout:

```text
runs/<run-id>/index.json
runs/<run-id>/steps/<step-id>/metadata.json
runs/<run-id>/steps/<step-id>/input.json
runs/<run-id>/steps/<step-id>/output.json
runs/<run-id>/tool-calls/<tool-call-id>/metadata.json
runs/<run-id>/tool-calls/<tool-call-id>/output.json
runs/<run-id>/events/events.jsonl
runs/<run-id>/llm-calls/<step-id>/request.json
runs/<run-id>/llm-calls/<step-id>/response.sse
runs/<run-id>/artifacts/<artifact-id>
```

Required behavior:

- append-only events;
- item-level updates for steps/tool calls;
- atomic write via temp file and rename;
- projection indexes for workspace/run list views;
- no default all-events/all-payload workspace snapshot.

## Acceptance Criteria

The improvement is considered complete when:

1. Repair-stage LLM input contains a deterministic repair packet.
2. Current Goal and DoD are preserved without semantic loss.
3. Current failure target snapshots are present, or the LLM is explicitly instructed to read the target before patching.
4. Superseded runs do not enter repair prompt as verbose history.
5. Memory recall contains decision-useful facts rather than process traces.
6. Raw stream/tool output/full payloads are referenced, not injected by default.
7. Workspace and run detail APIs can render large runs without loading all raw event, step, and tool payloads.
8. File persistence no longer rewrites large run-level JSON maps for single event/step/tool updates.
9. Context budget reports show which layers were included, summarized, referenced, or blocked.

## Implementation Order

1. Add repair packet builder and tests.
2. Add stage-specific context projection.
3. Filter memory recall by decision value and current target relevance.
4. Split raw stream/tool output/payload references from read-model summaries.
5. Add event pagination and forbid unlimited workspace event loads.
6. Migrate file repository from large map rewrites to append/item-level storage.
7. Update console views to request paginated details and lazy-load raw payload refs.

## Non-Goals

- Do not copy Codex internal request schema into WorkOrch.
- Do not make WorkOrch core software-development-specific.
- Do not solve weak LLM output by only increasing context size.
- Do not use raw stream deltas as business state.
- Do not remove full evidence retention; retain full evidence by reference.

## Summary

Codex demonstrates that long software-development workflows can preserve useful continuity without injecting an unbounded raw state dump. WorkOrch should adopt the same architectural principle:

```text
small stable contracts
+ current deterministic task packet
+ bounded structured evidence
+ raw data by reference
+ stage-specific projection
```

This is the required direction for reducing CPU/memory pressure, improving LLM repair convergence, and keeping WorkOrch core runtime functionally neutral.
