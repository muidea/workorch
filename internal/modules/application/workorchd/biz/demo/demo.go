package demo

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

type Slice = appservice.Slice

func RunAllFromEnv(ctx context.Context) ([]Slice, error) {
	return appservice.RunAllWorkorchFromEnv(ctx)
}

func RunAllWithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	return appservice.RunAllWorkorchWithBackend(ctx, backend, dataRoot)
}
