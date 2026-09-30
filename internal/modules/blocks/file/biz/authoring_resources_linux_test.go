//go:build linux

package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAuthoringResourceConfinesEveryPathComponent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, "dir", "source.txt")
	if err := os.WriteFile(original, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []struct{ link, target string }{
		{filepath.Join(root, "file-link.txt"), original},
		{filepath.Join(root, "dir-link"), filepath.Join(root, "dir")},
		{filepath.Join(root, "out-link"), outside},
	} {
		if err := os.Symlink(path.target, path.link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(original, filepath.Join(root, "dir", "hard-link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"file-link.txt", "dir-link/source.txt", "out-link/secret.txt", "dir/hard-link.txt", "dir/source.txt", "../other.txt"} {
		if file, err := openAuthoringResource(root, ref); err == nil {
			file.Close()
			t.Errorf("unsafe path %q was opened", ref)
		}
	}
	if err := os.Remove(filepath.Join(root, "dir", "hard-link.txt")); err != nil {
		t.Fatal(err)
	}
	file, err := openAuthoringResource(root, "dir/source.txt")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if file, err := openAuthoringResource(filepath.Join(root, "dir-link"), "source.txt"); err == nil {
		file.Close()
		t.Fatal("symlink workspace root was opened")
	}
	if err := os.Mkdir(filepath.Join(root, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "dir"), filepath.Join(root, "alias", "middle")); err != nil {
		t.Fatal(err)
	}
	if file, err := openAuthoringResource(filepath.Join(root, "alias", "middle"), "source.txt"); err == nil {
		file.Close()
		t.Fatal("symlink in workspace root ancestry was opened")
	}
}

func TestAuthoringResourceVersionRejectsSameInodeRewrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "resource.txt")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openAuthoringResource(root, "resource.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("update"), 0o600); err != nil {
		t.Fatal(err)
	}
	modified := time.Unix(42, 0)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	after, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || sameAuthoringFileVersion(before, after) {
		t.Fatal("same-inode in-place rewrite was not detected")
	}
}
