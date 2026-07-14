package observed

import (
	"fmt"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	workorchdcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	topicWorkspaceSnapshot      = "/workspace/snapshot"
	topicRunSnapshot            = "/run/snapshot"
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
	matches         func(eventstreamevents.EventRecordData, workorchSnapshotRef) bool
	normalize       func(eventstreamevents.EventRecordData, workorchSnapshotRef) (SnapshotItem, bool)
}

var (
	workorchWorkspaceSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged},
		matches: func(item eventstreamevents.EventRecordData, ref workorchSnapshotRef) bool {
			return item.WorkspaceID == ref.workspaceID
		},
		normalize: func(item eventstreamevents.EventRecordData, ref workorchSnapshotRef) (SnapshotItem, bool) {
			data, ok := events.DecodeWorkspaceEventPayload(item.Payload)
			if !ok {
				return SnapshotItem{}, false
			}
			title := data.Title
			if title == "" {
				title = ref.title
			}
			description := data.Description
			if description == "" {
				description = ref.description
			}
			deliverableDirectory := data.DeliverableDirectory
			if deliverableDirectory == "" {
				deliverableDirectory = ref.deliverableDirectory
			}
			normalized := normalizePublishedSnapshot(item, topicWorkspaceSnapshot, "workspace", WorkspacePayload{
				Title:                title,
				Description:          description,
				DeliverableDirectory: deliverableDirectory,
				State:                string(data.State),
				ActiveRunID:          string(data.ActiveRunID),
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
		matches: func(item eventstreamevents.EventRecordData, ref workorchSnapshotRef) bool {
			return item.RunID == ref.runID
		},
		normalize: func(item eventstreamevents.EventRecordData, _ workorchSnapshotRef) (SnapshotItem, bool) {
			data, ok := events.DecodeRunEventPayload(item.Payload)
			if !ok {
				return SnapshotItem{}, false
			}
			normalized := normalizePublishedSnapshot(item, topicRunSnapshot, "run", RunPayload{
				GoalID:             string(data.GoalID),
				DoDID:              string(data.DoDID),
				State:              string(data.State),
				BlockingTicketID:   string(data.BlockingTicketID),
				BlockingApprovalID: string(data.BlockingApprovalID),
				FailureReason:      data.FailureReason,
			})
			normalized.StepID = ""
			return normalized, true
		},
	}
	workorchTicketSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{decisionevents.TopicTicketCreated, decisionevents.TopicTicketResolved, decisionevents.TopicTicketUpdated},
		matches: func(item eventstreamevents.EventRecordData, ref workorchSnapshotRef) bool {
			return item.SubjectID == ref.subjectID
		},
		normalize: normalizeDecisionTicketSnapshot,
	}
	workorchApprovalSnapshotContract = workorchSnapshotContract{
		publishedTopics: []string{approvalevents.TopicApprovalCreated, approvalevents.TopicApprovalResolved},
		matches: func(item eventstreamevents.EventRecordData, ref workorchSnapshotRef) bool {
			return item.SubjectID == ref.subjectID
		},
		normalize: func(item eventstreamevents.EventRecordData, _ workorchSnapshotRef) (SnapshotItem, bool) {
			data, ok := events.DecodeApprovalEventPayload(item.Payload)
			if !ok {
				return SnapshotItem{}, false
			}
			return normalizePublishedSnapshot(item, topicApprovalSnapshot, "approval", ApprovalPayload{
				Reason:  data.Reason,
				Action:  data.Action,
				Context: data.Context.Clone(),
				State:   string(data.State),
			}), true
		},
	}
	workorchSnapshotContracts = []workorchSnapshotContract{
		workorchWorkspaceSnapshotContract,
		workorchRunSnapshotContract,
		workorchTicketSnapshotContract,
		workorchApprovalSnapshotContract,
	}
)

func latestPublishedSnapshot(contract workorchSnapshotContract, ref workorchSnapshotRef, published []eventstreamevents.EventRecordData) (SnapshotItem, bool) {
	for idx := len(published) - 1; idx >= 0; idx-- {
		item := published[idx]
		if !contract.acceptsTopic(item.Topic) || !events.ValidateWorkorchEventVersion(item.Topic, item.Version) {
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

func normalizeDecisionTicketSnapshot(item eventstreamevents.EventRecordData, _ workorchSnapshotRef) (SnapshotItem, bool) {
	if item.Topic == decisionevents.TopicTicketResolved {
		data, ok := events.DecodeDecisionResolutionEventPayload(item.Payload)
		if !ok {
			return SnapshotItem{}, false
		}
		return normalizePublishedSnapshot(item, topicDecisionTicketSnapshot, "decision_ticket", DecisionTicketPayload{
			Reason:            data.Reason,
			State:             string(data.State),
			ResolutionID:      string(data.ResolutionID),
			ResolutionType:    data.Type,
			ResolutionPayload: data.Payload.Clone(),
		}), true
	}
	data, ok := events.DecodeDecisionTicketEventPayload(item.Payload)
	if !ok {
		return SnapshotItem{}, false
	}
	return normalizePublishedSnapshot(item, topicDecisionTicketSnapshot, "decision_ticket", DecisionTicketPayload{
		Reason: data.Reason,
		State:  string(data.State),
	}), true
}

func normalizePublishedSnapshot(item eventstreamevents.EventRecordData, snapshotTopic, subjectType string, payload SnapshotPayload) SnapshotItem {
	return SnapshotItem{
		ID:           fmt.Sprintf("%s:%s", snapshotTopic, item.SubjectID),
		Version:      SnapshotVersion,
		Topic:        snapshotTopic,
		OccurredAt:   item.OccurredAt,
		SourceModule: workorchdcommon.EventSourceModuleID,
		SubjectType:  subjectType,
		SubjectID:    item.SubjectID,
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      payload,
	}
}
