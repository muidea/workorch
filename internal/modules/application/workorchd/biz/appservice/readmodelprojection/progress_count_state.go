package readmodelprojection

func ProgressStateFromCount(count int, failed bool, completed bool) string {
	if failed {
		return "failed"
	}
	if completed || count > 0 {
		return "completed"
	}
	return "pending"
}
