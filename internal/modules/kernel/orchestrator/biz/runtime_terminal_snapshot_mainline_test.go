package orchestrator

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeReviewedGitSnapshotBootstrap(bootstrap runtimecontract.RuntimeBootstrap, additionalConsumer bool) runtimecontract.RuntimeBootstrap {
	git := runtimeFileShellGitBootstrap()
	declaration := git.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[2]
	declaration.Scope, declaration.TargetRefs = "dodItem", []string{"workspace-repository"}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = append(bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements, declaration)
	focus := git.DefinitionFocuses[2]
	focus.EvidenceDeclarationRef = ""
	bootstrap.DefinitionFocuses = append(bootstrap.DefinitionFocuses, focus)
	bootstrap.CoverageItems = append(bootstrap.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: "cwi-git", SliceID: "slice-mainline", SourceScope: "dodItem", SourceID: "cwi-git", TargetText: "commit the complete validated delivery and reviewed inputs", Order: 3, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}})
	bootstrap.Slices[0].CoverageWorkItemIDs = append(bootstrap.Slices[0].CoverageWorkItemIDs, "cwi-git")
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.StepKindGitWrite)
	for _, requirement := range git.Requirements {
		if requirement.OwnerModuleID == runtimecontract.RuntimeModuleGit {
			bootstrap.Requirements = append(bootstrap.Requirements, requirement)
		}
	}
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "original-final-owner-snapshot", Kind: runtimecontract.RuntimeLineageTerminalOwnerSnapshot, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: runtimecontract.RuntimeTerminalSnapshotProofContract, OwnerModuleID: runtimecontract.RuntimeModuleGit, OperationKind: runtimecontract.StepKindGitWrite, OperationVariantID: "commit", RequiredEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, ReceiptSourceScope: "dodItem", ReceiptSourceID: "cwi-git", ValidationSourceScope: "dodItem", ValidationSourceID: "cwi-2"}
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{rule}
	bootstrap.DefinitionFocuses[1].LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{rule}
	if additionalConsumer {
		// Two exact CWI obligations may share the same original source pair.
		// Each must retain its own assertion/proof identity; one proof cannot
		// stand in for both, nor may the source operations be rerun.
		last := len(bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements) - 1
		bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[last].LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{rule}
		bootstrap.DefinitionFocuses[len(bootstrap.DefinitionFocuses)-1].LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{rule}
	}
	return bootstrap
}

func startRuntimeReviewedGitSnapshotOwner(t *testing.T, hub magicevent.Hub, root string) {
	t.Helper()
	receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	owner := gitmodule.New()
	if err := owner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Stop(context.Background()) })
}

func runtimeReviewedGitSnapshotPaths(t *testing.T, directory string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ref, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(ref))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func runtimeReviewedGitSnapshotSemanticDocument(document *runtimecontract.RuntimeSemanticPlanDocument, command llmevents.RequestRuntimeCandidateCommand) {
	for i := range document.Proposals {
		proposal := &document.Proposals[i]
		if proposal.CoverageWorkItemID != "cwi-git" {
			continue
		}
		proposal.OwnerModuleID, proposal.OperationKind, proposal.OperationVariantID = runtimecontract.RuntimeModuleGit, runtimecontract.StepKindGitWrite, "commit"
		proposal.Target = runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"workspace-repository"}}
	}
}

func TestRuntimeReviewedGitTerminalSnapshotOwnerMainlineAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, ValidationCount: 1, TerminalSnapshot: true, SnapshotInputSources: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	for _, cut := range []string{"before_material_seal", "after_material_seal", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatal("terminal snapshot recovery repeated provider or execution")
			}
		})
	}
}

func TestRuntimeReviewedGitTerminalSnapshotMultipleConsumerMainlineAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, ValidationCount: 1, TerminalSnapshot: true, SnapshotInputSources: true, AdditionalTerminalSnapshotConsumer: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	for _, cut := range []string{"before_material_seal", "after_material_seal", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatal("multiple terminal consumers repeated provider or source execution")
			}
		})
	}
}

func runtimeReviewedGitProjectExecutionCount(t *testing.T, source runtimecontract.RuntimeStateRecord) string {
	t.Helper()
	return runtimeReviewedProjectExecutionCount(t, source, 1, map[common.CoverageWorkItemID]int{"cwi-2": 1})
}

