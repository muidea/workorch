package contextregistryports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
)

func preparationInstructionFixture(t *testing.T, global bool, id string, raw []byte) (string, string, *preparationStatusRunner) {
	t.Helper()
	directory := t.TempDir()
	root := filepath.Join(directory, ".agents", "skills")
	if global {
		root = filepath.Join(directory, "codex")
		t.Setenv("CODEX_SKILLS_DIR", root)
	}
	path := filepath.Join(root, filepath.FromSlash(id), "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	item := contextregistrycore.SkillHubStatusItem{SkillID: id, LocalPath: path}
	status := contextregistrycore.SkillHubStatus{Scope: "project", ProjectPath: directory, SkillCount: 1, Items: []contextregistrycore.SkillHubStatusItem{item}}
	if global {
		status.Scope, status.ProjectPath = "global", ""
		status.Items[0].Agent, status.Items[0].TargetPath = "codex", filepath.Dir(path)
		directory = ""
	}
	output, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	return directory, root, &preparationStatusRunner{output: output}
}

func TestPreparationSkillHubInstalledInstructionsPreserveBytesAndReobserve(t *testing.T) {
	for _, global := range []bool{true, false} {
		t.Run(fmt.Sprint(global), func(t *testing.T) {
			raw := []byte("---\r\nname: 原始名称\r\n---\r\n\x00原始正文\r\n")
			dir, root, runner := preparationInstructionFixture(t, global, "group/nested", raw)
			value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, global)
			if err != nil || !value.Inventory.InventoryComplete || !value.InstructionsReobserved || len(value.Instructions) != 1 || value.InstalledRoot != root || value.CompletedAt.Before(value.StartedAt) {
				t.Fatalf("incomplete installed observation: %s %v", value.FailureCode, err)
			}
			file := value.Instructions[0]
			if len(value.ReobservedInstructions) != 1 || !bytes.Equal(value.ReobservedInstructions[0].Raw, raw) || !value.ReobservedInstructions[0].Complete {
				t.Fatal("second actual read not retained")
			}
			sum := sha256.Sum256(raw)
			if !file.Complete || !file.Reobserved || file.FailureCode != "" || !bytes.Equal(file.Raw, raw) || file.ReadBytes != uint64(len(raw)) || file.ObservedSize != int64(len(raw)) || file.RawSHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("instruction bytes changed")
			}
			stored, err := os.ReadFile(file.Path)
			if err != nil || !bytes.Equal(stored, raw) || runner.calls != 1 {
				t.Fatal("file changed or status retried")
			}
		})
	}
}

