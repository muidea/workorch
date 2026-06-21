package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := eventcontract.Build("workorchd", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic,
		TopicWorkorchDCommandDone,
	)
	ret = append(ret, eventcontract.Build("workorchd.scheduler", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicWorkorchDCommandWorkspaceRunRequested,
	)...)
	ret = append(ret, eventcontract.Build("workorchd.workspace_projection", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicWorkspaceProjectionChanged,
	)...)
	ret = append(ret, eventcontract.Build("cli", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic,
		TopicCLICommand,
		TopicCLIResult,
		TopicCLIProgress,
	)...)
	return ret
}
