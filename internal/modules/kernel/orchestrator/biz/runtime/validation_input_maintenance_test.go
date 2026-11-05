package runtime

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This fixture isolates the private maintenance admission guard. The real-owner
// mainline separately proves aggregate CAS, effects, fresh review and delivery.
func ownerInputMaintenanceGuardFixture(t *testing.T) RuntimeAggregate {
	t.Helper()
	f := controllerFacts()
	r := runtimeTestOwnerFactValidationRequirement(t)
	b, err := rc.RuntimeValidationProofRequirementBindingForRecipe(r, r.ValidationRecipeID, runtimeTestValidationCheckAttestations(r, r.ValidationRecipeID)...)
	if err != nil {
		t.Fatal(err)
	}
	file := rc.RuntimeValidationInputReviewFile{TargetRef: "checks/input", Content: "original malformed input\n"}
	file.ContentHash = rc.RuntimeValidationInputContentHash(file.Content)
	m := &rc.RuntimeValidationInputMaintenance{Recipe: &rc.RuntimeValidationRecipe{ID: r.ValidationRecipeID, Command: "check", TimeoutMillis: 60000}, Binding: b, Files: []rc.RuntimeValidationInputReviewFile{file}, TargetRefs: []string{file.TargetRef}}
	u := rc.RuntimeValidationProofUnavailability{Requirement: r, ValidationRecipeID: r.ValidationRecipeID, Maintenance: m, OriginalDiagnostic: &rc.RuntimeOriginalDiagnostic{SchemaVersion: rc.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: rc.RuntimeModuleShell, ObservedRevision: "original", OriginalClass: "validation_readiness", OriginalCode: "structural", OriginalMessage: "original owner failure", Signature: "input/structure"}}
	c := rc.RuntimeOperationAdmissibility{PrerequisiteID: "input-owner", OwnerModuleID: rc.RuntimeModuleShell, ObservedRevision: "original", OperationKinds: []string{rc.RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: []rc.RuntimeValidationSurface{{ID: r.ValidationRecipeID, OwnerModuleID: rc.RuntimeModuleShell, OperationKind: rc.RuntimeBlockKindManagedValidationSession, Readiness: rc.RuntimeValidationSurfaceReady, InputSchemaHash: "schema", InputBindingRef: "input", ProofApplicability: rc.RuntimeValidationProofDirect, EffectScope: rc.RuntimeEffectEphemeralSession, IdempotencyClass: rc.RuntimeOperationIdempotencyAtMostOnce, InputTargetOwnerModuleID: rc.RuntimeModuleFile, InputTargetRefs: m.TargetRefs, ProofRequirementUnavailabilities: []rc.RuntimeValidationProofUnavailability{u}}}}
	c.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(c)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	o := rc.RuntimePrerequisiteObservation{PrerequisiteID: c.PrerequisiteID, Kind: rc.PrerequisiteKindOperationAdmissibility, OwnerModuleID: c.OwnerModuleID, ContentHash: c.SnapshotID, Dependencies: []rc.RuntimeFreshnessDependency{{Domain: rc.RuntimeResourceWorkspaceTree, OwnerModuleID: rc.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "original-tree"}}}
	o.ObservationID = rc.StableRuntimePrerequisiteObservationID(o)
	f.Resources.OperationAdmissibilities = []rc.RuntimeOperationAdmissibility{c}
	f.Resources.PrerequisiteObservations = []rc.RuntimePrerequisiteObservation{o}
	f.Prerequisites.Pending = nil
	f.Focus = rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{r.CoverageWorkItemID}}
	f.Coverage = []rc.RuntimeCoverageItemFact{{ID: r.CoverageWorkItemID, CWIConvergenceState: rc.RuntimeCWIConvergenceStateUnbound}}
	f.DefinitionFocuses = []rc.RuntimeDefinitionFocus{{CoverageWorkItemIDs: f.Focus.CoverageWorkItemIDs, RequiredOwnerFacts: []rc.RuntimeOwnerFactRequirement{{OwnerModuleID: c.OwnerModuleID, FactKind: rc.RuntimeOwnerFactSelectableValidationSurface, OperationKind: rc.RuntimeBlockKindManagedValidationSession, ValidationProofRequirementHash: r.Hash}}, ImplementationWorkPolicy: &rc.RuntimeImplementationWorkPolicy{InputRevisionProtocolRef: rc.RuntimeValidationInputRevisionProtocolV1, OwnerModuleID: rc.RuntimeModuleFile}}}
	return RuntimeAggregate{StateRevision: 9, Facts: f, SemanticPlanning: rc.RuntimeSemanticPlanningLedger{SchemaVersion: rc.RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: rc.RuntimeSemanticPlanningRequired, DefinitionSeed: f.DefinitionSeed}}
}

