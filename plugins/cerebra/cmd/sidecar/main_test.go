package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func testServer(t *testing.T, customCfg ...func(*config)) (*server, http.Handler) {
	t.Helper()
	cfg := config{
		Port:              "0",
		SessionTTL:        time.Hour,
		UnavailTTL:        time.Hour,
		MaxPins:           5000,
		MaxEscalatedTurns: 5,
		MaxPinLifetime:    24 * time.Hour,
		CleanupInterval:   0, // Disabled unless explicitly tested
	}
	for _, fn := range customCfg {
		fn(&cfg)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := newServer(context.Background(), cfg, logger)
	handler := srv.routes()
	t.Cleanup(func() {
		srv.Close()
	})
	return srv, handler
}

func doJSONPost(t *testing.T, handler http.Handler, path string, payload any) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return resp, data
}

// 1. Circuit breaker persists: mark model A unavailable on request 1; on request 2, assert model A is skipped and fallback is used without calling A again.
func TestCircuitBreakerPersists(t *testing.T) {
	_, handler := testServer(t)

	tierMap := map[string]string{
		"simple":   "claude-3-5-haiku",
		"standard": "claude-3-5-sonnet",
		"heavy":    "claude-3-opus",
	}

	// Request 1: Simple task routes to claude-3-5-haiku
	req1 := routeRequest{
		TaskID:    "t-1",
		SessionID: "s-1",
		Prompt:    "hello world",
		TierMap:   tierMap,
	}
	resp, data := doJSONPost(t, handler, "/route", req1)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	var res1 routeResponse
	if err := json.Unmarshal(data, &res1); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if res1.RecommendedModel != "claude-3-5-haiku" {
		t.Fatalf("expected claude-3-5-haiku, got %s", res1.RecommendedModel)
	}

	// Simulate 429 on claude-3-5-haiku via /unavailable endpoint
	unavailReq := unavailableRequest{
		Model: "claude-3-5-haiku",
	}
	resp, _ = doJSONPost(t, handler, "/unavailable", unavailReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to mark unavailable: %d", resp.StatusCode)
	}

	// Request 2: Another simple task in a new session.
	// Since claude-3-5-haiku was marked unavailable in the shared store,
	// Request 2 must skip it and escalate to claude-3-5-sonnet (fallback used).
	req2 := routeRequest{
		TaskID:    "t-2",
		SessionID: "s-2",
		Prompt:    "hello again",
		TierMap:   tierMap,
	}
	resp, data = doJSONPost(t, handler, "/route", req2)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	var res2 routeResponse
	if err := json.Unmarshal(data, &res2); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if res2.RecommendedModel != "claude-3-5-sonnet" {
		t.Fatalf("expected circuit breaker to skip haiku and select sonnet, got %s", res2.RecommendedModel)
	}
	if !res2.FallbackUsed && res2.Status != "fallback" {
		t.Fatalf("expected fallback to be recorded in result, got fallback_used=%v status=%s", res2.FallbackUsed, res2.Status)
	}

	// Also verify that reporting FailedModel directly on /route marks it unavailable immediately
	req3 := routeRequest{
		TaskID:      "t-3",
		SessionID:   "s-3",
		Prompt:      "hello standard",
		TierMap:     tierMap,
		FailedModel: "claude-3-5-sonnet", // sonnet hit 429
	}
	resp, data = doJSONPost(t, handler, "/route", req3)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var res3 routeResponse
	if err := json.Unmarshal(data, &res3); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	// Now both haiku and sonnet are unavailable -> escalates to opus
	if res3.RecommendedModel != "claude-3-opus" {
		t.Fatalf("expected escalation to opus, got %s", res3.RecommendedModel)
	}
}

