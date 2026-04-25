package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

func RunAllWorkorchWithSession(ctx context.Context, session *RuntimeSession) (ret []Slice, err error) {
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return nil, common.ErrInvalidArgument
	}

	return RunAllWorkorch(ctx, session.Runtime)
}

func RunAllWorkorchWithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
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

func RunWorkorchSliceWithSession(ctx context.Context, session *RuntimeSession, sliceID string) (ret Slice, err error) {
	if session == nil {
		return Slice{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return Slice{}, common.ErrInvalidArgument
	}

	return RunWorkorchSlice(ctx, session.Runtime, sliceID)
}

func RunWorkorchSliceWithBackend(ctx context.Context, backend string, dataRoot string, sliceID string) (Slice, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Slice{}, err
	}
	return RunWorkorchSliceWithSession(ctx, session, sliceID)
}

func RunWorkorchSliceFromEnv(ctx context.Context, sliceID string) (Slice, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Slice{}, err
	}
	return RunWorkorchSliceWithSession(ctx, session, sliceID)
}

func ReadWorkorchSliceSnapshotWithSession(ctx context.Context, session *RuntimeSession, sliceID string) (ret Slice, err error) {
	if session == nil {
		return Slice{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return Slice{}, common.ErrInvalidArgument
	}

	return ReadWorkorchSliceSnapshot(ctx, session.Runtime, sliceID)
}

func ReadWorkorchSliceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, sliceID string) (Slice, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Slice{}, err
	}
	return ReadWorkorchSliceSnapshotWithSession(ctx, session, sliceID)
}

func ReadWorkorchSliceSnapshotFromEnv(ctx context.Context, sliceID string) (Slice, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Slice{}, err
	}
	return ReadWorkorchSliceSnapshotWithSession(ctx, session, sliceID)
}

func SummarizeWorkorchSliceStatusWithSession(ctx context.Context, session *RuntimeSession, sliceID string) (ret SliceStatus, err error) {
	if session == nil {
		return SliceStatus{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return SliceStatus{}, common.ErrInvalidArgument
	}

	return SummarizeWorkorchSliceStatus(ctx, session.Runtime, sliceID)
}

func SummarizeWorkorchSliceStatusWithBackend(ctx context.Context, backend string, dataRoot string, sliceID string) (SliceStatus, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return SliceStatus{}, err
	}
	return SummarizeWorkorchSliceStatusWithSession(ctx, session, sliceID)
}

func SummarizeWorkorchSliceStatusFromEnv(ctx context.Context, sliceID string) (SliceStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return SliceStatus{}, err
	}
	return SummarizeWorkorchSliceStatusWithSession(ctx, session, sliceID)
}

func ListAvailableWorkorchSliceStatusesWithSession(ctx context.Context, session *RuntimeSession) (ret []SliceStatus, err error) {
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return nil, common.ErrInvalidArgument
	}

	return ListAvailableWorkorchSliceStatuses(ctx, session.Runtime)
}

func ListAvailableWorkorchSliceStatusesWithBackend(ctx context.Context, backend string, dataRoot string) ([]SliceStatus, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListAvailableWorkorchSliceStatusesWithSession(ctx, session)
}

func ListAvailableWorkorchSliceStatusesFromEnv(ctx context.Context) ([]SliceStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListAvailableWorkorchSliceStatusesWithSession(ctx, session)
}

func ListAvailableWorkorchSliceSnapshotsWithSession(ctx context.Context, session *RuntimeSession) (ret []Slice, err error) {
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)
	if session.Runtime == nil || session.Runtime.Runtime == nil {
		return nil, common.ErrInvalidArgument
	}

	return ListAvailableWorkorchSliceSnapshots(ctx, session.Runtime)
}

func ListAvailableWorkorchSliceSnapshotsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Slice, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListAvailableWorkorchSliceSnapshotsWithSession(ctx, session)
}

func ListAvailableWorkorchSliceSnapshotsFromEnv(ctx context.Context) ([]Slice, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListAvailableWorkorchSliceSnapshotsWithSession(ctx, session)
}
