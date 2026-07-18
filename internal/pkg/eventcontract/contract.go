package eventcontract

import "strings"

type CommandClass string

const (
	CommandClassRead    CommandClass = "read"
	CommandClassWrite   CommandClass = "write"
	CommandClassExecute CommandClass = "execute"
	CommandClassNotify  CommandClass = "notify"
	CommandClassFacade  CommandClass = "facade"
)

type LanePolicy string

const (
	LanePolicyTopic   LanePolicy = "topic"
	LanePolicyQuery   LanePolicy = "query"
	LanePolicyState   LanePolicy = "state"
	LanePolicyExecute LanePolicy = "execute"
	LanePolicyGlobal  LanePolicy = "global"
)

type Contract struct {
	Topic        string
	Owner        string
	CommandClass CommandClass
	LanePolicy   LanePolicy
}

// LaneScope identifies the authority object that a command lane serializes.
// It lives with the lane policy contract so topic packages can bind their own
// events without importing the repository-wide event catalog.
type LaneScope struct {
	SubjectType string
	SubjectID   string
	WorkspaceID string
	RunID       string
	StepID      string
}

func TopicLaneKey(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "topic/unknown"
	}
	return "topic/" + topic
}

func QueryLaneKey(topic string, scope LaneScope) string {
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

func StateLaneKey(owner string, scope LaneScope) string {
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

func lanePart(value string, fallback string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return fallback
	}
	return strings.ReplaceAll(value, "/", "-")
}

type MessageContract struct {
	Topic             string
	Owner             string
	DefinitionPackage string
	CommandClass      CommandClass
	LanePolicy        LanePolicy
}

func Build(owner string, class CommandClass, lane LanePolicy, topics ...string) []Contract {
	ret := make([]Contract, 0, len(topics))
	for _, topic := range topics {
		if topic == "" {
			continue
		}
		ret = append(ret, Contract{
			Topic:        topic,
			Owner:        owner,
			CommandClass: class,
			LanePolicy:   lane,
		})
	}
	return ret
}

type NoCommand struct{}
type NoData struct{}
type NoResult struct{}

type TypedTopic interface {
	EventContract() Contract
	MessageContract() MessageContract
}

type Topic[Command any, Data any, Result any] struct {
	Name              string
	Owner             string
	DefinitionPackage string
	CommandClass      CommandClass
	LanePolicy        LanePolicy
}

func NewTopic[Command any, Data any, Result any](owner string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, Data, Result] {
	return Topic[Command, Data, Result]{
		Name:              topic,
		Owner:             owner,
		DefinitionPackage: definitionPackage,
		CommandClass:      class,
		LanePolicy:        lane,
	}
}

func CommandTopic[Command any, Result any](owner string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, NoData, Result] {
	return NewTopic[Command, NoData, Result](owner, definitionPackage, class, lane, topic)
}

func DataTopic[Data any](owner string, definitionPackage string, lane LanePolicy, topic string) Topic[NoCommand, Data, NoResult] {
	return NewTopic[NoCommand, Data, NoResult](owner, definitionPackage, CommandClassNotify, lane, topic)
}

func FacadeTopic[Command any, Result any](owner string, definitionPackage string, topic string) Topic[Command, NoData, Result] {
	return NewTopic[Command, NoData, Result](owner, definitionPackage, CommandClassFacade, LanePolicyTopic, topic)
}

func (t Topic[Command, Data, Result]) EventContract() Contract {
	return Contract{
		Topic:        t.Name,
		Owner:        t.Owner,
		CommandClass: t.CommandClass,
		LanePolicy:   t.LanePolicy,
	}
}

func (t Topic[Command, Data, Result]) MessageContract() MessageContract {
	return MessageContract{
		Topic:             t.Name,
		Owner:             t.Owner,
		DefinitionPackage: t.DefinitionPackage,
		CommandClass:      t.CommandClass,
		LanePolicy:        t.LanePolicy,
	}
}

func MessageContracts(topics ...TypedTopic) []MessageContract {
	ret := make([]MessageContract, 0, len(topics))
	for _, topic := range topics {
		if topic == nil {
			continue
		}
		contract := topic.MessageContract()
		if contract.Topic == "" {
			continue
		}
		ret = append(ret, contract)
	}
	return ret
}
