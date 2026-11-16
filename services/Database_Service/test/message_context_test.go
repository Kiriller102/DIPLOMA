package test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	grpcserver "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

// A context-aware database/sql driver keeps cancellation tests independent of PostgreSQL.
type messageQuery func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
type messageConnector struct{ query messageQuery }
type messageConn struct{ query messageQuery }
type messageDriver struct{}

func (c messageConnector) Connect(context.Context) (driver.Conn, error) {
	return messageConn{c.query}, nil
}
func (c messageConnector) Driver() driver.Driver       { return messageDriver{} }
func (messageDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }
func (messageConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare: use QueryContext")
}
func (messageConn) Close() error              { return nil }
func (messageConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c messageConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type messageRows struct {
	columns []string
	values  [][]driver.Value
}

func (r *messageRows) Columns() []string { return r.columns }
func (r *messageRows) Close() error      { return nil }
func (r *messageRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func contextMessageService(t *testing.T, query messageQuery) (*sqlx.DB, *services.MessageService) {
	t.Helper()
	db := sqlx.NewDb(sql.OpenDB(messageConnector{query}), "message-context-test")
	t.Cleanup(func() { db.Close() })
	svc, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, svc
}

func TestMessageReadContext(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		_, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			t.Error("canceled context reached driver")
			return nil, errors.New("unexpected query")
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := svc.GetMessagesByChatID(ctx, 1)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
	t.Run("cancel running query", func(t *testing.T) {
		started := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, svc := contextMessageService(t, func(got context.Context, _ string, args []driver.NamedValue) (driver.Rows, error) {
			if got != ctx {
				t.Error("service did not forward request context")
			}
			if len(args) != 1 || args[0].Value != int64(7) {
				t.Errorf("unexpected chat ID: %v", args)
			}
			close(started)
			select {
			case <-got.Done():
				return nil, got.Err()
			case <-time.After(testWaitTimeout):
				return nil, errors.New("driver did not receive cancellation")
			}
		})
		done := startTestOperation(t, cancel, func() error { _, err := svc.GetMessagesByChatID(ctx, 7); return err })
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("query did not start")
		}
		cancel()
		if err := receiveTestError(t, done); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("cancel waiting for connection", func(t *testing.T) {
		db, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			t.Error("query must not run while only connection is held")
			return nil, errors.New("unexpected query")
		})
		db.SetMaxOpenConns(1)
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := startTestOperation(t, cancel, func() error { _, err := svc.GetMessagesByChatID(ctx, 1); return err })
		waitForTestState(t, "waiting for connection pool", func(context.Context) (bool, error) {
			return db.Stats().WaitCount > 0, nil
		})
		cancel()
		if err := receiveTestError(t, done); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})
	for _, empty := range []bool{false, true} {
		name := "messages"
		if empty {
			name = "empty chat"
		}
		t.Run(name, func(t *testing.T) {
			sentAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			_, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				rows := &messageRows{columns: []string{"id", "chat_id", "sender_id", "content", "encryption_algorithm", "sent_at", "crypto_version", "client_id"}}
				if !empty {
					rows.values = [][]driver.Value{{int64(2), int64(7), int64(3), "encrypted", "AES", sentAt, int64(1), "client"}}
				}
				return rows, nil
			})
			got, err := svc.GetMessagesByChatID(context.Background(), 7)
			if err != nil {
				t.Fatal(err)
			}
			if empty {
				if got == nil || len(got) != 0 {
					t.Fatalf("want empty non-nil slice, got %v", got)
				}
				return
			}
			want := models.Message{ID: 2, ChatID: 7, SenderID: 3, Content: "encrypted", EncryptionAlgorithm: "AES", SentAt: sentAt, CryptoVersion: 1, ClientID: "client"}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

// Authentication and membership succeed; only reading messages blocks.
// Return a driver error after cancellation to check handlers also inspect ctx.Err().
func messageReadHandlerFixture(t *testing.T) (*services.MessageService, *auth.Service, context.Context, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	db, svc := contextMessageService(t, func(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "SELECT u.id,u.username") {
			now := time.Now()
			return &messageRows{columns: []string{"id", "username", "email", "first_name", "last_name", "created_at", "updated_at", "is_active", "expires_at"}, values: [][]driver.Value{{int64(1), "user", "user@example.org", "", "", now, now, true, now.Add(time.Hour)}}}, nil
		}
		if strings.Contains(query, "FROM messages") {
			close(started)
			select {
			case <-ctx.Done():
				return nil, errors.New("driver: query canceled")
			case <-time.After(testWaitTimeout):
				return nil, errors.New("driver did not receive cancellation")
			}
		}
		return &messageRows{columns: []string{"exists"}, values: [][]driver.Value{{true}}}, nil
	})
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx, err := access.Authenticate(parent, strings.Repeat("A", 43))
	if err != nil {
		t.Fatal(err)
	}
	return svc, access, ctx, cancel, started
}

func TestMessageReadHTTPCancellation(t *testing.T) {
	svc, access, ctx, cancel, started := messageReadHandlerFixture(t)
	router := gin.New()
	handler, writes := observeHandlerWrites(controllers.NewMessageController(svc, access).GetMessagesByChatID)
	router.GET("/messages/:chat_id", handler)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/messages/7", nil).WithContext(ctx)
	done := startTestOperation(t, cancel, func() error { router.ServeHTTP(recorder, req); return nil })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("message query did not start")
	}
	cancel()
	receiveTestError(t, done)
	writes.assertNone(t)
	if recorder.Body.Len() != 0 {
		t.Fatalf("canceled handler wrote response: %s", recorder.Body.String())
	}
}

func TestMessageReadGRPCCancellation(t *testing.T) {
	svc, access, ctx, cancel, started := messageReadHandlerFixture(t)
	handler := grpcserver.NewMessageGRPCService(svc, access)
	done := startTestOperation(t, cancel, func() error {
		_, err := handler.GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 7})
		return err
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("message query did not start")
	}
	cancel()
	if err := receiveTestError(t, done); status.Code(err) != codes.Canceled {
		t.Fatalf("got %v, want Canceled", err)
	}
}
