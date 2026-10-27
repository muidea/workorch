package contextregistryports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type preparationStatusRunner struct {
	output    []byte
	err       error
	calls     int
	directory string
	global    bool
	retained  []byte
}

func (r *preparationStatusRunner) ObserveStatus(ctx context.Context, directory string, global bool) (SkillHubPreparationOutput, error) {
	r.calls++
	r.directory, r.global = directory, global
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		return SkillHubPreparationOutput{}, errors.New("missing operation budget")
	}
	r.retained = bytes.Clone(r.output)
	return SkillHubPreparationOutput{Bytes: r.retained, TotalBytes: uint64(len(r.output)), CommandStarted: true, ExitCodeKnown: true}, r.err
}

func TestPreparationSkillHubInventoryPreservesWholeStatusWithoutReadingInstructions(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, ".agents", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, global := range []bool{true, false} {
		t.Run(fmt.Sprint(global), func(t *testing.T) {
			dir, header := "", `"scope":"global"`
			if !global {
				dir, header = directory, fmt.Sprintf(`"scope":"project","project_path":%q`, directory)
			}
			agent := ""
			if global {
				agent = `,"agent":"codex"`
			}
			raw := []byte(fmt.Sprintf("{\n%s,\"skill_count\":1,\"extra_original_field\":{\"v\":7},\"items\":[{\"skill_id\":\"s\"%s,\"version\":\"v1\",\"status\":\"unknown\",\"target_path\":\"/does/not/exist\",\"actual_hash\":\"opaque-owner-hash\"}]}\n", header, agent))
			runner := &preparationStatusRunner{output: raw}
			value, err := ObserveSkillHubPreparation(context.Background(), runner, "ws", dir, global)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(raw)
			if !value.InventoryComplete || !value.OutputComplete || value.FailureCode != "" || !bytes.Equal(value.RawOutput, raw) || value.RawSHA256 != hex.EncodeToString(sum[:]) || value.TotalOutputBytes != uint64(len(raw)) || value.CompletedAt.Before(value.StartedAt) {
				t.Fatal("lost whole status or original output")
			}
			if runner.calls != 1 || runner.directory != dir || runner.global != global || value.Status.Items[0].ActualHash != "opaque-owner-hash" || value.Status.Items[0].Status != "unknown" {
				t.Fatal("changed owner data or operation")
			}
			runner.retained[0] = '!'
			if value.RawOutput[0] != '{' {
				t.Fatal("output aliases runner")
			}
		})
	}
}

func TestPreparationSkillHubRejectsPartialAmbiguousAndFailedStatus(t *testing.T) {
	for _, tc := range []struct {
		name, raw, code string
		err             error
	}{
		{"missing_command", "diagnostic\x00\xff\n", "command_failed", exec.ErrNotFound},
		{"failed_command", `{"scope":"global","items":[]}`, "command_failed", errors.New("exit status 2")},
		{"missing_items", `{"scope":"global"}`, "invalid_status", nil},
		{"null_items", `{"scope":"global","items":null}`, "invalid_status", nil},
		{"wrong_scope", `{"scope":"workspace","items":[]}`, "invalid_status", nil},
		{"count_mismatch", `{"scope":"global","skill_count":2,"items":[]}`, "invalid_status", nil},
		{"duplicate_id", `{"scope":"global","items":[{"skill_id":"s","agent":"codex"},{"skill_id":"s","agent":"codex"}]}`, "duplicate_skill_identity", nil},
		{"missing_agent", `{"scope":"global","items":[{"skill_id":"s"}]}`, "invalid_skill_identity", nil},
		{"duplicate_agent", `{"scope":"global","items":[],"agents":[{"name":"codex"},{"name":"codex"}]}`, "duplicate_agent_identity", nil},
		{"duplicate_orphan", `{"scope":"global","items":[{"skill_id":"s","agent":"codex"}],"orphaned":[{"skill_id":"s","agent":"codex"}]}`, "duplicate_skill_identity", nil},
		{"blank_id", `{"scope":"global","items":[{"skill_id":" "}]}`, "invalid_skill_identity", nil},
		{"duplicate_key", `{"scope":"global","items":[{"skill_id":"a","skill_id":"b"}]}`, "invalid_status", nil},
		{"case_alias", `{"scope":"global","items":[],"Items":[]}`, "invalid_status", nil},
		{"trailing_document", `{"scope":"global","items":[]} {}`, "invalid_status", nil},
		{"invalid_utf8", "{\"scope\":\"global\",\"items\":[],\"x\":\"\xff\"}", "invalid_status", nil},
		{"lone_surrogate", `{"scope":"global","items":[{"skill_id":"\ud800"}]}`, "invalid_status", nil},
		{"lone_low_surrogate", `{"scope":"global","items":[],"diagnostic":"\udfff"}`, "invalid_status", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &preparationStatusRunner{output: []byte(tc.raw), err: tc.err}
			value, err := ObserveSkillHubPreparation(context.Background(), runner, "ws", "", true)
			if err == nil || value.InventoryComplete || value.FailureCode != tc.code || runner.calls != 1 || !bytes.Equal(value.RawOutput, []byte(tc.raw)) {
				t.Fatalf("failure not preserved: %s %v", value.FailureCode, err)
			}
		})
	}
	value, err := ObserveSkillHubPreparation(context.Background(), &preparationStatusRunner{output: []byte(`{"scope":"global","items":[]}`)}, "ws", "", true)
	if err != nil || !value.InventoryComplete || len(value.Status.Items) != 0 {
		t.Fatal("actual empty inventory rejected")
	}
}

