package readmodelprojection

import (
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapWebSourceTraces(steps []workorchprojectioncontract.StepView) []workorchprojectioncontract.WebSourceTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]workorchprojectioncontract.WebSourceTrace, 0)
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
