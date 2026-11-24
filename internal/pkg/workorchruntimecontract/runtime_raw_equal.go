package workorchruntimecontract

import (
	"reflect"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Compare complete raw values with DeepEqual semantics. The plan contains type
// instructions only: no business values, hashes, validity or owner scope survive
// a comparison. Unlike DeepEqual's general graph walk, ordinary finite DTO trees
// need no per-node visited map. Recursive graphs and opaque values still use the
// standard comparator. This changes comparison cost, never a validation gate.
type runtimeRawEqualPlan struct {
	equal func(reflect.Value, reflect.Value, int) bool
}

var runtimeRawEqualPlans sync.Map // reflect.Type -> *runtimeRawEqualPlan

func RuntimeRawEqual(left, right any) bool {
	return runtimeRawEqual(left, right)
}

func runtimeRawEqual(left, right any) bool {
	switch l := left.(type) {
	case []string:
		if r, ok := right.([]string); ok {
			return equalComparableSlices(l, r)
		}
	case []common.CoverageWorkItemID:
		if r, ok := right.([]common.CoverageWorkItemID); ok {
			return equalComparableSlices(l, r)
		}
	case []RuntimeEvidenceApplicabilityObservation:
		if r, ok := right.([]RuntimeEvidenceApplicabilityObservation); ok {
			return equalSlices(l, r, equalRuntimeEvidenceApplicabilityObservation)
		}
	case []RuntimePrerequisiteObservation:
		if r, ok := right.([]RuntimePrerequisiteObservation); ok {
			return equalSlices(l, r, equalRuntimePrerequisiteObservation)
		}
	case []RuntimeFreshnessDependency:
		if r, ok := right.([]RuntimeFreshnessDependency); ok {
			return equalComparableSlices(l, r)
		}
	case *RuntimeOwnerRevisionSet:
		if r, ok := right.(*RuntimeOwnerRevisionSet); ok {
			return equalRuntimeOwnerRevisionSetPtr(l, r)
		}
	case []RuntimeCandidateRequestSession:
		if r, ok := right.([]RuntimeCandidateRequestSession); ok {
			return equalSlices(l, r, equalRuntimeCandidateRequestSession)
		}
	case []RuntimeCandidateAttemptRecord:
		if r, ok := right.([]RuntimeCandidateAttemptRecord); ok {
			return equalSlices(l, r, equalRuntimeCandidateAttemptRecord)
		}
	}
	a, b := reflect.ValueOf(left), reflect.ValueOf(right)
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	t := a.Type()
	value, ok := runtimeRawEqualPlans.Load(t)
	if !ok {
		// Build privately, including recursive edges, before publication. Losing
		// a concurrent construction only discards identical type instructions.
		plan := buildRuntimeRawEqualPlan(t, map[reflect.Type]*runtimeRawEqualPlan{})
		value, _ = runtimeRawEqualPlans.LoadOrStore(t, plan)
	}
	return value.(*runtimeRawEqualPlan).equal(a, b, 0)
}

func reflectValueAs[T any](v reflect.Value) T {
	val, _ := reflect.TypeAssert[T](v)
	return val
}

func equalComparableSlices[T comparable](a, b []T) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSlices[T any](a, b []T, eq func(*T, *T) bool) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true
	}
	for i := range a {
		if !eq(&a[i], &b[i]) {
			return false
		}
	}
	return true
}

func equalStringPtr(a, b *string) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalBoolPtr(a, b *bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalRuntimeOwnerRevisionSet(a, b *RuntimeOwnerRevisionSet) bool {
	return a.DefinitionRevision == b.DefinitionRevision &&
		a.SetHash == b.SetHash &&
		equalComparableSlices(a.Entries, b.Entries)
}

func equalRuntimeOwnerRevisionSetPtr(a, b *RuntimeOwnerRevisionSet) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equalRuntimeOwnerRevisionSet(a, b)
}

