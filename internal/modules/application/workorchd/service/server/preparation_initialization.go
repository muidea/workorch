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

func registerPreparationInitializationRoutes(router engine.RouteRegistry, state *serverState) {
	for _, action := range []string{"initialize-resources", "recover-resources"} {
		router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/activation-preparation/"+action, http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
			id, selected, ok := workspaceResourceIDFromPath(r.URL.Path, "activation-preparation")
			if !ok || selected != action {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			port := state.initializeStartupPreparationResources
			if action == "recover-resources" {
				port = state.recoverStartupPreparationResources
			}
			if port == nil {
				writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "resource initialization unavailable"})
				return
			}
			var c workorchmanagementcontract.PreparationCheckRequest
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			defer r.Body.Close()
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&c) != nil || c.Validate() != nil {
				writeBusinessError(w, common.ErrInvalidArgument)
				return
			}
			var extra json.RawMessage
			if decoder.Decode(&extra) != io.EOF {
				writeBusinessError(w, common.ErrInvalidArgument)
				return
			}
			value, err := port(r.Context(), id, c)
			if err != nil {
				switch {
				case errors.Is(err, common.ErrNotFound):
					writeBusinessError(w, common.ErrNotFound)
				case errors.Is(err, common.ErrInvalidArgument):
					writeBusinessError(w, common.ErrInvalidArgument)
				case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrBlocked):
					writeBusinessError(w, common.ErrConflict)
				default:
					writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "resource initialization response unavailable; reconcile original request"})
				}
				return
			}
			if value.ValidateFor(id, c) != nil {
				writeBusinessError(w, common.ErrConflict)
				return
			}
			status := http.StatusOK
			for _, r := range value.Preparation.ResourceInitializations {
				if r.RequestID == c.RequestID && r.State == "pending" {
					status = http.StatusAccepted
				}
			}
			writeJSON(w, status, value)
		})
	}
}
