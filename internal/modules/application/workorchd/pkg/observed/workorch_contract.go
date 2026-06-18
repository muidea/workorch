package observed

import (
	"fmt"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workorchdcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	topicWorkspaceSnapshot      = "/workspace/snapshot"
	topicRunSnapshot            = "/run/snapshot"
	topicStepSnapshot           = "/step/snapshot"
	topicDecisionTicketSnapshot = "/decision/ticket/snapshot"
	topicApprovalSnapshot       = "/approval/snapshot"
)

type workorchSnapshotRef struct {
	subjectID            string
	workspaceID          common.WorkspaceID
	runID                common.RunID
	stepID               common.StepID
	title                string
	description          string
	deliverableDirectory string
}

type workorchSnapshotContract struct {
	publishedTopics []string
	matches         func(events.WorkorchEvent, workorchSnapshotRef) bool
	normalize       func(events.WorkorchEvent, workorchSnapshotRef) (SnapshotItem, bool)
}

var (
	workorchWorkspaceSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().WorkspaceID == string(ref.workspaceID)
		},
		normalize: func(item events.WorkorchEvent, ref workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.WorkspaceEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			title := event.Data.Title
			if title == "" {
				title = ref.title
			}
			description := event.Data.Description
			if description == "" {
				description = ref.description
			}
			deliverableDirectory := event.Data.DeliverableDirectory
			if deliverableDirectory == "" {
				deliverableDirectory = ref.deliverableDirectory
			}
			normalized := normalizePublishedSnapshot(event.Envelope(), topicWorkspaceSnapshot, "workspace", WorkspacePayload{
				Title:                title,
				Description:          description,
				DeliverableDirectory: deliverableDirectory,
				State:                string(event.Data.State),
				ActiveRunID:          string(event.Data.ActiveRunID),
			})
			normalized.StepID = ""
			if normalized.RunID == "" {
				normalized.RunID = string(ref.runID)
			}
			return normalized, true
		},
	}
	workorchRunSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{runevents.TopicRunCreated, runevents.TopicRunStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().RunID == string(ref.runID)
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.RunEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			normalized := normalizePublishedSnapshot(event.Envelope(), topicRunSnapshot, "run", RunPayload{
				GoalID:             string(event.Data.GoalID),
				DoDID:              string(event.Data.DoDID),
				State:              string(event.Data.State),
				BlockingTicketID:   string(event.Data.BlockingTicketID),
				BlockingApprovalID: string(event.Data.BlockingApprovalID),
				FailureReason:      event.Data.FailureReason,
			})
			normalized.StepID = ""
			return normalized, true
		},
	}
	workorchStepSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{runevents.TopicStepDispatched, runevents.TopicStepStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().StepID == string(ref.stepID)
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.StepEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicStepSnapshot, "step", StepPayload{
				Kind:   llmcommon.CanonicalRuntimeBlockKind(event.Data.Kind),
				State:  string(event.Data.State),
				Input:  cloneMap(event.Data.Input),
				Output: cloneMap(event.Data.Output),
			}), true
		},
	}
	workorchTicketSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{decisionevents.TopicTicketCreated, decisionevents.TopicTicketResolved, decisionevents.TopicTicketUpdated},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().SubjectID == ref.subjectID
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.DecisionTicketEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicDecisionTicketSnapshot, "decision_ticket", DecisionTicketPayload{
				Reason:            event.Data.Reason,
				State:             string(event.Data.State),
				ResolutionID:      string(event.ResolutionID),
				ResolutionType:    event.ResolutionType,
				ResolutionPayload: cloneMap(event.ResolutionPayload),
			}), true
		},
	}
	workorchApprovalSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{approvalevents.TopicApprovalCreated, approvalevents.TopicApprovalResolved},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().SubjectID == ref.subjectID
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.ApprovalEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicApprovalSnapshot, "approval", ApprovalPayload{
				Reason: event.Data.Reason,
				State:  string(event.Data.State),
			}), true
		},
	}
	workorchSnapshotContracts = []workorchSnapshotContract{
		workorchWorkspaceSnapshotContract,
		workorchRunSnapshotContract,
		workorchStepSnapshotContract,
		workorchTicketSnapshotContract,
		workorchApprovalSnapshotContract,
	}
)

func workorchObservedTopics() []string {
	return events.WorkorchTopics()
}

func latestPublishedSnapshot(contract workorchSnapshotContract, ref workorchSnapshotRef, published []events.WorkorchEvent) (SnapshotItem, bool) {
	for idx := len(published) - 1; idx >= 0; idx-- {
		item := published[idx]
		if item == nil || !contract.acceptsTopic(item.Topic()) {
			continue
		}
		if !contract.matches(item, ref) {
			continue
		}
		normalized, ok := contract.normalize(item, ref)
		if ok {
			return normalized, true
		}
	}
	return SnapshotItem{}, false
}

func (c workorchSnapshotContract) acceptsTopic(topic string) bool {
	for _, candidate := range c.publishedTopics {
		if candidate == topic {
			return true
		}
	}
	return false
}

func normalizePublishedSnapshot(item events.Envelope, snapshotTopic, subjectType string, payload SnapshotPayload) SnapshotItem {
	envelope := cloneEnvelope(item)
	return SnapshotItem{
		ID:           fmt.Sprintf("%s:%s", snapshotTopic, envelope.SubjectID),
		Version:      events.WorkorchSnapshotVersion,
		Topic:        snapshotTopic,
		OccurredAt:   envelope.OccurredAt,
		SourceModule: workorchdcommon.EventSourceModuleID,
		SubjectType:  subjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      payload,
	}
}
