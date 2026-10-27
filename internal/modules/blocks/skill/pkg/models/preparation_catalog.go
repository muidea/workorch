package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RegistryCatalogVersion = "workorch-preparation-skill-registry/v1"
const RegistryCatalogMaxItems = 1024
const RegistryCatalogMaxBytes = 8 * 1024 * 1024

// One complete registry read, never a merged SkillHub catalog, a selected Skill
// proof or readiness. OwnerEpoch changes on owner reconstruction; CatalogHash
// identifies content but does not authorize skipping full record comparison.
type PreparationRegistryCatalog struct {
	Version     string
	WorkspaceID common.WorkspaceID
	OwnerEpoch  string
	ObservedAt  time.Time
	Skills      []*Skill
	CatalogHash string
}

func (r PreparationRegistryCatalog) Clone() PreparationRegistryCatalog {
	r.Skills = CloneSkills(r.Skills)
	return r
}

func RegistryCatalogHash(skills []*Skill) (string, error) {
	if skills == nil || len(skills) > RegistryCatalogMaxItems {
		return "", common.ErrConflict
	}
	var previous common.SkillID
	bytes := 0
	for _, item := range skills {
		if ValidateRegistrySnapshotSkill(item) != nil || previous != "" && item.ID <= previous {
			return "", common.ErrConflict
		}
		previous = item.ID
		bytes += item.Content().ByteSize()
		if bytes > RegistryCatalogMaxBytes {
			return "", common.ErrConflict
		}
	}
	encoded, err := json.Marshal(skills)
	if err != nil || len(encoded) > RegistryCatalogMaxBytes {
		return "", common.ErrConflict
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func (r PreparationRegistryCatalog) ValidateFor(workspaceID common.WorkspaceID) error {
	id := string(workspaceID)
	if id == "" || len(id) > 256 || !utf8.ValidString(id) || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") || r.Version != RegistryCatalogVersion || r.WorkspaceID != workspaceID || r.ObservedAt.IsZero() || !validRegistryEpoch(r.OwnerEpoch) {
		return common.ErrConflict
	}
	hash, err := RegistryCatalogHash(r.Skills)
	if err != nil || r.CatalogHash != hash {
		return common.ErrConflict
	}
	encoded, err := json.Marshal(r)
	if err != nil || len(encoded) > RegistryCatalogMaxBytes {
		return common.ErrConflict
	}
	return nil
}

func validRegistryEpoch(value string) bool {
	const prefix = "skill-preparation-epoch:"
	if !strings.HasPrefix(value, prefix) || len(value) > 128 {
		return false
	}
	suffix := strings.TrimPrefix(value, prefix)
	if len(suffix) < 26 {
		return false
	}
	for _, ch := range suffix {
		if !(ch >= 'A' && ch <= 'Z' || ch >= '2' && ch <= '7') {
			return false
		}
	}
	return true
}
