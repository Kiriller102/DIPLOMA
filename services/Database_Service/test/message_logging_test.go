package test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	grpcapi "project/services/Database_Service/api/grpc"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/services"
)

type pilotEvent struct {
	Event      string         `json:"event"`
	ID         string         `json:"operation_id"`
	Level      string         `json:"level"`
	Code       string         `json:"rpc_code"`
	HTTPStatus int            `json:"http_status"`
	Written    bool           `json:"response_written"`
	Diagnosis  map[string]any `json:"diagnosis"`
}

// The sink is safe to inspect while network handlers are finishing.
type pilotSink struct {
	mu    sync.Mutex
	data  bytes.Buffer
	ready chan struct{}
}

func newPilotSink() *pilotSink { return &pilotSink{ready: make(chan struct{}, 100)} }
func (s *pilotSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	n, err := s.data.Write(p)
	s.mu.Unlock()
	s.ready <- struct{}{}
	return n, err
}
func (s *pilotSink) events(t *testing.T) []pilotEvent {
	t.Helper()
	s.mu.Lock()
	raw := append([]byte(nil), s.data.Bytes()...)
	s.mu.Unlock()
	if bytes.Contains(raw, []byte("PRIVATE")) {
		t.Fatal("secret leaked into completion")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var events []pilotEvent
	for {
		var e pilotEvent
		err := decoder.Decode(&e)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}
func (s *pilotSink) wait(t *testing.T) pilotEvent {
	t.Helper()
	select {
	case <-s.ready:
	case <-time.After(testWaitTimeout):
		t.Fatal("server completion was not logged")
	}
	events := s.events(t)
	if len(events) != 1 || events[0].Event != "operation.finished" || events[0].ID == "" {
		t.Fatalf("want one completion: %+v", events)
	}
	return events[0]
}

type failedPilotRows struct {
	messageRows
	failure error
}

func (r *failedPilotRows) Next([]driver.Value) error { return r.failure }

type pilotFixture struct {
	service *services.MessageService
	access  *auth.Service
	http    *rest.HTTPServer
	sink    *pilotSink
	started chan struct{}
	reads   atomic.Int32
	query   messageQuery
}

func newPilotFixture(t *testing.T, mode string) *pilotFixture {
	t.Helper()
	f := &pilotFixture{sink: newPilotSink(), started: make(chan struct{})}
	f.query = func(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "SELECT u.id,u.username") {
			if mode == "authentication" {
				return nil, &pq.Error{Code: "08006", Message: "PRIVATE"}
			}
			now := time.Now()
			return &messageRows{columns: []string{"id", "username", "email", "first_name", "last_name", "created_at", "updated_at", "is_active", "expires_at"}, values: [][]driver.Value{{int64(1), "user", "PRIVATE", "", "", now, now, true, now.Add(time.Hour)}}}, nil
		}
		if strings.Contains(query, "FROM chat_participants") {
			if mode == "access" {
				return nil, &pq.Error{Code: "XX000", Message: "PRIVATE"}
			}
			return &messageRows{columns: []string{"exists"}, values: [][]driver.Value{{mode != "forbidden"}}}, nil
		}
		if strings.Contains(query, "FROM messages") {
			f.reads.Add(1)
			switch mode {
			case "query":
				return nil, &pq.Error{Code: "XX000", Message: "PRIVATE"}
			case "scan":
				return &messageRows{columns: []string{"id", "chat_id", "sender_id", "content", "encryption_algorithm", "sent_at", "crypto_version", "client_id"}, values: [][]driver.Value{{"PRIVATE", int64(1), int64(1), "PRIVATE", "", time.Now(), int64(0), ""}}}, nil
			case "iteration":
				return &failedPilotRows{messageRows: messageRows{columns: []string{"id"}}, failure: &pq.Error{Code: "XX000", Detail: "PRIVATE"}}, nil
			case "cancel":
				close(f.started)
				<-ctx.Done()
				return nil, &pq.Error{Code: "57014", Message: "PRIVATE"}
			}
			return &messageRows{columns: []string{"id"}}, nil
		}
		return &messageRows{columns: []string{"exists"}, values: [][]driver.Value{{true}}}, nil
	}
	db, svc := contextMessageService(t, f.query)
	f.service = svc
	var err error
	f.access, err = auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	f.http = rest.NewHTTPServer(controllers.NewUserController(nil, f.access), controllers.NewMessageController(svc, f.access), controllers.NewChatController(nil, f.access), controllers.NewChatParticipantController(nil, f.access), db, f.access, false, logging.New(f.sink, slog.LevelDebug))
	t.Cleanup(f.http.CancelPolls)
	return f
}

// Uses NewGRPCServer and RegisterServices: no test-built interceptor chain.
func pilotGRPC(t *testing.T, f *pilotFixture) *grpc.ClientConn {
	t.Helper()
	s := grpcapi.NewGRPCServer(f.access, logging.New(f.sink, slog.LevelDebug))
	s.RegisterServices(nil, f.service, nil, nil, f.access)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Serve(listener) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()
		s.Shutdown(ctx)
		listener.Close()
		<-done
	})
	return conn
}

