package server

import (
	"context"
	"errors"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type startupPreparationResponse struct {
	Preparation workorchmanagementcontract.StartupPreparationView `json:"preparation"`
}

func registerStartupPreparationReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/activation-preparation", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		id, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "activation-preparation" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if state.readStartupPreparation == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "startup preparation reader unavailable"})
			return
		}
		// Never cache operation identities, even when no record exists. GET
		// does not accept caller PASS facts, paths or recovery instructions.
		view, err := state.readStartupPreparation(r.Context(), id)
		if err != nil {
			// Owner diagnostic text is not a public error payload. Preserve it
			// at its source, but return only the classified status here.
			switch {
			case errors.Is(err, common.ErrNotFound):
				writeBusinessError(w, common.ErrNotFound)
			case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrBlocked):
				writeBusinessError(w, common.ErrConflict)
			case errors.Is(err, common.ErrInvalidArgument):
				writeBusinessError(w, common.ErrInvalidArgument)
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			}
			return
		}
		if view.ValidateFor(id) != nil {
			writeBusinessError(w, common.ErrConflict)
			return
		}
		writeJSON(w, http.StatusOK, startupPreparationResponse{Preparation: view})
	})
}
