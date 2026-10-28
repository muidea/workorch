package workspace

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"
	"time"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

const PreparationInitializationVersion = "workorch-preparation-resource-initialization/v1"
const PreparationInitializationLimit = 16
const PreparationInitializationResultLimit = 4

// Private, bounded write journal. Raw before/after bytes and owner errors stay
// in restricted persistence. Neither completion nor the plan grants readiness.
type PreparationResourceInitialization struct {
	Version                                               string
	ExpectedWorkspaceRevision, CommittedWorkspaceRevision uint64
	StartupInputHash, ConfigurationHash, CreationEpoch    string
	PreparationEpoch                                      uint64
	DefaultSoulID                                         common.SoulID
	SoulBindings                                          map[string]common.SoulID
	PreparedAt                                            time.Time
	Plan                                                  fileevents.InitializePreparationResourcesCommand
	Attempts                                              []PreparationResourceInitializationAttempt
	Registration                                          *PreparationRegistration
}

type PreparationResourceInitializationAttempt struct {
	PreparedAt time.Time
	Result     *fileevents.InitializePreparationResourcesResult
}

func (r PreparationResourceInitialization) Clone() PreparationResourceInitialization {
	r.Registration = r.Registration.Clone()
	r.Plan = r.Plan.Clone()
	r.SoulBindings = maps.Clone(r.SoulBindings)
	r.Attempts = append([]PreparationResourceInitializationAttempt(nil), r.Attempts...)
	for i := range r.Attempts {
		if r.Attempts[i].Result != nil {
			v := r.Attempts[i].Result.Clone()
			r.Attempts[i].Result = &v
		}
	}

	return r
}
func ClonePreparationInitializations(rs []PreparationResourceInitialization) []PreparationResourceInitialization {
	if rs == nil {
		return nil
	}
	out := make([]PreparationResourceInitialization, len(rs))
	for i := range rs {
		out[i] = rs[i].Clone()
	}
	return out
}
func (r PreparationResourceInitialization) BindingHash() string {
	plan := r.Plan.Clone()
	plan.BindingHash = ""
	raw, _ := json.Marshal(struct {
		Startup, Config, Epoch string
		Preparation            uint64
		Soul                   common.SoulID
		Bindings               map[string]common.SoulID
		Plan                   fileevents.InitializePreparationResourcesCommand
		Registration           *skillhubpreparation.RegistrationPlan `json:",omitempty"`
	}{r.StartupInputHash, r.ConfigurationHash, r.CreationEpoch, r.PreparationEpoch, r.DefaultSoulID, r.SoulBindings, plan, r.registrationPlan()})
	return fileobservation.Digest(raw)
}

