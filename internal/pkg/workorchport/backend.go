// Package workorchport defines the shared interface consumed by CLI commands,
// TUI, and agent session logic. Multiple transport implementations satisfy the
// same interface so callers never branch on transport.
package workorchport

// Backend is the shared CLI backend consumed by Cobra commands, the TUI model,
// and the agent session flow. Every interaction with workorch business state
// goes through this interface.
type Backend interface {
	Workspace() WorkspaceBackend
	Goal() GoalBackend
	DoD() DoDBackend
	Run() RunBackend
	Decision() DecisionBackend
	Approval() ApprovalBackend
	Soul() SoulBackend
	Skill() SkillBackend
	MCP() MCPBackend
	Memory() MemoryBackend
	Template() TemplateBackend
	Config() ConfigBackend
	// Events is the generic/full event stream surface for specialized CLI/agent
	// tooling. TUI main-view refresh must use ProjectionEvents instead.
	Events() EventsBackend
	// ProjectionEvents is the typed projection-invalidation + subscription
	// health surface for TUI runtime refresh and history readback.
	ProjectionEvents() WorkspaceProjectionEventsBackend
}
