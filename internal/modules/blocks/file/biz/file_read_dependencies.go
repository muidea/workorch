package file

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 仅消费本次实际流式读取结果；不重新打开文件或从目录 snapshot 推断历史内容。
func fileReadOwnerDependencies(command fileevents.ExecuteFileReadOperationCommand, observed FileReadResult, startLine, endLine, maxBytes int64) (runtimecontract.RuntimeOwnerRevisionSet, error) {
	definition := command.Transaction.DefinitionRevision
	if definition == "" || definition != strings.TrimSpace(definition) || command.WorkspaceID == "" || !observed.Exists || observed.TargetRef == "" || observed.ContentHash == "" {
		return runtimecontract.RuntimeOwnerRevisionSet{}, fmt.Errorf("file read dependencies require exact definition, workspace and actual read observation")
	}
	preimage := struct {
		WorkspaceID                        string
		TargetRef                          string
		ContentHash                        string
		SelectedContent                    string
		StartLine, EndLine, MaxBytes       int64
		Truncated, Binary, ContentComplete bool
	}{string(command.WorkspaceID), observed.TargetRef, observed.ContentHash, observed.Content,
		startLine, endLine, maxBytes, observed.Truncated, observed.Binary, observed.ContentComplete}
	encoded, err := json.Marshal(preimage)
	if err != nil {
		return runtimecontract.RuntimeOwnerRevisionSet{}, err
	}
	digest := sha256.Sum256(append([]byte("file-read-dependency/v1\x00"), encoded...))
	set := (runtimecontract.RuntimeOwnerRevisionSet{
		DefinitionRevision: definition,
		Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
			OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + string(command.WorkspaceID),
			ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: observed.TargetRef,
			Revision: observed.ContentHash, DependencyHash: hex.EncodeToString(digest[:]),
		}},
	}).WithHash()
	if err := set.Validate(); err != nil {
		return runtimecontract.RuntimeOwnerRevisionSet{}, err
	}
	return set, nil
}
