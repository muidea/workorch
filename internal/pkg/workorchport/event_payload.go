package workorchport

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type EventRecordPayloadKind string

const (
	EventRecordPayloadWorkspace            EventRecordPayloadKind = "workspace"
	EventRecordPayloadRun                  EventRecordPayloadKind = "run"
	EventRecordPayloadGoal                 EventRecordPayloadKind = "goal"
	EventRecordPayloadDoD                  EventRecordPayloadKind = "dod"
	EventRecordPayloadDecisionTicket       EventRecordPayloadKind = "decision_ticket"
	EventRecordPayloadDecisionResolution   EventRecordPayloadKind = "decision_resolution"
	EventRecordPayloadApproval             EventRecordPayloadKind = "approval"
	EventRecordPayloadResult               EventRecordPayloadKind = "result"
	EventRecordPayloadArtifact             EventRecordPayloadKind = "artifact"
	EventRecordPayloadPatch                EventRecordPayloadKind = "patch"
	EventRecordPayloadMemory               EventRecordPayloadKind = "memory"
	EventRecordPayloadMemoryRecall         EventRecordPayloadKind = "memory_recall"
	EventRecordPayloadAutoGovernance       EventRecordPayloadKind = "auto_governance"
	EventRecordPayloadArtifactContinuation EventRecordPayloadKind = "artifact_continuation"
)

type EventRecordPayloadInfo struct {
	Kind         EventRecordPayloadKind
	Fields       []EventPayloadFieldInfo
	Workspace    *WorkspaceEventPayloadInfo
	Run          *RunEventPayloadInfo
	Goal         *GoalEventPayloadInfo
	DoD          *DoDEventPayloadInfo
	Decision     *DecisionEventPayloadInfo
	Approval     *ApprovalEventPayloadInfo
	Result       *ResultEventPayloadInfo
	Artifact     *ArtifactEventPayloadInfo
	Patch        *PatchEventPayloadInfo
	Memory       *MemoryEventPayloadInfo
	MemoryRecall *MemoryRecallEventPayloadInfo
	Governance   *AutoGovernanceEventPayloadInfo
	Continuation *ArtifactContinuationEventPayloadInfo
}

type WorkspaceEventPayloadInfo struct {
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
}

type RunEventPayloadInfo struct {
	GoalID               common.GoalID
	DoDID                common.DoDID
	State                common.RunState
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	FailureReason        string
	RuntimeStateRevision int
	RuntimeStateRef      string
}

