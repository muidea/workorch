package testkit

import "encoding/json"

// OwnerBoundFixtureDocument renders a synthetic fixture for the current contract. This is never
// used on real provider data or by a production decoder.
func OwnerBoundFixtureDocument(payload []byte) []byte {
	var document map[string]json.RawMessage
	if json.Unmarshal(payload, &document) != nil {
		return payload
	}
	changed := false
	if _, ok := document["AuthoringInputHash"]; ok {
		delete(document, "AuthoringInputHash")
		delete(document, "TemplateSnapshotHash")
		changed = true
	}
	for _, key := range []string{"mapping", "review"} {
		raw, ok := document[key]
		if !ok {
			continue
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil {
			continue
		}
		for _, field := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash", "Kind", "SpecificationHash", "GoalRequirement", "Dimensions", "Requirements"} {
			if _, ok := body[field]; ok {
				delete(body, field)
				changed = true
			}
		}
		if key == "mapping" {
			var rows []map[string]json.RawMessage
			if json.Unmarshal(body["Obligations"], &rows) == nil {
				for _, row := range rows {
					delete(row, "RequirementRef")
				}
				body["Obligations"], _ = json.Marshal(rows)
				changed = true
			}
		}
		document[key], _ = json.Marshal(body)
	}
	if !changed {
		return payload
	}
	out, err := json.Marshal(document)
	if err != nil {
		return payload
	}
	return out
}
