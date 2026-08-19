package appservice

import (
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
)

func subscribeGovernanceEvidenceAcks(t *testing.T, rawHub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(decisionevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	observer.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(approvalevents.RecordApprovalEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(approvalevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID,
			WorkspaceID:      command.WorkspaceID,
			RunID:            command.RunID,
			ApprovalID:       command.ApprovalID,
			ResolutionID:     command.ResolutionID,
			ApprovalState:    command.ApprovalState,
		}, nil)
	})
	resolutionObserver := magicevent.NewSimpleObserver(approvalevents.ResolutionRecordCommandDestination, rawHub)
	resolutionObserver.Subscribe(approvalevents.TopicApprovalCommandRecordResolution, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(approvalevents.RecordApprovalResolutionCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		data := command.Data
		result.Set(approvalevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	t.Cleanup(func() {
		resolutionObserver.Unsubscribe(approvalevents.TopicApprovalCommandRecordResolution)
		observer.Unsubscribe(approvalevents.TopicApprovalCommandRecordEvidence)
		observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
	})
}
