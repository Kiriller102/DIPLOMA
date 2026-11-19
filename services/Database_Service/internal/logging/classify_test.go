package logging_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"project/services/Database_Service/internal/logging"
)

func TestClassification(t *testing.T) {
	invalid, unauthenticated, forbidden := errors.New("invalid input"), errors.New("unauthenticated"), errors.New("forbidden")
	c := logging.NewClassifier(logging.Sentinels{InvalidInput: invalid, Unauthenticated: unauthenticated, Forbidden: forbidden})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	pgCanceled := &pq.Error{Code: "57014"}
	pgInternal := &pq.Error{Code: "XX000"}
	for _, tc := range []struct {
		name    string
		err     error
		ctx     context.Context
		opts    logging.Options
		level   slog.Level
		outcome string
		class   string
	}{
		{name: "success", level: slog.LevelInfo, outcome: "ok"},
		{name: "health success", opts: logging.Options{Kind: logging.HealthCheck}, level: slog.LevelDebug, outcome: "ok"},
		{name: "poll timeout", opts: logging.Options{Kind: logging.LongPollTimeout}, level: slog.LevelDebug, outcome: "ok"},
		{name: "wrapped input", err: fmt.Errorf("private detail: %w", invalid), level: slog.LevelInfo, outcome: "rejected", class: "invalid_input"},
		{name: "unauthenticated", err: unauthenticated, level: slog.LevelInfo, outcome: "rejected", class: "unauthenticated"},
		{name: "forbidden", err: forbidden, level: slog.LevelInfo, outcome: "rejected", class: "forbidden"},
		{name: "missing", err: fmt.Errorf("lookup: %w", sql.ErrNoRows), level: slog.LevelInfo, outcome: "rejected", class: "not_found"},
		{name: "unique", err: &pq.Error{Code: "23505"}, level: slog.LevelInfo, outcome: "rejected", class: "conflict"},
		{name: "reference", err: &pq.Error{Code: "23503"}, level: slog.LevelInfo, outcome: "rejected", class: "invalid_input"},
		{name: "invalid text", err: &pq.Error{Code: "22P02"}, level: slog.LevelInfo, outcome: "rejected", class: "invalid_input"},
		{name: "too long", err: &pq.Error{Code: "22001"}, level: slog.LevelInfo, outcome: "rejected", class: "invalid_input"},
		{name: "null", err: &pq.Error{Code: "23502"}, level: slog.LevelInfo, outcome: "rejected", class: "invalid_input"},
		{name: "wrapped cancel", err: fmt.Errorf("query: %w", context.Canceled), level: slog.LevelDebug, outcome: "canceled", class: "canceled"},
		{name: "driver cancel", err: pgCanceled, ctx: canceled, level: slog.LevelError, outcome: "failed", class: "query_canceled"},
		{name: "driver deadline", err: pgCanceled, ctx: deadline, level: slog.LevelError, outcome: "failed", class: "query_canceled"},
		{name: "client deadline", err: context.DeadlineExceeded, opts: logging.Options{Source: logging.SourceClient}, level: slog.LevelInfo, outcome: "deadline_exceeded", class: "deadline_exceeded"},
		{name: "server deadline", err: context.DeadlineExceeded, opts: logging.Options{Source: logging.SourceServer}, level: slog.LevelWarn, outcome: "deadline_exceeded", class: "deadline_exceeded"},
		{name: "live query cancellation", err: pgCanceled, level: slog.LevelError, outcome: "failed", class: "query_canceled"},
		{name: "internal after cancel", err: pgInternal, ctx: canceled, level: slog.LevelError, outcome: "failed", class: "database"},
		{name: "internal after deadline", err: pgInternal, ctx: deadline, level: slog.LevelError, outcome: "failed", class: "database"},
		{name: "unknown after cancel", err: errors.New("private cause"), ctx: canceled, level: slog.LevelError, outcome: "failed", class: "internal"},
		{name: "joined cancel then failure", err: errors.Join(context.Canceled, pgInternal), ctx: canceled, level: slog.LevelError, outcome: "failed", class: "database"},
		{name: "joined failure then cancel", err: errors.Join(pgInternal, context.Canceled), ctx: canceled, level: slog.LevelError, outcome: "failed", class: "database"},
		{name: "joined rejection then failure", err: errors.Join(invalid, pgInternal), level: slog.LevelError, outcome: "failed", class: "database"},
		{name: "wrapped joined failure", err: fmt.Errorf("operation: %w", errors.Join(pgCanceled, pgInternal)), ctx: canceled, level: slog.LevelError, outcome: "failed", class: "query_canceled"},
		{name: "health failure", err: pgInternal, opts: logging.Options{Kind: logging.HealthCheck}, level: slog.LevelWarn, outcome: "failed", class: "database"},
		{name: "health client cancel", err: context.Canceled, opts: logging.Options{Kind: logging.HealthCheck}, level: slog.LevelDebug, outcome: "canceled", class: "canceled"},
		{name: "startup rejected", err: invalid, opts: logging.Options{Kind: logging.Startup}, level: slog.LevelError, outcome: "rejected", class: "invalid_input"},
		{name: "shutdown forced", opts: logging.Options{Kind: logging.ShutdownTimeout}, level: slog.LevelWarn, outcome: "timeout", class: "shutdown_timeout"},
		{name: "shutdown deadline", err: context.DeadlineExceeded, opts: logging.Options{Kind: logging.ShutdownTimeout}, level: slog.LevelWarn, outcome: "timeout", class: "shutdown_timeout"},
		{name: "shutdown independent failure", err: pgInternal, opts: logging.Options{Kind: logging.ShutdownTimeout}, level: slog.LevelError, outcome: "failed", class: "database"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			d := c.Classify(ctx, tc.err, tc.opts)
			event := recordDiagnosis(t, d)
			if d.Level() != tc.level || event["outcome"] != tc.outcome || stringField(event, "error_class") != tc.class {
				t.Fatalf("got level=%v diagnosis=%v; want level=%v outcome=%s class=%s", d.Level(), event, tc.level, tc.outcome, tc.class)
			}
		})
	}
}

