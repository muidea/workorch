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
