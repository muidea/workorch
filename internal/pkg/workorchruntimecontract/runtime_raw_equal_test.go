package workorchruntimecontract

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeRawEqualStandardOracle(t *testing.T) {
	type cycle struct {
		Text string
		Next *cycle
	}
	a, b := &cycle{Text: "cycle"}, &cycle{Text: "cycle"}
	a.Next, b.Next = a, b
	unequal := &cycle{Text: "different"}
	unequal.Next = unequal
	nan := math.NaN()
	nans := []float64{nan}
	values := []any{nil, true, false, "", "\x00原文 \n", 1, int64(1), uint64(1),
		float32(1), float64(1), nan, complex(nan, 1), []string(nil), []string{},
		[]string{"a", "b"}, [2]string{"a", "b"}, (*cycle)(nil), a, b, unequal,
		nans, []float64{nan}, map[string]any(nil), map[string]any{},
		map[string]any{"nested": nans}, time.Now(), time.Time{},
		(func())(nil), func() {}, make(chan int)}
	// Same-pointer NaNs, cycles, nil/empty, dynamic/unexported fields and
	// differing types must agree with the standard comparator in both orders.
	for _, left := range values {
		for _, right := range values {
			if got, want := runtimeRawEqual(left, right), reflect.DeepEqual(left, right); got != want {
				t.Fatalf("raw comparison differs for %T / %T: %v != %v", left, right, got, want)
			}
		}
	}
}

func TestRuntimeRawEqualCompleteDTOMutations(t *testing.T) {
	base := terminalHistoricalAcceptanceFixture(t)
	compare := func(changed RuntimeStateRecord) {
		t.Helper()
		if got, want := runtimeRawEqual(base, changed), reflect.DeepEqual(base, changed); got != want {
			t.Fatal("complete DTO raw equality differs from standard comparison")
		}
	}
	compare(base)
	copy := base.Clone()
	compare(copy)
	ownerLineageFieldMutations(reflect.ValueOf(&copy).Elem(), func() { compare(copy) })
	if !runtimeRawEqual(base, base.Clone()) {
		t.Fatal("later comparison retained a previous business value")
	}
}

func TestRuntimeRawEqualParallelTypePlans(t *testing.T) {
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Go(func() {
			for i := 0; i < 20; i++ {
				left := struct {
					Workspace string
					Values    []int
				}{"scope", []int{i, i + 1}}
				right := struct {
					Workspace string
					Values    []int
				}{"other", []int{i, i + 1}}
				if !runtimeRawEqual(left, left) || runtimeRawEqual(left, right) {
					t.Error("type plan mixed independent values")
				}
			}
		})
	}
	wg.Wait()
}

func FuzzRuntimeRawEqualStandardOracle(f *testing.F) {
	f.Add([]byte("original\x00 bytes"), []byte("different"), true)
	f.Fuzz(func(t *testing.T, left, right []byte, present bool) {
		type value struct {
			Data   []byte
			Nested *struct{ Text string }
		}
		a, b := value{Data: left}, value{Data: right}
		if present {
			a.Nested, b.Nested = &struct{ Text string }{string(left)}, &struct{ Text string }{string(right)}
		}
		if runtimeRawEqual(a, b) != reflect.DeepEqual(a, b) {
			t.Fatal("raw comparator differs from standard oracle")
		}
	})
}

