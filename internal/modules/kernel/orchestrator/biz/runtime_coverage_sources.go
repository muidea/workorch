package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var coverageMarkdownLeafPattern = regexp.MustCompile(`^\s*(?:[-*+]\s+(?:\[[ xX]\]\s*)?|\d+[.)]\s+)(.+?)\s*$`)

func coverageWorkItemSources(executionContext map[string]any) ([]coverageWorkItemSource, []map[string]any) {
	sources := make([]coverageWorkItemSource, 0)
	excluded := make([]map[string]any, 0)
	addExcluded := func(source, scope, path, text, reason string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		excluded = append(excluded, map[string]any{
			"source":         source,
			"sourceScope":    scope,
			"sourcePath":     path,
			"targetText":     text,
			"excludedReason": reason,
		})
	}
	addGoal := func(goal map[string]any, pathPrefix string) {
		if len(goal) == 0 {
			return
		}
		goalID := strings.TrimSpace(stringFromMap(goal, "id"))
		for idx, leaf := range goalCoverageLeafItems(stringFromMap(goal, "text")) {
			legacyID := fmt.Sprintf("%sgoal-leaf-%03d", coverageRequirementPrefix(goalID), idx+1)
			sources = append(sources, coverageWorkItemSource{
				Source:              "goal",
				SourceScope:         "goal_leaf",
				SourceGoalID:        goalID,
				SourceRequirementID: goalCoverageRequirementID(idx+1, normalizeDoDCoverageText(leaf.Text)),
				SourceAliases:       compactUniqueStrings([]string{legacyID}),
				SourcePath:          fmt.Sprintf("%sgoal.leaves[%d]", pathPrefix, idx),
				TargetText:          leaf.Text,
				ParentContext:       leaf.Parent,
			})
		}
	}
	addDoD := func(dod map[string]any, pathPrefix string) {
		if len(dod) == 0 {
			return
		}
		dodID := strings.TrimSpace(stringFromMap(dod, "id"))
		for idx, item := range stringSliceFromValue(dod["items"]) {
			item = strings.TrimSpace(item)
			if item == "" || coverageTextLooksContainer(item) {
				if item != "" {
					addExcluded("dod", "dod_item", fmt.Sprintf("%sdod.items[%d]", pathPrefix, idx), item, "container_source")
				}
				continue
			}
			sources = append(sources, coverageWorkItemSource{
				Source:              "dod",
				SourceScope:         "dod_item",
				SourceDoDID:         dodID,
				SourceRequirementID: fmt.Sprintf("%sdod-item-%03d", coverageRequirementPrefix(dodID), idx+1),
				SourcePath:          fmt.Sprintf("%sdod.items[%d]", pathPrefix, idx),
				TargetText:          item,
			})
		}
	}
	addGoal(mapFromValue(executionContext["goal"]), "")
	for idx, goal := range mapSliceFromValue(executionContext["goals"]) {
		addGoal(goal, fmt.Sprintf("goals[%d].", idx))
	}
	addDoD(mapFromValue(executionContext["dod"]), "")
	for idx, dod := range mapSliceFromValue(executionContext["dods"]) {
		addDoD(dod, fmt.Sprintf("dods[%d].", idx))
	}
	sources = append(sources, profileCoverageWorkItemSources(executionContext)...)
	sources = append(sources, failedCoverageWorkItemSources(executionContext)...)
	return sources, excluded
}

func profileCoverageWorkItemSources(executionContext map[string]any) []coverageWorkItemSource {
	executionContract := mapFromValue(executionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	if len(profileContracts) == 0 {
		return nil
	}
	profileNames := make([]string, 0, len(profileContracts))
	for name := range profileContracts {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	ret := make([]coverageWorkItemSource, 0)
	for _, profileName := range profileNames {
		profile := mapFromValue(profileContracts[profileName])
		for idx, item := range mapSliceFromValue(profile["acceptanceDimensions"]) {
			if !boolValue(item["required"]) {
				continue
			}
			id := firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "dimension"), stringFromMap(item, "key"), stringFromMap(item, "name"))
			text := firstNonEmptyString(id, stringFromMap(item, "description"))
			if strings.TrimSpace(text) == "" {
				continue
			}
			ret = append(ret, coverageWorkItemSource{
				Source:              "acceptance_dimension",
				SourceScope:         "acceptance_dimension",
				SourceRequirementID: id,
				SourcePath:          fmt.Sprintf("executionContract.profileContracts.%s.acceptanceDimensions[%d]", profileName, idx),
				TargetText:          text,
				EvidenceRole:        normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")),
			})
		}
		for idx, item := range mapSliceFromValue(profile["deliverableExpectations"]) {
			if !boolValue(item["required"]) {
				continue
			}
			id := firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "deliverable"), stringFromMap(item, "name"), stringFromMap(item, "path"))
			text := firstNonEmptyString(id, stringFromMap(item, "description"))
			if strings.TrimSpace(text) == "" {
				continue
			}
			ret = append(ret, coverageWorkItemSource{
				Source:              "deliverable",
				SourceScope:         "deliverable",
				SourceRequirementID: id,
				SourcePath:          fmt.Sprintf("executionContract.profileContracts.%s.deliverableExpectations[%d]", profileName, idx),
				TargetText:          text,
				EvidenceRole:        normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")),
			})
		}
	}
	return ret
}

