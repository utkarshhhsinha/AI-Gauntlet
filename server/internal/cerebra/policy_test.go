package cerebra

import "testing"

func TestPolicy_Allow(t *testing.T) {
	policy := &Policy{
		ModelAllowlist: []string{"claude-haiku-3-5", "claude-sonnet-4-5"},
		ModelDenylist:  []string{"claude-opus-legacy"},
	}

	if !policy.Allow("rt-1", "claude-haiku-3-5") {
		t.Errorf("expected haiku to be allowed")
	}

	if policy.Allow("rt-1", "gpt-4o") {
		t.Errorf("expected gpt-4o to be blocked by allowlist")
	}

	if policy.Allow("rt-1", "claude-opus-legacy") {
		t.Errorf("expected claude-opus-legacy to be blocked by denylist")
	}
}

func TestPolicy_BudgetCeiling(t *testing.T) {
	// claude-opus-4.5 on TierHeavy (~2000 output tokens):
	// 1000 input tokens => $0.055
	// claude-haiku-4.5 on TierSimple (~250 output tokens):
	// 1000 input tokens => 1000*1/1e6 + 250*5/1e6 = 0.001 + 0.00125 = $0.00225

	// Case 1: unconstrained (BudgetCeiling = 0)
	unconstrained := &Policy{}
	allowed, _ := unconstrained.AllowBudget("claude-opus-4.5", TierHeavy, 1000)
	if !allowed {
		t.Errorf("expected unconstrained policy to allow opus")
	}

	// Case 2: strict budget ceiling ($0.01 max)
	tightBudget := &Policy{
		BudgetCeiling: 0.01,
	}

	// Opus costs ~$0.055 -> must be blocked
	allowed, cost := tightBudget.AllowBudget("claude-opus-4.5", TierHeavy, 1000)
	if allowed {
		t.Errorf("expected tight budget to reject opus (cost=%v, ceiling=0.01)", cost)
	}

	// Haiku costs ~$0.00225 -> must be allowed
	allowed, cost = tightBudget.AllowBudget("claude-haiku-4.5", TierSimple, 1000)
	if !allowed {
		t.Errorf("expected tight budget to allow haiku (cost=%v, ceiling=0.01)", cost)
	}

	// Case 3: AllowWithBudget enforces both static denylist and budget
	denylistAndBudget := &Policy{
		BudgetCeiling: 0.05,
		ModelDenylist: []string{"claude-haiku-4.5"},
	}
	allowed, _ = denylistAndBudget.AllowWithBudget("rt-1", "claude-haiku-4.5", TierSimple, 100)
	if allowed {
		t.Errorf("expected denylisted haiku to be rejected regardless of low cost")
	}
}
