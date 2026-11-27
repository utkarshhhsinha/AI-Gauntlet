package cerebra

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// RoutingResult is the output of a single Route() call.
type RoutingResult struct {
	RuntimeID     string
	Model         string
	Tier          Tier
	MatchedRule   string
	FallbackUsed  bool
	LatencyMs     int64
	Status        string // "ok" | "fallback" | "error"
	PolicyReason  string
	EstimatedCost float64
	InputTokens   int
	OutputTokens  int
}

// TierMap maps Tier values to concrete model IDs for one runtime.
// A nil or missing entry means that tier is not configured.
type TierMap map[Tier]string

// RuntimeEntry is one candidate runtime with its tier map.
type RuntimeEntry struct {
	RuntimeID string
	TierMap   TierMap
}

// Router is the central request-level model router. It combines the classifier,
// policy evaluator, session affinity, and availability cache into a single
// Route() call that returns the selected model (or a safe fallback).
//
// INVARIANT: filter candidates BEFORE selection. Never roll selection and then
// discard bad picks — that is the production failure mode Router was designed to close.
type Router struct {
	classifier   Classifier
	policy       *Policy
	session      *SessionStore
	unavail      *UnavailabilityStore
	logger       *slog.Logger
	routingLogFn func(ctx context.Context, entry RoutingLogEntry) // async write hook
}

// NewRouter constructs a Router with the given dependencies.
func NewRouter(
	classifier Classifier,
	policy *Policy,
	session *SessionStore,
	unavail *UnavailabilityStore,
	logger *slog.Logger,
	logFn func(ctx context.Context, entry RoutingLogEntry),
) *Router {
	return &Router{
		classifier:   classifier,
		policy:       policy,
		session:      session,
		unavail:      unavail,
		logger:       logger,
		routingLogFn: logFn,
	}
}

