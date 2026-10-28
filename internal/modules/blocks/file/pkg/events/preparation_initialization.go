package events

import (
	"encoding/hex"
	"reflect"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

const TopicInitializePreparationResources = "/file/startup-preparation/resources/initialize"
const PreparationInitializationVersion = "workorch-file-preparation-initialization/v1"

type InitializePreparationResourcesCommand struct {
	WorkspaceID                             common.WorkspaceID
	RequestID, BindingHash                  string
	InternalDirectory, DeliverableDirectory string
	Declarations                            []PreparationInitializationDeclaration
}

func (c InitializePreparationResourcesCommand) Clone() InitializePreparationResourcesCommand {
	c.Declarations = append([]PreparationInitializationDeclaration(nil), c.Declarations...)
	for i := range c.Declarations {
		c.Declarations[i] = c.Declarations[i].Clone()
	}
	return c
}
func (c InitializePreparationResourcesCommand) Validate() error {
	if c.WorkspaceID == "" || len(c.RequestID) == 0 || len(c.RequestID) > 256 || len(c.BindingHash) != 64 || !fileobservation.ValidPath(c.InternalDirectory) || !fileobservation.ValidPath(c.DeliverableDirectory) || len(c.Declarations) > fileobservation.ResourceLimit {
		return common.ErrInvalidArgument
	}
	if _, e := hex.DecodeString(c.BindingHash); e != nil || strings.TrimSpace(c.RequestID) != c.RequestID {
		return common.ErrInvalidArgument
	}
	if (ObservePreparationDirectoriesCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.BindingHash, WorkspaceRevision: 1, InternalDirectory: c.InternalDirectory, DeliverableDirectory: c.DeliverableDirectory}).Validate() != nil {
		return common.ErrInvalidArgument
	}
	seen := map[string]bool{}
	total := 0
	for _, d := range c.Declarations {
		k := d.Scope + "/" + d.Path
		if d.Validate() != nil || seen[k] {
			return common.ErrInvalidArgument
		}
		seen[k] = true
		total += len(d.DefaultsJSON)
		if total > fileobservation.ResourceTotalBytes {
			return common.ErrInvalidArgument
		}
	}
	return nil
}

type InitializedPreparationConfig struct {
	Before, After   fileobservation.Resource
	Reason          string
	WriteDiagnostic InitializationDiagnostic
}
type InitializePreparationResourcesResult struct {
	Version                string
	Command                InitializePreparationResourcesCommand
	StartedAt, CompletedAt time.Time
	Files                  []InitializedPreparationConfig
	ProjectSkills          fileobservation.Directory
	SkillWriteDiagnostic   InitializationDiagnostic
	RootDiagnostic         InitializationDiagnostic
}

