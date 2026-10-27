// Package skillselection contains the bounded pure matching policy shared by
// activation presentation and the preparation persistence owner. No I/O.
package skillselection

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

const DefaultMaxAutoSelectedSkills = 5

type Candidate struct {
	ID, Name, SourceSkillID, Scope, Source string
	TriggerRules                           []string
}

// TypedSelection is selection-policy output, not inventory completeness,
// source validity, a context budget or resource readiness. It deliberately
// does not export the legacy display Evidence's map-based representation.
type TypedSelection struct {
	Policy  string
	Matches []TypedMatch
}

type TypedMatch struct {
	Candidate Candidate
	Score     int
	Reasons   []string
}

// AutoSelectTyped uses the same scoring and ordering as existing activation.
// The supplied full catalog must have unique exact IDs and supported scopes;
// errors cannot become "no match" or silently missing selected materials.
func AutoSelectTyped(candidates []Candidate, goal string, dod, allowedRefs []string) (TypedSelection, error) {
	value := TypedSelection{Policy: "goal_dod_auto_match"}
	if len(candidates) > 3*1024 || !utf8.ValidString(goal) {
		return value, common.ErrConflict
	}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == "" || strings.TrimSpace(candidate.ID) != candidate.ID || seen[candidate.ID] ||
			!utf8.ValidString(candidate.ID) || !utf8.ValidString(candidate.Name) || !utf8.ValidString(candidate.SourceSkillID) ||
			(candidate.Scope != workorchskillcontract.ScopeGlobal && candidate.Scope != workorchskillcontract.ScopeWorkspace) ||
			(candidate.Source != workorchskillcontract.SourceWorkOrchRegistry && candidate.Source != workorchskillcontract.SourceSkillHub) {
			return value, common.ErrConflict
		}
		if candidate.Source == workorchskillcontract.SourceWorkOrchRegistry && candidate.Scope != workorchskillcontract.ScopeGlobal ||
			candidate.Source == workorchskillcontract.SourceSkillHub && strings.TrimSpace(candidate.SourceSkillID) == "" {
			return value, common.ErrConflict
		}
		seen[candidate.ID] = true
		for _, rule := range candidate.TriggerRules {
			if strings.TrimSpace(rule) == "" || !utf8.ValidString(rule) {
				return value, common.ErrConflict
			}
		}
	}
	for _, values := range [][]string{dod, allowedRefs} {
		for _, text := range values {
			if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
				return value, common.ErrConflict
			}
		}
	}
	scores := collectScores(candidates, SearchText(goal, dod), allowedRefs)
	for _, scored := range scores[:min(len(scores), DefaultMaxAutoSelectedSkills)] {
		candidate := scored.Candidate
		candidate.TriggerRules = slices.Clone(candidate.TriggerRules)
		value.Matches = append(value.Matches, TypedMatch{Candidate: candidate, Score: scored.Score, Reasons: slices.Clone(scored.Reasons)})
	}
	return value, nil
}

func collectScores(candidates []Candidate, search string, allowedRefs []string) []TypedMatch {
	allowed := map[string]bool{}
	for _, ref := range allowedRefs {
		allowed[strings.TrimSpace(ref)] = true
	}
	scores := make([]TypedMatch, 0, len(candidates))
	for _, c := range candidates {
		if len(allowed) > 0 && !allowed[c.ID] && !allowed[c.SourceSkillID] {
			continue
		}
		score, reasons := ScoreCandidate(c, search)
		if score > 0 {
			scores = append(scores, TypedMatch{Candidate: c, Score: score, Reasons: reasons})
		}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if scores[i].Candidate.Scope != scores[j].Candidate.Scope {
			return scores[i].Candidate.Scope == workorchskillcontract.ScopeWorkspace
		}
		return scores[i].Candidate.ID < scores[j].Candidate.ID
	})
	return scores
}
