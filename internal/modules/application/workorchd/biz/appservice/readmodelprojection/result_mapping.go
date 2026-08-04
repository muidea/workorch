package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapResults(items []*resultmodel.Result) []projectioncontract.Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Result, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, mapResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
