package runtimeprofiledecode

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// InputFields describes the closed wire shape of one object in a Runtime
// Profile declaration. These fields are shared by configuration producers and
// consumers; they are not an authority registry.
type InputFields struct {
	Text, Boolean, StringList, Integer string
	Arrays                             map[string]string
}

// ValidateInputShape rejects lossy or unsupported Profile values before
// sources are merged or converted into typed Runtime contracts.
func ValidateInputShape(input map[string]any) error {
	raw, found := input["profileContracts"]
	if !found {
		return nil
	}
	if containsField(raw, "prospectiveTargetRef") {
		return fmt.Errorf("%w: pre-seal execution contract rejects prospectiveTargetRef; declare targetSelectorCandidateRefs instead", common.ErrInvalidArgument)
	}
	if containsBootstrapArtifactField(input, "targetRef") {
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
		if err := ValidateObject(profile, "profile", "profileContracts["+id+"]"); err != nil {
			return err
		}
	}
	return nil
}

// Fields returns the allowed field/type mapping for a Profile input object.
func Fields(kind string) InputFields {
	switch kind {
	case "profile":
		return InputFields{Arrays: map[string]string{
			"acceptanceDimensions": "dimension", "deliverableExpectations": "deliverable", "evidenceRequirements": "evidence",
			"coverageDependencies": "dependency", "bootstrapRecipes": "recipe",
		}}
	case "dimension":
		return InputFields{Text: "id description", Boolean: "required"}
	case "deliverable":
		return InputFields{Text: "id description evidenceRole", Boolean: "required"}
	case "dependency":
		return InputFields{Text: "id sourceScope sourceRequirementId dependentScope dependentRequirementId", Boolean: "requireSourceSettled", StringList: "triggerEvidenceKinds"}
	case "recipe":
		return InputFields{Text: "id", StringList: "requiredGoalTerms requiredDoDTerms", Arrays: map[string]string{"artifacts": "artifact"}}
	case "artifact":
		return InputFields{Text: "id targetRole", StringList: "allowedOperationKinds dependsOnArtifactIds"}
	case "evidence":
		return InputFields{
			Text:       "id scope evidenceRole targetRole targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef validationRecipeId description",
			Boolean:    "requireTargetGrounding requireDirectEvidence requireClaimEvidence requireExecutableBehaviorEvidence requireCoverageTargets",
			StringList: "targetRefs targetDiscoverySources targetSelectorCandidateRefs prospectiveTargetAllOfConstraintRefs requiredEvidenceKinds acceptedStepKinds requiredCheckRefs metricConstraints artifactRefs rejectedStepKinds",
			Arrays:     map[string]string{"acceptedOperationVariants": "variant", "capabilityStateRequirements": "capability", "requiredOwnerFacts": "ownerFact", "ownerFactConvergences": "convergence"},
		}
	case "variant":
		return InputFields{Text: "kind variantId"}
	case "implementationWork":
		return InputFields{
			Text:       "protocolRef ownerModuleId targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef",
			Boolean:    "requireFreshObservation requireFreshValidation protectValidationInputs",
			Integer:    "maxRoundsPerCWI maxTargetsPerRound",
			StringList: "allowedTargetPolicies targetSelectorCandidateRefs prospectiveTargetAllOfConstraintRefs",
			Arrays:     map[string]string{"allowedOperations": "variant"},
		}
	case "capability":
		return InputFields{Text: "ownerModuleId capabilityStateKind subjectRef"}
	case "ownerFact":
		return InputFields{Text: "ownerModuleId factKind operationKind"}
	case "convergence":
		return InputFields{
			Text:    "ownerModuleId factKind operationKind missingTrigger acquisitionOwnerModuleId acquisitionOperationKind acquisitionOperationVariantId acquisitionTargetPolicy acquisitionTargetSelectorRef prospectiveSatisfactionContractRef targetSelectorRef targetSelectorOwnerRef targetSelectorSubjectRef reobserveOwnerModuleId reobserveFactKind",
			Boolean: "requirePreexisting", Integer: "maxAcquisitionAttempts", StringList: "dependsOnRequirementRefs",
		}
	default:
		return InputFields{}
	}
}

// ValidateObject recursively validates one Profile-owned declaration object.
func ValidateObject(object map[string]any, kind, subject string) error {
	fields := Fields(kind)
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := object[key]
		path := subject + "." + key
		if child, found := fields.Arrays[key]; found {
			items, exact := ObjectSliceExact(raw)
			if !exact {
				return fmt.Errorf("%w: %s must be an object array", common.ErrInvalidArgument, path)
			}
			for i, item := range items {
				if item == nil {
					return fmt.Errorf("%w: %s[%d] must be an object", common.ErrInvalidArgument, path, i)
				}
				if err := ValidateObject(item, child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			continue
		}
		valid, expected := false, ""
		switch {
		case kind == "evidence" && key == "implementationWorkPolicy":
			value, ok := raw.(map[string]any)
			if !ok || value == nil {
				return fmt.Errorf("%w: %s must be an object", common.ErrInvalidArgument, path)
			}
			if err := ValidateObject(value, "implementationWork", path); err != nil {
				return err
			}
			continue
		case kind == "evidence" && key == "validationFailureRepairConvergence":
			// Keep the existing exact decoder's behavior: malformed repair input
			// remains a visible invalid typed declaration for Workspace admission.
			continue
		case slices.Contains(strings.Fields(fields.Text), key):
			_, valid = raw.(string)
			expected = "a string"
		case slices.Contains(strings.Fields(fields.Boolean), key):
			_, valid = raw.(bool)
			expected = "a boolean"
		case slices.Contains(strings.Fields(fields.StringList), key):
			valid, expected = ProfileStringListHasExactShape(raw), "an array of nonblank strings"
		case slices.Contains(strings.Fields(fields.Integer), key):
			valid, expected = IntegerHasExactShape(raw), "an exactly representable integer"
		default:
			return fmt.Errorf("%w: %s is not a supported Profile field", common.ErrInvalidArgument, path)
		}
		if !valid {
			return fmt.Errorf("%w: %s must be %s", common.ErrInvalidArgument, path, expected)
		}
	}
	return nil
}

func containsField(value any, fieldName string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(strings.TrimSpace(key), fieldName) || containsField(child, fieldName) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsField(child, fieldName) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if containsField(child, fieldName) {
				return true
			}
		}
	}
	return false
}

