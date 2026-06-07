package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotValidationViews(ret *readmodelcore.RunSnapshot) {
	if len(ret.ValidationResults) == 0 {
		return
	}
	limit := min(len(ret.ValidationResults), validationPageLimit)
	projected := make([]readmodelcore.ValidationResult, 0, limit)
	for _, item := range ret.ValidationResults[:limit] {
		projected = append(projected, ProjectValidationResultView(item))
	}
	ret.ValidationResults = projected
}
