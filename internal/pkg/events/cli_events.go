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
	CommandID     string             `json:"commandId"`
	CorrelationID string             `json:"correlationId"`
	Actor         string             `json:"actor"`
	IssuedAt      time.Time          `json:"issuedAt"`
	WorkspaceID   common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID         common.RunID       `json:"runId,omitempty"`
	SubjectType   string             `json:"subjectType"`
	SubjectID     string             `json:"subjectId,omitempty"`
	Command       string             `json:"command"` // e.g. "workspace/create", "run/activate"
	Payload       map[string]any     `json:"payload"`
}

// ── Result Envelope ──

// CLIResultPayload carries a command result from the runtime back to the CLI.
type CLIResultPayload struct {
	CorrelationID string             `json:"correlationId"`
	CommandID     string             `json:"commandId"`
	OK            bool               `json:"ok"`
	Error         string             `json:"error,omitempty"`
	Resource      map[string]any     `json:"resource,omitempty"`  // single result object
	Resources     []map[string]any   `json:"resources,omitempty"` // list result
	Snapshot      map[string]any     `json:"snapshot,omitempty"`
	OccurredAt    time.Time          `json:"occurredAt"`
	WorkspaceID   common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID         common.RunID       `json:"runId,omitempty"`
}

// ── Progress Payload ──

