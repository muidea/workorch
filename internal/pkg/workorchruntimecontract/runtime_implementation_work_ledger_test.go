package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func implementationWorkLedgerFixture(t *testing.T) (RuntimeImplementationWorkLedger, RuntimeImplementationWorkLedgerSources) {
	t.Helper()
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	sources := RuntimeImplementationWorkLedgerSources{WorkspaceID: "ws-1", RunID: "run-1", StartupInputHash: "startup-1", Seed: seed, Focuses: focuses, Profiles: profiles, MaxRecords: 16}
	observation := workScopeObservationFixture(t)
	r := RuntimeImplementationWorkRound{WorkspaceID: sources.WorkspaceID, RunID: sources.RunID, StartupInputHash: sources.StartupInputHash, DefinitionSeedHash: seed.Hash,
		CoverageWorkItemID: "cwi-1", PolicyHash: StableRuntimeImplementationWorkPolicyHash(observation.Request.Policy), Ordinal: 1,
		ScopeRequest: observation.Request, ScopeObservation: &observation}
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: "file-catalog", OwnerModuleID: RuntimeModuleFile, ObservedRevision: "tree-1", OperationKinds: []string{StepKindFileWrite},
		TargetRefs: []string{"resource-a"}, SelectorRefs: []string{"work-scope"}}
	catalog = catalog.Normalized()
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	catalogObservation := observation.Observation
	catalogObservation.PrerequisiteID, catalogObservation.Kind, catalogObservation.ContentHash = catalog.PrerequisiteID, PrerequisiteKindOperationAdmissibility, catalog.SnapshotID
	catalogObservation.ObservationID = StableRuntimePrerequisiteObservationID(catalogObservation)
	proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi-1", OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"resource-a"}},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, DeclarationRef: "definition/dodItem/dod-1"}.Normalized()
	binding := RuntimeImplementationWorkingBinding{SemanticManifestHash: "semantic-1", LogicalRequestID: "semantic-request-1", BindingGeneration: 1,
		Proposals: []RuntimeSemanticProofProposal{proposal}, OwnerCatalog: catalog, CatalogObservation: catalogObservation}
	binding.SemanticGenerationID = StableRuntimeSemanticGenerationID(binding.SemanticManifestHash, binding.LogicalRequestID)
	binding.Feasibilities = []RuntimeSemanticProofFeasibility{{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: binding.SemanticManifestHash, DefinitionSeedHash: seed.Hash,
		CoverageWorkItemID: "cwi-1", ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: RuntimeModuleFile,
		CatalogSnapshotID: catalog.SnapshotID, CatalogObservationID: catalogObservation.ObservationID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "tree-1", TargetRefs: []string{"resource-a"}}}
	binding.Hash = StableRuntimeImplementationWorkingBindingHash(binding)
	r.WorkingBinding = &binding
	r.Hash = StableRuntimeImplementationWorkRoundHash(r)
	ledger := RuntimeImplementationWorkLedger{Rounds: []RuntimeImplementationWorkRound{r}}
	if err := ledger.ValidateForSources(sources); err != nil {
		t.Fatal(err)
	}
	return ledger, sources
}

func rehashImplementationWorkRound(r *RuntimeImplementationWorkRound) {
	if r.WorkingBinding != nil {
		r.WorkingBinding.Hash = StableRuntimeImplementationWorkingBindingHash(*r.WorkingBinding)
	}
	r.Hash = StableRuntimeImplementationWorkRoundHash(*r)
}

func TestImplementationWorkLedgerRawCloneHashAndSameVersionReload(t *testing.T) {
	ledger, sources := implementationWorkLedgerFixture(t)
	before, _ := json.Marshal(ledger)
	clone := ledger.Clone()
	if !reflect.DeepEqual(ledger, clone) {
		t.Fatal("clone changed raw ledger")
	}
	clone.Rounds[0].WorkingBinding.Proposals[0].Target.OwnerScopeTargetRefs[0] = "changed"
	clone.Rounds[0].WorkingBinding.Feasibilities[0].TargetRefs[0] = "changed"
	clone.Rounds[0].WorkingBinding.OwnerCatalog.TargetRefs[0] = "changed"
	clone.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0].Revision = "changed"
	clone.Rounds[0].ScopeObservation.Selection.TargetRefs[0] = "changed"
	clone.Rounds[0].ScopeRequest.Policy.TargetSelectorCandidateRefs[0] = "changed"
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("nested work clone aliased original")
	}
	var loaded RuntimeImplementationWorkLedger
	if err := json.Unmarshal(before, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger, loaded) || loaded.ValidateForSources(sources) != nil {
		t.Fatal("same-version JSON reload changed identity")
	}
	invalid := ledger.Clone()
	invalid.Rounds[0].WorkingBinding.Proposals[0].Target.OwnerScopeTargetRefs = []string{"", "resource-a", "resource-a"}
	invalid.Rounds[0].ScopeObservation.Selection.TargetRefs = []string{}
	if !reflect.DeepEqual(invalid, invalid.Clone()) || invalid.ValidateForSources(sources) == nil {
		t.Fatal("clone normalized invalid/empty source into acceptable authority")
	}
	if StableRuntimeImplementationWorkRoundHash(invalid.Rounds[0]) == ledger.Rounds[0].Hash {
		t.Fatal("raw hash hid noncanonical preimage")
	}
}

