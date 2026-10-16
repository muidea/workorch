package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// The protocol is owner-neutral. Concrete snapshot grammar and workspace/index
// observations belong to the capability adapter, never the Runtime reducer.
const RuntimeTerminalOwnerSnapshotProtocol = "runtime/owner-terminal-snapshot/v1"

type RuntimeOwnerSnapshotIdentity struct {
	ProtocolRef, SnapshotID, RootSnapshotID string
}

func (s RuntimeOwnerSnapshotIdentity) Validate() error {
	if s.ProtocolRef != RuntimeTerminalOwnerSnapshotProtocol {
		return fmt.Errorf("unsupported immutable owner snapshot protocol")
	}
	for _, id := range []string{s.SnapshotID, s.RootSnapshotID} {
		if id == "" || strings.TrimSpace(id) != id || len(id) > 2048 {
			return fmt.Errorf("immutable owner snapshot requires exact bounded identity")
		}
	}
	return nil
}

type RuntimeTerminalSnapshotReceiptSource struct {
	DispatchID, OperationID, ReceiptHash, ReceiptRef, ExecutionKey, EvidenceRef string
	Fence                                                                       uint64
}

type RuntimeTerminalSnapshotSource struct {
	Assertion        RuntimeLineageAssertion
	Snapshot         RuntimeTerminalSnapshotReceiptSource
	Validation       RuntimeTerminalSnapshotReceiptSource
	SnapshotAddress  RuntimeEvidenceContentAddress
	InputSubjectHash string
	SnapshotIdentity RuntimeOwnerSnapshotIdentity
}

type RuntimeTerminalSnapshotMembership struct {
	TargetRef, ContentHash, ObjectID string
}

// This is an observed material input, not SM-2 evidence or an execution grant.
// Currentness means one bounded observation before the first material CAS.
type RuntimeTerminalSnapshotProof struct {
	ProtocolRef                                    string
	Source                                         RuntimeTerminalSnapshotSource
	ObservedAtStateRevision                        uint64
	SnapshotID, RootSnapshotID, ObservedSnapshotID string
	WorkspaceClean                                 bool
	CurrentDependencies                            RuntimeOwnerRevisionSet
	InputMemberships                               []RuntimeTerminalSnapshotMembership
	ValidationMemberships                          []RuntimeTerminalSnapshotMembership
	ProofHash                                      string
}

func (p RuntimeTerminalSnapshotProof) CloneRaw() RuntimeTerminalSnapshotProof {
	p.CurrentDependencies = *CloneRuntimeOwnerRevisionSet(&p.CurrentDependencies)
	p.InputMemberships = slices.Clone(p.InputMemberships)
	p.ValidationMemberships = slices.Clone(p.ValidationMemberships)
	return p
}

func (p RuntimeTerminalSnapshotProof) WithHash() RuntimeTerminalSnapshotProof {
	p = p.CloneRaw()
	p.ProofHash = ""
	p.ProofHash = stableFormalValueHash("runtime-terminal-snapshot-proof/v1", p)
	return p
}

func (p RuntimeTerminalSnapshotProof) ValidateRaw() error {
	if p.ProtocolRef != RuntimeTerminalOwnerSnapshotProtocol || p.Source.Assertion.Rule.Kind != RuntimeLineageTerminalOwnerSnapshot || p.Source.Assertion.Rule.Validate() != nil || p.ObservedAtStateRevision == 0 || !p.WorkspaceClean || p.SnapshotID != p.ObservedSnapshotID || p.SnapshotID == "" || p.RootSnapshotID == "" || p.ProofHash == "" || p.WithHash().ProofHash != p.ProofHash {
		return fmt.Errorf("terminal snapshot requires complete unchanged clean owner observation")
	}
	if err := validateRuntimeOwnerRevisionSetRaw(&p.CurrentDependencies); err != nil {
		return err
	}
	if len(p.CurrentDependencies.Entries) < 2 || len(p.CurrentDependencies.Entries) > 1024 || len(p.InputMemberships) == 0 || len(p.InputMemberships) > 16 || len(p.ValidationMemberships) == 0 || len(p.ValidationMemberships) > 1024 {
		return fmt.Errorf("terminal snapshot exceeds complete observation bounds")
	}
	for _, value := range []string{p.SnapshotID, p.RootSnapshotID, p.Source.InputSubjectHash} {
		if strings.TrimSpace(value) != value || len(value) > 2048 {
			return fmt.Errorf("terminal snapshot identity is not bounded canonical")
		}
	}
	return nil
}