func TestPreparationSkillHubInstalledCaptureUsesOnlyRawWithoutStatusRetry(t *testing.T) {
	dir, _, runner := preparationInstructionFixture(t, false, "s", []byte("原始指令\n"))
	capture, err := ObserveSkillHubPreparation(context.Background(), runner, "ws", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	capture.Status.Items = []contextregistrycore.SkillHubStatusItem{{SkillID: "forged", LocalPath: "/outside"}}
	capture.Status.Orphaned = []contextregistrycore.SkillHubStatusItem{{SkillID: "forged"}}
	capture.InventoryComplete = false
	value, err := ObserveSkillHubInstalledFromCapture(context.Background(), capture)
	if err != nil || runner.calls != 1 || len(value.Instructions) != 1 || value.Instructions[0].SkillID != "s" || len(value.Inventory.Status.Orphaned) != 0 {
		t.Fatal("parsed caller state used or status repeated", err)
	}
	for _, mutate := range []func(*SkillHubPreparationObservation){
		func(v *SkillHubPreparationObservation) { v.RawSHA256 = "wrong" },
		func(v *SkillHubPreparationObservation) { v.ExitCodeKnown = false },
		func(v *SkillHubPreparationObservation) { v.OutputComplete = false },
		func(v *SkillHubPreparationObservation) { v.TotalOutputBytes++ },
		func(v *SkillHubPreparationObservation) { v.StartedAt = v.CompletedAt.Add(1) },
	} {
		bad := capture
		mutate(&bad)
		rejected, err := ObserveSkillHubInstalledFromCapture(context.Background(), bad)
		if err == nil || rejected.InstalledRoot != "" || len(rejected.Instructions) > 0 || runner.calls != 1 {
			t.Fatal("invalid capture reached filesystem or repeated status")
		}
	}
}

func TestPreparationSkillHubInstalledReadNeverFallsBackToSource(t *testing.T) {
	dir, root, runner := preparationInstructionFixture(t, false, "s", []byte("installed"))
	var status contextregistrycore.SkillHubStatus
	if err := json.Unmarshal(runner.output, &status); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(source, []byte("source must not replace installed"), 0600); err != nil {
		t.Fatal(err)
	}
	status.Items[0].SourcePath, status.Items[0].RepoPath = source, source
	runner.output, _ = json.Marshal(status)
	if err := os.Remove(filepath.Join(root, "s", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, false)
	if err == nil || value.InstructionsReobserved || len(value.Instructions) != 1 || len(value.Instructions[0].Raw) != 0 || value.Instructions[0].Complete {
		t.Fatal("missing installed instruction replaced by source")
	}
}

func TestPreparationSkillHubInstalledPathMustMatchIndependentRoot(t *testing.T) {
	for _, scenario := range []string{"outside", "wrong_target", "root_id", "parent_id", "unclean_id", "absolute_id", "backslash_id", "wrong_agent"} {
		t.Run(scenario, func(t *testing.T) {
			dir, _, runner := preparationInstructionFixture(t, false, "s", []byte("raw"))
			var status contextregistrycore.SkillHubStatus
			if err := json.Unmarshal(runner.output, &status); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "outside":
				status.Items[0].LocalPath = filepath.Join(t.TempDir(), "SKILL.md")
			case "wrong_target":
				status.Items[0].TargetPath = "/outside"
			case "parent_id":
				status.Items[0].SkillID = "../s"
			case "root_id":
				status.Items[0].SkillID = "."
			case "unclean_id":
				status.Items[0].SkillID = "x/../s"
			case "absolute_id":
				status.Items[0].SkillID = "/s"
			case "backslash_id":
				status.Items[0].SkillID = `x\s`
			case "wrong_agent":
				status.Items[0].Agent = "codex"
			}
			runner.output, _ = json.Marshal(status)
			value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, false)
			if err == nil || value.InstructionsReobserved || len(value.Instructions) != 0 {
				t.Fatal("unbound instruction path read")
			}
		})
	}
}

