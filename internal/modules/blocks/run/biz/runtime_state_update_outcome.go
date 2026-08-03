package run

import (
	"fmt"

	runcontracts "github.com/muidea/workorch/internal/modules/blocks/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeStateUpdateOutcome builds the Run-owned factual response for an
// opaque Runtime aggregate update. It intentionally does not inspect the
// aggregate beyond the revision already supplied by the persistence boundary.
func runtimeStateUpdateOutcome(
	current *runmodel.Run,
	cmd runcontracts.UpdateRunCommand,
	category runtimecontract.RuntimeFeedbackCategory,
	observedRevision uint64,
	cause error,
) runevents.RunCommandResult {
	workspaceID, runID := cmd.WorkspaceID, cmd.RunID
	if current != nil {
		workspaceID, runID = current.WorkspaceID, current.ID
	}
	expectedRevision := uint64(0)
	if cmd.ExpectedRuntimeStateRevision != nil {
		expectedRevision = *cmd.ExpectedRuntimeStateRevision
	}
	feedback := runtimecontract.RuntimeFeedback{
		Domain:         runtimecontract.RuntimeFeedbackDomainPersistence,
		StableCategory: category,
		WorkspaceID:    workspaceID,
		RunID:          runID,
	}
	if category != runtimecontract.RuntimeFeedbackCategoryAccepted {
		message := "Run owner could not persist the opaque Runtime state update"
		if cause != nil {
			message = cause.Error()
		}
		feedback.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion:    runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
			OwnerModuleID:    ModuleID,
			OriginalClass:    string(category),
			OriginalCode:     string(category),
			OriginalMessage:  message,
			Signature:        fmt.Sprintf("runtime-state-update/%s/%s/%d/%d/%s", workspaceID, runID, expectedRevision, observedRevision, category),
			ExpectedRevision: fmt.Sprintf("%d", expectedRevision),
			ObservedRevision: fmt.Sprintf("%d", observedRevision),
		}
		// Diagnostic safety is part of the owner contract. In the unlikely case
		// an underlying storage error carries forbidden transport content, retain
		// the fact that it was omitted rather than persisting the unsafe value.
		if err := feedback.Validate(); err != nil {
			feedback.OriginalDiagnostic.OriginalMessage = "Run owner persistence diagnostic omitted for safety"
			feedback.OriginalDiagnostic.OmittedReason = "unsafe_persistence_diagnostic"
			feedback.OriginalDiagnostic.Redacted = true
		}
	}
	result := runevents.RuntimeStateUpdateResult{
		WorkspaceID:             workspaceID,
		RunID:                   runID,
		ExpectedRuntimeRevision: expectedRevision,
		ObservedRuntimeRevision: observedRevision,
		Feedback:                feedback,
	}
	return runevents.RunCommandResult{Run: current, RuntimeStateUpdate: &result}
}
