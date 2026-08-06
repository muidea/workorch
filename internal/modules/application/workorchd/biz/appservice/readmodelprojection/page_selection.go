package readmodelprojection

// selectProjectedPage applies the common cursor window used by bounded run
// resource projections. Source identity and projection stay application-local
// so this helper never exposes owner models through a shared package.
func selectProjectedPage[S any, P any](
	items []*S,
	limit int,
	cursorAfterID string,
	sourceID func(*S) string,
	project func(*S) P,
) ([]P, string, bool) {
	started := cursorAfterID == ""
	selected := make([]P, 0, min(limit, len(items)))
	nextCursor := ""
	for _, item := range items {
		if item == nil {
			continue
		}
		itemID := sourceID(item)
		if !started {
			if itemID == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			return selected, nextCursor, true
		}
		selected = append(selected, project(item))
		nextCursor = itemID
	}
	return selected, nextCursor, false
}
