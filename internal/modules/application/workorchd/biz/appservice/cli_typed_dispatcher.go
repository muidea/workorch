package appservice

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) runTypedCLI() error {
	if d == nil || d.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if d.typedReady {
		return nil
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIWorkspaceCreate,
		workorchdevents.TopicCLIWorkspaceGet,
		workorchdevents.TopicCLIWorkspaceList,
		workorchdevents.TopicCLIWorkspaceSnapshot,
		workorchdevents.TopicCLIWorkspaceDelete,
		workorchdevents.TopicCLIWorkspaceUpdateLLMTarget,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIWorkspace)
	}
	d.SubscribeFunc(workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView, d.handleTypedWorkspaceRuntimeView)
	for _, topic := range []string{
		workorchdevents.TopicCLIGoalDefine,
		workorchdevents.TopicCLIGoalList,
		workorchdevents.TopicCLIGoalGet,
		workorchdevents.TopicCLIGoalUpdate,
		workorchdevents.TopicCLIDoDDefine,
		workorchdevents.TopicCLIDoDList,
		workorchdevents.TopicCLIDoDGet,
		workorchdevents.TopicCLIDoDUpdate,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIDefinition)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIRunGet,
		workorchdevents.TopicCLIRunList,
		workorchdevents.TopicCLIRunStatus,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIRun)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIDecisionResolve,
		workorchdevents.TopicCLIDecisionBatchResolve,
		workorchdevents.TopicCLIDecisionList,
		workorchdevents.TopicCLIDecisionGet,
		workorchdevents.TopicCLIApprovalApprove,
		workorchdevents.TopicCLIApprovalReject,
		workorchdevents.TopicCLIApprovalBatchResolve,
		workorchdevents.TopicCLIApprovalList,
		workorchdevents.TopicCLIApprovalGet,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIGovernance)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLISoulCreate,
		workorchdevents.TopicCLISoulGet,
		workorchdevents.TopicCLISoulList,
		workorchdevents.TopicCLISoulUpdate,
		workorchdevents.TopicCLISoulDelete,
		workorchdevents.TopicCLISkillCreate,
		workorchdevents.TopicCLISkillGet,
		workorchdevents.TopicCLISkillList,
		workorchdevents.TopicCLIMCPServerCreate,
		workorchdevents.TopicCLIMCPServerGet,
		workorchdevents.TopicCLIMCPServerList,
		workorchdevents.TopicCLIMCPServerUpdate,
		workorchdevents.TopicCLIMCPCapabilityList,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIContextRegistry)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIMemoryCreate,
		workorchdevents.TopicCLIMemoryGet,
		workorchdevents.TopicCLIMemoryUpdate,
		workorchdevents.TopicCLIMemoryDelete,
		workorchdevents.TopicCLIMemoryList,
		workorchdevents.TopicCLIMemorySearch,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLIMemory)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLITemplateCreate,
		workorchdevents.TopicCLITemplateGet,
		workorchdevents.TopicCLITemplateList,
		workorchdevents.TopicCLITemplateUpdate,
		workorchdevents.TopicCLITemplateDelete,
	} {
		d.SubscribeFunc(topic, d.handleTypedCLITemplate)
	}
	d.SubscribeFunc(workorchdevents.TopicCLIConfigurationGet, d.handleTypedCLIConfiguration)
	d.SubscribeFunc(workorchdevents.TopicCLIConfigurationRefreshChatModels, d.handleTypedCLIConfiguration)
	d.typedReady = true
	return nil
}

func (d *CLICommandDispatcher) closeTypedCLI() {
	if d == nil || !d.typedReady {
		return
	}
	for _, topic := range workorchdevents.CLITopics() {
		d.UnsubscribeFunc(topic)
	}
	d.UnsubscribeFunc(workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView)
	d.typedReady = false
}

func setTypedCLIError(result magicevent.Result, err error) bool {
	if err == nil {
		return false
	}
	result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
	return true
}

func setInvalidTypedCLICommand(result magicevent.Result) {
	result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
}