func TestImplementationWorkLedgerRejectsCoordinatedTamperAndTerminalProof(t *testing.T) {
	ledger, sources := implementationWorkLedgerFixture(t)
	cases := map[string]func(*RuntimeImplementationWorkRound){
		"startup":               func(r *RuntimeImplementationWorkRound) { r.StartupInputHash = "other" },
		"cross workspace":       func(r *RuntimeImplementationWorkRound) { r.WorkspaceID = "other" },
		"cross run":             func(r *RuntimeImplementationWorkRound) { r.RunID = "other" },
		"cross definition":      func(r *RuntimeImplementationWorkRound) { r.DefinitionSeedHash = "other" },
		"round zero":            func(r *RuntimeImplementationWorkRound) { r.Ordinal = 0 },
		"round overflow":        func(r *RuntimeImplementationWorkRound) { r.Ordinal = 9 },
		"borrow preceding":      func(r *RuntimeImplementationWorkRound) { r.PrecedingReceiptSetHash = "other" },
		"binding without scope": func(r *RuntimeImplementationWorkRound) { r.ScopeObservation = nil },
		"dispatch without binding": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding = nil
			r.DispatchID = "dispatch"
			r.DispatchAuthorityHash = "seal"
		},
		"completion without dispatch": func(r *RuntimeImplementationWorkRound) { r.CompletedReceiptSetHash = "receipts" },
		"half dispatch":               func(r *RuntimeImplementationWorkRound) { r.DispatchID = "dispatch" },
		"unknown protocol":            func(r *RuntimeImplementationWorkRound) { r.ScopeRequest.Policy.ProtocolRef = "other" },
		"no fresh observation":        func(r *RuntimeImplementationWorkRound) { r.ScopeRequest.Policy.RequireFreshObservation = false },
		"wrong owner": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Proposals[0].OwnerModuleID = RuntimeModuleShell
		},
		"undeclared variant": func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.Proposals[0].OperationVariantID = "other" },
		"terminal validation": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Proposals[0].RequiredEvidenceKinds = []string{EvidenceKindValidation}
		},
		"terminal recipe": func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.Proposals[0].ValidationRecipeID = "recipe" },
		"terminal checks": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Proposals[0].RequiredCheckRefs = []string{"check"}
		},
		"unbounded scope": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Proposals[0].Target.OwnerScopeTargetRefs = nil
			r.WorkingBinding.Proposals[0].Target.OwnerSelectorRefs = []string{"work-scope"}
		},
		"duplicate proposal": func(r *RuntimeImplementationWorkRound) {
			b := r.WorkingBinding
			b.Proposals = append(b.Proposals, b.Proposals[0])
			b.Feasibilities = append(b.Feasibilities, b.Feasibilities[0])
		},
		"partial feasibilities":     func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.Feasibilities = nil },
		"wrong catalog snapshot":    func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.Feasibilities[0].CatalogSnapshotID = "other" },
		"wrong semantic generation": func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.SemanticGenerationID = "other" },
		"stale owner feasibility":   func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.Feasibilities[0].ObservedRevision = "other" },
		"rejected owner feasibility": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Feasibilities[0].Status = RuntimeSemanticFeasibilityInfeasible
		},
		"target differs": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.Feasibilities[0].TargetRefs = []string{"resource-b"}
		},
		"missing tree dependency": func(r *RuntimeImplementationWorkRound) {
			r.WorkingBinding.CatalogObservation.Dependencies = nil
			r.WorkingBinding.CatalogObservation.ObservationID = StableRuntimePrerequisiteObservationID(r.WorkingBinding.CatalogObservation)
		},
		"invented exclusions": func(r *RuntimeImplementationWorkRound) { r.WorkingBinding.ExcludedTargetRefs = []string{"unrelated"} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			invalid := ledger.Clone()
			change(&invalid.Rounds[0])
			rehashImplementationWorkRound(&invalid.Rounds[0])
			if invalid.ValidateForSources(sources) == nil {
				t.Fatal("self-rehashed invalid work ledger accepted")
			}
		})
	}
}

