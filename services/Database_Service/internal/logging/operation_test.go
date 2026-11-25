package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"project/services/Database_Service/internal/logging"

	"github.com/lib/pq"
)

func TestCompletionConcurrentFinishAndOriginalCause(t *testing.T) {
	var output bytes.Buffer
	ctx, completion := logging.Begin(context.Background(), logging.New(&output, slog.LevelDebug), logging.Classifier{}, "direct", logging.MessageHistory)
	pg := &pq.Error{Code: "57014", Message: "PRIVATE"}
	logging.Capture(ctx, pg, "query")
	logging.Capture(ctx, context.Canceled, "transport")
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); completion.Finish(ctx) }()
	}
	wg.Wait()
	logging.Capture(ctx, errors.New("late failure"), "late")
	completion.Finish(ctx)
	decoder := json.NewDecoder(&output)
	var event struct {
		Level     string         `json:"level"`
		Diagnosis map[string]any `json:"diagnosis"`
	}
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	if event.Level != "ERROR" || event.Diagnosis["sqlstate"] != "57014" || event.Diagnosis["stage"] != "query" {
		t.Fatalf("original cause lost: %+v", event)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("second completion: %v", err)
	}
}

func TestDirectOperationsDoNotDeduplicateIndependentErrors(t *testing.T) {
	var output bytes.Buffer
	logger := logging.New(&output, slog.LevelDebug)
	failure := errors.New("same private error")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := logging.Run(context.Background(), logger, logging.Classifier{}, logging.MessageHistory, func(context.Context) error { return failure })
			if err != failure {
				t.Error("Run replaced callback error")
			}
		}()
	}
	wg.Wait()
	seen := make(map[string]bool)
	decoder := json.NewDecoder(&output)
	for {
		var event struct {
			ID string `json:"operation_id"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.ID == "" || seen[event.ID] {
			t.Fatalf("invalid/duplicate operation ID %q", event.ID)
		}
		seen[event.ID] = true
	}
	if len(seen) != 20 {
		t.Fatalf("got %d independent records", len(seen))
	}
}
