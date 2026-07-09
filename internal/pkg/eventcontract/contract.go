package eventcontract

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
