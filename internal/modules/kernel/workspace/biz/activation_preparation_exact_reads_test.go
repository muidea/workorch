package workspace

import (
	"context"
	"errors"
	"fmt"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type preparationGoalDrift struct {
	repository.GoalRepository
	drift bool
	reads int
}

func (r *preparationGoalDrift) ListByWorkspaceID(ctx context.Context, id common.WorkspaceID) ([]*workspace.Goal, error) {
	r.reads++
	rows, err := r.GoalRepository.ListByWorkspaceID(ctx, id)
	if r.drift && len(rows) > 0 {
		rows[0].Text += " 实际正式行变化"
	}
	return rows, err
}
func TestPreparationExactReadsReuseOnlyIdenticalRawRowsWithinInvocation(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-local-reuse")
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	ctx := withPreparationExactReads(context.Background())
	reads := ctx.Value(preparationExactReadsKey{}).(*preparationExactReads)
	goals := &preparationGoalDrift{GoalRepository: owners.workspace.goals}
	owners.workspace.goals = goals
	first, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	detached := first.Snapshot.Clone()
	first.Snapshot.Goal.Text = "caller mutation"
	second, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command)
	if err != nil || !reflect.DeepEqual(second.Snapshot, detached) || reads.hits != 1 || goals.reads != 2 {
		t.Fatal("reuse aliased result or skipped actual row read", err)
	}
	goals.drift = true
	if _, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command); err == nil || reads.hits != 1 {
		t.Fatal("hash/revision-only reuse accepted changed formal Goal")
	}
	goals.drift = false
	other := withPreparationExactReads(context.Background())
	if _, err := owners.workspace.queryExactConfirmedStartupSnapshot(other, command); err != nil || other.Value(preparationExactReadsKey{}).(*preparationExactReads).hits != 0 {
		t.Fatal("reuse crossed invocation", err)
	}
	// Even a hit performs the final Workspace revision fence.
	owners.workspace.workspaces = &preparationRevisionReadRepository{WorkspaceRepository: owners.workspace.workspaces}
	if _, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command); err == nil {
		t.Fatal("hit bypassed final revision fence")
	}
}

func TestPreparationLargeExactReadsTwoWorkspaceIsolation(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	inputs := []struct {
		id     common.WorkspaceID
		count  int
		domain string
	}{{"ws-logistics-large", 68, "物流配送原始约束"}, {"ws-accounting-large", 128, "会计结算原始约束"}}
	var workspaces []*workspace.Workspace
	for _, input := range inputs {
		input := input
		w := seedPreparationEligibleWithConfiguration(t, owners, input.id, "delivery/"+string(input.id), nil, func(template *workspace.WorkspaceTemplate) {
			template.ID = "template-" + string(input.id)
			template.Description = input.domain
			template.DoDItems = nil
			for i := 0; i < input.count-2; i++ {
				template.DoDItems = append(template.DoDItems, fmt.Sprintf("%s %03d：%s", input.domain, i, strings.Repeat(input.domain, 6)))
			}
		})
		if len(w.DefinitionAuthoringConfirmation.DoD.Items) != input.count {
			t.Fatal("fixture size mismatch")
		}
		workspaces = append(workspaces, w)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, w := range workspaces {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(withPreparationExactReads(context.Background()), 12*time.Second)
			defer cancel()
			began := time.Now()
			c := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
			var hash string
			for n := 0; n < 17; n++ {
				result, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, c)
				if err != nil {
					t.Errorf("Workspace %s read %d: %v", w.ID, n, err)
					return
				}
				if result.Snapshot.WorkspaceID != w.ID || len(result.Snapshot.DoD.Items) != inputs[i].count || !reflect.DeepEqual(result.Snapshot.DoD, w.DefinitionAuthoringConfirmation.DoD) {
					t.Error("mixed Workspace raw source")
					return
				}
				if n == 0 {
					hash = result.Snapshot.SnapshotHash
				} else if hash != result.Snapshot.SnapshotHash {
					t.Error("stable source hash drifted")
				}
			}
			if ctx.Value(preparationExactReadsKey{}).(*preparationExactReads).hits != 16 {
				t.Error("same invocation repeated full compilation")
			}
			t.Logf("%s %d items, 17 fenced actual reads: %s", w.ID, inputs[i].count, time.Since(began))
		}()
	}
	close(start)
	wg.Wait()
}

type preparationCancelRead struct {
	repository.WorkspaceRepository
	cancel context.CancelFunc
}

func (r *preparationCancelRead) Get(ctx context.Context, id common.WorkspaceID) (*workspace.Workspace, error) {
	w, err := r.WorkspaceRepository.Get(ctx, id)
	r.cancel()
	return w, err
}
func TestPreparationSettlementCancellationBeforeCASPreservesOriginalAcceptedCut(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-cancel-before-cas")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(context.Background(), c, false, true); err != nil {
		t.Fatal(err)
	}
	before, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalRepo := owners.workspace.workspaces
	owners.workspace.workspaces = &preparationCancelRead{WorkspaceRepository: originalRepo, cancel: cancel}
	_, err = owners.workspace.finishBlockedPreparation(ctx, workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", At: time.Now().UTC()})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled settlement reached CAS", err)
	}
	owners.workspace.workspaces = originalRepo
	after, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(before, after) || after.StartupPreparation.Attempts[0].State != workspace.WorkspacePreparationAccepted {
		t.Fatal("cancellation rewrote original accepted identity", err)
	}
	if _, err := owners.workspace.beginWorkspacePreparation(context.Background(), c, true, true); err != nil {
		t.Fatal("explicit original recovery failed", err)
	}
	recovered, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || recovered.StartupPreparation.Attempts[0].RequestID != c.RequestID || recovered.StartupPreparation.Attempts[0].Checks[0].Reason != "original_check_interrupted" || len(recovered.StartupPreparation.Completions) != 0 {
		t.Fatal("recovery granted completion or replaced request", err)
	}
}