// CompileRuntimeTerminalSnapshotSources resolves unique original CURRENT
// accepted operations. It never chooses the most recent matching receipt or
// substitutes a historical consistent review. Callers validate the aggregate.
func CompileRuntimeTerminalSnapshotSources(source RuntimeStateRecord) ([]RuntimeTerminalSnapshotSource, error) {
	bindings, err := RuntimeDeliveryLineageBindingsFromState(source)
	if err != nil {
		return nil, err
	}
	outcome, terminal, err := DeriveRuntimeTerminalOutcome(source.Decision, source.Termination)
	if err != nil {
		return nil, err
	}
	if !terminal || outcome.Kind != TerminalOutcomeKindCompleted {
		return nil, nil
	}
	var result []RuntimeTerminalSnapshotSource
	for _, binding := range bindings {
		for _, assertion := range binding.Assertions {
			rule := assertion.Rule
			if rule.Kind != RuntimeLineageTerminalOwnerSnapshot {
				continue
			}
			snapshot, receipt, _, err := runtimeTerminalSnapshotOperationSource(source, rule.ReceiptSourceScope, rule.ReceiptSourceID, &rule)
			if err != nil {
				return nil, err
			}
			validation, _, input, err := runtimeTerminalSnapshotOperationSource(source, rule.ValidationSourceScope, rule.ValidationSourceID, nil)
			if err != nil {
				return nil, err
			}
			if receipt.Evidence.ContentAddress == nil || receipt.Evidence.OwnerSnapshot == nil || receipt.Evidence.OwnerSnapshot.Validate() != nil || input == nil || input.InputReview == nil {
				return nil, fmt.Errorf("terminal snapshot lacks original archived/reviewed input source")
			}
			result = append(result, RuntimeTerminalSnapshotSource{Assertion: assertion, Snapshot: snapshot, Validation: validation, SnapshotAddress: *receipt.Evidence.ContentAddress, InputSubjectHash: input.InputReview.Hash, SnapshotIdentity: *receipt.Evidence.OwnerSnapshot})
			if len(result) > MaxRuntimeLineageAssertions {
				return nil, fmt.Errorf("terminal snapshot source inventory exceeds bound")
			}
		}
	}
	return result, nil
}

