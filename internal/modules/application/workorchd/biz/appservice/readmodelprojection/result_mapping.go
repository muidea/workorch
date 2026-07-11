package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func MapResults(items []*resultmodel.Result) []readmodelcore.Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.Result, 0, len(items))
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
