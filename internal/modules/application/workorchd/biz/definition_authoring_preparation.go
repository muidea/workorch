package biz

import (
	"context"
	"errors"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 准备诊断只是观察，不是 permit 或重试权威。
// 原始 cause 在任何 Application 包装前交给 owner 保全。
type authoringPreparationError struct {
	Stage                   workspace.DefinitionAuthoringPreparationStage
	PhaseCode               string
	DiagnosticID            string
	Persisted               bool
	PersistenceFailureClass string
	cause                   error
	persistenceFailure      error
}

func (e *authoringPreparationError) Error() string {
	return e.Unwrap().Error()
}

func (e *authoringPreparationError) Unwrap() error {
	return errors.Join(e.cause, e.persistenceFailure)
}

func authoringPreparationIdentity(view workspaceevents.QueryDefinitionAuthoringResult, stage workspace.DefinitionAuthoringPreparationStage, target string) workspace.DefinitionAuthoringPreparationDiagnostic {
	return workspace.DefinitionAuthoringPreparationDiagnostic{
		WorkspaceID: view.WorkspaceID, InputHash: view.Record.InputHash,
		TemplateSnapshotHash: view.Record.TemplateSnapshotHash, ObservedWorkspaceRevision: view.WorkspaceRevision,
		Stage: stage, TargetRequestID: target,
	}
}

func (b *Biz) captureAuthoringPreparationFailure(ctx context.Context, identity workspace.DefinitionAuthoringPreparationDiagnostic, phase string, cause error) error {
	identity.PhaseCode = phase
	originalCause := cause
	var refusal *workspace.DefinitionAuthoringBudgetRefusalError
	if errors.As(cause, &refusal) && refusal.Cause != nil {
		originalCause = refusal.Cause
	}
	diagnostic := workspace.NewDefinitionAuthoringPreparationDiagnostic(identity, []byte(originalCause.Error()))
	failure := &authoringPreparationError{Stage: identity.Stage, PhaseCode: phase, DiagnosticID: diagnostic.DiagnosticID, cause: cause}
	if err := ctx.Err(); err != nil {
		failure.persistenceFailure = err
	} else {
		_, failure.persistenceFailure = b.RecordPreparationDiagnostic(ctx, workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: diagnostic})
		failure.Persisted = failure.persistenceFailure == nil
	}
	failure.PersistenceFailureClass = authoringPreparationPersistenceFailureClass(failure.persistenceFailure)
	return failure
}

func (b *Biz) captureAuthoringBudgetRefusal(ctx context.Context, identity workspace.DefinitionAuthoringPreparationDiagnostic, cause error) error {
	var refusal *workspace.DefinitionAuthoringBudgetRefusalError
	// 未知 reserve 冲突和响应丢失不能证明未签发 permit；
	// 只有 owner 的 typed guard 拒绝具有这一确定含义。
	if !errors.As(cause, &refusal) || refusal.Stage != identity.Stage || refusal.RequestID != identity.TargetRequestID || refusal.Limit == 0 {
		return cause
	}
	switch refusal.Budget {
	case "request", "attempt":
		return b.captureAuthoringPreparationFailure(ctx, identity, refusal.Budget+"_budget", cause)
	default:
		return cause
	}
}

func authoringPreparationPersistenceFailureClass(err error) string {
	if err == nil {
		return ""
	}
	var limit *workspace.PreparationDiagnosticLimitError
	switch {
	case errors.As(err, &limit):
		return "diagnostic_limit"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, common.ErrConflict):
		return "conflict"
	case errors.Is(err, common.ErrInvalidArgument):
		return "invalid_argument"
	default:
		return "owner_unavailable"
	}
}

// 只有明确的单资源失败才携带 ResourceRef。Error 原样返回，
// 诊断 cause bytes 因而仍是 File 原始错误 bytes。
type authoringResourcePreparationError struct {
	ref   string
	cause error
}

func (e *authoringResourcePreparationError) Error() string { return e.cause.Error() }
func (e *authoringResourcePreparationError) Unwrap() error { return e.cause }