func TestPreparationSkillHubPreservesValidEscapedUnicode(t *testing.T) {
	raw := []byte(`{"scope":"global","items":[{"skill_id":"\ud83d\ude00","agent":"codex","message":"\\ud800\\\"原文"}]}`)
	value, err := ObserveSkillHubPreparation(context.Background(), &preparationStatusRunner{output: raw}, "ws", "", true)
	if err != nil || !value.InventoryComplete || value.Status.Items[0].SkillID != "😀" || !bytes.Equal(value.RawOutput, raw) {
		t.Fatal("valid reversible Unicode rejected or changed")
	}
}

func TestPreparationSkillHubDirectoryAndCancellationNeverBecomeEmptyInventory(t *testing.T) {
	for _, scenario := range []string{"missing", "file", "dangling", "wrong_project", "cancelled", "invalid_scope", "nil_runner"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, ".agents", "skills")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "file":
				if err := os.WriteFile(path, []byte("not directory"), 0600); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				if err := os.Symlink(filepath.Join(directory, "absent"), path); err != nil {
					t.Fatal(err)
				}
			case "missing":
			default:
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			runner := &preparationStatusRunner{output: []byte(`{"project_path":"/other","items":[]}`)}
			var port SkillHubPreparationRunner = runner
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "invalid_scope" {
				directory = "relative"
			}
			if scenario == "nil_runner" {
				port = nil
			}
			before, _ := os.ReadDir(filepath.Dir(path))
			value, err := ObserveSkillHubPreparation(ctx, port, "ws", directory, false)
			if err == nil || value.InventoryComplete {
				t.Fatal("unavailable scope became empty inventory")
			}
			wantCalls := 0
			if scenario == "wrong_project" {
				wantCalls = 1
			}
			if runner.calls != wantCalls {
				t.Fatal("unexpected command or retry")
			}
			after, _ := os.ReadDir(filepath.Dir(path))
			if !reflect.DeepEqual(before, after) {
				t.Fatal("observation wrote directory")
			}
		})
	}
}

