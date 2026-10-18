package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"hash/fnv"
	"strings"
	"time"
)

func PodLogFormatter(timestamps, sources bool) func(string) textview.Line {
	return func(line string) textview.Line {
		prefix, pod := "", ""
		if sources && strings.HasPrefix(line, "[") {
			if identity, rest, found := strings.Cut(line, "] "); found {
				if name, _, ok := strings.Cut(identity[1:], "/"); ok {
					prefix, pod, line = identity+"] ", name, rest
				}
			}
		}
		text := DisplayLogText(line, timestamps)
		styled := text
		if timestamps {
			styled = StyledLogTimestamps(text)
		}
		if prefix != "" {
			styled = PodLogSourceStyle(pod).Render(strings.TrimSuffix(prefix, " ")) + " " + styled
		}
		return textview.Line{Text: prefix + text, Styled: styled, ContinuationIndent: ansi.StringWidth(prefix)}
	}
}

func PodLogSourceStyle(pod string) lipgloss.Style {
	colors := [...]string{"#8DBBEB", "#7DCBB3", "#D9B879", "#BCA4EC", "#E6A0BF", "#8CCBD8", "#BFCD87", "#DCA68D"}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(pod))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colors[hash.Sum32()%uint32(len(colors))]))
}

func StyledLogTimestamps(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		stamp, rest, found := strings.Cut(line, " ")
		if found {
			if _, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				lines[i] = ui.Muted.Render(stamp) + " " + rest
			}
		}
	}
	return strings.Join(lines, "\n")
}

const ExploreLogBufferLimit = 128 * 1024
