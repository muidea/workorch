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

type workorchSnapshotRef struct {
	subjectID   string
	workspaceID common.WorkspaceID
	runID       common.RunID
	stepID      common.StepID
	title       string
}

type workorchSnapshotContract struct {
	snapshotTopic   string
	subjectType     string
	publishedTopics []string
	matches         func(events.WorkorchEvent, workorchSnapshotRef) bool
	normalize       func(events.WorkorchEvent, workorchSnapshotRef) (SnapshotItem, bool)
}

var (
	workorchWorkspaceSnapshotContract = workorchSnapshotContract{
		snapshotTopic:   topicWorkspaceSnapshot,
		subjectType:     "workspace",
		publishedTopics: []string{events.TopicWorkspaceCreated, events.TopicWorkspaceStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().WorkspaceID == string(ref.workspaceID)
		},
		normalize: func(item events.WorkorchEvent, ref workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.WorkspaceEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			normalized := normalizePublishedSnapshot(event.Envelope(), topicWorkspaceSnapshot, "workspace", WorkspacePayload{
				Title:       ref.title,
				State:       string(event.Data.State),
				ActiveRunID: string(event.Data.ActiveRunID),
			})
			normalized.StepID = ""
			if normalized.RunID == "" {
				normalized.RunID = string(ref.runID)
			}
			return normalized, true
		},
	}
	workorchRunSnapshotContract = workorchSnapshotContract{
		snapshotTopic:   topicRunSnapshot,
		subjectType:     "run",
		publishedTopics: []string{events.TopicRunStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().RunID == string(ref.runID)
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.RunEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			normalized := normalizePublishedSnapshot(event.Envelope(), topicRunSnapshot, "run", RunPayload{
				State:              string(event.Data.State),
				BlockingTicketID:   string(event.Data.BlockingTicketID),
				BlockingApprovalID: string(event.Data.BlockingApprovalID),
			})
			normalized.StepID = ""
			return normalized, true
		},
	}
	workorchStepSnapshotContract = workorchSnapshotContract{
		snapshotTopic:   topicStepSnapshot,
		subjectType:     "step",
		publishedTopics: []string{events.TopicStepDispatched, events.TopicStepStateChanged},
		matches: func(item events.WorkorchEvent, ref workorchSnapshotRef) bool {
			return item.Envelope().StepID == string(ref.stepID)
		},
		normalize: func(item events.WorkorchEvent, _ workorchSnapshotRef) (SnapshotItem, bool) {
			event, ok := item.(events.StepEvent)
			if !ok {
				return SnapshotItem{}, false
			}
			return normalizePublishedSnapshot(event.Envelope(), topicStepSnapshot, "step", StepPayload{
				Kind:   event.Data.Kind,
				State:  string(event.Data.State),
				Input:  cloneMap(event.Data.Input),
				Output: cloneMap(event.Data.Output),
			}), true
		},
	}
	workorchTicketSnapshotContract = workorchSnapshotContract{
		snapshotTopic:   topicDecisionTicketSnapshot,
		subjectType:     "decision_ticket",
		publishedTopics: []string{events.TopicTicketCreated, events.TopicTicketResolved},
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
		snapshotTopic:   topicApprovalSnapshot,
		subjectType:     "approval",
		publishedTopics: []string{events.TopicApprovalCreated, events.TopicApprovalResolved},
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

func (c workorchSnapshotContract) fallback(ref workorchSnapshotRef, payload SnapshotPayload) SnapshotItem {
	return newSnapshotItem(c.snapshotTopic, c.subjectType, ref.subjectID, ref.workspaceID, ref.runID, ref.stepID, payload)
}

func normalizePublishedSnapshot(item events.Envelope, snapshotTopic, subjectType string, payload SnapshotPayload) SnapshotItem {
	envelope := cloneEnvelope(item)
	return SnapshotItem{
		ID:           fmt.Sprintf("%s:%s", snapshotTopic, envelope.SubjectID),
		Version:      events.WorkorchSnapshotVersion,
		Topic:        snapshotTopic,
		OccurredAt:   envelope.OccurredAt,
		SourceModule: "application/workorchd",
		SubjectType:  subjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      payload,
	}
}
