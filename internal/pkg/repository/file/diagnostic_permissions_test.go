package file

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
)

func TestSaveMapPrivatePermissionsReplaceExistingPublicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	defer EvictMapFileCache(path)
	items := map[string]string{"diagnostic": "原始错误\n未经改写"}
	if err := saveMap(path, map[string]string{"before": "public"}); err != nil {
		t.Fatal(err)
	}
	if err := saveMapWithPermissions(path, items, 0o600, commitFileAtomically); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("诊断文件权限 = %o", info.Mode().Perm())
	}
	stored, err := loadMap[string, string](path)
	if err != nil || stored["diagnostic"] != items["diagnostic"] {
		t.Fatalf("原文存储偏差: %v %v", stored, err)
	}
	if err := saveMap(path, map[string]string{"after": "没有诊断"}); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("已收紧权限被重新开放: %v %v", info, err)
	}
}

func TestWorkspaceDiagnosticUsesPrivateAtomicPublish(t *testing.T) {
	for _, preparation := range []bool{false, true} {
		for _, historical := range []bool{false, true} {
			t.Run(map[bool]string{false: "current", true: "history"}[historical]+fmt.Sprint(preparation), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "workspace.json")
				defer EvictMapFileCache(path)
				item := workspace.Workspace{ID: "ws-diagnostic"}
				items := map[common.WorkspaceID]workspace.Workspace{item.ID: item}
				if err := saveWorkspacePersistenceMap(path, items); err != nil {
					t.Fatal(err)
				}
				record := workspace.DefinitionAuthoringRecord{SpecificationRequests: []workspace.DefinitionAuthoringSpecificationRequest{{Diagnostic: &workspace.DefinitionAuthoringDiagnostic{}}}}
				if preparation {
					record = workspace.DefinitionAuthoringRecord{PreparationDiagnostics: []workspace.DefinitionAuthoringPreparationDiagnostic{{}}}
				}
				if historical {
					item.DefinitionAuthoringHistory = []workspace.DefinitionAuthoringConfirmedRevision{{Record: record}}
				} else {
					item.DefinitionAuthoring = &record
				}
				items[item.ID] = item
				if err := saveWorkspacePersistenceMap(path, items); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("Workspace 原始诊断未使用私有权限: %v %v", info, err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := loadWorkspacePersistenceMap(path); err == nil {
					t.Fatal("公开可读的原始诊断文件未拒绝")
				}
			})
		}
	}
}

func TestSaveMapPermissionsRejectBeforeCommit(t *testing.T) {
	called := false
	err := saveMapWithPermissions("unused", map[string]string{}, 0o666, func(string, []byte, os.FileMode) (filecommit.Outcome, error) {
		called = true
		return filecommit.Outcome{}, nil
	})
	if err == nil || called {
		t.Fatalf("非法权限未拒绝: err=%v committed=%v", err, called)
	}
}
