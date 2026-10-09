package workspace

import "testing"

// currentProposalFixture 只用于 synthetic 样本：明确生成当前 wire，再由生产 compiler 生成全部身份。
func currentProposalFixture(t testing.TB, d DefinitionDerivationRequest, seed GoalSpecification) (DefinitionAuthoringMapping, DefinitionAuthoringSuccessByteAudit, GoalSpecification) {
	t.Helper()
	m := mappingFixtureForDerivation(t, d)
	for i := range m.Obligations {
		ref := m.Obligations[i].CandidateItemRef
		for _, o := range seed.Obligations {
			if len(o.DoDItemRefs) != 1 || o.DoDItemRefs[0] != ref || o.Text == "验证目标" || o.ID == "goal-obligation" {
				continue
			}
			for _, v := range seed.Verifications {
				if v.ObligationID == o.ID {
					m.Obligations[i].Verification = DefinitionAuthoringVerificationMapping{Declaration: v.Declaration, Status: v.Status, ProfileID: v.ProfileID, EvidenceRequirementID: v.EvidenceRequirementID, Scope: v.Scope}
					break
				}
			}
			break
		}
	}
	a := mappingAuditFixture(t, m)
	s, err := CompileDefinitionAuthoringMapping(d, m)
	if err != nil {
		t.Fatal(err)
	}
	return m, a, s
}
func completeCurrentProposalFixture(t testing.TB, r DefinitionAuthoringRecord, permit string, proposal *GoalSpecification) (DefinitionAuthoringRecord, error) {
	t.Helper()
	var d DefinitionDerivationRequest
	for _, request := range r.SpecificationRequests {
		if request.PermitID == permit {
			var err error
			d, err = r.derivationForSpecification(request.DerivationPermitID)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	m, a, s := currentProposalFixture(t, d, *proposal)
	*proposal = s
	return r.CompleteSpecificationMapping(permit, m, a)
}
func currentGoalRequirementFixture(s GoalSpecification) string {
	for _, req := range s.Requirements {
		if len(req.SourceItemRefs) == 1 && req.SourceItemRefs[0] == DefinitionAuthoringGoalItemRef {
			return req.ID
		}
	}
	return ""
}
