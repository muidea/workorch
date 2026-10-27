package skillhubpreparation

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	skillcommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"gopkg.in/yaml.v3"
)

// SkillHubPreparationMetadata is a pure projection of installed SKILL.md.
// It neither reads fallback files nor establishes availability. The caller
// retains the original file bytes, including all unconsumed frontmatter.
type SkillHubPreparationMetadata struct {
	Name                   string
	Version                string
	Description            string
	TriggerRules           []string
	ReferenceRefs          []string
	RequiredToolKinds      []string
	RequiredMCPServerRefs  []string
	RequiredCapabilityRefs []string
	Constraints            []string
}

// ParsePreparationSkillHubMetadata deliberately does not use the permissive
// management-list parser. Ambiguous YAML cannot silently become an empty
// allowlist or lose a resource requirement. YAML aliases, merge keys, custom
// tags and non-string mapping keys are unsupported and fail closed.
func ParsePreparationSkillHubMetadata(raw []byte) (SkillHubPreparationMetadata, error) {
	var value SkillHubPreparationMetadata
	if len(raw) == 0 || len(raw) > skillcommon.SkillInstructionMaxBytes || !utf8.Valid(raw) {
		return value, common.ErrConflict
	}
	lines := bytes.Split(raw, []byte("\n"))
	if !bytes.Equal(bytes.TrimSuffix(lines[0], []byte("\r")), []byte("---")) {
		// Headerless instructions remain supported, but a malformed opening
		// delimiter is not silently interpreted as a headerless document.
		if bytes.HasPrefix(bytes.TrimSpace(lines[0]), []byte("---")) || bytes.HasPrefix(bytes.TrimSpace(lines[0]), []byte("\xef\xbb\xbf---")) {
			return value, common.ErrConflict
		}
		return value, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if bytes.Equal(bytes.TrimSuffix(lines[i], []byte("\r")), []byte("---")) {
			end = i
			break
		}
	}
	if end < 0 {
		return value, common.ErrConflict
	}
	decoder := yaml.NewDecoder(bytes.NewReader(bytes.Join(lines[1:end], []byte("\n"))))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return value, common.ErrConflict
	}
	var trailing yaml.Node
	if decoder.Decode(&trailing) != io.EOF {
		return value, common.ErrConflict
	}
	count := 0
	if !validPreparationMetadataNode(document.Content[0], 0, &count) {
		return value, common.ErrConflict
	}
	seen := make(map[string]bool)
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key, node := root.Content[i].Value, root.Content[i+1]
		canonical := preparationMetadataKey(key)
		if canonical == "" {
			// Do not turn a differently cased requirement into ignored metadata.
			for _, known := range []string{"name", "version", "description", "constraints", "triggerRules", "trigger_rules", "triggers", "referenceRefs", "reference_refs", "requiredToolKinds", "required_tool_kinds", "requiredTools", "required_tools", "requiredMCPServerRefs", "required_mcp_server_refs", "requiredMcpServers", "required_mcp_servers", "requiredCapabilityRefs", "required_capability_refs"} {
				if strings.EqualFold(key, known) {
					return SkillHubPreparationMetadata{}, common.ErrConflict
				}
			}
			continue // Unknown metadata remains in the original diagnostic bytes.
		}
		if seen[canonical] {
			return SkillHubPreparationMetadata{}, common.ErrConflict
		}
		seen[canonical] = true
		var scalar *string
		var list *[]string
		switch canonical {
		case "name":
			scalar = &value.Name
		case "version":
			scalar = &value.Version
		case "description":
			scalar = &value.Description
		case "triggerRules":
			list = &value.TriggerRules
		case "referenceRefs":
			list = &value.ReferenceRefs
		case "requiredToolKinds":
			list = &value.RequiredToolKinds
		case "requiredMCPServerRefs":
			list = &value.RequiredMCPServerRefs
		case "requiredCapabilityRefs":
			list = &value.RequiredCapabilityRefs
		case "constraints":
			list = &value.Constraints
		}
		if scalar != nil {
			// Versions such as unquoted 1.0 are accepted as their exact
			// scalar spelling, never numerically decoded/reformatted.
			versionNumber := canonical == "version" && (node.Tag == "!!int" || node.Tag == "!!float")
			if node.Kind != yaml.ScalarNode || node.Tag != "!!str" && !versionNumber {
				return SkillHubPreparationMetadata{}, common.ErrConflict
			}
			*scalar = node.Value
			continue
		}
		parsed, err := preparationMetadataList(node)
		if err != nil {
			return SkillHubPreparationMetadata{}, err
		}
		*list = parsed
	}
	if len(value.Name) > skillcommon.SkillNameMaxBytes || len(value.Version) > skillcommon.SkillVersionMaxBytes || len(value.Description) > skillcommon.SkillDescriptionMaxBytes {
		return SkillHubPreparationMetadata{}, common.ErrConflict
	}
	return value, nil
}

func validPreparationMetadataNode(node *yaml.Node, depth int, count *int) bool {
	*count = *count + 1
	if depth > 16 || *count > 4096 || node.Anchor != "" || node.Alias != nil || node.Kind == yaml.AliasNode {
		return false
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" || len(node.Content)%2 != 0 {
			return false
		}
		keys := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" || keys[key.Value] {
				return false
			}
			keys[key.Value] = true
		}
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return false
		}
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!null", "!!bool", "!!int", "!!float", "!!timestamp":
		default:
			return false
		}
	default:
		return false
	}
	for _, child := range node.Content {
		if !validPreparationMetadataNode(child, depth+1, count) {
			return false
		}
	}
	return true
}

func preparationMetadataKey(key string) string {
	switch key {
	case "name", "version", "description", "constraints":
		return key
	case "triggerRules", "trigger_rules", "triggers":
		return "triggerRules"
	case "referenceRefs", "reference_refs":
		return "referenceRefs"
	case "requiredToolKinds", "required_tool_kinds", "requiredTools", "required_tools":
		return "requiredToolKinds"
	case "requiredMCPServerRefs", "required_mcp_server_refs", "requiredMcpServers", "required_mcp_servers":
		return "requiredMCPServerRefs"
	case "requiredCapabilityRefs", "required_capability_refs":
		return "requiredCapabilityRefs"
	}
	return ""
}

func preparationMetadataList(node *yaml.Node) ([]string, error) {
	var values []string
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		values = strings.Split(node.Value, ",") // Existing scalar-list convention.
	} else if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode || child.Tag != "!!str" {
				return nil, common.ErrConflict
			}
			values = append(values, child.Value)
		}
	} else {
		return nil, common.ErrConflict
	}
	if len(values) > skillcommon.SkillListValueMaxCount {
		return nil, common.ErrConflict
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > skillcommon.SkillListValueMaxBytes {
			return nil, common.ErrConflict
		}
		result = append(result, value)
	}
	return result, nil
}
