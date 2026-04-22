package observed

import (
	"fmt"

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

type phase1SnapshotRef struct {
	subjectID   string
	workspaceID common.WorkspaceID
	runID       common.RunID
	stepID      common.StepID
}

type phase1SnapshotContract struct {
	snapshotTopic   string
	subjectType     string
	publishedTopics []string
	matches         func(events.Phase1Event, phase1SnapshotRef) bool
	normalize       func(events.Phase1Event, phase1SnapshotRef) (events.Envelope, bool)
}

var (
	phase1WorkspaceSnapshotContract = phase1SnapshotContract{
		snapshotTopic:   topicWorkspaceSnapshot,
		subjectType:     "workspace",
		publishedTopics: []string{events.TopicWorkspaceCreated, events.TopicWorkspaceStateChanged},
		matches: func(item events.Phase1Event, ref phase1SnapshotRef) bool {
			return item.Envelope().WorkspaceID == string(ref.workspaceID)
		},
		normalize: func(item events.Phase1Event, ref phase1SnapshotRef) (events.Envelope, bool) {
			event, ok := item.(events.WorkspaceEvent)
			if !ok {
				return events.Envelope{}, false
			}
			envelope := normalizePublishedSnapshot(event.Envelope(), topicWorkspaceSnapshot, "workspace", events.WorkspacePayload(event.Data.Title, event.Data.State, event.Data.ActiveRunID))
			envelope.StepID = ""
			if envelope.RunID == "" {
				envelope.RunID = string(ref.runID)
			}
			return envelope, true
		},
	}
	phase1RunSnapshotContract = phase1SnapshotContract{
		snapshotTopic:   topicRunSnapshot,
		subjectType:     "run",
		publishedTopics: []string{events.TopicRunStateChanged},
		matches: func(item events.Phase1Event, ref phase1SnapshotRef) bool {
			return item.Envelope().RunID == string(ref.runID)
		},
		normalize: func(item events.Phase1Event, _ phase1SnapshotRef) (events.Envelope, bool) {
			event, ok := item.(events.RunEvent)
			if !ok {
				return events.Envelope{}, false
			}
			envelope := normalizePublishedSnapshot(event.Envelope(), topicRunSnapshot, "run", events.RunPayload(event.Data.State, event.Data.BlockingTicketID, event.Data.BlockingApprovalID))
			envelope.StepID = ""
			return envelope, true
		},
	}
	phase1StepSnapshotContract = phase1SnapshotContract{
		snapshotTopic:   topicStepSnapshot,
		subjectType:     "step",
		publishedTopics: []string{events.TopicStepDispatched, events.TopicStepStateChanged},
		matches: func(item events.Phase1Event, ref phase1SnapshotRef) bool {
			return item.Envelope().StepID == string(ref.stepID)
		},
		normalize: func(item events.Phase1Event, _ phase1SnapshotRef) (events.Envelope, bool) {
			event, ok := item.(events.StepEvent)
			if !ok {
				return events.Envelope{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicStepSnapshot, "step", events.StepPayload(event.Data.Kind, event.Data.State, event.Data.Input, event.Data.Output)), true
		},
	}
	phase1TicketSnapshotContract = phase1SnapshotContract{
		snapshotTopic:   topicDecisionTicketSnapshot,
		subjectType:     "decision_ticket",
		publishedTopics: []string{events.TopicTicketCreated, events.TopicTicketResolved},
		matches: func(item events.Phase1Event, ref phase1SnapshotRef) bool {
			return item.Envelope().SubjectID == ref.subjectID
		},
		normalize: func(item events.Phase1Event, _ phase1SnapshotRef) (events.Envelope, bool) {
			event, ok := item.(events.DecisionTicketEvent)
			if !ok {
				return events.Envelope{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicDecisionTicketSnapshot, "decision_ticket", decisionTicketPayload(event)), true
		},
	}
	phase1ApprovalSnapshotContract = phase1SnapshotContract{
		snapshotTopic:   topicApprovalSnapshot,
		subjectType:     "approval",
		publishedTopics: []string{events.TopicApprovalCreated, events.TopicApprovalResolved},
		matches: func(item events.Phase1Event, ref phase1SnapshotRef) bool {
			return item.Envelope().SubjectID == ref.subjectID
		},
		normalize: func(item events.Phase1Event, _ phase1SnapshotRef) (events.Envelope, bool) {
			event, ok := item.(events.ApprovalEvent)
			if !ok {
				return events.Envelope{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicApprovalSnapshot, "approval", events.ApprovalPayload(event.Data.Reason, event.Data.State)), true
		},
	}
	phase1SnapshotContracts = []phase1SnapshotContract{
		phase1WorkspaceSnapshotContract,
		phase1RunSnapshotContract,
		phase1StepSnapshotContract,
		phase1TicketSnapshotContract,
		phase1ApprovalSnapshotContract,
	}
)

func phase1ObservedTopics() []string {
	return events.Phase1Topics()
}

func latestPublishedSnapshot(contract phase1SnapshotContract, ref phase1SnapshotRef, published []events.Phase1Event) (SnapshotItem, bool) {
	for idx := len(published) - 1; idx >= 0; idx-- {
		item := published[idx]
		if item == nil || !contract.acceptsTopic(item.Topic()) {
			continue
		}
		if !contract.matches(item, ref) {
			continue
		}
		envelope, ok := contract.normalize(item, ref)
		if ok {
			return mustSnapshotItem(envelope), true
		}
	}
	return SnapshotItem{}, false
}

func (c phase1SnapshotContract) acceptsTopic(topic string) bool {
	for _, candidate := range c.publishedTopics {
		if candidate == topic {
			return true
		}
	}
	return false
}

func (c phase1SnapshotContract) fallback(ref phase1SnapshotRef, payload map[string]any) events.Envelope {
	return newEnvelope(c.snapshotTopic, c.subjectType, ref.subjectID, ref.workspaceID, ref.runID, ref.stepID, payload)
}

func normalizePublishedSnapshot(item events.Envelope, snapshotTopic, subjectType string, payload map[string]any) events.Envelope {
	envelope := cloneEnvelope(item)
	envelope.ID = fmt.Sprintf("%s:%s", snapshotTopic, envelope.SubjectID)
	envelope.Version = events.Phase1SnapshotVersion
	envelope.Topic = snapshotTopic
	envelope.SubjectType = subjectType
	envelope.Payload = payload
	return envelope
}

func decisionTicketPayload(event events.DecisionTicketEvent) map[string]any {
	if event.Topic() == events.TopicTicketResolved {
		return events.DecisionResolutionPayload(
			event.Data.Reason,
			event.Data.State,
			event.ResolutionID,
			event.ResolutionType,
			event.ResolutionPayload,
		)
	}
	return events.DecisionTicketPayload(event.Data.Reason, event.Data.State)
}
