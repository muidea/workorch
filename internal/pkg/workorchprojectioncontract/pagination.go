package workorchprojectioncontract

// CursorPage is the owner-neutral cursor metadata shared by bounded read
// projections. Concrete projections embed it so their resource collection
// keeps its protocol-specific JSON field name.
type CursorPage struct {
	Limit             int    `json:"limit"`
	CursorAfterID     string `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string `json:"nextCursorAfterId,omitempty"`
	HasMore           bool   `json:"hasMore"`
}