func TestPreparationSkillHubExecUsesCurrentArgumentsAndPreservesStatusEffects(t *testing.T) {
	bin := t.TempDir()
	path := filepath.Join(bin, "skill-hub")
	marker := filepath.Join(bin, "status-state")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'warning\\000\\377\\n' >&2\nif [ \"$#\" = 3 ] && [ \"$3\" = --global ]; then\nprintf '{\"scope\":\"global\",\"items\":[]}'\nelse\nprintf '{\"scope\":\"project\",\"project_path\":\"%%s\",\"items\":[]}' \"$PWD\"\nfi\n", marker)
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, ".agents", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, global := range []bool{true, false} {
		dir, wantArgs := "", "status\n--json\n--global\n"
		if !global {
			dir, wantArgs = directory, "status\n--json\n"
		}
		value, err := ObserveSkillHubPreparation(context.Background(), SkillHubPreparationExecRunner{}, "ws", dir, global)
		if err != nil || !value.CommandStarted || !value.OutputComplete || !value.StderrComplete || !value.InventoryComplete || value.EffectContract != "skillhub-status-effects/v1" || !bytes.Equal(value.RawStderr, []byte("warning\x00\xff\n")) {
			t.Fatalf("current status effects or separate diagnostics lost: %v", err)
		}
		args, err := os.ReadFile(marker)
		if err != nil || string(args) != wantArgs {
			t.Fatalf("unsupported flags or wrong directory: %q %v", args, err)
		}
		sum := sha256.Sum256(value.RawStderr)
		if value.StderrSHA256 != hex.EncodeToString(sum[:]) || value.TotalStderrBytes != uint64(len(value.RawStderr)) {
			t.Fatal("stderr identity lost")
		}
	}
}

func TestPreparationSkillHubOutputBoundAndUnavailableAdapter(t *testing.T) {
	// No installed skill-hub, home catalog, network or live config is read.
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := (SkillHubPreparationExecRunner{}).ObserveStatus(ctx, "", true)
	if !errors.Is(err, exec.ErrNotFound) || output.TotalBytes != 0 || output.CommandStarted {
		t.Fatal("missing adapter concealed")
	}
	if _, err := (SkillHubPreparationExecRunner{}).ObserveStatus(context.Background(), "", true); err == nil {
		t.Fatal("unbounded process accepted")
	}
	value, err := ObserveSkillHubPreparation(context.Background(), &preparationStatusRunner{output: []byte(strings.Repeat("x", SkillHubPreparationOutputLimit+1))}, "ws", "", true)
	if err == nil || value.OutputComplete || value.InventoryComplete || value.FailureCode != "output_incomplete" || len(value.RawOutput) != SkillHubPreparationOutputLimit || value.TotalOutputBytes != SkillHubPreparationOutputLimit+1 {
		t.Fatal("over-limit output accepted")
	}
}

func TestPreparationSkillHubStatusCaptureIsBoundedAndDetached(t *testing.T) {
	w := &preparationStatusCapture{}
	raw := []byte(strings.Repeat("x", SkillHubPreparationOutputLimit+73))
	if n, err := w.Write(raw); err != nil || n != len(raw) {
		t.Fatal("capture stopped draining")
	}
	stored, total := w.snapshot()
	if len(stored) != SkillHubPreparationOutputLimit || total != uint64(len(raw)) || !bytes.Equal(stored, raw[:len(stored)]) {
		t.Fatal("capture lost prefix or total")
	}
	stored[0] = '!'
	second, _ := w.snapshot()
	if second[0] != 'x' {
		t.Fatal("capture aliases diagnostic consumer")
	}
}

func TestPreparationSkillHubExecCancelledBeforeStartHasNoEffect(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "started")
	if err := os.WriteFile(filepath.Join(bin, "skill-hub"), []byte(fmt.Sprintf("#!/bin/sh\nprintf started > %q\n", marker)), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	value, err := ObserveSkillHubPreparation(ctx, SkillHubPreparationExecRunner{}, "ws", "", true)
	if !errors.Is(err, context.Canceled) || value.CommandStarted || value.InventoryComplete {
		t.Fatal("cancelled request started status")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled command had effects")
	}
}

