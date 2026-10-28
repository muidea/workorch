package workspace

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

const PreparationSkillHubEffectsVersion = "workorch-preparation-skillhub-effects/v1"
const PreparationSkillHubRefPrefix = "skillhub-status:"
const PreparationSkillHubOutputLimit = 1 << 20
const PreparationSkillHubErrorLimit = 64 << 10

// Owner-private operation summary, not resource readiness or execution rights.
type PreparationSkillHubEffectState string

const (
	PreparationSkillHubReserved   PreparationSkillHubEffectState = "reserved"
	PreparationSkillHubRecorded   PreparationSkillHubEffectState = "recorded"
	PreparationSkillHubUnresolved PreparationSkillHubEffectState = "unresolved"
)

// Raw capture only. It deliberately excludes parsed inventory/PASS flags.
// Complete means the recorded stream is complete, never that status is ready.
type PreparationSkillHubStatusFacts struct {
	Version, EffectContract, Scope, Directory string
	StartedAt, CompletedAt                    time.Time
	RawOutput, RawStderr                      []byte
	RawSHA256, StderrSHA256                   string
	TotalOutputBytes, TotalStderrBytes        uint64
	OutputComplete, StderrComplete            bool
	CommandStarted, ExitCodeKnown             bool
	ExitCode                                  int
	FailureCode                               string
	ErrorText                                 []byte
	ErrorBytes                                uint64
	ErrorSHA256                               string
	ErrorNotSavedReason                       string
	Installed                                 *skillhubpreparation.Installed
}

func (f PreparationSkillHubStatusFacts) Clone() PreparationSkillHubStatusFacts {
	f.RawOutput = bytes.Clone(f.RawOutput)
	f.RawStderr = bytes.Clone(f.RawStderr)
	f.ErrorText = bytes.Clone(f.ErrorText)
	if f.Installed != nil {
		v := f.Installed.Clone()
		f.Installed = &v
	}
	return f
}
func validPreparationSkillHubStream(raw []byte, total uint64, digest string, complete bool) bool {
	if len(raw) > PreparationSkillHubOutputLimit || total < uint64(len(raw)) || complete && total != uint64(len(raw)) {
		return false
	}
	if digest == "" {
		return len(raw) == 0 && total == 0 && !complete
	}
	return digest == fileobservation.Digest(raw)
}
func (f PreparationSkillHubStatusFacts) ValidateFor(scope, directory string) error {
	if f.Version != "workorch-skillhub-preparation-observation/v2" || f.EffectContract != "skillhub-status-effects/v1" || f.Scope != scope || f.Directory != directory || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || !validPreparationSkillHubStream(f.RawOutput, f.TotalOutputBytes, f.RawSHA256, f.OutputComplete) || !validPreparationSkillHubStream(f.RawStderr, f.TotalStderrBytes, f.StderrSHA256, f.StderrComplete) || f.ExitCodeKnown && (!f.CommandStarted || f.ExitCode < 0) || !f.ExitCodeKnown && f.ExitCode != 0 {
		return common.ErrConflict
	}
	if f.Installed != nil {
		status, err := skillhubpreparation.ParseStatus(f.RawOutput, directory, scope == "global")
		if err != nil || !f.CommandStarted || !f.ExitCodeKnown || f.ExitCode != 0 || f.FailureCode != "" || !f.OutputComplete || !f.StderrComplete || f.Installed.StartedAt.Before(f.CompletedAt) || f.Installed.Validate(status, directory, scope == "global") != nil {
			return common.ErrConflict
		}
	}
	if f.FailureCode != "" && !boundedPreparationIdentity(f.FailureCode) || !f.CommandStarted && (f.ExitCodeKnown || len(f.RawOutput) > 0 || len(f.RawStderr) > 0 || f.TotalOutputBytes != 0 || f.TotalStderrBytes != 0 || f.FailureCode == "") {
		return common.ErrConflict
	}
	if f.ErrorBytes == 0 {
		if len(f.ErrorText) != 0 || f.ErrorSHA256 != "" || f.ErrorNotSavedReason != "" {
			return common.ErrConflict
		}
	} else if f.ErrorNotSavedReason == "" {
		if len(f.ErrorText) > PreparationSkillHubErrorLimit || uint64(len(f.ErrorText)) != f.ErrorBytes || f.ErrorSHA256 != fileobservation.Digest(f.ErrorText) {
			return common.ErrConflict
		}
	} else if f.ErrorNotSavedReason != "error_exceeds_storage_limit" || len(f.ErrorText) != 0 || f.ErrorBytes <= PreparationSkillHubErrorLimit || !preparationFenceHex(f.ErrorSHA256, 32) {
		return common.ErrConflict
	}
	return nil
}

