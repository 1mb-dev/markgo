package serve

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

// TestGracefulShutdown_CleanupRunsOnError is the regression test for the
// pre-v3.11.0 bug where a server.Shutdown error caused HandleCLIError to
// os.Exit before the templateService / sessionStore / rate-limiter cleanups
// could run. The function now decouples the two so callers can run all
// cleanups before deciding what to do with the shutdown error.
func TestGracefulShutdown_CleanupRunsOnError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	shutdownErr := errors.New("context deadline exceeded")

	var ran []string
	cleanups := []func(){
		func() { ran = append(ran, "template") },
		func() { ran = append(ran, "session") },
		func() { ran = append(ran, "ratelimit") },
	}

	stubShutdown := func(_ context.Context) error { return shutdownErr }

	gotErr := gracefulShutdown(context.Background(), stubShutdown, cleanups, logger)

	if !errors.Is(gotErr, shutdownErr) {
		t.Fatalf("expected shutdown error to propagate, got %v", gotErr)
	}
	if len(ran) != 3 || ran[0] != "template" || ran[1] != "session" || ran[2] != "ratelimit" {
		t.Fatalf("expected all three cleanups to run in order, got %v", ran)
	}
}

func TestGracefulShutdown_CleanupRunsOnSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var ran int
	cleanups := []func(){
		func() { ran++ },
		func() { ran++ },
	}

	stubShutdown := func(_ context.Context) error { return nil }

	if err := gracefulShutdown(context.Background(), stubShutdown, cleanups, logger); err != nil {
		t.Fatalf("expected nil shutdown error, got %v", err)
	}
	if ran != 2 {
		t.Fatalf("expected 2 cleanups to run on success, got %d", ran)
	}
}

func TestGracefulShutdown_NoCleanups(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	stubShutdown := func(_ context.Context) error { return nil }

	if err := gracefulShutdown(context.Background(), stubShutdown, nil, logger); err != nil {
		t.Fatalf("expected nil shutdown error with no cleanups, got %v", err)
	}
}