func TestPreparationSkillHubInstalledFilesRejectLinksDirectoriesAndOversize(t *testing.T) {
	for _, scenario := range []string{"file_link", "directory_link", "directory", "oversize", "empty", "invalid_utf8"} {
		t.Run(scenario, func(t *testing.T) {
			dir, root, runner := preparationInstructionFixture(t, false, "s", []byte("raw"))
			path := filepath.Join(root, "s", "SKILL.md")
			var wantRaw []byte
			switch scenario {
			case "file_link", "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if scenario == "file_link" {
					outside := filepath.Join(t.TempDir(), "SKILL.md")
					if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "directory_link":
				if err := os.Rename(filepath.Join(root, "s"), filepath.Join(root, "real")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("real", filepath.Join(root, "s")); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, []byte(strings.Repeat("x", SkillHubPreparationInstructionLimit+1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "empty", "invalid_utf8":
				wantRaw = []byte{}
				if scenario == "invalid_utf8" {
					wantRaw = []byte("raw\xff\x00\r\n")
				}
				if err := os.WriteFile(path, wantRaw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, false)
			if err == nil || value.InstructionsReobserved || len(value.Instructions) != 1 || value.Instructions[0].Reobserved {
				t.Fatal("unsafe instruction accepted")
			}
			if wantRaw != nil && (!value.Instructions[0].Complete || !bytes.Equal(value.Instructions[0].Raw, wantRaw)) {
				t.Fatal("invalid original content discarded")
			}
			if len(value.Instructions[0].Raw) > SkillHubPreparationInstructionLimit {
				t.Fatal("diagnostic storage exceeded")
			}
		})
	}
}

func TestPreparationSkillHubInstructionScopeRetainsOtherAgentsAndOrphans(t *testing.T) {
	dir, _, runner := preparationInstructionFixture(t, true, "s", []byte("codex raw"))
	var status contextregistrycore.SkillHubStatus
	if err := json.Unmarshal(runner.output, &status); err != nil {
		t.Fatal(err)
	}
	status.Items = append(status.Items, contextregistrycore.SkillHubStatusItem{SkillID: "s", Agent: "claude", TargetPath: "/not-read"})
	status.Orphaned = []contextregistrycore.SkillHubStatusItem{{SkillID: "orphan", Agent: "codex", TargetPath: "/not-read"}}
	runner.output, _ = json.Marshal(status)
	value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, true)
	if err != nil || !value.InstructionsReobserved || len(value.Instructions) != 1 || value.OtherAgentItemCount != 1 || len(value.Inventory.Status.Items) != 2 || len(value.Inventory.Status.Orphaned) != 1 {
		t.Fatalf("other scope hidden or read: %v", err)
	}
	// Orphans remain facts, not instruction/selection credit.
}

func TestPreparationSkillHubInstructionRootAndCancellationFailClosed(t *testing.T) {
	for _, scenario := range []string{"missing", "root_link", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			dir, root, runner := preparationInstructionFixture(t, true, "s", []byte("raw"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "missing":
				t.Setenv("CODEX_SKILLS_DIR", filepath.Join(t.TempDir(), "absent"))
			case "root_link":
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(root, link); err != nil {
					t.Fatal(err)
				}
				t.Setenv("CODEX_SKILLS_DIR", link)
			case "cancelled":
				cancel()
			}
			value, err := ObserveSkillHubPreparationInstructions(ctx, runner, "ws", dir, true)
			if err == nil || value.InstructionsReobserved || len(value.Instructions) != 0 {
				t.Fatal("unavailable root credited")
			}
			if scenario == "cancelled" && runner.calls != 0 {
				t.Fatal("cancelled request executed status")
			}
		})
	}
}

func TestPreparationSkillHubInstructionBudgetAndFileIdentity(t *testing.T) {
	_, path, _ := preparationInstructionFixture(t, true, "s", []byte("raw"))
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	value, err := readPreparationSkillInstruction(context.Background(), root, filepath.Join("s", "SKILL.md"), "s", 2)
	if err == nil || value.Complete || value.ObservedSize != 3 || len(value.Raw) != 0 {
		t.Fatal("remaining inventory budget exceeded")
	}
	before, err := root.Stat(filepath.Join("s", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "s", "replacement"), []byte("raw"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(path, "s", "replacement"), filepath.Join(path, "s", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	after, err := root.Stat(filepath.Join("s", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if samePreparationInstructionFile(before, after) {
		t.Fatal("equal content replacement treated as same observed file")
	}
}

func TestPreparationSkillHubInstructionInventoryBudgetDoesNotStoreExtraFile(t *testing.T) {
	dir, root, runner := preparationInstructionFixture(t, false, "s", []byte("raw"))
	items := make([]contextregistrycore.SkillHubStatusItem, 0, 129)
	raw := []byte(strings.Repeat("x", SkillHubPreparationInstructionLimit))
	for i := 0; i < 129; i++ {
		id := fmt.Sprintf("s-%03d", i)
		path := filepath.Join(root, id, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		items = append(items, contextregistrycore.SkillHubStatusItem{SkillID: id, LocalPath: path})
	}
	runner.output, _ = json.Marshal(contextregistrycore.SkillHubStatus{Scope: "project", ProjectPath: dir, SkillCount: 129, Items: items})
	value, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, false)
	if err == nil || value.InstructionsReobserved || len(value.Instructions) != 129 || value.Instructions[128].Complete || value.Instructions[128].ReadBytes != 0 {
		t.Fatal("inventory limit not enforced before extra file read")
	}
	total := 0
	for _, file := range value.Instructions {
		total += len(file.Raw)
	}
	if total != SkillHubPreparationInstructionsLimit {
		t.Fatalf("stored bytes=%d", total)
	}
}