type GoalEventPayloadInfo struct {
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

type DoDEventPayloadInfo struct {
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

type DecisionEventPayloadInfo struct {
	Reason             string
	State              common.DecisionTicketState
	ResolutionID       common.ResolutionID
	ResolutionType     string
	ResolutionPayload  []EventPayloadFieldInfo
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type ApprovalEventPayloadInfo struct {
	Reason  string
	Action  string
	Context []EventPayloadFieldInfo
	State   common.ApprovalState
}

type ResultEventPayloadInfo struct {
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []string
}

type ArtifactEventPayloadInfo struct {
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
}

type PatchEventPayloadInfo struct {
	PatchID      common.PatchID
	StepID       common.StepID
	ToolCallID   common.ToolCallID
	PatchType    string
	TargetRef    string
	Summary      string
	State        common.PatchState
	AddedLines   int
	DeletedLines int
	ContentHash  string
	DiffHash     string
	ApprovalID   common.ApprovalID
	Risk         string
	Error        string
}

type MemoryEventPayloadInfo struct {
	MemoryType string
	Summary    string
	Content    string
	SourceType string
	SourceRef  string
	Tags       []string
	Importance int
	Confidence int
	Visibility string
}

type MemoryRecallEventPayloadInfo struct {
	TraceID            common.MemoryRecallTraceID
	Stage              string
	Source             string
	Policy             string
	Status             string
	ProviderID         string
	Operation          string
	RetrievedMemoryIDs []common.MemoryID
	Reason             string
}

type AutoGovernanceEventPayloadInfo struct {
	TraceID            common.AutoGovernanceTraceID
	ItemType           string
	ItemID             string
	Action             string
	RiskLevel          string
	Confidence         float64
	PolicyMatched      string
	PolicyBlocked      string
	BudgetBefore       int
	BudgetAfter        int
	PromptFingerprint  string
	LLMProviderID      string
	LLMModel           string
	LLMDecisionPreview string
	Status             string
	Reason             string
	EscalationReason   string
}

type ArtifactContinuationEventPayloadInfo struct {
	ContinuationID    common.ArtifactContinuationID
	RunID             common.RunID
	ArtifactID        string
	TargetRef         string
	State             common.ArtifactContinuationState
	ExpectedSequence  int
	FinalReceived     bool
	BytesWritten      int64
	RollingHash       string
	LastChunkHash     string
	ReceivedSequences []int
	CoverageTargets   []string
	EvidenceRefs      []string
	FailureReason     string
}

type EventPayloadValueKind string

const (
	EventPayloadValueNull   EventPayloadValueKind = "null"
	EventPayloadValueString EventPayloadValueKind = "string"
	EventPayloadValueBool   EventPayloadValueKind = "bool"
	EventPayloadValueInt    EventPayloadValueKind = "int"
	EventPayloadValueUint   EventPayloadValueKind = "uint"
	EventPayloadValueFloat  EventPayloadValueKind = "float"
	EventPayloadValueObject EventPayloadValueKind = "object"
	EventPayloadValueArray  EventPayloadValueKind = "array"
)

type EventPayloadFieldInfo struct {
	Name  string
	Value EventPayloadValueInfo
}

type EventPayloadValueInfo struct {
	Kind   EventPayloadValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []EventPayloadFieldInfo
	Array  []EventPayloadValueInfo
}

func (payload EventRecordPayloadInfo) String(name string) (string, bool) {
	for _, field := range payload.Fields {
		if field.Name == name && field.Value.Kind == EventPayloadValueString {
			return field.Value.String, true
		}
	}
	return "", false
}

func (payload EventRecordPayloadInfo) CompactSummary() string {
	switch payload.Kind {
	case EventRecordPayloadWorkspace:
		if payload.Workspace == nil {
			return ""
		}
		return firstEventSummary(payload.Workspace.Title, string(payload.Workspace.State))
	case EventRecordPayloadRun:
		if payload.Run == nil {
			return ""
		}
		return firstEventSummary(payload.Run.FailureReason, string(payload.Run.State))
	case EventRecordPayloadGoal:
		if payload.Goal == nil {
			return ""
		}
		return firstEventSummary(payload.Goal.Text, payload.Goal.LineageState)
	case EventRecordPayloadDoD:
		if payload.DoD == nil {
			return ""
		}
		return firstEventSummary(string(payload.DoD.State), payload.DoD.LineageState)
	case EventRecordPayloadDecisionTicket, EventRecordPayloadDecisionResolution:
		if payload.Decision == nil {
			return ""
		}
		return firstEventSummary(payload.Decision.Reason, payload.Decision.ResolutionType, string(payload.Decision.State))
	case EventRecordPayloadApproval:
		if payload.Approval == nil {
			return ""
		}
		return firstEventSummary(payload.Approval.Reason, payload.Approval.Action, string(payload.Approval.State))
	case EventRecordPayloadResult:
		if payload.Result == nil {
			return ""
		}
		return firstEventSummary(payload.Result.Summary, payload.Result.Title, string(payload.Result.State))
	case EventRecordPayloadArtifact:
		if payload.Artifact == nil {
			return ""
		}
		return firstEventSummary(payload.Artifact.Summary, payload.Artifact.Name, string(payload.Artifact.State))
	case EventRecordPayloadPatch:
		if payload.Patch == nil {
			return ""
		}
		return firstEventSummary(payload.Patch.Summary, string(payload.Patch.State), payload.Patch.Error)
	case EventRecordPayloadMemory:
		if payload.Memory == nil {
			return ""
		}
		return firstEventSummary(payload.Memory.Summary, payload.Memory.MemoryType)
	case EventRecordPayloadMemoryRecall:
		if payload.MemoryRecall == nil {
			return ""
		}
		return firstEventSummary(payload.MemoryRecall.Reason, payload.MemoryRecall.Status, payload.MemoryRecall.Operation)
	case EventRecordPayloadAutoGovernance:
		if payload.Governance == nil {
			return ""
		}
		return firstEventSummary(payload.Governance.Reason, payload.Governance.Status, payload.Governance.Action)
	case EventRecordPayloadArtifactContinuation:
		if payload.Continuation == nil {
			return ""
		}
		return firstEventSummary(payload.Continuation.FailureReason, string(payload.Continuation.State), payload.Continuation.TargetRef)
	default:
		return eventPayloadFieldsSummary(payload.Fields)
	}
}

func eventPayloadFieldsSummary(fields []EventPayloadFieldInfo) string {
	for _, key := range []string{"summary", "title", "reason", "failureReason", "state", "kind", "action", "error", "targetRef"} {
		for _, field := range fields {
			if field.Name == key && field.Value.Kind == EventPayloadValueString && strings.TrimSpace(field.Value.String) != "" {
				return strings.TrimSpace(field.Value.String)
			}
		}
	}
	return ""
}

func firstEventSummary(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