func containsBootstrapArtifactField(contract map[string]any, fieldName string) bool {
	profiles, _ := contract["profileContracts"].(map[string]any)
	for _, rawProfile := range profiles {
		profile, _ := rawProfile.(map[string]any)
		for _, recipe := range MapSlice(profile["bootstrapRecipes"]) {
			for _, artifact := range MapSlice(recipe["artifacts"]) {
				for key := range artifact {
					if strings.EqualFold(strings.TrimSpace(key), fieldName) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ObjectSliceExact preserves cardinality while rejecting malformed members.
func ObjectSliceExact(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...), true
	case []any:
		ret := make([]map[string]any, len(typed))
		for index, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			ret[index] = mapped
		}
		return ret, true
	default:
		return nil, false
	}
}

func StringListHasExactShape(value any) bool {
	switch typed := value.(type) {
	case []string:
		return true
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func ProfileStringListHasExactShape(raw any) bool {
	if !StringListHasExactShape(raw) {
		return false
	}
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

func IntegerHasExactShape(value any) bool {
	switch typed := value.(type) {
	case int:
		return true
	case int64:
		return int64(int(typed)) == typed
	case float64:
		limit := math.Ldexp(1, strconv.IntSize-1)
		return !math.IsNaN(typed) && !math.IsInf(typed, 0) && typed == math.Trunc(typed) && typed >= -limit && typed < limit
	default:
		return false
	}
}

func MapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func Map(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func String(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func Bool(value any) bool {
	result, _ := value.(bool)
	return result
}

func Int(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func Strings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := String(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints, workspaceHints map[string]any) map[string]any {
	if len(templateHints) == 0 {
		return cloneMap(workspaceHints)
	}
	ret := cloneMap(templateHints)
	for key, value := range workspaceHints {
		if key == "profileContracts" {
			ret[key] = MergeTemplateWorkspaceProfileContracts(Map(templateHints[key]), Map(value))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneValue(value)
	}
	return ret
}

func MergeTemplateWorkspaceProfileContracts(templateProfiles, workspaceProfiles map[string]any) map[string]any {
	if len(templateProfiles) == 0 {
		if cloned, ok := cloneValue(workspaceProfiles).(map[string]any); ok {
			return cloned
		}
		return nil
	}
	ret := map[string]any{}
	for profileID, profile := range templateProfiles {
		ret[profileID] = cloneValue(profile)
	}
	for profileID, workspaceProfile := range workspaceProfiles {
		templateProfile, exists := ret[profileID]
		if !exists {
			ret[profileID] = cloneValue(workspaceProfile)
			continue
		}
		ret[profileID] = mergeProfileContract(Map(templateProfile), Map(workspaceProfile))
	}
	return ret
}

func MergeTemplateWorkspaceProfileContract(templateProfile, workspaceProfile map[string]any) map[string]any {
	return mergeProfileContract(templateProfile, workspaceProfile)
}

func mergeProfileContract(templateProfile, workspaceProfile map[string]any) map[string]any {
	if len(templateProfile) == 0 {
		if cloned, ok := cloneValue(workspaceProfile).(map[string]any); ok {
			return cloned
		}
		return nil
	}
	ret := cloneMap(templateProfile)
	for key, value := range workspaceProfile {
		if profileCollectionKey(key) {
			ret[key] = mergeProfileCollection(MapSlice(templateProfile[key]), MapSlice(value), profileCollectionIdentityFields(key))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneValue(value)
	}
	return ret
}

func profileCollectionKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "acceptanceDimensions", "deliverableExpectations", "evidenceRequirements", "coverageDependencies", "bootstrapRecipes":
		return true
	default:
		return false
	}
}

func profileCollectionIdentityFields(key string) []string {
	if strings.TrimSpace(key) == "evidenceRequirements" {
		return []string{"scope", "id"}
	}
	return []string{"id"}
}

func mergeProfileCollection(template, workspace []map[string]any, identityFields []string) []map[string]any {
	templateCounts, workspaceCounts := map[string]int{}, map[string]int{}
	for _, item := range template {
		templateCounts[profileCollectionIdentity(item, identityFields)]++
	}
	for _, item := range workspace {
		workspaceCounts[profileCollectionIdentity(item, identityFields)]++
	}
	ret := make([]map[string]any, 0, len(template)+len(workspace))
	for _, item := range template {
		key := profileCollectionIdentity(item, identityFields)
		if key != "" && templateCounts[key] == 1 && workspaceCounts[key] == 1 {
			continue
		}
		ret = append(ret, cloneMap(item))
	}
	for _, item := range workspace {
		ret = append(ret, cloneMap(item))
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return profileCollectionIdentity(ret[i], identityFields) < profileCollectionIdentity(ret[j], identityFields)
	})
	return ret
}

func profileCollectionIdentity(item map[string]any, fields []string) string {
	if len(item) == 0 || len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value, ok := item[field].(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			return ""
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x00")
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneValue(value)
	}
	return ret
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []map[string]any:
		if typed == nil {
			return []map[string]any(nil)
		}
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneMap(item))
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
