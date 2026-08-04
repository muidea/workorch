package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func (d *CLICommandDispatcher) handleTypedCLIConfiguration(event magicevent.Event, result magicevent.Result) {
	switch command := event.Data().(type) {
	case workorchdevents.CLIConfigurationGetCommand:
		item, err := ReadServiceConfigurationWithCatalog(event.Context(), d.EventHub())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIConfigurationGetResult{Configuration: readmodelports.PortServiceConfig(item)}, nil)
	case workorchdevents.CLIConfigurationRefreshChatModelsCommand:
		if _, err := RefreshLLMModelCatalog(event.Context(), d.EventHub(), configcore.RefreshLLMModelCatalogCommand{ProviderID: command.ProviderID}); setTypedCLIError(result, err) {
			return
		}
		item, err := ReadServiceConfigurationWithCatalog(event.Context(), d.EventHub())
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIConfigurationRefreshChatModelsResult{Configuration: readmodelports.PortServiceConfig(item)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