func runtimeReviewedProjectExecutionCount(t *testing.T, source runtimecontract.RuntimeStateRecord, expectedOrdinary int, expectedReconciliations map[common.CoverageWorkItemID]int) string {
	t.Helper()
	// The ordinary receipt and satisfied reconciliation are distinct owner
	// calls. The latter never becomes another validation receipt/credit.
	ordinary, reconciliation := 0, 0
	reconciled := make(map[common.CoverageWorkItemID]int)
	seen := make(map[string]bool)
	for _, receipt := range source.OperationReceipts {
		if receipt.Evidence.Validation != nil && receipt.Evidence.Validation.ExecutionConjunction != nil {
			if !receipt.Evidence.Validation.ExecutionConjunction.Complete {
				t.Fatal("original project conjunction incomplete")
			}
			ordinary++
		}
	}
	batches := []*runtimecontract.RuntimeCWIReconciliationBatch{source.Reconciliation.Batch}
	for _, history := range source.Reconciliation.History {
		batches = append(batches, history.Batch)
	}
	for _, batch := range batches {
		if batch == nil {
			continue
		}
		for _, observed := range batch.Observations {
			if observed.OwnerModuleID == runtimecontract.RuntimeModuleShell && observed.Status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
				if len(observed.CheckRefs) != 1 {
					t.Fatal("reconciliation omitted exact original check")
				}
				identity := runtimecontract.StableRuntimeCWIReconciliationObservationRef(observed)
				if seen[identity] {
					t.Fatal("duplicate original owner reconciliation")
				}
				seen[identity] = true
				reconciliation++
				reconciled[observed.CoverageWorkItemID]++
			}
		}
	}
	expectedTotal := 0
	for _, count := range expectedReconciliations {
		expectedTotal += count
	}
	if ordinary != expectedOrdinary || reconciliation != expectedTotal || len(source.Reconciliation.Reopened) != 0 {
		t.Fatalf("unexpected project execution provenance: ordinary=%d reconciliation=%d", ordinary, reconciliation)
	}
	for id, count := range reconciled {
		if count != expectedReconciliations[id] {
			t.Fatalf("original CWI reconciliation count: %s got %d want %d", id, count, expectedReconciliations[id])
		}
	}
	return strings.Repeat("executed\n", 2*(ordinary+reconciliation))
}

