package file

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 本次 patch 执行读取的输入/不存在事实或 recovery 确认读取的后像；不是 receipt.BeforeHash。
type filePatchReadObservation struct {
	TargetRef   string
	ContentHash string
	Size        int64
	Purpose     string
}

func observedFilePatchRead(target string, content []byte, purpose string) *filePatchReadObservation {
	return &filePatchReadObservation{TargetRef: target, ContentHash: sha256Hex(content), Size: int64(len(content)), Purpose: purpose}
}

// Absence comes only from a successful readPatchTarget not-found observation.
// It is an existence revision, never the content hash of an empty file.
func observedFilePatchTargetRead(target string, content []byte, exists bool) *filePatchReadObservation {
	if exists {
		return observedFilePatchRead(target, content, "patch_input")
	}
	return &filePatchReadObservation{TargetRef: target, ContentHash: sha256Hex([]byte("file-target-existence/v1\x00absent")), Purpose: "patch_absence"}
}

func filePatchOwnerDependencies(scope runtimecontract.RuntimeOperationExecutionContext, operation runtimecontract.RuntimeOperation, observed *filePatchReadObservation) (*runtimecontract.RuntimeOwnerRevisionSet, error) {
	if observed == nil {
		return nil, nil
	}
	definition := scope.Transaction.DefinitionRevision
	if definition == "" || definition != strings.TrimSpace(definition) || scope.WorkspaceID == "" || string(scope.WorkspaceID) != strings.TrimSpace(string(scope.WorkspaceID)) ||
		observed.TargetRef == "" || observed.TargetRef != strings.TrimSpace(observed.TargetRef) || observed.ContentHash == "" || observed.ContentHash != strings.TrimSpace(observed.ContentHash) || observed.Size < 0 ||
		(observed.Purpose != "patch_input" && observed.Purpose != "replay_postcondition" && observed.Purpose != "patch_absence") {
		return nil, fmt.Errorf("file patch dependencies require exact definition, workspace and actual read observation")
	}
	if len(operation.TargetRefs) != 1 || operation.TargetRefs[0] != observed.TargetRef {
		return nil, fmt.Errorf("file patch read observation differs from the exact sealed operation target")
	}
	resourceKind := string(runtimecontract.RuntimeResourceWorkspaceTarget)
	if observed.Purpose == "patch_absence" {
		if observed.Size != 0 || observed.ContentHash != sha256Hex([]byte("file-target-existence/v1\x00absent")) {
			return nil, fmt.Errorf("file absence dependency requires exact observed existence identity")
		}
		resourceKind = "workspace_target_existence"
	}
	preimage := struct {
		WorkspaceID   string
		OperationHash string
		Observation   filePatchReadObservation
	}{string(scope.WorkspaceID), runtimecontract.StableRuntimeOperationHash(operation), *observed}
	encoded, err := json.Marshal(preimage)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte("file-patch-read-dependency/v1\x00"), encoded...))
	set := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: definition, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + string(scope.WorkspaceID), ResourceKind: resourceKind,
		ResourceID: observed.TargetRef, Revision: observed.ContentHash, DependencyHash: hex.EncodeToString(digest[:]),
	}}}).WithHash()
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return &set, nil
}

// The resource selector is independent of execution identity. The receipt
// already binds the exact operation; two observations of the same complete
// bytes therefore agree, while different versions remain conflicting.
func filePatchApplicabilityDependencies(scope runtimecontract.RuntimeOperationExecutionContext, operation runtimecontract.RuntimeOperation, observed *filePatchReadObservation) (*runtimecontract.RuntimeEvidenceApplicabilityDependencies, error) {
	if observed == nil {
		return nil, nil
	}
	if observed.Purpose != "mutation_postcondition" || observed.Size < 0 || len(observed.ContentHash) != 64 ||
		len(operation.TargetRefs) != 1 || operation.TargetRefs[0] != observed.TargetRef ||
		scope.WorkspaceID == "" || string(scope.WorkspaceID) != strings.TrimSpace(string(scope.WorkspaceID)) ||
		scope.Transaction.DefinitionRevision == "" || scope.Transaction.DefinitionRevision != strings.TrimSpace(scope.Transaction.DefinitionRevision) {
		return nil, fmt.Errorf("file applicability requires actual exact-target postcondition observation")
	}
	if decoded, err := hex.DecodeString(observed.ContentHash); err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != observed.ContentHash {
		return nil, fmt.Errorf("file applicability content hash is not canonical SHA256")
	}
	encoded, err := json.Marshal(struct {
		WorkspaceID string
		TargetRef   string
		ContentHash string
		Size        int64
	}{string(scope.WorkspaceID), observed.TargetRef, observed.ContentHash, observed.Size})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte("file-target-content/v1\x00"), encoded...))
	set := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: scope.Transaction.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + string(scope.WorkspaceID),
		ResourceKind: "workspace_target_content", ResourceID: observed.TargetRef,
		Revision: observed.ContentHash, DependencyHash: hex.EncodeToString(digest[:]),
	}}}).WithHash()
	if err := set.Validate(); err != nil {
		return nil, err
	}
	if scope.HistoricalEffectProofContractRef == "" {
		return &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: set}, nil
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.OwnerModuleID != ModuleID || descriptor.HistoricalEffectProofContractRef != runtimecontract.RuntimeFileHistoricalEffectProofV1 {
		return nil, fmt.Errorf("File mutation has no published historical effect purpose")
	}
	if scope.HistoricalEffectProofContractRef != descriptor.HistoricalEffectProofContractRef {
		return nil, fmt.Errorf("File effect purpose differs from owner contract")
	}
	return &runtimecontract.RuntimeEvidenceApplicabilityDependencies{
		ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV2, OwnerRevisionSet: set,
		PurposeRef: runtimecontract.RuntimeHistoricalEffectProofV1, OwnerModuleID: ModuleID, OperationKind: operation.Kind,
		OwnerContractRef: descriptor.HistoricalEffectProofContractRef,
	}, nil
}
