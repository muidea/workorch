package server

import engine "github.com/muidea/magicEngine/http"

func registerBusinessWriteRoutes(router engine.RouteRegistry, state *serverState) {
	registerDefinitionAuthoringRoutes(router, state)
	registerWorkspaceSetupDraftRoutes(router, state)
	registerGovernanceWriteRoutes(router, state)
	registerMemoryWriteRoutes(router, state)
	registerMCPWriteRoutes(router, state)
	registerCatalogWriteRoutes(router, state)
	registerWorkspaceWriteRoutes(router, state)
}
