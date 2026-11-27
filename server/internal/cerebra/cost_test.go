package cerebra

import (
	"strings"
	"testing"
)

func TestCost_EstimatePromptTokens(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		minWords int
	}{
		{"empty prompt", "", 1},
		{"single word", "hello", 1},
		{"short sentence", "Fix the bug in the login button", 7},
		{"long prompt", strings.Repeat("code test refactor architect ", 50), 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimatePromptTokens(tc.prompt)
			if got < tc.minWords {
				t.Errorf("EstimatePromptTokens(%q) = %d, expected at least %d", tc.prompt, got, tc.minWords)
			}
		})
	}
}

func TestCost_ModelRates(t *testing.T) {
	// 1. Known model: claude-opus-4.5 ($5/M in, $25/M out)
	inRate, outRate := ModelRates("claude-opus-4.5", TierHeavy)
	if inRate != 5.0 || outRate != 25.0 {
		t.Errorf("expected claude-opus-4.5 rates (5.0, 25.0), got (%v, %v)", inRate, outRate)
	}

	// 2. Known model: claude-haiku-4.5 ($1/M in, $5/M out)
	inRate, outRate = ModelRates("claude-haiku-4.5", TierSimple)
	if inRate != 1.0 || outRate != 5.0 {
		t.Errorf("expected claude-haiku-4.5 rates (1.0, 5.0), got (%v, %v)", inRate, outRate)
	}

	// 3. Local/free model (e.g. ollama:llama3 or qwen-free) -> 0.0, 0.0
	inRate, outRate = ModelRates("ollama/llama3:8b", TierStandard)
	if inRate != 0.0 || outRate != 0.0 {
		t.Errorf("expected local model to have 0 rates, got (%v, %v)", inRate, outRate)
	}

	// 4. Unknown model fallback by tier
	inRate, outRate = ModelRates("custom-enterprise-heavy", TierHeavy)
	if inRate != 15.0 || outRate != 30.0 {
		t.Errorf("expected TierHeavy fallback rates (15.0, 30.0), got (%v, %v)", inRate, outRate)
	}
}

func TestCost_EstimateModelCost(t *testing.T) {
	// For claude-opus-4.5 on TierHeavy (2000 output tokens):
	// promptTokens = 1000:
	// input: 1000 * 5.0 / 1e6 = 0.005 USD
	// output: 2000 * 25.0 / 1e6 = 0.050 USD
	// total = 0.055 USD
	cost := EstimateModelCost("claude-opus-4.5", TierHeavy, 1000)
	expected := 0.055
	if diff := cost - expected; diff < -1e-6 || diff > 1e-6 {
		t.Errorf("expected cost %v, got %v", expected, cost)
	}

	// Local model should always cost 0
	freeCost := EstimateModelCost("ollama/deepseek-coder", TierHeavy, 10000)
	if freeCost != 0.0 {
		t.Errorf("expected 0 cost for local model, got %v", freeCost)
	}
}
