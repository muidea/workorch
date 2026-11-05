package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"unicode/utf8"
)

// RuntimeValidationInputMaintenance is an owner observation of existing input
// bytes, not a readiness binding or a semantic review. Only the existing exact
// Profile input-revision protocol can authorize a later ordinary work round.
type RuntimeValidationInputMaintenance struct {
	Binding    RuntimeValidationProofRequirementBinding
	Recipe     *RuntimeValidationRecipe `json:",omitempty"`
	Files      []RuntimeValidationInputReviewFile
	TargetRefs []string
}

func (m *RuntimeValidationInputMaintenance) Clone() *RuntimeValidationInputMaintenance {
	if m == nil {
		return nil
	}
	c := *m
	c.Binding = cloneInputMaintenanceBindingRaw(m.Binding)
	c.Recipe = cloneRuntimeInputRecipe(m.Recipe)
	c.Files = slices.Clone(m.Files)
	c.TargetRefs = slices.Clone(m.TargetRefs)
	return &c
}
func (m *RuntimeValidationInputMaintenance) Hash() string {
	b, _ := json.Marshal(m)
	return RuntimeValidationInputContentHash(string(b))
}
func (m *RuntimeValidationInputMaintenance) Validate(r RuntimeValidationProofRequirement, recipe string) error {
	if m == nil {
		return nil
	}
	b := m.Binding
	if m.Recipe != nil && (m.Recipe.ValidateDeclaration() != nil || m.Recipe.ID != recipe) {
		return fmt.Errorf("input maintenance requires its original owner recipe declaration")
	}
	if b.InputReview != nil || b.Validate() != nil || b.RequirementHash != r.Hash || b.DefinitionRevision != r.DefinitionRevision || b.CoverageWorkItemID != r.CoverageWorkItemID || b.DeclarationRef != r.DeclarationRef || b.ValidationRecipeID != recipe || !reflect.DeepEqual(b.RequiredCheckRefs, r.RequiredCheckRefs) {
		return fmt.Errorf("input maintenance requires the exact original requirement, without a readiness or review claim")
	}
	if len(m.Files) == 0 || len(m.Files) > RuntimeValidationDiagnosticSourceMaxFiles || len(m.TargetRefs) == 0 || !reflect.DeepEqual(m.TargetRefs, normalizeStrings(m.TargetRefs)) {
		return fmt.Errorf("input maintenance requires complete bounded files and exact targets")
	}
	previous, size := "", 0
	for _, f := range m.Files {
		if !runtimeProspectiveTargetRefValid(f.TargetRef) || f.TargetRef <= previous || !utf8.ValidString(f.Content) || f.ContentHash != RuntimeValidationInputContentHash(f.Content) {
			return fmt.Errorf("input maintenance file identity mismatch")
		}
		previous = f.TargetRef
		size += len(f.Content)
	}
	if size > RuntimeValidationDiagnosticSourceMaxBytes {
		return fmt.Errorf("input maintenance capacity exceeded")
	}
	for _, target := range m.TargetRefs {
		found := false
		for _, f := range m.Files {
			found = found || f.TargetRef == target
		}
		if !found {
			return fmt.Errorf("input maintenance target was not observed")
		}
	}
	return nil
}

// This source retains the complete typed owner catalog and its independently
// persisted prerequisite observation. It never fabricates a provider request or
// conflict report. Original diagnostics remain inside the original catalog.
type RuntimeValidationInputRevisionObservation struct {
	Catalog     RuntimeOperationAdmissibility
	Observation RuntimePrerequisiteObservation
}

