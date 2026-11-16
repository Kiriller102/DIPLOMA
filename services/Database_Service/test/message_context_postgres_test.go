package test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	grpcserver "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/services"
)

func TestMessageReadPostgresCancellation(t *testing.T) {
	db := testDB(t)
	observer := postgresObserver(t, db)
	svc, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("context-test-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET password_hash=$1 WHERE id=1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	session, err := access.Login(context.Background(), auth.Credentials{Email: "alice@example.com", Password: "context-test-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	var messageID int
	if err = db.Get(&messageID, `INSERT INTO messages(chat_id,sender_id,content) VALUES(1,1,'context cancellation fixture') RETURNING id`); err != nil {
		t.Fatal(err)
	}

	for _, transport := range []string{"service", "HTTP", "gRPC"} {
		t.Run(transport, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, err := access.Authenticate(parent, session.Token)
			if err != nil {
				t.Fatal(err)
			}
			// Only this test schema is locked. Authentication and membership queries
			// use other tables and can finish before the message SELECT blocks.
			tx, err := db.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec(`LOCK TABLE messages IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}

			setup, setupCancel := context.WithTimeout(context.Background(), testWaitTimeout)
			var blocker int
			err = tx.GetContext(setup, &blocker, "SELECT pg_backend_pid()")
			setupCancel()
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			var writes *handlerResponseWrites
			var read func() error
			switch transport {
			case "service":
				read = func() error { _, err := svc.GetMessagesByChatID(ctx, 1); return err }
			case "HTTP":
				router := gin.New()
				var handler gin.HandlerFunc
				handler, writes = observeHandlerWrites(controllers.NewMessageController(svc, access).GetMessagesByChatID)
				router.GET("/messages/:chat_id", handler)
				req := httptest.NewRequest(http.MethodGet, "/messages/1", nil).WithContext(ctx)
				read = func() error { router.ServeHTTP(recorder, req); return nil }
			case "gRPC":
				handler := grpcserver.NewMessageGRPCService(svc, access)
				read = func() error {
					_, err := handler.GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
					return err
				}
			}
			done := startTestOperation(t, func() { cancel(); _ = tx.Rollback() }, read)
			pid := waitForPostgresLock(t, observer, "messages", "AccessShareLock", blocker)
			started := time.Now()
			cancel()
			err = receiveTestError(t, done)
			switch transport {
			case "service":
				// lib/pq reports server-side query cancellation as SQLSTATE 57014.
				var pg *pq.Error
				if !errors.As(err, &pg) || pg.Code != "57014" {
					t.Fatalf("got %v, want PostgreSQL query_canceled (57014)", err)
				}
			case "HTTP":
				writes.assertNone(t)
				if recorder.Body.Len() != 0 {
					t.Fatalf("canceled handler wrote response: %s", recorder.Body.String())
				}
			case "gRPC":
				if status.Code(err) != codes.Canceled {
					t.Fatalf("got %v, want gRPC Canceled", err)
				}
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("context not canceled: %v", ctx.Err())
			}
			waitForPostgresRelease(t, observer, pid, "messages")
			t.Logf("PostgreSQL backend %d: cancellation completed in %s; message locks released", pid, time.Since(started))
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			messages, err := svc.GetMessagesByChatID(context.Background(), 1)
			if err != nil {
				t.Fatalf("read after cancellation: %v", err)
			}
			found := false
			for _, message := range messages {
				if message.ID == messageID && message.Content == "context cancellation fixture" {
					found = true
				}
			}
			if !found {
				t.Fatalf("fixture message %d missing after cancellation", messageID)
			}
		})
	}
}
