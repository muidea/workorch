package shell

import (
	"fmt"
	"strings"

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

// A grammar or assertion defect in existing owner inputs is reparable only
// within those same observed resources. Scope, I/O, capacity and unsupported
// protocol failures cannot acquire mutation authority from their error text.
func shellValidationInputMaintenance(directory string, refs []string, inspection shellNativeCheckInspection, requirement runtimecontract.RuntimeValidationProofRequirement, recipe runtimecontract.RuntimeValidationRecipe, checks []runtimecontract.RuntimeValidationCheckAttestationBinding) *runtimecontract.RuntimeValidationInputMaintenance {
	switch inspection.Code {
	case "native_check_versioned_contract_invalid", "native_check_source_invalid", "native_check_signature_invalid", "native_check_assertion_unresolved", "native_check_missing", "native_check_ambiguous", "native_check_runner_override", "native_check_unmanaged_self_execution":
	default:
		return nil
	}
	b, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
	if err != nil {
		return nil
	}
	subject, code := shellReadValidationInputSubject(directory, refs, b, false)
	if code != "" {
		return nil
	}
	pkg, err := shellIsolatedValidationPackage(checks)
	if err != nil || pkg == "" {
		return nil
	}
	targets := normalizeShellStrings(inspection.TargetRefs)
	if len(targets) == 0 {
		targets = []string{shellValidationNativeCheckTargetRef(requirement, checks...)}
	}
	for _, target := range targets {
		if !strings.HasPrefix(target, pkg+"/") {
			return nil
		}
	}
	m := &runtimecontract.RuntimeValidationInputMaintenance{Binding: b, Recipe: &recipe, Files: subject.Files, TargetRefs: targets}
	if m.Validate(requirement, recipe.ID) != nil {
		return nil
	}
	return m
}
