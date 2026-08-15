package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Configuration/catalog operations are application-module adapters. They
// return typed values and never mutate Runtime authority.
func (s *serverState) configureConfigurationOperations(session *runtimecore.Session) {
	s.getLLMModelCatalog = func(ctx context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error) {
		var ret workorchmanagementcontract.ProviderCatalogSummary
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.GetLLMModelCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.refreshLLMModelCatalog = func(ctx context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error) {
		var ret workorchmanagementcontract.ProviderCatalogSummary
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.RefreshLLMModelCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.readConfigurationWithCatalog = func(ctx context.Context) (workorchconfigurationcontract.ServiceConfiguration, error) {
		var ret workorchconfigurationcontract.ServiceConfiguration
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadServiceConfigurationWithCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.validate = func(ctx context.Context, _, _ string) error {
		return withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			_, err := session.Exports()
			return err
		})
	}
}
