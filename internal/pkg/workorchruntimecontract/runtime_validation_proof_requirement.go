package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeValidationProofRequirementSchemaCurrent = "workorch.runtime_validation_proof_requirement"
	RuntimeValidationProofRequirementHashPrefix    = "runtime-validation-proof-requirement-"
)

// RuntimeValidationProofRequirement is the immutable, Definition-owned proof
// identity for one validation CWI. A recipe is only an optional Profile-owned
// transport constraint; when it is empty, a typed capability-owner catalog may
// bind any compatible executable recipe. The CWI-specific check below is what
// prevents one broad successful recipe from receiving credit for unrelated
// requirements.
type RuntimeValidationProofRequirement struct {
	SchemaVersion          string
	Hash                   string
	DefinitionRevision     string
	CoverageWorkItemID     common.CoverageWorkItemID
	SourceScope            string
	SourceID               string
	SourceTextHash         string
	DeclarationRef         string
	ValidationRecipeID     string
	RequiredCheckRefs      []string
	SourceObligationChecks []RuntimeSourceObligationCheck `json:",omitempty"`
	LineageAssertions      []RuntimeLineageAssertion      `json:",omitempty"`
}

func (r RuntimeValidationProofRequirement) Normalized() RuntimeValidationProofRequirement {
	r.SchemaVersion = strings.TrimSpace(r.SchemaVersion)
	r.Hash = strings.TrimSpace(r.Hash)
	r.DefinitionRevision = strings.TrimSpace(r.DefinitionRevision)
	r.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(r.CoverageWorkItemID)))
	r.SourceScope = strings.TrimSpace(r.SourceScope)
	r.SourceID = strings.TrimSpace(r.SourceID)
	r.SourceTextHash = strings.TrimSpace(r.SourceTextHash)
	r.DeclarationRef = strings.TrimSpace(r.DeclarationRef)
	r.ValidationRecipeID = strings.TrimSpace(r.ValidationRecipeID)
	r.RequiredCheckRefs = normalizeRuntimeValidationProofCheckRefs(r.RequiredCheckRefs)
	r.SourceObligationChecks = CloneRuntimeSourceObligationChecks(r.SourceObligationChecks)
	r.LineageAssertions = slices.Clone(r.LineageAssertions)
	return r
}

func (r RuntimeValidationProofRequirement) Clone() RuntimeValidationProofRequirement {
	r = r.Normalized()
	r.RequiredCheckRefs = append([]string(nil), r.RequiredCheckRefs...)
	return r
}

func (r RuntimeValidationProofRequirement) Validate() error {
	if duplicate := duplicateRuntimeValidationProofCheckRef(r.RequiredCheckRefs); duplicate != "" {
		return fmt.Errorf("runtime validation proof requirement duplicates check %q", duplicate)
	}
	r = r.Normalized()
	if r.SchemaVersion != RuntimeValidationProofRequirementSchemaCurrent || r.DefinitionRevision == "" || r.CoverageWorkItemID == "" ||
		r.SourceScope == "" || r.SourceID == "" || r.SourceTextHash == "" || r.DeclarationRef == "" ||
		len(r.RequiredCheckRefs) == 0 {
		return fmt.Errorf("runtime validation proof requirement requires current schema, definition, CWI source, exact declaration and checks")
	}
	canonicalDeclarationRef := CanonicalRuntimeDefinitionEvidenceDeclarationRef(r.SourceScope, r.SourceID)
	if r.DeclarationRef != canonicalDeclarationRef {
		return fmt.Errorf("runtime validation proof requirement declaration %q does not match canonical admitted-CWI declaration %q", r.DeclarationRef, canonicalDeclarationRef)
	}
	if err := ValidateRuntimeSourceObligationChecks(r.DefinitionRevision, r.CoverageWorkItemID, r.SourceObligationChecks); err != nil {
		return err
	}
	for _, check := range r.SourceObligationChecks {
		if !containsString(r.RequiredCheckRefs, check.CheckRef) {
			return fmt.Errorf("runtime validation proof requirement omits source obligation %q", check.ObligationID)
		}
	}
	if err := ValidateRuntimeLineageAssertions(r.LineageAssertions, r.CoverageWorkItemID, r.DeclarationRef); err != nil {
		return err
	}
	for _, assertion := range r.LineageAssertions {
		goalConsumer := assertion.Rule.ExecutionSourceContractRef == RuntimeGoalSourceExecutionProofV2 && assertion.SourceScope == "goal" && r.SourceScope == "dodItem"
		if !goalConsumer && (assertion.SourceScope != r.SourceScope || assertion.SourceID != r.SourceID) {
			return fmt.Errorf("validation composition assertion differs from original CWI source")
		}
	}
	if r.Hash != StableRuntimeValidationProofRequirementHash(r) {
		return fmt.Errorf("runtime validation proof requirement hash does not match contents")
	}
	return nil
}

