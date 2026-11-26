package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobNameDatabaseOrphanGC is the canonical name written to sys_cron_executions audit rows.
// Stable across releases — do not rename without migration.
const JobNameDatabaseOrphanGC = "database_orphan_gc"

const (
	// EnvOrphanGCDryRun makes the job count what it would remove without deleting.
	EnvOrphanGCDryRun = "AI_GAUNTLET_ORPHAN_GC_DRY_RUN"

	defaultOrphanGraceAge      = time.Hour
	defaultOrphanBatchSize     = 1000
	defaultOrphanMaxRowsPerRun = 50_000
	defaultOrphanWarnThreshold = 1_000

	// uuidPattern matches canonical UUID text. Text-typed reference columns are
	// only compared when they hold a valid UUID, so malformed values are never
	// mistaken for orphans.
	uuidPattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
)

// OrphanGCConfig tunes the safety limits of the orphan cleanup.
type OrphanGCConfig struct {
	// GraceAge: only rows older than this are eligible, so rows written by an
	// in-flight transaction (child visible before its parent commits) are safe.
	GraceAge time.Duration
	// BatchSize is the maximum rows touched by a single statement.
	BatchSize int
	// MaxRowsPerRun caps the total rows changed (or counted in dry-run) per run.
	MaxRowsPerRun int64
	// WarnThreshold logs a warning when a run removes more rows than this.
	WarnThreshold int64
	// DryRun counts candidates and logs them without modifying data.
	DryRun bool
}

// DefaultOrphanGCConfig returns production defaults. DryRun is read from
// AI_GAUNTLET_ORPHAN_GC_DRY_RUN=1.
func DefaultOrphanGCConfig() OrphanGCConfig {
	return OrphanGCConfig{
		GraceAge:      defaultOrphanGraceAge,
		BatchSize:     defaultOrphanBatchSize,
		MaxRowsPerRun: defaultOrphanMaxRowsPerRun,
		WarnThreshold: defaultOrphanWarnThreshold,
		DryRun:        os.Getenv(EnvOrphanGCDryRun) == "1",
	}
}

// DatabaseOrphanGCJob returns the JobSpec that drives automated garbage collection
// of orphaned database rows. Because the repository architecture forbids foreign keys
// and cascading deletes, child records (e.g. cerebra routing logs, task messages,
// labels, MCP bindings, chat draft restores) can become orphaned if a parent record
// is removed or if a transaction fails mid-flight.
//
// This job runs on the distributed DB-backed scheduler, serialized via sys_cron_executions.
func DatabaseOrphanGCJob(pool *pgxpool.Pool) JobSpec {
	return JobSpec{
		Name:              JobNameDatabaseOrphanGC,
		Cadence:           1 * time.Hour,
		ScheduleDelay:     5 * time.Minute,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     24 * time.Hour,
		RunTimeout:        15 * time.Minute,
		StaleTimeout:      20 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff: []time.Duration{
			1 * time.Minute,
			5 * time.Minute,
			15 * time.Minute,
		},
		Scopes:  StaticScopes(ScopeGlobal),
		Handler: makeDatabaseOrphanGCHandler(pool),
	}
}

func makeDatabaseOrphanGCHandler(pool *pgxpool.Pool) Handler {
	return func(ctx context.Context, in HandlerInput) (HandlerResult, error) {
		return RunDatabaseOrphanGC(ctx, pool, in.Heartbeat)
	}
}

// RunDatabaseOrphanGC runs the cleanup with DefaultOrphanGCConfig.
func RunDatabaseOrphanGC(ctx context.Context, pool *pgxpool.Pool, heartbeat func(context.Context) error) (HandlerResult, error) {
	return RunDatabaseOrphanGCWithConfig(ctx, pool, heartbeat, DefaultOrphanGCConfig())
}

