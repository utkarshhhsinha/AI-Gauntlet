package cerebra

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestRouter_FilterBeforeSelect_And_Fallback(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	classifier := HeuristicClassifier{}
	policy := &Policy{}
	session := NewSessionStore(time.Hour)
	unavail := NewUnavailabilityStore(time.Hour)

	router := NewRouter(classifier, policy, session, unavail, logger, nil)

	runtimes := []RuntimeEntry{
		{
			RuntimeID: "rt-1",
			TierMap: TierMap{
				TierSimple:   "claude-haiku-3-5",
				TierStandard: "claude-sonnet-4-5",
				TierHeavy:    "claude-opus-4-5",
			},
		},
	}

	// 1. Simple task routes to simple model
	res := router.Route(ctx, "Hello world", TaskMeta{}, runtimes, "default-model")
	if res.Model != "claude-haiku-3-5" || res.Tier != TierSimple {
		t.Fatalf("expected haiku/simple, got model=%q, tier=%q", res.Model, res.Tier)
	}

	// 2. Mark simple model unavailable -> should escalate to standard
	unavail.MarkUnavailable(ctx, "rt-1", "claude-haiku-3-5", time.Hour)

	res2 := router.Route(ctx, "Hello world", TaskMeta{}, runtimes, "default-model")
	if res2.Model != "claude-sonnet-4-5" || res2.Tier != TierStandard {
		t.Fatalf("expected escalation to sonnet/standard, got model=%q, tier=%q", res2.Model, res2.Tier)
	}

	// 3. Mark standard also unavailable -> simple should escalate to heavy
	unavail.MarkUnavailable(ctx, "rt-1", "claude-sonnet-4-5", time.Hour)

	res3 := router.Route(ctx, "Hello world", TaskMeta{}, runtimes, "default-model")
	if res3.Model != "claude-opus-4-5" || res3.Tier != TierHeavy {
		t.Fatalf("expected escalation to opus/heavy, got model=%q, tier=%q", res3.Model, res3.Tier)
	}

	// 4. Mark all unavailable -> hard fallback to default-model
	unavail.MarkUnavailable(ctx, "rt-1", "claude-opus-4-5", time.Hour)

	res4 := router.Route(ctx, "Hello world", TaskMeta{}, runtimes, "default-model")
	if res4.Model != "default-model" || !res4.FallbackUsed {
		t.Fatalf("expected hard fallback to default-model, got model=%q, fallback=%v", res4.Model, res4.FallbackUsed)
	}
}

func TestRouter_StandardAndHeavyFallbacks(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	classifier := HeuristicClassifier{}
	policy := &Policy{}

	// Test Heavy task falling back to Standard when Heavy is unavailable
	t.Run("heavy task falls back to standard", func(t *testing.T) {
		session := NewSessionStore(time.Hour)
		unavail := NewUnavailabilityStore(time.Hour)
		router := NewRouter(classifier, policy, session, unavail, logger, nil)

		runtimes := []RuntimeEntry{
			{
				RuntimeID: "rt-1",
				TierMap: TierMap{
					TierSimple:   "claude-haiku-3-5",
					TierStandard: "claude-sonnet-4-5",
					TierHeavy:    "claude-opus-4-5",
				},
			},
		}

		// Mark heavy unavailable
		unavail.MarkUnavailable(ctx, "rt-1", "claude-opus-4-5", time.Hour)

		res := router.Route(ctx, "architect and design distributed cache", TaskMeta{}, runtimes, "default-model")
		if res.Model != "claude-sonnet-4-5" || res.Tier != TierStandard {
			t.Fatalf("expected heavy to fallback to standard (sonnet), got %q, tier=%q", res.Model, res.Tier)
		}
	})

	// Test Standard task escalating to Heavy when Standard is unavailable
	t.Run("standard task escalates to heavy", func(t *testing.T) {
		session := NewSessionStore(time.Hour)
		unavail := NewUnavailabilityStore(time.Hour)
		router := NewRouter(classifier, policy, session, unavail, logger, nil)

		runtimes := []RuntimeEntry{
			{
				RuntimeID: "rt-1",
				TierMap: TierMap{
					TierSimple:   "claude-haiku-3-5",
					TierStandard: "claude-sonnet-4-5",
					TierHeavy:    "claude-opus-4-5",
				},
			},
		}

		// Mark standard unavailable
		unavail.MarkUnavailable(ctx, "rt-1", "claude-sonnet-4-5", time.Hour)

		res := router.Route(ctx, "debug this issue", TaskMeta{}, runtimes, "default-model")
		if res.Model != "claude-opus-4-5" || res.Tier != TierHeavy {
			t.Fatalf("expected standard to escalate to heavy (opus), got %q, tier=%q", res.Model, res.Tier)
		}
	})

	// Test Standard task with MCP tool floor does NOT fallback to Simple
	t.Run("standard tool task does not fallback to simple", func(t *testing.T) {
		session := NewSessionStore(time.Hour)
		unavail := NewUnavailabilityStore(time.Hour)
		router := NewRouter(classifier, policy, session, unavail, logger, nil)

		runtimes := []RuntimeEntry{
			{
				RuntimeID: "rt-1",
				TierMap: TierMap{
					TierSimple:   "claude-haiku-3-5",
					TierStandard: "claude-sonnet-4-5",
					TierHeavy:    "claude-opus-4-5",
				},
			},
		}

		// Mark standard and heavy unavailable
		unavail.MarkUnavailable(ctx, "rt-1", "claude-sonnet-4-5", time.Hour)
		unavail.MarkUnavailable(ctx, "rt-1", "claude-opus-4-5", time.Hour)

		meta := TaskMeta{WillUseMCPTools: true}
		res := router.Route(ctx, "debug this issue", meta, runtimes, "default-model")
		if res.Model != "default-model" || !res.FallbackUsed {
			t.Fatalf("expected hard fallback to default-model because simple is forbidden for tools, got %q", res.Model)
		}
	})
}