// Route selects the appropriate model for a task.
//
// Algorithm:
//  1. Classify prompt → tier + matched rule.
//  2. Load eligible candidates from runtimes' tier maps.
//  3. Remove unavailable candidates.
//  4. Remove policy-disallowed or over-budget candidates.
//  5. Reuse valid session pin if the same tier is requested.
//  6. Select from remaining (first available; extend with round-robin later).
//  7. Apply cross-runtime fallback / tier escalation if no candidate remains.
//  8. Write routing log asynchronously.
func (r *Router) Route(
	ctx context.Context,
	prompt string,
	meta TaskMeta,
	runtimes []RuntimeEntry,
	defaultModel string,
) RoutingResult {
	start := time.Now()
	promptTokens := EstimatePromptTokens(prompt)

	tier, matchedRule, err := r.classifier.Score(ctx, prompt, meta)
	if err != nil {
		r.logger.Warn("cerebra: classifier error; using default model", "error", err)
		return r.fallback(ctx, defaultModel, matchedRule, promptTokens, false, start)
	}

	// Step 2–4: filter candidates.
	candidates := r.filterCandidates(ctx, runtimes, tier, promptTokens)

	// Step 5: session affinity — reuse pinned model if still valid and same tier or higher (sticky escalation).
	if pin := r.session.Get(ctx, meta.IssueID, meta.SessionID); pin != nil {
		if pin.Tier == tier && r.candidateExists(candidates, pin.RuntimeID, pin.Model) {
			r.session.Refresh(ctx, meta.IssueID, meta.SessionID)
			cost := r.policy.EstimateCost(pin.Model, tier, promptTokens)
			result := RoutingResult{
				RuntimeID:     pin.RuntimeID,
				Model:         pin.Model,
				Tier:          tier,
				MatchedRule:   matchedRule + "+session_pin",
				LatencyMs:     time.Since(start).Milliseconds(),
				Status:        "ok",
				EstimatedCost: cost,
				InputTokens:   promptTokens,
				OutputTokens:  ExpectedOutputTokens(tier),
			}
			r.writeLog(ctx, result, meta)
			return result
		}
		if tierRank(pin.Tier) > tierRank(tier) {
			pinnedTierCandidates := r.filterCandidates(ctx, runtimes, pin.Tier, promptTokens)
			if r.candidateExists(pinnedTierCandidates, pin.RuntimeID, pin.Model) {
				r.session.Refresh(ctx, meta.IssueID, meta.SessionID)
				cost := r.policy.EstimateCost(pin.Model, pin.Tier, promptTokens)
				result := RoutingResult{
					RuntimeID:     pin.RuntimeID,
					Model:         pin.Model,
					Tier:          pin.Tier,
					MatchedRule:   matchedRule + "+session_pin_escalated",
					LatencyMs:     time.Since(start).Milliseconds(),
					Status:        "ok",
					EstimatedCost: cost,
					InputTokens:   promptTokens,
					OutputTokens:  ExpectedOutputTokens(pin.Tier),
				}
				r.writeLog(ctx, result, meta)
				return result
			}
		}
	}

	// Step 6: select from filtered candidates.
	if len(candidates) > 0 {
		chosen := candidates[0] // deterministic: first candidate (extend to round-robin)
		r.session.Set(ctx, meta.IssueID, meta.SessionID, chosen.RuntimeID, chosen.Model, tier)
		result := RoutingResult{
			RuntimeID:     chosen.RuntimeID,
			Model:         chosen.Model,
			Tier:          tier,
			MatchedRule:   matchedRule,
			LatencyMs:     time.Since(start).Milliseconds(),
			Status:        "ok",
			EstimatedCost: chosen.EstimatedCost,
			InputTokens:   promptTokens,
			OutputTokens:  ExpectedOutputTokens(tier),
		}
		r.writeLog(ctx, result, meta)
		return result
	}

	// Step 7: no candidates in target tier — try multi-level fallback before hard fallback.
	wasBudgetPruned := r.wasPrunedByBudget(ctx, runtimes, tier, promptTokens)

	switch tier {
	case TierSimple:
		// Simple -> Standard -> Heavy
		if escalated := r.filterCandidates(ctx, runtimes, TierStandard, promptTokens); len(escalated) > 0 {
			result := RoutingResult{
				RuntimeID:     escalated[0].RuntimeID,
				Model:         escalated[0].Model,
				Tier:          TierStandard,
				MatchedRule:   matchedRule + "+escalated_to_standard",
				FallbackUsed:  true,
				LatencyMs:     time.Since(start).Milliseconds(),
				Status:        "fallback",
				EstimatedCost: escalated[0].EstimatedCost,
				InputTokens:   promptTokens,
				OutputTokens:  ExpectedOutputTokens(TierStandard),
			}
			r.writeLog(ctx, result, meta)
			return result
		}
		if escalated := r.filterCandidates(ctx, runtimes, TierHeavy, promptTokens); len(escalated) > 0 {
			result := RoutingResult{
				RuntimeID:     escalated[0].RuntimeID,
				Model:         escalated[0].Model,
				Tier:          TierHeavy,
				MatchedRule:   matchedRule + "+escalated_to_heavy",
				FallbackUsed:  true,
				LatencyMs:     time.Since(start).Milliseconds(),
				Status:        "fallback",
				EstimatedCost: escalated[0].EstimatedCost,
				InputTokens:   promptTokens,
				OutputTokens:  ExpectedOutputTokens(TierHeavy),
			}
			r.writeLog(ctx, result, meta)
			return result
		}

	case TierStandard:
		// Standard -> Heavy -> Simple (if not MCP tool task)
		if escalated := r.filterCandidates(ctx, runtimes, TierHeavy, promptTokens); len(escalated) > 0 {
			result := RoutingResult{
				RuntimeID:     escalated[0].RuntimeID,
				Model:         escalated[0].Model,
				Tier:          TierHeavy,
				MatchedRule:   matchedRule + "+escalated_to_heavy",
				FallbackUsed:  true,
				LatencyMs:     time.Since(start).Milliseconds(),
				Status:        "fallback",
				EstimatedCost: escalated[0].EstimatedCost,
				InputTokens:   promptTokens,
				OutputTokens:  ExpectedOutputTokens(TierHeavy),
			}
			r.writeLog(ctx, result, meta)
			return result
		}
		if !meta.WillUseMCPTools {
			if deescalated := r.filterCandidates(ctx, runtimes, TierSimple, promptTokens); len(deescalated) > 0 {
				rule := matchedRule + "+fallback_to_simple"
				reason := ""
				if wasBudgetPruned {
					rule = matchedRule + "+budget_fallback_to_simple"
					reason = "budget_ceiling_enforced"
				}
				result := RoutingResult{
					RuntimeID:     deescalated[0].RuntimeID,
					Model:         deescalated[0].Model,
					Tier:          TierSimple,
					MatchedRule:   rule,
					FallbackUsed:  true,
					LatencyMs:     time.Since(start).Milliseconds(),
					Status:        "fallback",
					PolicyReason:  reason,
					EstimatedCost: deescalated[0].EstimatedCost,
					InputTokens:   promptTokens,
					OutputTokens:  ExpectedOutputTokens(TierSimple),
				}
				r.writeLog(ctx, result, meta)
				return result
			}
		}

	case TierHeavy:
		// Heavy -> Standard -> Simple (if not MCP tool task)
		if deescalated := r.filterCandidates(ctx, runtimes, TierStandard, promptTokens); len(deescalated) > 0 {
			rule := matchedRule + "+fallback_to_standard"
			reason := ""
			if wasBudgetPruned {
				rule = matchedRule + "+budget_fallback_to_standard"
				reason = "budget_ceiling_enforced"
			}
			result := RoutingResult{
				RuntimeID:     deescalated[0].RuntimeID,
				Model:         deescalated[0].Model,
				Tier:          TierStandard,
				MatchedRule:   rule,
				FallbackUsed:  true,
				LatencyMs:     time.Since(start).Milliseconds(),
				Status:        "fallback",
				PolicyReason:  reason,
				EstimatedCost: deescalated[0].EstimatedCost,
				InputTokens:   promptTokens,
				OutputTokens:  ExpectedOutputTokens(TierStandard),
			}
			r.writeLog(ctx, result, meta)
			return result
		}
		if !meta.WillUseMCPTools {
			if deescalated := r.filterCandidates(ctx, runtimes, TierSimple, promptTokens); len(deescalated) > 0 {
				rule := matchedRule + "+fallback_to_simple"
				reason := ""
				if wasBudgetPruned {
					rule = matchedRule + "+budget_fallback_to_simple"
					reason = "budget_ceiling_enforced"
				}
				result := RoutingResult{
					RuntimeID:     deescalated[0].RuntimeID,
					Model:         deescalated[0].Model,
					Tier:          TierSimple,
					MatchedRule:   rule,
					FallbackUsed:  true,
					LatencyMs:     time.Since(start).Milliseconds(),
					Status:        "fallback",
					PolicyReason:  reason,
					EstimatedCost: deescalated[0].EstimatedCost,
					InputTokens:   promptTokens,
					OutputTokens:  ExpectedOutputTokens(TierSimple),
				}
				r.writeLog(ctx, result, meta)
				return result
			}
		}
	}

	// Hard fallback: use the pre-configured default model.
	r.logger.Warn("cerebra: no eligible candidates; using default model",
		"tier", tier, "matched_rule", matchedRule)
	return r.fallback(ctx, defaultModel, matchedRule, promptTokens, wasBudgetPruned, start)
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

type candidate struct {
	RuntimeID     string
	Model         string
	EstimatedCost float64
}

// filterCandidates applies the FILTER-BEFORE-SELECT invariant:
// unavailability → policy/budget → return clean pool.
func (r *Router) filterCandidates(ctx context.Context, runtimes []RuntimeEntry, tier Tier, promptTokens int) []candidate {
	var out []candidate
	for _, rt := range runtimes {
		model, ok := rt.TierMap[tier]
		if !ok || model == "" {
			continue
		}
		if !r.unavail.IsAvailable(ctx, rt.RuntimeID, model) {
			continue
		}
		allowed, cost := r.policy.AllowWithBudget(rt.RuntimeID, model, tier, promptTokens)
		if !allowed {
			continue
		}
		out = append(out, candidate{RuntimeID: rt.RuntimeID, Model: model, EstimatedCost: cost})
	}
	return out
}

// wasPrunedByBudget returns true if any candidate for this tier would have been eligible
// (passed availability & static policy) but was rejected specifically by BudgetCeiling.
func (r *Router) wasPrunedByBudget(ctx context.Context, runtimes []RuntimeEntry, tier Tier, promptTokens int) bool {
	if r.policy == nil || r.policy.BudgetCeiling <= 0 {
		return false
	}
	for _, rt := range runtimes {
		model, ok := rt.TierMap[tier]
		if !ok || model == "" {
			continue
		}
		if !r.unavail.IsAvailable(ctx, rt.RuntimeID, model) {
			continue
		}
		if !r.policy.Allow(rt.RuntimeID, model) {
			continue
		}
		if allowed, _ := r.policy.AllowBudget(model, tier, promptTokens); !allowed {
			return true
		}
	}
	return false
}

func (r *Router) candidateExists(candidates []candidate, runtimeID, model string) bool {
	for _, c := range candidates {
		if c.RuntimeID == runtimeID && c.Model == model {
			return true
		}
	}
	return false
}

func (r *Router) fallback(_ context.Context, defaultModel, matchedRule string, promptTokens int, wasBudgetPruned bool, start time.Time) RoutingResult {
	cost := 0.0
	if r.policy != nil && defaultModel != "" {
		cost = r.policy.EstimateCost(defaultModel, TierSimple, promptTokens)
	}
	status := "fallback"
	reason := ""
	if wasBudgetPruned {
		reason = "budget_ceiling_enforced"
	}
	if r.policy != nil && r.policy.BudgetCeiling > 0 && cost > r.policy.BudgetCeiling {
		status = "blocked"
		reason = "budget_ceiling_exceeded"
	}
	return RoutingResult{
		RuntimeID:     "",
		Model:         defaultModel,
		Tier:          TierSimple,
		MatchedRule:   matchedRule,
		FallbackUsed:  true,
		LatencyMs:     time.Since(start).Milliseconds(),
		Status:        status,
		PolicyReason:  reason,
		EstimatedCost: cost,
		InputTokens:   promptTokens,
		OutputTokens:  ExpectedOutputTokens(TierSimple),
	}
}

func sanitizeLogField(s string, maxLen int) string {
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (r *Router) writeLog(_ context.Context, result RoutingResult, meta TaskMeta) {
	if r.routingLogFn == nil {
		return
	}
	taskID := meta.TaskID
	if taskID == "" {
		taskID = meta.IssueID
	}
	entry := RoutingLogEntry{
		TaskID:            sanitizeLogField(taskID, 64),
		IssueID:           sanitizeLogField(meta.IssueID, 64),
		SessionID:         sanitizeLogField(meta.SessionID, 64),
		RuntimeID:         sanitizeLogField(result.RuntimeID, 64),
		ChosenModel:       sanitizeLogField(result.Model, 128),
		Tier:              string(result.Tier),
		MatchedRule:       sanitizeLogField(result.MatchedRule, 128),
		ToolChainExpected: meta.WillUseMCPTools,
		FallbackUsed:      result.FallbackUsed,
		LatencyMs:         int(result.LatencyMs),
		Status:            result.Status,
		PolicyReason:      result.PolicyReason,
		EstimatedCost:     result.EstimatedCost,
		InputTokens:       result.InputTokens,
		OutputTokens:      result.OutputTokens,
	}
	// Fire-and-forget so routing evidence never adds latency to task dispatch.
	// Use an independent timeout context so caller cancellation does not abort the write.
	go func() {
		logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := func() (retErr error) {
			defer func() {
				if rec := recover(); rec != nil {
					retErr = fmt.Errorf("routing log panic: %v", rec)
				}
			}()
			r.routingLogFn(logCtx, entry)
			return nil
		}(); err != nil {
			r.logger.Warn("cerebra: routing log write failed", "error", err)
		}
	}()
}

// RoutingLogEntry is the record written to cerebra_routing_log.
type RoutingLogEntry struct {
	TaskID            string
	IssueID           string
	SessionID         string
	RuntimeID         string
	ChosenModel       string
	Tier              string
	MatchedRule       string
	ToolChainExpected bool
	FallbackUsed      bool
	LatencyMs         int
	Status            string

	// Optional
	PolicyReason      string
	CandidateCount    int
	ClassifierVersion string
	EstimatedCost     float64
	InputTokens       int
	OutputTokens      int
}

// ErrNoRoute is returned by integration tests when no model can be selected
// and no default is provided.
var ErrNoRoute = errors.New("cerebra: no eligible model candidate")

// InvalidateSession clears any session pin for the given issue and session ID.
func (r *Router) InvalidateSession(ctx context.Context, issueID, sessionID string) {
	if r.session != nil {
		r.session.Delete(ctx, issueID, sessionID)
	}
}

// UnavailabilityStore returns the underlying UnavailabilityStore.
func (r *Router) UnavailabilityStore() *UnavailabilityStore {
	return r.unavail
}

