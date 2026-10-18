package ui

func SettingsChoiceLine(label string, active bool, width int) string {
	prefix := "  "
	style := CanvasStyle
	if active {
		prefix = "› "
		style = ShortcutStyle.Background(Highlight)
	}
	return style.Width(width).Render(Clipped(prefix+OneLine(label), width))
}