func implementationWorkReceiptFixture(t *testing.T, ledger *RuntimeImplementationWorkLedger) RuntimeImplementationWorkReceiptSet {
	t.Helper()
	r := &ledger.Rounds[len(ledger.Rounds)-1]
	r.DispatchID, r.DispatchAuthorityHash = "work-dispatch", "work-dispatch-seal"
	rehashImplementationWorkRound(r)
	content := "synthetic durable implementation"
	op := RuntimeOperation{ID: "op-1", Kind: StepKindFileWrite, TargetRefs: []string{"resource-a"}, Parameters: []OperationParameter{{Name: "content", String: &content}}}.Normalized()
	execution := RuntimeOperationExecutionRecord{OperationID: op.ID, OperationHash: StableRuntimeOperationHash(op), ExecutionKey: StableRuntimeOperationExecutionKey(r.WorkspaceID, r.RunID, r.DispatchID, op.ID),
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		AdmissibilitySnapshotID: r.WorkingBinding.OwnerCatalog.SnapshotID, AcquisitionBinding: r.WorkingBinding.OwnerCatalog.AcquisitionBinding,
		State: OperationExecutionEvidenceCommitted, Fence: 1, ClaimedAt: RuntimeOperationClaimMarkerInitial, OwnerReceiptRef: "owner-receipt-1", EvidenceFactID: "effect-evidence", EvidenceFactHash: "effect-evidence-hash"}
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(execution)
	receipt := RuntimeOperationReceipt{OperationID: op.ID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, Outcome: RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"resource-a"}, BeforeHash: "before", AfterHash: "after", Evidence: RuntimeOperationEvidence{Quality: common.EvidenceQualityValid}}.Normalized()
	dependency := r.WorkingBinding.CatalogObservation.Dependencies[0]
	receipt.ResourceChanges = []RuntimeResourceChange{{OwnerModuleID: dependency.OwnerModuleID, Domain: dependency.Domain, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "fresh-tree"}}
	evidence := EvidenceFact{ID: execution.EvidenceFactID, Kind: EvidenceKindStateChange, DispatchID: r.DispatchID, OperationID: op.ID, SourceRef: execution.OwnerReceiptRef, TargetRefs: []string{"resource-a"}, Quality: common.EvidenceQualityValid}
	execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(evidence)
	execution.OwnerReceiptHash = StableRuntimeOperationReceiptHash(receipt)
	set := RuntimeImplementationWorkReceiptSet{DispatchID: r.DispatchID, Members: []RuntimeImplementationWorkReceiptMember{{OwnerModuleID: RuntimeModuleFile, DispatchID: r.DispatchID, Operation: op, Execution: execution, Receipt: receipt, Evidence: evidence}}}
	hash, err := ValidateRuntimeImplementationWorkReceiptSet(*r, set.Members)
	if err != nil {
		t.Fatal(err)
	}
	r.CompletedReceiptSetHash = hash
	rehashImplementationWorkRound(r)
	return set
}

