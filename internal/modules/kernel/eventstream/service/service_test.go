package service

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func TestModuleListEventsRejectsUnconfiguredHandler(t *testing.T) {
	if _, err := (*Module)(nil).ListEvents(context.Background(), repository.EventRecordQuery{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil module list events err = %v, want invalid argument", err)
	}
	if _, err := New(Dependencies{}).ListEvents(context.Background(), repository.EventRecordQuery{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unconfigured module list events err = %v, want invalid argument", err)
	}
}
