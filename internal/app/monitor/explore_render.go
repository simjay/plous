package monitor

import (
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

var ExploreTabs = []string{"Describe", "YAML", "Events", "Related", "Logs"}

var LogRanges = []struct {
	Label   string
	Seconds int64
}{
	{"Latest 200 lines", 0}, {"Last minute", 60}, {"Last 5 minutes", 300}, {"Last 15 minutes", 900}, {"Last hour", 3600}, {"Last 6 hours", 21600}, {"Last 24 hours", 86400},
}

func ResourceTitle(resource kubecommon.Resource) string {
	return kubernetes.ResourceTitle(resource)
}

func DisplayLogText(text string, timestamps bool) string {
	if timestamps {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		stamp, rest, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			lines[i] = rest
		}
	}
	return strings.Join(lines, "\n")
}
