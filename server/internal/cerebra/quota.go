package cerebra

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultAgentTokenLimit is the standard per-agent task token budget (100k tokens).
const DefaultAgentTokenLimit int64 = 100_000

// QuotaMilestone indicates token usage percentage thresholds.
type QuotaMilestone string

const (
	MilestoneNone       QuotaMilestone = ""
	Milestone50Percent  QuotaMilestone = "50%"
	Milestone90Percent  QuotaMilestone = "90%"
	Milestone100Percent QuotaMilestone = "100%"
)

// TokenQuotaCheck carries the results of a token usage assessment.
type TokenQuotaCheck struct {
	TotalTokens     int64          `json:"total_tokens"`
	TokenLimit      int64          `json:"token_limit"`
	UsagePercentage float64        `json:"usage_percentage"`
	Milestone       QuotaMilestone `json:"milestone"`
	IsExhausted     bool           `json:"is_exhausted"`
	Message         string         `json:"message"`
}

// ResolveModelTokenLimit returns the maximum token limit for an agent using the given model.
func ResolveModelTokenLimit(model string, customLimit int64) int64 {
	if customLimit > 0 {
		return customLimit
	}
	if envLimitStr := os.Getenv("CEREBRA_TOKEN_LIMIT"); envLimitStr != "" {
		if limit, err := strconv.ParseInt(envLimitStr, 10, 64); err == nil && limit > 0 {
			return limit
		}
	}
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"), strings.Contains(lower, "sonnet"), strings.Contains(lower, "gpt-5"), strings.Contains(lower, "gpt-4o"):
		return 200_000
	case strings.Contains(lower, "haiku"), strings.Contains(lower, "flash"), strings.Contains(lower, "mini"):
		return 100_000
	default:
		return DefaultAgentTokenLimit
	}
}

// CheckTokenQuota evaluates token usage against the quota limit and identifies milestones (50%, 90%, 100%).
func CheckTokenQuota(usedTokens, tokenLimit int64) TokenQuotaCheck {
	if tokenLimit <= 0 {
		tokenLimit = DefaultAgentTokenLimit
	}
	if usedTokens < 0 {
		usedTokens = 0
	}

	pct := float64(usedTokens) / float64(tokenLimit) * 100.0

	var milestone QuotaMilestone
	var isExhausted bool
	var msg string

	switch {
	case pct >= 100.0:
		milestone = Milestone100Percent
		isExhausted = true
		msg = fmt.Sprintf("model has used 100%% of token quota (%d/%d tokens) - quota exhausted! Auto-routing to different available model", usedTokens, tokenLimit)
	case pct >= 90.0:
		milestone = Milestone90Percent
		isExhausted = false
		msg = fmt.Sprintf("model has used 90%% of token quota (%d/%d tokens) - warning: approaching token exhaustion", usedTokens, tokenLimit)
	case pct >= 50.0:
		milestone = Milestone50Percent
		isExhausted = false
		msg = fmt.Sprintf("model has used 50%% of token quota (%d/%d tokens)", usedTokens, tokenLimit)
	default:
		milestone = MilestoneNone
		isExhausted = false
		msg = fmt.Sprintf("model token usage normal (%.1f%% of quota used)", pct)
	}

	return TokenQuotaCheck{
		TotalTokens:     usedTokens,
		TokenLimit:      tokenLimit,
		UsagePercentage: pct,
		Milestone:       milestone,
		IsExhausted:     isExhausted,
		Message:         msg,
	}
}
