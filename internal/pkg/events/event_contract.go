package events

import (
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
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
var eventHubMessageContracts = buildEventHubMessageContracts()

func ResolveEventHubContract(topic string) (EventHubContract, bool) {
	topic = strings.TrimSpace(topic)
	contract, ok := eventHubContracts[topic]
	return contract, ok
}

func ResolveEventHubMessageContract(topic string) (eventcontract.MessageContract, bool) {
	topic = strings.TrimSpace(topic)
	contract, ok := eventHubMessageContracts[topic]
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

type EventHubLaneScope struct {
	SubjectType string
	SubjectID   string
	WorkspaceID string
	RunID       string
	StepID      string
}

func EventHubLaneKeyFor(topic string, scope EventHubLaneScope) string {
	topic = strings.TrimSpace(topic)
	if contract, ok := ResolveEventHubContract(topic); ok {
		return contract.LaneKey(scope)
	}
	return fallbackEventLaneKey(topic)
}

func (c EventHubContract) LaneKey(scope EventHubLaneScope) string {
	switch c.LanePolicy {
	case EventLanePolicyQuery:
		return queryLaneKey(c.Topic, scope)
	case EventLanePolicyState:
		return stateLaneKey(c.Owner, scope)
	case EventLanePolicyExecute:
		return executeLaneKey(c, scope)
	case EventLanePolicyGlobal:
		return globalLaneKey(c.Owner)
	case EventLanePolicyTopic:
		return topicLaneKey(c.Topic)
	default:
		return fallbackEventLaneKey(c.Topic)
	}
}

func buildEventHubContracts() map[string]EventHubContract {
	contracts := map[string]EventHubContract{}
	for _, source := range [][]eventcontract.Contract{
		workorchdevents.Contracts(),
		llmevents.Contracts(),
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

func buildEventHubMessageContracts() map[string]eventcontract.MessageContract {
	contracts := map[string]eventcontract.MessageContract{}
	for _, source := range [][]eventcontract.MessageContract{
		workorchdevents.MessageContracts(),
		llmevents.MessageContracts(),
		approvalevents.MessageContracts(),
		decisionevents.MessageContracts(),
		eventstreamevents.MessageContracts(),
		governanceevents.MessageContracts(),
		mcpevents.MessageContracts(),
		memoryevents.MessageContracts(),
		orchestratorevents.MessageContracts(),
		resultevents.MessageContracts(),
		runevents.MessageContracts(),
		skillevents.MessageContracts(),
		soulevents.MessageContracts(),
		workspaceevents.MessageContracts(),
	} {
		for _, contract := range source {
			if strings.TrimSpace(contract.Topic) == "" {
				continue
			}
			contracts[contract.Topic] = contract
		}
	}
	return contracts
}

func fallbackEventLaneKey(topic string) string {
	topic = strings.TrimSpace(topic)
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

func queryLaneKey(topic string, scope EventHubLaneScope) string {
	topicPart := strings.Trim(strings.TrimSpace(topic), "/")
	if topicPart == "" {
		topicPart = "unknown"
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if runID := strings.TrimSpace(scope.RunID); runID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "query/" + topicPart + "/run/" + runID
	}
	if stepID := strings.TrimSpace(scope.StepID); stepID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/step/" + stepID
		}
		return "query/" + topicPart + "/step/" + stepID
	}
	if workspaceID != "" {
		return "query/" + topicPart + "/workspace/" + workspaceID
	}
	if subjectID := strings.TrimSpace(scope.SubjectID); subjectID != "" {
		return "query/" + topicPart + "/subject/" + subjectID
	}
	return "query/" + topicPart
}

func stateLaneKey(owner string, scope EventHubLaneScope) string {
	owner = lanePart(owner, "owner")
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	switch owner {
	case "run":
		if runID := strings.TrimSpace(scope.RunID); runID != "" {
			if workspaceID != "" {
				return "state/run/" + workspaceID + "/" + runID
			}
			return "state/run/" + runID
		}
	case "orchestrator":
		if runID := strings.TrimSpace(scope.RunID); runID != "" {
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
	if subjectID := strings.TrimSpace(scope.SubjectID); subjectID != "" {
		if workspaceID != "" {
			return "state/" + owner + "/workspace/" + workspaceID + "/subject/" + subjectID
		}
		return "state/" + owner + "/" + subjectID
	}
	if runID := strings.TrimSpace(scope.RunID); runID != "" {
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

func executeLaneKey(contract EventHubContract, scope EventHubLaneScope) string {
	kind := lanePart(executionKindFromTopic(contract.Topic), "")
	if kind == "" {
		kind = lanePart(contract.Owner, "unknown")
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if stepID := strings.TrimSpace(scope.StepID); stepID != "" {
		if workspaceID != "" {
			return "exec/block/" + kind + "/workspace/" + workspaceID + "/step/" + stepID
		}
		return "exec/block/" + kind + "/" + stepID
	}
	if subjectID := strings.TrimSpace(scope.SubjectID); subjectID != "" {
		if workspaceID != "" {
			return "exec/block/" + kind + "/workspace/" + workspaceID + "/subject/" + subjectID
		}
		return "exec/block/" + kind + "/subject/" + subjectID
	}
	if runID := strings.TrimSpace(scope.RunID); runID != "" {
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

func globalLaneKey(owner string) string {
	return "global/" + lanePart(owner, "owner")
}

func executionKindFromTopic(topic string) string {
	topic = strings.TrimSpace(topic)
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
