package workspace

import (
	"fmt"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
)

// 仅检查封存输入和声明适用性，不生成确认、准入、owner fact、proof 或执行 credit。
func validateAuthoringVerificationAgainstSealedTemplate(sealedTemplateHash string, derivation workspacemodel.DefinitionDerivationRequest, specification workspacemodel.GoalSpecification) error {
	if derivation.Proposal == nil || sealedTemplateHash == "" || workspacemodel.WorkspaceTemplateSnapshotHash(&derivation.Template) != sealedTemplateHash {
		return fmt.Errorf("authoring specification is not bound to the sealed Template")
	}
	bindings, err := definitionconfiguration.CompileAuthoring(derivation)
	if err != nil {
		return fmt.Errorf("compile sealed Template profiles: %w", err)
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		return err
	}
	return definitionadmission.ValidateAuthoringVerificationApplicability(specification, items, bindings.ProfileContracts, workspacemodel.DefinitionCandidateItem{CandidateItemRef: specification.GoalItemRef, Text: derivation.Input.GoalText})
}