func TestImplementationWorkReceiptDispositionUsesOriginalOwnerFact(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	round := ledger.Rounds[0]
	receipt := RuntimeOperationReceipt{Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectWorkspaceDurable, Changed: true, BeforeHash: "before", AfterHash: "after", Evidence: RuntimeOperationEvidence{Quality: common.EvidenceQualityValid}}
	dependency := round.WorkingBinding.CatalogObservation.Dependencies[0]
	receipt.ResourceChanges = []RuntimeResourceChange{{Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "fresh-tree"}}
	if got := RuntimeImplementationWorkDispositionForReceipt(round, receipt); got != "" {
		t.Fatalf("durable intersecting owner fact rejected: %s", got)
	}
	cases := []struct {
		name   string
		change func(*RuntimeOperationReceipt)
		want   RuntimeImplementationWorkReceiptDisposition
	}{
		{"undeclared quality", func(r *RuntimeOperationReceipt) { r.Evidence.Quality = "" }, ""},
		{"invalid quality", func(r *RuntimeOperationReceipt) { r.Evidence.Quality = common.EvidenceQualityInvalid }, RuntimeImplementationWorkReceiptRejected},
		{"inconclusive quality", func(r *RuntimeOperationReceipt) { r.Evidence.Quality = common.EvidenceQualityInconclusive }, RuntimeImplementationWorkReceiptRejected},
		{"skipped quality", func(r *RuntimeOperationReceipt) { r.Evidence.Quality = common.EvidenceQualitySkipped }, RuntimeImplementationWorkReceiptRejected},
		{"unknown quality", func(r *RuntimeOperationReceipt) { r.Evidence.Quality = "undeclared-enum" }, RuntimeImplementationWorkReceiptRejected},
		{"zero work", func(r *RuntimeOperationReceipt) { r.Evidence.ZeroWork = true }, RuntimeImplementationWorkReceiptRejected},
		{"missing change", func(r *RuntimeOperationReceipt) { r.ResourceChanges = nil }, RuntimeImplementationWorkReceiptInvalidationMissing},
		{"foreign revision", func(r *RuntimeOperationReceipt) { r.ResourceChanges[0].BeforeRevision = "foreign" }, RuntimeImplementationWorkReceiptInvalidationMissing},
		{"foreign owner", func(r *RuntimeOperationReceipt) { r.ResourceChanges[0].OwnerModuleID = RuntimeModuleShell }, RuntimeImplementationWorkReceiptInvalidationMissing},
		{"no-op", func(r *RuntimeOperationReceipt) { r.Outcome = RuntimeOperationOutcomeNoOp }, RuntimeImplementationWorkReceiptRejected},
		{"weak effect", func(r *RuntimeOperationReceipt) { r.ObservedEffectScope = RuntimeEffectNone }, RuntimeImplementationWorkReceiptRejected},
		{"governance", func(r *RuntimeOperationReceipt) { r.Outcome = RuntimeOperationOutcomeGovernanceRequired }, RuntimeImplementationWorkReceiptGovernance},
		{"unknown", func(r *RuntimeOperationReceipt) { r.Outcome = RuntimeOperationOutcomeUnknown }, RuntimeImplementationWorkReceiptUnknown},
		{"stale", func(r *RuntimeOperationReceipt) { r.PrerequisiteStale = &RuntimePrerequisiteStale{} }, RuntimeImplementationWorkReceiptStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := receipt.Clone()
			tc.change(&original)
			before, _ := json.Marshal(original)
			if got := RuntimeImplementationWorkDispositionForReceipt(round, original); got != tc.want {
				t.Fatalf("disposition = %s, want %s", got, tc.want)
			}
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatal("disposition rewrote original owner fact")
			}
		})
	}
}

