package specifications

import (
	"bytes"
	"encoding/json"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

// ParseDerivedAcceptance 不接受模型修改原始 Goal、用户条件或模板条件。
func ParseDerivedAcceptance(payload []byte, input workspace.DefinitionAuthoringInput, template *workspace.WorkspaceTemplate) (workspace.DefinitionDerivedAcceptance, error) {
	if err := validateDocumentShape(payload); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	var proposal workspace.DefinitionDerivedAcceptance
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if err := proposal.ValidateFor(input, template); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	return proposal, nil
}
