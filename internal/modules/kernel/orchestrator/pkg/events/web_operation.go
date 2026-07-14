package events

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func WebOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (WebOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindWebSearch && operation.Kind != runtimecontract.StepKindWebFetch {
		return WebOperationData{}, fmt.Errorf("operation kind %q is not a web operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return WebOperationData{}, err
	}
	ret := WebOperationData{SafeSearch: true}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "providerId":
			ret.ProviderID = *parameter.String
		case "query":
			ret.Query = *parameter.String
		case "locale":
			ret.Locale = *parameter.String
		case "recency":
			ret.Recency = *parameter.String
		case "domains":
			ret.Domains = append([]string(nil), parameter.StringList...)
		case "excludeDomains":
			ret.ExcludeDomains = append([]string(nil), parameter.StringList...)
		case "maxResults":
			ret.MaxResults = *parameter.Integer
		case "safeSearch":
			ret.SafeSearch = *parameter.Boolean
		case "approvalId":
			ret.ApprovalID = common.ApprovalID(*parameter.String)
		case "url":
			ret.URL = *parameter.String
		case "maxBytes":
			ret.MaxBytes = *parameter.Integer
		case "extractMode":
			ret.ExtractMode = *parameter.String
		}
	}
	return ret, nil
}