func (r PreparationResourceInitialization) registrationPlan() *skillhubpreparation.RegistrationPlan {
	if r.Registration == nil {
		return nil
	}
	p := r.Registration.Plan
	return &p
}
func (r PreparationResourceInitialization) InfrastructureObserved() bool {
	return r.FileObserved() && (r.Registration == nil || r.Registration.Succeeded())
}
func (r PreparationResourceInitialization) FileObserved() bool {
	return len(r.Attempts) > 0 && r.Attempts[len(r.Attempts)-1].Result != nil && r.Attempts[len(r.Attempts)-1].Result.InfrastructureObserved()
}
func (r PreparationResourceInitialization) State() string {
	if r.InfrastructureObserved() {
		return "completed"
	}
	if r.FileObserved() && r.Registration != nil {
		if len(r.Registration.Attempts) == 0 || r.Registration.Status() == "pending" {
			return "pending"
		}
		return "blocked"
	}
	if len(r.Attempts) > 0 && r.Attempts[len(r.Attempts)-1].Result != nil {
		return "blocked"
	}
	return "pending"
}
func (r PreparationResourceInitialization) Matches(w *Workspace) bool {
	return w != nil && w.StartupInput != nil && w.PreparationFence != nil && w.ActiveRunID == "" && w.PreparationFence.Pending == nil && w.PreparationFence.CreationEpoch == r.CreationEpoch && w.PreparationFence.PreparationEpoch == r.PreparationEpoch && len(w.PreparationFence.Settled) == 0 && w.StartupInput.StartupInputHash == r.StartupInputHash && WorkspaceConfigurationHash(w) == r.ConfigurationHash && w.InternalDirectory == r.Plan.InternalDirectory && w.DeliverableDirectory == r.Plan.DeliverableDirectory && r.matchesDeclarations(w)
}
func (r PreparationResourceInitialization) Validate(id common.WorkspaceID) error {
	if r.validateRegistration() != nil || r.Plan.WorkspaceID != id || r.Plan.Validate() != nil || r.Plan.BindingHash != r.BindingHash() || r.StartupInputHash == "" || r.ConfigurationHash == "" || r.CreationEpoch == "" || r.PreparedAt.IsZero() || r.ExpectedWorkspaceRevision == 0 || r.CommittedWorkspaceRevision < r.ExpectedWorkspaceRevision+1 || len(r.Attempts) == 0 || len(r.Attempts) > PreparationInitializationResultLimit || !r.Attempts[0].PreparedAt.Equal(r.PreparedAt) {
		return common.ErrConflict
	}
	at := r.PreparedAt
	for _, a := range r.Attempts {
		if a.PreparedAt.Before(at) {
			return common.ErrConflict
		}
		at = a.PreparedAt
		if a.Result != nil {
			if a.Result.ValidateFor(r.Plan) != nil || a.Result.StartedAt.Before(at) {
				return common.ErrConflict
			}
			at = a.Result.CompletedAt
		}
	}

	return nil
}
func ValidatePreparationInitializations(w *Workspace) error {
	if w == nil || len(w.PreparationResourceInitializations) > PreparationInitializationLimit {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for i, r := range w.PreparationResourceInitializations {
		if r.Validate(w.ID) != nil || seen[r.Plan.RequestID] || r.CommittedWorkspaceRevision > w.Revision || i < len(w.PreparationResourceInitializations)-1 && !r.InfrastructureObserved() {
			return common.ErrConflict
		}
		seen[r.Plan.RequestID] = true
	}
	return nil
}
func PreparationInitializationPending(w *Workspace) bool {
	return w != nil && len(w.PreparationResourceInitializations) > 0 && !w.PreparationResourceInitializations[len(w.PreparationResourceInitializations)-1].InfrastructureObserved()
}
func validatePreparationInitializationTransition(current, next *Workspace, cas bool) error {
	if next == nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationResourceInitializations) > 0 {
			return common.ErrConflict
		}
		return nil
	}
	a, b := current.PreparationResourceInitializations, next.PreparationResourceInitializations
	// A pending filesystem transaction cannot race a new execution reservation.
	if PreparationInitializationPending(current) && (!reflect.DeepEqual(current.PreparationFence, next.PreparationFence) || !reflect.DeepEqual(current.StartupPreparation, next.StartupPreparation)) {
		return common.ErrConflict
	}
	if reflect.DeepEqual(a, b) {
		return nil
	}
	probe := *next
	probe.PreparationResourceInitializations = current.PreparationResourceInitializations
	probe.Revision = current.Revision
	if !reflect.DeepEqual(*current, probe) {
		return common.ErrConflict
	}
	if !cas || current.ID != next.ID || current.Revision == ^uint64(0) || !reflect.DeepEqual(current.PreparationFence, next.PreparationFence) || !reflect.DeepEqual(current.StartupPreparation, next.StartupPreparation) {
		return common.ErrConflict
	}
	if len(b) == len(a)+1 && len(b) <= PreparationInitializationLimit {
		r := b[len(a)]
		if PreparationInitializationPending(current) || !reflect.DeepEqual(a, b[:len(a)]) && len(a) > 0 || r.Validate(next.ID) != nil || !r.matchesMetadata(current) || r.ExpectedWorkspaceRevision != current.Revision || r.CommittedWorkspaceRevision != current.Revision+1 || len(r.Attempts) != 1 || r.Attempts[0].Result != nil || r.Registration != nil && len(r.Registration.Attempts) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if len(a) == 0 || len(b) != len(a) || !reflect.DeepEqual(a[:len(a)-1], b[:len(b)-1]) {
		return common.ErrConflict
	}
	old, updated := a[len(a)-1], b[len(b)-1]
	if old.InfrastructureObserved() || !old.matchesMetadata(current) || updated.Validate(next.ID) != nil || updated.CommittedWorkspaceRevision != current.Revision+1 {
		return common.ErrConflict
	}
	if !reflect.DeepEqual(old.Registration, updated.Registration) {
		if validateRegistrationProgress(old, updated) != nil {
			return common.ErrConflict
		}
	} else if old.FileObserved() {
		return common.ErrConflict
	} else if len(updated.Attempts) == len(old.Attempts)+1 {
		if !reflect.DeepEqual(old.Attempts, updated.Attempts[:len(old.Attempts)]) || updated.Attempts[len(old.Attempts)].Result != nil {
			return common.ErrConflict
		}
	} else if len(updated.Attempts) == len(old.Attempts) {
		last := len(old.Attempts) - 1
		if old.Attempts[last].Result != nil || updated.Attempts[last].Result == nil || !reflect.DeepEqual(old.Attempts[:last], updated.Attempts[:last]) || !old.Attempts[last].PreparedAt.Equal(updated.Attempts[last].PreparedAt) {
			return common.ErrConflict
		}
	} else {
		return common.ErrConflict
	}
	updated.Registration = old.Registration
	updated.Attempts = old.Attempts
	updated.CommittedWorkspaceRevision = old.CommittedWorkspaceRevision
	if !reflect.DeepEqual(old, updated) {
		return common.ErrConflict
	}

	return nil
}

