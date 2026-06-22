package events

import (
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtimeeventdefs "github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent/eventdefs"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

type EventCommandClass = eventcontract.CommandClass

const (
	EventCommandClassRead    = eventcontract.CommandClassRead
	EventCommandClassWrite   = eventcontract.CommandClassWrite
	EventCommandClassExecute = eventcontract.CommandClassExecute
	EventCommandClassNotify  = eventcontract.CommandClassNotify
	EventCommandClassFacade  = eventcontract.CommandClassFacade
)

type EventLanePolicy = eventcontract.LanePolicy

const (
	EventLanePolicyTopic   = eventcontract.LanePolicyTopic
	EventLanePolicyQuery   = eventcontract.LanePolicyQuery
	EventLanePolicyState   = eventcontract.LanePolicyState
	EventLanePolicyExecute = eventcontract.LanePolicyExecute
	EventLanePolicyGlobal  = eventcontract.LanePolicyGlobal
)

type EventHubDeliveryMode string

const (
	EventHubDeliveryModeSync  EventHubDeliveryMode = "sync"
	EventHubDeliveryModeAsync EventHubDeliveryMode = "async"
)

type EventHubContract struct {
	Topic        string
	Owner        string
	CommandClass EventCommandClass
	LanePolicy   EventLanePolicy
}

var eventHubContracts = buildEventHubContracts()

func ResolveEventHubContract(topic string) (EventHubContract, bool) {
	topic = strings.TrimSpace(topic)
	if strings.HasPrefix(topic, runtimeeventdefs.TopicRuntimeBlockCommandExecutePrefix) {
		return EventHubContract{
			Topic:        topic,
			Owner:        runtimeBlockKindFromTopic(topic),
			CommandClass: EventCommandClassExecute,
			LanePolicy:   EventLanePolicyExecute,
		}, true
	}
	contract, ok := eventHubContracts[topic]
	return contract, ok
}

func EventHubDeliveryModeForTopic(topic string) EventHubDeliveryMode {
	contract, ok := ResolveEventHubContract(topic)
	if !ok {
		return EventHubDeliveryModeSync
	}
	switch contract.CommandClass {
	case EventCommandClassNotify:
		return EventHubDeliveryModeAsync
	case EventCommandClassRead, EventCommandClassWrite, EventCommandClassExecute, EventCommandClassFacade:
		return EventHubDeliveryModeSync
	default:
		return EventHubDeliveryModeSync
	}
}

func EventHubLaneKey(envelope Envelope) string {
	envelope.Topic = strings.TrimSpace(envelope.Topic)
	if runtimeBlockExecuteAcceptedTopic(envelope.Topic) {
		return runtimeExecutionStateLaneKey(envelope)
	}
	if runtimeBlockExecuteResultTopic(envelope.Topic) {
		return stateLaneKey("orchestrator", envelope)
	}
	if contract, ok := ResolveEventHubContract(envelope.Topic); ok {
		return contract.LaneKey(envelope)
	}
	return fallbackEventLaneKey(envelope)
}

func (c EventHubContract) LaneKey(envelope Envelope) string {
	switch c.LanePolicy {
	case EventLanePolicyQuery:
		return queryLaneKey(c.Topic, envelope)
	case EventLanePolicyState:
		return stateLaneKey(c.Owner, envelope)
	case EventLanePolicyExecute:
		return executeLaneKey(c, envelope)
	case EventLanePolicyGlobal:
		return globalLaneKey(c.Owner)
	case EventLanePolicyTopic:
		return topicLaneKey(c.Topic)
	default:
		return fallbackEventLaneKey(envelope)
	}
}

func buildEventHubContracts() map[string]EventHubContract {
	contracts := map[string]EventHubContract{}
	for _, source := range [][]eventcontract.Contract{
		workorchdevents.Contracts(),
		llmevents.Contracts(),
		runtimeeventdefs.Contracts(),
		approvalevents.Contracts(),
		decisionevents.Contracts(),
		eventstreamevents.Contracts(),
		governanceevents.Contracts(),
		mcpevents.Contracts(),
		memoryevents.Contracts(),
		orchestratorevents.Contracts(),
		resultevents.Contracts(),
		runevents.Contracts(),
		skillevents.Contracts(),
		soulevents.Contracts(),
		workspaceevents.Contracts(),
	} {
		for _, contract := range source {
			if strings.TrimSpace(contract.Topic) == "" {
				continue
			}
			contracts[contract.Topic] = EventHubContract{
				Topic:        contract.Topic,
				Owner:        contract.Owner,
				CommandClass: contract.CommandClass,
				LanePolicy:   contract.LanePolicy,
			}
		}
	}

	return contracts
}

func fallbackEventLaneKey(envelope Envelope) string {
	topic := strings.TrimSpace(envelope.Topic)
	if topic != "" {
		return topicLaneKey(topic)
	}
	return "topic/unknown"
}

func topicLaneKey(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "topic/unknown"
	}
	return "topic/" + topic
}