func TestImplementationWorkCollectiveReceiptsRejectNoOpPartialStaleAndCrossSource(t *testing.T) {
	ledger, sources := implementationWorkLedgerFixture(t)
	set := implementationWorkReceiptFixture(t, &ledger)
	sources.ReceiptSets = []RuntimeImplementationWorkReceiptSet{set}
	if err := ledger.ValidateForSources(sources); err != nil {
		t.Fatal(err)
	}
	undeclared := set.Members[0]
	undeclared.Receipt = cloneRuntimeOperationReceiptRaw(undeclared.Receipt)
	undeclared.Receipt.Evidence.Quality, undeclared.Evidence.Quality = "", ""
	undeclared.Execution.OwnerReceiptHash = StableRuntimeOperationReceiptHash(undeclared.Receipt)
	undeclared.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(undeclared.Evidence)
	round := ledger.Rounds[0].Clone()
	round.CompletedReceiptSetHash = ""
	round.Hash = StableRuntimeImplementationWorkRoundHash(round)
	before, _ := json.Marshal(undeclared)
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, []RuntimeImplementationWorkReceiptMember{undeclared}); err != nil {
		t.Fatalf("original undeclared owner quality rejected: %v", err)
	}
	after, _ := json.Marshal(undeclared)
	if string(before) != string(after) {
		t.Fatal("quality predicate rewrote original effect")
	}
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(ledger.Rounds[0], nil); err == nil {
		t.Fatal("empty/partial receipts completed round")
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkReceiptMember){
		"terminal credit": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Evidence.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
			m.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(m.Evidence)
		},
		"terminal evidence": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Evidence.Kind = EvidenceKindValidation
			m.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(m.Evidence)
		},
		"unknown owner quality": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Receipt.Evidence.Quality = "undeclared-enum"
			m.Evidence.Quality = m.Receipt.Evidence.Quality
			m.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(m.Evidence)
		},
		"quality rewritten": func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.Evidence.Quality = "" },
		"skipped owner quality": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Receipt.Evidence.Quality = common.EvidenceQualitySkipped
		},
		"invalid owner quality": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Receipt.Evidence.Quality = common.EvidenceQualityInvalid
		},
		"weak owner evidence": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Receipt.Evidence.Quality = common.EvidenceQualityInconclusive
		},
		"changed original declaration": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Evidence.ContentHash = "invented"
			m.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(m.Evidence)
		},
		"missing resource chain": func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.ResourceChanges = nil },
		"wrong owner":            func(m *RuntimeImplementationWorkReceiptMember) { m.OwnerModuleID = RuntimeModuleShell },
		"owner whitespace":       func(m *RuntimeImplementationWorkReceiptMember) { m.OwnerModuleID = " " + RuntimeModuleFile },
		"wrong key":              func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.ExecutionKey = "other" },
		"wrong fence":            func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.Fence++ },
		"wrong operation":        func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.OperationID = "other" },
		"wrong targets":          func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.TargetRefs = []string{"resource-b"} },
		"cross dispatch": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Execution.ExecutionKey = StableRuntimeOperationExecutionKey("ws-1", "run-1", "other", "op-1")
			m.Receipt.ExecutionKey = m.Execution.ExecutionKey
		},
		"partial promotion": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Execution.State = OperationExecutionOwnerCommitted
			m.Execution.EvidenceFactID = ""
			m.Execution.EvidenceFactHash = ""
		},
		"terminal binding": func(m *RuntimeImplementationWorkReceiptMember) { m.Operation.BindingRefs = []string{"terminal-proof"} },
		"no op": func(m *RuntimeImplementationWorkReceiptMember) {
			m.Receipt.Outcome = RuntimeOperationOutcomeNoOp
			m.Receipt.Changed = false
			m.Receipt.ObservedEffectScope = RuntimeEffectNone
		},
		"unknown":              func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.Outcome = RuntimeOperationOutcomeUnknown },
		"zero work":            func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.Evidence.ZeroWork = true },
		"no changed postimage": func(m *RuntimeImplementationWorkReceiptMember) { m.Receipt.AfterHash = m.Receipt.BeforeHash },
		"wrong catalog":        func(m *RuntimeImplementationWorkReceiptMember) { m.Execution.AdmissibilitySnapshotID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			m := set.Members[0]
			m.Operation = cloneRuntimeOperationRaw(m.Operation)
			m.Receipt = cloneRuntimeOperationReceiptRaw(m.Receipt)
			mutate(&m)
			switch name {
			case "unknown owner quality", "skipped owner quality", "invalid owner quality", "weak owner evidence":
				// receipt/effect 来源保持同步；唯一变化是原 quality，而非 hash 或投影漂移。
				m.Evidence.Quality = m.Receipt.Evidence.Quality
				m.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(m.Evidence)
				if runtimeImplementationWorkEffectQualityAcceptable(m.Receipt.Evidence.Quality) {
					t.Fatal("weak/unknown quality predicate accepted")
				}
			}
			m.Execution.OperationHash = StableRuntimeOperationHash(m.Operation)
			m.Execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(m.Execution)
			m.Execution.OwnerReceiptHash = StableRuntimeOperationReceiptHash(m.Receipt)
			if _, err := ValidateRuntimeImplementationWorkReceiptSet(ledger.Rounds[0], []RuntimeImplementationWorkReceiptMember{m}); err == nil {
				t.Fatal("self-rehashed mismatched receipt accepted")
			}
		})
	}
	missing := sources
	missing.ReceiptSets = nil
	if ledger.ValidateForSources(missing) == nil {
		t.Fatal("self-labeled completed round accepted without original receipt source")
	}
}