func TestGRPCClassification(t *testing.T) {
	for _, tc := range []struct {
		code  codes.Code
		level slog.Level
		class string
	}{
		{codes.Canceled, slog.LevelDebug, "canceled"},
		{codes.DeadlineExceeded, slog.LevelWarn, "deadline_exceeded"},
		{codes.InvalidArgument, slog.LevelInfo, "invalid_input"},
		{codes.OutOfRange, slog.LevelInfo, "invalid_input"},
		{codes.NotFound, slog.LevelInfo, "not_found"},
		{codes.AlreadyExists, slog.LevelInfo, "conflict"},
		{codes.Unauthenticated, slog.LevelInfo, "unauthenticated"},
		{codes.PermissionDenied, slog.LevelInfo, "forbidden"},
		{codes.ResourceExhausted, slog.LevelInfo, "resource_exhausted"},
		{codes.Internal, slog.LevelError, "rpc"},
		{codes.Unavailable, slog.LevelError, "rpc"},
		{codes.Unknown, slog.LevelError, "rpc"},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			d := (logging.Classifier{}).Classify(context.Background(), fmt.Errorf("private wrapper: %w", status.Error(tc.code, "private status")), logging.Options{})
			if got := recordDiagnosis(t, d); d.Level() != tc.level || got["error_class"] != tc.class {
				t.Fatalf("level=%v diagnosis=%v", d.Level(), got)
			}
		})
	}
}

func TestDiagnosticPrivacyAndErrorIdentity(t *testing.T) {
	const secret = "token_password_email_key_DSN_SECRET"
	pg := &pq.Error{Code: "XX000", Message: secret, Detail: secret, Hint: secret, Where: secret, Table: secret}
	err := logging.Annotate(fmt.Errorf("%s: %w", secret, pg), "message.get_by_chat", "query")
	if err.Error() != fmt.Errorf("%s: %w", secret, pg).Error() || !errors.Is(err, pg) {
		t.Fatal("annotation changed error text or identity")
	}
	var found *pq.Error
	if !errors.As(err, &found) || found != pg {
		t.Fatal("annotation lost PostgreSQL error")
	}
	d := (logging.Classifier{}).Classify(context.Background(), err, logging.Options{})
	got := recordDiagnosis(t, d)
	encoded, _ := json.Marshal(got)
	if bytes.Contains(encoded, []byte(secret)) {
		t.Fatalf("sensitive error details leaked: %s", encoded)
	}
	for key, want := range map[string]string{"sqlstate": "XX000", "error_type": "*pq.Error", "operation": "message.get_by_chat", "stage": "query"} {
		if got[key] != want {
			t.Errorf("%s=%v, want %s", key, got[key], want)
		}
	}
	if logging.Annotate(nil, "unused", "unused") != nil {
		t.Fatal("annotating nil created an error")
	}
	if got := recordDiagnosis(t, (logging.Classifier{}).Classify(context.Background(), errors.New(secret), logging.Options{})); strings.Contains(fmt.Sprint(got), secret) {
		t.Fatal("unknown error message leaked")
	}
}

func TestDiagnosticMetadata(t *testing.T) {
	c := logging.Classifier{}
	err := logging.Annotate(logging.Annotate(&pq.Error{Code: "XX000"}, "", "query"), "message.read", "service")
	got := recordDiagnosis(t, c.Classify(context.Background(), err, logging.Options{}))
	if got["stage"] != "query" || got["operation"] != "message.read" {
		t.Fatalf("wrong cause metadata: %v", got)
	}
	got = recordDiagnosis(t, c.Classify(context.Background(), logging.Annotate(&pq.Error{Code: "token-secret"}, "https://secret", strings.Repeat("x", 65)), logging.Options{}))
	if got["operation"] != "unknown" || got["stage"] != "unknown" || got["sqlstate"] != "unknown" {
		t.Fatalf("invalid diagnostic labels accepted: %v", got)
	}
	for _, source := range []logging.Source{logging.SourceClient, logging.SourceServer, logging.SourceShutdown, logging.SourceUnknown, "private-value"} {
		got := recordDiagnosis(t, c.Classify(context.Background(), context.Canceled, logging.Options{Source: source}))
		want := source
		if source == "private-value" {
			want = logging.SourceUnknown
		}
		if got["source"] != string(want) {
			t.Fatalf("source=%v, want %v", got["source"], want)
		}
	}
}

func recordDiagnosis(t *testing.T, d logging.Diagnosis) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelDebug).LogAttrs(context.Background(), d.Level(), "test.operation", slog.Any("diagnosis", d))
	var event struct {
		Diagnosis map[string]any `json:"diagnosis"`
	}
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	return event.Diagnosis
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}
