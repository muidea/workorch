package workspace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
)

const PreparationExecutionInputsVersion = "workorch-preparation-execution-inputs/v1"

// The exact non-sensitive process inputs used by activation. This is not a
// copy of application.toml: credentials, endpoints and provider settings have
// no field here. Raw language/locale values are preserved before compilation.
type PreparationProcessPolicy struct {
	ConfiguredDialogLanguage    string
	LCAll, LCMessages, Lang     string
	MaxRuntimeRepairAttempts    int
	MaxDocumentRecoveryAttempts int
}

func (p PreparationProcessPolicy) Validate() error {
	for _, value := range []string{p.ConfiguredDialogLanguage, p.LCAll, p.LCMessages, p.Lang} {
		if len(value) > 1024 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return common.ErrConflict
		}
	}
	if p.MaxRuntimeRepairAttempts <= 0 || p.MaxDocumentRecoveryAttempts <= 0 {
		return common.ErrConflict
	}
	return nil
}

// Same precedence as activation's DefaultDialogLanguageFromContext, without
// reading the environment or current configuration during historical replay.
func (p PreparationProcessPolicy) DialogLanguage() string {
	for _, value := range []string{p.ConfiguredDialogLanguage, p.LCAll, p.LCMessages, p.Lang} {
		if language := dialoglanguage.NormalizeTag(value); language != "" {
			return language
		}
	}
	return ""
}

type PreparationProcessPolicyFacts struct {
	Version                string
	WorkspaceID            common.WorkspaceID
	WorkspaceRevision      uint64
	StartedAt, CompletedAt time.Time
	Before, After          PreparationProcessPolicy
}

func (f PreparationProcessPolicyFacts) Validate() error {
	if f.Version != PreparationExecutionInputsVersion || f.WorkspaceID == "" || f.WorkspaceRevision == 0 || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || f.Before.Validate() != nil || f.After.Validate() != nil || f.Before != f.After {
		return common.ErrConflict
	}
	return nil
}

// Stored only inside the restricted original seal archive. Workspace supplies
// its own complete declarations and the confirmed (not live) template. Pure
// policy compilation must consume these values, never today's configuration.
// This input archive grants no readiness, M3 or execution permission.
type PreparationExecutionInputs struct {
	Version       string
	Process       PreparationProcessPolicyFacts
	Template      WorkspaceTemplate
	Configuration WorkspaceConfigurationSnapshot
}

func (i PreparationExecutionInputs) Clone() PreparationExecutionInputs {
	i.Template = i.Template.Clone()
	i.Configuration = i.Configuration.Clone()
	return i
}

func (i PreparationExecutionInputs) Validate() error {
	raw, err := json.Marshal(i)
	if i.Version != PreparationExecutionInputsVersion || i.Process.Validate() != nil || i.Template.ID == "" || err != nil || len(raw) > 1024*1024 {
		return common.ErrConflict
	}
	return nil
}

func (i PreparationExecutionInputs) validateFor(s PreparationSealSnapshot, a WorkspacePreparationAttempt, record *DefinitionAuthoringRecord) error {
	if i.Validate() != nil || record == nil || i.Process.WorkspaceID != s.WorkspaceID || i.Process.WorkspaceRevision != s.WorkspaceRevision || i.Process.StartedAt.Before(a.StartedAt) || i.Process.CompletedAt.After(a.CompletedAt) || !reflect.DeepEqual(i.Template, record.TemplateSnapshot) || WorkspaceTemplateSnapshotHash(&i.Template) != s.StartupInput.TemplateSnapshotHash || i.Configuration.Hash() != s.AuthoringConfigurationHash || i.Configuration.InternalDirectory != s.InternalDirectory || i.Configuration.Directory != s.StartupInput.ResolvedProjectWorkspaceDirectory || i.Configuration.TemplateID != s.StartupInput.WorkspaceTemplateRef {
		return common.ErrConflict
	}
	for _, check := range a.Checks {
		if check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" {
			return common.ErrConflict
		}
	}
	return nil
}

// Scoped storage persists Soul bindings separately from Workspace metadata.
// The scoped adapter calls this while holding its persistence lock, before
// dropping those fields for the lower metadata CAS. No caller validation flag
// or hash hit substitutes for the complete actual configuration preimage.
func ValidatePreparationExecutionInputsPreimage(current, next *Workspace) error {
	if current == nil || next == nil {
		return common.ErrConflict
	}
	before := make(map[string]bool, len(current.PreparationSealEvidence))
	for _, e := range current.PreparationSealEvidence {
		before[e.Ref] = true
	}
	for _, e := range next.PreparationSealEvidence {
		if e.ExecutionInputs != nil && !before[e.Ref] {
			if current.ID != next.ID || !reflect.DeepEqual(CaptureWorkspaceConfiguration(current), e.ExecutionInputs.Configuration) || WorkspaceConfigurationHash(current) != e.Binding.AuthoringConfigurationHash {
				return common.ErrConflict
			}
		}
	}
	return nil
}
