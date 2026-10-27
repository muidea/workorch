package events

import (
	"strings"
	"time"
	"unicode/utf8"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicObserveRegistryCatalog = "/skill/startup-preparation/registry-catalog/query"
const RegistryCatalogVersion = skillmodel.RegistryCatalogVersion
const RegistryCatalogMaxItems = skillmodel.RegistryCatalogMaxItems
const RegistryCatalogMaxBytes = skillmodel.RegistryCatalogMaxBytes

type ObserveRegistryCatalogCommand struct {
	WorkspaceID common.WorkspaceID
}

func (c ObserveRegistryCatalogCommand) Validate() error {
	id := string(c.WorkspaceID)
	if id == "" || len(id) > 256 || !utf8.ValidString(id) || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") {
		return common.ErrInvalidArgument
	}
	return nil
}

// One complete registry read, never a merged SkillHub catalog, a selected Skill
// proof or readiness. OwnerEpoch changes on owner reconstruction; CatalogHash
// identifies content but does not authorize skipping full record comparison.
type ObserveRegistryCatalogResult struct {
	Version     string
	WorkspaceID common.WorkspaceID
	OwnerEpoch  string
	ObservedAt  time.Time
	Skills      []*skillmodel.Skill
	CatalogHash string
}

// Snapshot returns detached factual values, never a validation credential.
func (r ObserveRegistryCatalogResult) Snapshot() skillmodel.PreparationRegistryCatalog {
	return skillmodel.PreparationRegistryCatalog{Version: r.Version, WorkspaceID: r.WorkspaceID, OwnerEpoch: r.OwnerEpoch, ObservedAt: r.ObservedAt, Skills: skillmodel.CloneSkills(r.Skills), CatalogHash: r.CatalogHash}
}
func (r ObserveRegistryCatalogResult) Clone() ObserveRegistryCatalogResult {
	r.Skills = skillmodel.CloneSkills(r.Skills)
	return r
}
func RegistryCatalogHash(skills []*skillmodel.Skill) (string, error) {
	return skillmodel.RegistryCatalogHash(skills)
}
func (r ObserveRegistryCatalogResult) ValidateFor(c ObserveRegistryCatalogCommand) error {
	if c.Validate() != nil {
		return common.ErrConflict
	}
	return (skillmodel.PreparationRegistryCatalog{Version: r.Version, WorkspaceID: r.WorkspaceID, OwnerEpoch: r.OwnerEpoch, ObservedAt: r.ObservedAt, Skills: r.Skills, CatalogHash: r.CatalogHash}).ValidateFor(c.WorkspaceID)
}
