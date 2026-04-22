package appservice

import (
	"context"
	"errors"
)

func RunAllPhase1WithSession(ctx context.Context, session *RuntimeSession) (ret []Slice, err error) {
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	return RunAllPhase1(ctx, session.Runtime)
}

func RunAllPhase1WithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	return RunAllPhase1WithSession(ctx, NewRuntimeSessionWithBackend(backend, dataRoot))
}

func RunAllPhase1FromEnv(ctx context.Context) ([]Slice, error) {
	return RunAllPhase1WithSession(ctx, NewRuntimeSessionFromEnv())
}
