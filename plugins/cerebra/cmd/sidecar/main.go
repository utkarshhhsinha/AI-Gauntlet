package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/utkarshhhsinha/AI-Gauntlet/server/internal/cerebra"
)

type config struct {
	Port              string
	SessionTTL        time.Duration
	UnavailTTL        time.Duration
	MaxPins           int
	MaxEscalatedTurns int
	MaxPinLifetime    time.Duration
	CleanupInterval   time.Duration
	DefaultModel      string
}

func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

func parseIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

func loadConfig() config {
	port := os.Getenv("CEREBRA_PORT")
	if port == "" {
		port = "7842"
	}
	return config{
		Port:              port,
		SessionTTL:        parseDurationEnv("CEREBRA_SESSION_TTL", 2*time.Hour),
		UnavailTTL:        parseDurationEnv("CEREBRA_UNAVAIL_TTL", time.Hour),
		MaxPins:           parseIntEnv("CEREBRA_MAX_PINS", 5000),
		MaxEscalatedTurns: parseIntEnv("CEREBRA_MAX_ESCALATED_TURNS", 5),
		MaxPinLifetime:    parseDurationEnv("CEREBRA_MAX_PIN_LIFETIME", 24*time.Hour),
		CleanupInterval:   parseDurationEnv("CEREBRA_CLEANUP_INTERVAL", 10*time.Minute),
		DefaultModel:      os.Getenv("CEREBRA_DEFAULT_MODEL"),
	}
}

type routeRequest struct {
	IssueID      string            `json:"issue_id"`
	TaskID       string            `json:"task_id"`
	SessionID    string            `json:"session_id"`
	Prompt       string            `json:"prompt"`
	UseMCP       bool              `json:"will_use_mcp_tools"`
	FailedModel  string            `json:"failed_model,omitempty"`
	RuntimeID    string            `json:"runtime_id,omitempty"`
	DefaultModel string            `json:"default_model,omitempty"`
	Models       []string          `json:"models,omitempty"`
	TierMap      map[string]string `json:"tier_map,omitempty"`
}

type routeResponse struct {
	RecommendedModel string `json:"recommended_model"`
	Tier             string `json:"tier"`
	MatchedRule      string `json:"matched_rule"`
	FallbackUsed     bool   `json:"fallback_used,omitempty"`
	Status           string `json:"status,omitempty"`
}