// Failure remains a known command/parse failure even when external effects
// are unresolved. No return/exit-code alone asserts that a service did not save.
func (f PreparationSkillHubStatusFacts) EffectsUnresolved() bool {
	return f.CommandStarted && (f.FailureCode != "" || !f.ExitCodeKnown || f.ExitCode != 0 || !f.OutputComplete || !f.StderrComplete)
}

// A finite historical diagnostic only. The original unresolved fence and raw
// receipt remain unchanged; a missing project is not an empty Skill inventory.
func PreparationSkillHubProjectIssue(w *Workspace) (string, error) {
	if ValidatePreparationSkillHubEffectsPersistence(w) != nil {
		return "", common.ErrConflict
	}
	if w.StartupPreparation == nil || len(w.StartupPreparation.Attempts) == 0 {
		return "", nil
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	for _, e := range w.PreparationSkillHubEffects {
		if e.Ref != a.SkillHubEffectRef || e.Project == nil || a.State != WorkspacePreparationBlocked {
			continue
		}
		f := e.Project
		if f.CommandStarted && f.ExitCodeKnown && f.ExitCode == 1 && f.OutputComplete && f.StderrComplete && (f.FailureCode == "command_failed" || f.FailureCode == "project_not_registered") && skillhubpreparation.ProjectNotFound(f.RawOutput) {
			return "project_registration_required", nil
		}
	}
	return "", nil
}

// The two exact targets are sealed in the initial accepted CAS, before either
// command. Only its unambiguous first continuation may dispatch them once.
type WorkspacePreparationSkillHubEffects struct {
	Version, Ref              string
	WorkspaceID               common.WorkspaceID
	RequestID, PayloadHash    string
	ExpectedWorkspaceRevision uint64
	Binding                   WorkspacePreparationBinding
	ProjectDirectory          string
	ReservedAt, CompletedAt   time.Time
	Global, Project           *PreparationSkillHubStatusFacts
	ContentHash               string
}

func ValidPreparationSkillHubRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationSkillHubRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationSkillHubRefPrefix), 16)
}
func (e WorkspacePreparationSkillHubEffects) Clone() WorkspacePreparationSkillHubEffects {
	if e.Global != nil {
		f := e.Global.Clone()
		e.Global = &f
	}
	if e.Project != nil {
		f := e.Project.Clone()
		e.Project = &f
	}
	return e
}
func (e WorkspacePreparationSkillHubEffects) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}
func (e WorkspacePreparationSkillHubEffects) Summary() PreparationSkillHubEffectState {
	if e.CompletedAt.IsZero() {
		return PreparationSkillHubReserved
	}
	if e.Global == nil || e.Project == nil || e.Global.EffectsUnresolved() || e.Project.EffectsUnresolved() {
		return PreparationSkillHubUnresolved
	}
	return PreparationSkillHubRecorded
}