// 2. Session stickiness persists: two requests with the same session ID are routed to the same model.
func TestSessionStickinessPersists(t *testing.T) {
	_, handler := testServer(t)

	tierMap := map[string]string{
		"simple":   "claude-3-5-haiku",
		"standard": "claude-3-5-sonnet",
		"heavy":    "claude-3-opus",
	}

	sessionID := "sticky-session-123"

	// Request 1: Heavy design prompt -> routes to claude-3-opus (TierHeavy) and pins it
	req1 := routeRequest{
		TaskID:    "task-design",
		SessionID: sessionID,
		Prompt:    "architect and design a distributed ledger consensus system",
		TierMap:   tierMap,
	}
	resp, data := doJSONPost(t, handler, "/route", req1)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var res1 routeResponse
	json.Unmarshal(data, &res1)
	if res1.RecommendedModel != "claude-3-opus" || res1.Tier != "heavy" {
		t.Fatalf("expected heavy/opus on first request, got model=%s tier=%s", res1.RecommendedModel, res1.Tier)
	}

	// Request 2: A simple prompt ("hello") under the same session ID.
	// Natural classification is TierSimple, but sticky session pinning must keep claude-3-opus.
	req2 := routeRequest{
		TaskID:    "task-followup",
		SessionID: sessionID,
		Prompt:    "hello",
		TierMap:   tierMap,
	}
	resp, data = doJSONPost(t, handler, "/route", req2)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var res2 routeResponse
	json.Unmarshal(data, &res2)

	if res2.RecommendedModel != "claude-3-opus" {
		t.Fatalf("expected sticky session pin claude-3-opus to persist, got %s", res2.RecommendedModel)
	}
	if res2.Tier != "heavy" {
		t.Fatalf("expected tier to remain heavy due to sticky escalation, got %s", res2.Tier)
	}
}

// 3. Isolation: different session IDs don't affect each other.
func TestSessionIsolation(t *testing.T) {
	_, handler := testServer(t)

	tierMap := map[string]string{
		"simple":   "claude-3-5-haiku",
		"standard": "claude-3-5-sonnet",
		"heavy":    "claude-3-opus",
	}

	// Session A escalates to Heavy
	reqA1 := routeRequest{
		TaskID:    "task-A1",
		SessionID: "session-A",
		Prompt:    "deep architecture design and refactor",
		TierMap:   tierMap,
	}
	_, dataA1 := doJSONPost(t, handler, "/route", reqA1)
	var resA1 routeResponse
	json.Unmarshal(dataA1, &resA1)
	if resA1.RecommendedModel != "claude-3-opus" {
		t.Fatalf("session A expected opus, got %s", resA1.RecommendedModel)
	}

	// Session B sends a simple prompt -> should get Simple model (claude-3-5-haiku), isolated from Session A
	reqB1 := routeRequest{
		TaskID:    "task-B1",
		SessionID: "session-B",
		Prompt:    "hello simple check",
		TierMap:   tierMap,
	}
	_, dataB1 := doJSONPost(t, handler, "/route", reqB1)
	var resB1 routeResponse
	json.Unmarshal(dataB1, &resB1)
	if resB1.RecommendedModel != "claude-3-5-haiku" || resB1.Tier != "simple" {
		t.Fatalf("session B expected haiku/simple, got model=%s tier=%s", resB1.RecommendedModel, resB1.Tier)
	}

	// Session A sends simple prompt -> sticky pin retains claude-3-opus
	reqA2 := routeRequest{
		TaskID:    "task-A2",
		SessionID: "session-A",
		Prompt:    "hello follow-up",
		TierMap:   tierMap,
	}
	_, dataA2 := doJSONPost(t, handler, "/route", reqA2)
	var resA2 routeResponse
	json.Unmarshal(dataA2, &resA2)
	if resA2.RecommendedModel != "claude-3-opus" {
		t.Fatalf("session A expected to retain opus pin, got %s", resA2.RecommendedModel)
	}

	// Session B sends another simple prompt -> retains claude-3-5-haiku
	reqB2 := routeRequest{
		TaskID:    "task-B2",
		SessionID: "session-B",
		Prompt:    "continue simple",
		TierMap:   tierMap,
	}
	_, dataB2 := doJSONPost(t, handler, "/route", reqB2)
	var resB2 routeResponse
	json.Unmarshal(dataB2, &resB2)
	if resB2.RecommendedModel != "claude-3-5-haiku" {
		t.Fatalf("session B expected haiku, got %s", resB2.RecommendedModel)
	}
}

