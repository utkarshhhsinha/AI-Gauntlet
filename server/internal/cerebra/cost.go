package cerebra

import (
	"strings"

	"github.com/utkarshhhsinha/AI-Gauntlet/server/internal/metrics"
)

// ExpectedOutputTokens returns the estimated completion tokens generated for a given task tier.
func ExpectedOutputTokens(tier Tier) int {
	switch tier {
	case TierHeavy:
		return 2000
	case TierStandard:
		return 800
	case TierSimple:
		fallthrough
	default:
		return 250
	}
}

// EstimatePromptTokens estimates the token count of a prompt string using standard word/character heuristics.
func EstimatePromptTokens(prompt string) int {
	clean := strings.TrimSpace(prompt)
	if len(clean) == 0 {
		return 1
	}

	words := tokenize(strings.ToLower(clean))
	wordEst := len(words) * 4 / 3
	charEst := len(clean) / 4

	est := wordEst
	if charEst > est {
		est = charEst
	}
	if est < 1 {
		est = 1
	}
	return est
}

// ModelRates returns the input and output rate per million tokens for a given model.
// If the model is an unmapped local model (e.g. Ollama, Llama, Qwen local), rates are 0.
// If the model is an unmapped commercial model, conservative tier-based default rates are used.
func ModelRates(model string, tier Tier) (inputPerM float64, outputPerM float64) {
	if price, ok := metrics.PriceForModelAlias(model); ok {
		return price.InputPerM, price.OutputPerM
	}

	lower := strings.ToLower(model)
	// Local / free runtimes have 0 cost.
	if strings.Contains(lower, "ollama") ||
		strings.Contains(lower, "local") ||
		strings.Contains(lower, "llama") ||
		strings.Contains(lower, "mistral") ||
		strings.Contains(lower, "gemma") ||
		strings.Contains(lower, ":free") ||
		strings.HasSuffix(lower, "-free") {
		return 0.0, 0.0
	}

	// Conservative fallback rates by tier for unknown cloud models
	switch tier {
	case TierHeavy:
		return 15.00, 30.00
	case TierStandard:
		return 3.00, 15.00
	case TierSimple:
		fallthrough
	default:
		return 1.00, 5.00
	}
}

// EstimateModelCost calculates the projected USD cost for a given model, tier, and prompt token count.
func EstimateModelCost(model string, tier Tier, promptTokens int) float64 {
	if promptTokens < 1 {
		promptTokens = 1
	}
	inRate, outRate := ModelRates(model, tier)
	inputCost := float64(promptTokens) * inRate / 1_000_000.0
	outTokens := ExpectedOutputTokens(tier)
	outputCost := float64(outTokens) * outRate / 1_000_000.0
	return inputCost + outputCost
}