// CLIProgressPayload carries a runtime state change to TUI/agent subscribers.
type CLIProgressPayload struct {
	WorkspaceID common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID       common.RunID       `json:"runId,omitempty"`
	StepID      common.StepID      `json:"stepId,omitempty"`
	SubjectType string             `json:"subjectType"`
	SubjectID   string             `json:"subjectId,omitempty"`
	State       string             `json:"state,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Data        map[string]any     `json:"data,omitempty"`
}

// ── Command Constants ──

const (
	CommandWorkspaceCreate   = "workspace/create"
	CommandWorkspaceGet      = "workspace/get"
	CommandWorkspaceList     = "workspace/list"
	CommandWorkspaceSnapshot = "workspace/snapshot"
	CommandWorkspaceDelete   = "workspace/delete"
	CommandGoalDefine        = "goal/define"
	CommandGoalList          = "goal/list"
	CommandGoalGet           = "goal/get"
	CommandGoalUpdate        = "goal/update"
	CommandDoDDefine         = "dod/define"
	CommandDoDList           = "dod/list"
	CommandDoDGet            = "dod/get"
	CommandDoDUpdate         = "dod/update"
	CommandRunActivate       = "run/activate"
	CommandRunResume         = "run/resume"
	CommandRunGet            = "run/get"
	CommandRunList           = "run/list"
	CommandRunStatus         = "run/status"
	CommandRunCancel         = "run/cancel"
	CommandDecisionResolve   = "decision/resolve"
	CommandDecisionBatch     = "decision/batch-resolve"
	CommandDecisionList      = "decision/list"
	CommandDecisionGet       = "decision/get"
	CommandApprovalApprove   = "approval/approve"
	CommandApprovalReject    = "approval/reject"
	CommandApprovalBatch     = "approval/batch-resolve"
	CommandApprovalList      = "approval/list"
	CommandApprovalGet       = "approval/get"
	CommandSoulCreate        = "soul/create"
	CommandSoulGet           = "soul/get"
	CommandSoulList          = "soul/list"
	CommandSoulUpdate        = "soul/update"
	CommandSoulDelete        = "soul/delete"
	CommandSkillCreate       = "skill/create"
	CommandSkillGet          = "skill/get"
	CommandSkillList         = "skill/list"
	CommandMCPServerCreate   = "mcp-server/create"
	CommandMCPServerGet      = "mcp-server/get"
	CommandMCPServerList     = "mcp-server/list"
	CommandMCPServerUpdate   = "mcp-server/update"
	CommandMCPCapabilityList = "mcp-capability/list"
	CommandMemoryCreate      = "memory/create"
	CommandMemoryGet         = "memory/get"
	CommandMemoryUpdate      = "memory/update"
	CommandMemoryDelete      = "memory/delete"
	CommandMemoryList        = "memory/list"
	CommandMemorySearch      = "memory/search"
	CommandTemplateCreate    = "template/create"
	CommandTemplateGet       = "template/get"
	CommandTemplateList      = "template/list"
	CommandTemplateUpdate    = "template/update"
	CommandTemplateDelete    = "template/delete"
	CommandConfigurationGet  = "configuration/get"
	CommandConfigurationLLM  = "configuration/llm-update"
)

func CLICommandPayloadMap(payload CLICommandPayload) map[string]any {
	return map[string]any{
		"commandId":     payload.CommandID,
		"correlationId": payload.CorrelationID,
		"actor":         payload.Actor,
		"issuedAt":      payload.IssuedAt.Format(time.RFC3339Nano),
		"workspaceId":   string(payload.WorkspaceID),
		"runId":         string(payload.RunID),
		"subjectType":   payload.SubjectType,
		"subjectId":     payload.SubjectID,
		"command":       payload.Command,
		"payload":       clonePayloadMap(payload.Payload),
	}
}

func DecodeCLICommandPayload(payload map[string]any) (CLICommandPayload, bool) {
	commandID, ok := stringField(payload, "commandId")
	if !ok || commandID == "" {
		return CLICommandPayload{}, false
	}
	correlationID, ok := stringField(payload, "correlationId")
	if !ok || correlationID == "" {
		return CLICommandPayload{}, false
	}
	command, ok := stringField(payload, "command")
	if !ok || command == "" {
		return CLICommandPayload{}, false
	}
	issuedAt := time.Now().UTC()
	if raw, ok := optionalStringField(payload, "issuedAt"); ok && raw != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			issuedAt = parsed
		}
	}
	actor, _ := optionalStringField(payload, "actor")
	workspaceID, _ := optionalStringField(payload, "workspaceId")
	runID, _ := optionalStringField(payload, "runId")
	subjectType, _ := optionalStringField(payload, "subjectType")
	subjectID, _ := optionalStringField(payload, "subjectId")
	data, ok := optionalMapField(payload, "payload")
	if !ok {
		return CLICommandPayload{}, false
	}
	return CLICommandPayload{
		CommandID:     commandID,
		CorrelationID: correlationID,
		Actor:         actor,
		IssuedAt:      issuedAt,
		WorkspaceID:   common.WorkspaceID(workspaceID),
		RunID:         common.RunID(runID),
		SubjectType:   subjectType,
		SubjectID:     subjectID,
		Command:       command,
		Payload:       data,
	}, true
}

func CLIResultPayloadMap(payload CLIResultPayload) map[string]any {
	return map[string]any{
		"correlationId": payload.CorrelationID,
		"commandId":     payload.CommandID,
		"ok":            payload.OK,
		"error":         payload.Error,
		"resource":      clonePayloadMap(payload.Resource),
		"resources":     clonePayloadMaps(payload.Resources),
		"snapshot":      clonePayloadMap(payload.Snapshot),
		"occurredAt":    payload.OccurredAt.Format(time.RFC3339Nano),
		"workspaceId":   string(payload.WorkspaceID),
		"runId":         string(payload.RunID),
	}
}

func DecodeCLIResultPayload(payload map[string]any) (CLIResultPayload, bool) {
	correlationID, ok := stringField(payload, "correlationId")
	if !ok || correlationID == "" {
		return CLIResultPayload{}, false
	}
	commandID, ok := stringField(payload, "commandId")
	if !ok || commandID == "" {
		return CLIResultPayload{}, false
	}
	okValue, ok := payload["ok"].(bool)
	if !ok {
		return CLIResultPayload{}, false
	}
	errorText, _ := optionalStringField(payload, "error")
	resource, ok := optionalMapField(payload, "resource")
	if !ok {
		return CLIResultPayload{}, false
	}
	resources, ok := optionalMapSliceField(payload, "resources")
	if !ok {
		return CLIResultPayload{}, false
	}
	snapshot, ok := optionalMapField(payload, "snapshot")
	if !ok {
		return CLIResultPayload{}, false
	}
	occurredAt := time.Now().UTC()
	if raw, ok := optionalStringField(payload, "occurredAt"); ok && raw != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			occurredAt = parsed
		}
	}
	workspaceID, _ := optionalStringField(payload, "workspaceId")
	runID, _ := optionalStringField(payload, "runId")
	return CLIResultPayload{
		CorrelationID: correlationID,
		CommandID:     commandID,
		OK:            okValue,
		Error:         errorText,
		Resource:      resource,
		Resources:     resources,
		Snapshot:      snapshot,
		OccurredAt:    occurredAt,
		WorkspaceID:   common.WorkspaceID(workspaceID),
		RunID:         common.RunID(runID),
	}, true
}

func clonePayloadMaps(items []map[string]any) []map[string]any {
	if items == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, clonePayloadMap(item))
	}
	return out
}

func optionalMapSliceField(payload map[string]any, key string) ([]map[string]any, bool) {
	if payload == nil {
		return nil, false
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil, true
	}
	if typed, ok := raw.([]map[string]any); ok {
		return clonePayloadMaps(typed), true
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, clonePayloadMap(item))
	}
	return out, true
}
