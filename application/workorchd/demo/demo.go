package demo

import (
	"context"

	"github.com/muidea/workorch/application/workorchd/appservice"
)

type Slice = appservice.Slice

func RunAllFromEnv(ctx context.Context) ([]Slice, error) {
	return appservice.RunAllPhase1FromEnv(ctx)
}

func RunAllWithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	return appservice.RunAllPhase1WithBackend(ctx, backend, dataRoot)
}
