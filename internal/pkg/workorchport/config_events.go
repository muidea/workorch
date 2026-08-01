package workorchport

import (
	"context"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type TemplateInfo struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    common.SoulID
	SoulBindings     map[string]string
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type TemplateBackend interface {
	Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error)
	Get(ctx context.Context, id string) (*TemplateInfo, error)
	List(ctx context.Context) ([]TemplateInfo, error)
	Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error)
	Delete(ctx context.Context, id string) error
}

type ProviderModelTarget struct {
	ProviderID string
	ModelID    string
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
	DefaultChat       ProviderModelTarget
	MaxConcurrentRuns int64
	AIProxyBaseURL    string
	LLMOutput         LLMOutputConfig
	ChatModelTargets  []ChatModelTargetOption
}

type ChatModelTargetOption struct {
	ProviderID          string
	ModelID             string
	DisplayName         string
	ContextWindowTokens int
	MaxOutputTokens     int
	CapabilityReady     bool
	CapabilityRevision  string
	Source              string
}

type ConfigBackend interface {
	Get(ctx context.Context) (*ServiceConfig, error)
	ListChatModelTargets(ctx context.Context) ([]ChatModelTargetOption, error)
	RefreshChatModelTargets(ctx context.Context) ([]ChatModelTargetOption, error)
}

func NormalizeChatModelTargetOptions(items []ChatModelTargetOption) []ChatModelTargetOption {
	unique := map[string]ChatModelTargetOption{}
	for _, item := range items {
		item.ProviderID = strings.TrimSpace(item.ProviderID)
		item.ModelID = strings.TrimSpace(item.ModelID)
		if item.ProviderID == "" || item.ModelID == "" || !item.CapabilityReady {
			continue
		}
		if strings.TrimSpace(item.DisplayName) == "" {
			item.DisplayName = item.ModelID
		}
		unique[item.ProviderID+"\x00"+item.ModelID] = item
	}
	ret := make([]ChatModelTargetOption, 0, len(unique))
	for _, item := range unique {
		ret = append(ret, item)
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].ProviderID != ret[j].ProviderID {
			return ret[i].ProviderID < ret[j].ProviderID
		}
		return ret[i].ModelID < ret[j].ModelID
	})
	return ret
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
	EventDecisionCreated             EventType = "decision/created"
	EventDecisionResolved            EventType = "decision/resolved"
	EventApprovalCreated             EventType = "approval/created"
	EventApprovalResolved            EventType = "approval/resolved"
	EventResultAccepted              EventType = "result/accepted"
	EventMemoryCreated               EventType = "memory/created"
	EventMemoryUpdated               EventType = "memory/updated"
	EventMemoryDeleted               EventType = "memory/deleted"
	EventMemoryRecallRecorded        EventType = "memory/recall/recorded"
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
