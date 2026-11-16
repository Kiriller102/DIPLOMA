package test

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

const testWaitTimeout = 5 * time.Second

// The callback must honor ctx. Each observation shares the same total deadline.
func waitForTestState(t *testing.T, description string, check func(context.Context) (bool, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, err := check(ctx)
		if err != nil {
			t.Fatalf("%s: %v", description, err)
		}
		if ok {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("%s: %v", description, ctx.Err())
		}
	}
}

// Cleanup cancels the request and releases test-owned blockers before waiting
// for server completion. A buffered result cannot strand a finished goroutine.
func startTestOperation(t *testing.T, cleanup func(), run func() error) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() { defer close(finished); result <- run() }()
	t.Cleanup(func() {
		cleanup()
		select {
		case <-finished:
		case <-time.After(testWaitTimeout):
			t.Error("server operation did not finish during cleanup")
		}
	})
	return result
}

func receiveTestError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(testWaitTimeout):
		t.Fatal("server operation did not finish")
		return nil
	}
}

// Observe on a reserved connection, independent of the pool used by the request.
func postgresObserver(t *testing.T, db *sqlx.DB) *sqlx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	conn, err := db.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn
}

func waitForPostgresLock(t *testing.T, observer *sqlx.Conn, relation, mode string, blocker int) int {
	t.Helper()
	var pid int
	waitForTestState(t, "waiting for PostgreSQL lock on "+relation, func(ctx context.Context) (bool, error) {
		err := observer.GetContext(ctx, &pid, `SELECT COALESCE((SELECT pid FROM pg_locks
   WHERE relation=$1::regclass AND mode=$2 AND NOT granted
   AND pid<>pg_backend_pid() AND $3::int=ANY(pg_blocking_pids(pid)) LIMIT 1),0)`, relation, mode, blocker)
		return pid != 0, err
	})
	return pid
}

func waitForPostgresRelease(t *testing.T, observer *sqlx.Conn, pid int, relation string) {
	t.Helper()
	waitForTestState(t, "waiting for PostgreSQL operation and locks to finish", func(ctx context.Context) (bool, error) {
		var released bool
		err := observer.GetContext(ctx, &released, `SELECT
   NOT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND relation=$2::regclass)
   AND NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND state='active')`, pid, relation)
		return released, err
	})
}

// Wrap the handler's Gin writer, not the network writer: Gin itself writes a
// default status after the handler returns, even when the handler wrote nothing.
type handlerResponseWrites struct {
	gin.ResponseWriter
	headers, bodies, flushes int
}

func (w *handlerResponseWrites) WriteHeader(code int) {
	w.headers++
	w.ResponseWriter.WriteHeader(code)
}
func (w *handlerResponseWrites) WriteHeaderNow() { w.headers++; w.ResponseWriter.WriteHeaderNow() }
func (w *handlerResponseWrites) Write(p []byte) (int, error) {
	w.bodies++
	return w.ResponseWriter.Write(p)
}
func (w *handlerResponseWrites) WriteString(s string) (int, error) {
	w.bodies++
	return w.ResponseWriter.WriteString(s)
}
func (w *handlerResponseWrites) Flush() { w.flushes++; w.ResponseWriter.Flush() }
func observeHandlerWrites(handler gin.HandlerFunc) (gin.HandlerFunc, *handlerResponseWrites) {
	writes := &handlerResponseWrites{}
	return func(c *gin.Context) { writes.ResponseWriter = c.Writer; c.Writer = writes; handler(c) }, writes
}
func (w *handlerResponseWrites) assertNone(t *testing.T) {
	t.Helper()
	if w.headers != 0 || w.bodies != 0 || w.flushes != 0 {
		t.Errorf("handler wrote response after cancellation: headers=%d bodies=%d flushes=%d", w.headers, w.bodies, w.flushes)
	}
}
