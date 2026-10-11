package shell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestProjectRawStorePreservesExactBytesAcrossReopen(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "exec/ws/run/original", StageID: "project-test", Stream: "stdout"}
	store, err := openShellProjectRawStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	buffer := newShellProjectRawBuffer()
	_, _ = buffer.Write([]byte("原始输出\r\n\x00\xff\xfe TOKEN=原样"))
	raw := buffer.fact()
	ref, err := store.Store(ctx, scope, raw)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.Store(ctx, scope, raw)
	if err != nil || duplicate != ref {
		t.Fatal("duplicate 未精确复用原文")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openShellProjectRawStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	got, err := store.Read(ctx, scope, ref)
	if err != nil || !reflect.DeepEqual(got, raw) {
		t.Fatalf("fresh store 丢失原 bytes／hash：%v", err)
	}
	got.Raw[0] = 'x'
	fresh, err := store.Read(ctx, scope, ref)
	if err != nil || !bytes.Equal(fresh.Raw, raw.Raw) {
		t.Fatal("读取结果共享 owner 原文")
	}
	for _, field := range []string{"workspace", "run", "execution", "stage", "stream"} {
		wrong := scope
		switch field {
		case "workspace":
			wrong.WorkspaceID = "other"
		case "run":
			wrong.RunID = "other"
		case "execution":
			wrong.ExecutionKey = "other"
		case "stage":
			wrong.StageID = "other"
		case "stream":
			wrong.Stream = "stderr"
		}
		if _, err := store.Read(ctx, wrong, ref); err == nil {
			t.Fatalf("跨 scope 原文读取未拒绝：%s", field)
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			other, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Error(err)
				return
			}
			defer other.Close()
			if actual, err := other.Store(ctx, scope, raw); err != nil || actual != ref {
				t.Errorf("并发原文发布漂移：%v", err)
			}
		})
	}
	wg.Wait()
	path := filepath.Join(directory, shellProjectRawScopeDirectory(scope), ref+".json")
	if err := os.WriteFile(path, []byte("篡改"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, scope, ref); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("篡改原文没有拒绝：%v", err)
	}
	if _, err := store.Store(ctx, scope, raw); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("发布覆盖了已存在的异常原文：%v", err)
	}
}

func TestProjectRawStoreRetainsExplicitIncompleteOutputAndRejectsInvalidFacts(t *testing.T) {
	ctx := context.Background()
	store, err := openShellProjectRawStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "execution", StageID: "stage", Stream: "stderr"}
	buffer := newShellProjectRawBuffer()
	_, _ = buffer.Write(bytes.Repeat([]byte{'x'}, MaxCommandOutputBytes+1))
	raw := buffer.fact()
	ref, err := store.Store(ctx, scope, raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(ctx, scope, ref)
	if err != nil || !reflect.DeepEqual(got, raw) || got.Complete || got.OmittedReason == "" || got.SavedBytes != MaxCommandOutputBytes || got.Bytes != MaxCommandOutputBytes+1 {
		t.Fatal("超界前缀被洗成完整原文或遗漏原因丢失")
	}
	for _, scenario := range []string{"saved", "bytes", "hash", "complete", "reason", "overbound"} {
		bad := raw
		switch scenario {
		case "saved":
			bad.SavedBytes--
		case "bytes":
			bad.Bytes = 0
		case "hash":
			bad.SHA256 = "invalid"
		case "complete":
			bad.Complete = true
		case "reason":
			bad.OmittedReason = ""
		case "overbound":
			bad.Raw = append(append([]byte(nil), raw.Raw...), 'x')
			bad.SavedBytes++
		}
		if _, err := store.Store(ctx, scope, bad); err == nil {
			t.Fatalf("非法 raw identity 未拒绝：%s", scenario)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Store(cancelled, scope, raw); !errors.Is(err, context.Canceled) {
		t.Fatal("取消后仍发布原文")
	}
	for _, bad := range []string{"../escape", strings.ToUpper(ref), strings.Repeat("g", 64), ""} {
		if _, err := store.Read(ctx, scope, bad); err == nil {
			t.Fatal("非 canonical 原文 ref 未拒绝")
		}
	}
}

func TestProjectRawStorePublicationFailureDoesNotAcknowledgeDurability(t *testing.T) {
	for _, scenario := range []string{"before_link", "after_link_sync"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			directory := t.TempDir()
			store, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "exec", StageID: "stage", Stream: "stdout"}
			buffer := newShellProjectRawBuffer()
			_, _ = buffer.Write([]byte("准确原文"))
			raw := buffer.fact()
			failure := errors.New("注入持久化错误")
			if scenario == "before_link" {
				store.linkFile = func(*os.Root, string, string) error { return failure }
			} else {
				store.linkFile = func(root *os.Root, old, new string) error {
					if err := root.Link(old, new); err != nil {
						return err
					}
					store.syncDirectory = func(*os.Root) error { return failure }
					return nil
				}
			}
			if ref, err := store.Store(ctx, scope, raw); ref != "" || !errors.Is(err, failure) {
				t.Fatalf("故障被标成持久成功：ref=%q err=%v", ref, err)
			}
			leaf := filepath.Join(directory, shellProjectRawScopeDirectory(scope))
			entries, err := os.ReadDir(leaf)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "after_link_sync" {
				want = 1
			}
			if len(entries) != want {
				t.Fatal("失败遗留了部分 final file 或临时别名")
			}
			fresh, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			ref, err := fresh.Store(ctx, scope, raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := fresh.Read(ctx, scope, ref)
			if err != nil || !reflect.DeepEqual(got, raw) {
				t.Fatal("重载重试未沿原准确 bytes 完成持久确认")
			}
		})
	}
}