func TestHistoryLoggingProductionBoundaries(t *testing.T) {
	for _, transport := range []string{"rest", "grpc"} {
		for _, tc := range []struct {
			mode, stage, class string
			http               int
			rpc                codes.Code
		}{
			{"success", "", "", 200, codes.OK}, {"query", "query", "database", 500, codes.Internal},
			{"scan", "scan", "internal", 500, codes.Internal}, {"iteration", "iteration", "database", 500, codes.Internal},
			{"authentication", "authentication", "database", 503, codes.Unavailable}, {"access", "access", "database", 500, codes.Internal},
			{"forbidden", "access", "forbidden", 403, codes.PermissionDenied},
		} {
			t.Run(transport+"/"+tc.mode, func(t *testing.T) {
				f := newPilotFixture(t, tc.mode)
				if transport == "rest" {
					req := httptest.NewRequest("GET", "/messages/1?ignored=PRIVATE", nil)
					req.Header.Set("Authorization", "Bearer "+strings.Repeat("A", 43))
					w := httptest.NewRecorder()
					f.http.Handler().ServeHTTP(w, req)
					if w.Code != tc.http {
						t.Fatalf("HTTP %d body=%s", w.Code, w.Body.String())
					}
					if tc.http == 500 && w.Body.String() != `{"error":"internal server error"}` {
						t.Fatalf("response changed: %s", w.Body.String())
					}
				} else {
					conn := pilotGRPC(t, f)
					ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+strings.Repeat("A", 43)), testWaitTimeout)
					defer cancel()
					_, err := messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
					if status.Code(err) != tc.rpc {
						t.Fatalf("RPC=%v want=%v", err, tc.rpc)
					}
				}
				event := f.sink.wait(t)
				if tc.stage != "" && event.Diagnosis["stage"] != tc.stage {
					t.Fatalf("cause stage lost: %+v", event)
				}
				if stringFieldPilot(event.Diagnosis, "error_class") != tc.class {
					t.Fatalf("cause class lost: %+v", event)
				}
				if tc.mode == "authentication" || tc.mode == "access" || tc.mode == "forbidden" {
					if f.reads.Load() != 0 {
						t.Fatal("message query ran before access allowed")
					}
				}
				if transport == "rest" && event.HTTPStatus != tc.http {
					t.Fatalf("HTTP status not recorded: %+v", event)
				}
				if transport == "grpc" && event.Code != tc.rpc.String() {
					t.Fatalf("mapped code lost: %+v", event)
				}
			})
		}
	}
}

