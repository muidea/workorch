# WorkOrch Agent Instructions

Before changing WorkOrch functionality, read `docs/workorch-runtime-stage-contract-neutrality-design.md`.

That document is the authoritative design contract for WorkOrch runtime evolution. Changes that affect runtime behavior, orchestration flow, LLM execution, memory / recall, soul / skill / tool / MCP capability selection, repair / replan, verification / acceptance, or profile mapping must be designed, implemented, and verified against it.

Core runtime changes must remain functionally neutral. Do not hard-code software development, code patching, test framework, language stack, or business-domain behavior into the core runtime. Put feature-specific behavior in profile / capability layers.

If older docs or local closure plans conflict with the stage contract and neutrality design, treat `docs/workorch-runtime-stage-contract-neutrality-design.md` as the source of truth and update the older docs or profile-specific docs instead of weakening the core contract.
