package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleGit

const StepKindRead = runtimemodel.StepKindGitRead
const StepKindWrite = runtimemodel.StepKindGitWrite

const (
	OperationStatus = "status"
	OperationDiff   = "diff"
	OperationLog    = "log"
	OperationShow   = "show"
	OperationBranch = "branch"
	OperationInit   = "init"
	OperationAdd    = "add"
	OperationCommit = "commit"
)

const (
	ErrorInvalidRequest   = "invalid_request"
	ErrorOperationDenied  = "operation_denied"
	ErrorPathRejected     = "path_rejected"
	ErrorNotGitRepository = "not_git_repository"
	ErrorBareRepository   = "bare_repository"
	ErrorHeadMismatch     = "head_mismatch"
	ErrorIdentityMissing  = "identity_missing"
	ErrorNoStagedChanges  = "no_staged_changes"
	ErrorTimeout          = "timeout"
	ErrorGitCommandFailed = "git_command_failed"
)
