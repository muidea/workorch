# Software-Development Profile

Status: normative

Type: profile-capability-index

Last Verified: 2026-09-05

| Topic | Contract |
| --- | --- |
| Code-agent behavior and operation constraints | [Code Agent](code-agent.md) |
| Goal/DoD coverage and evidence policy | [Goal Coverage And DoD](goal-coverage-and-dod.md) |
| Optional language-server evidence bridge | [LSP Bridge](lsp-bridge-integration.md) |
| Profile-specific Runtime assertions | [Verification](verification.md) |

These documents specialize Template/Profile/Soul/capability behavior without adding a Runtime state machine or hard-coded
software-development branch to Orchestrator.

The built-in `software-dev` template keeps a language- and framework-neutral identity. It also seals the declarative
File-to-Shell prospective-satisfaction contract used by typed managed-validation adapters for one-shot DoD validation
preparation. The current Shell owner includes a Go-native adapter, while other framework-native or MCP-only proof closures use
their own explicit typed adapter contracts; adapter names and protocols are not part of the stable template presentation.
File owns the hypothetical workspace-manifest delta, Shell owns managed-validation applicability, and Orchestrator only joins
their typed facts inside SM-2; none of the three may transition a sibling owner's state.

Its validation-failure convergence is an exact route table, not terminal `one_of`. The built-in Profile exposes only three
Shell-owned, production-reachable Go tuples—module parse, source compile and typed test failure—and binds each to an existing File
target confirmed by File before `file/edit`. Generic `shell_exit_nonzero` and dependency/toolchain/environment tuples have no
default repair route. Additional remediation requires an explicit project-specific owner capability and Profile extension.
