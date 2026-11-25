package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	grpcapi "project/services/Database_Service/api/grpc"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	"project/services/Database_Service/internal/logging"
)

const historyAcceptanceChild = "DIPLOMA_HISTORY_ACCEPTANCE_CHILD"

type acceptanceLegacySink struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *acceptanceLegacySink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.Write(p)
}

func (s *acceptanceLegacySink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

// Observe the actual server context supplied to the production logger. This is
// a sink adapter, not an additional or replacement gRPC interceptor.
type acceptanceContextState struct {
	mu     sync.Mutex
	errors []error
}

type acceptanceContextHandler struct {
	slog.Handler
	state *acceptanceContextState
}

func (h acceptanceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "operation.finished" {
		h.state.mu.Lock()
		h.state.errors = append(h.state.errors, ctx.Err())
		h.state.mu.Unlock()
	}
	return h.Handler.Handle(ctx, r)
}

func (h acceptanceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return acceptanceContextHandler{h.Handler.WithAttrs(attrs), h.state}
}

func (h acceptanceContextHandler) WithGroup(name string) slog.Handler {
	return acceptanceContextHandler{h.Handler.WithGroup(name), h.state}
}

func historyDuplicateGuard(records []pilotEvent, legacy string) error {
	if len(records) != 1 || records[0].Event != "operation.finished" || records[0].ID == "" {
		return fmt.Errorf("want one identified terminal record, got %d", len(records))
	}
	for _, line := range strings.Split(legacy, "\n") {
		// These are the repository/service/transport diagnostics of the same
		// history read, not authentication/security or database server logs.
		for _, prefix := range []string{
			"Error while getting all messages:",
			"messageRepo.GetMessagesByChatID error:",
			"grpc error getting message:",
		} {
			if strings.Contains(line, prefix) {
				return fmt.Errorf("duplicate history diagnostic without operation_id: %s", line)
			}
		}
	}
	return nil
}

func TestAcceptanceLegacyDuplicateGuard(t *testing.T) {
	record := pilotEvent{Event: "operation.finished", ID: "expected-operation"}
	for _, tc := range []struct {
		name      string
		records   []pilotEvent
		legacy    string
		wantError bool
	}{
		{"single completion", []pilotEvent{record}, "", false},
		{"two completions", []pilotEvent{record, record}, "", true},
		{"original repository and service pair", []pilotEvent{record}, "Error while getting all messages: pq: canceling statement due to user request\nerror: messageRepo.GetMessagesByChatID error: pq: canceling statement due to user request", true},
		{"repository only", []pilotEvent{record}, "Error while getting all messages: pq: canceling statement due to user request", true},
		{"service only", []pilotEvent{record}, "error: messageRepo.GetMessagesByChatID error: pq: canceling statement due to user request", true},
		{"transport only", []pilotEvent{record}, "grpc error getting message: pq: canceling statement due to user request", true},
		{"independent security and infrastructure", []pilotEvent{record}, "authenticate gRPC request: independent failure\nPostgreSQL backend: canceling statement due to user request", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := historyDuplicateGuard(tc.records, tc.legacy)
			if (err != nil) != tc.wantError {
				t.Fatalf("guard=%v, want rejection=%v", err, tc.wantError)
			}
		})
	}
}

