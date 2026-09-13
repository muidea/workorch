package activationports

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Profile decoding must be lossless before typed owner admission. This is the
// closed configuration wire shape consumed by runtimeExecutionContract, not a
// runtime authority registry. Presence never authorizes unsupported work;
// identity/semantic decisions remain with Workspace's typed compiler.
func validateRuntimeProfileInputShape(input map[string]any) error {
	raw, found := input["profileContracts"]
	if !found {
		return nil
	}
	// Enforce removed surfaces before either source layer can be merged away,
	// preserving the same explicit rejection used by the final activation gate.
	if runtimeValueContainsField(raw, "prospectiveTargetRef") {
		return fmt.Errorf("%w: pre-seal execution contract rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
	}
	if runtimeExecutionContractContainsBootstrapArtifactField(input, "targetRef") {
		return fmt.Errorf("%w: pre-seal bootstrap artifact rejects targetRef; the semantic proposal must choose its target after activation", common.ErrInvalidArgument)
	}
	profiles, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: profileContracts must be an object", common.ErrInvalidArgument)
	}
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		profile, ok := profiles[id].(map[string]any)
		if !ok || profile == nil {
			return fmt.Errorf("%w: profileContracts[%q] must be an object", common.ErrInvalidArgument, id)
		}
		if err := validateRuntimeProfileObject(profile, "profile", "profileContracts["+id+"]"); err != nil {
			return err
		}
	}
	return nil
}

type runtimeProfileInputFields struct {
	text, boolean, stringList, integer string
	arrays                             map[string]string
}

// These are protocol field names, scoped to their actual containing object.
// A known name in the wrong object is still unknown: accepting it would drop
// policy during decoding. New contracts require an explicit decoder and tests.
func runtimeProfileFields(kind string) runtimeProfileInputFields {
	switch kind {
	case "profile":
		return runtimeProfileInputFields{arrays: map[string]string{
			"acceptanceDimensions": "dimension", "deliverableExpectations": "deliverable", "evidenceRequirements": "evidence",
			"coverageDependencies": "dependency", "bootstrapRecipes": "recipe",
		}}
	case "dimension":
		return runtimeProfileInputFields{text: "id description", boolean: "required"}
	case "deliverable":
		return runtimeProfileInputFields{text: "id description evidenceRole", boolean: "required"}
	case "dependency":
		return runtimeProfileInputFields{text: "id sourceScope sourceRequirementId dependentScope dependentRequirementId", boolean: "requireSourceSettled", stringList: "triggerEvidenceKinds"}
	case "recipe":
		return runtimeProfileInputFields{text: "id", stringList: "requiredGoalTerms requiredDoDTerms", arrays: map[string]string{"artifacts": "artifact"}}
	case "artifact":
		return runtimeProfileInputFields{text: "id targetRole", stringList: "allowedOperationKinds dependsOnArtifactIds"}
	case "evidence":
		return runtimeProfileInputFields{
			text:       "id scope evidenceRole targetRole targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef validationRecipeId description",
			boolean:    "requireTargetGrounding requireDirectEvidence requireClaimEvidence requireExecutableBehaviorEvidence requireCoverageTargets",
			stringList: "targetRefs targetDiscoverySources targetSelectorCandidateRefs prospectiveTargetAllOfConstraintRefs requiredEvidenceKinds acceptedStepKinds requiredCheckRefs metricConstraints artifactRefs rejectedStepKinds",
			arrays:     map[string]string{"acceptedOperationVariants": "variant", "capabilityStateRequirements": "capability", "requiredOwnerFacts": "ownerFact", "ownerFactConvergences": "convergence"},
		}
	case "variant":
		return runtimeProfileInputFields{text: "kind variantId"}
	case "implementationWork":
		return runtimeProfileInputFields{
			text:       "protocolRef ownerModuleId targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef",
			boolean:    "requireFreshObservation requireFreshValidation protectValidationInputs",
			integer:    "maxRoundsPerCWI maxTargetsPerRound",
			stringList: "allowedTargetPolicies targetSelectorCandidateRefs prospectiveTargetAllOfConstraintRefs",
			arrays:     map[string]string{"allowedOperations": "variant"},
		}
	case "capability":
		return runtimeProfileInputFields{text: "ownerModuleId capabilityStateKind subjectRef"}
	case "ownerFact":
		return runtimeProfileInputFields{text: "ownerModuleId factKind operationKind"}
	case "convergence":
		return runtimeProfileInputFields{
			text:    "ownerModuleId factKind operationKind missingTrigger acquisitionOwnerModuleId acquisitionOperationKind acquisitionOperationVariantId acquisitionTargetPolicy acquisitionTargetSelectorRef prospectiveSatisfactionContractRef targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef reobserveOwnerModuleId reobserveFactKind",
			boolean: "requirePreexisting", integer: "maxAcquisitionAttempts", stringList: "dependsOnRequirementRefs",
		}
	default:
		return runtimeProfileInputFields{}
	}
}

func validateRuntimeProfileObject(object map[string]any, kind, subject string) error {
	fields := runtimeProfileFields(kind)
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := object[key]
		path := subject + "." + key
		if child, found := fields.arrays[key]; found {
			items, exact := runtimeObjectSliceExact(raw)
			if !exact {
				return fmt.Errorf("%w: %s must be an object array", common.ErrInvalidArgument, path)
			}
			for i, item := range items {
				if item == nil {
					return fmt.Errorf("%w: %s[%d] must be an object", common.ErrInvalidArgument, path, i)
				}
				if err := validateRuntimeProfileObject(item, child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			continue
		}
		valid, expected := false, ""
		switch {
		case kind == "evidence" && key == "implementationWorkPolicy":
			object, ok := raw.(map[string]any)
			if !ok || object == nil {
				return fmt.Errorf("%w: %s must be an object", common.ErrInvalidArgument, path)
			}
			if err := validateRuntimeProfileObject(object, "implementationWork", path); err != nil {
				return err
			}
			continue
		case kind == "evidence" && key == "validationFailureRepairConvergence":
			// Its existing exact decoder preserves malformed input as a typed
			// invalid declaration for Workspace to reject. Do not downgrade it
			// to an absent policy or invent a second semantic disposition here.
			continue
		case slices.Contains(strings.Fields(fields.text), key):
			_, valid = raw.(string)
			expected = "a string"
		case slices.Contains(strings.Fields(fields.boolean), key):
			_, valid = raw.(bool)
			expected = "a boolean"
		case slices.Contains(strings.Fields(fields.stringList), key):
			valid, expected = runtimeProfileStringListHasExactShape(raw), "an array of nonblank strings"
		case slices.Contains(strings.Fields(fields.integer), key):
			valid, expected = runtimeIntegerHasExactShape(raw), "an exactly representable integer"
		default:
			return fmt.Errorf("%w: %s is not a supported Profile field", common.ErrInvalidArgument, path)
		}
		if !valid {
			return fmt.Errorf("%w: %s must be %s", common.ErrInvalidArgument, path, expected)
		}
	}
	return nil
}

func runtimeProfileStringListHasExactShape(raw any) bool {
	if !runtimeStringListHasExactShape(raw) {
		return false
	}
	// Empty arrays may express an absent optional constraint. An explicit
	// blank member is not absence and must not disappear in runtimeStrings.
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return false
			}
		}
	case []any:
		for _, value := range values {
			if strings.TrimSpace(value.(string)) == "" {
				return false
			}
		}
	}
	return true
}
