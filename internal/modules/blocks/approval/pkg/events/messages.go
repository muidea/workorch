package events

import (
	"fmt"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
const CommandDestination = modulecommon.ModuleID
const ResolutionRecordCommandDestination = "blocks/eventstream"

const (
	DefaultListLimit = 100
	MaxListLimit     = 200
)

type ApprovalListScope string

const ApprovalListScopeAll ApprovalListScope = "all"

type ResolveApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Principal   approvalmodel.ApprovalResolutionPrincipal
	// ResolutionContext is audit-only. It cannot replace fields in the
	// immutable authorization Context stored on the Approval.
	ResolutionContext []workorcheventpayload.Field
}

type EnsureApprovedCommand ResolveApprovalCommand
type EnsureRejectedCommand ResolveApprovalCommand

type GetApprovalCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	ApprovalID  common.ApprovalID
}

type ListAllApprovalsCommand struct {
	Scope  ApprovalListScope
	Cursor string
	Limit  int
}

type ListApprovalsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Cursor      string
	Limit       int
}

type ListApprovalsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Cursor      string
	Limit       int
}

type PurgeWorkspaceApprovalsCommand struct{ WorkspaceID common.WorkspaceID }

type OpenRuntimeApprovalRequestCommand struct {
	ApprovalID           common.ApprovalID
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	DispatchID           string
	AuthoritySnapshotID  string
	DependencyKind       string
	SubjectRef           string
	Reason               string
	RequestedAction      string
	RiskLevel            string
	AuthorizationContext []workorcheventpayload.Field
}

type OpenRuntimeApprovalRequestResult struct {
	ApprovalID          common.ApprovalID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	AuthoritySnapshotID string
}

type ApprovalChangedData struct {
	NotificationID    string
	ResolutionID      common.ResolutionID
	ApprovalID        common.ApprovalID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	StepID            common.StepID
	Reason            string
	Action            string
	Context           []workorcheventpayload.Field
	ResolutionContext []workorcheventpayload.Field
	Principal         approvalmodel.ApprovalResolutionPrincipal
	State             common.ApprovalState
}

func (d ApprovalChangedData) Validate() error {
	if strings.TrimSpace(d.NotificationID) == "" || d.ResolutionID == "" || d.ApprovalID == "" || d.WorkspaceID == "" || d.RunID == "" || d.Principal.Validate() != nil {
		return fmt.Errorf("%w: approval resolution notification requires identity, scope and principal", common.ErrInvalidArgument)
	}
	if d.State != common.ApprovalStateApproved && d.State != common.ApprovalStateRejected {
		return fmt.Errorf("%w: approval resolution notification requires terminal state", common.ErrInvalidArgument)
	}
	return nil
}

type RecordApprovalResolutionCommand struct{ Data ApprovalChangedData }

type RecordApprovalResolutionResult struct {
	NotificationID string
	ResolutionID   common.ResolutionID
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
}

type GetApprovalResult struct{ Approval *approvalmodel.Approval }
type ListApprovalsResult struct {
	Approvals  []*approvalmodel.Approval
	NextCursor string
}
type ListApprovalsByRunResult struct {
	Approvals  []*approvalmodel.Approval
	NextCursor string
}
type ListApprovalsByWorkspaceResult struct {
	Approvals  []*approvalmodel.Approval
	NextCursor string
}
type EnsureApprovedResult struct{ Approval *approvalmodel.Approval }
type EnsureRejectedResult struct{ Approval *approvalmodel.Approval }
type PurgeWorkspaceApprovalsResult struct{ Deleted int }

func NormalizeListWindow(cursor string, limit int) (string, int, error) {
	cursor = strings.TrimSpace(cursor)
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return "", 0, fmt.Errorf("%w: approval list limit must be between 1 and %d", common.ErrInvalidArgument, MaxListLimit)
	}
	return cursor, limit, nil
}

func ApprovalCursor(item *approvalmodel.Approval) string {
	if item == nil {
		return ""
	}
	return string(item.WorkspaceID) + "\x00" + string(item.RunID) + "\x00" + string(item.ID)
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[GetApprovalCommand, GetApprovalResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListAllApprovalsCommand, ListApprovalsResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandList).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByRunCommand, ListApprovalsByRunResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByRun).MessageContract(),
		eventcontract.CommandTopic[ListApprovalsByWorkspaceCommand, ListApprovalsByWorkspaceResult]("approval", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicApprovalCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[PurgeWorkspaceApprovalsCommand, PurgeWorkspaceApprovalsResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandPurgeWorkspace).MessageContract(),
		eventcontract.CommandTopic[EnsureApprovedCommand, EnsureApprovedResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureApproved).MessageContract(),
		eventcontract.CommandTopic[EnsureRejectedCommand, EnsureRejectedResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandEnsureRejected).MessageContract(),
		eventcontract.CommandTopic[RecordApprovalEvidenceCommand, RecordApprovalEvidenceResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandRecordEvidence).MessageContract(),
		eventcontract.CommandTopic[RecordApprovalResolutionCommand, RecordApprovalResolutionResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicApprovalCommandRecordResolution).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeApprovalRequestCommand, OpenRuntimeApprovalRequestResult]("approval", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenRuntimeApprovalRequest).MessageContract(),
	}
	for _, topic := range []string{
		TopicApprovalResolved,
	} {
		ret = append(ret, eventcontract.DataTopic[ApprovalChangedData]("approval", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
