//go:build agentintegration

package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/utkarshhhsinha/AI-Gauntlet/server/internal/cerebra"
	"github.com/utkarshhhsinha/AI-Gauntlet/server/pkg/agent"
)

// TestDiscoverKiroLive talks to the real, user-installed kiro-cli, so it lives
// behind the agentintegration build tag and runs only when explicitly enabled:
//
//	AI_GAUNTLET_RUN_REAL_AGENT_SMOKE=1 go test -tags=agentintegration ./internal/daemon -run TestDiscoverKiroLive
func TestDiscoverKiroLive(t *testing.T) {
	if os.Getenv("AI_GAUNTLET_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("set AI_GAUNTLET_RUN_REAL_AGENT_SMOKE=1 to run against the real kiro-cli")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cat, err := agent.ListModels(ctx, "kiro", agent.NewCommand("kiro-cli", nil))
	t.Logf("Discover err: %v, count: %d", err, len(cat.Models))
	for _, m := range cat.Models {
		t.Logf("Discovered model: ID=%s, Label=%s, Tier=%s", m.ID, m.Label, cerebra.ClassifyModelTier(m.ID))
	}
	var modelIDs []string
	for _, m := range cat.Models {
		modelIDs = append(modelIDs, m.ID)
	}
	tierMap := cerebra.BuildTierMapFromCatalog(modelIDs)
	t.Logf("Dynamic TierMap: Simple=%s, Standard=%s, Heavy=%s",
		tierMap[cerebra.TierSimple],
		tierMap[cerebra.TierStandard],
		tierMap[cerebra.TierHeavy],
	)
	if tierMap[cerebra.TierSimple] != "claude-haiku-4.5" {
		t.Errorf("Expected Simple tier to be claude-haiku-4.5, got %s", tierMap[cerebra.TierSimple])
	}
	if tierMap[cerebra.TierStandard] != "claude-sonnet-4" {
		t.Errorf("Expected Standard tier to be claude-sonnet-4, got %s", tierMap[cerebra.TierStandard])
	}
	if tierMap[cerebra.TierHeavy] != "claude-sonnet-4.5" {
		t.Errorf("Expected Heavy tier to be claude-sonnet-4.5, got %s", tierMap[cerebra.TierHeavy])
	}
}
