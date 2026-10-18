package chat

import (
	"fmt"
	agent "github.com/simjay/plous/internal/ai"
	ai "github.com/simjay/plous/internal/ai/model"
	"strings"
)

func EstimateTokens(messages []ai.Message) int { return agent.Estimate(messages) }

func CompactTokens(tokens int) string {
	if tokens >= 1_000_000 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(tokens)/1_000_000), "0"), ".") + "M"
	}
	if tokens >= 1_000 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", float64(tokens)/1_000), "0"), ".") + "k"
	}
	return fmt.Sprint(tokens)
}