func TestRuntimeRawEqualCustomLeafTypesMatchDeepEqual(t *testing.T) {
	strVal := "match"
	boolVal := true
	type customBundle struct {
		Strings           []string
		CWIs              []common.CoverageWorkItemID
		TargetPolicies    []RuntimeOperationTargetPolicy
		Deps              []RuntimeFreshnessDependency
		RevRefs           []RuntimeOwnerRevisionRef
		RevSet            RuntimeOwnerRevisionSet
		RevSetPtr         *RuntimeOwnerRevisionSet
		RevSetEmpty       *RuntimeOwnerRevisionSet
		AppObs            []RuntimeEvidenceApplicabilityObservation
		PrereqObs         []RuntimePrerequisiteObservation
		ParamBindings     []RuntimeTargetParameterBinding
		Attestations      []RuntimeValidationCheckAttestationBinding
		Constraint        *RuntimeProspectiveTargetSatisfactionConstraint
		Obligations       []RuntimeSourceObligationCheck
		Recipes           []RuntimeValidationRecipe
		Policies          []RuntimeOperationPolicyFact
		MCPs              []RuntimeMCPCapabilityContext
		Unavailabilities  []RuntimeValidationProofUnavailability
		Transaction       AuthorityTransaction
		BindingRef        RuntimeCWIBindingRef
		Diagnostics       []RuntimeDiagnosticRecord
		Acknowledgements  []RuntimeDeliveryAcknowledgement
		NilSliceStrings   []string
		EmptySliceStrings []string
	}
	base := customBundle{
		Strings:        []string{"a", "b"},
		CWIs:           []common.CoverageWorkItemID{"cwi-1"},
		TargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget},
		Deps:           []RuntimeFreshnessDependency{{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: "file", SubjectRef: "src/a.go", Revision: "r1"}},
		RevRefs:        []RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "file", ResourceKind: "file", ResourceID: "src/a.go", SnapshotID: "s1", Revision: "r1", Fence: 3, DependencyHash: "h1"}},
		RevSet:         RuntimeOwnerRevisionSet{DefinitionRevision: "d1", Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "k", Revision: "r"}}, SetHash: "s"},
		RevSetPtr:      &RuntimeOwnerRevisionSet{DefinitionRevision: "d1", Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "k", Revision: "r"}}, SetHash: "s"},
		RevSetEmpty:    &RuntimeOwnerRevisionSet{DefinitionRevision: "d2", Entries: []RuntimeOwnerRevisionRef{}, SetHash: "s2"},
		AppObs: []RuntimeEvidenceApplicabilityObservation{
			{
				Request:                 RuntimeEvidenceApplicabilityRequest{ProtocolRef: RuntimeEvidenceApplicabilityObservationV1, RequestID: "id1", IssuedAtStateRevision: 1, EvidenceRevision: "1", SourceEvidenceID: "e1", SourceEvidenceHash: "h", SourceReceiptHash: "r", DispatchID: "d", OperationID: "o", ReconciliationRequestHash: "rr", PurposeRef: RuntimeHistoricalEffectProofV1},
				RecordedAtStateRevision: 2,
				Missing:                 false,
				CurrentDependencies:     &RuntimeOwnerRevisionSet{DefinitionRevision: "d1", Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "k", Revision: "r"}}, SetHash: "s"},
			},
		},
		PrereqObs: []RuntimePrerequisiteObservation{
			{PrerequisiteID: "p1", Kind: PrerequisiteKindOperationAdmissibility, OwnerModuleID: "shell", ObservationID: "obs1", ContentHash: "ch1", Dependencies: []RuntimeFreshnessDependency{{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: "file", SubjectRef: "main.go", Revision: "1"}}, ObservedAt: "now"},
		},
		ParamBindings: []RuntimeTargetParameterBinding{
			{Kind: "file/write", Name: "path", Values: []RuntimeTargetParameterValue{{TargetRef: "a.go", Value: "a.go"}}},
		},
		Attestations: []RuntimeValidationCheckAttestationBinding{
			{RequiredCheckRef: "c1", ObservationProtocolRef: "p", NativeCheckRef: "n1", SupplementalNativeCheckRef: "n2", ExecutionStagesHash: "sh"},
		},
		Constraint: &RuntimeProspectiveTargetSatisfactionConstraint{
			SchemaVersion: "v1", OwnerModuleID: "shell", ProspectiveSatisfactionContractRef: "c", ValidationSurfaceID: "vs", ValidationRecipeID: "vr", InputSchemaHash: "ish", CatalogObservedRevision: "rev",
			AllOf: []RuntimeProspectiveTargetClause{{ID: "cl", AllOf: []RuntimeProspectiveTargetPredicate{{Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: ".go"}}}},
		},
		Obligations: []RuntimeSourceObligationCheck{
			{ProjectDesigns: []RuntimeSourceProjectDesign{{CandidateItemRef: "ci", Decision: "d", VerificationBasis: "vb"}}, InventoryHash: "ih", ObligationID: "oid", RequirementID: "rid", VerificationID: "vid", Criterion: "crit", Declaration: "decl", CheckRef: "cr"},
		},
		Recipes: []RuntimeValidationRecipe{
			{ID: "r1", AdapterRef: "ar", InputReviewProtocolRef: "rp", Command: "go", Args: []string{"test"}, TimeoutMillis: 1000},
		},
		Policies: []RuntimeOperationPolicyFact{
			{ID: "p1", OwnerModuleID: "shell", OperationKind: "shell/run", SubjectRef: "sub", ParameterMatches: []RuntimeOperationParameterMatch{{Name: "n", String: &strVal, Boolean: &boolVal}}, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay},
		},
		MCPs: []RuntimeMCPCapabilityContext{
			{ID: "m1", ServerID: "srv", Kind: "tool", OperationKind: "mcp/tool_call", Name: "nm", DisplayName: "dn", Description: "desc", Category: "cat", Tags: []string{"t"}, UsageHint: "uh", InputSummary: "is", OutputSummary: "os", StageHints: []string{"sh"}, RiskLevel: "low", EffectScope: "none", IdempotencyClass: "idempotent", URI: "uri", Enabled: true, ApprovalPolicy: "ap", Status: "ready"},
		},
		Unavailabilities: []RuntimeValidationProofUnavailability{
			{
				Requirement: RuntimeValidationProofRequirement{
					SchemaVersion: "v1", Hash: "h", DefinitionRevision: "dr", CoverageWorkItemID: "cwi", SourceScope: "dodItem", SourceID: "sid", SourceTextHash: "sth", DeclarationRef: "dref", ValidationRecipeID: "vr",
					RequiredCheckRefs:      []string{"c1"},
					SourceObligationChecks: []RuntimeSourceObligationCheck{{InventoryHash: "ih"}},
					LineageAssertions:      []RuntimeLineageAssertion{{Rule: RuntimeLineageAssertionRule{Kind: RuntimeLineageTerminalOwnerReceipt}, RuleHash: "rh", DeclarationRef: "dr", CoverageWorkItemID: "cwi", SourceScope: "dodItem", SourceID: "sid", SourceTextHash: "sth"}},
				},
				ValidationRecipeID: "vr",
				OriginalDiagnostic: &RuntimeOriginalDiagnostic{OwnerModuleID: "shell", OriginalClass: "c", OriginalCode: "code", OriginalMessage: "msg"},
				Maintenance:        &RuntimeValidationInputMaintenance{},
			},
		},
		Transaction: AuthorityTransaction{
			AuthoritySnapshotID: "a", FocusID: "f", State: ConvergenceStateNeedStateChange, RequiredOutputHash: "roh", DefinitionRevision: "dr", EvidenceRevision: "er", CapabilityStateHash: "csh", PrerequisiteFreshnessHash: "pfh", OperationContractRevision: "ocr", AuthorityInputID: "aii", Attempt: 1,
		},
		BindingRef:        RuntimeCWIBindingRef{BindingID: "b", BindingHash: "bh", FocusID: "f", SemanticManifestHash: "smh"},
		Diagnostics:       []RuntimeDiagnosticRecord{{EventID: "e", AuthoritySnapshotID: "a", CurrentSnapshotID: "c", DispatchID: "d", Reason: "r"}},
		Acknowledgements:  []RuntimeDeliveryAcknowledgement{{WorkspaceID: "ws", RunID: "run", ResultID: "res", ResultCloseout: &RuntimeResultCloseout{}, TerminalDeliveryID: "td", Owner: RuntimeDeliveryOwnerRun, AcknowledgedState: RuntimeDeliveryAcknowledgedRunTerminal, AuthoritySnapshotID: "a", CausationID: "c", Outcome: TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: "ok", Summary: "done"}}},
		NilSliceStrings:   nil,
		EmptySliceStrings: []string{},
	}

	assertMatch := func(other customBundle) {
		t.Helper()
		if got, want := runtimeRawEqual(base, other), reflect.DeepEqual(base, other); got != want {
			t.Fatalf("custom leaf comparison mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.Strings, other.Strings), reflect.DeepEqual(base.Strings, other.Strings); got != want {
			t.Fatalf("top-level []string mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.CWIs, other.CWIs), reflect.DeepEqual(base.CWIs, other.CWIs); got != want {
			t.Fatalf("top-level []CoverageWorkItemID mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.AppObs, other.AppObs), reflect.DeepEqual(base.AppObs, other.AppObs); got != want {
			t.Fatalf("top-level []RuntimeEvidenceApplicabilityObservation mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.PrereqObs, other.PrereqObs), reflect.DeepEqual(base.PrereqObs, other.PrereqObs); got != want {
			t.Fatalf("top-level []RuntimePrerequisiteObservation mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.Deps, other.Deps), reflect.DeepEqual(base.Deps, other.Deps); got != want {
			t.Fatalf("top-level []RuntimeFreshnessDependency mismatch: got %v want %v", got, want)
		}
		if got, want := runtimeRawEqual(base.RevSetPtr, other.RevSetPtr), reflect.DeepEqual(base.RevSetPtr, other.RevSetPtr); got != want {
			t.Fatalf("top-level *RuntimeOwnerRevisionSet mismatch: got %v want %v", got, want)
		}
	}

	// Copy detached from base slices/pointers
	copyVal := base
	copyVal.Strings = append([]string(nil), base.Strings...)
	copyVal.CWIs = append([]common.CoverageWorkItemID(nil), base.CWIs...)
	copyVal.TargetPolicies = append([]RuntimeOperationTargetPolicy(nil), base.TargetPolicies...)
	copyVal.Deps = append([]RuntimeFreshnessDependency(nil), base.Deps...)
	copyVal.RevRefs = append([]RuntimeOwnerRevisionRef(nil), base.RevRefs...)
	copyVal.RevSet.Entries = append([]RuntimeOwnerRevisionRef(nil), base.RevSet.Entries...)
	rsCopy := *base.RevSetPtr
	rsCopy.Entries = append([]RuntimeOwnerRevisionRef(nil), base.RevSetPtr.Entries...)
	copyVal.RevSetPtr = &rsCopy
	rsEmpty := *base.RevSetEmpty
	rsEmpty.Entries = []RuntimeOwnerRevisionRef{}
	copyVal.RevSetEmpty = &rsEmpty
	copyVal.AppObs = CloneRuntimeEvidenceApplicabilityObservations(base.AppObs)
	copyVal.PrereqObs = []RuntimePrerequisiteObservation{base.PrereqObs[0].Normalized()}
	copyVal.ParamBindings = cloneRuntimeTargetParameterBindings(base.ParamBindings)
	copyVal.Attestations = append([]RuntimeValidationCheckAttestationBinding(nil), base.Attestations...)
	cCopy := *base.Constraint
	cCopy.AllOf = []RuntimeProspectiveTargetClause{base.Constraint.AllOf[0].Normalized()}
	copyVal.Constraint = &cCopy
	copyVal.Obligations = CloneRuntimeSourceObligationChecks(base.Obligations)
	copyVal.Recipes = []RuntimeValidationRecipe{base.Recipes[0].Normalized()}
	strCopy := *base.Policies[0].ParameterMatches[0].String
	boolCopy := *base.Policies[0].ParameterMatches[0].Boolean
	copyVal.Policies = []RuntimeOperationPolicyFact{base.Policies[0]}
	copyVal.Policies[0].ParameterMatches = []RuntimeOperationParameterMatch{{Name: base.Policies[0].ParameterMatches[0].Name, String: &strCopy, Boolean: &boolCopy}}
	copyVal.MCPs = []RuntimeMCPCapabilityContext{base.MCPs[0]}
	copyVal.MCPs[0].Tags = append([]string(nil), base.MCPs[0].Tags...)
	copyVal.MCPs[0].StageHints = append([]string(nil), base.MCPs[0].StageHints...)
	copyVal.Unavailabilities = []RuntimeValidationProofUnavailability{base.Unavailabilities[0].Normalized()}
	copyVal.Diagnostics = append([]RuntimeDiagnosticRecord(nil), base.Diagnostics...)
	copyVal.Acknowledgements = []RuntimeDeliveryAcknowledgement{base.Acknowledgements[0].CloneRaw()}
	copyVal.EmptySliceStrings = []string{}

	assertMatch(copyVal)
	ownerLineageFieldMutations(reflect.ValueOf(&copyVal).Elem(), func() { assertMatch(copyVal) })
}