func (o *RuntimeValidationInputRevisionObservation) Clone() *RuntimeValidationInputRevisionObservation {
	if o == nil {
		return nil
	}
	return &RuntimeValidationInputRevisionObservation{Catalog: cloneRuntimeOperationAdmissibilityRaw(o.Catalog), Observation: cloneRuntimePrerequisiteObservationRaw(o.Observation)}
}
func (v RuntimeValidationInputRevision) ownerMaintenance() *RuntimeValidationInputMaintenance {
	if v.OwnerObservation == nil {
		return nil
	}
	for _, s := range v.OwnerObservation.Catalog.ValidationSurfaces {
		for _, u := range s.ProofRequirementUnavailabilities {
			if u.Requirement.Hash == v.Binding.RequirementHash {
				return u.Maintenance
			}
		}
	}
	return nil
}
func (v RuntimeValidationInputRevision) InputFiles() []RuntimeValidationInputReviewFile {
	if m := v.ownerMaintenance(); m != nil {
		return m.Files
	}
	if v.Binding.InputReview != nil {
		return v.Binding.InputReview.Files
	}
	return nil
}
func (v RuntimeValidationInputRevision) validateOwnerObservation() error {
	o := v.OwnerObservation
	if o == nil || o.Catalog.Validate() != nil || o.Observation.Validate() != nil || o.Observation.Kind != PrerequisiteKindOperationAdmissibility || o.Observation.PrerequisiteID != o.Catalog.PrerequisiteID || o.Observation.OwnerModuleID != o.Catalog.OwnerModuleID || o.Observation.ContentHash != o.Catalog.SnapshotID {
		return fmt.Errorf("input revision requires an exact observed owner catalog")
	}
	for _, ref := range []string{string(v.WorkspaceID), string(v.RunID), v.DefinitionSeedHash, v.OwnerModuleID} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("input revision requires original startup scope")
		}
	}
	if v.SourceManifestHash != "" || v.SourceLogicalRequestID != "" || v.SourceDocumentHash != "" || !reflect.DeepEqual(v.Report, RuntimeValidationInputReview{}) {
		return fmt.Errorf("structural input observation cannot claim a semantic report or request")
	}
	matches := 0
	for _, s := range o.Catalog.ValidationSurfaces {
		for _, u := range s.ProofRequirementUnavailabilities {
			if u.Maintenance == nil || !reflect.DeepEqual(u.Maintenance.Binding, v.Binding) {
				continue
			}
			if u.Validate() != nil || !reflect.DeepEqual(u.Maintenance.Recipe, v.Recipe) || u.OriginalDiagnostic.OwnerModuleID != o.Catalog.OwnerModuleID || u.OriginalDiagnostic.ObservedRevision != o.Catalog.ObservedRevision || s.InputTargetOwnerModuleID != v.OwnerModuleID || !reflect.DeepEqual(u.Maintenance.TargetRefs, v.InputTargetRefs) {
				return fmt.Errorf("structural input source mismatch")
			}
			for _, target := range v.InputTargetRefs {
				if !slices.Contains(s.InputTargetRefs, target) {
					return fmt.Errorf("structural maintenance escapes owner input scope")
				}
			}
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("structural input source is missing or ambiguous")
	}
	return nil
}

// Generation admission validates the derived maintenance product. Consumers
// of an exact formal preimage project its original value without repeating it.
func CompileRuntimeValidationInputRevisionGeneration(g RuntimeSemanticPlanningGeneration) (*RuntimeValidationInputRevision, error) {
	v, err := runtimeValidationInputRevisionGeneration(g)
	if err != nil {
		return nil, err
	}
	return v, v.Validate()
}

func runtimeValidationInputRevisionGeneration(g RuntimeSemanticPlanningGeneration) (*RuntimeValidationInputRevision, error) {
	if g.OwnerInputRevision != nil {
		return g.OwnerInputRevision.Clone(), nil
	}
	return runtimeValidationInputRevision(g.Snapshot)
}