func (r InitializePreparationResourcesResult) Clone() InitializePreparationResourcesResult {
	r.Command = r.Command.Clone()
	r.Files = append([]InitializedPreparationConfig(nil), r.Files...)
	for i := range r.Files {
		r.Files[i].Before = r.Files[i].Before.Clone()
		r.Files[i].After = r.Files[i].After.Clone()
	}
	r.ProjectSkills = r.ProjectSkills.Clone()
	r.SkillWriteDiagnostic = r.SkillWriteDiagnostic.Clone()
	r.RootDiagnostic = r.RootDiagnostic.Clone()
	for i := range r.Files {
		r.Files[i].WriteDiagnostic = r.Files[i].WriteDiagnostic.Clone()
	}
	return r
}
func (r InitializePreparationResourcesResult) InfrastructureObserved() bool {
	if r.SkillWriteDiagnostic.Present() || r.RootDiagnostic.Present() || !r.ProjectSkills.Complete || !r.ProjectSkills.Readable || !r.ProjectSkills.Searchable {
		return false
	}
	for _, f := range r.Files {
		if f.WriteDiagnostic.Present() || f.Reason != "managed_configs_observed_remaining_checks" {
			return false
		}
	}
	return true
}
func (r InitializePreparationResourcesResult) ValidateFor(c InitializePreparationResourcesCommand) error {
	if c.Validate() != nil || r.Version != PreparationInitializationVersion || !reflect.DeepEqual(r.Command, c) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || len(r.Files) != len(c.Declarations) || r.ProjectSkills.Path != c.DeliverableDirectory+"/.agents/skills" || r.ProjectSkills.Validate() != nil {
		return common.ErrConflict
	}
	if r.SkillWriteDiagnostic.Validate() != nil || r.RootDiagnostic.Validate() != nil {
		return common.ErrConflict
	}
	total := 0
	for i, f := range r.Files {
		total += f.Before.ContentBytes + f.After.ContentBytes
		if f.WriteDiagnostic.Validate() != nil || total > 2*fileobservation.ResourceTotalBytes {
			return common.ErrConflict
		}
		if f.Before.Ref != c.Declarations[i].Path || f.After.Ref != f.Before.Ref || f.Before.Validate() != nil || f.After.Validate() != nil {
			return common.ErrConflict
		}
		reason := "managed_config_observation_incomplete"
		if f.After.Complete {
			reason = workorchmanagedconfig.PreparationJSONReason(c.Declarations[i].Policy, c.Declarations[i].DefaultsJSON, f.After.Content)
		}
		if f.Reason != reason {
			return common.ErrConflict
		}
	}
	return nil
}

// DefaultsJSON is the complete owner-derived file body. It is kept as bytes so
// persistence cannot round large integers or normalize the confirmed content.
type PreparationInitializationDeclaration struct {
	Scope, Path, Policy string
	DefaultsJSON        []byte
}

func (d PreparationInitializationDeclaration) Clone() PreparationInitializationDeclaration {
	d.DefaultsJSON = append([]byte(nil), d.DefaultsJSON...)
	return d
}
func (d PreparationInitializationDeclaration) Validate() error {
	if !workorchmanagedconfig.ValidPreparationTarget(d.Scope, d.Path) || len(d.DefaultsJSON) > fileobservation.ResourceMaxBytes || d.Scope == workorchmanagedconfig.ScopeSystem && d.Policy != workorchmanagedconfig.PolicySystemExact || d.Scope == workorchmanagedconfig.ScopeWorkspace && d.Policy != workorchmanagedconfig.PolicyWorkspacePreserve && d.Policy != workorchmanagedconfig.PolicyWorkspaceMergeMissing || workorchmanagedconfig.PreparationJSONReason(d.Policy, d.DefaultsJSON, []byte(`{}`)) == "managed_config_declaration_invalid" {
		return common.ErrInvalidArgument
	}
	return nil
}

// Known write failures remain known even when their original text exceeds the
// explicit archive boundary. Never truncate error bytes or mark them complete.
type InitializationDiagnostic struct {
	Raw                    []byte
	Bytes                  int
	SHA256, NotSavedReason string
}

func (d InitializationDiagnostic) Present() bool { return d.SHA256 != "" }
func (d InitializationDiagnostic) Clone() InitializationDiagnostic {
	d.Raw = append([]byte(nil), d.Raw...)
	return d
}
func (d InitializationDiagnostic) Validate() error {
	if !d.Present() {
		if len(d.Raw) != 0 || d.Bytes != 0 || d.NotSavedReason != "" {
			return common.ErrConflict
		}
		return nil
	}
	if d.NotSavedReason != "" {
		if d.NotSavedReason != "diagnostic_exceeds_byte_boundary" || len(d.Raw) != 0 || d.Bytes <= fileobservation.DiagnosticLimit || len(d.SHA256) != 64 {
			return common.ErrConflict
		}
		if _, e := hex.DecodeString(d.SHA256); e != nil {
			return common.ErrConflict
		}
		return nil
	}
	if d.Bytes != len(d.Raw) || len(d.Raw) > fileobservation.DiagnosticLimit || d.SHA256 != fileobservation.Digest(d.Raw) {
		return common.ErrConflict
	}
	return nil
}