type unavailableRequest struct {
	RuntimeID  string `json:"runtime_id"`
	Model      string `json:"model"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type server struct {
	cfg        config
	logger     *slog.Logger
	session    *cerebra.SessionStore
	unavail    *cerebra.UnavailabilityStore
	router     *cerebra.Router
	cancelFunc context.CancelFunc
	done       chan struct{}
}

func newServer(ctx context.Context, cfg config, logger *slog.Logger) *server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	session := cerebra.NewSessionStore(cfg.SessionTTL)
	session.SetLimits(cfg.MaxPins, cfg.MaxEscalatedTurns, cfg.MaxPinLifetime)

	unavail := cerebra.NewUnavailabilityStore(cfg.UnavailTTL)

	classifier := cerebra.HeuristicClassifier{}
	policy := &cerebra.Policy{}
	router := cerebra.NewRouter(classifier, policy, session, unavail, logger, nil)

	srvCtx, cancel := context.WithCancel(ctx)
	s := &server{
		cfg:        cfg,
		logger:     logger,
		session:    session,
		unavail:    unavail,
		router:     router,
		cancelFunc: cancel,
		done:       make(chan struct{}),
	}

	if cfg.CleanupInterval > 0 {
		go s.runBackgroundCleanup(srvCtx)
	} else {
		close(s.done)
	}

	return s
}

func (s *server) runBackgroundCleanup(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionEvicted := s.session.EvictExpired()
			unavailEvicted := s.unavail.EvictExpired()
			if sessionEvicted > 0 || unavailEvicted > 0 {
				s.logger.Debug("cerebra sidecar evicted expired entries",
					"session_pins", sessionEvicted,
					"unavail_models", unavailEvicted,
				)
			}
		}
	}
}

func (s *server) Close() error {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	<-s.done
	return nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/route", s.handleRoute)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/unavailable", s.handleUnavailable)
	return mux
}

func (s *server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	runtimeID := req.RuntimeID
	if runtimeID == "" {
		runtimeID = "cerebra-sidecar"
	}

	// If caller signals that a model returned a 429/failure, mark it unavailable immediately.
	if req.FailedModel != "" {
		s.unavail.MarkUnavailable(r.Context(), runtimeID, req.FailedModel, 0)
		s.logger.Info("circuit breaker: model marked unavailable", "model", req.FailedModel, "runtime", runtimeID)
	}

	// Model tier map determination:
	// 1. Explicit tier map provided in request (e.g. tests or custom configs)
	// 2. Explicit models catalog provided in request
	// 3. Dynamic model discovery (probes Ollama + local runtimes)
	var tierMap cerebra.TierMap
	if len(req.TierMap) > 0 {
		tierMap = make(cerebra.TierMap)
		for k, v := range req.TierMap {
			switch strings.ToLower(k) {
			case "simple":
				tierMap[cerebra.TierSimple] = v
			case "standard":
				tierMap[cerebra.TierStandard] = v
			case "heavy":
				tierMap[cerebra.TierHeavy] = v
			}
		}
	} else if len(req.Models) > 0 {
		tierMap = cerebra.BuildTierMapFromCatalog(req.Models)
	} else {
		models := cerebra.DiscoverModels(r.Context())
		tierMap = cerebra.BuildTierMapFromCatalog(models)
	}

	runtimes := []cerebra.RuntimeEntry{{RuntimeID: runtimeID, TierMap: tierMap}}
	meta := cerebra.TaskMeta{
		TaskID:          req.TaskID,
		IssueID:         req.IssueID,
		SessionID:       req.SessionID,
		WillUseMCPTools: req.UseMCP,
	}

	defaultModel := req.DefaultModel
	if defaultModel == "" {
		defaultModel = s.cfg.DefaultModel
	}

	result := s.router.Route(r.Context(), req.Prompt, meta, runtimes, defaultModel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routeResponse{
		RecommendedModel: result.Model,
		Tier:             string(result.Tier),
		MatchedRule:      result.MatchedRule,
		FallbackUsed:     result.FallbackUsed,
		Status:           result.Status,
	})

	s.logger.Info("routed",
		"task_id", req.TaskID,
		"session_id", req.SessionID,
		"tier", result.Tier,
		"model", result.Model,
		"rule", result.MatchedRule,
		"fallback", result.FallbackUsed,
	)
}

func (s *server) handleUnavailable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req unavailableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	runtimeID := req.RuntimeID
	if runtimeID == "" {
		runtimeID = "cerebra-sidecar"
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	s.unavail.MarkUnavailable(r.Context(), runtimeID, req.Model, ttl)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "marked_unavailable",
		"runtime_id": runtimeID,
		"model":      req.Model,
	})

	s.logger.Info("circuit breaker: model explicitly marked unavailable",
		"model", req.Model,
		"runtime", runtimeID,
		"ttl_seconds", req.TTLSeconds,
	)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "1.0.1",
	})
}

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := newServer(ctx, cfg, logger)
	defer srv.Close()

	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.routes(),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("sidecar server shutdown error", "error", err)
		}
	}()

	logger.Info("cerebra sidecar started",
		"port", cfg.Port,
		"session_ttl", cfg.SessionTTL,
		"unavail_ttl", cfg.UnavailTTL,
		"max_pins", cfg.MaxPins,
		"cleanup_interval", cfg.CleanupInterval,
	)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("sidecar failed", "error", err)
		os.Exit(1)
	}
	logger.Info("cerebra sidecar stopped gracefully")
}