func TestHistoryLoggingCancellation(t *testing.T) {
	for _, transport := range []string{"rest", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			f := newPilotFixture(t, "cancel")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			if transport == "rest" {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/messages/1", nil).WithContext(ctx)
				req.Header.Set("Authorization", "Bearer "+strings.Repeat("A", 43))
				go func() {
					f.http.Handler().ServeHTTP(w, req)
					if w.Body.Len() != 0 {
						done <- errors.New("HTTP wrote canceled response")
					} else {
						done <- nil
					}
				}()
			} else {
				conn := pilotGRPC(t, f)
				ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+strings.Repeat("A", 43))
				go func() {
					_, err := messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
					if status.Code(err) != codes.Canceled {
						done <- errors.New("RPC code changed")
					} else {
						done <- nil
					}
				}()
			}
			select {
			case <-f.started:
			case <-time.After(testWaitTimeout):
				t.Fatal("query did not start")
			}
			cancel()
			if err := receiveTestError(t, done); err != nil {
				t.Fatal(err)
			}
			event := f.sink.wait(t)
			if event.Level != "ERROR" || event.Diagnosis["sqlstate"] != "57014" || event.Diagnosis["stage"] != "query" {
				t.Fatalf("SQL error replaced with transport cancellation: %+v", event)
			}
			if transport == "grpc" && event.Code != "Canceled" {
				t.Fatalf("missing final RPC code: %+v", event)
			}
			if transport == "rest" && event.Written {
				t.Fatal("canceled handler wrote response")
			}
		})
	}
}

func TestHistoryLoggingParallelRequests(t *testing.T) {
	for _, transport := range []string{"rest", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			f := newPilotFixture(t, "query")
			var conn *grpc.ClientConn
			if transport == "grpc" {
				conn = pilotGRPC(t, f)
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if transport == "rest" {
						req := httptest.NewRequest("GET", "/messages/1", nil)
						req.Header.Set("Authorization", "Bearer "+strings.Repeat("A", 43))
						w := httptest.NewRecorder()
						f.http.Handler().ServeHTTP(w, req)
						if w.Code != 500 {
							t.Error("unexpected HTTP response")
						}
					} else {
						ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+strings.Repeat("A", 43)), testWaitTimeout)
						defer cancel()
						_, err := messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
						if status.Code(err) != codes.Internal {
							t.Error("unexpected RPC response")
						}
					}
				}()
			}
			wg.Wait()
			waitForTestState(t, "all completion logs", func(context.Context) (bool, error) { return len(f.sink.events(t)) == 8, nil })
			seen := map[string]bool{}
			for _, e := range f.sink.events(t) {
				if e.ID == "" || seen[e.ID] || e.Diagnosis["stage"] != "query" {
					t.Fatalf("mixed or deduplicated events: %+v", e)
				}
				seen[e.ID] = true
			}
		})
	}
}

func TestHistoryLoggingDoesNotOwnOtherRoutes(t *testing.T) {
	f := newPilotFixture(t, "success")
	f.http.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	conn := pilotGRPC(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancel()
	if _, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := f.sink.events(t); len(got) != 0 {
		t.Fatalf("pilot leaked into other routes: %+v", got)
	}
}

func TestHistoryLoggingDirectCall(t *testing.T) {
	f := newPilotFixture(t, "query")
	_, raw := f.service.GetMessagesByChatID(context.Background(), 1)
	if len(f.sink.events(t)) != 0 {
		t.Fatal("core service emitted completion")
	}
	var callbackError error
	err := logging.Run(context.Background(), logging.New(f.sink, slog.LevelDebug), logging.Classifier{}, logging.MessageHistory, func(ctx context.Context) error {
		_, callbackError = f.service.GetMessagesByChatID(ctx, 1)
		return callbackError
	})
	if raw == nil || err != callbackError {
		t.Fatal("error contract changed")
	}
	e := f.sink.wait(t)
	if e.Diagnosis["stage"] != "query" {
		t.Fatalf("direct cause lost: %+v", e)
	}
}

func stringFieldPilot(fields map[string]any, name string) string {
	s, _ := fields[name].(string)
	return s
}
