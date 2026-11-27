package cerebra

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestExhaustion_ThresholdsAndAutoRouting(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	classifier := HeuristicClassifier{}
	policy := &Policy{}
	sessionStore := NewSessionStore(time.Hour)
	unavailStore := NewUnavailabilityStore(time.Hour)

	runtimes := []RuntimeEntry{
		{
			RuntimeID: "rt-1",
			TierMap: TierMap{
				TierSimple:   "claude-haiku-4.5",
				TierStandard: "claude-sonnet-4.5",
				TierHeavy:    "claude-opus-4.5",
			},
		},
	}

	router := NewRouter(classifier, policy, sessionStore, unavailStore, logger, nil)

	// Step 1: Initial task routes to Simple (claude-haiku-4.5)
	res1 := router.Route(ctx, "Hello world", TaskMeta{IssueID: "issue-quota-1"}, runtimes, "default-model")
	if res1.Model != "claude-haiku-4.5" || res1.Tier != TierSimple {
		t.Fatalf("expected haiku/simple, got model=%q tier=%q", res1.Model, res1.Tier)
	}

	limit := ResolveModelTokenLimit(res1.Model, 0)
	if limit != 100_000 {
		t.Fatalf("expected 100k limit for haiku, got %d", limit)
	}

	// Step 2: Milestone 50% reached (50,000 tokens)
	check50 := CheckTokenQuota(50_000, limit)
	if check50.Milestone != Milestone50Percent || check50.IsExhausted {
		t.Errorf("expected 50%% milestone, got %q (exhausted=%v)", check50.Milestone, check50.IsExhausted)
	}

	// Step 3: Milestone 90% reached (90,000 tokens)
	check90 := CheckTokenQuota(90_000, limit)
	if check90.Milestone != Milestone90Percent || check90.IsExhausted {
		t.Errorf("expected 90%% milestone, got %q (exhausted=%v)", check90.Milestone, check90.IsExhausted)
	}

	// Step 4: Milestone 100% reached (100,000 tokens) -> EXHAUSTED!
	check100 := CheckTokenQuota(100_000, limit)
	if check100.Milestone != Milestone100Percent || !check100.IsExhausted {
		t.Errorf("expected 100%% milestone and exhausted=true, got %q (exhausted=%v)", check100.Milestone, check100.IsExhausted)
	}

	// Step 5: Auto-route on exhaustion!
	// Mark exhausted model unavailable in unavailStore and invalidate session pin
	unavailStore.MarkUnavailable(ctx, "rt-1", res1.Model, time.Hour)
	router.InvalidateSession(ctx, "issue-quota-1", "")

	// Router must automatically fail over to next best available model (claude-sonnet-4.5)
	res2 := router.Route(ctx, "Hello world", TaskMeta{IssueID: "issue-quota-1"}, runtimes, "default-model")
	if res2.Model != "claude-sonnet-4.5" || res2.Tier != TierStandard {
		t.Fatalf("expected auto-routing to sonnet/standard after haiku exhaustion, got %q, tier=%q", res2.Model, res2.Tier)
	}
	if !res2.FallbackUsed {
		t.Errorf("expected FallbackUsed to be true on auto-route")
	}

	// Step 6: When sonnet also exhausts its quota (200k tokens), router auto-routes to Opus
	sonnetLimit := ResolveModelTokenLimit(res2.Model, 0)
	checkSonnet100 := CheckTokenQuota(200_000, sonnetLimit)
	if !checkSonnet100.IsExhausted {
		t.Errorf("expected sonnet 200k to be exhausted")
	}

	unavailStore.MarkUnavailable(ctx, "rt-1", res2.Model, time.Hour)
	router.InvalidateSession(ctx, "issue-quota-1", "")

	res3 := router.Route(ctx, "Hello world", TaskMeta{IssueID: "issue-quota-1"}, runtimes, "default-model")
	if res3.Model != "claude-opus-4.5" || res3.Tier != TierHeavy {
		t.Fatalf("expected auto-routing to opus/heavy after sonnet exhaustion, got %q, tier=%q", res3.Model, res3.Tier)
	}
}