func TestAcceptanceGRPCPostgresCancellation(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	if os.Getenv(historyAcceptanceChild) != "1" {
		// Standard log output is process-global. Isolate capture from every other
		// test, including tests running in parallel; inherit this race-built binary.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcceptanceGRPCPostgresCancellation$", "-test.v", "-test.timeout=75s")
		cmd.Env = append(os.Environ(), historyAcceptanceChild+"=1")
		output, err := cmd.CombinedOutput()
		t.Logf("isolated production-chain check:\n%s", output)
		if err != nil {
			t.Fatalf("production-chain subprocess: %v (context=%v)", err, ctx.Err())
		}
		return
	}

	var legacy acceptanceLegacySink
	oldWriter := log.Writer()
	log.SetOutput(&legacy)
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	f := newCancellationFixture(t)
	before := f.snapshot(t)
	barrier := f.readBarrier(t, "messages", "SELECT id, chat_id, sender_id, content")
	sink := newPilotSink()
	state := &acceptanceContextState{}
	logger := slog.New(acceptanceContextHandler{
		Handler: slog.NewJSONHandler(sink, &slog.HandlerOptions{Level: slog.LevelDebug}), state: state,
	})
	server := grpcapi.NewGRPCServer(f.access, logger)
	server.RegisterServices(f.users, f.messages, f.chats, f.participants, f.access)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	served := make(chan struct{})
	go func() { defer close(served); _ = server.Serve(listener) }()
	requestContext, cancel := context.WithCancel(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+f.token))
	var conn *grpc.ClientConn
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			barrier.release() // Failure cleanup must unblock SQL before draining.
			if conn != nil {
				conn.Close()
			}
			ctx, done := context.WithTimeout(context.Background(), testWaitTimeout)
			defer done()
			stopped := make(chan struct{})
			go func() { server.Shutdown(ctx); close(stopped) }()
			select {
			case <-stopped:
			case <-ctx.Done():
				t.Error("gRPC shutdown exceeded cleanup deadline")
			}
			listener.Close()
			select {
			case <-served:
			case <-time.After(testWaitTimeout):
				t.Error("gRPC Serve did not finish")
			}
		})
	}
	t.Cleanup(stop)
	conn, err = grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	operation := startTestOperation(t, func() { cancel(); barrier.release() }, func() error {
		_, err := messagepb.NewMessageServiceClient(conn).GetMessagesByChatID(requestContext, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
		return err
	})
	pid := f.waitBlocked(t, barrier.pid, "SELECT id, chat_id, sender_id, content")
	t.Logf("before cancel: PostgreSQL pid=%d waits for exact blocker=%d", pid, barrier.pid)
	cancel()
	if err := receiveTestError(t, operation); status.Code(err) != codes.Canceled {
		t.Fatalf("client code=%v, want Canceled", err)
	}
	if !errors.Is(requestContext.Err(), context.Canceled) {
		t.Fatalf("client context=%v", requestContext.Err())
	}
	event := sink.wait(t)
	if event.Level != "ERROR" || event.Code != "Canceled" {
		t.Fatalf("wrong level or server transport status: %+v", event)
	}
	for key, want := range map[string]string{
		"sqlstate": "57014", "error_type": "*pq.Error", "error_class": "query_canceled",
		"stage": "query", "operation": logging.MessageHistory, "context_state": "canceled", "outcome": "failed",
	} {
		if event.Diagnosis[key] != want {
			t.Errorf("diagnosis[%s]=%v, want %s", key, event.Diagnosis[key], want)
		}
	}
	state.mu.Lock()
	if len(state.errors) != 1 || !errors.Is(state.errors[0], context.Canceled) {
		t.Errorf("server logging contexts=%v, want exactly one canceled context", state.errors)
	}
	state.mu.Unlock()
	f.waitStopped(t, pid)
	ctx, done := context.WithTimeout(context.Background(), testWaitTimeout)
	var held bool
	err = f.observer.GetContext(ctx, &held, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND granted)", barrier.pid)
	done()
	if err != nil || !held {
		t.Fatalf("blocker released before SQL termination proof: held=%v err=%v", held, err)
	}
	t.Logf("after cancel: pid=%d SQL stopped and locks released while blocker=%d is STILL held", pid, barrier.pid)
	barrier.release()
	stop() // Drain handlers before asserting that no late duplicate was emitted.
	if err := historyDuplicateGuard(sink.events(t), legacy.text()); err != nil {
		t.Fatal(err)
	}
	if f.snapshot(t) != before {
		t.Fatal("canceled read changed persisted state")
	}
	t.Logf("operation_id=%s sqlstate=57014 type=*pq.Error class=query_canceled stage=query level=ERROR rpc=Canceled client/server contexts=canceled; exactly 1 completion, 0 legacy history duplicates", event.ID)
}
