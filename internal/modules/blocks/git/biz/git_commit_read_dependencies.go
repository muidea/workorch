package git

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"strings"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 只记录 owner 实际完整读取的 immutable commit/tree，不代表工作树或子模块内容。
func (a *RuntimeAdapter) commitOwnerDependencies(ctx context.Context, scope runtimecontract.RuntimeOperationExecutionContext, output RuntimeOperationResult) (*runtimecontract.RuntimeOwnerRevisionSet, error) {
	if output.Operation != gitcommon.OperationCommit || output.Status != "completed" || output.ZeroWork {
		return nil, nil
	}
	definition, workspace := scope.Transaction.DefinitionRevision, string(scope.WorkspaceID)
	if err := validateGitCommitDependencyScope(scope); err != nil {
		return nil, err
	}
	if output.RepoRoot == "" || !gitDependencyObjectID(output.CommitHash) {
		return nil, fmt.Errorf("Git commit dependencies require actual repository and commit identity")
	}
	commit, err := a.readDependencyObject(ctx, output.RepoRoot, "commit", output.CommitHash)
	if err != nil {
		return nil, err
	}
	first, _, found := strings.Cut(commit, "\n")
	tree, foundTree := strings.CutPrefix(first, "tree ")
	if !found || !foundTree || !gitDependencyObjectID(tree) {
		return nil, fmt.Errorf("Git commit dependency has no exact tree identity")
	}
	treeBody, err := a.readDependencyObject(ctx, output.RepoRoot, "tree", tree)
	if err != nil {
		return nil, err
	}
	set := runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: definition}
	for _, object := range []struct {
		Kind string
		ID   string
		Body string
	}{{"commit", output.CommitHash, commit}, {"tree", tree, treeBody}} {
		digest := sha256.Sum256([]byte(object.Body))
		revision := hex.EncodeToString(digest[:])
		preimage := struct {
			WorkspaceID string
			ObjectKind  string
			ObjectID    string
			Revision    string
			Bytes       int
		}{workspace, object.Kind, object.ID, revision, len(object.Body)}
		encoded, err := json.Marshal(preimage)
		if err != nil {
			return nil, err
		}
		dependency := sha256.Sum256(append([]byte("git-immutable-object-read/v1\x00"), encoded...))
		set.Entries = append(set.Entries, runtimecontract.RuntimeOwnerRevisionRef{
			OwnerKind: "capability", OwnerInstanceID: ModuleID + "/" + workspace,
			ResourceKind: "git_object", ResourceID: object.ID, SnapshotID: object.ID,
			Revision: revision, DependencyHash: hex.EncodeToString(dependency[:]),
		})
	}
	set = set.WithHash()
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return &set, nil
}

func (a *RuntimeAdapter) readDependencyObject(ctx context.Context, repo, kind, id string) (string, error) {
	// 不读取 replace ref 指向的替代正文；bounded runner 必须证明完整输出。
	if !gitDependencyObjectID(id) || (kind != "commit" && kind != "tree") {
		return "", fmt.Errorf("Git dependency requires an exact commit/tree object")
	}
	result, err := a.runGit(ctx, repo, a.timeout, maxOutputBytes, "--no-replace-objects", "cat-file", kind, id)
	if err != nil {
		return "", fmt.Errorf("Git dependency object %s/%s was not read: %w", kind, id, err)
	}
	if result.Truncated {
		return "", fmt.Errorf("Git dependency object %s/%s was not completely retained: stdout=%d stderr=%d limit=%d", kind, id, result.StdoutBytes, result.StderrBytes, maxOutputBytes)
	}
	var objectHash hash.Hash = sha1.New()
	if len(id) == 64 {
		objectHash = sha256.New()
	}
	_, _ = fmt.Fprintf(objectHash, "%s %d\x00", kind, len(result.Stdout))
	_, _ = objectHash.Write([]byte(result.Stdout))
	if hex.EncodeToString(objectHash.Sum(nil)) != id {
		return "", fmt.Errorf("Git dependency object %s/%s body does not match its identity", kind, id)
	}
	return result.Stdout, nil
}

func validateGitCommitDependencyScope(scope runtimecontract.RuntimeOperationExecutionContext) error {
	definition, workspace := scope.Transaction.DefinitionRevision, string(scope.WorkspaceID)
	if definition == "" || definition != strings.TrimSpace(definition) || workspace == "" || workspace != strings.TrimSpace(workspace) {
		return fmt.Errorf("Git commit dependencies require exact definition and workspace")
	}
	return nil
}

func gitDependencyObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
