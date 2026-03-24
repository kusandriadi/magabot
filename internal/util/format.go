package util

// BoolIcon returns ✅ for true and ❌ for false.
func BoolIcon(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}
