// Package logging provides logging policy without owning application operations.
// It does not install a global logger or emit records on its own.
package logging

import (
	"io"
	"log/slog"
)

// New creates a synchronous JSON logger. A nil level means INFO.
// The caller owns the non-nil writer and its lifetime.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
