// Package preparation defines File-owned, detached preparation observations.
// A window bounds the age of an observation, not the lifetime of filesystem
// state. It is not a filesystem lease or permission to perform an operation.
package preparation

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

const Version = "workorch-file-preparation-revalidation/v1"
const MaxAge = 30 * time.Second

type ConfigTarget struct{ Scope, Path string }

type Scope struct {
	WorkspaceID                             common.WorkspaceID
	RequestID, PayloadHash                  string
	WorkspaceRevision                       uint64
	InternalDirectory, DeliverableDirectory string
	ResourceRefs                            []string
	Configs                                 []ConfigTarget
}

func (s Scope) Clone() Scope {
	s.ResourceRefs = slices.Clone(s.ResourceRefs)
	s.Configs = slices.Clone(s.Configs)
	return s
}

func (s Scope) Validate() error {
	for _, value := range []string{string(s.WorkspaceID), s.RequestID, s.PayloadHash} {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return common.ErrInvalidArgument
		}
	}
	if s.WorkspaceRevision == 0 || !fileobservation.ValidPath(s.InternalDirectory) || !fileobservation.ValidPath(s.DeliverableDirectory) || len(s.ResourceRefs) > fileobservation.ResourceLimit || len(s.Configs) > fileobservation.ResourceLimit {
		return common.ErrInvalidArgument
	}
	for _, ref := range s.ResourceRefs {
		if !fileobservation.ValidResourceRef(ref) {
			return common.ErrInvalidArgument
		}
	}
	seen := map[ConfigTarget]bool{}
	for _, c := range s.Configs {
		if !workorchmanagedconfig.ValidPreparationTarget(c.Scope, c.Path) || seen[c] {
			return common.ErrInvalidArgument
		}
		seen[c] = true
	}
	return nil
}

type Observation struct {
	Version, OwnerEpoch, TokenID     string
	Scope                            Scope
	StartedAt, ObservedAt, ExpiresAt time.Time
	Internal, Deliverable            fileobservation.Directory
	Resources, Configs               []fileobservation.Resource
}

func ValidEpoch(s string) bool {
	return strings.HasPrefix(s, "file-preparation-epoch:") && validNonce(strings.TrimPrefix(s, "file-preparation-epoch:"))
}
func validNonce(s string) bool {
	if len(s) != 26 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

func (o Observation) Clone() Observation {
	o.Scope = o.Scope.Clone()
	o.Internal, o.Deliverable = o.Internal.Clone(), o.Deliverable.Clone()
	o.Resources, o.Configs = slices.Clone(o.Resources), slices.Clone(o.Configs)
	for i := range o.Resources {
		o.Resources[i] = o.Resources[i].Clone()
	}
	for i := range o.Configs {
		o.Configs[i] = o.Configs[i].Clone()
	}
	return o
}

func (o Observation) Validate() error {
	if o.Version != Version || !ValidEpoch(o.OwnerEpoch) || !validNonce(o.TokenID) || o.Scope.Validate() != nil || o.StartedAt.IsZero() || o.ObservedAt.Before(o.StartedAt) || !o.ExpiresAt.Equal(o.StartedAt.Add(MaxAge)) || !o.ObservedAt.Before(o.ExpiresAt) || o.Internal.Validate() != nil || o.Deliverable.Validate() != nil || o.Internal.Path != o.Scope.InternalDirectory || o.Deliverable.Path != o.Scope.DeliverableDirectory || len(o.Resources) != len(o.Scope.ResourceRefs) || len(o.Configs) != len(o.Scope.Configs) {
		return common.ErrConflict
	}
	for index, items := range [][]fileobservation.Resource{o.Resources, o.Configs} {
		total := 0
		for i, r := range items {
			ref := ""
			if index == 0 {
				ref = o.Scope.ResourceRefs[i]
			} else {
				ref = o.Scope.Configs[i].Path
			}
			if r.Validate() != nil || r.Ref != ref {
				return common.ErrConflict
			}
			total += r.ContentBytes
		}
		if total > fileobservation.ResourceTotalBytes {
			return common.ErrConflict
		}
	}
	return nil
}
