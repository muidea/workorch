package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (p *runtimeApplicationPorts) configureConfigurationPorts() {
	ports := &p.ports
	ports.Configuration.ReadConfiguration = appservice.ReadServiceConfiguration
	ports.Configuration.UpdateServerConfiguration = appservice.UpdateServerConfiguration
	ports.Configuration.UpdateRuntimeConfiguration = appservice.UpdateRuntimeConfiguration
	ports.Configuration.UpdateLLMConfiguration = appservice.UpdateLLMConfiguration
	ports.Configuration.UpdateWebSearchConfiguration = appservice.UpdateWebSearchConfiguration
	ports.Configuration.GetLLMModelCatalog = func(ctx context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error) {
		var ret workorchmanagementcontract.ProviderCatalogSummary
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.GetLLMModelCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Configuration.RefreshLLMModelCatalog = func(ctx context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error) {
		var ret workorchmanagementcontract.ProviderCatalogSummary
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.RefreshLLMModelCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Configuration.ReadConfigurationWithCatalog = func(ctx context.Context) (workorchconfigurationcontract.ServiceConfiguration, error) {
		var ret workorchconfigurationcontract.ServiceConfiguration
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadServiceConfigurationWithCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	ports.Workspace.Validate = func(ctx context.Context, _, _ string) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			_, err := session.Exports()
			return err
		})
	}
}
