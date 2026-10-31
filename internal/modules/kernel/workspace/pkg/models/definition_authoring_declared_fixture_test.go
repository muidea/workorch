package workspace

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// 正式确认样本只包含可追溯的声明；它不是 owner fact 或实际 validation credit。
func declaredAuthoringLedgerFixture(t testing.TB) (DefinitionAuthoringRecord, GoalSpecification) {
	t.Helper()
	input, template, derived, proposal := authoringSpecificationFixture(t)
	hints, err := workorcheventpayload.FromMap(map[string]any{
		"profileContracts": map[string]any{"generic": map[string]any{
			"evidenceRequirements": []any{map[string]any{
				"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"},
				"acceptedStepKinds": []any{"validation/managed_session"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	template.ContextHints = hints
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	derived.TemplateSnapshotHash = record.TemplateSnapshotHash
	var request DefinitionDerivationRequest
	record, request, _, err = record.ReserveDerivation("derive-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestDerivation(record, request.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	for index := range proposal.Verifications {
		verification := &proposal.Verifications[index]
		verification.Status = GoalVerificationDeclared
		verification.Declaration = "由 generic Profile 的 validation 声明验证"
		verification.ProfileID, verification.EvidenceRequirementID, verification.Scope = "generic", "*", "dodItem"
	}
	_, _, proposal = currentProposalFixture(t, record.DerivationRequests[0], proposal)
	return record, proposal
}
