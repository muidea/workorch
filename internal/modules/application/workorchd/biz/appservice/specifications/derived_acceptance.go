package specifications

import (
	"bytes"
	"encoding/json"
	"fmt"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// ParseDerivedAcceptance 不接受模型修改原始 Goal、用户条件或模板条件。
func ParseDerivedAcceptance(payload []byte, input workspace.DefinitionAuthoringInput, template *workspace.WorkspaceTemplate) (workspace.DefinitionDerivedAcceptance, error) {
	if err := validateCurrentDocument(payload, derivationShape); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	var proposal workspace.DefinitionDerivedAcceptance
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if proposal.AdditionalItems == nil {
		return workspace.DefinitionDerivedAcceptance{}, fmt.Errorf("derived acceptance requires AdditionalItems array")
	}
	if err := proposal.ValidateFor(input, template); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	return proposal, nil
}

func ParseDerivedAcceptanceFor(payload []byte, d workspace.DefinitionDerivationRequest) (workspace.DefinitionDerivedAcceptance, error) {
	if d.WireContractRef == "" {
		return ParseDerivedAcceptance(payload, d.Input, &d.Template)
	}
	return workspace.DecodeOwnerBoundDerivation(d, payload)
}