func normalizeRuntimeValidationProofCheckRefs(values []string) []string {
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		if value := strings.TrimSpace(raw); value != "" {
			ret = append(ret, value)
		}
	}
	sort.Strings(ret)
	return nilIfEmpty(ret)
}

func duplicateRuntimeValidationProofCheckRef(values []string) string {
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			return value
		}
		seen[value] = struct{}{}
	}
	return ""
}

// CompileRuntimeValidationProofRequirements compiles exact admitted Definition
// declarations into exact CWI requirements without interpreting requirement
// text. Wildcard Profile rules are quality gates only and must already have
// been resolved by the Application into an exact Definition declaration. The
// stable CWI check is always required, even when a Profile also declares
// tool-native checks, so two detailed CWIs can never share the same complete
// validation proof identity.
func CompileRuntimeValidationProofRequirements(definitionRevision string, coverageItems []RuntimeCoverageItem, focuses []RuntimeDefinitionFocus) ([]RuntimeValidationProofRequirement, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	if definitionRevision == "" {
		return nil, fmt.Errorf("runtime validation proof requirements require definition revision")
	}
	for _, raw := range focuses {
		if duplicate := duplicateRuntimeValidationProofCheckRef(raw.RequiredCheckRefs); duplicate != "" {
			return nil, fmt.Errorf("runtime validation proof declaration %q duplicates check %q", strings.TrimSpace(raw.ID), duplicate)
		}
		if len(raw.CoverageWorkItemIDs) != len(normalizeCoverageWorkItemIDs(raw.CoverageWorkItemIDs)) {
			return nil, fmt.Errorf("runtime validation proof declaration %q has duplicate or empty coverage identity", strings.TrimSpace(raw.ID))
		}
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(coverageItems))
	for _, item := range NormalizeRuntimeCoverageItems(coverageItems) {
		if item.ID == "" || item.SourceScope == "" || item.SourceID == "" {
			return nil, fmt.Errorf("runtime validation proof coverage item requires identity and exact source")
		}
		if _, duplicate := items[item.ID]; duplicate {
			return nil, fmt.Errorf("runtime validation proof coverage item %q is duplicated", item.ID)
		}
		items[item.ID] = item
	}
	ret := make([]RuntimeValidationProofRequirement, 0, len(focuses))
	for _, focus := range NormalizeRuntimeDefinitionFocuses(focuses) {
		if !containsString(focus.RequiredEvidenceKinds, EvidenceKindValidation) ||
			!runtimeOperationKindsContain(focus.AcceptedStepKinds, RuntimeBlockKindManagedValidationSession) {
			continue
		}
		if len(focus.CoverageWorkItemIDs) != 1 {
			return nil, fmt.Errorf("runtime validation proof declaration %q must identify one CWI", focus.ID)
		}
		item, found := items[focus.CoverageWorkItemIDs[0]]
		if !found {
			// The coverage item list is an explicit semantic-generation subset;
			// immutable DefinitionFocuses may contain the rest of the Definition.
			continue
		}
		canonicalDeclarationRef := CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
		if focus.EvidenceDeclarationRef != canonicalDeclarationRef {
			return nil, fmt.Errorf("runtime validation proof declaration %q does not match canonical Definition declaration %q", focus.ID, canonicalDeclarationRef)
		}
		if focus.AssessmentScope != item.SourceScope || focus.AssessmentID != item.SourceID || focus.AssessmentText != item.TargetText {
			return nil, fmt.Errorf("runtime validation proof declaration %q source does not exactly match coverage %q", focus.ID, item.ID)
		}
		checks := append([]string(nil), focus.RequiredCheckRefs...)
		if err := ValidateRuntimeSourceObligationChecks(definitionRevision, item.ID, item.SourceObligationChecks); err != nil {
			return nil, err
		}
		for _, check := range item.SourceObligationChecks {
			checks = append(checks, check.CheckRef)
		}
		cwiCheck := StableRuntimeCWIValidationCheckRef(definitionRevision, item)
		if !containsString(checks, cwiCheck) {
			checks = append(checks, cwiCheck)
		}
		assertions, err := CompileRuntimeCWILineageAssertions(focus.LineageAssertions, focus.EvidenceDeclarationRef, item)
		if err != nil {
			return nil, err
		}
		requirement := RuntimeValidationProofRequirement{
			SchemaVersion:      RuntimeValidationProofRequirementSchemaCurrent,
			DefinitionRevision: definitionRevision, CoverageWorkItemID: item.ID,
			SourceScope: item.SourceScope, SourceID: item.SourceID,
			SourceTextHash: stableRuntimeValidationSourceTextHash(item.TargetText),
			DeclarationRef: focus.EvidenceDeclarationRef, ValidationRecipeID: focus.ValidationRecipeID,
			RequiredCheckRefs:      checks,
			SourceObligationChecks: CloneRuntimeSourceObligationChecks(item.SourceObligationChecks),
			LineageAssertions:      assertions,
		}.Normalized()
		requirement.Hash = StableRuntimeValidationProofRequirementHash(requirement)
		if err := requirement.Validate(); err != nil {
			return nil, fmt.Errorf("runtime validation proof requirement for coverage %q: %w", item.ID, err)
		}
		ret = append(ret, requirement)
	}
	if err := ValidateRuntimeValidationProofRequirements(ret); err != nil {
		return nil, err
	}
	return NormalizeRuntimeValidationProofRequirements(ret), nil
}

