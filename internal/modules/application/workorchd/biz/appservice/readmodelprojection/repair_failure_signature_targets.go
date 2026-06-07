package readmodelprojection

import "strings"

func readModelTargetsFromFailureSignature(signature string) []string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return nil
	}
	var ret []string
	for _, segment := range strings.Split(signature, "|") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "failureTargets":
			for _, target := range strings.Split(value, ",") {
				if target = strings.TrimSpace(target); target != "" {
					ret = append(ret, target)
				}
			}
		case "targetRef", "conflictRef":
			if value != "" {
				ret = append(ret, value)
			}
		}
	}
	return compactUniqueStrings(ret)
}
