# Software-Development Profile

Status: normative

Type: profile-capability-index

Last Verified: 2026-09-04

| Topic | Contract |
| --- | --- |
| Code-agent behavior and operation constraints | [Code Agent](code-agent.md) |
| Goal/DoD coverage and evidence policy | [Goal Coverage And DoD](goal-coverage-and-dod.md) |
| Optional language-server evidence bridge | [LSP Bridge](lsp-bridge-integration.md) |
| Profile-specific Runtime assertions | [Verification](verification.md) |

These documents specialize Template/Profile/Soul/capability behavior without adding a Runtime state machine or hard-coded
software-development branch to Orchestrator.

The built-in Profile keeps the stable `software-dev` ID but is the current **Go-native validation** specialization. It also
seals the File-to-Shell prospective-satisfaction contract for one-shot DoD validation preparation. Other framework-native or
MCP-only proof closures require a distinct explicit typed Profile; Application and Orchestrator never infer one from workspace
contents.
File owns the hypothetical workspace-manifest delta, Shell owns managed-validation applicability, and Orchestrator only joins
their typed facts inside SM-2; none of the three may transition a sibling owner's state.

Its validation-failure convergence is an exact route table, not terminal `one_of`. The built-in Profile exposes only three
Shell-owned, production-reachable Go tuples—module parse, source compile and typed test failure—and binds each to an existing File
target confirmed by File before `file/edit`. Generic `shell_exit_nonzero` and dependency/toolchain/environment tuples have no
default repair route. Additional remediation requires an explicit project-specific owner capability and Profile extension.