func stableRuntimeValidationSourceTextHash(text string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return "runtime-validation-source-" + hex.EncodeToString(digest[:])[:20]
}

func StableRuntimeCWIValidationCheckRef(definitionRevision string, item RuntimeCoverageItem) string {
	item = item.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-cwi-validation-check")
	writeHashString(h, strings.TrimSpace(definitionRevision))
	writeHashString(h, string(item.ID))
	writeHashString(h, item.SourceScope)
	writeHashString(h, item.SourceID)
	writeHashString(h, stableRuntimeValidationSourceTextHash(item.TargetText))
	return "workorch-cwi-check-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeValidationProofRequirementHash(requirement RuntimeValidationProofRequirement) string {
	requirement = requirement.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, RuntimeValidationProofRequirementSchemaCurrent)
	writeHashString(h, requirement.DefinitionRevision)
	writeHashString(h, string(requirement.CoverageWorkItemID))
	writeHashString(h, requirement.SourceScope)
	writeHashString(h, requirement.SourceID)
	writeHashString(h, requirement.SourceTextHash)
	writeHashString(h, requirement.DeclarationRef)
	writeHashString(h, requirement.ValidationRecipeID)
	writeHashStrings(h, requirement.RequiredCheckRefs)
	if len(requirement.SourceObligationChecks) != 0 {
		writeRuntimeSourceObligationChecksHash(h, requirement.SourceObligationChecks)
	}
	if len(requirement.LineageAssertions) != 0 {
		payload, _ := json.Marshal(requirement.LineageAssertions)
		writeHashString(h, "validation-proof-composition/v1")
		writeHashString(h, string(payload))
	}
	return RuntimeValidationProofRequirementHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

func NormalizeRuntimeValidationProofRequirements(values []RuntimeValidationProofRequirement) []RuntimeValidationProofRequirement {
	ret := make([]RuntimeValidationProofRequirement, 0, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.CoverageWorkItemID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].CoverageWorkItemID != ret[j].CoverageWorkItemID {
			return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID
		}
		return ret[i].Hash < ret[j].Hash
	})
	return nilIfEmpty(ret)
}

// ValidateRuntimeValidationProofRequirements rejects duplicate authority
// identities before any caller uses the normalized ordering. Normalization is
// deliberately not a conflict-resolution mechanism.
func ValidateRuntimeValidationProofRequirements(values []RuntimeValidationProofRequirement) error {
	seenHashes := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, duplicate := seenHashes[value.Hash]; duplicate {
			return fmt.Errorf("runtime validation proof requirement %q is duplicated", value.Hash)
		}
		if _, duplicate := seenCoverage[value.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation proof requirement coverage %q is duplicated", value.CoverageWorkItemID)
		}
		seenHashes[value.Hash] = struct{}{}
		seenCoverage[value.CoverageWorkItemID] = struct{}{}
	}
	return nil
}

func RuntimeValidationProofRequirementForCoverage(values []RuntimeValidationProofRequirement, coverageID common.CoverageWorkItemID) (RuntimeValidationProofRequirement, bool) {
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	var match RuntimeValidationProofRequirement
	found := false
	for _, value := range NormalizeRuntimeValidationProofRequirements(values) {
		if value.CoverageWorkItemID == coverageID {
			if found {
				return RuntimeValidationProofRequirement{}, false
			}
			match, found = value.Clone(), true
		}
	}
	return match, found
}

func sameRuntimeValidationProofRequirement(left, right RuntimeValidationProofRequirement) bool {
	left, right = left.Normalized(), right.Normalized()
	return left.Hash == right.Hash && left.DefinitionRevision == right.DefinitionRevision &&
		left.CoverageWorkItemID == right.CoverageWorkItemID && left.SourceScope == right.SourceScope &&
		left.SourceID == right.SourceID && left.SourceTextHash == right.SourceTextHash &&
		left.DeclarationRef == right.DeclarationRef && left.ValidationRecipeID == right.ValidationRecipeID &&
		reflect.DeepEqual(left.LineageAssertions, right.LineageAssertions) &&
		sameRuntimeSourceObligationChecks(left.SourceObligationChecks, right.SourceObligationChecks) &&
		sameStrings(left.RequiredCheckRefs, right.RequiredCheckRefs)
}
