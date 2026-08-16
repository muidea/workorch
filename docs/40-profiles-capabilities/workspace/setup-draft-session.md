# Workspace Setup Draft Session

Status: active

Type: profile-capability

Last Verified: 2026-08-16

## Purpose

Setup draft session is an application/UI aid for turning a multi-turn conversation into a proposed Workspace Goal and DoD.
It is not a Workspace owner aggregate, Runtime activation, Run or recovery state machine.

## Current Surface

| Method | Path | Behavior |
| --- | --- | --- |
| `POST` | `/api/v1/workorch/workspace-setup-draft-sessions` | create an in-process session; returns `201` |
| `POST` | `/api/v1/workorch/workspace-setup-draft-sessions/:id/messages` | append one user message and generate a revision |
| `POST` | `/api/v1/workorch/workspace-setup-draft-sessions/:id/regenerate` | regenerate using current messages/draft; body may be empty |

The server keeps sessions in a mutex-protected in-process map. They are lost at process restart and must never be treated as
durable Workspace, Goal or DoD state.

## Inputs And Outputs

Session creation accepts:

- optional `templateId`;
- `title` and/or `description` (at least one must be present);
- optional `deliverableDirectory`;
- optional `dialogLanguage`.

A message request accepts `content`, optional updated workspace/template fields, optional current `draft`, and optional
`dialogLanguage`. The normalized draft contains only:

- `goalText`;
- `doDItems`;
- `assumptions`;
- `clarifyingQuestions`;
- `warnings`;
- `confidence`, `needsClarification` and `generationSummary`;
- provenance flags for template context and previous-draft use.

Template and Soul content may be included as bounded generation context. The LLM completion is parsed as a strict draft
object and merged with the revision baseline; it does not write owner repositories.

## Confirmation Boundary

Applying a draft is a separate product action:

1. create Workspace using title/directory/template;
2. create or update formal Goal through Workspace owner;
3. create or update formal DoD through Workspace owner;
4. configure Workspace Soul bindings/LLM selection when needed;
5. submit `execution-activations` with `intent=resume|restart`.

No draft-session endpoint performs those steps automatically. The UI must show which values are still proposals and require
explicit confirmation before formal writes.

## Runtime Boundary

Draft sessions never contain or generate:

- `RuntimeBootstrap`, Runtime focus, target, recipe, check or evidence binding;
- provider request history, candidate attempts or repair state;
- selected Memory/capability evidence;
- `executionContext` / `ExecutionContextPatch`;
- Run ID, target Run state or replacement successor identity.

After confirmation, workorchd builds `RuntimeBootstrap.Activation` from canonical owner state. That typed activation may
contain frozen Soul content, Skill instructions, MCP capability metadata, Memories, provider selection and execution policy;
none of these are draft-session state.

## UI Rules

- manual and assisted setup can share the same form values without silently clearing user input;
- show Goal/DoD draft, assumptions, questions and warnings before “apply to form”;
- do not imply that an LLM draft is already a formal definition or accepted evidence;
- do not show runtime progress, recall traces, provider retries or raw authority data in the setup step;
- failure may fall back to manual editing, but must not manufacture a successful draft.

## Verification

- server route and multi-turn merge tests live in `internal/modules/application/workorchd/service/server/http_test.go`;
- strict parse/normalization/generator tests live in
  `internal/modules/application/workorchd/biz/appservice/setupdraft`;
- deployed template/Soul/setup behavior is covered by `make verify-workspace-template-soul`.

## Related Documents

- [Service API](../../50-product-interface/service-api.md)
- [Built-in Templates And Souls](../builtin-templates-and-souls.md)
- [Soul Routing](soul-routing.md)
- [TUI Requirements](../../50-product-interface/tui/requirements.md)
