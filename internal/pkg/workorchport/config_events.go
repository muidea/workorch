package workorchport

import (
	"context"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type TemplateBackend interface {
	Create(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error)
	Get(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error)
	List(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error)
	Update(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error)
	Delete(ctx context.Context, id string) error
}

type ServiceConfig struct {
	ServerAddr        string
	Backend           string
	DataDir           string
	DefaultChat       runtimecontract.ProviderModelTarget
	MaxConcurrentRuns int64
	AIProxyBaseURL    string
	LLMOutput         workorchconfig.LLMOutputRuntimeConfig
	ChatModelTargets  []ChatModelTargetOption
}

type ChatModelTargetOption struct {
	ProviderID             string
	ModelID                string
	DisplayName            string
	ContextWindowTokens    int
	MaxOutputTokens        int
	CapabilityReady        bool
	CapabilityRevision     string
	ReasoningSupported     bool
	ReasoningValid         bool
	DefaultReasoningEffort string
	ReasoningEfforts       []string
	ReasoningInvalidReason string
	Source                 string
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
		item.DefaultReasoningEffort = strings.TrimSpace(item.DefaultReasoningEffort)
		item.ReasoningInvalidReason = strings.TrimSpace(item.ReasoningInvalidReason)
		seenEfforts := map[string]struct{}{}
		efforts := make([]string, 0, len(item.ReasoningEfforts))
		for _, effort := range item.ReasoningEfforts {
			effort = strings.TrimSpace(effort)
			if effort == "" {
				continue
			}
			if _, ok := seenEfforts[effort]; ok {
				continue
			}
			seenEfforts[effort] = struct{}{}
			efforts = append(efforts, effort)
		}
		if len(efforts) == 0 {
			item.ReasoningEfforts = nil
		} else {
			item.ReasoningEfforts = efforts
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
	EventWorkspaceCreated           EventType = "workspace/created"
	EventWorkspaceListChanged       EventType = "workspace/list/changed"
	EventWorkspaceProjectionChanged EventType = "workspace/projection/changed"
	EventWorkspaceStateChanged      EventType = "workspace/state/changed"
	EventWorkspaceDeleted           EventType = "workspace/deleted"
	EventGoalDefined                EventType = "goal/defined"
	EventDoDDefined                 EventType = "dod/defined"
	EventRunStateChanged            EventType = "run/state/changed"
	EventDecisionCreated            EventType = "decision/created"
	EventDecisionResolved           EventType = "decision/resolved"
	EventApprovalResolved           EventType = "approval/resolved"
	EventResultAccepted             EventType = "result/accepted"
	EventMemoryCreated              EventType = "memory/created"
	EventMemoryUpdated              EventType = "memory/updated"
	EventMemoryDeleted              EventType = "memory/deleted"
	EventMemoryRecallRecorded       EventType = "memory/recall/recorded"
	EventGovernanceEvaluated        EventType = "governance/auto/evaluated"
	EventGovernanceResolved         EventType = "governance/auto/resolved"
	EventGovernanceEscalated        EventType = "governance/auto/escalated"
	EventGovernanceBlocked          EventType = "governance/auto/blocked"
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
