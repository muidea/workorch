//go:build unix

package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestShellDependencyDescriptorOpenRejectsLinkAndFIFOSelectors(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "real", "target"), []byte{'a', 0, 0xff, '\n'}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(directory, "link-parent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real/target", filepath.Join(directory, "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(directory, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx := context.Background()
	for _, ref := range []string{"link-parent/target", "link-file", "../outside", "/outside", "", ".", "real/../real/target", "real//target"} {
		if file, err := openShellDependencySelector(ctx, root, ref); err == nil {
			_ = file.Close()
			t.Fatalf("descriptor open followed %s despite original selector gate", ref)
		}
	}
	start := time.Now()
	file, err := openShellDependencySelector(ctx, root, "fifo")
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := observeShellValidationCopiedFile(ctx, file, "fifo", 1024)
	_ = file.Close()
	if readErr == nil || time.Since(start) > time.Second {
		t.Fatal("racing FIFO blocked or became regular-file dependency evidence")
	}
	file, err = openShellDependencySelector(ctx, root, "real/target")
	if err != nil {
		t.Fatal(err)
	}
	input, err := observeShellValidationCopiedFile(ctx, file, "real/target", 1024)
	_ = file.Close()
	wantHash := sha256.Sum256([]byte{'a', 0, 0xff, '\n'})
	if err != nil || input.Size != 4 || input.Mode != 0600 || input.ContentHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("actual binary regular-file observation failed: %+v %v", input, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if file, err := openShellDependencySelector(cancelled, root, "real/target"); err == nil {
		_ = file.Close()
		t.Fatal("cancelled selector read was admitted")
	}
}
