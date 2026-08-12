package runtimecore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextIDs(items []map[string]any, key string) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if id, _ := item[key].(string); strings.TrimSpace(id) != "" {
			ret = append(ret, strings.TrimSpace(id))
		}
	}
	return ret
}

func StringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func ContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func CloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func MapRun(item *runmodel.Run) appmodelcore.Run {
	if item == nil {
		return appmodelcore.Run{}
	}
	ret := appmodelcore.Run{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		GoalID:      string(item.GoalID),
		DoDID:       string(item.DoDID),
		Lineage: projectioncontract.RunLineage{
			PredecessorRunID:    string(item.Lineage.PredecessorRunID),
			PredecessorResultID: string(item.Lineage.PredecessorResultID),
			SuccessorRunID:      string(item.Lineage.SuccessorRunID),
			LineageState:        item.Lineage.LineageState,
		},
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		State:              string(common.CanonicalRunState(item.State)),
		Priority:           item.Priority,
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		EndedAt:            item.EndedAt,
	}
	if item.RuntimeState != nil {
		state := item.RuntimeState.Clone()
		ret.RuntimeState = &state
	}
	if item.RuntimeBootstrap != nil {
		selection := item.RuntimeBootstrap.Activation.ProviderSelection.Normalized()
		if selection.Validate() == nil {
			ret.SealedLLMSelection = &selection
		}
	}
	if item.RuntimeActivationStatus != nil {
		ret.RuntimeActivation = &projectioncontract.RuntimeActivationStatus{
			Disposition:            string(item.RuntimeActivationStatus.Disposition),
			Reason:                 string(item.RuntimeActivationStatus.Incompatibility.Reason),
			ActualSchemaVersion:    item.RuntimeActivationStatus.Incompatibility.ActualSchemaVersion,
			SupportedSchemaVersion: item.RuntimeActivationStatus.Incompatibility.SupportedSchemaVersion,
		}
	}
	return ret
}
