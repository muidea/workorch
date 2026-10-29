package workspace

import (
	"reflect"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationOwnerRevalidationVersion = "workorch-preparation-owner-revalidation/v1"
const PreparationOwnerRevalidationMaxAge = 30 * time.Second

// Raw final observations of the original non-Provider sources. Nil means no
// complete observation was obtained, never empty inventory or permission. The
// public command cannot supply this value and GET/recovery cannot refresh it.
type PreparationOwnerRevalidation struct {
	Version                string
	WorkspaceID            common.WorkspaceID
	RequestID, PayloadHash string
	WorkspaceRevision      uint64
	StartedAt, CompletedAt time.Time
	Souls                  *PreparationSoulFacts
	Skills                 *PreparationSkillCatalogFacts
	Memory                 *PreparationMemoryInventoryFacts
	MCP                    *PreparationMCPObservationFacts
}

func (r PreparationOwnerRevalidation) Clone() PreparationOwnerRevalidation {
	if r.Souls != nil {
		v := r.Souls.Clone()
		r.Souls = &v
	}
	if r.Skills != nil {
		v := r.Skills.Clone()
		r.Skills = &v
	}
	if r.Memory != nil {
		v := r.Memory.Clone()
		r.Memory = &v
	}
	if r.MCP != nil {
		v := r.MCP.Clone()
		r.MCP = &v
	}
	return r
}

func (r PreparationOwnerRevalidation) Validate() error {
	if r.Version != PreparationOwnerRevalidationVersion || !boundedPreparationIdentity(string(r.WorkspaceID)) || !boundedPreparationIdentity(r.RequestID) || !boundedPreparationIdentity(r.PayloadHash) || r.WorkspaceRevision == 0 || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || !r.CompletedAt.Before(r.StartedAt.Add(PreparationOwnerRevalidationMaxAge)) {
		return common.ErrConflict
	}
	inside := func(start, end time.Time) bool { return !start.Before(r.StartedAt) && !end.After(r.CompletedAt) }
	if f := r.Souls; f != nil && (f.Validate() != nil || f.WorkspaceID != r.WorkspaceID || f.WorkspaceRevision != r.WorkspaceRevision || !inside(f.StartedAt, f.CompletedAt)) {
		return common.ErrConflict
	}
	if f := r.Skills; f != nil && (f.Validate() != nil || f.WorkspaceID != r.WorkspaceID || f.WorkspaceRevision != r.WorkspaceRevision || !inside(f.StartedAt, f.CompletedAt)) {
		return common.ErrConflict
	}
	if f := r.Memory; f != nil && (f.Validate() != nil || f.Observation.WorkspaceID != r.WorkspaceID || f.WorkspaceRevision != r.WorkspaceRevision || !inside(f.Observation.ObservedAt, f.Observation.ObservedAt) || f.SelectionSource != nil && !inside(f.SelectionSource.StartedAt, f.SelectionSource.CompletedAt)) {
		return common.ErrConflict
	}
	if f := r.MCP; f != nil && (f.Validate() != nil || f.WorkspaceID != r.WorkspaceID || f.WorkspaceRevision != r.WorkspaceRevision || !inside(f.StartedAt, f.CompletedAt)) {
		return common.ErrConflict
	}
	return nil
}

// Each comparison retains full original values (including epoch, nil/empty,
// ordering and provenance). Observation times are not content identities.
func (r PreparationOwnerRevalidation) MatchesOriginal(w *Workspace, a WorkspacePreparationAttempt) bool {
	if w == nil || r.Validate() != nil || r.WorkspaceID != w.ID || r.RequestID != a.RequestID || r.PayloadHash != a.PayloadHash || a.ExpectedWorkspaceRevision == ^uint64(0) || r.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || r.CompletedAt.After(a.CompletedAt) || r.StartedAt.Before(a.StartedAt) || r.Souls == nil || r.Skills == nil || r.Memory == nil || r.MCP == nil {
		return false
	}
	soul, skill, memory, mcp := false, false, false, false
	counts := [4]int{}
	for _, e := range w.PreparationSoulEvidence {
		if e.Ref != a.SoulEvidenceRef {
			continue
		}
		counts[0]++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.Validate() != nil || r.StartedAt.Before(e.Facts.CompletedAt) {
			continue
		}
		before, after := e.Facts.Clone(), r.Souls.Clone()
		before.StartedAt, before.CompletedAt = after.StartedAt, after.CompletedAt
		soul = reflect.DeepEqual(before, after)
	}
	for _, e := range w.PreparationSkillRegistryEvidence {
		if e.Ref != a.SkillRegistryEvidenceRef {
			continue
		}
		counts[1]++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.Validate() != nil || r.StartedAt.Before(e.Facts.CompletedAt) {
			continue
		}
		before, after := e.Facts.Clone(), r.Skills.Clone()
		before.StartedAt, before.CompletedAt = after.StartedAt, after.CompletedAt
		before.Catalog.ObservedAt, before.ReobservedCatalog.ObservedAt = after.Catalog.ObservedAt, after.ReobservedCatalog.ObservedAt
		skill = reflect.DeepEqual(before, after)
	}
	for _, e := range w.PreparationMemoryInventoryEvidence {
		if e.Ref != a.MemoryInventoryEvidenceRef {
			continue
		}
		counts[2]++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.Validate() != nil || r.StartedAt.Before(e.Facts.Observation.ObservedAt) || e.Facts.SelectionSource == nil || r.Memory.SelectionSource == nil || r.StartedAt.Before(e.Facts.SelectionSource.CompletedAt) {
			continue
		}
		before, after := e.Facts.Clone(), r.Memory.Clone()
		before.Observation.ObservedAt = after.Observation.ObservedAt
		before.SelectionSource.StartedAt, before.SelectionSource.CompletedAt = after.SelectionSource.StartedAt, after.SelectionSource.CompletedAt
		memory = reflect.DeepEqual(before, after)
	}
	for _, e := range w.PreparationMCPRegistryEvidence {
		if e.Ref != a.MCPRegistryEvidenceRef {
			continue
		}
		counts[3]++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.Validate() != nil || r.StartedAt.Before(e.Facts.CompletedAt) {
			continue
		}
		before, after := e.Facts.Clone(), r.MCP.Clone()
		before.StartedAt, before.CompletedAt = after.StartedAt, after.CompletedAt
		mcp = reflect.DeepEqual(before, after)
	}
	return counts == [4]int{1, 1, 1, 1} && soul && skill && memory && mcp
}

// A historical match is not a lease. This reports only known absence/drift or
// age expiry. It never issues readiness/M3/activation permission.
func PreparationOwnerRevalidationStale(w *Workspace, now time.Time) (bool, error) {
	if w == nil || now.IsZero() {
		return false, common.ErrInvalidArgument
	}
	if w.StartupPreparation == nil || len(w.StartupPreparation.Attempts) == 0 {
		return false, nil
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	return PreparationOwnerRevalidationStaleForAttempt(w, a, now)
}
func PreparationOwnerRevalidationStaleForAttempt(w *Workspace, a WorkspacePreparationAttempt, now time.Time) (bool, error) {
	if w == nil || now.IsZero() {
		return false, common.ErrInvalidArgument
	}

	if a.OwnerRevalidationVersion == "" {
		return false, nil // reads cannot upgrade old histories
	}
	if a.OwnerRevalidationVersion != PreparationOwnerRevalidationVersion {
		return false, common.ErrConflict
	}
	for _, e := range w.PreparationSealEvidence {
		if e.Ref != a.SealEvidenceRef || e.FinalOwners == nil {
			continue
		}
		r := e.FinalOwners
		if r.Validate() != nil {
			return false, common.ErrConflict
		}
		return !r.MatchesOriginal(w, a) || now.Before(r.CompletedAt) || !now.Before(r.StartedAt.Add(PreparationOwnerRevalidationMaxAge)), nil
	}
	// Missing originals are already blocked, not evidence of empty inventory.
	// Once all originals exist, omitting the required final read cannot evade
	// the same stale projection used for an explicitly incomplete final read.
	return a.State == WorkspacePreparationBlocked && a.SoulEvidenceRef != "" && a.SkillRegistryEvidenceRef != "" && a.MemoryInventoryEvidenceRef != "" && a.MCPRegistryEvidenceRef != "", nil
}
