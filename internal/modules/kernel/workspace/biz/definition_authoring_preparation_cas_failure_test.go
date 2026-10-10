package workspace

import (
	"context"
	"errors"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type failPreparationDiagnosticCASRepository struct {
	repository.WorkspaceRepository
	cause error
	calls int
}

func (r *failPreparationDiagnosticCASRepository) CompareAndSwap(context.Context, *workspace.Workspace, uint64) error {
	r.calls++
	return r.cause
}

// 诊断 CAS 失败必须保留存储错误，不得宣称已保存或留下部分 Workspace 写入。
func TestPreparationDiagnosticCASFailureIsNotReportedAsStored(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	workspaceRow, command := seedPreparationWorkspace(t, owners, "ws-prep-cas-failure")
	cause := errors.New("synthetic preparation diagnostic CAS failure")
	failing := &failPreparationDiagnosticCASRepository{WorkspaceRepository: owners.workspace.workspaces, cause: cause}
	owners.workspace.workspaces = failing
	_, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{
		WorkspaceID: workspaceRow.ID, PreparationDiagnostic: &command,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("CAS failure = %v, want original repository error", err)
	}
	if failing.calls != 1 {
		t.Fatalf("diagnostic CAS attempts = %d, want exactly one", failing.calls)
	}
	query := workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: workspaceRow.ID, DiagnosticID: command.Diagnostic.DiagnosticID}
	if _, err := owners.workspace.queryPreparationDiagnostic(ctx, query); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("failed diagnostic CAS became queryable: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, workspaceRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DefinitionAuthoring == nil {
		t.Fatal("failed diagnostic CAS removed authoring record")
	}
	if stored.Revision != workspaceRow.Revision || len(stored.DefinitionAuthoring.PreparationDiagnostics) != 0 {
		t.Fatalf("failed diagnostic CAS partially changed workspace: revision=%d diagnostics=%d", stored.Revision, len(stored.DefinitionAuthoring.PreparationDiagnostics))
	}
}
