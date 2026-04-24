package appservice

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/pkg/common"
)

func RunAllWorkorchWithSession(ctx context.Context, session *RuntimeSession) (ret []Slice, err error) {
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return nil, common.ErrInvalidArgument
	}

	return RunAllWorkorch(ctx, session.Runtime)
}

func RunAllWorkorchWithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return RunAllWorkorchWithSession(ctx, session)
}

func RunAllWorkorchFromEnv(ctx context.Context) ([]Slice, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return RunAllWorkorchWithSession(ctx, session)
}