// CompileRuntimeOwnerInputRevision selects only the current exact CWI's typed
// repairable input fact. Permission and work budget are checked by SM-2 and the
// existing scope/dispatch transactions, not by this owner fact compiler.
func CompileRuntimeOwnerInputRevision(f RuntimeFacts) (*RuntimeValidationInputRevision, error) {
	if len(f.Focus.CoverageWorkItemIDs) != 1 {
		return nil, fmt.Errorf("input maintenance requires one current CWI")
	}
	var result *RuntimeValidationInputRevision
	for _, c := range f.Resources.OperationAdmissibilities {
		for _, s := range c.ValidationSurfaces {
			for _, u := range s.ProofRequirementUnavailabilities {
				if u.Maintenance == nil || u.Requirement.CoverageWorkItemID != f.Focus.CoverageWorkItemIDs[0] {
					continue
				}
				var obs *RuntimePrerequisiteObservation
				for _, o := range f.Resources.PrerequisiteObservations {
					if o.PrerequisiteID == c.PrerequisiteID && o.ContentHash == c.SnapshotID {
						if obs != nil {
							return nil, fmt.Errorf("ambiguous input observation")
						}
						copy := o
						obs = &copy
					}
				}
				if obs == nil {
					return nil, fmt.Errorf("input maintenance lacks current observation")
				}
				v := &RuntimeValidationInputRevision{WorkspaceID: f.WorkspaceID, RunID: f.RunID, DefinitionSeedHash: f.DefinitionSeed.Hash, OwnerModuleID: s.InputTargetOwnerModuleID, InputTargetRefs: slices.Clone(u.Maintenance.TargetRefs), Binding: u.Maintenance.Binding, Recipe: cloneRuntimeInputRecipe(u.Maintenance.Recipe), OwnerObservation: &RuntimeValidationInputRevisionObservation{Catalog: c.Clone(), Observation: *obs}}
				if err := v.Validate(); err != nil {
					return nil, err
				}
				if result != nil {
					return nil, fmt.Errorf("ambiguous input maintenance")
				}
				result = v.Clone()
			}
		}
	}
	if result == nil {
		return nil, fmt.Errorf("no current repairable input observation")
	}
	return result, nil
}

func RuntimeInputRevisionSamePhysicalState(a, b *RuntimeValidationInputRevision) bool {
	return a != nil && b != nil && a.Binding.RequirementHash == b.Binding.RequirementHash && a.OwnerModuleID == b.OwnerModuleID && reflect.DeepEqual(a.InputTargetRefs, b.InputTargetRefs) && reflect.DeepEqual(a.InputFiles(), b.InputFiles())
}

// Resource invalidation deliberately removes current catalog observations. Its
// append-only owner journal retains the original observation ID/dependencies;
// the maintenance generation retains the full original bytes/catalog. Both
// forms must join on reload, without reviving that observation as current.
func ValidateRuntimeInputRevisionObservationSources(l RuntimeSemanticPlanningLedger, f RuntimeFacts, invalidations []RuntimePrerequisiteInvalidationRecord) error {
	return validateRuntimeInputRevisionObservationSourcesInSession(l, f, invalidations, nil)
}

func validateRuntimeInputRevisionObservationSourcesInSession(l RuntimeSemanticPlanningLedger, f RuntimeFacts, invalidations []RuntimePrerequisiteInvalidationRecord, session *runtimeSemanticValidationSession) error {
	for _, g := range l.AllHistory() {
		v := g.OwnerInputRevision
		if v == nil {
			continue
		}
		// The exact formal generation retains the original catalog verdict.
		// Its observation must still exist in the CURRENT owner facts/journal.
		if !session.committedGeneration(g) {
			if err := v.Validate(); err != nil {
				return err
			}
		}
		original := v.OwnerObservation.Observation
		found := false
		for _, o := range append(slices.Clone(f.Resources.PrerequisiteObservations), f.Resources.RetiredPrerequisiteObservations...) {
			if reflect.DeepEqual(o, original) {
				found = true
			}
		}
		for _, i := range invalidations {
			if slices.Contains(i.ObservationIDs, original.ObservationID) && slices.Contains(i.PrerequisiteIDs, original.PrerequisiteID) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("input revision lacks original owner observation/invalidation source")
		}
	}
	return nil
}

func cloneInputMaintenanceBindingRaw(b RuntimeValidationProofRequirementBinding) RuntimeValidationProofRequirementBinding {
	b.RequiredCheckRefs = slices.Clone(b.RequiredCheckRefs)
	b.CheckAttestations = slices.Clone(b.CheckAttestations)
	if b.InputReview != nil {
		c := b.InputReview.Clone()
		b.InputReview = &c
	}
	return b
}
