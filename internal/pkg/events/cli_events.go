package events

import (
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ── CLI Event Topics ──

const (
	// Command topics — CLI publishes these to request business operations.
	TopicCLICommand = "/cli/command"

	// Result topic — runtime publishes command results back to CLI.
	TopicCLIResult = "/cli/result"

	// Progress topics — runtime publishes state changes.
	TopicCLIProgress = "/cli/progress"
)

// ── Command Envelope ──

// CLICommandPayload carries a typed CLI command through the EventHub.
type CLICommandPayload struct {
	CommandID     string         `json:"commandId"`
	CorrelationID string         `json:"correlationId"`
	Actor         string         `json:"actor"`
	IssuedAt      time.Time      `json:"issuedAt"`
	WorkspaceID   common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID         common.RunID       `json:"runId,omitempty"`
	SubjectType   string         `json:"subjectType"`
	SubjectID     string         `json:"subjectId,omitempty"`
	Command       string         `json:"command"`       // e.g. "workspace/create", "run/activate"
	Payload       map[string]any `json:"payload"`
}

// ── Result Envelope ──

// CLIResultPayload carries a command result from the runtime back to the CLI.
type CLIResultPayload struct {
	CorrelationID string         `json:"correlationId"`
	CommandID     string         `json:"commandId"`
	OK            bool           `json:"ok"`
	Error         string         `json:"error,omitempty"`
	Resource      map[string]any `json:"resource,omitempty"`  // single result object
	Resources     []map[string]any `json:"resources,omitempty"` // list result
	Snapshot      map[string]any `json:"snapshot,omitempty"`
	OccurredAt    time.Time      `json:"occurredAt"`
	WorkspaceID   common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID         common.RunID       `json:"runId,omitempty"`
}

// ── Progress Payload ──

// CLIProgressPayload carries a runtime state change to TUI/agent subscribers.
type CLIProgressPayload struct {
	WorkspaceID common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID       common.RunID       `json:"runId,omitempty"`
	StepID      common.StepID      `json:"stepId,omitempty"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId,omitempty"`
	State       string         `json:"state,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

// ── Command Constants ──

const (
	CommandWorkspaceCreate   = "workspace/create"
	CommandWorkspaceDelete   = "workspace/delete"
	CommandGoalDefine        = "goal/define"
	CommandDoDDefine         = "dod/define"
	CommandRunActivate       = "run/activate"
	CommandRunResume         = "run/resume"
	CommandRunCancel         = "run/cancel"
	CommandDecisionResolve   = "decision/resolve"
	CommandDecisionBatch     = "decision/batch-resolve"
	CommandApprovalApprove   = "approval/approve"
	CommandApprovalReject    = "approval/reject"
	CommandApprovalBatch     = "approval/batch-resolve"
	CommandSoulCreate        = "soul/create"
	CommandSoulDelete        = "soul/delete"
	CommandSkillCreate       = "skill/create"
	CommandMCPServerCreate   = "mcp-server/create"
	CommandMemoryCreate      = "memory/create"
	CommandConfigurationGet  = "configuration/get"
	CommandConfigurationLLM  = "configuration/llm-update"
)
