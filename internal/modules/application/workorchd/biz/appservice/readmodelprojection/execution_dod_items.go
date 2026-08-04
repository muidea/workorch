package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func runDoDItems(snapshot readmodelcore.RunSnapshotState) []string {
	if snapshot.Run.DoDID == "" {
		return nil
	}
	for _, item := range snapshot.Results {
		if item.DoDID != snapshot.Run.DoDID || item.DoDAssessment == nil {
			continue
		}
		ret := make([]string, 0, len(item.DoDAssessment.Items))
		for _, assessmentItem := range item.DoDAssessment.Items {
			if strings.TrimSpace(assessmentItem.Item) != "" {
				ret = append(ret, assessmentItem.Item)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}
