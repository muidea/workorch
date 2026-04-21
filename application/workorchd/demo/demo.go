package demo

import (
	"context"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/presenter"
)

type Slice struct {
	Title    string
	Snapshot *presenter.Snapshot
}

func RunAllFromEnv(ctx context.Context) ([]Slice, error) {
	rt := bootstrap.NewFromEnv()
	return runAll(ctx, rt)
}

func runAll(ctx context.Context, rt *bootstrap.Runtime) ([]Slice, error) {
	slices := []struct {
		title string
		run   func(context.Context, *bootstrap.Runtime) (*presenter.Snapshot, error)
	}{
		{title: "phase1 runnable slice", run: runRunnableSlice},
		{title: "phase1 decision-blocked-resume slice", run: runDecisionBlockedResumeSlice},
		{title: "phase1 approval-blocked-resume slice", run: runApprovalBlockedResumeSlice},
		{title: "phase1 approval-rejected-terminal slice", run: runApprovalRejectedTerminalSlice},
	}

	ret := make([]Slice, 0, len(slices))
	for _, item := range slices {
		snapshot, err := item.run(ctx, rt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, Slice{
			Title:    item.title,
			Snapshot: snapshot,
		})
	}

	return ret, nil
}
