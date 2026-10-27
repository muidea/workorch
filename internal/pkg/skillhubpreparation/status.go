// Package skillhubpreparation contains pure, bounded SkillHub preparation
// parsing contracts shared by observers and the persistence owner. No I/O.
package skillhubpreparation

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

type StatusItem struct {
	SkillID      string `json:"skill_id"`
	Agent        string `json:"agent"`
	LocalPath    string `json:"local_path"`
	TargetPath   string `json:"target_path"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	LocalVersion string `json:"local_version"`
	Message      string `json:"message"`
}
type Status struct {
	Scope       string       `json:"scope"`
	ProjectPath string       `json:"project_path"`
	Items       []StatusItem `json:"items"`
	Orphaned    []StatusItem `json:"orphaned"`
	Agents      []struct {
		Name string `json:"name"`
	} `json:"agents"`
}

// The original document, including unconsumed fields, remains in raw storage.
// This parser establishes only unambiguous inventory membership, never ready.
func ParseStatus(raw []byte, directory string, global bool) (Status, error) {
	var result Status
	if !ValidStatusDocument(raw) {
		return result, common.ErrConflict
	}
	var document struct {
		Status
		Count *int `json:"skill_count"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return result, common.ErrConflict
	}
	result = document.Status
	if result.Items == nil || len(result.Items)+len(result.Orphaned) > 1024 || len(result.Agents) > 32 || global && result.Scope != "global" || !global && (result.Scope != "project" || result.ProjectPath != directory) {
		return Status{}, common.ErrConflict
	}
	seen := map[[2]string]bool{}
	ids := map[string]bool{}
	for _, item := range result.Items {
		key := [2]string{item.SkillID, item.Agent}
		if !validIdentity(item.SkillID) || global && !validIdentity(item.Agent) || !global && item.Agent != "" || seen[key] {
			return Status{}, common.ErrConflict
		}
		seen[key] = true
		ids[item.SkillID] = true
	}
	if document.Count != nil && (*document.Count < 0 || *document.Count != len(ids)) {
		return Status{}, common.ErrConflict
	}
	for _, item := range result.Orphaned {
		key := [2]string{item.SkillID, item.Agent}
		if !global || !validIdentity(item.SkillID) || !validIdentity(item.Agent) || seen[key] {
			return Status{}, common.ErrConflict
		}
		seen[key] = true
	}
	agents := map[string]bool{}
	for _, agent := range result.Agents {
		if !global || !validIdentity(agent.Name) || agents[agent.Name] {
			return Status{}, common.ErrConflict
		}
		agents[agent.Name] = true
	}
	return result, nil
}
func ValidStatusDocument(raw []byte) bool {
	return len(raw) > 0 && len(raw) <= 1<<20 && utf8.Valid(raw) && validUnicodeEscapes(raw) && unambiguousJSON(raw)
}
func validIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func validUnicodeEscapes(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value < 0xd800 || value > 0xdbff {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return !inString
}
func unambiguousJSON(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var visit func(int) bool
	visit = func(depth int) bool {
		if depth > 32 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, container := token.(json.Delim)
		if !container {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				name = strings.ToLower(name)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !visit(depth + 1) {
					return false
				}
			}
		case '[':
			for decoder.More() {
				if !visit(depth + 1) {
					return false
				}
			}
		default:
			return false
		}
		end, err := decoder.Token()
		return err == nil && (delim == '{' && end == json.Delim('}') || delim == '[' && end == json.Delim(']'))
	}
	if !visit(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
