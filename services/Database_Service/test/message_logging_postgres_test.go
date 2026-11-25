package test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/internal/logging"
)

func TestHistoryLoggingPostgresProductionCancellation(t *testing.T) {
	for _, transport := range []string{"direct", "rest", "grpc"} {
		for _, point := range []struct{ name, table, sql, diagnostic string }{
			{"query", "messages", "SELECT id, chat_id, sender_id, content", "query"},
			{"authentication", "user_sessions", "SELECT u.id,u.username", "authentication"},
			{"access", "user_sessions", "SELECT EXISTS(SELECT 1 FROM user_sessions", "access"},
		} {
			if transport == "direct" && point.name != "query" {
				continue
			}
			t.Run(transport+"/"+point.name, func(t *testing.T) {
				f := newCancellationFixture(t)
				sink := newPilotSink()
				logger := logging.New(sink, slog.LevelDebug)
				barrier := f.readBarrier(t, point.table, point.sql)
				before := f.snapshot(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var read func() error
				switch transport {
				case "direct":
					read = func() error {
						return logging.Run(ctx, logger, logging.Classifier{}, logging.MessageHistory, func(ctx context.Context) error { _, err := f.messages.GetMessagesByChatID(ctx, 1); return err })
					}
				case "rest":
					server := rest.NewHTTPServer(controllers.NewUserController(f.users, f.access), controllers.NewMessageController(f.messages, f.access), controllers.NewChatController(f.chats, f.access), controllers.NewChatParticipantController(f.participants, f.access), f.operationDB, f.access, false, logger)
					t.Cleanup(server.CancelPolls)
					ts := httptest.NewServer(server.Handler())
					t.Cleanup(ts.Close)
					req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/messages/1", nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", "Bearer "+f.token)
					read = func() error {
						response, err := ts.Client().Do(req)
						if response != nil {
							io.Copy(io.Discard, response.Body)
							response.Body.Close()
						}
						return err
					}
				case "grpc":
					conn := pilotGRPC(t, &pilotFixture{service: f.messages, access: f.access, sink: sink})
					ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+f.token)
					read = func() error {
						_, err := messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
						return err
					}
				}
				done := startTestOperation(t, func() { cancel(); barrier.release() }, read)
				pid := f.waitBlocked(t, barrier.pid, point.sql)
				cancel()
				err := receiveTestError(t, done)
				if transport == "direct" {
					var pg *pq.Error
					if !errors.As(err, &pg) || pg.Code != "57014" {
						t.Fatalf("lost raw SQL error: %v", err)
					}
				} else if transport == "grpc" {
					if status.Code(err) != codes.Canceled {
						t.Fatalf("RPC=%v", err)
					}
				} else if !errors.Is(err, context.Canceled) {
					t.Fatalf("HTTP client=%v", err)
				}
				event := sink.wait(t)
				if event.Level != "ERROR" || event.Diagnosis["sqlstate"] != "57014" || event.Diagnosis["stage"] != point.diagnostic {
					t.Fatalf("lost original cancellation: %+v", event)
				}
				if transport == "grpc" && event.Code != "Canceled" {
					t.Fatalf("lost final server code: %+v", event)
				}
				if transport == "rest" && event.Written {
					t.Fatal("canceled HTTP wrote a response")
				}
				f.waitStopped(t, pid)
				barrier.release()
				if f.snapshot(t) != before {
					t.Fatal("cancellation changed persisted state")
				}
				t.Logf("production %s stage=%s backend=%d SQLSTATE=%v level=%s completions=1; SQL stopped, locks released, data unchanged", transport, point.name, pid, event.Diagnosis["sqlstate"], event.Level)
			})
		}
	}
}
