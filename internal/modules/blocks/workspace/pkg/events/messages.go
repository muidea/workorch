package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

// Workspace and WorkspaceTemplate are owner-local EventHub DTOs.
// They preserve the owner result information without exposing repository
// models or map-backed mutable state across the component boundary.
type Workspace struct {
	Revision                      uint64
	ID                            common.WorkspaceID
	Title                         string
	Description                   string
	DeliverableDirectory          string
	InternalDirectory             string
	State                         common.WorkspaceState
	ActiveRunID                   common.RunID
	TemplateID                    string
	ProfileID                     string
	DefaultLLMSelection           *runtimecontract.ProviderInferenceSelection
	DefaultSoulID                 common.SoulID
	SoulBindings                  []soulbinding.Binding
	ContextHints                  workorcheventpayload.Payload
	EvidencePolicies              []workorcheventpayload.Payload
	DefinitionAdmissionCandidates []workspacemodel.DefinitionAdmissionCandidate
	StartupInput                  *workspacemodel.WorkspaceStartupInputContract
	ActivationAdmission           *workspacemodel.WorkspaceActivationAdmission
	TerminalDelivery              *runtimecontract.RuntimeTerminalDeliveryIdentity
}

type WorkspaceTemplate struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     []soulbinding.Binding
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type UpdateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	TemplateID           string
	ProfileID            string
	DefaultLLMSelection  *runtimecontract.ProviderInferenceSelection
	DefaultSoulID        common.SoulID
	SoulBindings         []soulbinding.Binding
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
}
type UpdateWorkspaceLLMSelectionCommand struct {
	WorkspaceID      common.WorkspaceID
	Selection        *runtimecontract.ProviderInferenceSelection
	ExpectedRevision uint64
}
type DeleteWorkspaceCommand struct {
	WorkspaceID      common.WorkspaceID
	RequestID        string
	Purge            bool
	ExpectedRevision uint64
}

func (c DeleteWorkspaceCommand) Normalized() DeleteWorkspaceCommand {
	c.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(c.WorkspaceID)))
	c.RequestID = strings.TrimSpace(c.RequestID)
	return c
}

func (c DeleteWorkspaceCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RequestID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

func (c DeleteWorkspaceCommand) Mode() deletioncontract.Mode {
	if c.Purge {
		return deletioncontract.ModePurge
	}
	return deletioncontract.ModeRetain
}

func StableWorkspaceDeletionID(workspaceID common.WorkspaceID, requestID string) string {
	material := fmt.Sprintf("workspace-deletion\x00%s\x00%s", strings.TrimSpace(string(workspaceID)), strings.TrimSpace(requestID))
	digest := sha256.Sum256([]byte(material))
	return "workspace-deletion-sha256:" + hex.EncodeToString(digest[:])
}

type DeleteWorkspaceDisposition string

const (
	DeleteWorkspaceCompleted DeleteWorkspaceDisposition = "completed"
	DeleteWorkspaceDuplicate DeleteWorkspaceDisposition = "duplicate"
)

// DeleteWorkspaceResult is an immutable receipt only. Workspace-private
// deletion phase and owner acknowledgement state never cross EventHub.
type DeleteWorkspaceResult struct {
	WorkspaceID       common.WorkspaceID
	DeletionID        string
	RequestID         string
	Mode              deletioncontract.Mode
	Disposition       DeleteWorkspaceDisposition
	DeletionRevision  uint64
	WorkspaceRevision uint64
	Completed         bool
}

// WorkspaceDeletedData is the immutable, completed deletion receipt emitted
// after every participating owner has sealed (and, for purge, purged) its
// Workspace scope. It deliberately excludes the Workspace-private phase and
// acknowledgement journal. DeletionID is the stable replay identity, so an
// EventHub/SSE consumer can de-duplicate response-loss retries without relying
// on transport event IDs.
type WorkspaceDeletedData struct {
	WorkspaceID       common.WorkspaceID
	DeletionID        string
	RequestID         string
	Mode              deletioncontract.Mode
	Disposition       DeleteWorkspaceDisposition
	DeletionRevision  uint64
	WorkspaceRevision uint64
	CompletedAt       time.Time
}

func (d WorkspaceDeletedData) Validate() error {
	d.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(d.WorkspaceID)))
	d.DeletionID = strings.TrimSpace(d.DeletionID)
	d.RequestID = strings.TrimSpace(d.RequestID)
	if d.WorkspaceID == "" || d.RequestID == "" || d.DeletionRevision == 0 || d.WorkspaceRevision == 0 || d.CompletedAt.IsZero() {
		return common.ErrInvalidArgument
	}
	if d.DeletionID != StableWorkspaceDeletionID(d.WorkspaceID, d.RequestID) || d.Disposition != DeleteWorkspaceCompleted {
		return common.ErrConflict
	}
	switch d.Mode {
	case deletioncontract.ModeRetain, deletioncontract.ModePurge:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

func (r DeleteWorkspaceResult) ValidateFor(command DeleteWorkspaceCommand) error {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != command.WorkspaceID || strings.TrimSpace(r.DeletionID) != StableWorkspaceDeletionID(command.WorkspaceID, command.RequestID) || r.RequestID != command.RequestID || r.Mode != command.Mode() || r.DeletionRevision == 0 || r.WorkspaceRevision == 0 || !r.Completed {
		return common.ErrConflict
	}
	switch r.Disposition {
	case DeleteWorkspaceCompleted, DeleteWorkspaceDuplicate:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

type DefineGoalCommand struct {
	GoalID      common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}
type UpdateGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	Text        string
}
type DeleteGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}
type DefineDoDCommand struct {
	DoDID       common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}
type UpdateDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
	Items       []string
}
type DeleteDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}
type CreateWorkspaceTemplateCommand struct {
	TemplateID       string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     []soulbinding.Binding
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type UpdateWorkspaceTemplateCommand CreateWorkspaceTemplateCommand
type DeleteWorkspaceTemplateCommand struct{ TemplateID string }

type GetWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListWorkspacesCommand struct{}

type GetGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}

type ListGoalsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}

type ListDoDsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetWorkspaceTemplateCommand struct {
	TemplateID string
}

type ListWorkspaceTemplatesCommand struct{}
type EnsureWorkspaceTemplateDefaultsCommand struct{}

type EnsureWorkspaceTemplateDefaultsResult struct {
	Ensured bool
}

// DefinitionAdmissionIntentCommand is the only public Definition-admission
// ingress. Candidate is immutable authoring intent. The optional observations
// are immutable facts returned by the Decision owner; they do not name a
// Workspace target state. The Workspace owner maps this command to one private
// event and independently selects persistence, promotion and eligibility.
type DefinitionAdmissionIntentCommand struct {
	Candidate                 workspacemodel.DefinitionAdmissionCandidate
	ExpectedCandidateRevision uint64
	ClarificationTicket       *WorkspaceDefinitionClarificationTicketFact
	ClarificationResolution   *WorkspaceDefinitionClarificationResolutionFact
}

type DefinitionAdmissionCandidateDisposition string

const (
	DefinitionAdmissionCandidateCreated   DefinitionAdmissionCandidateDisposition = "created"
	DefinitionAdmissionCandidateDuplicate DefinitionAdmissionCandidateDisposition = "duplicate"
)

type DefinitionAdmissionIntentDisposition string

const (
	DefinitionAdmissionIntentClarificationRequired           DefinitionAdmissionIntentDisposition = "clarification_required"
	DefinitionAdmissionIntentClarificationTicketRequired     DefinitionAdmissionIntentDisposition = "clarification_ticket_required"
	DefinitionAdmissionIntentClarificationResolutionRequired DefinitionAdmissionIntentDisposition = "clarification_resolution_required"
	DefinitionAdmissionIntentEligible                        DefinitionAdmissionIntentDisposition = "eligible"
)

// DefinitionAdmissionClarificationBinding is the Workspace-owned factual
// binding from which the application may compose a Decision-owner command.
type DefinitionAdmissionClarificationBinding struct {
	Candidate       workspacemodel.DefinitionAdmissionCandidate
	AdmissionResult workspacemodel.DefinitionAdmissionResult
}

// WorkspaceDefinitionClarificationTicketFact carries the exact concrete
// Decision command/result pair. Workspace recomputes both deterministic
// identities before accepting it; an application-authored opaque ID/hash is
// never transition authority.
type WorkspaceDefinitionClarificationTicketFact struct {
	RequestIdentity definitioncontract.WorkspaceDefinitionClarificationRequestIdentity
	TicketBinding   definitioncontract.WorkspaceDefinitionClarificationTicketBinding
}

// WorkspaceDefinitionClarificationResolutionFact carries the exact concrete
// Decision resolution command/result pair. The Workspace reducer recomputes
// its binding against the pending admission before selecting eligibility.
type WorkspaceDefinitionClarificationResolutionFact struct {
	ResolutionIdentity definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity
	ResolutionBinding  definitioncontract.WorkspaceDefinitionClarificationResolutionBinding
}

// DefinitionAdmissionIntentResult is a factual Workspace-owner outcome. The
// application may coordinate the Decision owner from this result, but cannot
// select the Workspace transition that produced it.
type DefinitionAdmissionIntentResult struct {
	Candidate            workspacemodel.DefinitionAdmissionCandidate
	AdmissionResult      workspacemodel.DefinitionAdmissionResult
	CandidateDisposition DefinitionAdmissionCandidateDisposition
	Disposition          DefinitionAdmissionIntentDisposition
	PriorClarification   *DefinitionAdmissionClarificationBinding
	StartupInput         *workspacemodel.WorkspaceStartupInputContract
	ActivationAdmission  *workspacemodel.WorkspaceActivationAdmission
}

type WorkspaceChangedData struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	RunID                common.RunID
	DefaultLLMSelection  *runtimecontract.ProviderInferenceSelection
}