// RunDatabaseOrphanGCWithConfig executes every cleanup phase. A failing phase
// does not stop the others; all phase errors are returned joined together.
func RunDatabaseOrphanGCWithConfig(ctx context.Context, pool *pgxpool.Pool, heartbeat func(context.Context) error, cfg OrphanGCConfig) (HandlerResult, error) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultOrphanBatchSize
	}
	if cfg.MaxRowsPerRun <= 0 {
		cfg.MaxRowsPerRun = defaultOrphanMaxRowsPerRun
	}
	if cfg.GraceAge < 0 {
		cfg.GraceAge = 0
	}

	g := &orphanGC{
		pool:      pool,
		cfg:       cfg,
		breakdown: make(map[string]int64),
		beat: func() {
			if heartbeat != nil {
				_ = heartbeat(ctx)
			}
		},
	}

	phases := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"cerebra", g.pruneCerebra},
		{"resource labels", g.pruneResourceLabels},
		{"mcp bindings", g.pruneMCPBindings},
		{"chat draft restores", g.pruneChatDraftRestores},
		{"task messages", g.pruneTaskMessages},
		{"comments", g.pruneComments},
		{"attachments", g.pruneAttachments},
	}

	var errs []error
	for _, p := range phases {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		if err := p.fn(ctx); err != nil {
			slog.Error("orphan_gc: phase failed", "phase", p.name, "error", err)
			errs = append(errs, fmt.Errorf("prune %s: %w", p.name, err))
		}
		g.beat()
	}

	if g.total > cfg.WarnThreshold {
		slog.Warn("orphan_gc: unusually many orphaned rows", "rows", g.total, "threshold", cfg.WarnThreshold, "dry_run", cfg.DryRun)
	}

	affected := g.total
	if cfg.DryRun {
		affected = 0 // nothing was modified
	}
	return HandlerResult{
		RowsAffected: affected,
		Result: map[string]any{
			"total_pruned": g.total,
			"breakdown":    g.breakdown,
			"dry_run":      cfg.DryRun,
			"capped":       g.capped,
		},
	}, errors.Join(errs...)
}

type orphanGC struct {
	pool      *pgxpool.Pool
	cfg       OrphanGCConfig
	beat      func()
	total     int64
	capped    bool
	breakdown map[string]int64
}

// tableExists reports whether a table exists in the current schema.
func tableExists(ctx context.Context, pool *pgxpool.Pool, tableName string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

// missingUUIDRef is a SQL predicate that is true only when col holds a valid
// UUID whose row is absent from parentTable. NULL, empty and malformed values
// yield false (they are never treated as orphans). CASE guarantees the regex
// check runs before the ::uuid cast.
func missingUUIDRef(col, parentTable string) string {
	return fmt.Sprintf(
		`(CASE WHEN %[1]s ~* '%[3]s' THEN NOT EXISTS (SELECT 1 FROM %[2]s p WHERE p.id = %[1]s::uuid) ELSE FALSE END)`,
		col, parentTable, uuidPattern)
}

// batchStmt wraps the row selector (SELECT ctid ... LIMIT n) in the statement
// that acts on those rows.
type batchStmt func(selector string) string

func deleteStmt(table string) batchStmt {
	return func(sel string) string {
		return fmt.Sprintf("DELETE FROM %s WHERE ctid IN (%s)", table, sel)
	}
}

// run applies stmt to rows of table matching cond in bounded batches. ageCol,
// when set, restricts the work to rows older than the grace age. Every
// identifier and number is a code constant, never user input.
func (g *orphanGC) run(ctx context.Context, key, table, ageCol, cond string, stmt batchStmt) (int64, error) {
	exists, err := tableExists(ctx, g.pool, table)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	where := cond
	if ageCol != "" && g.cfg.GraceAge > 0 {
		where = fmt.Sprintf("(%s) AND %s < now() - interval '%d seconds'", cond, ageCol, int64(g.cfg.GraceAge.Seconds()))
	}

	var done int64
	for {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		remaining := g.cfg.MaxRowsPerRun - g.total
		if remaining <= 0 {
			g.capped = true
			slog.Warn("orphan_gc: per-run row cap reached; remainder waits for next run", "cap", g.cfg.MaxRowsPerRun, "phase", key)
			return done, nil
		}
		limit := int64(g.cfg.BatchSize)
		if limit > remaining {
			limit = remaining
		}
		selector := fmt.Sprintf("SELECT ctid FROM %s WHERE %s LIMIT %d", table, where, limit)

		var n int64
		if g.cfg.DryRun {
			// One counting pass up to the remaining budget.
			if err := g.pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM (%s) c", selector)).Scan(&n); err != nil {
				return done, fmt.Errorf("count %s: %w", key, err)
			}
			done += n
			g.total += n
			g.breakdown[key] += n
			return done, nil
		}

		tag, err := g.pool.Exec(ctx, stmt(selector))
		if err != nil {
			return done, fmt.Errorf("%s: %w", key, err)
		}
		n = tag.RowsAffected()
		done += n
		g.total += n
		g.breakdown[key] += n
		g.beat()
		if n < limit {
			return done, nil
		}
	}
}

