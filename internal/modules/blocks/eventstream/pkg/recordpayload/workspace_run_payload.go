package recordpayload

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type WorkspacePayloadData struct {
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
}

type RunPayloadData struct {
	GoalID               common.GoalID
	DoDID                common.DoDID
	State                common.RunState
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	FailureReason        string
	RuntimeStateRevision int
	RuntimeStateRef      string
}

type GoalPayloadData struct {
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

type DoDPayloadData struct {
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

func (data WorkspacePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(common.CanonicalWorkspaceOwnerState(data.State))),
		workorcheventpayload.StringField(PayloadKeyActiveRunID, string(data.ActiveRunID)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyTitle, data.Title)
	fields = appendOptionalPayloadString(fields, PayloadKeyDescription, data.Description)
	fields = appendOptionalPayloadString(fields, PayloadKeyDeliverableDirectory, data.DeliverableDirectory)
	fields = appendOptionalPayloadString(fields, PayloadKeyInternalDirectory, data.InternalDirectory)
	return workorcheventpayload.New(fields...)
}

func (data RunPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(common.CanonicalRunState(data.State))),
		workorcheventpayload.StringField(PayloadKeyBlockingTicketID, string(data.BlockingTicketID)),
		workorcheventpayload.StringField(PayloadKeyBlockingApprovalID, string(data.BlockingApprovalID)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyGoalID, string(data.GoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyDoDID, string(data.DoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureReason, data.FailureReason)
	if data.RuntimeStateRevision > 0 {
		fields = append(fields, workorcheventpayload.IntField(PayloadKeyRuntimeStateRevision, int64(data.RuntimeStateRevision)))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyRuntimeStateRef, data.RuntimeStateRef)
	return workorcheventpayload.New(fields...)
}

func (data GoalPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyTextSHA256, SHA256String(strings.TrimSpace(data.Text))),
		workorcheventpayload.IntField(PayloadKeyTextBytes, int64(len(data.Text))),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyDefinitionRevision, data.DefinitionRevision)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDefinitionHash, data.PreviousDefinitionHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousGoalID, string(data.PreviousGoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyLineageState, data.LineageState)
	return workorcheventpayload.New(fields...)
}

func (data DoDPayloadData) EventPayload() workorcheventpayload.Payload {
	normalizedItems := make([]string, 0, len(data.Items))
	for _, item := range data.Items {
		if item = strings.TrimSpace(item); item != "" {
			normalizedItems = append(normalizedItems, item)
		}
	}
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyItemsSHA256, SHA256String(strings.Join(normalizedItems, "\n"))),
		workorcheventpayload.IntField(PayloadKeyItemCount, int64(len(normalizedItems))),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyDefinitionRevision, data.DefinitionRevision)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDefinitionHash, data.PreviousDefinitionHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyPreviousDoDID, string(data.PreviousDoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyLineageState, data.LineageState)
	return workorcheventpayload.New(fields...)
}

func DecodeWorkspaceEventPayload(payload workorcheventpayload.Payload) (WorkspacePayloadData, bool) {
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	title, ok := optionalEventPayloadString(payload, PayloadKeyTitle)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	description, ok := optionalEventPayloadString(payload, PayloadKeyDescription)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	deliverableDirectory, ok := optionalEventPayloadString(payload, PayloadKeyDeliverableDirectory)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	internalDirectory, ok := optionalEventPayloadString(payload, PayloadKeyInternalDirectory)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	activeRunID, ok := optionalEventPayloadString(payload, PayloadKeyActiveRunID)
	if !ok {
		return WorkspacePayloadData{}, false
	}
	return WorkspacePayloadData{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
		InternalDirectory:    internalDirectory,
		State:                common.CanonicalWorkspaceOwnerState(common.WorkspaceState(state)),
		ActiveRunID:          common.RunID(activeRunID),
	}, true
}

func DecodeRunEventPayload(payload workorcheventpayload.Payload) (RunPayloadData, bool) {
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return RunPayloadData{}, false
	}
	goalID, ok := optionalEventPayloadString(payload, PayloadKeyGoalID)
	if !ok {
		return RunPayloadData{}, false
	}
	dodID, ok := optionalEventPayloadString(payload, PayloadKeyDoDID)
	if !ok {
		return RunPayloadData{}, false
	}
	blockingTicketID, ok := optionalEventPayloadString(payload, PayloadKeyBlockingTicketID)
	if !ok {
		return RunPayloadData{}, false
	}
	blockingApprovalID, ok := optionalEventPayloadString(payload, PayloadKeyBlockingApprovalID)
	if !ok {
		return RunPayloadData{}, false
	}
	failureReason, ok := optionalEventPayloadString(payload, PayloadKeyFailureReason)
	if !ok {
		return RunPayloadData{}, false
	}
	runtimeStateRevision, ok := optionalEventPayloadInt(payload, PayloadKeyRuntimeStateRevision)
	if !ok {
		return RunPayloadData{}, false
	}
	runtimeStateRef, ok := optionalEventPayloadString(payload, PayloadKeyRuntimeStateRef)
	if !ok {
		return RunPayloadData{}, false
	}
	return RunPayloadData{
		GoalID:               common.GoalID(goalID),
		DoDID:                common.DoDID(dodID),
		State:                common.CanonicalRunState(common.RunState(state)),
		BlockingTicketID:     common.TicketID(blockingTicketID),
		BlockingApprovalID:   common.ApprovalID(blockingApprovalID),
		FailureReason:        failureReason,
		RuntimeStateRevision: runtimeStateRevision,
		RuntimeStateRef:      runtimeStateRef,
	}, true
}