func queryLaneKey(topic string, envelope Envelope) string {
	topicPart := strings.Trim(strings.TrimSpace(topic), "/")
	if topicPart == "" {
		topicPart = "unknown"
	}
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	if runID := strings.TrimSpace(envelope.RunID); runID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "query/" + topicPart + "/run/" + runID
	}
	if stepID := strings.TrimSpace(envelope.StepID); stepID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/step/" + stepID
		}
		return "query/" + topicPart + "/step/" + stepID
	}
	if workspaceID != "" {
		return "query/" + topicPart + "/workspace/" + workspaceID
	}
	if subjectID := strings.TrimSpace(envelope.SubjectID); subjectID != "" {
		return "query/" + topicPart + "/subject/" + subjectID
	}
	return "query/" + topicPart
}

func stateLaneKey(owner string, envelope Envelope) string {
	owner = lanePart(owner, "owner")
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	switch owner {
	case "run":
		if runID := strings.TrimSpace(envelope.RunID); runID != "" {
			if workspaceID != "" {
				return "state/run/" + workspaceID + "/" + runID
			}
			return "state/run/" + runID
		}
	case "orchestrator":
		if runID := strings.TrimSpace(envelope.RunID); runID != "" {
			if workspaceID != "" {
				return "state/orchestrator/workspace/" + workspaceID + "/run/" + runID
			}
			return "state/orchestrator/run/" + runID
		}
	case "workspace":
		if workspaceID != "" {
			return "state/workspace/" + workspaceID
		}
	}
	if subjectID := strings.TrimSpace(envelope.SubjectID); subjectID != "" {
		if workspaceID != "" {
			return "state/" + owner + "/workspace/" + workspaceID + "/subject/" + subjectID
		}
		return "state/" + owner + "/" + subjectID
	}
	if runID := strings.TrimSpace(envelope.RunID); runID != "" {
		if workspaceID != "" {
			return "state/" + owner + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "state/" + owner + "/run/" + runID
	}
	if workspaceID != "" {
		return "state/" + owner + "/workspace/" + workspaceID
	}
	return "state/" + owner
}

func executeLaneKey(contract EventHubContract, envelope Envelope) string {
	kind := lanePart(runtimeBlockKindFromTopic(contract.Topic), "")
	if kind == "" {
		kind = lanePart(contract.Owner, "unknown")
	}
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	if stepID := strings.TrimSpace(envelope.StepID); stepID != "" {
		if workspaceID != "" {
			return "exec/block/" + kind + "/workspace/" + workspaceID + "/step/" + stepID
		}
		return "exec/block/" + kind + "/" + stepID
	}
	if subjectID := strings.TrimSpace(envelope.SubjectID); subjectID != "" {
		if workspaceID != "" {
			return "exec/block/" + kind + "/workspace/" + workspaceID + "/subject/" + subjectID
		}
		return "exec/block/" + kind + "/subject/" + subjectID
	}
	if runID := strings.TrimSpace(envelope.RunID); runID != "" {
		if workspaceID != "" {
			return "exec/block/" + kind + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "exec/block/" + kind + "/run/" + runID
	}
	if workspaceID != "" {
		return "exec/block/" + kind + "/workspace/" + workspaceID
	}
	return "exec/block/" + kind
}

func runtimeExecutionStateLaneKey(envelope Envelope) string {
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	runID := strings.TrimSpace(envelope.RunID)
	stepID := strings.TrimSpace(envelope.StepID)
	if workspaceID != "" && runID != "" && stepID != "" {
		return "state/runtime-exec/workspace/" + workspaceID + "/run/" + runID + "/step/" + stepID
	}
	if workspaceID != "" && runID != "" {
		return "state/runtime-exec/workspace/" + workspaceID + "/run/" + runID
	}
	if workspaceID != "" {
		return "state/runtime-exec/workspace/" + workspaceID
	}
	if stepID != "" {
		return "state/runtime-exec/step/" + stepID
	}
	return "state/runtime-exec"
}

func runtimeBlockExecuteAcceptedTopic(topic string) bool {
	return topic == runtimeeventdefs.TopicRuntimeBlockExecuteAccepted
}

func runtimeBlockExecuteResultTopic(topic string) bool {
	switch topic {
	case runtimeeventdefs.TopicRuntimeBlockExecuteCompleted,
		runtimeeventdefs.TopicRuntimeBlockExecuteFailed:
		return true
	default:
		return false
	}
}

func globalLaneKey(owner string) string {
	return "global/" + lanePart(owner, "owner")
}

func runtimeBlockKindFromTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if strings.HasPrefix(topic, runtimeeventdefs.TopicRuntimeBlockCommandExecutePrefix) {
		return strings.TrimSpace(strings.TrimPrefix(topic, runtimeeventdefs.TopicRuntimeBlockCommandExecutePrefix))
	}
	switch topic {
	case llmevents.TopicLLMCommandCompleteProvider:
		return "llm-provider-completion"
	case llmevents.TopicLLMCommandGenerateEmbedding:
		return "llm-embedding"
	default:
		return ""
	}
}

func lanePart(value string, fallback string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '/', r == '-', r == '_', r == '.':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	ret := strings.Trim(b.String(), "-")
	if ret == "" {
		return fallback
	}
	return ret
}
