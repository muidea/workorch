package backend

import (
	"github.com/muidea/workorch/internal/pkg/common"
	client "github.com/muidea/workorch/internal/pkg/workorchclient"
)

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func commonSoulBindings(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		if value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func workspaceInfoFromClient(ws *client.Workspace) WorkspaceInfo {
	if ws == nil {
		return WorkspaceInfo{}
	}
	return WorkspaceInfo{
		ID:                   common.WorkspaceID(ws.ID),
		Title:                ws.Title,
		Description:          ws.Description,
		DeliverableDirectory: ws.DeliverableDirectory,
		State:                common.WorkspaceState(ws.State),
		ActiveRunID:          common.RunID(ws.ActiveRunID),
		BlockingTicketID:     common.TicketID(ws.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(ws.BlockingApprovalID),
		TemplateID:           ws.TemplateID,
		ProfileID:            ws.ProfileID,
		DefaultSoulID:        common.SoulID(ws.DefaultSoulID),
		SoulBindings:         commonSoulBindings(ws.SoulBindings),
	}
}

func projectionInfoFromClient(input client.ProjectionStatus) ProjectionInfo {
	return ProjectionInfo{
		State:                   input.State,
		GlobalRevision:          input.GlobalRevision,
		WorkspaceRevision:       input.WorkspaceRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
		UpdatedAt:               input.UpdatedAt,
		RebuiltAt:               input.RebuiltAt,
		Degraded:                input.Degraded,
		StaleReason:             input.StaleReason,
		Error:                   input.Error,
	}
}

func cloneTemplateEvidencePolicies(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = cloneTemplateEvidencePolicyValue(value)
		}
		ret = append(ret, cloned)
	}
	return ret
}

func cloneTemplateEvidencePolicyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, value := range typed {
			cloned[key] = cloneTemplateEvidencePolicyValue(value)
		}
		return cloned
	case []map[string]any:
		return cloneTemplateEvidencePolicies(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneTemplateEvidencePolicyValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
