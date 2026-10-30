// Package preparationtrace records preparation/consumption timing without
// carrying business inputs, raw diagnostics or credentials into general logs.
// Timings are diagnostic only and never participate in readiness or authority.
package preparationtrace

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func Start(ctx context.Context, workspaceID common.WorkspaceID, requestID, phase string, revision uint64) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	return func() {
		state := "active"
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			state = "deadline_exceeded"
		case errors.Is(ctx.Err(), context.Canceled):
			state = "cancelled"
		}
		slog.InfoContext(ctx, "preparation phase completed", "workspace_id", workspaceID, "request_id", requestID,
			"phase", phase, "workspace_revision", revision, "started_at", started.UTC(),
			"finished_at", time.Now().UTC(), "duration", time.Since(started), "context_state", state)
	}
}
