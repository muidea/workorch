package workspace

import (
	"fmt"
	"strconv"
	"strings"
)

// SpecificationDoDSources resolves original ordered identities without execution facts.
func SpecificationDoDSources(items []DefinitionCandidateItem) (map[string]string, bool) {
	result := make(map[string]string, len(items))
	groups := map[string]map[int]string{"user": {}, "template": {}, "generated": {}}
	derived, formal := false, false
	formalOrdinals := make(map[int]bool)
	for _, item := range items {
		if strings.HasPrefix(item.CandidateItemRef, "derived-dod:") {
			derived = true
			parts := strings.Split(item.CandidateItemRef, ":")
			if len(parts) != 3 {
				return nil, false
			}
			group, exists := groups[parts[1]]
			ordinal, err := strconv.Atoi(parts[2])
			if !exists || err != nil || ordinal < 1 || parts[2] != fmt.Sprintf("%06d", ordinal) || group[ordinal] != "" {
				return nil, false
			}
			group[ordinal] = item.CandidateItemRef
			continue
		}
		formal = true
		parts := strings.Split(item.CandidateItemRef, ":item:")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "dod:") || len(parts[0]) <= len("dod:") {
			return nil, false
		}
		ordinal, err := strconv.Atoi(parts[1])
		if err != nil || ordinal < 1 || parts[1] != fmt.Sprintf("%06d", ordinal) || formalOrdinals[ordinal] {
			return nil, false
		}
		formalOrdinals[ordinal] = true
		result[item.CandidateItemRef] = fmt.Sprintf("dod-%03d", ordinal)
	}
	if derived && formal {
		return nil, false
	}
	if derived {
		ordinal := 0
		for _, kind := range []string{"user", "template", "generated"} {
			for n := 1; n <= len(groups[kind]); n++ {
				ref := groups[kind][n]
				if ref == "" {
					return nil, false
				}
				ordinal++
				result[ref] = fmt.Sprintf("dod-%03d", ordinal)
			}
		}
	} else {
		for n := 1; n <= len(items); n++ {
			if !formalOrdinals[n] {
				return nil, false
			}
		}
	}
	return result, len(result) == len(items)
}