func TestImplementationWorkSameCWIRoundRequiresDurableReceiptAndFreshScope(t *testing.T) {
	ledger, sources := implementationWorkLedgerFixture(t)
	set := implementationWorkReceiptFixture(t, &ledger)
	sources.ReceiptSets = []RuntimeImplementationWorkReceiptSet{set}
	second := ledger.Rounds[0].Clone()
	second.Ordinal, second.PrecedingReceiptSetHash = 2, ledger.Rounds[0].CompletedReceiptSetHash
	second.DispatchID, second.DispatchAuthorityHash, second.CompletedReceiptSetHash = "", "", ""
	second.ScopeRequest.AuthoritySnapshotID = "authority-2"
	second.ScopeRequest.ManifestBinding.Manifest.Revision = "tree-2"
	second.ScopeRequest.ManifestBinding.Observation.ContentHash = "tree-2"
	second.ScopeRequest.ManifestBinding.Observation.Dependencies[0].Revision = "tree-2"
	second.ScopeRequest.ManifestBinding.Observation.ObservationID = StableRuntimePrerequisiteObservationID(second.ScopeRequest.ManifestBinding.Observation)
	second.ScopeRequest.RequestID = implementationWorkScopeRequestID(second.ScopeRequest)
	second.ScopeObservation.Request = second.ScopeRequest.Clone()
	second.ScopeObservation.Selection.PrerequisiteID, second.ScopeObservation.Selection.ObservedRevision = second.ScopeRequest.RequestID, "tree-2"
	second.ScopeObservation.Selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(second.ScopeObservation.Selection)
	second.ScopeObservation.Observation.PrerequisiteID = second.ScopeRequest.RequestID
	second.ScopeObservation.Observation.ContentHash = second.ScopeObservation.Selection.SnapshotID
	second.ScopeObservation.Observation.Dependencies[0].Revision = "tree-2"
	second.ScopeObservation.Observation.ObservationID = StableRuntimePrerequisiteObservationID(second.ScopeObservation.Observation)
	second.WorkingBinding = nil
	rehashImplementationWorkRound(&second)
	ledger.Rounds = append(ledger.Rounds, second)
	if err := ledger.ValidateForSources(sources); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkLedger){
		"no predecessor completion": func(l *RuntimeImplementationWorkLedger) { l.Rounds[0].CompletedReceiptSetHash = "" },
		"wrong preceding receipts":  func(l *RuntimeImplementationWorkLedger) { l.Rounds[1].PrecedingReceiptSetHash = "other" },
		"skip ordinal":              func(l *RuntimeImplementationWorkLedger) { l.Rounds[1].Ordinal = 3 },
		"reuse scope": func(l *RuntimeImplementationWorkLedger) {
			l.Rounds[1].ScopeRequest = l.Rounds[0].ScopeRequest.Clone()
			o := l.Rounds[0].ScopeObservation.Clone()
			l.Rounds[1].ScopeObservation = &o
		},
		"duplicate round":     func(l *RuntimeImplementationWorkLedger) { l.Rounds = append(l.Rounds, l.Rounds[1].Clone()) },
		"exhausted round cap": func(l *RuntimeImplementationWorkLedger) { l.Rounds[1].Ordinal = 9 },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := ledger.Clone()
			mutate(&invalid)
			for i := range invalid.Rounds {
				rehashImplementationWorkRound(&invalid.Rounds[i])
			}
			if invalid.ValidateForSources(sources) == nil {
				t.Fatal("invalid successor accepted")
			}
		})
	}
	if sources.Seed.CoverageItems[0].TargetText != "original criterion" || len(sources.Focuses[0].RequiredEvidenceKinds) != 1 || sources.Focuses[0].RequiredEvidenceKinds[0] != EvidenceKindValidation {
		t.Fatal("working receipt changed original terminal validation obligation")
	}
}

func TestImplementationWorkLedgerReservationsDoNotOpenProductionAvailability(t *testing.T) {
	ledger, sources := implementationWorkLedgerFixture(t)
	if ledger.ValidateForSources(sources) != nil {
		t.Fatal("value fixture invalid")
	}
	if err := ValidateRuntimeImplementationWorkAvailability(sources.Profiles); !errors.Is(err, ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("value contract incorrectly opened execution availability: %v", err)
	}
	if report := ExplainRuntimeImplementationWorkAvailability(sources.Profiles); report.Available || len(report.Blockers) != 1 {
		t.Fatalf("availability lost declaration blocker: %+v", report)
	}
	before, _ := json.Marshal(sources)
	if ledger.ValidateForSources(sources) != nil {
		t.Fatal("repeated pure validation failed")
	}
	after, _ := json.Marshal(sources)
	if string(before) != string(after) {
		t.Fatal("work contract mutated original Definition/terminal input")
	}
}
