package shell

import (
	"fmt"
	"path/filepath"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func inspectShellValidationInputPostimage(r rc.RuntimeValidationInputStructureRequest) (rc.RuntimeValidationInputStructureObservation, error) {
	if err := r.Validate(); err != nil {
		return rc.RuntimeValidationInputStructureObservation{}, err
	}
	files := make(map[string]string)
	var refs []string
	for _, f := range r.Revision.InputFiles() {
		if _, exists := files[f.TargetRef]; exists {
			return rc.RuntimeValidationInputStructureObservation{}, fmt.Errorf("duplicate frozen source")
		}
		files[f.TargetRef] = f.Content
		refs = append(refs, f.TargetRef)
	}
	if _, found := files[r.Postimage.TargetRef]; !found {
		return rc.RuntimeValidationInputStructureObservation{}, common.ErrInvalidArgument
	}
	files[r.Postimage.TargetRef] = r.Postimage.Content
	// Pure declaration and assertion readiness only, never semantic consistency.
	inspection := inspectShellNativeValidationSources(refs, r.Revision.Binding.CheckAttestations, func(ref string) ([]byte, error) {
		if filepath.IsAbs(ref) || strings.Contains(ref, "\\") {
			return nil, common.ErrInvalidArgument
		}
		body, found := files[ref]
		if !found {
			return nil, common.ErrInvalidArgument
		}
		return []byte(body), nil
	})
	return rc.RuntimeValidationInputStructureObservation{RequestHash: r.Hash(), OwnerModuleID: ModuleID, Code: inspection.Code, CheckRef: inspection.CheckRef, TargetRefs: inspection.TargetRefs, Proof: r.Proof.Clone(), Postimage: r.Postimage}, nil
}

func (m *Biz) handleRuntimeValidationInputStructure(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.InspectRuntimeValidationInputStructureCommand](event, result)
	if !ok {
		return
	}
	observation, err := inspectShellValidationInputPostimage(rc.RuntimeValidationInputStructureRequest(command))
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(shellevents.InspectRuntimeValidationInputStructureResult{Observation: observation}, nil)
}