func runtimeTerminalSnapshotOperationSource(source RuntimeStateRecord, scope, id string, rule *RuntimeLineageAssertionRule) (RuntimeTerminalSnapshotReceiptSource, RuntimeOperationReceipt, *RuntimeValidationProofRequirementBinding, error) {
	var item RuntimeCoverageItem
	count := 0
	for _, value := range source.Facts.DefinitionSeed.CoverageItems {
		if value.SourceScope == scope && value.SourceID == id {
			item, count = value, count+1
		}
	}
	if count != 1 {
		return RuntimeTerminalSnapshotReceiptSource{}, RuntimeOperationReceipt{}, nil, fmt.Errorf("terminal snapshot selector is missing or ambiguous")
	}
	var current CoverageEvidenceBinding
	count = 0
	for _, coverage := range source.Facts.Coverage {
		if coverage.ID != item.ID {
			continue
		}
		if binding, found := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage); found {
			current, count = binding, count+1
		}
	}
	if count != 1 {
		return RuntimeTerminalSnapshotReceiptSource{}, RuntimeOperationReceipt{}, nil, fmt.Errorf("terminal snapshot requires current original proof binding")
	}
	var selected RuntimeTerminalSnapshotReceiptSource
	var selectedReceipt RuntimeOperationReceipt
	var selectedInput *RuntimeValidationProofRequirementBinding
	count = 0
	sources := runtimeTerminalProofSources(source)
	for _, dispatch := range source.CompletedDispatches {
		if dispatch.State != RuntimeDispatchStateCommitted || dispatch.ImplementationWorkCommitment != nil {
			continue
		}
		for _, op := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
			if len(op.BindingRefs) != 1 || op.BindingRefs[0] != current.ID {
				continue
			}
			fact, receipt, valid := runtimeLineageOperationFact(dispatch, op, sources, true)
			if !valid || !runtimeTerminalEvidenceMatchesCurrentProof(source, fact, current) || receipt.Evidence.OwnerRevisionSet == nil {
				continue
			}
			if rule != nil {
				contract, found := RuntimeOperationContractForKind(op.Kind)
				variant, err := MatchRuntimeOperationVariant(op, contract)
				if !found || err != nil || op.Kind != rule.OperationKind || variant.ID != rule.OperationVariantID || receipt.ObservedEffectScope != rule.RequiredEffectScope {
					continue
				}
			} else if fact.Kind != EvidenceKindValidation {
				continue
			}
			var execution RuntimeOperationExecutionRecord
			for _, value := range dispatch.OperationExecutions {
				if value.OperationID == op.ID {
					execution = value
				}
			}
			if execution.State != OperationExecutionEvidenceCommitted {
				continue
			}
			selected = RuntimeTerminalSnapshotReceiptSource{DispatchID: dispatch.ID, OperationID: op.ID, ReceiptHash: execution.OwnerReceiptHash, ReceiptRef: fact.SourceRef, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, EvidenceRef: string(fact.ID)}
			selectedReceipt, count = receipt, count+1
			if rule == nil {
				input, err := CompileRuntimeBindingValidationInputReview(current, source.SemanticPlanning)
				if err != nil {
					return selected, receipt, nil, err
				}
				descriptor, _ := RuntimeOperationDescriptorForKind(op.Kind)
				if err := ValidateRuntimeReviewedInputReads(input, *receipt.Evidence.OwnerRevisionSet, descriptor.OwnerModuleID+"/"+string(source.Facts.WorkspaceID)); err != nil {
					return selected, receipt, nil, err
				}
				selectedInput = &input
			}
		}
	}
	if count != 1 {
		return selected, selectedReceipt, nil, fmt.Errorf("terminal snapshot %s/%s requires unique accepted current source operation, found %d", scope, id, count)
	}
	return selected, selectedReceipt, selectedInput, nil
}

