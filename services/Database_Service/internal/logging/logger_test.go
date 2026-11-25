package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"project/services/Database_Service/internal/logging"
)

func TestLoggerFilteringAndIsolation(t *testing.T) {
	global := slog.Default()
	var buf bytes.Buffer
	level := new(slog.LevelVar)
	l := logging.New(&buf, level)
	l.Debug("filtered")
	l.Info("visible")
	l.Warn("warning")
	l.Error("failure")
	level.Set(slog.LevelDebug)
	l.Debug("enabled")
	want := []string{"INFO", "WARN", "ERROR", "DEBUG"}
	decoder := json.NewDecoder(&buf)
	for _, expected := range want {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event["level"] != expected {
			t.Fatalf("got %v, want %s", event, expected)
		}
	}
	if slog.Default() != global {
		t.Fatal("logger factory replaced the global logger")
	}
	var defaultBuf bytes.Buffer
	logging.New(&defaultBuf, nil).Debug("filtered by default")
	if defaultBuf.Len() != 0 {
		t.Fatal("nil level must default to INFO")
	}
}

func TestConcurrentIndependentRecords(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(&buf, slog.LevelDebug)
	var wg sync.WaitGroup
	for id := 0; id < 40; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			l.With("operation_id", id).LogAttrs(context.Background(), slog.LevelInfo, "same independent error")
		}(id)
	}
	wg.Wait()
	seen := make(map[int]bool)
	decoder := json.NewDecoder(&buf)
	for decoder.More() {
		var event struct {
			ID int `json:"operation_id"`
		}
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if seen[event.ID] {
			t.Fatalf("duplicate operation id %d", event.ID)
		}
		seen[event.ID] = true
	}
	if len(seen) != 40 {
		t.Fatalf("got %d records, want 40; independent equal messages must not be deduplicated", len(seen))
	}
}
