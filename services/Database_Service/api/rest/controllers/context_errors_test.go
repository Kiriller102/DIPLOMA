package controllers

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type contextErrorWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *contextErrorWriter) WriteHeader(code int) { w.writes++; w.ResponseRecorder.WriteHeader(code) }
func (w *contextErrorWriter) Write(b []byte) (int, error) {
	w.writes++
	return w.ResponseRecorder.Write(b)
}

func TestContextStageCRESTErrors(t *testing.T) {
	for _, name := range []string{"canceled", "deadline", "driver_after_cancel", "live_database_error"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var err error = &pq.Error{Code: "XX000"}
			if name == "deadline" {
				c, cancel := context.WithDeadline(ctx, time.Unix(0, 0))
				defer cancel()
				ctx = c
				err = context.DeadlineExceeded
			} else if name != "live_database_error" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				err = context.Canceled
				if name == "driver_after_cancel" {
					err = &pq.Error{Code: "57014"}
				}
			}
			writer := &contextErrorWriter{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			respondError(c, err)
			if ctx.Err() != nil && writer.writes != 0 {
				t.Errorf("response written for completed request: calls=%d status=%d", writer.writes, writer.Code)
			}
			if ctx.Err() == nil && writer.Code != 500 {
				t.Errorf("live database failure status=%d, want 500", writer.Code)
			}
		})
	}
}
