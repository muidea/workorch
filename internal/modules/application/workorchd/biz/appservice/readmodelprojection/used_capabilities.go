package readmodelprojection

import (
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func usedRuntimeCapabilities(snapshot readmodelcore.RunSnapshotState, run *runmodel.Run, catalog map[string]projectioncontract.RuntimeCapability) []projectioncontract.RuntimeCapability {
	selected := runtimeCapabilityIndex(snapshot.SelectedCapabilities)
	used := map[string]projectioncontract.RuntimeCapability{}
	order := make([]string, 0)
	if len(snapshot.ToolCalls) > 0 {
		for _, toolCall := range snapshot.ToolCalls {
			identity := runtimeCapabilityIdentity(toolCall)
			if identity.ID == "" {
				continue
			}
			if _, exists := used[identity.ID]; !exists {
				order = append(order, identity.ID)
			}
			used[identity.ID] = mergeUsedRuntimeCapability(used[identity.ID], identity, catalog, selected, toolCall.ID, toolCall.StepID)
		}
	} else if run != nil && run.RuntimeState != nil {
		for _, dispatch := range run.RuntimeState.CompletedDispatches {
			if dispatch.State != runtimecontract.RuntimeDispatchStateCommitted {
				continue
			}
			for _, operation := range runtimeCandidateOperations(dispatch.Candidate) {
				if !slices.Contains(dispatch.CompletedOperationIDs, operation.ID) {
					continue
				}
				identity := runtimeCapabilityIdentityFromOperation(operation)
				if identity.ID == "" {
					continue
				}
				if _, exists := used[identity.ID]; !exists {
					order = append(order, identity.ID)
				}
				evidenceRef := "workorch://runtime-operations/" + strings.TrimSpace(dispatch.ID) + "/" + strings.TrimSpace(operation.ID)
				used[identity.ID] = mergeUsedRuntimeCapability(used[identity.ID], identity, catalog, selected, evidenceRef, dispatch.Transaction.AuthoritySnapshotID)
			}
		}
	}
	ret := make([]projectioncontract.RuntimeCapability, 0, len(order))
	for _, id := range order {
		ret = append(ret, used[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateOperations(candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeOperation {
	ret := make([]runtimecontract.RuntimeOperation, 0)
	if candidate.StateChange != nil {
		for _, item := range candidate.StateChange.Operations {
			ret = append(ret, item.Operation)
		}
	}
	if candidate.Validation != nil {
		for _, item := range candidate.Validation.Operations {
			ret = append(ret, item.Operation)
		}
	}
	return ret
}

func runtimeCapabilityIdentityFromOperation(operation runtimecontract.RuntimeOperation) projectioncontract.RuntimeCapability {
	source := map[string]any{"operation": operation.Kind}
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" || parameter.String == nil {
			continue
		}
		source[name] = strings.TrimSpace(*parameter.String)
	}
	serverID := readModelStringValue(source["serverId"])
	kind, name := runtimeCapabilityKindAndName(operation.Kind, source)
	if serverID == "" || kind == "" || name == "" {
		return projectioncontract.RuntimeCapability{}
	}
	return projectioncontract.RuntimeCapability{
		ID: defaultRuntimeCapabilityID(serverID, kind, name), ServerID: serverID,
		Kind: kind, Name: name, DisplayName: name,
	}
}
