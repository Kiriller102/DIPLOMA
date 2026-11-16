package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/services"
)

type cancellationFixture struct {
	key                           int64
	db, operationDB               *sqlx.DB
	observer                      *sqlx.Conn
	access                        *auth.Service
	users                         *services.UserService
	chats                         *services.ChatService
	participants                  *services.ChatParticipantService
	messages                      *services.MessageService
	token, app, content, migrated string
	messageID                     int
	principal                     context.Context
}

func newCancellationFixture(t *testing.T) *cancellationFixture {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var schema string
	if err := db.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	app := fmt.Sprintf("context_stage_c_%d", time.Now().UnixNano())
	q := u.Query()
	q.Set("search_path", schema)
	q.Set("application_name", app)
	q.Set("statement_timeout", "15000")
	q.Set("connect_timeout", "5")
	u.RawQuery = q.Encode()
	op, err := sqlx.ConnectContext(ctx, "postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { op.Close() })
	f := &cancellationFixture{db: db, operationDB: op, observer: postgresObserver(t, db), app: app, key: time.Now().UnixNano()}
	f.access, err = auth.New(op)
	if err != nil {
		t.Fatal(err)
	}
	f.users, err = services.NewUserService(op)
	if err != nil {
		t.Fatal(err)
	}
	f.chats, err = services.NewChatService(op)
	if err != nil {
		t.Fatal(err)
	}
	f.participants, err = services.NewChatParticipantService(op)
	if err != nil {
		t.Fatal(err)
	}
	f.messages, err = services.NewMessageService(op)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("context-stage-c-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET password_hash=$1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	session, err := f.access.Login(ctx, auth.Credentials{Email: "alice@example.com", Password: "context-stage-c-password"})
	if err != nil {
		t.Fatal(err)
	}
	f.token = session.Token
	// Keep the principal independent of the setup context, which is canceled on return.
	f.principal, err = f.access.Authenticate(context.Background(), f.token)
	if err != nil {
		t.Fatal(err)
	}
	crypto := newCryptoFixture(t, db)
	f.content = crypto.seal(t, "new message", 1, 1, 1, 0)
	msg := struct {
		ID int `db:"id"`
	}{}
	if err = db.GetContext(ctx, &msg, `INSERT INTO messages(chat_id,sender_id,content) VALUES(1,1,'legacy') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	f.messageID = msg.ID
	f.migrated = crypto.seal(t, "legacy", 1, 1, 1, f.messageID)
	return f
}

func mustSQL(t *testing.T, db *sqlx.DB, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

// application_name identifies only the operation's connections. pg_blocking_pids
// proves the dependency on our particular locker for row AND advisory locks.
func (f *cancellationFixture) waitBlocked(t *testing.T, blocker int, stage string) int {
	t.Helper()
	var pid int
	waitForTestState(t, "SQL stage "+stage+" blocked by expected backend", func(ctx context.Context) (bool, error) {
		err := f.observer.GetContext(ctx, &pid, `SELECT COALESCE((SELECT pid FROM pg_stat_activity
   WHERE datname=current_database() AND application_name=$1 AND state='active'
   AND wait_event_type='Lock' AND position($2 in regexp_replace(query,'[[:space:]]+',' ','g'))>0
   AND $3::int=ANY(pg_blocking_pids(pid)) LIMIT 1),0)`, f.app, stage, blocker)
		return pid != 0, err
	})
	t.Logf("confirmed operation backend=%d blocker=%d SQL=%s", pid, blocker, stage)
	return pid
}

func (f *cancellationFixture) waitStopped(t *testing.T, pid int) {
	t.Helper()
	waitForTestState(t, "backend stopped and transaction/advisory/row locks released", func(ctx context.Context) (bool, error) {
		var stopped bool
		err := f.observer.GetContext(ctx, &stopped, `SELECT
   NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND (state='active' OR state LIKE 'idle in transaction%'))
   AND NOT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype<>'virtualxid')`, pid)
		return stopped, err
	})
}

func (f *cancellationFixture) snapshot(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	var all []json.RawMessage
	for _, table := range []string{"users", "chats", "chat_participants", "messages", "user_sessions", "crypto_identities"} {
		var data string
		err := f.db.GetContext(ctx, &data, `SELECT COALESCE(jsonb_agg(row ORDER BY row::text),'[]'::jsonb)::text FROM (SELECT to_jsonb(t) row FROM `+table+` t) s`)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, json.RawMessage(data))
	}
	raw, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type sqlBarrier struct {
	pid     int
	release func()
}

// The trigger runs inside the application's statement, not in a test goroutine.
// A BEFORE trigger can stop a later write after earlier writes in the same tx.
func (f *cancellationFixture) advisoryBarrier(t *testing.T, table, event, condition string) sqlBarrier {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	locker, err := f.db.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = locker.Close() })
	var pid int
	if err = locker.GetContext(ctx, &pid, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	key := f.key
	if _, err = locker.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		c, done := context.WithTimeout(context.Background(), testWaitTimeout)
		defer done()
		if _, err := locker.ExecContext(c, "SELECT pg_advisory_unlock($1)", key); err != nil {
			t.Error(err)
		}
		locker.Close()
	}
	t.Cleanup(release)
	function := fmt.Sprintf(`CREATE FUNCTION context_write_barrier() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF current_setting('application_name')=%s AND (%s) THEN PERFORM pg_advisory_xact_lock(%d); END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF; END $$`, quoteSQL(f.app), condition, key)
	mustSQL(t, f.db, function)
	mustSQL(t, f.db, "CREATE TRIGGER context_write_barrier BEFORE "+event+" ON "+table+" FOR EACH ROW EXECUTE FUNCTION context_write_barrier()")
	t.Cleanup(func() {
		mustSQL(t, f.db, "DROP TRIGGER context_write_barrier ON "+table)
		mustSQL(t, f.db, "DROP FUNCTION context_write_barrier()")
	})
	return sqlBarrier{pid, release}
}

// An updatable view adds a test-only read barrier without blocking preceding
// authentication queries. FK references continue to target the renamed table.
func (f *cancellationFixture) readBarrier(t *testing.T, table, stage string) sqlBarrier {
	return f.readBarrierCondition(t, table, stage, "true")
}
func (f *cancellationFixture) readBarrierCondition(t *testing.T, table, stage, condition string) sqlBarrier {
	t.Helper()
	b := f.advisoryLocker(t)
	mustSQL(t, f.db, "ALTER TABLE "+table+" RENAME TO context_backing")
	mustSQL(t, f.db, fmt.Sprintf(`CREATE FUNCTION context_read_barrier() RETURNS boolean LANGUAGE plpgsql VOLATILE AS $$ BEGIN
 IF current_setting('application_name')=%s AND position(%s in regexp_replace(current_query(),'[[:space:]]+',' ','g'))>0 AND (%s) THEN PERFORM pg_advisory_xact_lock(%d); END IF; RETURN true; END $$`, quoteSQL(f.app), quoteSQL(stage), condition, f.key))
	mustSQL(t, f.db, "CREATE VIEW "+table+" AS SELECT * FROM context_backing WHERE context_read_barrier()")
	t.Cleanup(func() {
		mustSQL(t, f.db, "DROP VIEW "+table)
		mustSQL(t, f.db, "ALTER TABLE context_backing RENAME TO "+table)
		mustSQL(t, f.db, "DROP FUNCTION context_read_barrier()")
	})
	return b
}
func quoteSQL(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (f *cancellationFixture) advisoryLocker(t *testing.T) sqlBarrier {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	conn, err := f.db.Connx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var pid int
	if err = conn.GetContext(ctx, &pid, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", f.key); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		c, done := context.WithTimeout(context.Background(), testWaitTimeout)
		defer done()
		if _, err := conn.ExecContext(c, "SELECT pg_advisory_unlock($1)", f.key); err != nil {
			t.Error(err)
		}
		conn.Close()
	}
	t.Cleanup(release)
	return sqlBarrier{pid, release}
}
func (f *cancellationFixture) rowBarrier(t *testing.T, table, where string) sqlBarrier {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	tx, err := f.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, "SELECT * FROM "+table+" WHERE "+where+" FOR UPDATE"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	var pid int
	if err = tx.GetContext(ctx, &pid, "SELECT pg_backend_pid()"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	release := func() { _ = tx.Rollback() }
	t.Cleanup(release)
	return sqlBarrier{pid, release}
}