func TestOwnerInputMaintenanceRequiresPermissionAndQuiescentOriginalSource(t *testing.T) {
	base := ownerInputMaintenanceGuardFixture(t)
	accepted, changed, err := retainRuntimeOwnerInputRevision(base)
	if err != nil || !changed || accepted.SemanticPlanning.HistoryCount() != 1 {
		t.Fatalf("original source not retained: changed=%t err=%v", changed, err)
	}
	if !pendingRuntimeOwnerInputRevision(&accepted, accepted.Facts) {
		t.Fatal("retained exact source did not enable ordinary maintenance")
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"no original permission": func(a *RuntimeAggregate) { a.Facts.DefinitionFocuses[0].ImplementationWorkPolicy = nil },
		"different input owner": func(a *RuntimeAggregate) {
			a.Facts.DefinitionFocuses[0].ImplementationWorkPolicy.OwnerModuleID = rc.RuntimeModuleShell
		},
		"no applicable obligation": func(a *RuntimeAggregate) { a.Facts.DefinitionFocuses[0].RequiredOwnerFacts = nil },
		"source observation lost":  func(a *RuntimeAggregate) { a.Facts.Resources.PrerequisiteObservations = nil },
		"settled CWI": func(a *RuntimeAggregate) {
			a.Facts.Coverage[0].CWIConvergenceState = rc.RuntimeCWIConvergenceStateSettled
		},
		"active operation": func(a *RuntimeAggregate) { a.ActiveDispatch = &rc.RuntimeDispatchRecord{ID: "original"} },
		"unknown unfinished round": func(a *RuntimeAggregate) {
			a.ImplementationWork.Rounds = []rc.RuntimeImplementationWorkRound{{CoverageWorkItemID: a.Facts.Focus.CoverageWorkItemIDs[0]}}
		},
		"provider in flight": func(a *RuntimeAggregate) {
			a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{ProviderSubmitted: true, State: rc.RuntimeCandidateRequestSessionPrepared}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := base.CloneRaw()
			mutate(&a)
			before := a.CloneRaw()
			got, changed, err := retainRuntimeOwnerInputRevision(a)
			if err != nil || changed || !reflect.DeepEqual(got, before) {
				t.Fatalf("guard changed original authority: changed=%t err=%v", changed, err)
			}
		})
	}
	// A refreshed catalog identity with identical physical inputs cannot reset
	// the progress guard, even after the original maintenance round completed.
	accepted.ImplementationWork.Rounds = []rc.RuntimeImplementationWorkRound{{CoverageWorkItemID: base.Facts.Focus.CoverageWorkItemIDs[0], ScopeRequest: rc.RuntimeImplementationWorkScopeRequest{InputRevision: accepted.SemanticPlanning.History[0].OwnerInputRevision}, CompletedReceiptSetHash: "original-receipt-set"}}
	c := &accepted.Facts.Resources.OperationAdmissibilities[0]
	c.ObservedRevision = "refreshed"
	c.ValidationSurfaces[0].ProofRequirementUnavailabilities[0].OriginalDiagnostic.ObservedRevision = c.ObservedRevision
	c.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*c)
	o := &accepted.Facts.Resources.PrerequisiteObservations[0]
	o.ContentHash = c.SnapshotID
	o.ObservationID = rc.StableRuntimePrerequisiteObservationID(*o)
	if _, changed, err := retainRuntimeOwnerInputRevision(accepted); err != nil || changed {
		t.Fatal("identical physical state reset maintenance", err)
	}
}

func TestOwnerInputMaintenanceCannotHideAnotherUnavailableObligation(t *testing.T) {
	a := ownerInputMaintenanceGuardFixture(t)
	v, err := rc.CompileRuntimeOwnerInputRevision(a.Facts)
	if err != nil {
		t.Fatal(err)
	}
	u := a.Facts.Resources.OperationAdmissibilities[0].ValidationSurfaces[0].ProofRequirementUnavailabilities[0]
	u.Maintenance = nil
	u.Requirement.SourceTextHash = "another-original-criterion"
	u.Requirement.Hash = rc.StableRuntimeValidationProofRequirementHash(u.Requirement)
	a.Facts.Resources.OperationAdmissibilities[0].ValidationSurfaces[0].ProofRequirementUnavailabilities = append(a.Facts.Resources.OperationAdmissibilities[0].ValidationSurfaces[0].ProofRequirementUnavailabilities, u)
	r := a.Facts.DefinitionFocuses[0].RequiredOwnerFacts[0]
	r.ValidationProofRequirementHash = u.Requirement.Hash
	a.Facts.DefinitionFocuses[0].RequiredOwnerFacts = append(a.Facts.DefinitionFocuses[0].RequiredOwnerFacts, r)
	if runtimeOwnerInputRevisionCoversUnavailable(a.Facts, v) {
		t.Fatal("maintenance hid another original unavailable obligation")
	}
}
