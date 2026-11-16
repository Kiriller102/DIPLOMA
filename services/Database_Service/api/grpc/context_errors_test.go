package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Unit checks isolate the production interceptor from transport-side rewriting.
// A canceled gRPC client alone could hide an incorrect Internal server result.
func TestContextStageCMapErrors(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		ctxerr, errorReturned error
		want                  codes.Code
	}{
		{"canceled", context.Canceled, context.Canceled, codes.Canceled},
		{"wrapped_canceled", context.Canceled, fmt.Errorf("query: %w", context.Canceled), codes.Canceled},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded, codes.DeadlineExceeded},
		{"driver_after_cancel", context.Canceled, &pq.Error{Code: "57014"}, codes.Canceled},
		{"driver_after_deadline", context.DeadlineExceeded, &pq.Error{Code: "57014"}, codes.DeadlineExceeded},
		{"driver_with_live_context", nil, &pq.Error{Code: "57014"}, codes.Internal},
		{"ordinary_database_error", nil, &pq.Error{Code: "XX000"}, codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ctx context.Context = context.Background()
			if tc.ctxerr == context.Canceled {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			if tc.ctxerr == context.DeadlineExceeded {
				c, cancel := context.WithDeadline(ctx, time.Unix(0, 0))
				defer cancel()
				ctx = c
			}
			_, err := mapErrors(ctx, nil, &grpc.UnaryServerInfo{}, func(context.Context, interface{}) (interface{}, error) { return nil, tc.errorReturned })
			if status.Code(err) != tc.want {
				t.Errorf("server interceptor returned %v, want %s", err, tc.want)
			}
		})
	}
}
