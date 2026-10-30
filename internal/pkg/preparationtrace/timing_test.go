package preparationtrace

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestTimingKeepsScopesSeparateAndReportsContextFailure(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	ctx, cancel := context.WithCancel(context.Background())
	endA := Start(ctx, "workspace-a", "request-a", "settlement", 4)
	endB := Start(context.Background(), "workspace-b", "request-b", "reservation", 7)
	cancel()
	endB()
	endA()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"workspace_id":"workspace-b"`) || !strings.Contains(lines[0], `"context_state":"active"`) || !strings.Contains(lines[1], `"workspace_id":"workspace-a"`) || !strings.Contains(lines[1], `"context_state":"cancelled"`) {
		t.Fatal("concurrent scopes or cancellation diagnostics lost", output.String())
	}
}