func equalRuntimeEvidenceApplicabilityObservation(a, b *RuntimeEvidenceApplicabilityObservation) bool {
	return a.RecordedAtStateRevision == b.RecordedAtStateRevision &&
		a.Missing == b.Missing &&
		a.Request == b.Request &&
		equalRuntimeOwnerRevisionSetPtr(a.CurrentDependencies, b.CurrentDependencies)
}

func equalRuntimePrerequisiteObservation(a, b *RuntimePrerequisiteObservation) bool {
	return a.PrerequisiteID == b.PrerequisiteID &&
		a.Kind == b.Kind &&
		a.OwnerModuleID == b.OwnerModuleID &&
		a.ObservationID == b.ObservationID &&
		a.ContentHash == b.ContentHash &&
		a.ObservedAt == b.ObservedAt &&
		equalComparableSlices(a.Dependencies, b.Dependencies)
}

func equalRuntimeTargetParameterBinding(a, b *RuntimeTargetParameterBinding) bool {
	return a.Kind == b.Kind &&
		a.Name == b.Name &&
		equalComparableSlices(a.Values, b.Values)
}

func equalRuntimeValidationRecipe(a, b *RuntimeValidationRecipe) bool {
	return a.ID == b.ID &&
		a.AdapterRef == b.AdapterRef &&
		a.InputReviewProtocolRef == b.InputReviewProtocolRef &&
		a.Command == b.Command &&
		a.TimeoutMillis == b.TimeoutMillis &&
		equalComparableSlices(a.Args, b.Args)
}

func equalRuntimeOperationParameterMatch(a, b *RuntimeOperationParameterMatch) bool {
	return a.Name == b.Name &&
		equalStringPtr(a.String, b.String) &&
		equalBoolPtr(a.Boolean, b.Boolean)
}

func equalRuntimeOperationPolicyFact(a, b *RuntimeOperationPolicyFact) bool {
	return a.ID == b.ID &&
		a.OwnerModuleID == b.OwnerModuleID &&
		a.OperationKind == b.OperationKind &&
		a.SubjectRef == b.SubjectRef &&
		a.EffectScope == b.EffectScope &&
		a.IdempotencyClass == b.IdempotencyClass &&
		equalSlices(a.ParameterMatches, b.ParameterMatches, equalRuntimeOperationParameterMatch)
}

func equalRuntimeMCPCapabilityContext(a, b *RuntimeMCPCapabilityContext) bool {
	return a.ID == b.ID &&
		a.ServerID == b.ServerID &&
		a.Kind == b.Kind &&
		a.OperationKind == b.OperationKind &&
		a.Name == b.Name &&
		a.DisplayName == b.DisplayName &&
		a.Description == b.Description &&
		a.Category == b.Category &&
		a.UsageHint == b.UsageHint &&
		a.InputSummary == b.InputSummary &&
		a.OutputSummary == b.OutputSummary &&
		a.RiskLevel == b.RiskLevel &&
		a.EffectScope == b.EffectScope &&
		a.IdempotencyClass == b.IdempotencyClass &&
		a.URI == b.URI &&
		a.Enabled == b.Enabled &&
		a.ApprovalPolicy == b.ApprovalPolicy &&
		a.Status == b.Status &&
		equalComparableSlices(a.Tags, b.Tags) &&
		equalComparableSlices(a.StageHints, b.StageHints)
}

func equalRuntimeSourceObligationCheck(a, b *RuntimeSourceObligationCheck) bool {
	return a.InventoryHash == b.InventoryHash &&
		a.ObligationID == b.ObligationID &&
		a.RequirementID == b.RequirementID &&
		a.VerificationID == b.VerificationID &&
		a.Criterion == b.Criterion &&
		a.Declaration == b.Declaration &&
		a.CheckRef == b.CheckRef &&
		equalComparableSlices(a.ProjectDesigns, b.ProjectDesigns)
}

