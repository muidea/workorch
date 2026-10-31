package workspace

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// 旧来源反例改走唯一当前 mapping 编译与原文重编译，不恢复 canonical provider wire。
func TestSealedMappingRejectsSourceNamespaceAndCanonicalSourceKindDrift(t *testing.T) {
	record, mapping, audit := fullSizeMappingFixture(t)
	derivation := record.DerivationRequests[0]
	proposal, err := CompileDefinitionAuthoringMapping(derivation, mapping)
	if err != nil {
		t.Fatal(err)
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{DefinitionAuthoringGoalItemRef: GoalRequirementUserExplicit}
	for _, item := range items {
		kinds[item.CandidateItemRef] = authoringItemKind(item)
	}
	seen := map[string]bool{}
	for _, requirement := range proposal.Requirements {
		for _, ref := range requirement.SourceItemRefs {
			if kinds[ref] == "" || requirement.SourceKind != kinds[ref] || seen[ref] {
				t.Fatalf("合法编译丢失封存来源身份：%q kind=%q", ref, requirement.SourceKind)
			}
			seen[ref] = true
		}
	}
	if len(seen) != len(kinds) {
		t.Fatal("合法编译未完整覆盖 Goal、用户、模板与派生来源")
	}
	for name, ref := range map[string]string{
		"resource_id":    "authoring-resource:000001",
		"resource_path":  "docs/specification.md",
		"unknown_prefix": "derived-dod:other:000001",
	} {
		t.Run(name, func(t *testing.T) {
			bad := mapping.Clone()
			original := bad.Requirements[0].SourceItemRefs[0]
			bad.Requirements[0].SourceItemRefs[0] = ref
			for index := range bad.Obligations {
				if bad.Obligations[index].CandidateItemRef == original {
					bad.Obligations[index].CandidateItemRef = ref
				}
			}
			_, err := CompileDefinitionAuthoringMapping(derivation, bad)
			var rejection *DefinitionAuthoringMappingError
			if !errors.As(err, &rejection) || rejection.Code != "source_mapping" || rejection.CandidateItemRef != ref {
				t.Fatalf("来源命名空间漂移未由真实来源门禁拒绝：%v", err)
			}
		})
	}
	pending, request, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "proposal"), RequestID: "source-kind", DerivationPermitID: derivation.PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "mock-provider"})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := pending.CompleteSpecificationMapping(request.PermitID, mapping, audit)
	if err != nil {
		t.Fatal(err)
	}
	request = completed.SpecificationRequests[0]
	if canonical, err := RecompileDefinitionAuthoringProposal(derivation, request); err != nil || !reflect.DeepEqual(canonical, proposal) {
		t.Fatalf("合法提案不能由原文独立重编译：%v", err)
	}
	for _, requirement := range proposal.Requirements {
		t.Run("relabel_"+requirement.SourceKind+"_"+requirement.ID, func(t *testing.T) {
			bad := request.Clone()
			for index := range bad.Proposal.Requirements {
				if bad.Proposal.Requirements[index].ID == requirement.ID {
					kind := GoalRequirementDerived
					if requirement.SourceKind == kind {
						kind = GoalRequirementUserExplicit
					}
					bad.Proposal.Requirements[index].SourceKind = kind
				}
			}
			if _, err := RecompileDefinitionAuthoringProposal(derivation, bad); err == nil {
				t.Fatal("协调提案重标来源类别绕过 owner 原文编译")
			}
		})
	}
	bad := mapping.Clone()
	bad.Requirements[0].SourceItemRefs = append(bad.Requirements[0].SourceItemRefs, bad.Requirements[1].SourceItemRefs[0])
	bad.Requirements[1].SourceItemRefs = slices.Clone(bad.Requirements[1].SourceItemRefs[1:])
	_, err = CompileDefinitionAuthoringMapping(derivation, bad)
	var rejection *DefinitionAuthoringMappingError
	if !errors.As(err, &rejection) || rejection.Code != "mixed_source_kind" {
		t.Fatalf("混合来源未由来源类别门禁拒绝：%v", err)
	}
	if _, err := RecompileDefinitionAuthoringProposal(derivation, request); err != nil {
		t.Fatalf("反例污染合法 owner 前像：%v", err)
	}
}
