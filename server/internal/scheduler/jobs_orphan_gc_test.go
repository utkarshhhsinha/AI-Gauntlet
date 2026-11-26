package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/utkarshhhsinha/AI-Gauntlet/server/internal/testutil"
)

func TestDatabaseOrphanGCJob_Spec(t *testing.T) {
	spec := DatabaseOrphanGCJob(nil)
	if spec.Name != JobNameDatabaseOrphanGC {
		t.Fatalf("expected name %q, got %q", JobNameDatabaseOrphanGC, spec.Name)
	}
	if spec.Cadence != 1*time.Hour {
		t.Fatalf("expected cadence 1h, got %v", spec.Cadence)
	}
	if spec.CatchUpMode != CatchUpLatestOnly {
		t.Fatalf("expected CatchUpLatestOnly, got %v", spec.CatchUpMode)
	}
	if spec.MaxAttempts != 3 {
		t.Fatalf("expected MaxAttempts=3, got %d", spec.MaxAttempts)
	}
	if spec.Handler == nil {
		t.Fatal("expected Handler to be non-nil")
	}
}

// gcTestEnv holds a real runtime so tests can seed both valid and orphaned
// cerebra_routing_log rows. cerebra_routing_log has no foreign keys, so a bad
// row can really be inserted (unlike task_message, whose cascade FK rejects it).
type gcTestEnv struct {
	pool      *pgxpool.Pool
	runtimeID string
}

func newGCTestEnv(t *testing.T) *gcTestEnv {
	t.Helper()
	pool := integrationPool(t)
	if ok, _ := tableExists(context.Background(), pool, "cerebra_routing_log"); !ok {
		t.Skip("cerebra_routing_log not present")
	}
	f := testutil.New(pool, "", "")
	f.UserID = f.User(t, "gc test", "gc-"+uuid.NewString()+"@example.com")
	f.WorkspaceID = f.Workspace(t, "gc ws", "gc-"+uuid.NewString())
	return &gcTestEnv{pool: pool, runtimeID: f.Runtime(t, "gc runtime")}
}

// insertLog seeds one routing log row aged ageHours old. Insert errors fail the
// test instead of being ignored.
func (e *gcTestEnv) insertLog(t *testing.T, runtimeID string, ageHours int) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO cerebra_routing_log (id, runtime_id, chosen_model, tier, matched_rule, status, created_at)
		VALUES ($1, $2, 'test-model', 'standard', 'test-rule', 'ok', now() - make_interval(hours => $3))
	`, id, runtimeID, ageHours); err != nil {
		t.Fatalf("seed cerebra_routing_log: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), `DELETE FROM cerebra_routing_log WHERE id = $1`, id)
	})
	return id
}

func (e *gcTestEnv) logExists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cerebra_routing_log WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count cerebra_routing_log: %v", err)
	}
	return n > 0
}

func noGrace() OrphanGCConfig {
	return OrphanGCConfig{GraceAge: 0, BatchSize: 100, MaxRowsPerRun: 1_000_000, WarnThreshold: 1_000_000}
}

func TestDatabaseOrphanGC_DeletesOrphansKeepsValidRows(t *testing.T) {
	e := newGCTestEnv(t)
	ctx := context.Background()

	orphan := e.insertLog(t, uuid.NewString(), 2) // runtime does not exist
	valid := e.insertLog(t, e.runtimeID, 2)       // runtime exists
	malformed := e.insertLog(t, "not-a-uuid", 2)  // cannot be judged, must be kept

	res, err := RunDatabaseOrphanGCWithConfig(ctx, e.pool, nil, noGrace())
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if res.RowsAffected < 1 {
		t.Errorf("expected at least 1 row affected, got %d", res.RowsAffected)
	}
	if e.logExists(t, orphan) {
		t.Error("orphaned routing log should have been deleted")
	}
	if !e.logExists(t, valid) {
		t.Error("routing log with a live runtime must be kept")
	}
	if !e.logExists(t, malformed) {
		t.Error("routing log with a non-UUID runtime_id must be kept, not treated as an orphan")
	}
}

func TestDatabaseOrphanGC_GraceAgeProtectsFreshRows(t *testing.T) {
	e := newGCTestEnv(t)
	cfg := noGrace()
	cfg.GraceAge = time.Hour

	fresh := e.insertLog(t, uuid.NewString(), 0) // just written, parent may still be committing
	old := e.insertLog(t, uuid.NewString(), 3)

	if _, err := RunDatabaseOrphanGCWithConfig(context.Background(), e.pool, nil, cfg); err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if !e.logExists(t, fresh) {
		t.Error("orphan younger than the grace age must be kept")
	}
	if e.logExists(t, old) {
		t.Error("orphan older than the grace age should have been deleted")
	}
}

func TestDatabaseOrphanGC_DryRunDeletesNothing(t *testing.T) {
	e := newGCTestEnv(t)
	cfg := noGrace()
	cfg.DryRun = true

	orphan := e.insertLog(t, uuid.NewString(), 3)

	res, err := RunDatabaseOrphanGCWithConfig(context.Background(), e.pool, nil, cfg)
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if !e.logExists(t, orphan) {
		t.Fatal("dry run must not delete rows")
	}
	if res.RowsAffected != 0 {
		t.Errorf("dry run must report 0 rows affected, got %d", res.RowsAffected)
	}
	if got, _ := res.Result["total_pruned"].(int64); got < 1 {
		t.Errorf("dry run should count the orphan, total_pruned=%v", res.Result["total_pruned"])
	}
}

func TestDatabaseOrphanGC_RowCapStopsRun(t *testing.T) {
	e := newGCTestEnv(t)
	cfg := noGrace()
	cfg.MaxRowsPerRun = 2
	cfg.BatchSize = 1

	for i := 0; i < 5; i++ {
		e.insertLog(t, uuid.NewString(), 3)
	}

	res, err := RunDatabaseOrphanGCWithConfig(context.Background(), e.pool, nil, cfg)
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if res.RowsAffected != 2 {
		t.Errorf("expected exactly the cap (2) rows affected, got %d", res.RowsAffected)
	}
	if capped, _ := res.Result["capped"].(bool); !capped {
		t.Error("expected result to report capped=true")
	}
}

func TestDatabaseOrphanGC_CancelledContextStopsWithError(t *testing.T) {
	e := newGCTestEnv(t)
	orphan := e.insertLog(t, uuid.NewString(), 3)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunDatabaseOrphanGCWithConfig(ctx, e.pool, nil, noGrace()); err == nil {
		t.Fatal("expected an error when the context is already cancelled")
	}
	if !e.logExists(t, orphan) {
		t.Error("nothing should be deleted when the run is cancelled before it starts")
	}
}
