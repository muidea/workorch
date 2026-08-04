package server

import (
	"context"
	"net/http"
	"strings"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func registerGovernanceReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/decisions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		state.metrics.decisionListRequests.Add(1)
		query := decisionQueryFromRequest(r)
		page, err := state.listDecisionPage(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Decisions: page.Decisions,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.decisionGetRequests.Add(1)
		item, err := state.readDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: appservice.ProjectDecisionView(item),
		})
	})
	router.AddHandler("/api/v1/workorch/approvals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		state.metrics.approvalListRequests.Add(1)
		query := approvalQueryFromRequest(r)
		page, err := state.listApprovalPage(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Approvals: page.Approvals,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalGetRequests.Add(1)
		item, err := state.readApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
}

func registerGovernanceWriteRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/decisions/:ticketId/resolve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "resolve" {
			http.NotFound(w, r)
			return
		}
		var req appmodelcore.ResolveDecisionCommand
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.decisionResolveRequests.Add(1)
		item, err := state.resolveDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspacesForDecisions(r.Context(), []appmodelcore.Decision{item})
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: appservice.ProjectDecisionView(item),
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/resolve-batch", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		var req appmodelcore.BatchResolveDecisionCommand
		if err := decodeJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.decisionResolveRequests.Add(uint64(len(req.TicketIDs)))
		items, err := state.batchResolveDecisions(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspacesForDecisions(r.Context(), items)
		writeJSON(w, http.StatusOK, decisionsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Decisions: projectDecisionViews(items),
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/approve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "approve" {
			http.NotFound(w, r)
			return
		}
		var req resolveApprovalRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.approvalApproveRequests.Add(1)
		item, err := state.approveApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID, appmodelcore.ResolveApprovalCommand{
			Action:  "approve",
			Payload: req.Payload,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspacesForApprovals(r.Context(), []appmodelcore.Approval{item})
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/reject", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "reject" {
			http.NotFound(w, r)
			return
		}
		var req resolveApprovalRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.approvalRejectRequests.Add(1)
		item, err := state.rejectApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID, appmodelcore.ResolveApprovalCommand{
			Action:  "reject",
			Payload: req.Payload,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspacesForApprovals(r.Context(), []appmodelcore.Approval{item})
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/resolve-batch", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(governanceRequestContext(r, state))
		var req appmodelcore.BatchResolveApprovalCommand
		if err := decodeJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		if req.Action == "approve" {
			state.metrics.approvalApproveRequests.Add(uint64(len(req.ApprovalIDs)))
		} else if req.Action == "reject" {
			state.metrics.approvalRejectRequests.Add(uint64(len(req.ApprovalIDs)))
		}
		items, err := state.batchResolveApprovals(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.rebuildProjectedWorkspacesForApprovals(r.Context(), items)
		writeJSON(w, http.StatusOK, approvalsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Approvals: items,
		})
	})
}

func projectDecisionViews(items []appmodelcore.Decision) []appmodelcore.Decision {
	if len(items) == 0 {
		return nil
	}
	ret := make([]appmodelcore.Decision, 0, len(items))
	for _, item := range items {
		ret = append(ret, appservice.ProjectDecisionView(item))
	}
	return ret
}

func governanceRequestContext(r *http.Request, state *serverState) context.Context {
	ctx := r.Context()
	if workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId")); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	}
	return ctx
}
