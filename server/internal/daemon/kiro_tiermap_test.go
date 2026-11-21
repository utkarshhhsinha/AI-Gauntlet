package daemon

import (
	"context"
	"testing"

	"github.com/utkarshhhsinha/AI-Gauntlet/server/internal/cerebra"
	"github.com/utkarshhhsinha/AI-Gauntlet/server/pkg/agent"
)

// TestKiroTierMap_UsesLiveDiscovery verifies that deriveDynamicRuntimeTierMap
// calls listModels for the kiro provider and classifies the returned models
// dynamically into tiers — no hardcoded model names anywhere in the path.
func TestKiroTierMap_UsesLiveDiscovery(t *testing.T) {
	orig := listModels
	listModels = func(_ context.Context, provider string, _ agent.Command) (agent.Catalog, error) {
		if provider != "kiro" {
			t.Errorf("expected provider kiro, got %q", provider)
		}
		// Simulate what the real kiro-cli would return after login.
		return agent.Catalog{Models: []agent.Model{
			{ID: "claude-haiku-4.5", Provider: "anthropic"},
			{ID: "claude-sonnet-4", Provider: "anthropic"},
			{ID: "claude-sonnet-4.5", Provider: "anthropic"},
		}}, nil
	}
	defer func() { listModels = orig }()

	unavail := cerebra.NewUnavailabilityStore(0)
	tierMap := deriveDynamicRuntimeTierMap(context.Background(), "kiro", agent.Command{}, unavail, "rt-kiro-1")

	if len(tierMap) == 0 {
		t.Fatal("expected non-empty tier map for kiro with live models available")
	}
	// Tier map must only contain IDs from the discovered catalog.
	allowed := map[string]bool{
		"claude-haiku-4.5":  true,
		"claude-sonnet-4":   true,
		"claude-sonnet-4.5": true,
	}
	for tier, model := range tierMap {
		if !allowed[model] {
			t.Errorf("tier %s: got unexpected model %q — only live-discovered models should appear", tier, model)
		}
	}
	t.Logf("✅ Kiro tier map built from live discovery: %v", tierMap)
}

// TestKiroTierMap_NeverContainsOpenCodeOrOllamaModels verifies that even if
// listModels somehow returned cross-runtime models, the Kiro catalog would
// only contain what was actually returned by the kiro listModels call (not
// opencode/ollama probes, which are skipped for non-opencode providers).
func TestKiroTierMap_NeverContainsOpenCodeOrOllamaModels(t *testing.T) {
	orig := listModels
	listModels = func(_ context.Context, provider string, _ agent.Command) (agent.Catalog, error) {
		// Return only kiro-native models — the Ollama probe path is skipped
		// for kiro because p != "opencode" && p != "ollama".
		return agent.Catalog{Models: []agent.Model{
			{ID: "claude-haiku-4.5", Provider: "anthropic"},
			{ID: "deepseek-3.2", Provider: "deepseek"},
		}}, nil
	}
	defer func() { listModels = orig }()

	unavail := cerebra.NewUnavailabilityStore(0)
	tierMap := deriveDynamicRuntimeTierMap(context.Background(), "kiro", agent.Command{}, unavail, "rt-kiro-1")

	for tier, model := range tierMap {
		for _, prefix := range []string{"opencode/", "ollama/"} {
			if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
				t.Errorf("tier %s: kiro tier map contains cross-runtime model %q", tier, model)
			}
		}
	}
	t.Logf("✅ Kiro tier map is clean: %v", tierMap)
}

// TestKiroTierMap_ExhaustionReturnsEmpty verifies that when all live-discovered
// Kiro models are marked unavailable, deriveDynamicRuntimeTierMap returns an
// empty map — causing the failover loop to stop rather than crossing runtimes.
func TestKiroTierMap_ExhaustionReturnsEmpty(t *testing.T) {
	orig := listModels
	listModels = func(_ context.Context, _ string, _ agent.Command) (agent.Catalog, error) {
		return agent.Catalog{Models: []agent.Model{
			{ID: "claude-haiku-4.5", Provider: "anthropic"},
			{ID: "claude-sonnet-4", Provider: "anthropic"},
		}}, nil
	}
	defer func() { listModels = orig }()

	unavail := cerebra.NewUnavailabilityStore(0)
	ctx := context.Background()
	unavail.MarkUnavailable(ctx, "rt-kiro-exhausted", "claude-haiku-4.5", 0)
	unavail.MarkUnavailable(ctx, "rt-kiro-exhausted", "claude-sonnet-4", 0)

	tierMap := deriveDynamicRuntimeTierMap(ctx, "kiro", agent.Command{}, unavail, "rt-kiro-exhausted")
	for tier, model := range tierMap {
		if model != "" {
			t.Errorf("tier %s: expected empty map after full exhaustion, got %q", tier, model)
		}
	}
	t.Log("✅ Kiro tier map is empty after full exhaustion — failover stops correctly")
}

// TestDynamicTierMap_FallbackCatalogIgnored verifies that when listModels
// returns a Fallback catalog (static stand-in, not real runtime catalog),
// deriveDynamicRuntimeTierMap treats it as an empty result rather than routing
// with stale static IDs.
func TestDynamicTierMap_FallbackCatalogIgnored(t *testing.T) {
	orig := listModels
	listModels = func(_ context.Context, _ string, _ agent.Command) (agent.Catalog, error) {
		return agent.Catalog{
			Models:   []agent.Model{{ID: "grok-3", Provider: "xai"}},
			Fallback: true, // signals this is a static stand-in, not live
		}, nil
	}
	defer func() { listModels = orig }()

	unavail := cerebra.NewUnavailabilityStore(0)
	tierMap := deriveDynamicRuntimeTierMap(context.Background(), "grok", agent.Command{}, unavail, "rt-grok-1")

	// Fallback catalog should be ignored → empty tier map
	for tier, model := range tierMap {
		if model != "" {
			t.Errorf("tier %s: fallback catalog models must not enter tier map, got %q", tier, model)
		}
	}
	t.Log("✅ Fallback catalog correctly ignored — no stale static IDs in tier map")
}

// TestGoogleDynamicTierMap_LiveDiscovery verifies that deriveDynamicRuntimeTierMap
// with "google" discovers live Gemini models using the API key and classifies them into
// Simple, Standard, and Heavy tiers without any hardcoded lists.
func TestGoogleDynamicTierMap_LiveDiscovery(t *testing.T) {
	orig := listModels
	listModels = agent.ListModels
	defer func() { listModels = orig }()

	tierMap := deriveDynamicRuntimeTierMap(context.Background(), "google", agent.Command{}, nil, "rt-google-test")
	if len(tierMap) == 0 {
		t.Fatal("expected non-empty tier map for google provider")
	}

	t.Logf("✅ Google live dynamic tier map: Simple=%s, Standard=%s, Heavy=%s",
		tierMap[cerebra.TierSimple],
		tierMap[cerebra.TierStandard],
		tierMap[cerebra.TierHeavy],
	)

	if tierMap[cerebra.TierSimple] == "" || tierMap[cerebra.TierHeavy] == "" {
		t.Fatalf("expected all tiers populated for google: %+v", tierMap)
	}
}