// ValidateRuntimeReviewedInputReads joins owner-neutral exact byte identities;
// it deliberately does not interpret a path, language or native check grammar.
func ValidateRuntimeReviewedInputReads(input RuntimeValidationProofRequirementBinding, dependencies RuntimeOwnerRevisionSet, ownerInstance string) error {
	if input.InputReview == nil || input.DefinitionRevision != dependencies.DefinitionRevision || input.Validate() != nil || validateRuntimeOwnerRevisionSetRaw(&dependencies) != nil {
		return fmt.Errorf("reviewed input requires exact original read dependencies")
	}
	for _, file := range input.InputReview.Files {
		matches := 0
		for _, read := range dependencies.Entries {
			if read.OwnerKind == "capability" && read.OwnerInstanceID == ownerInstance && read.ResourceKind == string(RuntimeResourceWorkspaceTarget) && read.ResourceID == file.TargetRef && read.Revision == file.ContentHash {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("reviewed input bytes are absent or ambiguous in original owner reads")
		}
	}
	return nil
}

// ApplyRuntimeTerminalSnapshotProofs is shared by first CAS and fresh-load.
// Material remains independently generated; callers cannot submit a Result body.
func ApplyRuntimeTerminalSnapshotProofs(source RuntimeStateRecord, material RuntimeTerminalResultMaterial, proofs []RuntimeTerminalSnapshotProof) (RuntimeTerminalResultMaterial, error) {
	expected, err := CompileRuntimeTerminalSnapshotSources(source)
	if err != nil {
		return RuntimeTerminalResultMaterial{}, err
	}
	if len(proofs) != len(expected) || (len(expected) == 0 && proofs != nil) {
		return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot proof inventory differs from sealed obligations")
	}
	if len(proofs) == 0 {
		return material, nil
	}
	if material.RuntimeCloseout == nil || source.StateRevision < 2 {
		return RuntimeTerminalResultMaterial{}, common.ErrConflict
	}
	for i, proof := range proofs {
		if err := proof.ValidateRaw(); err != nil {
			return RuntimeTerminalResultMaterial{}, err
		}
		if proof.Source != expected[i] || proof.ObservedAtStateRevision != source.StateRevision-1 {
			return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot observation source/revision drift")
		}
		if proof.SnapshotID != proof.Source.SnapshotIdentity.SnapshotID || proof.RootSnapshotID != proof.Source.SnapshotIdentity.RootSnapshotID {
			return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot identity differs from original owner receipt")
		}
		receipt, found := RuntimeOperationReceiptForExecutionKeyFence(source.OperationReceipts, proof.Source.Snapshot.ExecutionKey, proof.Source.Snapshot.Fence)
		if !found || !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &proof.CurrentDependencies) {
			return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot current objects differ from original receipt")
		}
		validation, found := RuntimeOperationReceiptForExecutionKeyFence(source.OperationReceipts, proof.Source.Validation.ExecutionKey, proof.Source.Validation.Fence)
		if !found || validation.Evidence.OwnerRevisionSet == nil || len(proof.ValidationMemberships) != len(validation.Evidence.OwnerRevisionSet.Entries) {
			return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot validation read set is incomplete")
		}
		for j, entry := range validation.Evidence.OwnerRevisionSet.Entries {
			member := proof.ValidationMemberships[j]
			count := 0
			for _, object := range proof.CurrentDependencies.Entries {
				if object.ResourceID == member.ObjectID && object.Revision == entry.Revision {
					count++
				}
			}
			if entry.ResourceKind != string(RuntimeResourceWorkspaceTarget) || member.TargetRef != entry.ResourceID || member.ContentHash != entry.Revision || count != 1 {
				return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot does not contain original validation byte dependency")
			}
		}
		_, _, input, err := runtimeTerminalSnapshotOperationSource(source, proof.Source.Assertion.Rule.ValidationSourceScope, proof.Source.Assertion.Rule.ValidationSourceID, nil)
		if err != nil || input == nil || len(proof.InputMemberships) != len(input.InputReview.Files) {
			return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot input membership incomplete")
		}
		for j, file := range input.InputReview.Files {
			member := proof.InputMemberships[j]
			matches := 0
			for _, entry := range proof.CurrentDependencies.Entries {
				if entry.ResourceID == member.ObjectID && entry.Revision == file.ContentHash {
					matches++
				}
			}
			if member.TargetRef != file.TargetRef || member.ContentHash != file.ContentHash || matches != 1 {
				return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot input differs from original reviewed bytes")
			}
		}
		for _, ref := range []string{proof.Source.Snapshot.EvidenceRef, proof.Source.Snapshot.ReceiptRef, proof.Source.Validation.EvidenceRef, proof.Source.Validation.ReceiptRef} {
			if !slices.Contains(material.EvidenceRefs, ref) {
				return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal snapshot sources are absent from formal Result index")
			}
		}
	}
	material = material.CloneRaw()
	material.RuntimeCloseout.TerminalSnapshotProofs = cloneRuntimeStateRawSlice(proofs, func(p RuntimeTerminalSnapshotProof) RuntimeTerminalSnapshotProof { return p.CloneRaw() })
	body := material.RuntimeCloseout.WithCloseoutHash()
	material.RuntimeCloseout = &body
	return material, nil
}
