package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type startupPreparationResponse struct {
	Preparation workorchmanagementcontract.StartupPreparationView `json:"preparation"`
}

func registerStartupPreparationWriteRoutes(router engine.RouteRegistry, state *serverState) {
	registerPreparationInitializationRoutes(router, state)
	for _, action := range []string{"check", "recover"} {
		router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/activation-preparation/"+action, http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
			id, selected, ok := workspaceResourceIDFromPath(r.URL.Path, "activation-preparation")
			if !ok || selected != action {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			port := state.checkStartupPreparation
			if action == "recover" {
				port = state.recoverStartupPreparation
			}
			if port == nil {
				writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "startup preparation command unavailable"})
				return
			}
			var request workorchmanagementcontract.PreparationCheckRequest
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			defer r.Body.Close()
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil || request.Validate() != nil {
				writeBusinessError(w, common.ErrInvalidArgument)
				return
			}
			var trailing json.RawMessage
			if err := decoder.Decode(&trailing); err != io.EOF {
				writeBusinessError(w, common.ErrInvalidArgument)
				return
			}
			operation, err := port(r.Context(), id, request)
			if err != nil {
				// Even a 409/500 may follow an accepted CAS. The caller retains
				// the original intent and reconciles with GET; no raw diagnostics.
				switch {
				case errors.Is(err, common.ErrNotFound):
					writeBusinessError(w, common.ErrNotFound)
				case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrBlocked):
					writeBusinessError(w, common.ErrConflict)
				case errors.Is(err, common.ErrInvalidArgument):
					writeBusinessError(w, common.ErrInvalidArgument)
				default:
					writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "preparation response unavailable; reconcile original request"})
				}
				return
			}
			if operation.ValidateFor(id, request) != nil {
				writeBusinessError(w, common.ErrConflict)
				return
			}
			status := http.StatusOK
			for _, a := range operation.Preparation.Record.Attempts {
				if a.RequestID == request.RequestID && (a.State == "accepted" || a.State == "unknown") {
					status = http.StatusAccepted
				}
			}
			writeJSON(w, status, operation)
		})
	}
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
