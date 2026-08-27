// Package semanticplan contains deterministic semantic-plan rules used by the
// Orchestrator owner.  It deliberately has no access to Runtime aggregates,
// repositories, EventHub or clocks. It may compile the owner-private initial
// Pending batch fact, but only biz/runtime's internal-event reducer may persist
// that initialization or select any later lifecycle transition.
package semanticplan

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ManifestIdentity is the complete immutable input used to compare a sealed
// semantic manifest with the current owner facts.  Keeping this value type
// here prevents the pure comparison from depending on RuntimeAggregate.
type ManifestIdentity struct {
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	Definition        runtimecontract.RuntimeDefinitionContext
	DefinitionSeed    runtimecontract.RuntimeDefinitionSeed
	DefinitionFocuses []runtimecontract.RuntimeDefinitionFocus
	Execution         runtimecontract.RuntimeExecutionContract
	Admissibilities   []runtimecontract.RuntimeOperationAdmissibility
	EligibleCoverage  []common.CoverageWorkItemID
	// ValidationFailureRepair is present only after a private SM-2 CWI reopen
	// persisted the exact typed rejection and Profile convergence. It selects
	// the shared repair-purpose compiler; it is never synthesized here.
	ValidationFailureRepair *runtimecontract.RuntimeValidationFailureRepairGenerationInput
}

// CompileManifest is the single focused-package entry for compiling a
// provider-visible semantic manifest.  The contract package owns the wire
// schema and hash; this wrapper keeps the Orchestrator's planning code free of
// duplicate compilation paths.
func CompileManifest(identity ManifestIdentity) (runtimecontract.RuntimeSemanticPlanManifest, error) {
	if identity.ValidationFailureRepair != nil {
		input := identity.ValidationFailureRepair.Normalized()
		return runtimecontract.CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair(
			identity.WorkspaceID,
			identity.RunID,
			identity.Definition,
			identity.DefinitionSeed,
			identity.DefinitionFocuses,
			identity.Execution,
			identity.Admissibilities,
			identity.EligibleCoverage,
			input,
		)
	}
	return runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		identity.WorkspaceID,
		identity.RunID,
		identity.Definition,
		identity.DefinitionSeed,
		identity.DefinitionFocuses,
		identity.Execution,
		identity.Admissibilities,
		identity.EligibleCoverage,
	)
}

// ValidateManifestIdentity compares a persisted manifest with the current
// canonical owner facts.  It returns only a factual conflict; it never opens
// a CWI, selects an operation or requests a refresh.
func ValidateManifestIdentity(identity ManifestIdentity, manifest runtimecontract.RuntimeSemanticPlanManifest) error {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if identity.WorkspaceID == "" || identity.RunID == "" {
		return common.ErrInvalidArgument
	}
	if err := identity.Definition.Validate(); err != nil {
		return err
	}
	if err := identity.DefinitionSeed.Validate(); err != nil {
		return err
	}
	if identity.Execution.OperationContractRevision == "" {
		return fmt.Errorf("runtime semantic manifest identity has no operation contract revision")
	}
	expected, err := CompileManifest(identity)
	if err != nil {
		return fmt.Errorf("%w: runtime semantic manifest cannot compile from canonical CWI ledger: %v", common.ErrConflict, err)
	}
	if manifest.WorkspaceID != identity.WorkspaceID || manifest.RunID != identity.RunID ||
		manifest.DefinitionRevision != identity.Definition.Normalized().Revision ||
		manifest.DefinitionSeedHash != identity.DefinitionSeed.Normalized().Hash ||
		manifest.OperationContractRevision != identity.Execution.Normalized().OperationContractRevision ||
		manifest.Hash != expected.Hash {
		return fmt.Errorf("%w: runtime semantic manifest identity is stale", common.ErrConflict)
	}
	return nil
}