// Scoped storage removes Soul sidecars in the lower metadata CAS. Only that
// omission can be filled from the immutable preimage; present values must match.
func (r PreparationResourceInitialization) matchesMetadata(w *Workspace) bool {
	if w == nil {
		return false
	}
	copy := *w
	if copy.DefaultSoulID == "" && len(copy.SoulBindings) == 0 {
		copy.DefaultSoulID = r.DefaultSoulID
		copy.SoulBindings = maps.Clone(r.SoulBindings)
	}
	return r.Matches(&copy)
}

// Called under the scoped store lock before metadata is stripped, and by the
// complete in-memory repository. No trusted caller flag bypasses the raw check.
func ValidatePreparationInitializationPreimage(current, next *Workspace) error {
	if current == nil || next == nil {
		return common.ErrConflict
	}
	if reflect.DeepEqual(current.PreparationResourceInitializations, next.PreparationResourceInitializations) {
		return nil
	}
	if len(next.PreparationResourceInitializations) == 0 {
		return common.ErrConflict
	}
	r := next.PreparationResourceInitializations[len(next.PreparationResourceInitializations)-1]
	if !r.Matches(current) || r.DefaultSoulID != current.DefaultSoulID || !reflect.DeepEqual(r.SoulBindings, current.SoulBindings) {
		return common.ErrConflict
	}
	return nil
}

// Reconstruct the complete declared batch from the actual sealed sources. A
// self-consistent plan hash cannot authorize an added target or changed body.
func (r PreparationResourceInitialization) matchesDeclarations(w *Workspace) bool {
	if w.DefinitionAuthoring == nil {
		return false
	}
	ds, err := workorchmanagedconfig.CompilePreparationDeclarations(w.DefinitionAuthoring.TemplateSnapshot.ContextHints, w.ContextHints)
	if err != nil || len(ds) != len(r.Plan.Declarations) {
		return false
	}
	for i, d := range ds {
		p := r.Plan.Declarations[i]
		raw, err := json.Marshal(d.Defaults)
		if err != nil || d.Scope != p.Scope || d.Path != p.Path || d.Policy != p.Policy || !bytes.Equal(raw, p.DefaultsJSON) {
			return false
		}
	}
	return true
}
