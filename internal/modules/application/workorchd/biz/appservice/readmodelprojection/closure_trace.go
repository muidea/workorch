package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapClosureTraces(snapshot readmodelcore.RunSnapshot) []readmodelcore.ClosureTrace {
	ret := make([]readmodelcore.ClosureTrace, 0)
	for _, step := range snapshot.Steps {
		trace, ok := closureTraceFromStep(step)
		if ok {
			ret = append(ret, trace)
		}
	}
	return ret
}
