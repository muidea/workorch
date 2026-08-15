package web

import (
	"fmt"
	"strings"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func requireWebApprovalFailure(result RuntimeOperationResult, executeErr error) (RuntimeOperationResult, error) {
	if result.ApprovalNeeded && executeErr == nil {
		executeErr = fmt.Errorf("web operation requires approval")
		result.FailureCode = webcommon.ErrorApprovalRequired
		result.FailureMessage = executeErr.Error()
	}
	return result, executeErr
}

func failedWebResult(request RuntimeOperationRequest, provider webmodels.ProviderConfig, code string, err error) RuntimeOperationResult {
	stage := "web_failed"
	if request.Kind == webcommon.StepKindSearch {
		stage = "web_search"
	} else if request.Kind == webcommon.StepKindFetch {
		stage = "web_fetch"
	}
	ret := RuntimeOperationResult{
		Status: "failed", Stage: stage, ProviderID: provider.ID, Query: strings.TrimSpace(request.Query),
		URL: strings.TrimSpace(request.URL), FailureCode: code, TargetRefs: append([]string(nil), request.TargetRefs...),
	}
	if err != nil {
		ret.FailureMessage = err.Error()
	}
	ret.Summary = firstWebNonEmpty(ret.FailureMessage, stage)
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: ret.Summary}
	return ret
}
