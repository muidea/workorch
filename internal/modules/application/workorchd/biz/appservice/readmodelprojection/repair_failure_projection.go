package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

type repairFailureProjection struct {
	stepID      string
	class       string
	signature   string
	occurredAt  string
	evidenceRef string
}

func latestRepairFailureProjection(snapshot readmodelcore.RunSnapshotState) *repairFailureProjection {
	var latest *repairFailureProjection
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		candidate := repairFailureProjectionFromCheckpoint(checkpoint)
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.occurredAt > latest.occurredAt || (candidate.occurredAt == latest.occurredAt && candidate.evidenceRef > latest.evidenceRef) {
			latest = candidate
		}
	}
	if latest != nil {
		return latest
	}
	for _, step := range snapshot.Steps {
		candidate := repairFailureProjectionFromStep(step)
		if candidate == nil {
			continue
		}
		latest = candidate
	}
	return latest
}
