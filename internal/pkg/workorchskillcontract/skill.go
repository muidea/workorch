// Package workorchskillcontract contains owner-neutral Skill provenance and
// canonical content helpers. It owns no Skill registry, catalog, EventHub
// topic, persistence, selection policy, or Runtime transition.
package workorchskillcontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	SourceWorkOrchRegistry = "workorch-registry"
	SourceSkillHub         = "skill-hub"

	ScopeGlobal    = "global"
	ScopeWorkspace = "workspace"
)

// Provenance identifies the immutable source snapshot from which a detached
// Skill context was built. Source-specific paths and lifecycle fields remain
// with their adapter; Runtime only needs this bounded identity.
type Provenance struct {
	Source      string
	Scope       string
	WorkspaceID common.WorkspaceID
	ContentHash string
}

func (p Provenance) Normalized() Provenance {
	p.Source = strings.TrimSpace(p.Source)
	p.Scope = strings.TrimSpace(p.Scope)
	p.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(p.WorkspaceID)))
	p.ContentHash = strings.ToLower(strings.TrimSpace(p.ContentHash))
	return p
}

func (p Provenance) Validate() error {
	p = p.Normalized()
	if !isSHA256(p.ContentHash) {
		return common.ErrInvalidArgument
	}
	if p.Source != SourceWorkOrchRegistry && p.Source != SourceSkillHub {
		return common.ErrInvalidArgument
	}
	switch p.Scope {
	case ScopeGlobal:
		if p.WorkspaceID != "" {
			return common.ErrInvalidArgument
		}
	case ScopeWorkspace:
		if p.WorkspaceID == "" {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	if p.Source == SourceWorkOrchRegistry && p.Scope != ScopeGlobal {
		return common.ErrInvalidArgument
	}
	return nil
}

func SameProvenance(left, right Provenance) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left == right
}

// Content is the canonical, source-independent Skill material covered by
// Provenance.ContentHash. It deliberately excludes adapter status/path/hash
// metadata so the same source content has one stable identity.
type Content struct {
	ID                     string
	Name                   string
	Version                string
	Description            string
	TriggerRules           []string
	Instruction            string
	ReferenceRefs          []string
	RequiredToolKinds      []string
	RequiredMCPServerRefs  []string
	RequiredCapabilityRefs []string
	Constraints            []string
}

func (c Content) Normalized() Content {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.Version = strings.TrimSpace(c.Version)
	c.Description = strings.TrimSpace(c.Description)
	c.TriggerRules = normalizeStrings(c.TriggerRules)
	c.Instruction = strings.TrimSpace(c.Instruction)
	c.ReferenceRefs = normalizeStrings(c.ReferenceRefs)
	c.RequiredToolKinds = normalizeStrings(c.RequiredToolKinds)
	c.RequiredMCPServerRefs = normalizeStrings(c.RequiredMCPServerRefs)
	c.RequiredCapabilityRefs = normalizeStrings(c.RequiredCapabilityRefs)
	c.Constraints = normalizeStrings(c.Constraints)
	return c
}

func (c Content) ByteSize() int {
	c = c.Normalized()
	total := len(c.ID) + len(c.Name) + len(c.Version) + len(c.Description) + len(c.Instruction)
	for _, values := range [][]string{c.TriggerRules, c.ReferenceRefs, c.RequiredToolKinds, c.RequiredMCPServerRefs, c.RequiredCapabilityRefs, c.Constraints} {
		for _, value := range values {
			total += len(value)
		}
	}
	return total
}

func StableContentHash(content Content) string {
	content = content.Normalized()
	h := sha256.New()
	writeString(h, content.ID)
	writeString(h, content.Name)
	writeString(h, content.Version)
	writeString(h, content.Description)
	writeStrings(h, content.TriggerRules)
	writeString(h, content.Instruction)
	writeStrings(h, content.ReferenceRefs)
	writeStrings(h, content.RequiredToolKinds)
	writeStrings(h, content.RequiredMCPServerRefs)
	writeStrings(h, content.RequiredCapabilityRefs)
	writeStrings(h, content.Constraints)
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	sort.Strings(ret)
	return ret
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeStrings(h hash.Hash, values []string) {
	writeUint64(h, uint64(len(values)))
	for _, value := range values {
		writeString(h, value)
	}
}

func writeString(h hash.Hash, value string) {
	writeUint64(h, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}

func writeUint64(h hash.Hash, value uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)
	_, _ = h.Write(buf[:])
}
