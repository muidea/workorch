package events

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindWebSearch && operation.Kind != runtimecontract.StepKindWebFetch {
		return OperationData{}, fmt.Errorf("operation kind %q is not a web operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{SafeSearch: true}
	for _, p := range operation.Parameters {
		switch p.Name {
		case "providerId":
			ret.ProviderID = *p.String
		case "query":
			ret.Query = *p.String
		case "locale":
			ret.Locale = *p.String
		case "recency":
			ret.Recency = *p.String
		case "domains":
			ret.Domains = append([]string(nil), p.StringList...)
		case "excludeDomains":
			ret.ExcludeDomains = append([]string(nil), p.StringList...)
		case "maxResults":
			ret.MaxResults = *p.Integer
		case "safeSearch":
			ret.SafeSearch = *p.Boolean
		case "approvalId":
			ret.ApprovalID = common.ApprovalID(*p.String)
		case "url":
			ret.URL = *p.String
		case "maxBytes":
			ret.MaxBytes = *p.Integer
		case "extractMode":
			ret.ExtractMode = *p.String
		}
	}
	return ret, nil
}
