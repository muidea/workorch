package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type TemplateInfo struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    common.SoulID
	SoulBindings     map[string]string
	EvidencePolicies []map[string]any
}

type TemplateBackend interface {
	Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error)
	Get(ctx context.Context, id string) (*TemplateInfo, error)
	List(ctx context.Context) ([]TemplateInfo, error)
	Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error)
	Delete(ctx context.Context, id string) error
}

type LLMProvider struct {
	ID         string
	Provider   string
	APISurface string
	BaseURL    string
	Model      string
	Timeout    string
	Disabled   bool
	Streaming  bool
}

type LLMOutputConfig struct {
	MaxPatchContentBytes               int
	MaxGraphBytes                      int
	MaxGraphFileWrites                 int
	SchemaInvalidLargePreviewThreshold int
	MaxArtifactChunkBytes              int
	PreviousChunkTailBytes             int
	MaxChunkRetryAttempts              int
	MaxArtifactPlanAttempts            int
	MaxArtifactReplanAttempts          int
}

type ServiceConfig struct {
	ServerAddr        string
	Backend           string
	DataDir           string
	DefaultProvider   string
	MaxConcurrentRuns int64
	Providers         []LLMProvider
	LLMOutput         LLMOutputConfig
}

type ConfigBackend interface {
	Get(ctx context.Context) (*ServiceConfig, error)
}

// EventType classifies backend-published events for the TUI/agent.
type EventType string

const (
	EventWorkspaceCreated            EventType = "workspace/created"
	EventWorkspaceListChanged        EventType = "workspace/list/changed"
	EventWorkspaceProjectionChanged  EventType = "workspace/projection/changed"
	EventWorkspaceStateChanged       EventType = "workspace/state/changed"
	EventWorkspaceDeleted            EventType = "workspace/deleted"
	EventGoalDefined                 EventType = "goal/defined"
	EventDoDDefined                  EventType = "dod/defined"
	EventRunStateChanged             EventType = "run/state/changed"
	EventStepDispatched              EventType = "step/dispatched"
	EventStepCompleted               EventType = "step/completed"
	EventDecisionCreated             EventType = "decision/created"
	EventDecisionResolved            EventType = "decision/resolved"
	EventApprovalCreated             EventType = "approval/created"
	EventApprovalResolved            EventType = "approval/resolved"
	EventResultAccepted              EventType = "result/accepted"
	EventMemoryCreated               EventType = "memory/created"
	EventMemoryUpdated               EventType = "memory/updated"
	EventMemoryDeleted               EventType = "memory/deleted"
	EventMemoryRecallRecorded        EventType = "memory/recall/recorded"
	EventLLMStreamProgress           EventType = "llm/stream/progress"
	EventLLMUsageRecorded            EventType = "llm/usage/recorded"
	EventPatchPreviewed              EventType = "patch/previewed"
	EventPatchApplied                EventType = "patch/applied"
	EventPatchFailed                 EventType = "patch/failed"
	EventPatchReverted               EventType = "patch/reverted"
	EventArtifactContinuationChanged EventType = "artifact_continuation/changed"
	EventGovernanceEvaluated         EventType = "governance/auto/evaluated"
	EventGovernanceResolved          EventType = "governance/auto/resolved"
	EventGovernanceEscalated         EventType = "governance/auto/escalated"
	EventGovernanceBlocked           EventType = "governance/auto/blocked"
)

type Event struct {
	ID          string
	Type        EventType
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Revision    int64
	Payload     map[string]any
}

type EventHandler func(ctx context.Context, evt Event)

type EventsBackend interface {
	Subscribe(ctx context.Context, handler EventHandler) (unsubscribe func(), err error)
}
