package ui

func MoveCursor(cursor, count int, key string) int {
	switch key {
	case "up", "k":
		cursor--
	case "down", "j":
		cursor++
	case "pgup":
		cursor -= 10
	case "pgdown":
		cursor += 10
	case "home":
		cursor = 0
	case "end":
		cursor = count - 1
	}
	return max(0, min(cursor, max(0, count-1)))
}
