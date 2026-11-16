package test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"project/services/Database_Service/migrations"
)

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path == "" || u.Path == "/" {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL with an explicit test database")
	}
	q := u.Query()
	q.Set("connect_timeout", "5")
	// Safety net for a regression that accidentally ignores the request context.
	q.Set("statement_timeout", "15000")
	u.RawQuery = q.Encode()
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()
	admin, err := sqlx.ConnectContext(setupCtx, "postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_messenger_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(setupCtx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.ConnectContext(setupCtx, "postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(4)
	for i := 0; i < 2; i++ {
		if err := migrations.Apply(setupCtx, db); err != nil {
			t.Fatal(err)
		}
		if err := migrations.SeedDemo(setupCtx, db); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.GetContext(setupCtx, &count, "SELECT COUNT(*) FROM chat_participants"); err != nil || count != 2 {
		t.Fatalf("idempotent seeding: participants=%d, err=%v", count, err)
	}
	return db
}
