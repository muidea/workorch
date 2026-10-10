package file

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 本次 patch 执行读取的输入或 recovery 确认读取的后像；不是 receipt.BeforeHash。
type filePatchReadObservation struct {
	TargetRef   string
	ContentHash string
	Size        int64
	Purpose     string
}

func observedFilePatchRead(target string, content []byte, purpose string) *filePatchReadObservation {
	return &filePatchReadObservation{TargetRef: target, ContentHash: sha256Hex(content), Size: int64(len(content)), Purpose: purpose}
}

func filePatchOwnerDependencies(scope runtimecontract.RuntimeOperationExecutionContext, operation runtimecontract.RuntimeOperation, observed *filePatchReadObservation) (*runtimecontract.RuntimeOwnerRevisionSet, error) {
	if observed == nil {
		return nil, nil
	}
	definition := scope.Transaction.DefinitionRevision
	if definition == "" || definition != strings.TrimSpace(definition) || scope.WorkspaceID == "" || string(scope.WorkspaceID) != strings.TrimSpace(string(scope.WorkspaceID)) ||
		observed.TargetRef == "" || observed.TargetRef != strings.TrimSpace(observed.TargetRef) || observed.ContentHash == "" || observed.ContentHash != strings.TrimSpace(observed.ContentHash) || observed.Size < 0 ||
		(observed.Purpose != "patch_input" && observed.Purpose != "replay_postcondition") {
		return nil, fmt.Errorf("file patch dependencies require exact definition, workspace and actual read observation")
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
		OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + string(scope.WorkspaceID), ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget),
		ResourceID: observed.TargetRef, Revision: observed.ContentHash, DependencyHash: hex.EncodeToString(digest[:]),
	}}}).WithHash()
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return &set, nil
}
