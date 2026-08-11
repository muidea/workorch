package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *CLICommandDispatcher) handleTypedCLIConfiguration(event magicevent.Event, result magicevent.Result) {
	switch event.Data().(type) {
	case workorchdevents.CLIConfigurationGetCommand:
		item, err := ReadServiceConfigurationWithCatalog(event.Context(), d.EventHub())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIConfigurationGetResult{Configuration: port.ServiceConfigFromConfiguration(item)}, nil)
	case workorchdevents.CLIConfigurationRefreshChatModelsCommand:
		if _, err := RefreshLLMModelCatalog(event.Context(), d.EventHub()); setTypedCLIError(result, err) {
			return
		}
		item, err := ReadServiceConfigurationWithCatalog(event.Context(), d.EventHub())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIConfigurationRefreshChatModelsResult{Configuration: port.ServiceConfigFromConfiguration(item)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
