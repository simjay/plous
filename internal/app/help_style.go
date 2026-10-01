package app

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

var helpTokens = regexp.MustCompile(`\S+`)

func helpKey(token string, leading bool) bool {
	token = strings.Trim(token, "(),.")
	token = strings.TrimSuffix(token, ":")
	if strings.HasPrefix(token, ":") || strings.HasPrefix(token, "Ctrl+") || strings.HasPrefix(token, "Shift+") {
		return true
	}
	switch token {
	case "/m", "/model", "//m", "//model", "Enter", "Esc", "Tab", "Space", "Backspace", "Delete", "Home", "End", "PgUp", "PgDn", "Arrows", "Left", "Right", "Up", "Down", "Paste", "bb", "ii", "ss", "cc", "hh", "11", "22", "33":
		return true
	}
	if strings.Contains(token, "/") && token != "/" {
		for _, key := range strings.Split(token, "/") {
			if !helpKey(key, leading) {
				return false
			}
		}
		return true
	}
	return leading && (utf8.RuneCountInString(token) == 1 || token == "gg" || token == "dd" || token == "yy" || token == "↑↓" || token == "←→")
}

func styledHelpText(text string) string {
	lines := strings.Split(plain(text), "\n")
	paragraph, description := true, false
	for i, line := range lines {
		if line == "" {
			paragraph, description = true, false
			continue
		}
		tokens := helpTokens.FindAllStringIndex(line, -1)
		keys := make([]bool, len(tokens))
		leading, hasKey := true, false
		for j, span := range tokens {
			token := line[span[0]:span[1]]
			if token == "·" {
				leading = true
				continue
			}
			if token == "/" && j > 0 || token == "or" && leading {
				continue
			}
			keys[j] = helpKey(token, leading)
			hasKey = hasKey || keys[j]
			leading = leading && keys[j]
		}
		style := lipgloss.NewStyle().Foreground(foreground)
		heading := paragraph && (!hasKey || line == "Enter text")
		if heading {
			lines[i] = sectionHeadingStyle.Render(line)
		} else {
			if !hasKey && !description || strings.HasSuffix(line, ".") {
				style = muted
			}
			var out strings.Builder
			last := 0
			for j, span := range tokens {
				if keys[j] {
					out.WriteString(style.Render(line[last:span[0]]))
					out.WriteString(shortcutStyle.Render(line[span[0]:span[1]]))
					last = span[1]
				}
			}
			out.WriteString(style.Render(line[last:]))
			lines[i] = out.String()
		}
		paragraph, description = false, leading && hasKey
	}
	return strings.Join(lines, "\n")
}
