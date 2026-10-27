package contextregistryports

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Explicit opt-in, no machine SkillHub configuration or project is used. This
// checks a real installed CLI, separately from deterministic shell fixtures.
func TestPreparationSkillHubInstalledBinaryProjectStatusEffects(t *testing.T) {
	binary := os.Getenv("WORKORCH_TEST_SKILLHUB_BINARY")
	if binary == "" {
		t.Skip("set WORKORCH_TEST_SKILLHUB_BINARY for isolated real CLI verification")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("test binary must be absolute")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "project")
	hubRoot := filepath.Join(root, "hub")
	bin := filepath.Join(root, "bin")
	for _, path := range []string{bin, hubRoot, filepath.Join(directory, ".agents", "skills", "s"), filepath.Join(hubRoot, "repositories", "main", "skills", "s")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(binary, filepath.Join(bin, "skill-hub")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("SKILL_HUB_HOME", hubRoot)
	// The production adapter does not force this flag. Isolated validation
	// selects SkillHub's existing local mode to avoid the real daemon/config.
	t.Setenv("SKILL_HUB_DISABLE_SERVICE_BRIDGE", "true")
	config := []byte("multi_repo:\n  enabled: true\n  default_repo: main\n  repositories:\n    main:\n      name: main\n      enabled: true\n")
	if err := os.WriteFile(filepath.Join(hubRoot, "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	raw := []byte("---\nname: Isolated\nversion: 1.2.3\n---\nNon-sensitive isolated instruction\n")
	installed := filepath.Join(directory, ".agents", "skills", "s", "SKILL.md")
	source := filepath.Join(hubRoot, "repositories", "main", "skills", "s", "SKILL.md")
	for _, path := range []string{installed, source} {
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := map[string]any{directory: map[string]any{"project_path": directory, "preferred_target": "open_code", "skills": map[string]any{"s": map[string]any{"skill_id": "s", "version": "0.0.0", "status": "missing", "source_repository": "main", "variables": map[string]string{}}}}}
	before, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(hubRoot, "state.json")
	if err := os.WriteFile(statePath, before, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := ObserveSkillHubPreparationCatalog(context.Background(), SkillHubPreparationExecRunner{}, "isolated-ws", directory, false)
	value := catalog.Observation
	if err != nil {
		t.Fatalf("actual binary project inspection failed: %s / %s: %v", value.FailureCode, value.Inventory.FailureCode, err)
	}
	if !value.Inventory.CommandStarted || !value.Inventory.InventoryComplete || value.Inventory.Status.Scope != "project" || !value.InstructionsReobserved || len(value.Instructions) != 1 || !bytes.Equal(value.Instructions[0].Raw, raw) {
		t.Fatal("actual CLI scope or instruction contract mismatch")
	}
	if len(catalog.Skills) != 1 || len(catalog.Metadata) != 1 || catalog.Metadata[0].Name != "Isolated" || catalog.Skills[0].Instruction != string(raw) || catalog.Skills[0].Version != "1.2.3" {
		t.Fatal("real project status and installed metadata did not map exactly")
	}
	after, err := os.ReadFile(statePath)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("actual status persistence effect not observed")
	}
	var saved struct {
		Skills map[string]struct{ Version, Status string } `json:"skills"`
	}
	var projects map[string]json.RawMessage
	if err := json.Unmarshal(after, &projects); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(projects[directory], &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Skills["s"].Version != "1.2.3" || saved.Skills["s"].Status != value.Inventory.Status.Items[0].Status {
		t.Fatal("status save differs from actual result")
	}
	for _, path := range []string{installed, source} {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, raw) {
			t.Fatal("inspection changed instruction files")
		}
	}
	t.Logf("actual project status: scope=%s items=%d status=%s; state persisted, installed/source instruction bytes unchanged", value.Inventory.Status.Scope, len(value.Inventory.Status.Items), value.Inventory.Status.Items[0].Status)
	savePreparationBinaryEvidence(t, "project", value)
	// Global reports one skill but two agent rows. No apply/use is run: the
	// unmanaged Codex copy remains a real conflict, not readiness success.
	codexRoot := filepath.Join(root, "codex")
	t.Setenv("CODEX_SKILLS_DIR", codexRoot)
	t.Setenv("CLAUDE_SKILLS_DIR", filepath.Join(root, "absent-claude"))
	t.Setenv("OPENCODE_SKILLS_DIR", filepath.Join(root, "absent-opencode"))
	if err := os.MkdirAll(filepath.Join(codexRoot, "s"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexRoot, "s", "SKILL.md"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	globalState := []byte(`{"enabled_skills":{"s":{"skill_id":"s","version":"1.2.3","source_repository":"main","agents":["codex","claude"]}}}`)
	if err := os.WriteFile(filepath.Join(hubRoot, "global-state.json"), globalState, 0600); err != nil {
		t.Fatal(err)
	}
	globalCatalog, err := ObserveSkillHubPreparationCatalog(context.Background(), SkillHubPreparationExecRunner{}, "isolated-ws", "", true)
	global := globalCatalog.Observation
	if err != nil || !global.Inventory.InventoryComplete || global.Inventory.Status.SkillCount != 1 || len(global.Inventory.Status.Items) != 2 || !global.InstructionsReobserved || len(global.Instructions) != 1 || global.OtherAgentItemCount != 1 {
		t.Fatalf("actual multi-agent global protocol mismatch: %v", err)
	}
	for _, item := range global.Inventory.Status.Items {
		if item.Agent == "codex" && (item.Status != "conflict" || item.Reason != "target_unmanaged") {
			t.Fatal("real unmanaged conflict was hidden")
		}
	}
	if len(globalCatalog.Skills) != 1 || globalCatalog.Skills[0].Status != "conflict" || globalCatalog.Skills[0].Instruction != string(raw) || globalCatalog.Skills[0].Agent != "codex" {
		t.Fatal("metadata mapping hid real unmanaged conflict or included another agent")
	}
	t.Logf("actual global status: scope=%s skills=%d rows=%d; unmanaged Codex conflict retained, other agent not read", global.Inventory.Status.Scope, global.Inventory.Status.SkillCount, len(global.Inventory.Status.Items))
	savePreparationBinaryEvidence(t, "global", global)
}

func TestPreparationSkillHubInstalledBinaryUsesConfiguredService(t *testing.T) {
	binary := os.Getenv("WORKORCH_TEST_SKILLHUB_BINARY")
	if binary == "" {
		t.Skip("set WORKORCH_TEST_SKILLHUB_BINARY for isolated real CLI verification")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("test binary must be absolute")
	}
	directory, _, fixture := preparationInstructionFixture(t, false, "s", []byte("isolated installed bytes"))
	bin := t.TempDir()
	if err := os.Symlink(binary, filepath.Join(bin, "skill-hub")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("SKILL_HUB_HOME", t.TempDir()) // no local config; fallback must fail
	t.Setenv("SKILL_HUB_DISABLE_SERVICE_BRIDGE", "false")
	t.Setenv("SKILL_HUB_SERVICE_SECRET_KEY", "")
	var health, status, unexpected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/health":
			health.Add(1)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/project-status" && r.URL.Query().Get("path") == directory && r.URL.Query().Get("skill_id") == "":
			status.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(append(append([]byte(`{"code":"OK","data":{"item":`), fixture.output...), []byte(`}}`)...))
		default:
			unexpected.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("SKILL_HUB_SERVICE_URL", server.URL)
	catalog, err := ObserveSkillHubPreparationCatalog(context.Background(), SkillHubPreparationExecRunner{}, "isolated-ws", directory, false)
	value := catalog.Observation
	if err != nil || !value.Inventory.CommandStarted || !value.InstructionsReobserved || health.Load() != 1 || status.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("configured service was bypassed, repeated or hidden: health=%d status=%d unexpected=%d err=%v", health.Load(), status.Load(), unexpected.Load(), err)
	}
	if len(catalog.Skills) != 1 || catalog.Skills[0].Instruction != "isolated installed bytes" || catalog.Skills[0].Name != "s" {
		t.Fatal("headerless installed instruction was not preserved through service-backed status")
	}
	t.Log("real CLI performed one health query and one project-status query against an isolated loopback HTTP fixture; no local fallback/config")
	savePreparationBinaryEvidence(t, "service", value)
}

func savePreparationBinaryEvidence(t *testing.T, label string, value SkillHubPreparationInstructions) {
	t.Helper()
	directory := os.Getenv("WORKORCH_TEST_SKILLHUB_EVIDENCE_DIR")
	if directory == "" {
		return
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == string(filepath.Separator) {
		t.Fatal("evidence requires an existing isolated absolute directory")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("evidence directory is unavailable")
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for suffix, raw := range map[string][]byte{"stdout.bin": value.Inventory.RawOutput, "stderr.bin": value.Inventory.RawStderr, "observation.json": encoded} {
		if err := os.WriteFile(filepath.Join(directory, label+"."+suffix), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
