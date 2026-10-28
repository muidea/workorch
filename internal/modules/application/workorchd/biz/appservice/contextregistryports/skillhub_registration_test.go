package contextregistryports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func TestPreparationRegistrationInstalledBinaryOriginalReceipts(t *testing.T) {
	binary := os.Getenv("WORKORCH_TEST_SKILLHUB_BINARY")
	if binary == "" {
		t.Skip("opt-in isolated installed CLI validation")
	}
	root := t.TempDir()
	home := filepath.Join(root, "hub")
	dir := filepath.Join(root, "project")
	bin := filepath.Join(root, "bin")
	for _, v := range []string{filepath.Join(home, "repositories", "main", "skills"), dir, bin} {
		if err := os.MkdirAll(v, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(binary, filepath.Join(bin, "skill-hub")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("SKILL_HUB_HOME", home)
	t.Setenv("SKILL_HUB_DISABLE_SERVICE_BRIDGE", "true")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("multi_repo:\n  enabled: true\n  default_repo: main\n  repositories:\n    main:\n      name: main\n      enabled: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	p := skillhubpreparation.RegistrationPlan{Version: skillhubpreparation.RegistrationVersion, RequestID: "isolated-original", Directory: dir, Home: home}
	first := ObserveSkillHubRegistration(context.Background(), p, false)
	receipt, err := first.Receipt(p, false)
	if err != nil || receipt.Status != "succeeded" || !receipt.Result.ProjectCreated || receipt.Result.SkillCount != 0 {
		t.Fatalf("real registration failed: %+v %v", first, err)
	}
	before, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []bool{true, false, true} {
		o := ObserveSkillHubRegistration(context.Background(), p, query)
		r, err := o.Receipt(p, query)
		if err != nil || r.Status != "succeeded" || !bytes.Equal(first.Stdout, o.Stdout) {
			t.Fatal("original receipt not replayed", err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(home, "state.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("query/replay changed current project state")
	}
	wrong := p
	wrong.Directory = t.TempDir()
	o := ObserveSkillHubRegistration(context.Background(), wrong, false)
	if !o.ExitCodeKnown || o.ExitCode != 1 || !json.Valid(o.Stdout) {
		t.Fatal("request/path conflict not retained")
	}
	if _, err := o.Receipt(wrong, false); err == nil {
		t.Fatal("conflict granted success")
	}
	// A claimed request without a final receipt is unknown, never re-executed.
	sum := sha256.Sum256([]byte("interrupted"))
	claim := filepath.Join(home, "project-registrations", hex.EncodeToString(sum[:])+".json")
	raw, _ := json.Marshal(skillhubpreparation.RegistrationReceipt{RequestID: "interrupted", ProjectPath: dir, Status: "running", Finished: new(bool), StartedAt: "2026-01-01T00:00:00Z"})
	if err := os.WriteFile(claim, raw, 0600); err != nil {
		t.Fatal(err)
	}
	unknown := p
	unknown.RequestID = "interrupted"
	u := ObserveSkillHubRegistration(context.Background(), unknown, true)
	r, err := u.Receipt(unknown, true)
	if err != nil || r.Status != "unknown" || r.Executed != nil || r.Result != nil {
		t.Fatal("interrupted request invented a terminal effect", err)
	}
	missing := p
	missing.RequestID = "missing-original"
	notFound := ObserveSkillHubRegistration(context.Background(), missing, true)
	if _, err := notFound.Receipt(missing, true); err == nil || !bytes.Contains(notFound.Stdout, []byte("REQUEST_NOT_FOUND")) {
		t.Fatal("missing receipt was treated as no effects")
	}
	// Registration creates no Skills; a fresh ordinary project status can now
	// prove an actual empty inventory, rather than inferring it from a directory.
	inventory, err := ObserveSkillHubPreparation(context.Background(), SkillHubPreparationExecRunner{}, "isolated", dir, false)
	if err != nil || !inventory.InventoryComplete || len(inventory.Status.Items) != 0 {
		t.Fatal("registered empty project status unavailable", err)
	}
	t.Setenv("SKILL_HUB_HOME", t.TempDir())
	drift := ObserveSkillHubRegistration(context.Background(), p, true)
	if drift.CommandStarted || drift.Validate() != nil {
		t.Fatal("scope drift executed against another Hub home")
	}
}