func equalRuntimeProspectiveTargetClause(a, b *RuntimeProspectiveTargetClause) bool {
	return a.ID == b.ID &&
		equalComparableSlices(a.AllOf, b.AllOf)
}

func equalRuntimeProspectiveTargetSatisfactionConstraint(a, b *RuntimeProspectiveTargetSatisfactionConstraint) bool {
	return a.SchemaVersion == b.SchemaVersion &&
		a.OwnerModuleID == b.OwnerModuleID &&
		a.ProspectiveSatisfactionContractRef == b.ProspectiveSatisfactionContractRef &&
		a.ValidationSurfaceID == b.ValidationSurfaceID &&
		a.ValidationRecipeID == b.ValidationRecipeID &&
		a.InputSchemaHash == b.InputSchemaHash &&
		a.CatalogObservedRevision == b.CatalogObservedRevision &&
		equalSlices(a.AllOf, b.AllOf, equalRuntimeProspectiveTargetClause)
}

func equalRuntimeValidationProofUnavailability(a, b *RuntimeValidationProofUnavailability) bool {
	if a.ValidationRecipeID != b.ValidationRecipeID ||
		a.Requirement.SchemaVersion != b.Requirement.SchemaVersion ||
		a.Requirement.Hash != b.Requirement.Hash ||
		a.Requirement.DefinitionRevision != b.Requirement.DefinitionRevision ||
		a.Requirement.CoverageWorkItemID != b.Requirement.CoverageWorkItemID ||
		a.Requirement.SourceScope != b.Requirement.SourceScope ||
		a.Requirement.SourceID != b.Requirement.SourceID ||
		a.Requirement.SourceTextHash != b.Requirement.SourceTextHash ||
		a.Requirement.DeclarationRef != b.Requirement.DeclarationRef ||
		a.Requirement.ValidationRecipeID != b.Requirement.ValidationRecipeID ||
		!equalComparableSlices(a.Requirement.RequiredCheckRefs, b.Requirement.RequiredCheckRefs) ||
		!equalSlices(a.Requirement.SourceObligationChecks, b.Requirement.SourceObligationChecks, equalRuntimeSourceObligationCheck) ||
		!equalComparableSlices(a.Requirement.LineageAssertions, b.Requirement.LineageAssertions) {
		return false
	}
	if a.OriginalDiagnostic != b.OriginalDiagnostic {
		if a.OriginalDiagnostic == nil || b.OriginalDiagnostic == nil || !runtimeRawEqual(a.OriginalDiagnostic, b.OriginalDiagnostic) {
			return false
		}
	}
	if a.Maintenance != b.Maintenance {
		if a.Maintenance == nil || b.Maintenance == nil || !runtimeRawEqual(a.Maintenance, b.Maintenance) {
			return false
		}
	}
	return true
}

func equalRuntimeDeliveryAcknowledgement(a, b *RuntimeDeliveryAcknowledgement) bool {
	if a.WorkspaceID != b.WorkspaceID ||
		a.RunID != b.RunID ||
		a.ResultID != b.ResultID ||
		a.TerminalDeliveryID != b.TerminalDeliveryID ||
		a.Owner != b.Owner ||
		a.AcknowledgedState != b.AcknowledgedState ||
		a.AuthoritySnapshotID != b.AuthoritySnapshotID ||
		a.CausationID != b.CausationID ||
		a.Outcome != b.Outcome {
		return false
	}
	if a.ResultCloseout != b.ResultCloseout {
		if a.ResultCloseout == nil || b.ResultCloseout == nil || !runtimeRawEqual(a.ResultCloseout, b.ResultCloseout) {
			return false
		}
	}
	return true
}

func equalPtr[T any](a, b *T) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return runtimeRawEqual(a, b)
}