type GoalDefinedData struct {
	GoalID                 common.GoalID
	WorkspaceID            common.WorkspaceID
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

type DoDDefinedData struct {
	DoDID                  common.DoDID
	WorkspaceID            common.WorkspaceID
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

// Management command results are deliberately narrow. A command topic must
// return only the fact it owns; consumers cannot infer a result variant by
// inspecting which fields of a shared union happen to be populated.
type WorkspaceResult struct{ Workspace *Workspace }
type WorkspaceListResult struct{ Workspaces []*Workspace }
type WorkspaceLLMSelectionResult struct {
	Workspace *Workspace
	Changed   bool
}
type GoalResult struct{ Goal *workspacemodel.Goal }
type GoalListResult struct{ Goals []*workspacemodel.Goal }
type DeleteGoalResult struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}
type DoDResult struct{ DoD *workspacemodel.DoD }
type DoDListResult struct{ DoDs []*workspacemodel.DoD }
type DeleteDoDResult struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}
type WorkspaceTemplateResult struct{ Template *WorkspaceTemplate }
type WorkspaceTemplateListResult struct{ Templates []*WorkspaceTemplate }
type DeleteWorkspaceTemplateResult struct{ TemplateID string }

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateWorkspaceCommand, WorkspaceResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetWorkspaceCommand, WorkspaceResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListWorkspacesCommand, WorkspaceListResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandList).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceCommand, WorkspaceResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceLLMSelectionCommand, WorkspaceLLMSelectionResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateLLMSelection).MessageContract(),
		eventcontract.CommandTopic[DeleteWorkspaceCommand, DeleteWorkspaceResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDelete).MessageContract(),
		eventcontract.CommandTopic[DefineGoalCommand, GoalResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDefineGoal).MessageContract(),
		eventcontract.CommandTopic[UpdateGoalCommand, GoalResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateGoal).MessageContract(),
		eventcontract.CommandTopic[DeleteGoalCommand, DeleteGoalResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDeleteGoal).MessageContract(),
		eventcontract.CommandTopic[GetGoalCommand, GoalResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGetGoal).MessageContract(),
		eventcontract.CommandTopic[ListGoalsByWorkspaceCommand, GoalListResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandListGoalsByWS).MessageContract(),
		eventcontract.CommandTopic[DefineDoDCommand, DoDResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDefineDoD).MessageContract(),
		eventcontract.CommandTopic[UpdateDoDCommand, DoDResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateDoD).MessageContract(),
		eventcontract.CommandTopic[DeleteDoDCommand, DeleteDoDResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDeleteDoD).MessageContract(),
		eventcontract.CommandTopic[GetDoDCommand, DoDResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGetDoD).MessageContract(),
		eventcontract.CommandTopic[ListDoDsByWorkspaceCommand, DoDListResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandListDoDsByWS).MessageContract(),
		eventcontract.CommandTopic[CreateWorkspaceTemplateCommand, WorkspaceTemplateResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetWorkspaceTemplateCommand, WorkspaceTemplateResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceTemplateCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListWorkspaceTemplatesCommand, WorkspaceTemplateListResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceTemplateCommandList).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceTemplateCommand, WorkspaceTemplateResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteWorkspaceTemplateCommand, DeleteWorkspaceTemplateResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandDelete).MessageContract(),
		eventcontract.CommandTopic[EnsureWorkspaceTemplateDefaultsCommand, EnsureWorkspaceTemplateDefaultsResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandEnsureDefaults).MessageContract(),
		eventcontract.CommandTopic[WorkspaceExecutionIntentCommand, WorkspaceExecutionIntentResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandExecutionIntent).MessageContract(),
		eventcontract.CommandTopic[RuntimeTerminalDeliveryFact, WorkspaceLifecycleCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceFactTerminalDelivery).MessageContract(),
		eventcontract.CommandTopic[DefinitionAdmissionIntentCommand, DefinitionAdmissionIntentResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceDefinitionAdmissionIntent).MessageContract(),
	}
	ret = append(ret, builtinDefaultMessageContracts()...)
	for _, topic := range []string{
		TopicWorkspaceCreated,
		TopicWorkspaceStateChanged,
	} {
		ret = append(ret, eventcontract.DataTopic[WorkspaceChangedData]("workspace", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret,
		eventcontract.DataTopic[WorkspaceDeletedData]("workspace", definitionPackage, eventcontract.LanePolicyState, TopicWorkspaceDeleted).MessageContract(),
		eventcontract.DataTopic[GoalDefinedData]("workspace", definitionPackage, eventcontract.LanePolicyState, TopicGoalDefined).MessageContract(),
		eventcontract.DataTopic[DoDDefinedData]("workspace", definitionPackage, eventcontract.LanePolicyState, TopicDoDDefined).MessageContract(),
	)
	return ret
}