// DocumentByBatchID returns a detached document so callers cannot mutate the
// persisted ledger through a helper result.
func DocumentByBatchID(values []runtimecontract.RuntimeSemanticPlanDocument, batchID string) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	batchID = strings.TrimSpace(batchID)
	for _, value := range values {
		if strings.TrimSpace(value.BatchID) == batchID {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticPlanDocument{}, false
}

// BatchByID returns a detached batch for the same reason as
// DocumentByBatchID. Empty identities never match.
func BatchByID(values []runtimecontract.RuntimeSemanticPlanBatch, batchID string) (runtimecontract.RuntimeSemanticPlanBatch, bool) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return runtimecontract.RuntimeSemanticPlanBatch{}, false
	}
	for _, value := range values {
		if strings.TrimSpace(value.BatchID) == batchID {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticPlanBatch{}, false
}

// ExistingTargetContext is a bounded, owner-neutral projection of the latest
// catalog and target facts relevant to a semantic replan.  It is descriptive:
// ExistingOperationKinds/Variants are admissible alternatives published by an
// owner, not a next-action decision by the Orchestrator.
type ExistingTargetContext struct {
	OwnerModuleID             string
	FailureClass              string
	FailureCode               string
	FailureSignature          string
	OperationID               string
	TargetRefs                []string
	ExpectedRevision          string
	ObservedRevision          string
	CatalogSnapshotID         string
	CatalogObservedRevision   string
	CatalogTargetRefs         []string
	ConstraintRefs            []string
	ExistingOperationKinds    []string
	ExistingOperationVariants []string
	ExistingTargetObserved    bool
}

// BuildExistingTargetContext projects only typed fields. Raw owner messages
// are intentionally excluded so this value is safe to render in a provider
// correction tail and stable enough to test independently.
func BuildExistingTargetContext(manifest runtimecontract.RuntimeSemanticPlanManifest, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (ExistingTargetContext, error) {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return ExistingTargetContext{}, err
	}
	if diagnostic == nil {
		return ExistingTargetContext{}, nil
	}
	owner := strings.TrimSpace(diagnostic.OwnerModuleID)
	if owner == "" {
		return ExistingTargetContext{}, fmt.Errorf("semantic replan diagnostic has no owner module")
	}
	var catalog runtimecontract.RuntimeOperationAdmissibility
	found := false
	for _, value := range manifest.OwnerCatalogs {
		if value.OwnerModuleID == owner {
			catalog = value.Normalized()
			found = true
			break
		}
	}
	if !found {
		// Provider/document diagnostics (for example an order-shape error from
		// the LLM owner) have no capability catalog. They still belong in the
		// typed rejection/correction lineage, but do not imply an existing-target
		// fact and must not be turned into a synthetic catalog error.
		return ExistingTargetContext{}, nil
	}
	context := ExistingTargetContext{
		OwnerModuleID:           owner,
		FailureClass:            strings.TrimSpace(diagnostic.OriginalClass),
		FailureCode:             strings.TrimSpace(diagnostic.OriginalCode),
		FailureSignature:        strings.TrimSpace(diagnostic.Signature),
		OperationID:             strings.TrimSpace(diagnostic.OperationID),
		TargetRefs:              append([]string(nil), diagnostic.TargetRefs...),
		ExpectedRevision:        strings.TrimSpace(diagnostic.ExpectedRevision),
		ObservedRevision:        strings.TrimSpace(diagnostic.ObservedRevision),
		CatalogSnapshotID:       catalog.SnapshotID,
		CatalogObservedRevision: catalog.ObservedRevision,
		CatalogTargetRefs:       append([]string(nil), catalog.TargetRefs...),
		ConstraintRefs:          append([]string(nil), catalog.ConstraintRefs...),
	}
	if observation := diagnostic.TargetObservation; observation != nil {
		observed := observation.Normalized()
		context.TargetRefs = appendUnique(context.TargetRefs, observed.TargetRefs...)
		if context.ObservedRevision == "" {
			context.ObservedRevision = observed.ObservedRevision
		}
		context.ExistingTargetObserved = len(observed.TargetRefs) != 0 || observed.ObservedRevision != ""
	}
	for _, target := range context.TargetRefs {
		if contains(catalog.TargetRefs, target) {
			context.ExistingTargetObserved = true
			break
		}
	}
	for _, variant := range catalog.OperationVariants {
		variant = variant.Normalized()
		context.ConstraintRefs = appendUnique(context.ConstraintRefs, variant.ConstraintRefs...)
		allowedExact, _, _, _, errExact := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, variant.Kind, variant.VariantID, runtimecontract.RuntimeOperationTargetPolicyExactTarget)
		if errExact != nil {
			return ExistingTargetContext{}, errExact
		}
		allowedExisting, _, _, _, errExisting := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, variant.Kind, variant.VariantID, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
		if errExisting != nil {
			return ExistingTargetContext{}, errExisting
		}
		if allowedExact || allowedExisting {
			if variant.VariantID == "" {
				context.ExistingOperationKinds = appendUnique(context.ExistingOperationKinds, variant.Kind)
			} else {
				context.ExistingOperationVariants = appendUnique(context.ExistingOperationVariants, variant.Kind+"/"+variant.VariantID)
			}
		}
	}
	// Non-discriminated catalogs expose a kind-level existing target fact.
	for _, kind := range catalog.OperationKinds {
		if contract, found := runtimecontract.RuntimeOperationContractForKind(kind); found && len(contract.Variants) != 0 {
			continue
		}
		allowedExact, _, _, _, errExact := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", runtimecontract.RuntimeOperationTargetPolicyExactTarget)
		if errExact != nil {
			return ExistingTargetContext{}, errExact
		}
		allowedExisting, _, _, _, errExisting := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
		if errExisting != nil {
			return ExistingTargetContext{}, errExisting
		}
		if allowedExact || allowedExisting {
			context.ExistingOperationKinds = appendUnique(context.ExistingOperationKinds, runtimecontract.CanonicalRuntimeBlockKind(kind))
		}
	}
	context.TargetRefs = uniqueStable(context.TargetRefs)
	context.CatalogTargetRefs = uniqueStable(context.CatalogTargetRefs)
	context.ConstraintRefs = uniqueStable(context.ConstraintRefs)
	context.ExistingOperationKinds = uniqueStable(context.ExistingOperationKinds)
	context.ExistingOperationVariants = uniqueStable(context.ExistingOperationVariants)
	return context, nil
}

func appendUnique(values []string, additions ...string) []string {
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || contains(values, value) {
			continue
		}
		values = append(values, value)
	}
	return values
}

func uniqueStable(values []string) []string {
	return appendUnique(nil, values...)
}

func contains(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
