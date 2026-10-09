package workspace

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/testkit"
)

// 协调篡改样本同步 raw、typed mapping、canonical 与六审身份，确保拒绝来自待验证的语义门禁。
func resealAuthoringProposalClaimForTest(t *testing.T, record *workspace.DefinitionAuthoringRecord, proposal workspace.GoalSpecification) {
	t.Helper()
	for index := range record.SpecificationRequests {
		request := &record.SpecificationRequests[index]
		if request.Proposal == nil {
			continue
		}
		mapping := request.Mapping.Clone()
		for i := range mapping.Obligations {
			for _, obligation := range proposal.Obligations {
				if len(obligation.DoDItemRefs) != 1 || obligation.DoDItemRefs[0] != mapping.Obligations[i].CandidateItemRef {
					continue
				}
				for _, verification := range proposal.Verifications {
					if verification.ObligationID == obligation.ID {
						mapping.Obligations[i].Verification = workspace.DefinitionAuthoringVerificationMapping{
							Declaration: verification.Declaration, Status: verification.Status,
							ProfileID: verification.ProfileID, EvidenceRequirementID: verification.EvidenceRequirementID, Scope: verification.Scope,
						}
					}
				}
			}
		}
		audit, err := testkit.Audit(mapping)
		if err != nil {
			t.Fatal(err)
		}
		var derivation workspace.DefinitionDerivationRequest
		for _, d := range record.DerivationRequests {
			if d.PermitID == request.DerivationPermitID {
				derivation = d
				break
			}
		}
		compiled, err := workspace.CompileDefinitionAuthoringMapping(derivation, mapping)
		if err != nil {
			t.Fatal(err)
		}
		request.Mapping, request.SuccessAudit, request.Proposal = &mapping, &audit, &compiled
		request.CompletionIdentity = workspace.MappingCompletionIdentity(request.PermitID, mapping, audit)
		for i := range record.SpecificationRequests {
			review := &record.SpecificationRequests[i]
			if review.Review != nil && review.DerivationPermitID == request.DerivationPermitID {
				review.ProposalHash = compiled.ContentHash()
				review.Review.SpecificationHash = compiled.ContentHash()
				review.PermitID = review.ExpectedPermitID()
			}
		}
		return
	}
	t.Fatal("协调篡改样本缺少已封存 proposal")
}