// Derived from full raw inventory and both actual file reads, not a caller flag.
func (e WorkspacePreparationSkillHubEffects) InstalledSummary() string {
	if (e.Global == nil || e.Global.Installed == nil) && (e.Project == nil || e.Project.Installed == nil) {
		return ""
	}
	for i, f := range []*PreparationSkillHubStatusFacts{e.Global, e.Project} {
		dir := ""
		if i == 1 {
			dir = e.ProjectDirectory
		}
		if f == nil || f.Installed == nil {
			return "incomplete"
		}
		status, err := skillhubpreparation.ParseStatus(f.RawOutput, dir, i == 0)
		if err != nil || !f.Installed.CompleteFor(status, dir, i == 0) {
			return "incomplete"
		}
	}
	return "observed_selection_pending"
}
func (e WorkspacePreparationSkillHubEffects) Validate() error {
	if e.Version != PreparationSkillHubEffectsVersion || !ValidPreparationSkillHubRef(e.Ref) || !boundedPreparationIdentity(string(e.WorkspaceID)) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.ExpectedWorkspaceRevision == 0 || e.ExpectedWorkspaceRevision == ^uint64(0) || e.Binding.Validate() != nil || e.ProjectDirectory == "" || len(e.ProjectDirectory) > 4096 || !utf8.ValidString(e.ProjectDirectory) || strings.ContainsRune(e.ProjectDirectory, 0) || e.ReservedAt.IsZero() || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	if e.CompletedAt.IsZero() {
		if e.Global != nil || e.Project != nil {
			return common.ErrConflict
		}
		return nil
	}
	if e.CompletedAt.Before(e.ReservedAt) {
		return common.ErrConflict
	}
	for i, f := range []*PreparationSkillHubStatusFacts{e.Global, e.Project} {
		if f == nil {
			continue
		}
		scope, dir := "global", ""
		if i == 1 {
			scope, dir = "workspace", e.ProjectDirectory
		}
		if f.ValidateFor(scope, dir) != nil || f.StartedAt.Before(e.ReservedAt) || f.CompletedAt.After(e.CompletedAt) || f.Installed != nil && f.Installed.CompletedAt.After(e.CompletedAt) {
			return common.ErrConflict
		}
	}
	globalEnd := time.Time{}
	if e.Global != nil {
		globalEnd = e.Global.CompletedAt
		if e.Global.Installed != nil {
			globalEnd = e.Global.Installed.CompletedAt
		}
	}
	if e.Global != nil && e.Project != nil && e.Project.StartedAt.Before(globalEnd) {
		return common.ErrConflict
	}
	return nil
}
func ClonePreparationSkillHubEffects(items []WorkspacePreparationSkillHubEffects) []WorkspacePreparationSkillHubEffects {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationSkillHubEffects, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}
func ValidatePreparationSkillHubEffectsPersistence(w *Workspace) error {
	if w == nil || len(w.PreparationSkillHubEffects) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := map[string]WorkspacePreparationAttempt{}
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.SkillHubEffectRef != "" {
				if _, ok := refs[a.SkillHubEffectRef]; ok {
					return common.ErrConflict
				}
				refs[a.SkillHubEffectRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationSkillHubEffects) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationSkillHubEffects {
		a, ok := refs[e.Ref]
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || e.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.ExpectedWorkspaceRevision != a.ExpectedWorkspaceRevision || !reflect.DeepEqual(e.Binding, a.Binding) || !e.ReservedAt.Equal(a.StartedAt) || !e.CompletedAt.Equal(a.CompletedAt) || e.Summary() != a.SkillHubEffectState || e.InstalledSummary() != a.SkillHubInstalledState || (e.CompletedAt.IsZero() && a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown) || (!e.CompletedAt.IsZero() && a.State != WorkspacePreparationBlocked) {
			return common.ErrConflict
		}
		delete(refs, e.Ref)
	}
	return nil
}
func validatePreparationSkillHubEffectsTransition(current, next *Workspace, cas bool) error {
	if ValidatePreparationSkillHubEffectsPersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationSkillHubEffects) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if ValidatePreparationSkillHubEffectsPersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationSkillHubEffects, next.PreparationSkillHubEffects
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || current.StartupPreparation == nil && len(before) > 0 || next.StartupPreparation == nil {
		return common.ErrConflict
	}
	if len(after) == len(before)+1 {
		attemptsBefore := 0
		if current.StartupPreparation != nil {
			attemptsBefore = len(current.StartupPreparation.Attempts)
		}
		if len(next.StartupPreparation.Attempts) != attemptsBefore+1 {
			return common.ErrConflict
		}
		for _, prior := range before {
			if prior.Summary() != PreparationSkillHubRecorded {
				return common.ErrConflict
			}
		}
		if len(before) > 0 && !reflect.DeepEqual(before, after[:len(before)]) {
			return common.ErrConflict
		}
		e := after[len(before)]
		a := next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
		if !e.CompletedAt.IsZero() || a.State != WorkspacePreparationAccepted || a.ExpectedWorkspaceRevision != current.Revision {
			return common.ErrConflict
		}
		return nil
	}
	if len(before) == 0 || len(before) != len(after) || !reflect.DeepEqual(before[:len(before)-1], after[:len(after)-1]) {
		return common.ErrConflict
	}
	a, b := before[len(before)-1], after[len(after)-1]
	if !a.CompletedAt.IsZero() || b.CompletedAt.IsZero() {
		return common.ErrConflict
	}
	b.CompletedAt, b.Global, b.Project, b.ContentHash = a.CompletedAt, a.Global, a.Project, a.ContentHash
	if !reflect.DeepEqual(a, b) {
		return common.ErrConflict
	}
	return nil
}
