package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapClosureTraces(snapshot readmodelcore.RunSnapshotState) []workorchprojectioncontract.ClosureTrace {
	ret := make([]workorchprojectioncontract.ClosureTrace, 0)
	for _, step := range snapshot.Steps {
		trace, ok := closureTraceFromStep(step)
		if ok {
			ret = append(ret, trace)
		}
	}
	return ret
}
