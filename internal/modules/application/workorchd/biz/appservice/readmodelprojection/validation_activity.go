package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func MapValidationResults(items []*runmodel.ValidationResult) []readmodelcore.ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ProjectValidationResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
