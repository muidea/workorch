package appservice

import (
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

func subscribeGovernanceEvidenceAcks(t *testing.T, rawHub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(orchestratorevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(orchestratorevents.RecordApprovalEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID,
			WorkspaceID:      command.WorkspaceID,
			RunID:            command.RunID,
			ApprovalID:       command.ApprovalID,
			ResolutionID:     command.ResolutionID,
			ApprovalState:    command.ApprovalState,
		}, nil)
	})
	resolutionObserver := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, rawHub)
	resolutionObserver.Subscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(eventstreamevents.RecordApprovalResolutionCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		data := command.Data
		result.Set(eventstreamevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	t.Cleanup(func() {
		resolutionObserver.Unsubscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence)
	})
}