func TestPreparationSkillHubExecFailureAndDeadlineKeepOriginalStreams(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	path := filepath.Join(bin, "skill-hub")
	for _, timeout := range []bool{false, true} {
		tail := "exit 2\n"
		if timeout {
			tail = "exec /bin/sleep 2\n"
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'raw\\000\\377\\n'\nprintf 'diagnostic\\000\\377\\n' >&2\n"+tail), 0700); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		start := time.Now()
		value, err := ObserveSkillHubPreparation(ctx, SkillHubPreparationExecRunner{}, "ws", "", true)
		cancel()
		code := "command_failed"
		if timeout {
			code = "command_incomplete"
		}
		if err == nil || !value.CommandStarted || value.InventoryComplete || value.FailureCode != code || !bytes.Equal(value.RawOutput, []byte("raw\x00\xff\n")) || !bytes.Equal(value.RawStderr, []byte("diagnostic\x00\xff\n")) || time.Since(start) > time.Second {
			t.Fatalf("failed status was hidden or retried: %s %v", value.FailureCode, err)
		}
		if timeout && (value.OutputComplete || value.StderrComplete) {
			t.Fatal("terminated streams called complete")
		}
		if !timeout && (!value.ExitCodeKnown || value.ExitCode != 2) {
			t.Fatal("known nonzero exit lost")
		}
	}
}

type preparationOutputRunner struct {
	output SkillHubPreparationOutput
	err    error
}

func (r preparationOutputRunner) ObserveStatus(context.Context, string, bool) (SkillHubPreparationOutput, error) {
	return r.output, r.err
}

func TestPreparationSkillHubIncompleteDiagnosticsCannotEraseKnownFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		runner := preparationOutputRunner{output: SkillHubPreparationOutput{Bytes: []byte(`{"scope":"global","items":[]}`), Stderr: []byte(strings.Repeat("d", SkillHubPreparationOutputLimit+1)), CommandStarted: true, ExitCodeKnown: true}}
		runner.output.TotalBytes = uint64(len(runner.output.Bytes))
		runner.output.TotalStderrBytes = uint64(len(runner.output.Stderr))
		code := "output_incomplete"
		if failed {
			runner.output.ExitCode = 2
			runner.err = errors.New("exit status 2")
			code = "command_failed"
		}
		value, err := ObserveSkillHubPreparation(context.Background(), runner, "ws", "", true)
		if err == nil || value.InventoryComplete || value.StderrComplete || value.FailureCode != code || len(value.RawStderr) != SkillHubPreparationOutputLimit || value.TotalStderrBytes != SkillHubPreparationOutputLimit+1 || !value.ExitCodeKnown || value.ExitCode != runner.output.ExitCode {
			t.Fatal("partial diagnostic became PASS or erased known exit")
		}
	}
}

func TestPreparationSkillHubGlobalCountIsUniqueSkillsNotAgentRows(t *testing.T) {
	raw := []byte(`{"scope":"global","skill_count":1,"agents":[{"name":"codex","skills_dir":"/codex","detected":true},{"name":"claude","skills_dir":"/claude","configured":true}],"items":[{"skill_id":"s","agent":"codex","status":"synced","reason":"source_and_target_match"},{"skill_id":"s","agent":"claude","status":"modified"}],"orphaned":[{"skill_id":"orphan","agent":"codex","status":"orphaned"}],"generated_at":"original-time"}`)
	value, err := ObserveSkillHubPreparation(context.Background(), &preparationStatusRunner{output: raw}, "ws", "", true)
	if err != nil || !value.InventoryComplete || value.Status.SkillCount != 1 || len(value.Status.Items) != 2 || len(value.Status.Agents) != 2 || len(value.Status.Orphaned) != 1 || value.Status.GeneratedAt != "original-time" || value.Status.Items[0].Reason != "source_and_target_match" || !bytes.Equal(value.RawOutput, raw) {
		t.Fatalf("legitimate multi-agent inventory lost: %v", err)
	}
}

func TestPreparationSkillHubRejectsExcessiveStatusDepthAndInventory(t *testing.T) {
	items := make([]string, 1025)
	for i := range items {
		items[i] = fmt.Sprintf(`{"skill_id":"s-%d"}`, i)
	}
	for _, raw := range []string{
		`{"scope":"global","items":[` + strings.Join(items, ",") + `]}`,
		`{"scope":"global","items":[],"unknown":` + strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34) + `}`,
	} {
		value, err := ObserveSkillHubPreparation(context.Background(), &preparationStatusRunner{output: []byte(raw)}, "ws", "", true)
		if err == nil || value.InventoryComplete || value.FailureCode != "invalid_status" || !bytes.Equal(value.RawOutput, []byte(raw)) {
			t.Fatal("oversized status accepted or original lost")
		}
	}
}
