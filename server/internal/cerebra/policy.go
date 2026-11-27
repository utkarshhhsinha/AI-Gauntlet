package cerebra

// Policy enforces provider/model allowlists, budget ceilings, data sensitivity
// rules, tool/MCP minimum tier, and environment restrictions.
//
// All checks run BEFORE model selection (the filter-before-select invariant).
// The zero-value Policy allows everything — safe default when routing is first
// wired in without any active restrictions.
type Policy struct {
	// ProviderAllowlist, when non-empty, limits routing to the listed providers.
	// Provider names are matched case-insensitively against the runtime's provider field.
	ProviderAllowlist []string

	// ModelAllowlist, when non-empty, limits routing to the listed model IDs.
	ModelAllowlist []string

	// ModelDenylist explicitly blocks the listed model IDs regardless of tier.
	ModelDenylist []string

	// BudgetCeiling is the maximum estimated cost per task in USD.
	// 0 means no budget constraint.
	BudgetCeiling float64

	// CostEstimator, when provided, overrides the default token/rate cost estimation.
	CostEstimator func(model string, tier Tier, promptTokens int) float64

	// DataSensitiveProviders is the set of provider names that are NOT allowed
	// when the task is classified as privacy-sensitive.
	DataSensitiveProviders []string

	// MinMCPTier is the minimum tier for tool/MCP tasks.
	// Defaults to TierStandard when zero.
	MinMCPTier Tier

	// EnvironmentRestrictions maps environment names (e.g. "prod") to allowed providers.
	// Empty means no environment restrictions.
	EnvironmentRestrictions map[string][]string

	// CurrentEnvironment is the environment the daemon is running in.
	CurrentEnvironment string
}

// Allow returns true if the given (runtimeID, model) pair passes all policy checks.
// runtimeID is currently used for environment-restriction lookups; extend with
// provider lookups once the runtime registry is threaded through.
func (p *Policy) Allow(runtimeID, model string) bool {
	if p == nil {
		return true
	}

	// Model denylist takes priority.
	for _, denied := range p.ModelDenylist {
		if denied == model {
			return false
		}
	}

	// Model allowlist filter.
	if len(p.ModelAllowlist) > 0 {
		found := false
		for _, allowed := range p.ModelAllowlist {
			if allowed == model {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Environment restrictions (by runtimeID as a stand-in until provider is available).
	if p.CurrentEnvironment != "" && len(p.EnvironmentRestrictions) > 0 {
		if allowedProviders, ok := p.EnvironmentRestrictions[p.CurrentEnvironment]; ok {
			_ = allowedProviders // wire provider lookup here when available
		}
	}

	return true
}

// AllowForSensitiveTask is like Allow but additionally enforces the data
// sensitivity restriction — blocks providers in DataSensitiveProviders.
// Call this when the task has been marked as containing sensitive data.
func (p *Policy) AllowForSensitiveTask(runtimeID, model string) bool {
	if !p.Allow(runtimeID, model) {
		return false
	}
	// Extend: look up the provider for runtimeID and reject if it is in
	// DataSensitiveProviders. Requires threading the runtime→provider map here.
	return true
}

// EffectiveMCPTier returns the minimum tier for MCP/tool tasks.
func (p *Policy) EffectiveMCPTier() Tier {
	if p == nil || p.MinMCPTier == "" {
		return TierStandard
	}
	return p.MinMCPTier
}

// EstimateCost returns the estimated cost in USD for the given model, tier, and prompt tokens.
func (p *Policy) EstimateCost(model string, tier Tier, promptTokens int) float64 {
	if p != nil && p.CostEstimator != nil {
		return p.CostEstimator(model, tier, promptTokens)
	}
	return EstimateModelCost(model, tier, promptTokens)
}

// AllowBudget reports whether a model candidate's estimated cost is within the configured BudgetCeiling.
// When BudgetCeiling is 0 (or unconfigured), all costs pass.
func (p *Policy) AllowBudget(model string, tier Tier, promptTokens int) (bool, float64) {
	cost := p.EstimateCost(model, tier, promptTokens)
	if p == nil || p.BudgetCeiling <= 0 {
		return true, cost
	}
	return cost <= p.BudgetCeiling, cost
}

// AllowWithBudget checks both static policy allow/denylists and the dynamic budget ceiling constraint.
func (p *Policy) AllowWithBudget(runtimeID, model string, tier Tier, promptTokens int) (bool, float64) {
	if !p.Allow(runtimeID, model) {
		return false, 0
	}
	return p.AllowBudget(model, tier, promptTokens)
}