func TestRouter_SessionAffinity(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	classifier := HeuristicClassifier{}
	policy := &Policy{}
	session := NewSessionStore(time.Hour)
	unavail := NewUnavailabilityStore(time.Hour)

	router := NewRouter(classifier, policy, session, unavail, logger, nil)

	runtimes := []RuntimeEntry{
		{
			RuntimeID: "rt-1",
			TierMap: TierMap{
				TierStandard: "claude-sonnet-4-5",
			},
		},
	}

	meta := TaskMeta{IssueID: "issue-123"}
	res1 := router.Route(ctx, "debug this issue", meta, runtimes, "default-model")
	if res1.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected sonnet, got %q", res1.Model)
	}

	// Next turn with same tier should hit session pin
	res2 := router.Route(ctx, "fix the failing test", meta, runtimes, "default-model")
	if res2.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected pinned sonnet, got %q", res2.Model)
	}
}

func TestRouter_BudgetCeilingEnforcement(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	classifier := HeuristicClassifier{}
	session := NewSessionStore(time.Hour)
	unavail := NewUnavailabilityStore(time.Hour)

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

	// 1. Without budget ceiling: "architect" prompt routes to Heavy (Opus)
	unconstrainedPolicy := &Policy{}
	routerUnconstrained := NewRouter(classifier, unconstrainedPolicy, session, unavail, logger, nil)
	res1 := routerUnconstrained.Route(ctx, "architect and design distributed cache", TaskMeta{}, runtimes, "default-model")
	if res1.Model != "claude-opus-4.5" || res1.Tier != TierHeavy {
		t.Fatalf("expected opus/heavy without budget constraint, got %q, tier=%q", res1.Model, res1.Tier)
	}
	if res1.EstimatedCost <= 0 {
		t.Errorf("expected estimated cost > 0, got %v", res1.EstimatedCost)
	}

	// 2. With $0.02 budget ceiling:
	// claude-opus-4.5 costs ~$0.055 -> exceeds ceiling!
	// claude-sonnet-4.5 on Standard costs ~$0.015 -> within ceiling!
	// Router must de-escalate to Standard (Sonnet) with budget rule annotation.
	budgetPolicy := &Policy{
		BudgetCeiling: 0.02,
	}
	var loggedEntry RoutingLogEntry
	routerWithBudget := NewRouter(classifier, budgetPolicy, session, unavail, logger, func(_ context.Context, entry RoutingLogEntry) {
		loggedEntry = entry
	})

	res2 := routerWithBudget.Route(ctx, "architect and design distributed cache", TaskMeta{TaskID: "task-budget-1"}, runtimes, "default-model")
	if res2.Model != "claude-sonnet-4.5" || res2.Tier != TierStandard {
		t.Fatalf("expected budget downgrade to sonnet/standard, got %q, tier=%q", res2.Model, res2.Tier)
	}
	if res2.PolicyReason != "budget_ceiling_enforced" {
		t.Errorf("expected PolicyReason 'budget_ceiling_enforced', got %q", res2.PolicyReason)
	}
	if res2.MatchedRule != "keyword:architect+budget_fallback_to_standard" {
		t.Errorf("expected matched rule to include '+budget_fallback_to_standard', got %q", res2.MatchedRule)
	}
	if res2.EstimatedCost > 0.02 {
		t.Errorf("chosen model cost (%v) must be <= budget ceiling (0.02)", res2.EstimatedCost)
	}

	// Verify routing log entry was captured with cost metrics
	time.Sleep(20 * time.Millisecond) // wait for async goroutine
	if loggedEntry.EstimatedCost != res2.EstimatedCost {
		t.Errorf("expected logged cost %v, got %v", res2.EstimatedCost, loggedEntry.EstimatedCost)
	}
	if loggedEntry.PolicyReason != "budget_ceiling_enforced" {
		t.Errorf("expected logged policy reason 'budget_ceiling_enforced', got %q", loggedEntry.PolicyReason)
	}

	// 3. Ultra-low budget ($0.005 max):
	// Opus (~$0.055) exceeds. Sonnet (~$0.015) exceeds.
	// Haiku (~$0.002) is within ceiling -> must de-escalate all the way to Simple (Haiku)!
	ultraLowBudget := &Policy{
		BudgetCeiling: 0.005,
	}
	routerUltraLow := NewRouter(classifier, ultraLowBudget, session, unavail, logger, nil)
	res3 := routerUltraLow.Route(ctx, "architect and design distributed cache", TaskMeta{}, runtimes, "default-model")
	if res3.Model != "claude-haiku-4.5" || res3.Tier != TierSimple {
		t.Fatalf("expected budget downgrade to haiku/simple, got %q, tier=%q", res3.Model, res3.Tier)
	}
	if res3.PolicyReason != "budget_ceiling_enforced" {
		t.Errorf("expected PolicyReason 'budget_ceiling_enforced', got %q", res3.PolicyReason)
	}
	if res3.EstimatedCost > 0.005 {
		t.Errorf("chosen model cost (%v) must be <= budget ceiling (0.005)", res3.EstimatedCost)
	}

	// 4. Zero allowable models (ceiling too low even for Haiku, e.g. $0.0001, and default-model also exceeds):
	nanoBudget := &Policy{
		BudgetCeiling: 0.0001,
	}
	routerNano := NewRouter(classifier, nanoBudget, session, unavail, logger, nil)
	res4 := routerNano.Route(ctx, "architect and design distributed cache", TaskMeta{}, runtimes, "claude-opus-4.5")
	if res4.Status != "blocked" {
		t.Errorf("expected status 'blocked' when all models exceed budget ceiling, got %q", res4.Status)
	}
	if res4.PolicyReason != "budget_ceiling_exceeded" {
		t.Errorf("expected PolicyReason 'budget_ceiling_exceeded', got %q", res4.PolicyReason)
	}

	// 5. Session Pin Bypass:
	// If a session was pinned to Opus, but a subsequent turn runs with a tight budget ceiling,
	// the session pin MUST be bypassed to prevent budget violations.
	sessionStore := NewSessionStore(time.Hour)
	sessionStore.Set(ctx, "issue-budget-pin", "sess-1", "rt-1", "claude-opus-4.5", TierHeavy)

	routerPinBudget := NewRouter(classifier, budgetPolicy, sessionStore, unavail, logger, nil)
	metaPin := TaskMeta{IssueID: "issue-budget-pin", SessionID: "sess-1"}
	resPin := routerPinBudget.Route(ctx, "architect the system", metaPin, runtimes, "default-model")
	if resPin.Model == "claude-opus-4.5" {
		t.Fatalf("session pin should NOT bypass budget ceiling! got pinned opus: %q", resPin.Model)
	}
	if resPin.Model != "claude-sonnet-4.5" {
		t.Fatalf("expected downgrade to sonnet under budget, got %q", resPin.Model)
	}
}
