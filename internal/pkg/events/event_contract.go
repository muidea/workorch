package events

import (
	"fmt"
	"strings"

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

type EventHubLaneScope = eventcontract.LaneScope

// EventHubLaneKeyFor resolves a lane only from contracts supplied by the
// receiving owner package. The shared helper deliberately has no global topic
// registry and imports no component packages.
func EventHubLaneKeyFor(contracts []eventcontract.Contract, topic string, scope EventHubLaneScope) string {
	topic = strings.TrimSpace(topic)
	for _, contract := range contracts {
		if contract.Topic == topic {
			return eventHubLaneKey(contract, scope)
		}
	}
	panic(fmt.Sprintf("EventHub lane contract %s is not present in the supplied owner contracts", topic))
}

func eventHubLaneKey(contract eventcontract.Contract, scope EventHubLaneScope) string {
	switch contract.LanePolicy {
	case EventLanePolicyQuery:
		return eventcontract.QueryLaneKey(contract.Topic, scope)
	case EventLanePolicyState:
		return eventcontract.StateLaneKey(contract.Metadata.LaneIdentity, scope)
	case EventLanePolicyExecute:
		return executeLaneKey(contract.Metadata.LaneIdentity, scope)
	case EventLanePolicyGlobal:
		return "global/" + lanePart(contract.Metadata.LaneIdentity, "owner")
	case EventLanePolicyTopic:
		return eventcontract.TopicLaneKey(contract.Topic)
	default:
		return eventcontract.TopicLaneKey(contract.Topic)
	}
}

func executeLaneKey(identity string, scope EventHubLaneScope) string {
	kind := lanePart(identity, "unknown")
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
