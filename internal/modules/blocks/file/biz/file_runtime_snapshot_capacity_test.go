package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestTargetSnapshotHonorsCompleteReadCapacity(t *testing.T) {
	for _, size := range []int{64733, 65535, 65536, 65537, 72001, 128*1024 + 1, 256 * 1024} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			directory := t.TempDir()
			raw := strings.Repeat("x", size-3) + "中"
			if err := os.WriteFile(filepath.Join(directory, "input.txt"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			for _, limit := range []int{0, size} {
				snapshot, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), directory, "input.txt", limit)
				if err != nil {
					t.Fatalf("complete snapshot size=%d limit=%d: %v", size, limit, err)
				}
				if !snapshot.Complete() || snapshot.Content != raw || snapshot.Size != int64(size) || snapshot.ContentHash != sha256Hex([]byte(raw)) {
					t.Fatal("snapshot lost original bytes or identity")
				}
			}
			read, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{WorkspaceDirectory: directory, Path: "input.txt"})
			if err != nil || (size > defaultMaxReadBytes && (read.ContentComplete || !read.Truncated)) {
				t.Fatal("ordinary bounded read contract changed", err)
			}
		})
	}
}

func TestTargetSnapshotCapacityIsExplicitAndNeverGrantsCompleteInput(t *testing.T) {
	dir := t.TempDir()
	raw := strings.Repeat("a", fileevents.DefaultTargetSnapshotBytes+1)
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, 5, 65536, fileevents.DefaultTargetSnapshotBytes} {
		snapshot, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "input.txt", limit)
		var capacity *fileevents.TargetSnapshotCapacityError
		expected := limit
		if expected == 0 {
			expected = fileevents.DefaultTargetSnapshotBytes
		}
		if !errors.As(err, &capacity) || capacity.SizeBytes != int64(len(raw)) || capacity.LimitBytes != expected || capacity.TargetRef != "input.txt" {
			t.Fatalf("lost capacity facts: %v", err)
		}
		if snapshot.Complete() || snapshot.ContentState != rc.SnapshotContentStateTruncated || len(snapshot.Content) != expected || snapshot.ContentHash != sha256Hex([]byte(raw)) {
			t.Fatal("partial snapshot became complete or lost full-stream identity")
		}
	}
	// An explicitly larger bounded request is legal. Never silently clamp it to
	// either the ordinary read limit or the default acquisition capacity.
	previous, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "input.txt", len(raw))
	if err != nil || !previous.Complete() || previous.Content != raw {
		t.Fatal("explicit capacity was clamped", err)
	}
	next, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "input.txt", len(raw))
	if err != nil || !reflect.DeepEqual(previous, next) {
		t.Fatal("fresh owner changed frozen bytes", err)
	}
	if _, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "input.txt", -1); err == nil {
		t.Fatal("negative capacity accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewRuntimeAdapter().AcquireTargetSnapshot(ctx, dir, "input.txt", len(raw)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	if _, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "../input.txt", len(raw)); err == nil {
		t.Fatal("workspace escape accepted")
	}
	other := t.TempDir()
	if _, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), other, "input.txt", len(raw)); err == nil {
		t.Fatal("workspace isolation lost")
	}
	// Binary bytes beyond the old read boundary remain a binary rejection,
	// rather than a capacity diagnosis or a usable prefix.
	if err := os.WriteFile(path, append([]byte(strings.Repeat("x", 72001)), 0xff), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewRuntimeAdapter().AcquireTargetSnapshot(context.Background(), dir, "input.txt", 0)
	var capacity *fileevents.TargetSnapshotCapacityError
	if err == nil || errors.As(err, &capacity) || snapshot.ContentState != rc.SnapshotContentStateBinary {
		t.Fatal("binary input misclassified", err)
	}
}
