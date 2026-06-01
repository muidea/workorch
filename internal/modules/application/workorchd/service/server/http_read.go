package server

import engine "github.com/muidea/magicEngine/http"

func registerReadRoutes(router engine.RouteRegistry, state *serverState) {
	registerEventReadRoutes(router, state)
	registerGovernanceReadRoutes(router, state)
	registerMemoryReadRoutes(router, state)
	registerMCPReadRoutes(router, state)
	registerCatalogReadRoutes(router, state)
	registerWorkspaceReadRoutes(router, state)
	registerRunReadRoutes(router, state)
}