func equalRuntimeCandidateRequestSession(a, b *RuntimeCandidateRequestSession) bool {
	if a.LogicalRequestID != b.LogicalRequestID ||
		a.DocumentKind != b.DocumentKind ||
		a.StablePrefixHash != b.StablePrefixHash ||
		a.ManifestHash != b.ManifestHash ||
		a.RepairCommitmentHash != b.RepairCommitmentHash ||
		a.CorrectionHash != b.CorrectionHash ||
		a.State != b.State ||
		a.NextSequence != b.NextSequence ||
		a.DocumentRecoveryOutcomes != b.DocumentRecoveryOutcomes ||
		a.TransportOutcomes != b.TransportOutcomes ||
		a.TransportRetryDueAt != b.TransportRetryDueAt ||
		a.LastPhysicalCallID != b.LastPhysicalCallID ||
		a.ProviderSubmitted != b.ProviderSubmitted ||
		a.LastOutcome != b.LastOutcome ||
		a.Transaction != b.Transaction {
		return false
	}
	return equalPtr(a.InputRef, b.InputRef) &&
		equalPtr(a.ExpectedDocument, b.ExpectedDocument) &&
		equalPtr(a.CandidateManifest, b.CandidateManifest) &&
		equalPtr(a.RepairCommitment, b.RepairCommitment) &&
		equalPtr(a.LastDocumentFraming, b.LastDocumentFraming)
}

func equalRuntimeCandidateAttemptRecord(a, b *RuntimeCandidateAttemptRecord) bool {
	if a.AuthorityContractID != b.AuthorityContractID ||
		a.Attempt != b.Attempt ||
		a.StablePrefixHash != b.StablePrefixHash ||
		a.ManifestHash != b.ManifestHash ||
		a.LogicalRequestID != b.LogicalRequestID ||
		a.State != b.State ||
		a.CandidateHash != b.CandidateHash ||
		a.DispatchID != b.DispatchID ||
		a.TargetAuthorizationHash != b.TargetAuthorizationHash ||
		a.AdmissionCoverageBindingHash != b.AdmissionCoverageBindingHash ||
		a.ValidationRecipeHash != b.ValidationRecipeHash ||
		a.RepairCommitmentHash != b.RepairCommitmentHash ||
		a.DispatchAuthorityHash != b.DispatchAuthorityHash ||
		a.Transaction != b.Transaction {
		return false
	}
	return equalPtr(a.AdmissionRepairCommitment, b.AdmissionRepairCommitment) &&
		equalPtr(a.AdmissionImplementationWorkCommitment, b.AdmissionImplementationWorkCommitment) &&
		equalPtr(a.Proposal, b.Proposal) &&
		equalPtr(a.Failure, b.Failure) &&
		equalPtr(a.Correction, b.Correction) &&
		equalPtr(a.Stale, b.Stale) &&
		equalPtr(a.OwnerFeasibility, b.OwnerFeasibility) &&
		equalPtr(a.BindingContradiction, b.BindingContradiction) &&
		equalPtr(a.PrerequisiteRequest, b.PrerequisiteRequest) &&
		equalPtr(a.ApplicabilityInvalidationRequest, b.ApplicabilityInvalidationRequest) &&
		runtimeRawEqual(a.AdmissionTargetAuthorizations, b.AdmissionTargetAuthorizations) &&
		runtimeRawEqual(a.AdmissionCoverageBindings, b.AdmissionCoverageBindings) &&
		runtimeRawEqual(a.AdmissionValidationRecipes, b.AdmissionValidationRecipes) &&
		runtimeRawEqual(a.Conformances, b.Conformances) &&
		runtimeRawEqual(a.Feasibilities, b.Feasibilities) &&
		runtimeRawEqual(a.FeasibilityCatalogs, b.FeasibilityCatalogs)
}

func comparablePlan[T comparable]() func(reflect.Value, reflect.Value, int) bool {
	return func(a, b reflect.Value, _ int) bool {
		return reflectValueAs[T](a) == reflectValueAs[T](b)
	}
}