func assertRuntimeTerminalSnapshotProof(t *testing.T, source runtimecontract.RuntimeStateRecord, directory string) {
	t.Helper()
	if source.TerminalMaterialSeal == nil {
		t.Fatal("no original material seal")
	}
	proofs := source.TerminalMaterialSeal.Material.RuntimeCloseout.TerminalSnapshotProofs
	expectedConsumers := map[common.CoverageWorkItemID]bool{}
	for _, focus := range source.Facts.DefinitionFocuses {
		for _, rule := range focus.LineageAssertions {
			if rule.Kind == runtimecontract.RuntimeLineageTerminalOwnerSnapshot {
				for _, id := range focus.CoverageWorkItemIDs {
					expectedConsumers[id] = true
				}
			}
		}
	}
	if len(proofs) == 0 || len(proofs) != len(expectedConsumers) {
		t.Fatal("formal Result omitted actual final owner snapshot")
	}
	head := strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD"))
	root := strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD^{tree}"))
	for _, proof := range proofs {
		if !expectedConsumers[proof.Source.Assertion.CoverageWorkItemID] || proof.SnapshotID != head || proof.RootSnapshotID != root || !proof.WorkspaceClean {
			t.Fatal("terminal consumer omitted, duplicated or substituted actual snapshot")
		}
		expectedConsumers[proof.Source.Assertion.CoverageWorkItemID] = false
		for _, member := range proof.InputMemberships {
			if member.ObjectID != strings.TrimSpace(runtimeGitOutput(t, directory, "rev-parse", "HEAD:"+member.TargetRef)) {
				t.Fatal("terminal membership does not match independent original Git path")
			}
		}
	}
	// Independently select original physical receipts in this fixture (one
	// File patch, one Shell validation, one Git commit), not compiler evidence
	// projections, plus the original satisfied assessment's owner observations.
	// Versioned receipts keep historical inputs on the formal evidence item;
	// only their exact SM-2-observed current set belongs in the terminal union.
	// Init/add have no sampled reads and cannot invent entries.
	var originalReads []runtimecontract.RuntimeOwnerRevisionSet
	owners := map[string]bool{}
	for _, receipt := range source.OperationReceipts {
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Evidence.OwnerRevisionSet == nil {
			continue
		}
		reads := receipt.Evidence.OwnerRevisionSet
		if receipt.Evidence.ApplicabilityDependencies != nil {
			var fact *runtimecontract.EvidenceFact
			for index := range source.Facts.Evidence.Accepted {
				candidate := &source.Facts.Evidence.Accepted[index]
				if candidate.OperationID == receipt.OperationID && reflect.DeepEqual(candidate.OwnerRevisionSet, receipt.Evidence.OwnerRevisionSet) {
					if fact != nil {
						t.Fatal("fixture receipt has ambiguous evidence source")
					}
					fact = candidate
				}
			}
			if fact == nil {
				t.Fatal("versioned receipt omitted original accepted evidence")
			}
			reads = nil
			for _, observation := range source.Facts.EvidenceApplicabilityObservations {
				if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == runtimecontract.StableRuntimeEvidenceFactHash(*fact) && observation.Request.SourceReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) && observation.Request.EvidenceRevision == source.Facts.EvidenceRevision && observation.Request.ReconciliationRequestHash == source.Reconciliation.RequestHash && !observation.Missing {
					reads = observation.CurrentDependencies
				}
			}
			if reads == nil {
				t.Fatal("versioned receipt omitted exact terminal-generation current observation")
			}
			preserved := false
			for _, outcome := range source.TerminalMaterialSeal.Material.RuntimeCloseout.RequirementOutcomes {
				for _, item := range outcome.EvidenceItems {
					if item.EvidenceRef == string(fact.ID) && reflect.DeepEqual(item.OwnerRevisionSet, receipt.Evidence.OwnerRevisionSet) && reflect.DeepEqual(item.ApplicabilityDependencies, receipt.Evidence.ApplicabilityDependencies) {
						preserved = true
					}
				}
			}
			if !preserved {
				t.Fatal("formal evidence item discarded original historical/current dependency pair")
			}
		}
		originalReads = append(originalReads, *runtimecontract.CloneRuntimeOwnerRevisionSet(reads))
		for _, entry := range receipt.Evidence.OwnerRevisionSet.Entries {
			owners[entry.OwnerInstanceID] = true
		}
	}
	assessmentReads := 0
	for _, dispatch := range source.CompletedDispatches {
		adjudication := dispatch.AssessmentAdjudication
		if adjudication == nil {
			continue
		}
		if dispatch.State != runtimecontract.RuntimeDispatchStateCommitted || adjudication.Disposition != runtimecontract.AssessmentDispositionSatisfied {
			t.Fatal("fixture must have only original committed satisfied assessments")
		}
		for _, observed := range adjudication.OwnerFactSources {
			if observed.OwnerRevisionSet != nil {
				originalReads = append(originalReads, *runtimecontract.CloneRuntimeOwnerRevisionSet(observed.OwnerRevisionSet))
				assessmentReads++
			}
		}
	}
	wantReads, err := runtimecontract.CompileRuntimeOwnerRevisionSetUnion(source.Facts.DefinitionRevision, originalReads...)
	if err != nil || !reflect.DeepEqual(wantReads, source.TerminalMaterialSeal.Material.RuntimeCloseout.OwnerRevisionSet) || len(owners) != 3 || assessmentReads == 0 {
		t.Fatalf("formal material omitted actual File/Shell/Git read union: owners=%v err=%v want=%+v actual=%+v", owners, err, wantReads, source.TerminalMaterialSeal.Material.RuntimeCloseout.OwnerRevisionSet)
	}
	for _, entry := range source.TerminalMaterialSeal.Material.RuntimeCloseout.OwnerRevisionSet.Entries {
		if entry.OwnerKind != "capability" || entry.ResourceKind == "accepted_evidence" {
			t.Fatal("synthetic Runtime evidence reference substituted for actual owner read")
		}
	}
	bindings, err := runtimecontract.RuntimeDeliveryLineageBindingsFromState(source)
	if err != nil || len(bindings) != len(proofs) {
		t.Fatal("snapshot declaration source missing")
	}
	for _, binding := range bindings {
		state, err := runtimecontract.EvaluateRuntimeDeliveryLineageAssertionFromState(source, binding)
		if err != nil || state != runtimecontract.RuntimeDeliveryLineageSatisfied {
			t.Fatalf("snapshot consumer: %s %v", state, err)
		}
	}
	body, _ := json.Marshal(source)
	var reload runtimecontract.RuntimeStateRecord
	if json.Unmarshal(body, &reload) != nil || runtimeAggregateFromRecord(reload).Validate() != nil {
		t.Fatal("snapshot sources did not survive independent JSON reload")
	}
	for _, scenario := range []string{"omit", "dirty", "head", "coordinated-head", "root", "input", "source", "revision", "dependencies", "terminal-union"} {
		t.Run("terminal-snapshot-tamper-"+scenario, func(t *testing.T) {
			bad := runtimeAggregateFromRecord(source).CloneRaw()
			seal := bad.TerminalMaterialSeal
			proof := &seal.Material.RuntimeCloseout.TerminalSnapshotProofs[0]
			switch scenario {
			case "omit":
				seal.Material.RuntimeCloseout.TerminalSnapshotProofs = nil
			case "dirty":
				proof.WorkspaceClean = false
			case "head":
				proof.ObservedSnapshotID = "substituted"
			case "coordinated-head":
				proof.SnapshotID, proof.ObservedSnapshotID = proof.RootSnapshotID, proof.RootSnapshotID
			case "root":
				proof.RootSnapshotID = proof.SnapshotID
			case "input":
				proof.InputMemberships[0].ContentHash = strings.Repeat("0", 64)
			case "source":
				proof.Source.Validation.ReceiptHash = strings.Repeat("0", 64)
			case "revision":
				proof.ObservedAtStateRevision--
			case "dependencies":
				proof.CurrentDependencies.Entries = proof.CurrentDependencies.Entries[:1]
				proof.CurrentDependencies = proof.CurrentDependencies.WithHash()
			case "terminal-union":
				seal.Material.RuntimeCloseout.OwnerRevisionSet.Entries = seal.Material.RuntimeCloseout.OwnerRevisionSet.Entries[:1]
				seal.Material.RuntimeCloseout.OwnerRevisionSet = seal.Material.RuntimeCloseout.OwnerRevisionSet.WithHash()
				for i := range seal.Material.RuntimeCloseout.RequirementOutcomes {
					outcome := &seal.Material.RuntimeCloseout.RequirementOutcomes[i]
					outcome.OwnerRevisionSetHash = seal.Material.RuntimeCloseout.OwnerRevisionSet.SetHash
					for j := range outcome.EvidenceItems {
						outcome.EvidenceItems[j].OwnerRevisionSetHash = outcome.OwnerRevisionSetHash
					}
				}
			}
			for i := range seal.Material.RuntimeCloseout.TerminalSnapshotProofs {
				seal.Material.RuntimeCloseout.TerminalSnapshotProofs[i] = seal.Material.RuntimeCloseout.TerminalSnapshotProofs[i].WithHash()
			}
			closeout := seal.Material.RuntimeCloseout.WithCloseoutHash()
			seal.Material.RuntimeCloseout = &closeout
			seal.MaterialHash, _ = seal.Material.CanonicalHash()
			for i := range bad.DeliveryAcknowledgements {
				if bad.DeliveryAcknowledgements[i].ResultCloseout != nil {
					value := closeout.CloneRaw()
					bad.DeliveryAcknowledgements[i].ResultCloseout = &value
				}
			}
			if err := bad.Validate(); err == nil {
				t.Fatal("coordinated material/hash/ACK mutation bypassed original snapshot source")
			}
		})
	}
	clone := runtimeAggregateFromRecord(source).CloneRaw()
	clone.TerminalMaterialSeal.Material.RuntimeCloseout.TerminalSnapshotProofs[0].InputMemberships[0].TargetRef = "caller"
	if reflect.DeepEqual(clone.TerminalMaterialSeal, source.TerminalMaterialSeal) || source.TerminalMaterialSeal.Material.RuntimeCloseout.TerminalSnapshotProofs[0].InputMemberships[0].TargetRef == "caller" {
		t.Fatal("snapshot clone aliases original owner material")
	}
}
