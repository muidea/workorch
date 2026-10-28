package contextregistryports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

const SkillHubPreparationOutputLimit = 1 << 20

// SkillHubPreparationOutput retains the exact captured prefix and the total
// bytes read from stdout/stderr. A prefix is never presented as a complete
// status document; a terminated process cannot prove unobserved output absent.
type SkillHubPreparationOutput struct {
	Bytes            []byte
	TotalBytes       uint64
	Stderr           []byte
	TotalStderrBytes uint64
	CommandStarted   bool
	ExitCodeKnown    bool
	ExitCode         int
}

// This Application-private adapter follows SkillHub's existing status effects:
// it may contact its configured service and persist project inspection state.
// It is explicit work, never a GET/refresh, and must not install/sync/use/repair.
type SkillHubPreparationRunner interface {
	ObserveStatus(context.Context, string, bool) (SkillHubPreparationOutput, error)
}

type SkillHubPreparationExecRunner struct{}

func (SkillHubPreparationExecRunner) ObserveStatus(ctx context.Context, directory string, global bool) (SkillHubPreparationOutput, error) {
	if ctx == nil {
		return SkillHubPreparationOutput{}, common.ErrInvalidArgument
	}
	if _, ok := ctx.Deadline(); !ok {
		return SkillHubPreparationOutput{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return SkillHubPreparationOutput{}, err
	}
	if (global && directory != "") || (!global && (!filepath.IsAbs(directory) || filepath.Clean(directory) != directory)) {
		return SkillHubPreparationOutput{}, common.ErrInvalidArgument
	}
	args := []string{"status", "--json"}
	if global {
		args = append(args, "--global")
	}
	cmd := exec.CommandContext(ctx, "skill-hub", args...)
	if !global {
		cmd.Dir = directory
	}
	cmd.WaitDelay = 100 * time.Millisecond
	stdout, stderr := &preparationStatusCapture{}, &preparationStatusCapture{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return SkillHubPreparationOutput{}, err
	}
	err := cmd.Wait()
	out, outTotal := stdout.snapshot()
	diagnostic, diagnosticTotal := stderr.snapshot()
	value := SkillHubPreparationOutput{Bytes: out, TotalBytes: outTotal, Stderr: diagnostic, TotalStderrBytes: diagnosticTotal, CommandStarted: true}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		value.ExitCodeKnown = true
		value.ExitCode = cmd.ProcessState.ExitCode()
	}
	return value, err
}

// Drain both streams without blocking the child or retaining unbounded bytes.
// Total counts only bytes actually read; termination cannot prove missing bytes.
type preparationStatusCapture struct {
	mu    sync.Mutex
	raw   []byte
	total uint64
}

func (w *preparationStatusCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.total += uint64(len(p))
	remaining := SkillHubPreparationOutputLimit - len(w.raw)
	if remaining > len(p) {
		remaining = len(p)
	}
	w.raw = append(w.raw, p[:remaining]...)
	return len(p), nil
}

func (w *preparationStatusCapture) snapshot() ([]byte, uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.raw), w.total
}

// This is restricted diagnostic data, not an HTTP DTO or readiness proof.
// InventoryComplete means a whole, unambiguous status inventory was observed.
// It does not establish instruction integrity, selection, freshness or readiness.
type SkillHubPreparationObservation struct {
	Version           string
	WorkspaceID       common.WorkspaceID
	Directory         string
	Scope             string
	StartedAt         time.Time
	CompletedAt       time.Time
	RawOutput         []byte
	RawSHA256         string
	TotalOutputBytes  uint64
	OutputComplete    bool
	InventoryComplete bool
	FailureCode       string
	Status            contextregistrycore.SkillHubStatus
	RawStderr         []byte
	StderrSHA256      string
	TotalStderrBytes  uint64
	StderrComplete    bool
	CommandStarted    bool
	EffectContract    string
	ExitCodeKnown     bool
	ExitCode          int
}

