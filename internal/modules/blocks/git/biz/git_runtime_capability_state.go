package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CapabilityState is a read-only owner observation used by Runtime before a
// candidate is requested. It never initializes a repository or changes files.
func (a *RuntimeAdapter) CapabilityState(ctx context.Context, requirementID, subjectRef, deliverable string) (runtimecontract.RuntimeCapabilityStateSnapshot, error) {
	requirementID = strings.TrimSpace(requirementID)
	subjectRef = strings.TrimSpace(subjectRef)
	deliverable = strings.TrimSpace(deliverable)
	if a == nil || requirementID == "" || subjectRef == "" || deliverable == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	status, err := a.ExecuteOperation(ctx, RuntimeOperationRequest{Kind: gitcommon.StepKindRead, WorkspaceDeliverableDirectory: deliverable, Operation: gitcommon.OperationStatus})
	sequence := []runtimecontract.RuntimeRequiredOperationVariant{}
	disposition := runtimecontract.RuntimeCapabilityStateReady
	initialized := err == nil
	unstaged, staged := false, false
	if initialized {
		if status.Truncated {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, fmt.Errorf("%w: Git repository state exceeds the bounded owner observation", common.ErrConflict)
		}
		unstaged, staged = gitStatusStateFlags(status.ChangedFiles)
		if unstaged {
			sequence = append(sequence, runtimecontract.RuntimeRequiredOperationVariant{RequirementID: "stage-changes", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd})
		}
		if unstaged || staged {
			dependsOn := []string(nil)
			if unstaged {
				dependsOn = []string{"stage-changes"}
			}
			sequence = append(sequence, runtimecontract.RuntimeRequiredOperationVariant{RequirementID: "create-commit", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit, DependsOnRequirementIDs: dependsOn})
		}
	} else if status.ErrorCode == gitcommon.ErrorNotGitRepository {
		entries, readErr := os.ReadDir(deliverable)
		if readErr != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, readErr
		}
		for _, entry := range entries {
			if entry.Name() != ".git" {
				unstaged = true
				break
			}
		}
		if unstaged {
			sequence = []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "repo-init", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationInit},
				{RequirementID: "stage-changes", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd, DependsOnRequirementIDs: []string{"repo-init"}},
				{RequirementID: "create-commit", Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit, DependsOnRequirementIDs: []string{"stage-changes"}},
			}
		}
	} else {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if len(sequence) > 0 {
		disposition = runtimecontract.RuntimeCapabilityStateSequenceRequired
	}
	revision := gitRepositoryStateRevision(status, initialized, unstaged, staged)
	hash := sha256.Sum256([]byte(revision))
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: requirementID, AcquisitionKey: "capability-state/" + requirementID,
		OwnerModuleID: gitcommon.ModuleID, CapabilityStateKind: "repository_state", SubjectRef: subjectRef,
		ObservedRevision: revision, ContentHash: hex.EncodeToString(hash[:]), Disposition: disposition,
		RequiredOperationSequence: sequence, QueryReceiptRef: "capability-state-receipt/" + requirementID,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	return snapshot, nil
}

func gitStatusStateFlags(changes []GitFileChange) (unstaged, staged bool) {
	for _, change := range changes {
		unstaged = unstaged || change.Unstaged || change.Untracked
		staged = staged || change.Staged
	}
	return unstaged, staged
}

const (
	gitRepositoryRevisionPrefix = "git-state-v2/repository/"
	gitAbsentRevisionPrefix     = "git-state-v2/absent/"
)

// gitRepositoryStateRevision binds Git authority to the complete bounded
// porcelain observation instead of only HEAD plus dirty booleans. File-owner
// workspace-tree freshness remains a separate exact dependency.
func gitRepositoryStateRevision(status RuntimeOperationResult, initialized, unstaged, staged bool) string {
	rows := make([]string, 0, len(status.ChangedFiles))
	for _, change := range status.ChangedFiles {
		rows = append(rows, strings.Join([]string{
			strings.TrimSpace(change.Path), strings.TrimSpace(change.OldPath), strings.TrimSpace(change.Status),
			fmt.Sprintf("%t", change.Staged), fmt.Sprintf("%t", change.Unstaged), fmt.Sprintf("%t", change.Untracked),
		}, "\x00"))
	}
	sort.Strings(rows)
	h := sha256.New()
	for _, value := range []string{
		strings.TrimSpace(status.RepoRoot), strings.TrimSpace(status.HeadCommit), strings.TrimSpace(status.Branch),
		fmt.Sprintf("%t", status.DetachedHead), fmt.Sprintf("%t", status.UnbornHead),
		fmt.Sprintf("%t", initialized), fmt.Sprintf("%t", unstaged), fmt.Sprintf("%t", staged),
		fmt.Sprintf("%d", status.AheadBehind.Ahead), fmt.Sprintf("%d", status.AheadBehind.Behind),
	} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(value), value)
	}
	for _, row := range rows {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(row), row)
	}
	prefix := gitAbsentRevisionPrefix
	if initialized {
		prefix = gitRepositoryRevisionPrefix
	}
	return prefix + hex.EncodeToString(h.Sum(nil))
}