func comparableSlicePlan[T comparable]() func(reflect.Value, reflect.Value, int) bool {
	return func(a, b reflect.Value, _ int) bool {
		return equalComparableSlices(reflectValueAs[[]T](a), reflectValueAs[[]T](b))
	}
}

func slicePlan[T any](eq func(*T, *T) bool) func(reflect.Value, reflect.Value, int) bool {
	return func(a, b reflect.Value, _ int) bool {
		return equalSlices(reflectValueAs[[]T](a), reflectValueAs[[]T](b), eq)
	}
}

func structPlan[T any](eq func(*T, *T) bool) func(reflect.Value, reflect.Value, int) bool {
	return func(a, b reflect.Value, _ int) bool {
		av := reflectValueAs[T](a)
		bv := reflectValueAs[T](b)
		return eq(&av, &bv)
	}
}

func customRuntimeRawEqualPlan(t reflect.Type) func(reflect.Value, reflect.Value, int) bool {
	switch t {
	case reflect.TypeFor[[]string]():
		return comparableSlicePlan[string]()
	case reflect.TypeFor[[]common.CoverageWorkItemID]():
		return comparableSlicePlan[common.CoverageWorkItemID]()
	case reflect.TypeFor[[]RuntimeOperationTargetPolicy]():
		return comparableSlicePlan[RuntimeOperationTargetPolicy]()
	case reflect.TypeFor[RuntimeFreshnessDependency]():
		return comparablePlan[RuntimeFreshnessDependency]()
	case reflect.TypeFor[[]RuntimeFreshnessDependency]():
		return comparableSlicePlan[RuntimeFreshnessDependency]()
	case reflect.TypeFor[RuntimeOwnerRevisionRef]():
		return comparablePlan[RuntimeOwnerRevisionRef]()
	case reflect.TypeFor[[]RuntimeOwnerRevisionRef]():
		return comparableSlicePlan[RuntimeOwnerRevisionRef]()
	case reflect.TypeFor[RuntimeOwnerRevisionSet]():
		return structPlan(equalRuntimeOwnerRevisionSet)
	case reflect.TypeFor[*RuntimeOwnerRevisionSet]():
		return func(a, b reflect.Value, _ int) bool {
			return equalRuntimeOwnerRevisionSetPtr(reflectValueAs[*RuntimeOwnerRevisionSet](a), reflectValueAs[*RuntimeOwnerRevisionSet](b))
		}
	case reflect.TypeFor[RuntimeEvidenceApplicabilityRequest]():
		return comparablePlan[RuntimeEvidenceApplicabilityRequest]()
	case reflect.TypeFor[RuntimeEvidenceApplicabilityObservation]():
		return structPlan(equalRuntimeEvidenceApplicabilityObservation)
	case reflect.TypeFor[[]RuntimeEvidenceApplicabilityObservation]():
		return slicePlan(equalRuntimeEvidenceApplicabilityObservation)
	case reflect.TypeFor[RuntimePrerequisiteObservation]():
		return structPlan(equalRuntimePrerequisiteObservation)
	case reflect.TypeFor[[]RuntimePrerequisiteObservation]():
		return slicePlan(equalRuntimePrerequisiteObservation)
	case reflect.TypeFor[RuntimeTargetParameterValue]():
		return comparablePlan[RuntimeTargetParameterValue]()
	case reflect.TypeFor[[]RuntimeTargetParameterValue]():
		return comparableSlicePlan[RuntimeTargetParameterValue]()
	case reflect.TypeFor[RuntimeTargetParameterBinding]():
		return structPlan(equalRuntimeTargetParameterBinding)
	case reflect.TypeFor[[]RuntimeTargetParameterBinding]():
		return slicePlan(equalRuntimeTargetParameterBinding)
	case reflect.TypeFor[RuntimeValidationCheckAttestationBinding]():
		return comparablePlan[RuntimeValidationCheckAttestationBinding]()
	case reflect.TypeFor[[]RuntimeValidationCheckAttestationBinding]():
		return comparableSlicePlan[RuntimeValidationCheckAttestationBinding]()
	case reflect.TypeFor[RuntimeProspectiveTargetPredicate]():
		return comparablePlan[RuntimeProspectiveTargetPredicate]()
	case reflect.TypeFor[[]RuntimeProspectiveTargetPredicate]():
		return comparableSlicePlan[RuntimeProspectiveTargetPredicate]()
	case reflect.TypeFor[RuntimeProspectiveTargetClause]():
		return structPlan(equalRuntimeProspectiveTargetClause)
	case reflect.TypeFor[[]RuntimeProspectiveTargetClause]():
		return slicePlan(equalRuntimeProspectiveTargetClause)
	case reflect.TypeFor[RuntimeProspectiveTargetSatisfactionConstraint]():
		return structPlan(equalRuntimeProspectiveTargetSatisfactionConstraint)
	case reflect.TypeFor[*RuntimeProspectiveTargetSatisfactionConstraint]():
		return func(a, b reflect.Value, _ int) bool {
			pa := reflectValueAs[*RuntimeProspectiveTargetSatisfactionConstraint](a)
			pb := reflectValueAs[*RuntimeProspectiveTargetSatisfactionConstraint](b)
			if pa == pb {
				return true
			}
			if pa == nil || pb == nil {
				return false
			}
			return equalRuntimeProspectiveTargetSatisfactionConstraint(pa, pb)
		}
	case reflect.TypeFor[RuntimeSourceProjectDesign]():
		return comparablePlan[RuntimeSourceProjectDesign]()
	case reflect.TypeFor[[]RuntimeSourceProjectDesign]():
		return comparableSlicePlan[RuntimeSourceProjectDesign]()
	case reflect.TypeFor[RuntimeSourceObligationCheck]():
		return structPlan(equalRuntimeSourceObligationCheck)
	case reflect.TypeFor[[]RuntimeSourceObligationCheck]():
		return slicePlan(equalRuntimeSourceObligationCheck)
	case reflect.TypeFor[RuntimeValidationRecipe]():
		return structPlan(equalRuntimeValidationRecipe)
	case reflect.TypeFor[[]RuntimeValidationRecipe]():
		return slicePlan(equalRuntimeValidationRecipe)
	case reflect.TypeFor[RuntimeOperationParameterMatch]():
		return structPlan(equalRuntimeOperationParameterMatch)
	case reflect.TypeFor[[]RuntimeOperationParameterMatch]():
		return slicePlan(equalRuntimeOperationParameterMatch)
	case reflect.TypeFor[RuntimeOperationPolicyFact]():
		return structPlan(equalRuntimeOperationPolicyFact)
	case reflect.TypeFor[[]RuntimeOperationPolicyFact]():
		return slicePlan(equalRuntimeOperationPolicyFact)
	case reflect.TypeFor[RuntimeMCPCapabilityContext]():
		return structPlan(equalRuntimeMCPCapabilityContext)
	case reflect.TypeFor[[]RuntimeMCPCapabilityContext]():
		return slicePlan(equalRuntimeMCPCapabilityContext)
	case reflect.TypeFor[RuntimeValidationProofUnavailability]():
		return structPlan(equalRuntimeValidationProofUnavailability)
	case reflect.TypeFor[[]RuntimeValidationProofUnavailability]():
		return slicePlan(equalRuntimeValidationProofUnavailability)
	case reflect.TypeFor[AuthorityTransaction]():
		return comparablePlan[AuthorityTransaction]()
	case reflect.TypeFor[RuntimeCWIBindingRef]():
		return comparablePlan[RuntimeCWIBindingRef]()
	case reflect.TypeFor[RuntimeDiagnosticRecord]():
		return comparablePlan[RuntimeDiagnosticRecord]()
	case reflect.TypeFor[[]RuntimeDiagnosticRecord]():
		return comparableSlicePlan[RuntimeDiagnosticRecord]()
	case reflect.TypeFor[RuntimeDeliveryAcknowledgement]():
		return structPlan(equalRuntimeDeliveryAcknowledgement)
	case reflect.TypeFor[[]RuntimeDeliveryAcknowledgement]():
		return slicePlan(equalRuntimeDeliveryAcknowledgement)
	case reflect.TypeFor[RuntimeCandidateRequestSession]():
		return structPlan(equalRuntimeCandidateRequestSession)
	case reflect.TypeFor[[]RuntimeCandidateRequestSession]():
		return slicePlan(equalRuntimeCandidateRequestSession)
	case reflect.TypeFor[RuntimeCandidateAttemptRecord]():
		return structPlan(equalRuntimeCandidateAttemptRecord)
	case reflect.TypeFor[[]RuntimeCandidateAttemptRecord]():
		return slicePlan(equalRuntimeCandidateAttemptRecord)
	default:
		return nil
	}
}

