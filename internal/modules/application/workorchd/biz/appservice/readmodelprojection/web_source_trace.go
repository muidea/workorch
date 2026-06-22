package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
)

func MapWebSourceTraces(steps []StepView) []readmodelcore.WebSourceTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]readmodelcore.WebSourceTrace, 0)
	for _, step := range steps {
		if step.Output == nil {
			continue
		}
		switch step.Kind {
		case webcommon.StepKindSearch:
			trace := webSearchTrace(step)
			if len(trace.Sources) > 0 || trace.Query != "" {
				ret = append(ret, trace)
			}
		case webcommon.StepKindFetch:
			trace := webFetchTrace(step)
			if len(trace.FetchedSources) > 0 || trace.ApprovalID != "" {
				ret = append(ret, trace)
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
