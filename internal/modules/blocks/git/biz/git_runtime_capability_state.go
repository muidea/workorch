package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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
		for _, change := range status.ChangedFiles {
			unstaged = unstaged || change.Unstaged || change.Untracked
			staged = staged || change.Staged
		}
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
	revision := strings.Join([]string{status.RepoRoot, status.HeadCommit, fmt.Sprintf("%t", initialized), fmt.Sprintf("%t", unstaged), fmt.Sprintf("%t", staged)}, "|")
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
