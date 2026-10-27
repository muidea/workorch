package biz

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type preparationSelectionForbiddenRunner struct{ calls int }

func (r *preparationSelectionForbiddenRunner) ObserveStatus(context.Context, string, bool) (contextregistryports.SkillHubPreparationOutput, error) {
	r.calls++
	return contextregistryports.SkillHubPreparationOutput{}, errors.New("unexpected external status")
}

func TestPreparationSkillSelectionScopeAndCancellationRejectBeforeIO(t *testing.T) {
	hub := magicevent.NewHub(4)
	defer hub.Terminate(context.Background())
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: "ws", ExpectedWorkspaceRevision: 1}
	runner := &preparationSelectionForbiddenRunner{}
	if _, err := b.observePreparationSkillSelection(nil, command, runner); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := b.observePreparationSkillSelection(context.Background(), command, nil); err == nil {
		t.Fatal("nil runner accepted")
	}
	if _, err := (*Biz)(nil).ObservePreparationSkillSelection(context.Background(), command); err == nil {
		t.Fatal("nil application accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := b.observePreparationSkillSelection(ctx, command, runner); !errors.Is(err, context.Canceled) || value.FailureCode != "cancelled" || value.SelectionCompiled || value.SealBindingCurrent {
		t.Fatal("cancelled observation accepted")
	}
	bound, _ := common.BindRuntimeWorkspaceContext(context.Background(), "other")
	if value, err := b.observePreparationSkillSelection(bound, command, runner); err == nil || value.FailureCode != "scope_conflict" {
		t.Fatal("wrong scope accepted")
	}
	if runner.calls != 0 {
		t.Fatal("invalid request reached external status")
	}
}