// 4. Expiry: an unavailable model becomes available again after its TTL.
func TestUnavailabilityExpiry(t *testing.T) {
	_, handler := testServer(t, func(c *config) {
		c.UnavailTTL = 50 * time.Millisecond
	})

	tierMap := map[string]string{
		"simple":   "claude-3-5-haiku",
		"standard": "claude-3-5-sonnet",
		"heavy":    "claude-3-opus",
	}

	// Mark haiku unavailable with 50ms TTL
	unavailReq := unavailableRequest{
		Model:      "claude-3-5-haiku",
		TTLSeconds: 0, // falls back to srv.cfg.UnavailTTL (50ms)
	}
	doJSONPost(t, handler, "/unavailable", unavailReq)

	// Immediately request: haiku is unavailable -> escalates to sonnet
	req1 := routeRequest{
		TaskID:    "task-exp-1",
		SessionID: "session-exp-1",
		Prompt:    "hello world",
		TierMap:   tierMap,
	}
	_, data1 := doJSONPost(t, handler, "/route", req1)
	var res1 routeResponse
	json.Unmarshal(data1, &res1)
	if res1.RecommendedModel != "claude-3-5-sonnet" {
		t.Fatalf("expected sonnet during unavailability window, got %s", res1.RecommendedModel)
	}

	// Wait for TTL to expire
	time.Sleep(75 * time.Millisecond)

	// Request after expiry: haiku should be available again
	req2 := routeRequest{
		TaskID:    "task-exp-2",
		SessionID: "session-exp-2",
		Prompt:    "hello world",
		TierMap:   tierMap,
	}
	_, data2 := doJSONPost(t, handler, "/route", req2)
	var res2 routeResponse
	json.Unmarshal(data2, &res2)
	if res2.RecommendedModel != "claude-3-5-haiku" {
		t.Fatalf("expected haiku to become available again after TTL, got %s", res2.RecommendedModel)
	}
}

// 5. Concurrency: run with go test -race and issue parallel requests to the handler.
func TestConcurrencyAndRace(t *testing.T) {
	_, handler := testServer(t)

	tierMap := map[string]string{
		"simple":   "claude-3-5-haiku",
		"standard": "claude-3-5-sonnet",
		"heavy":    "claude-3-opus",
	}

	const concurrency = 40
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			sessID := "concurrent-sess-" + strconv.Itoa(workerID%5)

			for round := 0; round < 5; round++ {
				switch round % 3 {
				case 0:
					// Simple task
					req := routeRequest{
						TaskID:    "task-c",
						SessionID: sessID,
						Prompt:    "syntax question",
						TierMap:   tierMap,
					}
					resp, _ := doJSONPost(t, handler, "/route", req)
					if resp.StatusCode != http.StatusOK {
						t.Errorf("worker %d /route failed: status %d", workerID, resp.StatusCode)
					}
				case 1:
					// Health check
					httpReq := httptest.NewRequest(http.MethodGet, "/health", nil)
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, httpReq)
					if rec.Code != http.StatusOK {
						t.Errorf("worker %d /health failed: status %d", workerID, rec.Code)
					}
				case 2:
					// Heavy task
					req := routeRequest{
						TaskID:    "task-h",
						SessionID: sessID,
						Prompt:    "architect database distributed replication",
						TierMap:   tierMap,
					}
					resp, _ := doJSONPost(t, handler, "/route", req)
					if resp.StatusCode != http.StatusOK {
						t.Errorf("worker %d /route failed: status %d", workerID, resp.StatusCode)
					}
				}
			}
		}()
	}

	wg.Wait()
}

// 6. Background cleanup routine and clean server shutdown
func TestBackgroundCleanupAndShutdown(t *testing.T) {
	srv, _ := testServer(t, func(c *config) {
		c.SessionTTL = 20 * time.Millisecond
		c.UnavailTTL = 20 * time.Millisecond
		c.CleanupInterval = 30 * time.Millisecond
	})

	ctx := context.Background()
	srv.session.Set(ctx, "issue-bg", "sess-bg", "rt-1", "m-1", "simple")
	srv.unavail.MarkUnavailable(ctx, "rt-1", "m-unavail", 20*time.Millisecond)

	if srv.session.Count() == 0 {
		t.Fatalf("expected session pin to exist")
	}

	// Wait for cleanup interval to pass
	time.Sleep(70 * time.Millisecond)

	// Pins should have been pruned in background
	if srv.session.Count() != 0 {
		t.Fatalf("expected session store to be cleaned up, got count=%d", srv.session.Count())
	}

	// Close server and verify shutdown completes without deadlock
	done := make(chan struct{})
	go func() {
		srv.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("server.Close() deadlocked or timed out")
	}
}
