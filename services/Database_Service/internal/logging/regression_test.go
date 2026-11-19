package logging_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"project/services/Database_Service/internal/logging"
)

func TestQueryCanceledDoesNotProveContextCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	for _, ctx := range []context.Context{context.Background(), canceled, deadline} {
		for _, message := range []string{"canceling statement due to user request", "canceling statement due to statement timeout", ""} {
			pg := &pq.Error{Code: "57014", Message: message}
			for _, err := range []error{pg, fmt.Errorf("query: %w", pg), errors.Join(context.Canceled, pg)} {
				d := (logging.Classifier{}).Classify(ctx, err, logging.Options{Source: logging.SourceClient})
				got := recordDiagnosis(t, d)
				if d.Level() != slog.LevelError || got["error_class"] != "query_canceled" || got["sqlstate"] != "57014" {
					t.Fatalf("ctx=%v message=%q: level=%v diagnosis=%v", ctx.Err(), message, d.Level(), got)
				}
				if got["source"] != nil {
					t.Fatalf("unproven cancellation attributed to client: %v", got)
				}
				want := ""
				if ctx.Err() == context.Canceled {
					want = "canceled"
				} else if ctx.Err() == context.DeadlineExceeded {
					want = "deadline_exceeded"
				}
				if stringField(got, "context_state") != want {
					t.Fatalf("context_state=%v, want %q", got["context_state"], want)
				}
			}
		}
	}
}

func TestJoinedDiagnosticsPreserveAllCausesAndAnnotations(t *testing.T) {
	const secret = "PRIVATE_MESSAGE_DETAIL_WRAPPER"
	first := &pq.Error{Code: "23505", Message: secret, Detail: secret}
	second := &pq.Error{Code: "XX000", Message: secret, Detail: secret}
	third := &pq.Error{Code: "08006", Message: secret, Detail: secret}
	left := logging.Annotate(first, "user.create", "insert")
	right := logging.Annotate(logging.Annotate(second, "message.read", "query"), "message.service", "read")
	inner := errors.Join(right, logging.Annotate(third, "database.close", "close"))
	joined := errors.Join(left, fmt.Errorf("%s: %w", secret, inner))
	err := logging.Annotate(joined, "application.run", "finish")
	if err.Error() != joined.Error() {
		t.Fatal("Annotate changed joined error text")
	}
	for _, target := range []error{first, second, third} {
		if !errors.Is(err, target) {
			t.Fatal("Annotate lost a joined branch")
		}
	}
	var pg *pq.Error
	if !errors.As(err, &pg) || pg != first {
		t.Fatal("Annotate changed standard errors.As traversal")
	}
	d := (logging.Classifier{}).Classify(context.Background(), err, logging.Options{})
	got := recordDiagnosis(t, d)
	if d.Level() != slog.LevelError || got["sqlstate"] != "XX000" || got["operation"] != "message.read" || got["stage"] != "query" {
		t.Fatalf("wrong most severe cause or tie order: %v", got)
	}
	causes, ok := got["causes"].([]any)
	if !ok || len(causes) != 3 {
		t.Fatalf("independent causes lost: %v", got)
	}
	for i, code := range []string{"23505", "XX000", "08006"} {
		cause := causes[i].(map[string]any)
		if cause["sqlstate"] != code {
			t.Fatalf("cause %d=%v, want SQLSTATE %s", i, cause, code)
		}
		wantLevel := "ERROR"
		if i == 0 {
			wantLevel = "INFO"
		}
		if cause["level"] != wantLevel {
			t.Fatalf("cause %d level=%v, want %s", i, cause["level"], wantLevel)
		}
		frames := arrayField(t, cause, "contexts")
		last := frames[len(frames)-1].(map[string]any)
		if last["operation"] != "application.run" || last["stage"] != "finish" {
			t.Fatalf("common annotation missing from cause %d: %v", i, cause)
		}
	}
	frames := arrayField(t, got, "contexts")
	if len(frames) != 3 {
		t.Fatalf("nested annotation lost: %v", frames)
	}
	for i, operation := range []string{"message.read", "message.service", "application.run"} {
		if frames[i].(map[string]any)["operation"] != operation {
			t.Fatalf("wrong annotation order: %v", frames)
		}
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), secret) {
		t.Fatal("private error data leaked from a joined branch")
	}
}

func TestJoinPreservesEqualIndependentErrors(t *testing.T) {
	first, second := &pq.Error{Code: "XX000"}, &pq.Error{Code: "08006"}
	for _, order := range [][]error{{first, second}, {second, first}, {first, first}} {
		err := logging.Annotate(fmt.Errorf("wrapped: %w", errors.Join(nil, order[0], order[1])), "application", "finish")
		got := recordDiagnosis(t, (logging.Classifier{}).Classify(context.Background(), err, logging.Options{}))
		causes := arrayField(t, got, "causes")
		if len(causes) != 2 {
			t.Fatalf("same errors deduplicated: %v", got)
		}
		if got["sqlstate"] != string(order[0].(*pq.Error).Code) {
			t.Fatalf("tie order changed: %v", got)
		}
	}
}

func TestAnnotationFramesKeepPairsAndSiblingIsolation(t *testing.T) {
	base := logging.Annotate(errors.New("private"), "", "query")
	left := logging.Annotate(base, "left.read", "service")
	right := logging.Annotate(base, "right.read", "other")
	err := logging.Annotate(errors.Join(left, right), "request", "")
	got := recordDiagnosis(t, (logging.Classifier{}).Classify(context.Background(), err, logging.Options{}))
	causes := arrayField(t, got, "causes")
	if len(causes) != 2 {
		t.Fatalf("got %d causes, want 2", len(causes))
	}
	for i, name := range []string{"left.read", "right.read"} {
		cause := causes[i].(map[string]any)
		frames := arrayField(t, cause, "contexts")
		if len(frames) != 3 || frames[0].(map[string]any)["stage"] != "query" || frames[1].(map[string]any)["operation"] != name || frames[2].(map[string]any)["operation"] != "request" {
			t.Fatalf("paired annotations or sibling isolation lost: %v", frames)
		}
	}
	// The same input error can be classified repeatedly without accumulating frames.
	for i := 0; i < 2; i++ {
		if frames := arrayField(t, recordDiagnosis(t, (logging.Classifier{}).Classify(context.Background(), base, logging.Options{})), "contexts"); len(frames) != 1 {
			t.Fatalf("classification mutated annotation chain: %v", frames)
		}
	}
}

func arrayField(t *testing.T, fields map[string]any, key string) []any {
	t.Helper()
	values, ok := fields[key].([]any)
	if !ok {
		t.Fatalf("missing or invalid %s: %v", key, fields)
	}
	return values
}
