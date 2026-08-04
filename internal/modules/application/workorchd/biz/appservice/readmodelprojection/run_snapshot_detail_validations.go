package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectRunSnapshotValidationViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.ValidationResults) == 0 {
		return
	}
	limit := min(len(ret.ValidationResults), validationPageLimit)
	projected := make([]projectioncontract.ValidationResult, 0, limit)
	for _, item := range ret.ValidationResults[:limit] {
		projected = append(projected, ProjectValidationResultView(item))
	}
	ret.ValidationResults = projected
}