func (g *orphanGC) del(ctx context.Context, key, table, ageCol, cond string) error {
	_, err := g.run(ctx, key, table, ageCol, cond, deleteStmt(table))
	return err
}

func (g *orphanGC) pruneCerebra(ctx context.Context) error {
	var errs []error
	// TTL-expired rows: the TTL itself is the age rule.
	errs = append(errs, g.del(ctx, "cerebra_model_unavailability_expired", "cerebra_model_unavailability", "",
		`marked_at + (ttl_seconds * INTERVAL '1 second') < NOW()`))
	errs = append(errs, g.del(ctx, "cerebra_model_unavailability", "cerebra_model_unavailability", "marked_at",
		missingUUIDRef("cerebra_model_unavailability.runtime_id", "agent_runtime")))
	errs = append(errs, g.del(ctx, "cerebra_routing_log", "cerebra_routing_log", "created_at", fmt.Sprintf(
		"%s OR %s OR %s OR %s",
		missingUUIDRef("cerebra_routing_log.task_id", "agent_task_queue"),
		missingUUIDRef("cerebra_routing_log.issue_id", "issue"),
		missingUUIDRef("cerebra_routing_log.session_id", "chat_session"),
		missingUUIDRef("cerebra_routing_log.runtime_id", "agent_runtime"))))
	return errors.Join(errs...)
}

func (g *orphanGC) pruneResourceLabels(ctx context.Context) error {
	return errors.Join(
		g.del(ctx, "agent_to_label", "agent_to_label", "created_at",
			`NOT EXISTS (SELECT 1 FROM agent WHERE agent.id = agent_to_label.agent_id)
			 OR NOT EXISTS (SELECT 1 FROM issue_label WHERE issue_label.id = agent_to_label.label_id)`),
		g.del(ctx, "skill_to_label", "skill_to_label", "created_at",
			`NOT EXISTS (SELECT 1 FROM skill WHERE skill.id = skill_to_label.skill_id)
			 OR NOT EXISTS (SELECT 1 FROM issue_label WHERE issue_label.id = skill_to_label.label_id)`),
		// issue_to_label has no timestamp column, so no grace age applies.
		g.del(ctx, "issue_to_label", "issue_to_label", "",
			`NOT EXISTS (SELECT 1 FROM issue WHERE issue.id = issue_to_label.issue_id)
			 OR NOT EXISTS (SELECT 1 FROM issue_label WHERE issue_label.id = issue_to_label.label_id)`),
	)
}

func (g *orphanGC) pruneMCPBindings(ctx context.Context) error {
	return g.del(ctx, "agent_mcp_server", "agent_mcp_server", "created_at",
		`NOT EXISTS (SELECT 1 FROM agent WHERE agent.id = agent_mcp_server.agent_id)
		 OR NOT EXISTS (SELECT 1 FROM workspace_mcp_server WHERE workspace_mcp_server.id = agent_mcp_server.server_id)`)
}

func (g *orphanGC) pruneChatDraftRestores(ctx context.Context) error {
	return g.del(ctx, "chat_draft_restore", "chat_draft_restore", "created_at",
		`NOT EXISTS (SELECT 1 FROM chat_session WHERE chat_session.id = chat_draft_restore.chat_session_id)`)
}

