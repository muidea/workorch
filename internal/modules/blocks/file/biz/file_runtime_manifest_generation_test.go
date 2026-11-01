package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceManifestRetainsSubsecondRevisionOnContentRollback(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "target.txt")
	adapter := NewRuntimeAdapter()
	write := func(content string, nanos int64) string {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(1700000000, nanos)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		m, err := adapter.AcquireWorkspaceManifest(directory)
		if err != nil {
			t.Fatal(err)
		}
		return m.Revision
	}
	a, b, returned := write("original", 100), write("changed", 200), write("original", 300)
	if a == b || a == returned || b == returned {
		t.Fatal("subsecond legal rollback reused an old tree revision")
	}
	reopened, err := NewRuntimeAdapter().AcquireWorkspaceManifest(directory)
	if err != nil || reopened.Revision != returned {
		t.Fatal("fresh adapter lost exact tree revision", err)
	}
}
