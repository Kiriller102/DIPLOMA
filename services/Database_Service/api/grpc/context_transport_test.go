package grpc

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	userpb "project/services/Database_Service/api/grpc/pb/user"
	gs "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/migrations"
	"project/services/Database_Service/services"
)

func grpcTestWait(t *testing.T, description string, check func(context.Context) (bool, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		ok, err := check(ctx)
		if err != nil {
			t.Fatalf("%s: %v", description, err)
		}
		if ok {
			return
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatalf("%s: %v", description, ctx.Err())
		}
	}
}

// Package-local integration test reuses the actual production interceptors.
// An outer observer records server completion and the server's own error code;
// a client-side Canceled/DeadlineExceeded is never the only assertion.
func TestContextStageCGRPCClient(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	setup, cancelSetup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSetup()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("connect_timeout", "5")
	q.Set("statement_timeout", "15000")
	u.RawQuery = q.Encode()
	admin, err := sqlx.ConnectContext(setup, "postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("test_context_grpc_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(setup, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	app := schema
	q.Set("search_path", schema)
	q.Set("application_name", app)
	u.RawQuery = q.Encode()
	db, err := sqlx.ConnectContext(setup, "postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = migrations.Apply(setup, db); err != nil {
		t.Fatal(err)
	}
	if err = migrations.SeedDemo(setup, db); err != nil {
		t.Fatal(err)
	}
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("grpc-context-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(setup, `UPDATE users SET password_hash=$1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	session, err := access.Login(setup, auth.Credentials{Email: "alice@example.com", Password: "grpc-context-password"})
	if err != nil {
		t.Fatal(err)
	}
	users, err := services.NewUserService(db)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"user_update", "history", "authentication"} {
		for _, mode := range []string{"cancel", "deadline"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				ctxSetup, cancelSetup := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancelSetup()
				state := func() string {
					var data string
					err := db.GetContext(ctxSetup, &data, `SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM users u),
 'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY token_hash) FROM user_sessions s),
 'messages',(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM messages m))::text`)
					if err != nil {
						t.Fatal(err)
					}
					return data
				}
				before := state()
				tx, err := db.BeginTxx(ctxSetup, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				stage := "UPDATE users"
				lock := "SELECT id FROM users WHERE id=1 FOR UPDATE"
				if name == "history" {
					lock = "LOCK TABLE messages IN ACCESS EXCLUSIVE MODE"
					stage = "FROM messages"
				}
				if name == "authentication" {
					lock = "LOCK TABLE user_sessions IN ACCESS EXCLUSIVE MODE"
					stage = "SELECT u.id,u.username"
				}
				if _, err = tx.ExecContext(ctxSetup, lock); err != nil {
					t.Fatal(err)
				}
				var blocker int
				if err = tx.GetContext(ctxSetup, &blocker, "SELECT pg_backend_pid()"); err != nil {
					t.Fatal(err)
				}
				serverDone := make(chan error, 1)
				observe := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (interface{}, error) {
					response, err := next(ctx, req)
					serverDone <- err
					return response, err
				}
				server := grpc.NewServer(grpc.ChainUnaryInterceptor(observe, mapErrors, authenticate(access)))
				defer server.Stop()
				userpb.RegisterUserServiceServer(server, gs.NewUserGRPCsService(users, access))
				messagepb.RegisterMessageServiceServer(server, gs.NewMessageGRPCService(messages, access))
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				served := make(chan struct{})
				go func() { defer close(served); _ = server.Serve(listener) }()
				defer func() {
					server.Stop()
					listener.Close()
					select {
					case <-served:
					case <-time.After(5 * time.Second):
						t.Error("gRPC Serve did not finish")
					}
				}()
				conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				parent := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)
				var ctx context.Context
				var cancel context.CancelFunc
				if mode == "deadline" {
					ctx, cancel = context.WithTimeout(parent, 2*time.Second)
				} else {
					ctx, cancel = context.WithCancel(parent)
				}
				defer cancel()
				clientDone := make(chan error, 1)
				go func() {
					var err error
					if name == "user_update" {
						_, err = userpb.NewUserServiceClient(conn).UpdateUser(ctx, &userpb.UpdateUserRequest{User: &userpb.User{Id: 1, FirstName: "canceled"}})
					} else {
						_, err = messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
					}
					clientDone <- err
				}()
				pid := 0
				cleanup := func() {
					c, done := context.WithTimeout(context.Background(), 5*time.Second)
					defer done()
					if pid != 0 {
						var ok bool
						if err := admin.GetContext(c, &ok, "SELECT pg_cancel_backend($1)", pid); err != nil {
							t.Error(err)
						}
					}
				}
				defer cleanup()
				grpcTestWait(t, "operation backend waits for exact blocker", func(c context.Context) (bool, error) {
					e := admin.GetContext(c, &pid, `SELECT COALESCE((SELECT pid FROM pg_stat_activity WHERE application_name=$1 AND state='active' AND wait_event_type='Lock' AND position($2 in query)>0 AND $3::int=ANY(pg_blocking_pids(pid)) LIMIT 1),0)`, app, stage, blocker)
					return pid != 0, e
				})
				t.Logf("backend=%d blocker=%d SQL=%s", pid, blocker, stage)
				if mode == "cancel" {
					cancel()
				} else {
					select {
					case <-ctx.Done():
					case <-time.After(3 * time.Second):
						t.Fatal("deadline did not expire")
					}
				}
				want := codes.Canceled
				if mode == "deadline" {
					want = codes.DeadlineExceeded
				}
				select {
				case err := <-clientDone:
					if status.Code(err) != want {
						t.Errorf("client=%v want %s", err, want)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("client did not finish")
				}
				var serverError error
				select {
				case serverError = <-serverDone:
				case <-time.After(time.Second):
					t.Error("DEFECT: client finished but server SQL did not stop")
					cleanup()
					select {
					case serverError = <-serverDone:
					case <-time.After(5 * time.Second):
						t.Fatal("server did not finish after cleanup")
					}
				}
				if status.Code(serverError) != want {
					t.Errorf("DEFECT: server interceptor=%v, want %s", serverError, want)
				}
				grpcTestWait(t, "SQL terminated and transaction locks released", func(c context.Context) (bool, error) {
					var stopped bool
					e := admin.GetContext(c, &stopped, `SELECT NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND (state='active' OR state LIKE 'idle in transaction%')) AND NOT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype<>'virtualxid')`, pid)
					return stopped, e
				})
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if state() != before {
					t.Error("data changed after canceled RPC")
				}

			})
		}
	}
}
