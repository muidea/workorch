package shell

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellNativeCheckReadinessDiagnostic(inspection shellNativeCheckInspection, requirement runtimecontract.RuntimeValidationProofRequirement, manifest runtimecontract.WorkspaceManifest, catalogRevision, preparationTarget string) *runtimecontract.RuntimeOriginalDiagnostic {
	code := inspection.Code
	if code == "" {
		code = shellValidationApplicableInputRequiredReason
	}
	// No source text, parser error, environment, or suggested state/action may
	// cross this boundary. Requirement/check/manifest identities are owner facts.
	message := fmt.Sprintf("validation proof %s for CWI %s is not ready: %s", requirement.Hash, requirement.CoverageWorkItemID, code)
	if inspection.CheckRef != "" {
		message += "; native check " + inspection.CheckRef
	}
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: ModuleID, OriginalClass: "validation_readiness", OriginalCode: code, OriginalMessage: message,
		Signature:        "validation-readiness/" + requirement.Hash + "/" + code,
		ObservedRevision: catalogRevision, TargetRefs: normalizeShellStrings(inspection.TargetRefs),
		FieldViolations: []runtimecontract.RuntimeFieldViolation{
			{Path: "validationProofRequirement.hash", Actual: requirement.Hash},
			{Path: "workspaceManifest.revision", Actual: manifest.Revision},
		},
	}
	if inspection.CheckRef != "" {
		diagnostic.FieldViolations = append(diagnostic.FieldViolations, runtimecontract.RuntimeFieldViolation{Path: "nativeCheckRef", Actual: inspection.CheckRef})
	}
	if preparationTarget != "" {
		diagnostic.FieldViolations = append(diagnostic.FieldViolations, runtimecontract.RuntimeFieldViolation{Path: "preparation.targetRef", Actual: preparationTarget})
	}
	if len(diagnostic.TargetRefs) > runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs {
		diagnostic.TargetRefs = diagnostic.TargetRefs[:runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs]
		diagnostic.Truncated, diagnostic.OmittedReason = true, "native_check_target_diagnostic_limit"
	}
	return diagnostic
}
