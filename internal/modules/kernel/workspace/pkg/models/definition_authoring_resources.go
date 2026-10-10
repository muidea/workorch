package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefinitionAuthoringResourceLimit    = 8
	DefinitionAuthoringResourceMaxBytes = 32 * 1024
	DefinitionAuthoringResourceTotal    = 128 * 1024
)

// DefinitionAuthoringResourceSnapshot is bounded data supplied for an owner
// derivation reservation. Workspace must verify File-origin content and its
// current revision before CAS; confirmation later must recheck the resources.
// A persisted snapshot alone is not an admission or provider grant.
type DefinitionAuthoringResourceSnapshot struct {
	Ref         string
	Content     string
	ContentHash string
}

func CloneDefinitionAuthoringResources(resources []DefinitionAuthoringResourceSnapshot) []DefinitionAuthoringResourceSnapshot {
	return append([]DefinitionAuthoringResourceSnapshot(nil), resources...)
}

func ValidateDefinitionAuthoringResources(input DefinitionAuthoringInput, resources []DefinitionAuthoringResourceSnapshot) error {
	if input.Validate() != nil || len(resources) != len(input.ResourceRefs) || len(resources) > DefinitionAuthoringResourceLimit {
		return fmt.Errorf("authoring resource snapshot does not match bounded input")
	}
	normalized := input.Normalized()
	total := 0
	for index, resource := range resources {
		if resource.Ref != normalized.ResourceRefs[index] || len(resource.Content) > DefinitionAuthoringResourceMaxBytes || !utf8.ValidString(resource.Content) || strings.ContainsRune(resource.Content, 0) {
			return fmt.Errorf("authoring resource snapshot ref or content invalid")
		}
		total += len(resource.Content)
		checksum := sha256.Sum256([]byte(resource.Content))
		if resource.ContentHash != "sha256:"+hex.EncodeToString(checksum[:]) {
			return fmt.Errorf("authoring resource snapshot content hash invalid")
		}
	}
	if total > DefinitionAuthoringResourceTotal {
		return fmt.Errorf("authoring resource snapshot exceeds total byte budget")
	}
	return nil
}

func DefinitionAuthoringResourcesHash(resources []DefinitionAuthoringResourceSnapshot) string {
	h := sha256.New()
	writeDefinitionHash(h, "workspace-authoring-resources-v1", fmt.Sprint(len(resources)))
	for _, resource := range resources {
		writeDefinitionHash(h, resource.Ref, resource.ContentHash)
	}
	return "authoring-resources-sha256:" + hex.EncodeToString(h.Sum(nil))
}