func TestProjectRawStoreCancellationDuringSyncDoesNotAcknowledgeSuccess(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate=%t", duplicate), func(t *testing.T) {
			directory := t.TempDir()
			store, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "exec", StageID: "stage", Stream: "stdout"}
			buffer := newShellProjectRawBuffer()
			_, _ = buffer.Write([]byte("原文同步期间取消"))
			raw := buffer.fact()
			if duplicate {
				if _, err := store.Store(context.Background(), scope, raw); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store.syncDirectory = func(root *os.Root) error {
				if err := syncShellProjectRawDirectory(root); err != nil {
					return err
				}
				entries, err := root.Open(".")
				if err != nil {
					return err
				}
				names, readErr := entries.Readdirnames(-1)
				closeErr := entries.Close()
				if readErr != nil {
					return readErr
				}
				if closeErr != nil {
					return closeErr
				}
				for _, name := range names {
					if strings.HasSuffix(name, ".json") {
						cancel()
					}
				}
				return nil
			}
			if ref, err := store.Store(ctx, scope, raw); ref != "" || !errors.Is(err, context.Canceled) {
				t.Fatalf("同步期间取消仍返回成功引用：ref=%q err=%v", ref, err)
			}
			fresh, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			ref, err := fresh.Store(context.Background(), scope, raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := fresh.Read(context.Background(), scope, ref)
			if err != nil || !reflect.DeepEqual(got, raw) {
				t.Fatal("取消后重试改写或丢失已发布原文")
			}
		})
	}
}

func TestProjectRawStoreRejectsSymlinkAndUnrestrictedRecord(t *testing.T) {
	ctx := context.Background()
	scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "exec", StageID: "stage", Stream: "stdout"}
	buffer := newShellProjectRawBuffer()
	_, _ = buffer.Write(nil)
	raw := buffer.fact()
	for _, scenario := range []string{"owner_directory", "raw_file", "mode", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			directory, outside := t.TempDir(), t.TempDir()
			store, err := openShellProjectRawStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if scenario == "owner_directory" {
				if err := os.Symlink(outside, filepath.Join(directory, "shell")); err != nil {
					t.Fatal(err)
				}
				if _, err := store.Store(ctx, scope, raw); err == nil {
					t.Fatal("owner symlink 未拒绝")
				}
				entries, _ := os.ReadDir(outside)
				if len(entries) != 0 {
					t.Fatal("写出 owner root")
				}
				return
			}
			ref, err := store.Store(ctx, scope, raw)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, shellProjectRawScopeDirectory(scope), ref+".json")
			switch scenario {
			case "raw_file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "secret"), path); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, shellProjectRawStoreMaxBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Read(ctx, scope, ref); err == nil {
				t.Fatal("受限原文读取未拒绝异常文件")
			}
		})
	}
	digest := sha256.Sum256(nil)
	if raw.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("空原文 hash 失真")
	}
}