func ObserveSkillHubPreparation(ctx context.Context, runner SkillHubPreparationRunner, workspaceID common.WorkspaceID, directory string, global bool) (SkillHubPreparationObservation, error) {
	observation := SkillHubPreparationObservation{Version: "workorch-skillhub-preparation-observation/v2", EffectContract: "skillhub-status-effects/v1", WorkspaceID: workspaceID, Directory: directory, Scope: contextregistrycore.SkillScopeWorkspace, StartedAt: time.Now().UTC()}
	if global {
		observation.Scope = contextregistrycore.SkillScopeGlobal
	}
	fail := func(code string, err error) (SkillHubPreparationObservation, error) {
		observation.FailureCode = code
		observation.CompletedAt = time.Now().UTC()
		return observation, err
	}
	if ctx == nil || runner == nil || workspaceID == "" || (!global && (!filepath.IsAbs(directory) || filepath.Clean(directory) != directory)) || (global && directory != "") {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if !global {
		// Absence, dangling links and non-directories are not empty inventories.
		info, err := os.Stat(filepath.Join(directory, ".agents", "skills"))
		if err != nil {
			return fail("workspace_catalog_unavailable", err)
		}
		if !info.IsDir() {
			return fail("workspace_catalog_unavailable", common.ErrConflict)
		}
	}
	output, err := runner.ObserveStatus(bounded, directory, global)
	observation.CommandStarted = output.CommandStarted
	observation.ExitCodeKnown, observation.ExitCode = output.ExitCodeKnown, output.ExitCode
	captured := output.Bytes
	if len(captured) > SkillHubPreparationOutputLimit {
		captured = captured[:SkillHubPreparationOutputLimit]
	}
	observation.RawOutput = bytes.Clone(captured)
	observation.TotalOutputBytes = output.TotalBytes
	sum := sha256.Sum256(observation.RawOutput)
	observation.RawSHA256 = hex.EncodeToString(sum[:])
	diagnostic := output.Stderr
	if len(diagnostic) > SkillHubPreparationOutputLimit {
		diagnostic = diagnostic[:SkillHubPreparationOutputLimit]
	}
	observation.RawStderr = bytes.Clone(diagnostic)
	observation.TotalStderrBytes = output.TotalStderrBytes
	diagnosticSum := sha256.Sum256(observation.RawStderr)
	observation.StderrSHA256 = hex.EncodeToString(diagnosticSum[:])
	observation.StderrComplete = uint64(len(output.Stderr)) == output.TotalStderrBytes && len(output.Stderr) <= SkillHubPreparationOutputLimit
	observation.OutputComplete = uint64(len(output.Bytes)) == output.TotalBytes && len(output.Bytes) <= SkillHubPreparationOutputLimit
	if errors.Is(err, exec.ErrWaitDelay) || bounded.Err() != nil {
		observation.OutputComplete = false
		observation.StderrComplete = false
		if output.ExitCodeKnown && output.ExitCode != 0 {
			return fail("command_failed", errors.Join(err, common.ErrConflict))
		}
		return fail("command_incomplete", errors.Join(err, common.ErrConflict))
	}
	if !observation.OutputComplete || !observation.StderrComplete {
		if output.ExitCodeKnown && output.ExitCode != 0 {
			return fail("command_failed", errors.Join(err, common.ErrConflict))
		}
		return fail("output_incomplete", errors.Join(err, common.ErrConflict))
	}
	if err != nil || (output.ExitCodeKnown && output.ExitCode != 0) {
		if !global && output.CommandStarted && output.ExitCodeKnown && output.ExitCode == 1 && skillhubpreparation.ProjectNotFound(output.Bytes) {
			return fail("project_not_registered", errors.Join(err, common.ErrConflict))
		}
		return fail("command_failed", errors.Join(err, common.ErrConflict))
	}
	if err := bounded.Err(); err != nil {
		return fail("cancelled", err)
	}
	if !skillhubpreparation.ValidStatusDocument(output.Bytes) {
		return fail("invalid_status", common.ErrConflict)
	}
	var document struct {
		contextregistrycore.SkillHubStatus
		Count *int `json:"skill_count"`
	}
	if err := json.Unmarshal(output.Bytes, &document); err != nil {
		return fail("invalid_status", err)
	}
	status := document.SkillHubStatus
	if status.Items == nil || len(status.Items)+len(status.Orphaned) > 1024 || len(status.Agents) > 32 ||
		(global && status.Scope != contextregistrycore.SkillScopeGlobal) ||
		(!global && (status.ProjectPath != directory || status.Scope != "project")) {
		return fail("invalid_status", common.ErrConflict)
	}
	seen := make(map[preparationSkillHubIdentity]struct{}, len(status.Items))
	skillIDs := make(map[string]struct{}, len(status.Items))
	for _, item := range status.Items {
		if !validPreparationStatusIdentity(item.SkillID) || (global && !validPreparationStatusIdentity(item.Agent)) || (!global && item.Agent != "") {
			return fail("invalid_skill_identity", common.ErrConflict)
		}
		key := preparationSkillHubIdentity{item.SkillID, item.Agent}
		if _, duplicate := seen[key]; duplicate {
			return fail("duplicate_skill_identity", common.ErrConflict)
		}
		seen[key] = struct{}{}
		skillIDs[item.SkillID] = struct{}{}
	}
	if document.Count != nil && (*document.Count < 0 || *document.Count != len(skillIDs)) {
		return fail("invalid_status", common.ErrConflict)
	}
	for _, item := range status.Orphaned {
		key := preparationSkillHubIdentity{item.SkillID, item.Agent}
		if !global || !validPreparationStatusIdentity(item.SkillID) || !validPreparationStatusIdentity(item.Agent) {
			return fail("invalid_skill_identity", common.ErrConflict)
		}
		if _, duplicate := seen[key]; duplicate {
			return fail("duplicate_skill_identity", common.ErrConflict)
		}
		seen[key] = struct{}{}
	}
	agents := make(map[string]struct{}, len(status.Agents))
	for _, agent := range status.Agents {
		if !global || !validPreparationStatusIdentity(agent.Name) {
			return fail("invalid_agent_identity", common.ErrConflict)
		}
		if _, duplicate := agents[agent.Name]; duplicate {
			return fail("duplicate_agent_identity", common.ErrConflict)
		}
		agents[agent.Name] = struct{}{}
	}
	if document.Count != nil {
		status.SkillCount = *document.Count
	}
	observation.Status = status
	observation.InventoryComplete = true
	observation.CompletedAt = time.Now().UTC()
	return observation, nil
}

type preparationSkillHubIdentity struct{ skillID, agent string }

func validPreparationStatusIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}
