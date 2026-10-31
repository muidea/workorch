package preparationcompiler

import (
	"errors"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type completionRejection struct {
	detail workspace.PreparationFinalization
	cause  error
}

func (e *completionRejection) Error() string { return e.cause.Error() }
func (e *completionRejection) Unwrap() error { return e.cause }
func rejectCompletion(code, owner, stage string, err error) error {
	if err == nil {
		err = common.ErrConflict
	}
	return &completionRejection{workspace.PreparationFinalization{Code: code, Owner: owner, Stage: stage}, err}
}
func CompletionDiagnostic(err error) *workspace.PreparationFinalization {
	if err == nil {
		return nil
	}
	var rejected *completionRejection
	if errors.As(err, &rejected) {
		v := rejected.detail
		return &v
	}
	return &workspace.PreparationFinalization{Code: "bootstrap_rejected", Owner: "workspace", Stage: "completion"}
}
