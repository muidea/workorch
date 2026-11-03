package result

import (
	"errors"
	"reflect"
	"strings"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func resultDeliveryFailure(err error, stage resultevents.TerminalDeliveryStage, id common.ResultID, fields ...string) error {
	if err == nil {
		return nil
	}
	var existing *resultevents.TerminalDeliveryFailure
	if errors.As(err, &existing) {
		return err // Preserve the most precise inner boundary and original cause.
	}
	return &resultevents.TerminalDeliveryFailure{Stage: stage, ResultID: id, MismatchFields: append([]string(nil), fields...), Cause: err}
}

// This is the same immutable comparison used for admission, with exact field
// paths for diagnosis. Do not normalize or repair either value to hide drift.
func terminalResultMismatchFields(left, right *resultmodel.Result) []string {
	if left == nil || right == nil {
		return []string{"result"}
	}
	var fields []string
	for _, check := range []struct {
		field string
		equal bool
	}{
		{"ID", left.ID == right.ID}, {"WorkspaceID", left.WorkspaceID == right.WorkspaceID},
		{"RunID", left.RunID == right.RunID}, {"GoalID", left.GoalID == right.GoalID}, {"DoDID", left.DoDID == right.DoDID},
		{"Title", left.Title == right.Title}, {"Summary", left.Summary == right.Summary},
		{"SourceType", left.SourceType == right.SourceType}, {"SourceRef", left.SourceRef == right.SourceRef},
		{"EvidenceRefs", reflect.DeepEqual(left.EvidenceRefs, right.EvidenceRefs)},
		{"ArtifactRefs", reflect.DeepEqual(left.ArtifactRefs, right.ArtifactRefs)},
		{"DoDAssessment", reflect.DeepEqual(left.DoDAssessment, right.DoDAssessment)},
		{"SummaryDetail", reflect.DeepEqual(left.SummaryDetail, right.SummaryDetail)},
		{"RuntimeCloseout", reflect.DeepEqual(left.RuntimeCloseout, right.RuntimeCloseout)},
		{"TerminalMaterialHash", strings.TrimSpace(left.TerminalMaterialHash) == strings.TrimSpace(right.TerminalMaterialHash)},
		{"TerminalDelivery", sameTerminalDeliveryIdentity(left.TerminalDelivery, right.TerminalDelivery)},
	} {
		if !check.equal {
			fields = append(fields, check.field)
		}
	}
	return fields
}
