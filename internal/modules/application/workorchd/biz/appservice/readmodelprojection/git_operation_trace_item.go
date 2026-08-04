package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func projectGitOperationTrace(step workorchprojectioncontract.StepView) workorchprojectioncontract.GitOperationTrace {
	commit := readModelMapValue(step.Output["commit"])
	return workorchprojectioncontract.GitOperationTrace{
		StepID:           step.ID,
		Kind:             step.Kind,
		Operation:        readModelStringValue(step.Output["operation"]),
		Status:           readModelStringValue(step.Output["status"]),
		RepoRoot:         readModelStringValue(step.Output["repoRoot"]),
		WorkingDir:       readModelStringValue(step.Output["workingDir"]),
		Branch:           readModelStringValue(step.Output["branch"]),
		HeadCommit:       readModelStringValue(step.Output["headCommit"]),
		HeadCommitBefore: readModelStringValue(step.Output["headCommitBefore"]),
		HeadCommitAfter:  readModelStringValue(step.Output["headCommitAfter"]),
		CommitHash:       readModelStringValue(commit["hash"]),
		CommitSubject:    readModelStringValue(commit["subject"]),
		ChangedFiles:     mapGitChangedFiles(step.Output["changedFiles"]),
		StagedFiles:      mapGitChangedFiles(step.Output["stagedFiles"]),
		Truncated:        readModelBoolValue(step.Output["truncated"]),
		ZeroWork:         readModelBoolValue(step.Output["zeroWork"]),
		ErrorCode:        readModelStringValue(step.Output["errorCode"]),
		Summary:          readModelStringValue(step.Output["summary"]),
		EvidenceRefs:     []string{step.ID},
	}
}
