package appservice

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) runTypedCLI() (err error) {
	defer func() {
		if value := recover(); value != nil {
			if failure, ok := value.(*cd.Error); ok {
				err = failure
				return
			}
			panic(value)
		}
	}()
	if d == nil || d.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if d.typedReady {
		return nil
	}
	if len(d.topics) != 0 {
		return common.ErrConflict
	}
	if err := d.lifecycle.Reset(); err != nil {
		return err
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIWorkspaceGet,
		workorchdevents.TopicCLIWorkspaceList,
		workorchdevents.TopicCLIWorkspaceSnapshot,
		workorchdevents.TopicCLIWorkspaceDelete,
		workorchdevents.TopicCLIWorkspaceUpdateLLMSelection,
	} {
		d.subscribeTypedCLI(topic, d.handleTypedCLIWorkspace)
	}
	d.subscribeTypedCLI(workorchdevents.TopicWorkorchDCommandReadWorkspaceRuntimeView, d.handleTypedWorkspaceRuntimeView)
	for _, topic := range []string{
		workorchdevents.TopicCLIGoalList,
		workorchdevents.TopicCLIGoalGet,
		workorchdevents.TopicCLIDoDList,
		workorchdevents.TopicCLIDoDGet,
	} {
		d.subscribeTypedCLI(topic, d.handleTypedCLIDefinition)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIRunGet,
		workorchdevents.TopicCLIRunList,
		workorchdevents.TopicCLIRunStatus,
	} {
		d.subscribeTypedCLI(topic, d.handleTypedCLIRun)
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
		d.subscribeTypedCLI(topic, d.handleTypedCLIGovernance)
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
		d.subscribeTypedCLI(topic, d.handleTypedCLIContextRegistry)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLIMemoryCreate,
		workorchdevents.TopicCLIMemoryGet,
		workorchdevents.TopicCLIMemoryUpdate,
		workorchdevents.TopicCLIMemoryDelete,
		workorchdevents.TopicCLIMemoryList,
		workorchdevents.TopicCLIMemorySearch,
	} {
		d.subscribeTypedCLI(topic, d.handleTypedCLIMemory)
	}
	for _, topic := range []string{
		workorchdevents.TopicCLITemplateCreate,
		workorchdevents.TopicCLITemplateGet,
		workorchdevents.TopicCLITemplateList,
		workorchdevents.TopicCLITemplateUpdate,
		workorchdevents.TopicCLITemplateDelete,
	} {
		d.subscribeTypedCLI(topic, d.handleTypedCLITemplate)
	}
	d.subscribeTypedCLI(workorchdevents.TopicCLIConfigurationGet, d.handleTypedCLIConfiguration)
	d.subscribeTypedCLI(workorchdevents.TopicCLIConfigurationRefreshChatModels, d.handleTypedCLIConfiguration)
	d.typedReady = true
	return nil
}

func (d *CLICommandDispatcher) subscribeTypedCLI(topic string, handler magicevent.ObserverFunc) {
	err := d.SubscribeFuncChecked(topic, func(event magicevent.Event, result magicevent.Result) {
		leave, err := d.lifecycle.Enter()
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "cli ingress is closing"))
			}
			return
		}
		defer leave()
		handler(event, result)
	})
	if err != nil {
		panic(err)
	}
	d.topics = append(d.topics, topic)
}

func (d *CLICommandDispatcher) closeTypedCLI() error {
	if d == nil {
		return nil
	}
	for len(d.topics) > 0 {
		last := len(d.topics) - 1
		if err := d.UnsubscribeFuncChecked(d.topics[last]); err != nil {
			return err
		}
		d.topics = d.topics[:last]
	}
	d.typedReady = false
	return nil
}

func setTypedCLIError(result magicevent.Result, err error) bool {
	if err == nil {
		return false
	}
	result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
	return true
}

func setRetiredDefinitionWrite(result magicevent.Result) {
	if result != nil {
		result.Set(nil, cd.NewError(cd.Unexpected, common.ErrConflict.Error()+": 旧规格写操作已退休，请通过 Web 持久规格创作完成审查与确认"))
	}
}

func setInvalidTypedCLICommand(result magicevent.Result) {
	result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
}
