package test

import (
	"context"
	"database/sql/driver"
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/emptypb"
	gs "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/services"
)

func TestContextStageCAlreadyCanceled(t *testing.T) {
	for _, transport := range []string{"REST", "gRPC"} {
		t.Run(transport, func(t *testing.T) {
			var queries atomic.Int32
			db, _ := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				queries.Add(1)
				return &messageRows{columns: []string{"id"}}, nil
			})
			users, err := services.NewUserService(db)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var run func() error
			var writes *handlerResponseWrites
			if transport == "REST" {
				router := gin.New()
				h, w := observeHandlerWrites(controllers.NewUserController(users, nil).GetUsers)
				writes = w
				router.GET("/users/all", h)
				req := httptest.NewRequest("GET", "/users/all", nil).WithContext(ctx)
				run = func() error { router.ServeHTTP(httptest.NewRecorder(), req); return nil }
			} else {
				h := gs.NewUserGRPCsService(users, nil)
				run = func() error { _, e := h.GetAllUsers(ctx, &emptypb.Empty{}); return e }
			}
			err = receiveTestError(t, startTestOperation(t, cancel, run))
			if queries.Load() != 0 {
				t.Errorf("DEFECT: already canceled request executed %d SQL queries", queries.Load())
			}
			if transport == "REST" {
				writes.assertNone(t)
			} else if !errors.Is(err, context.Canceled) {
				t.Errorf("expected cancellation, got %v", err)
			}
		})
	}
}

type cancelDuringRows struct {
	messageRows
	ctx     context.Context
	reading chan struct{}
	closed  atomic.Bool
	first   bool
}

func (r *cancelDuringRows) Next(dest []driver.Value) error {
	if !r.first {
		r.first = true
		return r.messageRows.Next(dest)
	}
	close(r.reading)
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case <-time.After(testWaitTimeout):
		return errors.New("row iteration did not receive cancellation")
	}
}
func (r *cancelDuringRows) Close() error { r.closed.Store(true); return nil }

func TestContextStageCCancelDuringRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rows := &cancelDuringRows{ctx: ctx, reading: make(chan struct{}), messageRows: messageRows{
		columns: []string{"id", "chat_id", "sender_id", "content", "encryption_algorithm", "sent_at", "crypto_version", "client_id"},
		values:  [][]driver.Value{{int64(1), int64(1), int64(1), "ciphertext", "AES", time.Now(), int64(1), "client"}},
	}}
	_, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return rows, nil })
	done := startTestOperation(t, cancel, func() error {
		messages, err := svc.GetMessagesByChatID(ctx, 1)
		if err == nil || len(messages) != 0 {
			return errors.New("partial result incorrectly returned as successful read")
		}
		return err
	})
	select {
	case <-rows.reading:
	case <-time.After(testWaitTimeout):
		t.Fatal("did not reach iteration after first row")
	}
	cancel()
	if err := receiveTestError(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if !rows.closed.Load() {
		t.Error("rows were not closed")
	}
}
