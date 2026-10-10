package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 仅记录 Shell 在复制 scratch 时实际读取的文件，不代表子进程的全部读取。
type shellValidationCopiedInput struct {
	TargetRef   string
	ContentHash string
	Size        int64
	Mode        uint32
}

func shellValidationOwnerDependencies(definition string, request RuntimeCommandRequest, inputs []shellValidationCopiedInput) (runtimecontract.RuntimeOwnerRevisionSet, error) {
	if definition == "" || definition != strings.TrimSpace(definition) || request.WorkspaceID == "" || string(request.WorkspaceID) != strings.TrimSpace(string(request.WorkspaceID)) || !shellInputHasManagedValidation(request) || inputs == nil {
		return runtimecontract.RuntimeOwnerRevisionSet{}, fmt.Errorf("Shell validation dependencies require exact definition, workspace and managed execution")
	}
	set := runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: definition}
	for _, input := range inputs {
		if input.TargetRef == "" || input.TargetRef != strings.TrimSpace(input.TargetRef) || input.ContentHash == "" || input.ContentHash != strings.TrimSpace(input.ContentHash) || input.Size < 0 {
			return runtimecontract.RuntimeOwnerRevisionSet{}, fmt.Errorf("Shell validation dependencies require actual copied input")
		}
		preimage := struct {
			WorkspaceID  string
			Input        shellValidationCopiedInput
			RecipeID     string
			Command      string
			Args         []string
			Checks       []runtimecontract.RuntimeValidationCheckAttestationBinding
			ScopeTargets []string
		}{string(request.WorkspaceID), input, request.ValidationRecipeID, request.Command, request.Args, request.ValidationCheckAttestations, request.ValidationScopeTargetRefs}
		encoded, err := json.Marshal(preimage)
		if err != nil {
			return runtimecontract.RuntimeOwnerRevisionSet{}, err
		}
		digest := sha256.Sum256(append([]byte("shell-validation-copied-input/v1\x00"), encoded...))
		set.Entries = append(set.Entries, runtimecontract.RuntimeOwnerRevisionRef{
			OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + string(request.WorkspaceID),
			ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: input.TargetRef,
			Revision: input.ContentHash, DependencyHash: hex.EncodeToString(digest[:]),
		})
	}
	set = set.WithHash()
	if err := set.Validate(); err != nil {
		return runtimecontract.RuntimeOwnerRevisionSet{}, err
	}
	return set, nil
}
