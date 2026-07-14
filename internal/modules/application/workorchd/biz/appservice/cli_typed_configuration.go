package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func (d *CLICommandDispatcher) handleTypedCLIConfiguration(event magicevent.Event, result magicevent.Result) {
	if _, ok := event.Data().(workorchdevents.CLIConfigurationGetCommand); !ok {
		setInvalidTypedCLICommand(result)
		return
	}
	item, err := ReadServiceConfiguration()
	if setTypedCLIError(result, err) {
		return
	}
	result.Set(workorchdevents.CLIConfigurationGetResult{Configuration: readmodelports.PortServiceConfig(item)}, nil)
}
