package mcp

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// attachMCPToolValidationObservation records the narrow owner fact that an
// authorized MCP tool call completed successfully. The proof identity is the
// exact sealed server/tool pair, never the candidate summary, tool input or
// returned body. This lets SM-2 match a C12 binding without granting the MCP
// owner any coverage or Runtime-state authority.
//
// Resource reads and prompt retrieval intentionally remain non-proof
// diagnostics. Rejected calls and non-validation calls likewise cannot obtain
// a validation observation merely from an MCP transport success.
func attachMCPToolValidationObservation(
	evidence *runtimecontract.RuntimeOperationEvidence,
	status runtimecontract.RuntimeOperationExecutionStatus,
	operationKind, evidenceKind, serverID, tool string,
) {
	if evidence == nil || status != runtimecontract.RuntimeOperationExecutionStatusAccepted ||
		runtimecontract.CanonicalRuntimeBlockKind(operationKind) != runtimecontract.StepKindMCPToolCall ||
		strings.TrimSpace(evidenceKind) != runtimecontract.EvidenceKindValidation {
		return
	}
	recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID(serverID, tool)
	if recipeID == "" {
		return
	}
	evidence.Quality = common.EvidenceQualityValid
	evidence.Validation = &runtimecontract.RuntimeValidationObservation{
		RecipeID: recipeID, ExecutedChecks: []string{recipeID}, PassedChecks: []string{recipeID},
	}
}
