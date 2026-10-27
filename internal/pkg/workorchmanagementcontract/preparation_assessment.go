package workorchmanagementcontract

import "github.com/muidea/workorch/internal/pkg/common"

// A historical checklist aggregate. It intentionally carries no ready,
// completion ID, freshness lease or execution permission.
type PreparationAssessmentView struct {
	Version             string                            `json:"version"`
	ManifestHash        string                            `json:"manifestHash"`
	Outcome             string                            `json:"outcome"`
	RequiredCount       int                               `json:"requiredCount"`
	PassedRequiredCount int                               `json:"passedRequiredCount"`
	Outstanding         []PreparationOutstandingCheckView `json:"outstanding"`
}

type PreparationOutstandingCheckView struct {
	ID     string `json:"id"`
	Owner  string `json:"owner"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}

func (s PreparationAssessmentView) ValidateFor(manifestHash string, checks []PreparationCheckView) error {
	if s.Version != "workorch-preparation-all-of-assessment/v1" || !preparationViewIdentity(manifestHash) || s.ManifestHash != manifestHash || len(checks) == 0 || len(checks) > 128 || s.Outstanding == nil {
		return common.ErrConflict
	}
	required, passed, index, last := 0, 0, 0, ""
	for _, c := range checks {
		if !preparationViewIdentity(c.ID) || !preparationViewIdentity(c.Owner) || !preparationViewIdentity(c.Reason) || c.ID <= last || c.State != "passed" && c.State != "pending" && c.State != "blocked" && c.State != "failed" {
			return common.ErrConflict
		}
		last = c.ID
		if !c.Required {
			continue
		}
		required++
		if c.State == "passed" {
			passed++
		} else {
			if index >= len(s.Outstanding) || s.Outstanding[index] != (PreparationOutstandingCheckView{c.ID, c.Owner, c.State, c.Reason}) {
				return common.ErrConflict
			}
			index++
		}
	}
	outcome := "checks_passed"
	if passed != required {
		outcome = "blocked"
	}
	if required == 0 || s.RequiredCount != required || s.PassedRequiredCount != passed || s.Outcome != outcome || index != len(s.Outstanding) {
		return common.ErrConflict
	}
	return nil
}