func failedCoverageWorkItemSources(executionContext map[string]any) []coverageWorkItemSource {
	var ret []coverageWorkItemSource
	for _, item := range mapSliceFromValue(executionContext["missingDoDItems"]) {
		if source := failedCoverageSourceFromItem("dod", "dod_item", "missingDoDItems", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	for _, item := range mapSliceFromValue(executionContext["missingGoalCoverage"]) {
		if source := failedCoverageSourceFromItem("goal", "goal_leaf", "missingGoalCoverage", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	for _, item := range mapSliceFromValue(executionContext["missingDeliverableCoverage"]) {
		if source := failedCoverageSourceFromItem("deliverable", "deliverable", "missingDeliverableCoverage", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	return ret
}

func failedCoverageSourceFromItem(source, scope, pathPrefix string, item map[string]any) coverageWorkItemSource {
	text := firstNonEmptyString(
		stringFromMap(item, "targetText"),
		stringFromMap(item, "item"),
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "requirement"),
		stringFromMap(item, "id"),
		stringFromMap(item, "deliverable"),
		stringFromMap(item, "name"),
		stringFromMap(item, "path"),
	)
	text = strings.TrimSpace(text)
	if text == "" || coverageTextLooksContainer(text) {
		return coverageWorkItemSource{}
	}
	id := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), stringFromMap(item, "id"), stringFromMap(item, "coverageTargetToken")))
	return coverageWorkItemSource{
		Source:              source,
		SourceScope:         scope,
		SourceRequirementID: id,
		SourcePath:          pathPrefix + "." + coverageShortHash(item),
		TargetText:          text,
	}
}

func coverageWorkItemSourceKey(source coverageWorkItemSource) string {
	if strings.TrimSpace(source.Source) == "" || strings.TrimSpace(source.TargetText) == "" {
		return ""
	}
	id := strings.TrimSpace(source.SourceRequirementID)
	if id == "" {
		id = strings.TrimSpace(source.SourcePath)
	}
	if id == "" {
		id = coverageShortHash(source.TargetText)
	}
	return strings.Join([]string{source.Source, source.SourceScope, id}, ":")
}

func coverageWorkItemSourceAliasKeys(source coverageWorkItemSource) []string {
	ret := make([]string, 0, len(source.SourceAliases))
	for _, alias := range source.SourceAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		ret = append(ret, strings.Join([]string{source.Source, source.SourceScope, alias}, ":"))
	}
	return compactUniqueStrings(ret)
}

func coverageWorkItemTextKey(source, scope, text string) string {
	source = strings.TrimSpace(source)
	scope = strings.TrimSpace(scope)
	normalized := normalizeDoDCoverageText(text)
	if source == "" || scope == "" || normalized == "" {
		return ""
	}
	return strings.Join([]string{source, scope, normalized}, ":")
}

func coverageRequirementPrefix(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return id + "-"
}

type goalCoverageLeafItem struct {
	Text   string
	Parent string
}

func goalCoverageLeafItems(text string) []goalCoverageLeafItem {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	ret := make([]goalCoverageLeafItem, 0)
	parent := ""
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			parent = strings.TrimSpace(strings.TrimLeft(line, "# "))
			continue
		}
		if isMarkdownTableSeparatorLine(line) {
			continue
		}
		if isMarkdownTableRowLine(line) {
			if nextNonEmptyLineIsMarkdownTableSeparator(lines, idx) {
				continue
			}
			if normalized := normalizeDoDCoverageText(line); normalized != "" {
				ret = append(ret, goalCoverageLeafItem{Text: line, Parent: parent})
			}
			continue
		}
		if match := coverageMarkdownLeafPattern.FindStringSubmatch(line); len(match) == 2 {
			leaf := strings.TrimSpace(match[1])
			if leaf != "" && !coverageTextLooksContainer(leaf) {
				ret = append(ret, goalCoverageLeafItem{Text: leaf, Parent: parent})
			}
			continue
		}
		if coverageTextLooksContainer(line) {
			parent = strings.TrimSuffix(line, ":")
			continue
		}
		line = trimDoDItemText(line)
		if line != "" && classifyDoDItemKind(line) == doDItemKindLeaf {
			ret = append(ret, goalCoverageLeafItem{Text: line, Parent: parent})
		}
	}
	if len(ret) == 0 && !coverageTextLooksContainer(text) {
		ret = append(ret, goalCoverageLeafItem{Text: text})
	}
	return ret
}

func coverageTextLooksContainer(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || strings.HasPrefix(text, "#") || strings.HasSuffix(text, ":") || strings.HasSuffix(text, "：")
}

func isMarkdownTableRowLine(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "|") {
		return false
	}
	return strings.HasPrefix(line, "|") || strings.HasSuffix(line, "|")
}

func nextNonEmptyLineIsMarkdownTableSeparator(lines []string, idx int) bool {
	for next := idx + 1; next < len(lines); next++ {
		line := strings.TrimSpace(lines[next])
		if line == "" {
			continue
		}
		return isMarkdownTableSeparatorLine(line)
	}
	return false
}

func isMarkdownTableSeparatorLine(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "|") {
		return false
	}
	line = strings.Trim(line, "|")
	cells := strings.Split(line, "|")
	if len(cells) == 0 {
		return false
	}
	hasSeparator := false
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		dashes := 0
		for _, r := range cell {
			switch r {
			case '-':
				dashes++
			case ':':
			default:
				return false
			}
		}
		if dashes < 3 {
			return false
		}
		hasSeparator = true
	}
	return hasSeparator
}
