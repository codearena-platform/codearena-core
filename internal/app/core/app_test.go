package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStartWithContextReturnsPromptlyWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{
		GRPCPort: "127.0.0.1:0",
		WebPort:  "127.0.0.1:0",
		DBPath:   filepath.Join(t.TempDir(), "core.db"),
	}

	done := make(chan error, 1)
	go func() {
		done <- StartWithContext(ctx, cfg)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartWithContext: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StartWithContext did not shut down after context cancellation")
	}
}