func (g *orphanGC) pruneTaskMessages(ctx context.Context) error {
	return g.del(ctx, "task_message", "task_message", "created_at",
		`NOT EXISTS (SELECT 1 FROM agent_task_queue WHERE agent_task_queue.id = task_message.task_id)`)
}

func (g *orphanGC) pruneComments(ctx context.Context) error {
	// Unlink parent_id when the referenced parent comment was deleted.
	unlink := func(sel string) string {
		return fmt.Sprintf("UPDATE comment SET parent_id = NULL WHERE ctid IN (%s)", sel)
	}
	_, err1 := g.run(ctx, "comment_parent_unlinked", "comment", "created_at",
		`parent_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM comment p WHERE p.id = comment.parent_id)`, unlink)

	// Delete comments whose issue or workspace was removed.
	err2 := g.del(ctx, "comments", "comment", "created_at",
		`NOT EXISTS (SELECT 1 FROM issue WHERE issue.id = comment.issue_id)
		 OR (workspace_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM workspace WHERE workspace.id = comment.workspace_id))`)
	return errors.Join(err1, err2)
}

func (g *orphanGC) pruneAttachments(ctx context.Context) error {
	exists, err := tableExists(ctx, g.pool, "attachment")
	if err != nil || !exists {
		return err
	}

	cond := `(NOT EXISTS (SELECT 1 FROM workspace WHERE workspace.id = attachment.workspace_id)
		 OR (attachment.issue_id IS NOT NULL
		     AND NOT EXISTS (SELECT 1 FROM issue WHERE issue.id = attachment.issue_id)
		     AND attachment.comment_id IS NULL AND attachment.task_id IS NULL
		     AND attachment.chat_session_id IS NULL AND attachment.chat_message_id IS NULL
		     AND attachment.source_context_id IS NULL))`

	// Never remove an attachment that another table still points at.
	if ok, err := tableExists(ctx, g.pool, "chat_draft_restore"); err != nil {
		return err
	} else if ok {
		cond += ` AND NOT EXISTS (SELECT 1 FROM chat_draft_restore d WHERE attachment.id = ANY(d.attachment_ids))`
	}
	if ok, err := tableExists(ctx, g.pool, "issue_source_context_object_intent"); err != nil {
		return err
	} else if ok {
		cond += ` AND NOT EXISTS (SELECT 1 FROM issue_source_context_object_intent o WHERE o.attachment_id = attachment.id)`
	}

	// Only the database rows are removed here. The stored files are not, so the
	// urls are logged for storage cleanup.
	return g.deleteAttachmentRows(ctx, cond)
}

func (g *orphanGC) deleteAttachmentRows(ctx context.Context, cond string) error {
	where := fmt.Sprintf("(%s) AND created_at < now() - interval '%d seconds'", cond, int64(g.cfg.GraceAge.Seconds()))
	if g.cfg.GraceAge <= 0 {
		where = cond
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := g.cfg.MaxRowsPerRun - g.total
		if remaining <= 0 {
			g.capped = true
			return nil
		}
		limit := int64(g.cfg.BatchSize)
		if limit > remaining {
			limit = remaining
		}
		selector := fmt.Sprintf("SELECT ctid FROM attachment WHERE %s LIMIT %d", where, limit)

		if g.cfg.DryRun {
			var n int64
			if err := g.pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM (%s) c", selector)).Scan(&n); err != nil {
				return fmt.Errorf("count attachments: %w", err)
			}
			g.total += n
			g.breakdown["attachments"] += n
			return nil
		}

		rows, err := g.pool.Query(ctx, fmt.Sprintf("DELETE FROM attachment WHERE ctid IN (%s) RETURNING url", selector))
		if err != nil {
			return fmt.Errorf("attachments: %w", err)
		}
		var urls []string
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return err
			}
			urls = append(urls, u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		n := int64(len(urls))
		g.total += n
		g.breakdown["attachments"] += n
		if n > 0 {
			slog.Info("orphan_gc: deleted attachment rows; storage objects may need cleanup", "count", n, "urls", urls)
		}
		g.beat()
		if n < limit {
			return nil
		}
	}
}