func buildRuntimeRawEqualPlan(t reflect.Type, plans map[reflect.Type]*runtimeRawEqualPlan) *runtimeRawEqualPlan {
	if plan := plans[t]; plan != nil {
		return plan
	}
	p := &runtimeRawEqualPlan{}
	plans[t] = p
	if custom := customRuntimeRawEqualPlan(t); custom != nil {
		p.equal = custom
		return p
	}
	standard := func(a, b reflect.Value, _ int) bool { return reflect.DeepEqual(a.Interface(), b.Interface()) }
	switch t.Kind() {
	case reflect.Bool:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Bool() == b.Bool() }
	case reflect.String:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.String() == b.String() }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Int() == b.Int() }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Uint() == b.Uint() }
	case reflect.Float32, reflect.Float64:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Float() == b.Float() }
	case reflect.Complex64, reflect.Complex128:
		p.equal = func(a, b reflect.Value, _ int) bool { return a.Complex() == b.Complex() }
	case reflect.Struct:
		fields := make([]*runtimeRawEqualPlan, t.NumField())
		for i := range fields {
			if t.Field(i).PkgPath != "" {
				p.equal = standard // preserve opaque/unexported-field semantics
				return p
			}
			fields[i] = buildRuntimeRawEqualPlan(t.Field(i).Type, plans)
		}
		p.equal = func(a, b reflect.Value, depth int) bool {
			if depth >= 64 {
				return standard(a, b, depth)
			}
			for i, field := range fields {
				if !field.equal(a.Field(i), b.Field(i), depth+1) {
					return false
				}
			}
			return true
		}
	case reflect.Pointer:
		element := buildRuntimeRawEqualPlan(t.Elem(), plans)
		p.equal = func(a, b reflect.Value, depth int) bool {
			if a.IsNil() || b.IsNil() || a.Pointer() == b.Pointer() {
				return a.IsNil() == b.IsNil() && a.Pointer() == b.Pointer()
			}
			if depth >= 64 {
				return standard(a, b, depth)
			}
			return element.equal(a.Elem(), b.Elem(), depth+1)
		}
	case reflect.Slice, reflect.Array:
		element := buildRuntimeRawEqualPlan(t.Elem(), plans)
		slice := t.Kind() == reflect.Slice
		p.equal = func(a, b reflect.Value, depth int) bool {
			if a.Len() != b.Len() || slice && a.IsNil() != b.IsNil() {
				return false
			}
			if slice && a.Pointer() == b.Pointer() {
				return true
			}
			if depth >= 64 {
				return standard(a, b, depth)
			}
			for i := 0; i < a.Len(); i++ {
				if !element.equal(a.Index(i), b.Index(i), depth+1) {
					return false
				}
			}
			return true
		}
	default:
		p.equal = standard // maps, dynamic interfaces, funcs and opaque handles
	}
	return p
}
